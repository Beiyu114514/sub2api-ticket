package store

import (
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
)

func (s *Settings) BotToken() string      { return s.Get(KeyBotToken) }
func (s *Settings) AdminGroupID() int64   { return parseInt64(s.Get(KeyAdminGroupID)) }
func (s *Settings) WebhookSecret() string { return s.Get(KeyWebhookSecret) }
func (s *Settings) BotUsername() string   { return s.Get(KeyBotUsername) }

func (s *Settings) NotifyUserEnabled() bool  { return s.Get(KeyNotifyUser) != "false" }
func (s *Settings) NotifyAdminEnabled() bool { return s.Get(KeyNotifyAdmin) != "false" }

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
