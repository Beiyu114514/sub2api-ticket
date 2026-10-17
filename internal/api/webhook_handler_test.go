package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/glebarez/sqlite"

	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/store"
	"sub2api-ticket/internal/tgbot"
)

const cleanWebhookPath = "/api/v1/telegram/webhook"

func newTestDeps(t *testing.T, secret string) (Deps, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "webhook.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := g.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	settings := store.NewSettings(g)
	if secret != "" {
		if err := settings.Set(store.KeyWebhookSecret, secret); err != nil {
			t.Fatalf("set secret: %v", err)
		}
	}
	core := &tgbot.Core{G: g, S: settings, Pool: tgbot.NewWorkerPool(4, 32)}
	deps := Deps{G: g, Settings: settings, BotCore: core}

	r := gin.New()
	wh := &WebhookHandler{Deps: deps}
	r.POST(cleanWebhookPath, wh.Handle)
	r.POST(cleanWebhookPath+"/:secret", wh.HandleLegacy)
	return deps, r
}

func postWebhook(t *testing.T, r *gin.Engine, urlPath, headerSecret string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"update_id":1,"message":{"message_id":1,"chat":{"id":2,"type":"private"},"text":"hi"}}`
	req := httptest.NewRequest(http.MethodPost, urlPath, strings.NewReader(body))
	if headerSecret != "" {
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", headerSecret)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestWebhookSecretConstantTimeCompare 回归 ISSUE-02：secret 校验走恒定时间比较。
// 新地址：secret 只走 header；旧地址：URL 段 + header 双因子。
func TestWebhookSecretConstantTimeCompare(t *testing.T) {
	const secret = "s3cr3t-token"

	t.Run("header match -> 200", func(t *testing.T) {
		_, r := newTestDeps(t, secret)
		w := postWebhook(t, r, cleanWebhookPath, secret)
		if w.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200", w.Code)
		}
	})

	t.Run("missing header -> 403", func(t *testing.T) {
		_, r := newTestDeps(t, secret)
		w := postWebhook(t, r, cleanWebhookPath, "")
		if w.Code != http.StatusForbidden {
			t.Fatalf("code = %d, want 403", w.Code)
		}
	})

	t.Run("wrong header secret -> 403", func(t *testing.T) {
		_, r := newTestDeps(t, secret)
		w := postWebhook(t, r, cleanWebhookPath, "wrong")
		if w.Code != http.StatusForbidden {
			t.Fatalf("code = %d, want 403", w.Code)
		}
	})

	t.Run("legacy url + header both match -> 200", func(t *testing.T) {
		_, r := newTestDeps(t, secret)
		w := postWebhook(t, r, cleanWebhookPath+"/"+secret, secret)
		if w.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200", w.Code)
		}
	})

	t.Run("legacy wrong url secret -> 403", func(t *testing.T) {
		_, r := newTestDeps(t, secret)
		w := postWebhook(t, r, cleanWebhookPath+"/wrong", secret)
		if w.Code != http.StatusForbidden {
			t.Fatalf("code = %d, want 403", w.Code)
		}
	})

	t.Run("legacy secret prefix match must not pass", func(t *testing.T) {
		_, r := newTestDeps(t, secret)
		// 常量时间比较必须要求完整相等：前缀相同但长度不同一律拒绝
		prefix := secret[:len(secret)-1]
		w := postWebhook(t, r, cleanWebhookPath+"/"+prefix, prefix)
		if w.Code != http.StatusForbidden {
			t.Fatalf("code = %d, want 403", w.Code)
		}
	})

	t.Run("no secret configured -> 403", func(t *testing.T) {
		_, r := newTestDeps(t, "")
		w := postWebhook(t, r, cleanWebhookPath, "anything")
		if w.Code != http.StatusForbidden {
			t.Fatalf("code = %d, want 403", w.Code)
		}
	})
}

// TestWebhookQueueBounded webhook 处理必须经过工作池而非裸 go func（ISSUE-03）：
// 池已注入时合法请求应立即 200，且分发动作在池内执行（容量有界）。
func TestWebhookQueueBounded(t *testing.T) {
	const secret = "pool-secret"
	deps, r := newTestDeps(t, secret)
	if deps.BotCore.Pool == nil {
		t.Fatal("BotCore.Pool 未注入，webhook 将退化为无界 goroutine")
	}
	w := postWebhook(t, r, cleanWebhookPath, secret)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", w.Code)
	}
	// 池内任务（无 token 时 DispatchRaw 直接返回）执行后不得 panic 影响响应
	time.Sleep(50 * time.Millisecond)
}

// TestWebhookQueueFullReturns503 队列满时必须回 503 让 Telegram 重推，
// 而不是先回 200 再把 update 丢掉（那样消息会永久丢失）。
func TestWebhookQueueFullReturns503(t *testing.T) {
	const secret = "full-secret"
	deps, r := newTestDeps(t, secret)

	// 换成 1 worker / 1 队列的小池，便于精确打满
	deps.BotCore.Pool = tgbot.NewWorkerPool(1, 1)

	block := make(chan struct{})
	started := make(chan struct{})
	if !deps.BotCore.Pool.Submit(func() { close(started); <-block }) {
		t.Fatal("第一个任务应被接纳")
	}
	<-started
	if !deps.BotCore.Pool.Submit(func() { <-block }) {
		t.Fatal("第二个任务应占满队列")
	}

	w := postWebhook(t, r, cleanWebhookPath, secret)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503（队列满时应让 TG 重推）", w.Code)
	}
	close(block)
}
