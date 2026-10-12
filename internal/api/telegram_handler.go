package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"sub2api-ticket/internal/model"
)

type BindHandler struct{ Deps }

// CreateBindCode 生成一次性绑定码 + 深层链接
func (h *BindHandler) CreateBindCode(c *gin.Context) {
	u := c.MustGet("user").(*model.User)
	if h.Settings.BotToken() == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "管理员尚未配置 Telegram Bot"})
		return
	}
	if u.TGChatID != 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "已绑定，如需换绑请先解绑"})
		return
	}
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	code := hex.EncodeToString(b) // 10 位随机码

	if err := h.G.Create(&model.BindCode{
		Code:      code,
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成失败"})
		return
	}
	link := "https://t.me/" + h.Settings.BotUsername() + "?start=bind_" + code
	c.JSON(http.StatusOK, gin.H{"code": code, "link": link, "expires_in": 600})
}

// Unbind 解绑
func (h *BindHandler) Unbind(c *gin.Context) {
	u := c.MustGet("user").(*model.User)
	h.G.Model(u).Update("tg_chat_id", 0)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Status 绑定状态
func (h *BindHandler) Status(c *gin.Context) {
	u := c.MustGet("user").(*model.User)
	c.JSON(http.StatusOK, gin.H{
		"bound":        u.TGChatID != 0,
		"bot_username": h.Settings.BotUsername(),
		"bot_ready":    h.Settings.BotToken() != "",
	})
}
