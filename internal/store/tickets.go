package store

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/storage"
)

var (
	ErrTicketClosed = errors.New("工单已关闭，请先重新开启")
	ErrNotFound     = errors.New("工单不存在")
	ErrNoPermission = errors.New("无权访问该工单")
)

// TicketStore 工单核心逻辑：HTTP 与 TG 两条入口共用，保证状态与通知行为一致。
type TicketStore struct {
	G *gorm.DB
	S *Settings
}

func NewTicketStore(g *gorm.DB, s *Settings) *TicketStore {
	return &TicketStore{G: g, S: s}
}

func ValidStatus(s string) bool {
	return s == "open" || s == "pending" || s == "closed"
}

func ValidPriority(s string) bool {
	return s == "low" || s == "normal" || s == "high"
}

// CreateTicket 建单 + 首条消息（含附件）+ 管理员通知入队。
// 附件随事务一并落库，避免“单已建、附件丢失”的中间态。
func (st *TicketStore) CreateTicket(u *model.User, subject, category, priority, content string, atts ...storage.Attachment) (*model.Ticket, *model.TicketMessage, error) {
	if priority == "" {
		priority = "normal"
	}
	t := &model.Ticket{
		UserID:      u.ID,
		Subject:     subject,
		Category:    category,
		Priority:    priority,
		Status:      "open",
		LastReplyAt: time.Now(),
		LastReplyBy: "user",
	}
	now := time.Now()
	msg := &model.TicketMessage{
		SenderRole:  "user",
		SenderName:  u.Username,
		Content:     content,
		Attachments: storage.EncodeAttachments(atts),
		Via:         "web",
	}
	err := st.G.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(t).Error; err != nil {
			return err
		}
		msg.TicketID = t.ID
		msg.CreatedAt = now
		if err := tx.Create(msg).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	st.enqueueAdminNotify(model.NotifyAdminNewTicket, t.ID, msg.ID)
	return t, msg, nil
}

// AppendMessage 追加回复并做状态流转/未读/通知。sender 为 nil 时视为系统消息。
// atts 为已校验归属的附件（可为 nil）。
func (st *TicketStore) AppendMessage(ticketID uint, sender *model.User, senderRole, senderName, content, via string, closedOK bool, atts []storage.Attachment) (*model.Ticket, *model.TicketMessage, error) {
	var t model.Ticket
	if err := st.G.First(&t, ticketID).Error; err != nil {
		return nil, nil, ErrNotFound
	}
	if t.Status == "closed" && !closedOK {
		return nil, nil, ErrTicketClosed
	}
	if senderRole == "" {
		senderRole = "user"
	}
	msg := &model.TicketMessage{
		TicketID:    t.ID,
		SenderRole:  senderRole,
		SenderName:  senderName,
		Content:     content,
		Attachments: storage.EncodeAttachments(atts),
		Via:         via,
	}
	now := time.Now()
	updates := map[string]any{
		"last_reply_at": now,
		"last_reply_by": senderRole,
		"updated_at":    now,
	}
	switch senderRole {
	case "user":
		updates["status"] = "open"
		if t.Status == "closed" {
			updates["closed_at"] = nil
		}
		// 使用 SQL 表达式原子 +1，防止并发覆盖
		updates["unread_admin_count"] = gorm.Expr("unread_admin_count + 1")
	case "admin":
		updates["status"] = "pending"
		updates["unread_user_count"] = gorm.Expr("unread_user_count + 1")
		if t.Status == "closed" {
			updates["closed_at"] = nil // 管理员回复已关闭工单 = 重开
		}
	}
	err := st.G.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(msg).Error; err != nil {
			return err
		}
		if err := tx.Model(&t).Updates(updates).Error; err != nil {
			return err
		}
		// 在同一个事务内刷新最新的内存数据
		return tx.First(&t, ticketID).Error
	})
	if err != nil {
		return nil, nil, err
	}

	switch senderRole {
	case "user":
		st.enqueueAdminNotify(model.NotifyAdminNewReply, t.ID, msg.ID)
	case "admin":
		if st.S.NotifyUserEnabled() {
			st.G.Create(&model.Notification{
				Kind:          model.NotifyUserNewReply,
				Payload:       mustJSON(map[string]any{"ticket_id": t.ID, "message_id": msg.ID}),
				Status:        "pending",
				NextAttemptAt: time.Now(),
			})
		}
	}
	return &t, msg, nil
}

// ChangeStatus 状态变更。管理员可任意切换；用户只能关闭/重开自己的工单。
// actorRole: user / admin；actorName 用于 system 消息记录。
// 返回是否发生了 closed<->open 切换（需要同步 TG 话题）。
func (st *TicketStore) ChangeStatus(t *model.Ticket, actorRole, actorName, target string) (bool, error) {
	if !ValidStatus(target) || target == t.Status {
		return false, nil
	}
	old := t.Status
	now := time.Now()
	updates := map[string]any{"status": target, "updated_at": now}
	if target == "closed" {
		updates["closed_at"] = now
	} else {
		updates["closed_at"] = nil
	}
	if err := st.G.Model(t).Updates(updates).Error; err != nil {
		return false, err
	}
	t.Status = target

	desc := map[string]string{
		"closed":  "工单已关闭",
		"open":    "工单已重新开启",
		"pending": "等待用户回复",
	}[target]
	st.G.Create(&model.TicketMessage{
		TicketID:   t.ID,
		SenderRole: "system",
		SenderName: "system",
		Content:    fmt.Sprintf("%s（由 %s %s 操作）", desc, actorRole, actorName),
		Via:        "web",
	})

	// 用户侧通知
	if actorRole == "admin" && st.S.NotifyUserEnabled() {
		st.G.Create(&model.Notification{
			Kind:          model.NotifyUserStatus,
			Payload:       mustJSON(map[string]any{"ticket_id": t.ID, "status": target, "to": "user", "operator": actorName}),
			Status:        "pending",
			NextAttemptAt: time.Now(),
		})
	}
	// 管理员群通知用户主动关闭
	if actorRole == "user" && target == "closed" && st.S.NotifyAdminEnabled() {
		st.G.Create(&model.Notification{
			Kind:          model.NotifyUserStatus,
			Payload:       mustJSON(map[string]any{"ticket_id": t.ID, "status": target, "to": "admin", "operator": actorName}),
			Status:        "pending",
			NextAttemptAt: time.Now(),
		})
	}
	return (old == "closed") != (target == "closed"), nil
}

// enqueueAdminNotify 管理员通知统一入口（受开关控制）
func (st *TicketStore) enqueueAdminNotify(kind string, ticketID, messageID uint) {
	if !st.S.NotifyAdminEnabled() {
		return
	}
	st.G.Create(&model.Notification{
		Kind:          kind,
		Payload:       mustJSON(map[string]any{"ticket_id": ticketID, "message_id": messageID}),
		Status:        "pending",
		NextAttemptAt: time.Now(),
	})
}

// SetMessageAttachments 为已创建的消息补写附件（建单流程：先建单再挂附件）
func (st *TicketStore) SetMessageAttachments(messageID uint, atts []storage.Attachment) error {
	return st.G.Model(&model.TicketMessage{ID: messageID}).
		Update("attachments", storage.EncodeAttachments(atts)).Error
}

func mustJSON(v any) string {
	b, _ := jsonMarshal(v)
	return string(b)
}
