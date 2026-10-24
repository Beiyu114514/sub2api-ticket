package db

import (
	"log"
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
	// 迁移必须在 AutoMigrate 之前：AutoMigrate 遇到旧库会直接新增
	// sub2api_id 空列，之后便无法分辨旧列里的存量数据。
	migrateLegacyUserColumn(g)
	if err := g.AutoMigrate(
		&model.User{},
		&model.Ticket{},
		&model.TicketMessage{},
		&model.TicketTopic{},
		&model.UserChatState{},
		&model.Session{},
		&model.Notification{},
		&model.BindCode{},
		&model.Upload{},
		&model.Setting{},
	); err != nil {
		return nil, err
	}
	return g, nil
}

// migrateLegacyUserColumn 兼容历史版本创建的库：GORM 对 User.Sub2APIID 的
// 默认蛇形命名随版本漂移，历史上生成过 sub_api_id 和 sub2_api_id 两种列名，
// 而当前模型显式固定为 sub2api_id。AutoMigrate 只加列不重命名，若不迁移，
// 旧库上 SSO 查询永远 miss、插入撞旧唯一索引，登录必然 500。
// 仅当 users 表存在旧列且不存在目标列时才执行，对新库零影响。
func migrateLegacyUserColumn(g *gorm.DB) {
	hasCol := func(name string) bool {
		var n int64
		g.Raw("SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = ?", name).Scan(&n)
		return n > 0
	}
	if hasCol("sub2api_id") {
		return
	}
	for _, legacy := range []string{"sub_api_id", "sub2_api_id"} {
		if !hasCol(legacy) {
			continue
		}
		if err := g.Exec("ALTER TABLE users RENAME COLUMN "+legacy+" TO sub2api_id").Error; err != nil {
			log.Printf("[db] 旧库列迁移失败（users.%s → sub2api_id），SSO 登录可能异常: %v", legacy, err)
			return
		}
		// SQLite ≥ 3.25 重命名列会同步更新索引定义，但索引名保留旧名；
		// 不影响功能，重建只为排查直观。
		g.Exec("DROP INDEX IF EXISTS idx_users_" + legacy)
		g.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_users_sub2api_id ON users(sub2api_id)")
		log.Printf("[db] 已迁移旧库列名：users.%s → sub2api_id", legacy)
		return
	}
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
