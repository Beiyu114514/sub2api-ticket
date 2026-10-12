package db

import (
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"sub2api-ticket/internal/model"
)

func Open(path string) (*gorm.DB, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	g, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	// modernc/sqlite 单写者，限制连接数避免 SQLITE_BUSY
	if sqlDB, err := g.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetConnMaxLifetime(0)
	}
	if err := g.AutoMigrate(
		&model.User{},
		&model.Ticket{},
		&model.TicketMessage{},
		&model.TicketTopic{},
		&model.UserChatState{},
		&model.Session{},
		&model.Notification{},
		&model.BindCode{},
		&model.Setting{},
	); err != nil {
		return nil, err
	}
	return g, nil
}

func CleanExpired(g *gorm.DB) {
	ticker := time.NewTicker(time.Hour)
	go func() {
		for range ticker.C {
			g.Where("expires_at < ?", time.Now()).Delete(&model.Session{})
			g.Where("expires_at < ?", time.Now()).Delete(&model.BindCode{})
		}
	}()
}
