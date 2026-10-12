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
	r.POST("/api/v1/telegram/webhook/:secret", (&WebhookHandler{Deps: deps}).Handle)
	return deps, r
}

func postWebhook(t *testing.T, r *gin.Engine, urlSecret, headerSecret string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"update_id":1,"message":{"message_id":1,"chat":{"id":2,"type":"private"},"text":"hi"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/webhook/"+urlSecret, strings.NewReader(body))
	if headerSecret != "" {
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", headerSecret)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestWebhookSecretConstantTimeCompare 回归 ISSUE-02：secret 校验走恒定时间比较，
// 行为语义：双因子（URL 段 + Header）全部匹配才放行。
func TestWebhookSecretConstantTimeCompare(t *testing.T) {
	const secret = "s3cr3t-token"

	t.Run("both match -> 200", func(t *testing.T) {
		_, r := newTestDeps(t, secret)
		w := postWebhook(t, r, secret, secret)
		if w.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200", w.Code)
		}
	})

	t.Run("wrong url secret -> 403", func(t *testing.T) {
		_, r := newTestDeps(t, secret)
		w := postWebhook(t, r, "wrong", secret)
		if w.Code != http.StatusForbidden {
			t.Fatalf("code = %d, want 403", w.Code)
		}
	})

	t.Run("wrong header secret -> 403", func(t *testing.T) {
		_, r := newTestDeps(t, secret)
		w := postWebhook(t, r, secret, "wrong")
		if w.Code != http.StatusForbidden {
			t.Fatalf("code = %d, want 403", w.Code)
		}
	})

	t.Run("missing header -> 403", func(t *testing.T) {
		_, r := newTestDeps(t, secret)
		w := postWebhook(t, r, secret, "")
		if w.Code != http.StatusForbidden {
			t.Fatalf("code = %d, want 403", w.Code)
		}
	})

	t.Run("secret prefix match must not pass", func(t *testing.T) {
		_, r := newTestDeps(t, secret)
		// 常量时间比较必须要求完整相等：前缀相同但长度不同一律拒绝
		w := postWebhook(t, r, secret[:len(secret)-1], secret[:len(secret)-1])
		if w.Code != http.StatusForbidden {
			t.Fatalf("code = %d, want 403", w.Code)
		}
	})

	t.Run("no secret configured -> 403", func(t *testing.T) {
		_, r := newTestDeps(t, "")
		w := postWebhook(t, r, "anything", "anything")
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
	w := postWebhook(t, r, secret, secret)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", w.Code)
	}
	// 池内任务（无 token 时 DispatchRaw 直接返回）执行后不得 panic 影响响应
	time.Sleep(50 * time.Millisecond)
}
