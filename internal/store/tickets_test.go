package store

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/glebarez/sqlite"

	"sub2api-ticket/internal/model"
)

// newTestStore 临时文件 SQLite + 自动迁移，模拟生产的单连接写者模式。
func newTestStore(t *testing.T) (*TicketStore, *gorm.DB) {
	t.Helper()
	g, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ticket.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := g.AutoMigrate(&model.User{}, &model.Ticket{}, &model.TicketMessage{},
		&model.TicketTopic{}, &model.UserChatState{}, &model.Session{},
		&model.Notification{}, &model.BindCode{}, &model.Setting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// 与 internal/db.Open 保持一致：单写者 + busy_timeout
	if sqlDB, err := g.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	}
	return NewTicketStore(g, NewSettings(g)), g
}

func newTestUser(t *testing.T, g *gorm.DB) *model.User {
	t.Helper()
	u := &model.User{Sub2APIID: time.Now().UnixNano(), Username: "tester", Role: "user"}
	if err := g.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

// TestAppendMessageNoLostUpdate 回归 ISSUE-01：并发 AppendMessage 不得丢失未读计数。
// 旧实现在应用层读-改-写（t.UnreadAdminCount + 1），并发下相互覆盖；
// 现实现使用 SQL 原子表达式 unread_admin_count + 1，N 条并发消息必须累加为 N。
func TestAppendMessageNoLostUpdate(t *testing.T) {
	st, g := newTestStore(t)
	u := newTestUser(t, g)
	ticket, _, err := st.CreateTicket(u, "并发回归", "", "normal", "首条")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	const n = 24
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := st.AppendMessage(ticket.ID, u, "user", u.Username,
				fmt.Sprintf("并发消息 %d", i), "web", false)
			if err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("AppendMessage: %v", err)
	}

	var got model.Ticket
	if err := g.First(&got, ticket.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	// 首条消息不带未读递增（CreateTicket 不设置未读），之后每条用户消息 +1
	if got.UnreadAdminCount != n {
		t.Fatalf("unread_admin_count = %d, want %d（丢失更新）", got.UnreadAdminCount, n)
	}
	var msgCount int64
	g.Model(&model.TicketMessage{}).Where("ticket_id = ?", ticket.ID).Count(&msgCount)
	if msgCount != n+1 {
		t.Fatalf("message count = %d, want %d", msgCount, n+1)
	}
	if got.Status != "open" {
		t.Fatalf("status = %q, want open", got.Status)
	}
}

// TestAppendMessageAdminConcurrent 管理员侧未读计数同样必须原子递增（ISSUE-01）。
func TestAppendMessageAdminConcurrent(t *testing.T) {
	st, g := newTestStore(t)
	u := newTestUser(t, g)
	ticket, _, err := st.CreateTicket(u, "管理员并发", "", "normal", "首条")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	const n = 16
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			st.AppendMessage(ticket.ID, nil, "admin", "admin", fmt.Sprintf("回复 %d", i), "telegram", true)
		}(i)
	}
	wg.Wait()
	var got model.Ticket
	if err := g.First(&got, ticket.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.UnreadUserCount != n {
		t.Fatalf("unread_user_count = %d, want %d（丢失更新）", got.UnreadUserCount, n)
	}
	if got.Status != "pending" {
		t.Fatalf("status = %q, want pending", got.Status)
	}
}

// TestAppendMessageClosedReopen 已关闭工单：closedOK=false 拒绝；admin 回复重开并清 closed_at。
func TestAppendMessageClosedReopen(t *testing.T) {
	st, g := newTestStore(t)
	u := newTestUser(t, g)
	ticket, _, err := st.CreateTicket(u, "关闭重开", "", "normal", "首条")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if _, err := st.ChangeStatus(ticket, "admin", "admin", "closed"); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, _, err := st.AppendMessage(ticket.ID, u, "user", u.Username, "关闭后回复", "web", false); err != ErrTicketClosed {
		t.Fatalf("closed ticket append err = %v, want ErrTicketClosed", err)
	}
	updated, _, err := st.AppendMessage(ticket.ID, nil, "admin", "admin", "管理员回复重开", "telegram", true)
	if err != nil {
		t.Fatalf("admin append: %v", err)
	}
	if updated.Status != "pending" {
		t.Fatalf("status = %q, want pending（管理员回复应重开）", updated.Status)
	}
	if updated.ClosedAt != nil {
		t.Fatalf("closed_at = %v, want nil", updated.ClosedAt)
	}
	// 事务内重载后的返回值必须与库中一致
	var got model.Ticket
	if err := g.First(&got, ticket.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.UnreadUserCount != 1 {
		t.Fatalf("unread_user_count = %d, want 1", got.UnreadUserCount)
	}
}

// TestAppendMessageReturnsFreshState 返回值必须是事务提交后的最新状态（ISSUE-01 收尾点）。
func TestAppendMessageReturnsFreshState(t *testing.T) {
	st, g := newTestStore(t)
	u := newTestUser(t, g)
	ticket, _, err := st.CreateTicket(u, "状态刷新", "", "normal", "首条")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	updated, msg, err := st.AppendMessage(ticket.ID, u, "user", u.Username, "第二条", "web", false)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if updated.UnreadAdminCount != 1 {
		t.Fatalf("returned unread_admin_count = %d, want 1", updated.UnreadAdminCount)
	}
	if updated.LastReplyBy != "user" {
		t.Fatalf("last_reply_by = %q, want user", updated.LastReplyBy)
	}
	if msg.TicketID != ticket.ID || msg.Content != "第二条" {
		t.Fatalf("unexpected message: %+v", msg)
	}
}
