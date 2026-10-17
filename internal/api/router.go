package api

import (
	"strings"

	"github.com/gin-gonic/gin"
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
	r.Use(skipWebhookAccessLog(), gin.Recovery())

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
	// 访问日志已由 skipWebhookAccessLog 屏蔽该前缀
	apiV1.POST("/telegram/webhook/:secret", wh.HandleLegacy)

	return r
}

// skipWebhookAccessLog 跳过 webhook 路径的访问日志：
// 旧版 webhook 把 secret 放在 URL 路径里，一旦写进日志，任何能读日志的人都能伪造 TG 更新。
func skipWebhookAccessLog() gin.HandlerFunc {
	logger := gin.Logger()
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/telegram/webhook") {
			c.Next()
			return
		}
		logger(c)
	}
}
