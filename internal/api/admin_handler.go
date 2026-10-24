package api

import (
	"crypto/rand"
	"encoding/json"
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
	tok := h.Settings.BotToken()
	c.JSON(http.StatusOK, gin.H{
		// 只回显打码后的 Token，完整值不再离开服务端
		"tg_bot_token_masked":  maskToken(tok),
		"has_bot_token":        tok != "",
		"tg_admin_group_id":    h.Settings.Get(store.KeyAdminGroupID),
		"notify_user_enabled":  h.Settings.NotifyUserEnabled(),
		"notify_admin_enabled": h.Settings.NotifyAdminEnabled(),
		"public_base_url":      h.Cfg.PublicBaseURL,
		"storage_mode":         h.Settings.StorageMode(),
		"storage_local_dir":    h.Settings.StorageLocalDir(),
		"storage_url_ttl_hrs":  h.Settings.StorageURLTTLHours(),
		"storage_s3":           maskS3Config(h.Settings.StorageS3JSON()),
		"assistant_enabled":       h.Settings.AssistantEnabled(),
		"assistant_base_url":      h.Settings.AssistantBaseURL(),
		"assistant_has_api_key":   h.Settings.AssistantAPIKey() != "",
		"assistant_model":         h.Settings.AssistantModel(),
		"assistant_temperature":   h.Settings.AssistantTemperature(),
		"assistant_silent_minutes": h.Settings.AssistantSilentMinutes(),
		"assistant_max_replies":   h.Settings.AssistantMaxReplies(),
		"assistant_system_prompt": h.Settings.AssistantSystemPrompt(),
	})
}

// maskS3Config S3 配置回显：secret 打码
func maskS3Config(jsonStr string) gin.H {
	out := gin.H{"endpoint": "", "region": "", "bucket": "", "access_key": "",
		"use_path_style": false, "public_base_url": "", "has_secret": false}
	if jsonStr == "" {
		return out
	}
	var cfg S3Config
	if err := json.Unmarshal([]byte(jsonStr), &cfg); err != nil {
		return out
	}
	out["endpoint"] = cfg.Endpoint
	out["region"] = cfg.Region
	out["bucket"] = cfg.Bucket
	out["access_key"] = cfg.AccessKey
	out["use_path_style"] = cfg.PathStyle
	out["public_base_url"] = cfg.PublicBase
	out["has_secret"] = cfg.SecretKey != ""
	return out
}

// maskToken 打码展示：123456789:AAF…4xY9
func maskToken(tok string) string {
	if tok == "" {
		return ""
	}
	if i := strings.Index(tok, ":"); i > 0 && len(tok) > i+6 {
		return tok[:i+1] + "…" + tok[len(tok)-4:]
	}
	if len(tok) > 8 {
		return tok[:4] + "…" + tok[len(tok)-4:]
	}
	return "…"
}

func (h *AdminHandler) UpdateSettings(c *gin.Context) {
	var req struct {
		TgBotToken         *string `json:"tg_bot_token"`
		TgAdminGroupID     *string `json:"tg_admin_group_id"`
		NotifyUserEnabled  *bool   `json:"notify_user_enabled"`
		NotifyAdminEnabled *bool   `json:"notify_admin_enabled"`
		StorageMode        *string `json:"storage_mode"`
		StorageLocalDir    *string `json:"storage_local_dir"`
		StorageURLTTLHrs   *int    `json:"storage_url_ttl_hrs"`
		StorageS3          *struct {
			Endpoint   string `json:"endpoint"`
			Region     string `json:"region"`
			Bucket     string `json:"bucket"`
			AccessKey  string `json:"access_key"`
			SecretKey  string `json:"secret_key"`
			PathStyle  bool   `json:"use_path_style"`
			PublicBase string `json:"public_base_url"`
		} `json:"storage_s3"`
		AssistantEnabled      *bool    `json:"assistant_enabled"`
		AssistantBaseURL      *string  `json:"assistant_base_url"`
		AssistantAPIKey       *string  `json:"assistant_api_key"`
		AssistantModel        *string  `json:"assistant_model"`
		AssistantTemperature  *float64 `json:"assistant_temperature"`
		AssistantSilentMin    *int     `json:"assistant_silent_minutes"`
		AssistantMaxReplies   *int     `json:"assistant_max_replies"`
		AssistantSystemPrompt *string  `json:"assistant_system_prompt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	oldToken := h.Settings.BotToken()
	if req.TgBotToken != nil {
		// 空字符串 = 显式清空；不传该字段 = 保持原值（前端只回传用户新输入的内容）
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
	if req.StorageMode != nil {
		m := strings.TrimSpace(*req.StorageMode)
		if m != "local" && m != "s3" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "storage_mode 只支持 local/s3"})
			return
		}
		_ = h.Settings.Set(store.KeyStorageMode, m)
	}
	if req.StorageLocalDir != nil {
		if v := strings.TrimSpace(*req.StorageLocalDir); v != "" {
			_ = h.Settings.Set(store.KeyStorageLocalDir, v)
		}
	}
	if req.StorageURLTTLHrs != nil && *req.StorageURLTTLHrs > 0 {
		_ = h.Settings.Set(store.KeyStorageURLTTLHrs, strconv.Itoa(*req.StorageURLTTLHrs))
	}
	if req.StorageS3 != nil {
		// secret 留空 = 保持原值（前端只回显 has_secret）
		var cfg S3Config
		_ = json.Unmarshal([]byte(h.Settings.StorageS3JSON()), &cfg)
		in := req.StorageS3
		cfg.Endpoint = strings.TrimRight(in.Endpoint, "/")
		cfg.Region = in.Region
		cfg.Bucket = in.Bucket
		cfg.AccessKey = in.AccessKey
		if in.SecretKey != "" {
			cfg.SecretKey = in.SecretKey
		}
		cfg.PathStyle = in.PathStyle
		cfg.PublicBase = strings.TrimRight(in.PublicBase, "/")
		b, _ := json.Marshal(cfg)
		_ = h.Settings.Set(store.KeyStorageS3JSON, string(b))
	}
	if req.AssistantEnabled != nil {
		_ = h.Settings.Set(store.KeyAssistantEnabled, boolStr(*req.AssistantEnabled))
	}
	if req.AssistantBaseURL != nil {
		_ = h.Settings.Set(store.KeyAssistantBaseURL, strings.TrimRight(strings.TrimSpace(*req.AssistantBaseURL), "/"))
	}
	if req.AssistantAPIKey != nil && strings.TrimSpace(*req.AssistantAPIKey) != "" {
		// 留空/不传 = 保持原值（前端只回显 has_api_key）
		_ = h.Settings.Set(store.KeyAssistantAPIKey, strings.TrimSpace(*req.AssistantAPIKey))
	}
	if req.AssistantModel != nil {
		_ = h.Settings.Set(store.KeyAssistantModel, strings.TrimSpace(*req.AssistantModel))
	}
	if req.AssistantTemperature != nil && *req.AssistantTemperature >= 0 && *req.AssistantTemperature <= 2 {
		_ = h.Settings.Set(store.KeyAssistantTemperature, strconv.FormatFloat(*req.AssistantTemperature, 'f', -1, 64))
	}
	if req.AssistantSilentMin != nil && *req.AssistantSilentMin > 0 {
		_ = h.Settings.Set(store.KeyAssistantSilentMin, strconv.Itoa(*req.AssistantSilentMin))
	}
	if req.AssistantMaxReplies != nil && *req.AssistantMaxReplies > 0 {
		_ = h.Settings.Set(store.KeyAssistantMaxReplies, strconv.Itoa(*req.AssistantMaxReplies))
	}
	if req.AssistantSystemPrompt != nil {
		_ = h.Settings.Set(store.KeyAssistantSystemPrompt, *req.AssistantSystemPrompt)
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
	// secret 只作为 header 传递，不再拼进 URL（避免进访问日志/反代日志）
	url := h.Cfg.PublicBaseURL + "/api/v1/telegram/webhook"
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
