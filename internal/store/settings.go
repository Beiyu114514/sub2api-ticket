package store

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"sub2api-ticket/internal/model"
)

// Settings 运行时配置存取，带短缓存避免高频查询。
type Settings struct {
	g        *gorm.DB
	mu       sync.RWMutex
	cache    map[string]string
	loadedAt time.Time
}

func NewSettings(g *gorm.DB) *Settings {
	return &Settings{g: g, cache: map[string]string{}}
}

func (s *Settings) Get(key string) string {
	s.mu.RLock()
	if time.Since(s.loadedAt) < 2*time.Second {
		v := s.cache[key]
		s.mu.RUnlock()
		return v
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.loadedAt) >= 2*time.Second {
		var rows []model.Setting
		s.g.Find(&rows)
		m := map[string]string{}
		for _, r := range rows {
			m[r.Key] = r.Value
		}
		s.cache = m
		s.loadedAt = time.Now()
	}
	return s.cache[key]
}

func (s *Settings) Set(key, value string) error {
	if err := s.g.Save(&model.Setting{Key: key, Value: value}).Error; err != nil {
		return err
	}
	s.mu.Lock()
	s.cache[key] = value
	s.mu.Unlock()
	return nil
}

// —— 常用键封装 ——

const (
	KeyBotToken      = "tg_bot_token"
	KeyAdminGroupID  = "tg_admin_group_id"
	KeyWebhookSecret = "tg_webhook_secret"
	KeyBotUsername   = "tg_bot_username"
	KeyNotifyUser    = "notify_user_enabled"
	KeyNotifyAdmin   = "notify_admin_enabled"

	// 图片存储
	KeyStorageMode      = "storage_mode"        // local / s3
	KeyStorageLocalDir  = "storage_local_dir"   // 默认 data/uploads
	KeyStorageSignKey   = "storage_sign_key"    // 本地签名 HMAC 密钥（随机生成）
	KeyStorageS3JSON    = "storage_s3_json"     // S3 配置 JSON
	KeyStorageURLTTLHrs = "storage_url_ttl_hrs" // 签名 URL 有效期小时，默认 168

	// LLM 临时客服
	KeyAssistantEnabled      = "assistant_enabled"
	KeyAssistantBaseURL      = "assistant_base_url"
	KeyAssistantAPIKey       = "assistant_api_key"
	KeyAssistantModel        = "assistant_model"
	KeyAssistantTemperature  = "assistant_temperature"
	KeyAssistantSilentMin    = "assistant_silent_minutes"
	KeyAssistantMaxReplies   = "assistant_max_replies_per_ticket"
	KeyAssistantSystemPrompt = "assistant_system_prompt"
)

func (s *Settings) BotToken() string      { return s.Get(KeyBotToken) }
func (s *Settings) AdminGroupID() int64   { return parseInt64(s.Get(KeyAdminGroupID)) }
func (s *Settings) WebhookSecret() string { return s.Get(KeyWebhookSecret) }
func (s *Settings) BotUsername() string   { return s.Get(KeyBotUsername) }

func (s *Settings) NotifyUserEnabled() bool  { return s.Get(KeyNotifyUser) != "false" }
func (s *Settings) NotifyAdminEnabled() bool { return s.Get(KeyNotifyAdmin) != "false" }

// StorageMode 返回 local / s3，未配置时 local
func (s *Settings) StorageMode() string {
	if v := s.Get(KeyStorageMode); v == "s3" {
		return "s3"
	}
	return "local"
}

// StorageSignKey 本地签名密钥；不存在则生成并持久化（32 字节 hex）
func (s *Settings) StorageSignKey() string {
	if v := s.Get(KeyStorageSignKey); v != "" {
		return v
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "fallback-insecure-key"
	}
	v := hex.EncodeToString(b)
	_ = s.Set(KeyStorageSignKey, v)
	return v
}

// StorageURLTTLHours 签名 URL 有效期，默认 168h
func (s *Settings) StorageURLTTLHours() int {
	if v := parseInt64(s.Get(KeyStorageURLTTLHrs)); v > 0 {
		return int(v)
	}
	return 168
}

func (s *Settings) StorageLocalDir() string {
	if v := s.Get(KeyStorageLocalDir); v != "" {
		return v
	}
	return "data/uploads"
}

// StorageS3JSON 原样返回 S3 配置 JSON 字符串
func (s *Settings) StorageS3JSON() string { return s.Get(KeyStorageS3JSON) }

func parseInt64(s string) int64 {
	var n int64
	var neg bool
	if s == "" {
		return 0
	}
	i := 0
	if s[0] == '-' {
		neg = true
		i = 1
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0
		}
		n = n*10 + int64(s[i]-'0')
	}
	if neg {
		return -n
	}
	return n
}


// ———— LLM 助手配置 ————
func (s *Settings) AssistantEnabled() bool   { return s.Get(KeyAssistantEnabled) == "true" }
func (s *Settings) AssistantBaseURL() string { return s.Get(KeyAssistantBaseURL) }
func (s *Settings) AssistantAPIKey() string  { return s.Get(KeyAssistantAPIKey) }
func (s *Settings) AssistantModel() string   { return s.Get(KeyAssistantModel) }
func (s *Settings) AssistantTemperature() float64 {
	if v := parseFloat(s.Get(KeyAssistantTemperature)); v > 0 {
		return v
	}
	return 0.3
}
func (s *Settings) AssistantSilentMinutes() int {
	if v := parseInt64(s.Get(KeyAssistantSilentMin)); v > 0 {
		return int(v)
	}
	return 5
}
func (s *Settings) AssistantMaxReplies() int {
	if v := parseInt64(s.Get(KeyAssistantMaxReplies)); v > 0 {
		return int(v)
	}
	return 5
}
func (s *Settings) AssistantSystemPrompt() string {
	if v := s.Get(KeyAssistantSystemPrompt); v != "" {
		return v
	}
	return "你是工单系统的临时客服助手。请礼貌、简洁地回答用户问题；无法确定的问题请如实说明并建议等待人工客服。不要编造事实，不要承诺具体处理时间。"
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}
