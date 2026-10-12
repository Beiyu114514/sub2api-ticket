package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"sub2api-ticket/internal/store"
	"sub2api-ticket/internal/tgbot"
)

type AdminHandler struct{ Deps }

func (h *AdminHandler) GetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"tg_bot_token":         h.Settings.BotToken(),
		"tg_admin_group_id":    h.Settings.Get(store.KeyAdminGroupID),
		"notify_user_enabled":  h.Settings.NotifyUserEnabled(),
		"notify_admin_enabled": h.Settings.NotifyAdminEnabled(),
		"public_base_url":      h.Cfg.PublicBaseURL,
	})
}

func (h *AdminHandler) UpdateSettings(c *gin.Context) {
	var req struct {
		TgBotToken         *string `json:"tg_bot_token"`
		TgAdminGroupID     *string `json:"tg_admin_group_id"`
		NotifyUserEnabled  *bool   `json:"notify_user_enabled"`
		NotifyAdminEnabled *bool   `json:"notify_admin_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	oldToken := h.Settings.BotToken()
	if req.TgBotToken != nil {
		tok := strings.TrimSpace(*req.TgBotToken)
		if err := h.Settings.Set(store.KeyBotToken, tok); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
			return
		}
		if tok != oldToken {
			h.Settings.Set(store.KeyBotUsername, "") // 失效缓存，状态查询时重新 getMe
		}
	}
	if req.TgAdminGroupID != nil {
		v := strings.TrimSpace(*req.TgAdminGroupID)
		if v != "" {
			if _, err := strconv.ParseInt(v, 10, 64); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "群 ID 必须是数字（超级群通常为 -100 开头）"})
				return
			}
		}
		_ = h.Settings.Set(store.KeyAdminGroupID, v)
	}
	if req.NotifyUserEnabled != nil {
		_ = h.Settings.Set(store.KeyNotifyUser, boolStr(*req.NotifyUserEnabled))
	}
	if req.NotifyAdminEnabled != nil {
		_ = h.Settings.Set(store.KeyNotifyAdmin, boolStr(*req.NotifyAdminEnabled))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// TelegramStatus getMe + webhook 状态
func (h *AdminHandler) TelegramStatus(c *gin.Context) {
	bot := h.Bot()
	if bot == nil {
		c.JSON(http.StatusOK, gin.H{"configured": false})
		return
	}
	me, err := bot.GetMe()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"configured": true, "bot_ok": false, "error": err.Error()})
		return
	}
	_ = h.Settings.Set(store.KeyBotUsername, me.Username)
	info, _ := bot.GetWebhookInfo()
	resp := gin.H{
		"configured":   true,
		"bot_ok":       true,
		"bot_id":       me.ID,
		"bot_username": me.Username,
	}
	if info != nil {
		resp["webhook_url"] = info["url"]
		resp["pending_updates"] = info["pending_update_count"]
		lastErr, _ := info["last_error_message"].(string)
		if lastErr != "" {
			resp["last_error"] = lastErr
		}
	}
	c.JSON(http.StatusOK, resp)
}

// SetupWebhook 注册 webhook（自动生成 secret）
func (h *AdminHandler) SetupWebhook(c *gin.Context) {
	if h.Cfg.PublicBaseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "需配置 PUBLIC_BASE_URL 环境变量后才能使用 webhook"})
		return
	}
	bot := h.Bot()
	if bot == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置 Bot Token"})
		return
	}
	secret := h.Settings.WebhookSecret()
	if secret == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		secret = hex.EncodeToString(b)
		_ = h.Settings.Set(store.KeyWebhookSecret, secret)
	}
	url := h.Cfg.PublicBaseURL + "/api/v1/telegram/webhook/" + secret
	if err := bot.SetWebhook(url, secret); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	_ = h.Settings.Set("tg_webhook_url", url)
	c.JSON(http.StatusOK, gin.H{"ok": true, "url": url})
}

// StopWebhook 停用 webhook
func (h *AdminHandler) StopWebhook(c *gin.Context) {
	bot := h.Bot()
	if bot != nil {
		_ = bot.DeleteWebhook()
	}
	_ = h.Settings.Set("tg_webhook_url", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// TestMessage 向管理员群发测试消息
func (h *AdminHandler) TestMessage(c *gin.Context) {
	bot := h.Bot()
	if bot == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置 Bot Token"})
		return
	}
	groupID := h.Settings.AdminGroupID()
	if groupID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置管理员群 ID"})
		return
	}
	_, err := bot.SendMessage(tgbot.SendMessageOpts{ChatID: groupID, Text: "✅ sub2api 工单系统测试消息"})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
