package api

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"sub2api-ticket/internal/storage"
	"gorm.io/gorm"

	"sub2api-ticket/internal/auth"
	"sub2api-ticket/internal/config"
	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/store"
	"sub2api-ticket/internal/sub2api"
	"sub2api-ticket/internal/tgbot"
)

type Deps struct {
	G        *gorm.DB
	Cfg      *config.Config
	Auth     *auth.Service
	Settings *store.Settings
	Tickets  *store.TicketStore
	Sub2API  *sub2api.Client
	BotCore  *tgbot.Core
}

// Bot 每次按 settings 里的 token 构造客户端（网页后台可随时改 token）
func (d Deps) Bot() *tgbot.Client {
	tok := d.Settings.BotToken()
	if tok == "" {
		return nil
	}
	return tgbot.NewWithProxy(tok, d.Cfg.TGProxy)
}

func NewRouter(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(accessLog(), gin.Recovery())

	apiV1 := r.Group("/api/v1")

	// 认证
	apiV1.POST("/auth/sso", d.Auth.SSO)
	apiV1.POST("/auth/login", d.Auth.LocalLogin)
	apiV1.POST("/auth/logout", d.Auth.Logout)

	authed := apiV1.Group("", d.Auth.RequireAuth())
	{
		authed.GET("/me", func(c *gin.Context) {
			u := c.MustGet("user").(*model.User)
			c.JSON(200, gin.H{"user": u})
		})

		// 工单
		th := &TicketHandler{Deps: d}
		authed.GET("/tickets", th.List)
		authed.POST("/tickets", th.Create)
		authed.GET("/tickets/:id", th.Get)
		authed.POST("/tickets/:id/messages", th.Reply)
		authed.POST("/tickets/:id/status", th.ChangeStatus)

		// TG 绑定
		bh := &BindHandler{Deps: d}
		authed.POST("/telegram/bind", bh.CreateBindCode)
		authed.DELETE("/telegram/bind", bh.Unbind)
		authed.GET("/telegram/bind-status", bh.Status)

		// 图片上传
		uh := &UploadHandler{Deps: d}
		authed.POST("/uploads", uh.Upload)

		// 管理端
		admin := authed.Group("", d.Auth.RequireAdmin())
		{
			ah := &AdminHandler{Deps: d}
			admin.GET("/admin/settings", ah.GetSettings)
			admin.PUT("/admin/settings", ah.UpdateSettings)
			admin.GET("/admin/telegram/status", ah.TelegramStatus)
			admin.POST("/admin/telegram/setup-webhook", ah.SetupWebhook)
			admin.POST("/admin/telegram/stop-webhook", ah.StopWebhook)
			admin.POST("/admin/telegram/test", ah.TestMessage)
		}
	}

	// TG webhook（自身带 secret 校验）
	wh := &WebhookHandler{Deps: d}
	// 新地址：secret 只走 X-Telegram-Bot-Api-Secret-Token 头，不进 URL、不进访问日志
	apiV1.POST("/telegram/webhook", wh.Handle)
	// 兼容旧地址：secret 仍在路径中（历史注册的 webhook 继续可用），
	// 访问日志已由 accessLog 屏蔽该前缀
	apiV1.POST("/telegram/webhook/:secret", wh.HandleLegacy)

	// 本地存储附件服务：/uploads/<key>?e=<expire>&s=<sig>，HMAC 签名校验
	registerUploads(r, d)

	return r
}

// registerUploads 提供 /uploads/<key> 静态访问（local 模式）。
// 每次请求按当前设置构造 Local 校验器，签名密钥来自 Setting。
func registerUploads(r *gin.Engine, d Deps) {
	r.GET("/uploads/*key", func(c *gin.Context) {
		key := strings.TrimPrefix(c.Param("key"), "/")
		expire, _ := strconv.ParseInt(c.Query("e"), 10, 64)
		sig := c.Query("s")
		l := &storage.Local{
			Root:    d.Settings.StorageLocalDir(),
			SignKey: d.Settings.StorageSignKey(),
			URLTTL:  time.Duration(d.Settings.StorageURLTTLHours()) * time.Hour,
		}
		if err := l.Verify(key, expire, sig, time.Now()); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		rc, err := l.Open(c.Request.Context(), key)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "附件不存在"})
			return
		}
		defer rc.Close()
		mimeType := mimeByExt(key)
		c.Header("Cache-Control", "private, max-age=3600")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, mimeType, nil)
		io.Copy(c.Writer, rc)
	})
}

func mimeByExt(key string) string {
	switch {
	case strings.HasSuffix(key, ".jpg"), strings.HasSuffix(key, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(key, ".png"):
		return "image/png"
	case strings.HasSuffix(key, ".gif"):
		return "image/gif"
	case strings.HasSuffix(key, ".webp"):
		return "image/webp"
	}
	return "application/octet-stream"
}

// accessLog 记录访问日志，并做两处脱敏：
//  1. 跳过 Telegram webhook 路径：旧版 webhook 把 secret 放在 URL 路径里，
//     一旦写进日志，任何能读日志的人都能伪造 TG 更新；
//  2. 不记录 query string：SPA 入口会把 SSO JWT 放在 ?token= 中，
//     该 token 可被重放，不能明文落盘或进入日志系统。
func accessLog() gin.HandlerFunc {
	logger := gin.LoggerWithConfig(gin.LoggerConfig{SkipQueryString: true})
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/telegram/webhook") {
			c.Next()
			return
		}
		logger(c)
	}
}
