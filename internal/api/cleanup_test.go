package api

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/store"
)

// newCleanupDB 内存迁移测试库（含 Upload 表）
func newCleanupDB(t *testing.T) *gorm.DB {
	t.Helper()
	g, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "t.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := g.AutoMigrate(&model.User{}, &model.Ticket{}, &model.TicketMessage{}, &model.Upload{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return g
}

// 孤儿清理：超期未挂单且无消息引用 → 删除；宽限期内 → 保留；仍被引用 → 修正 ticket_id
func TestCleanOrphanUploads(t *testing.T) {
	g := newCleanupDB(t)
	old := time.Now().Add(-25 * time.Hour)
	fresh := time.Now().Add(-time.Minute)

	// local 模式默认目录 data/uploads，对象不存在时 Delete 应幂等或容忍（见 local.Delete）
	orphan := &model.Upload{UserID: 1, Storage: "local", Key: "2026/10/aaaa.jpg", Mime: "image/jpeg", CreatedAt: old}
	kept := &model.Upload{UserID: 1, Storage: "local", Key: "2026/10/fresh.jpg", Mime: "image/jpeg", CreatedAt: fresh}
	referenced := &model.Upload{UserID: 1, Storage: "local", Key: "2026/10/ref.jpg", Mime: "image/jpeg", CreatedAt: old}
	for _, r := range []*model.Upload{orphan, kept, referenced} {
		if err := g.Create(r).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := g.Create(&model.TicketMessage{
		TicketID: 5, SenderRole: "user", SenderName: "u", Content: "[图片]",
		Attachments: `[{"url":"/uploads/2026/10/ref.jpg"}]`,
	}).Error; err != nil {
		t.Fatal(err)
	}

	s := &UploadService{Deps: Deps{G: g, Settings: store.NewSettings(g)}}
	s.cleanOrphanOnce()

	var cnt int64
	g.Model(&model.Upload{}).Where("key = ?", orphan.Key).Count(&cnt)
	if cnt != 0 {
		t.Errorf("孤儿记录应被删除")
	}
	g.Model(&model.Upload{}).Where("key = ?", kept.Key).Count(&cnt)
	if cnt != 1 {
		t.Errorf("宽限期内记录不应被删除")
	}
	var ref model.Upload
	if err := g.Where("key = ?", referenced.Key).First(&ref).Error; err != nil {
		t.Fatalf("被引用记录不应被删除: %v", err)
	}
	if ref.TicketID != 5 {
		t.Errorf("被引用记录应修正 ticket_id=5，实际 %d", ref.TicketID)
	}
}
