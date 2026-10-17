package api

import (
	"crypto/subtle"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

type WebhookHandler struct{ Deps }

// Handle TG webhook 入口（secret 仅走 header）。
func (h *WebhookHandler) Handle(c *gin.Context) {
	h.handle(c, "")
}

// HandleLegacy 兼容 secret 仍在 URL 路径中的旧 webhook 地址。
func (h *WebhookHandler) HandleLegacy(c *gin.Context) {
	h.handle(c, c.Param("secret"))
}

// handle 校验 secret 后交给 BotCore 分发。
// pathSecret 非空时要求 URL 段与 header 同时匹配（旧地址的双因子校验）。
func (h *WebhookHandler) handle(c *gin.Context, pathSecret string) {
	expectedSecret := h.Settings.WebhookSecret()
	if expectedSecret == "" {
		c.Status(http.StatusForbidden)
		return
	}

	headerSecret := c.GetHeader("X-Telegram-Bot-Api-Secret-Token")

	// 使用 subtle.ConstantTimeCompare 防止时序侧信道
	headerMatch := subtle.ConstantTimeCompare([]byte(headerSecret), []byte(expectedSecret)) == 1
	if !headerMatch {
		c.Status(http.StatusForbidden)
		return
	}
	if pathSecret != "" {
		pathMatch := subtle.ConstantTimeCompare([]byte(pathSecret), []byte(expectedSecret)) == 1
		if !pathMatch {
			c.Status(http.StatusForbidden)
			return
		}
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	// 尽快返回 200，处理异步做，避免 TG 超时重推。
	// 队列满时返回 503，让 Telegram 退避重推；否则消息会被静默丢弃。
	accepted := false
	if h.BotCore.Pool != nil {
		accepted = h.BotCore.Pool.Submit(func() {
			h.BotCore.DispatchRaw(body)
		})
	} else {
		go h.BotCore.DispatchRaw(body)
		accepted = true
	}
	if !accepted {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	c.Status(http.StatusOK)
}
