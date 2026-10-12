package api

import (
	"crypto/subtle"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

type WebhookHandler struct{ Deps }

// Handle TG webhook 入口：校验 secret 后交给 BotCore 分发
func (h *WebhookHandler) Handle(c *gin.Context) {
	expectedSecret := h.Settings.WebhookSecret()
	if expectedSecret == "" {
		c.Status(http.StatusForbidden)
		return
	}

	paramSecret := c.Param("secret")
	headerSecret := c.GetHeader("X-Telegram-Bot-Api-Secret-Token")

	// 使用 subtle.ConstantTimeCompare 防止时序侧信道
	paramMatch := subtle.ConstantTimeCompare([]byte(paramSecret), []byte(expectedSecret)) == 1
	headerMatch := subtle.ConstantTimeCompare([]byte(headerSecret), []byte(expectedSecret)) == 1

	if !paramMatch || !headerMatch {
		c.Status(http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	// 尽快返回 200，处理异步做，避免 TG 超时重推
	if h.BotCore.Pool != nil {
		h.BotCore.SubmitTask(func() {
			h.BotCore.DispatchRaw(body)
		})
	} else {
		go h.BotCore.DispatchRaw(body)
	}
	c.Status(http.StatusOK)
}
