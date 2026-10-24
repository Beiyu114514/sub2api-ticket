package db

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"sub2api-ticket/internal/model"
)

// buildLegacyDB 模拟历史版本二进制创建的库：users 表的 ID 列名为 legacyCol。
func buildLegacyDB(t *testing.T, path, legacyCol string) {
	t.Helper()
	g, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		` + legacyCol + ` INTEGER,
		username TEXT, email TEXT, role TEXT,
		tg_chat_id INTEGER DEFAULT 0, password_hash TEXT,
		created_at DATETIME, last_seen_at DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := g.Exec("CREATE UNIQUE INDEX idx_users_" + legacyCol + " ON users(" + legacyCol + ")").Error; err != nil {
		t.Fatal(err)
	}
	if err := g.Exec(`INSERT INTO users (`+legacyCol+`, username, email, role, tg_chat_id, password_hash, created_at, last_seen_at)
		VALUES (1, 'legacy', 'legacy@example.com', 'admin', 0, '', datetime('now'), datetime('now'))`).Error; err != nil {
		t.Fatal(err)
	}
}

func TestMigrateLegacyUserColumn(t *testing.T) {
	for _, legacy := range []string{"sub_api_id", "sub2_api_id"} {
		t.Run(legacy, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ticket.db")
			buildLegacyDB(t, path, legacy)

			g, err := Open(path)
			if err != nil {
				t.Fatalf("Open 迁移后失败: %v", err)
			}
			// 迁移后：旧数据可通过新列名查到
			var u model.User
			if err := g.Where("sub2api_id = ?", 1).First(&u).Error; err != nil {
				t.Fatalf("迁移后按 sub2api_id 查询失败: %v", err)
			}
			if u.Username != "legacy" {
				t.Fatalf("期望迁移保留旧数据，got username=%q", u.Username)
			}
			// 插入不再撞旧唯一索引
			if err := g.Create(&model.User{Sub2APIID: 2, Username: "new", Role: "user"}).Error; err != nil {
				t.Fatalf("迁移后插入失败: %v", err)
			}
			var cnt int64
			g.Model(&model.User{}).Where("sub2api_id = ?", 2).Count(&cnt)
			if cnt != 1 {
				t.Fatalf("期望新插入可查询，cnt=%d", cnt)
			}
			// 旧列名应已不存在
			var n int64
			g.Raw("SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = ?", legacy).Scan(&n)
			if n != 0 {
				t.Fatalf("旧列 %s 仍存在", legacy)
			}
		})
	}
}

// 新库走 Open 不应触发迁移、建列即为显式固定的 sub2api_id。
func TestFreshDBUsesFixedColumn(t *testing.T) {
	g, err := Open(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	var n int64
	g.Raw("SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'sub2api_id'").Scan(&n)
	if n != 1 {
		t.Fatalf("新库应包含固定列名 sub2api_id")
	}
	if err := g.Create(&model.User{Sub2APIID: 7, Username: "x", Role: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	var u model.User
	if err := g.Where("sub2api_id = ?", 7).First(&u).Error; err != nil {
		t.Fatal(err)
	}
}
