package llm

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"gorm.io/gorm"

	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/store"
)

// Agent LLM 自动客服：每分钟扫描满足触发条件的工单并生成回复。
// 不占用 notify worker；回复消息直接落库 + 走 outbox 通知用户。
type Agent struct {
	G *gorm.DB
	S *store.Settings
	T *store.TicketStore
}

const contextMessages = 20 // 喂给模型的上下文条数上限

// Run 每分钟扫描一次
func (a *Agent) Run() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		a.tick()
	}
}

func (a *Agent) tick() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[assistant] panic: %v", r)
		}
	}()
	if !a.S.AssistantEnabled() || a.S.AssistantBaseURL() == "" || a.S.AssistantAPIKey() == "" || a.S.AssistantModel() == "" {
		return
	}
	// 触发条件：open/pending + 最后一条来自 user + 静默超时 + 次数未满。
	// 只处理助手开启（或重新开启）之后收到的消息，不对历史积压工单批量回复。
	var tickets []model.Ticket
	silentBefore := time.Now().Add(-time.Duration(a.S.AssistantSilentMinutes()) * time.Minute)
	enabledAt := time.Unix(a.S.AssistantEnabledAt(), 0)
	if err := a.G.Where(
		"status IN ? AND last_reply_by = ? AND last_reply_at <= ? AND last_reply_at >= ? AND assistant_replies < ?",
		[]string{"open", "pending"}, "user", silentBefore, enabledAt, a.S.AssistantMaxReplies(),
	).Limit(5).Find(&tickets).Error; err != nil {
		return
	}
	for i := range tickets {
		a.replyOne(&tickets[i])
	}
}

// replyOne 对单个工单生成并落库一条 assistant 回复
func (a *Agent) replyOne(t *model.Ticket) {
	var msgs []model.TicketMessage
	if err := a.G.Where("ticket_id = ?", t.ID).Order("id ASC").Find(&msgs).Error; err != nil || len(msgs) == 0 {
		return
	}
	// 只取最近 N 条，且最后一条必须仍是 user（防并发窗口）
	recent := msgs
	if len(recent) > contextMessages {
		recent = recent[len(recent)-contextMessages:]
	}
	if recent[len(recent)-1].SenderRole != "user" {
		return
	}

	chat := []Message{{Role: "system", Content: a.S.AssistantSystemPrompt()}}
	for _, m := range recent {
		role := "user"
		switch m.SenderRole {
		case "admin", "assistant":
			role = "assistant"
		case "system":
			continue
		}
		chat = append(chat, Message{Role: role, Content: m.Content})
	}

	client := &Client{
		BaseURL: a.S.AssistantBaseURL(),
		APIKey:  a.S.AssistantAPIKey(),
		Model:   a.S.AssistantModel(),
	}
	content, err := client.ChatCompletion(context.Background(), chat, a.S.AssistantTemperature())
	if err != nil {
		log.Printf("[assistant] 工单 #%d 生成失败: %v", t.ID, err)
		return
	}

	// 重新加载数据，用事务保证“最后消息仍是 user”的检查到落库一致
	var msg *model.TicketMessage
	err = a.G.Transaction(func(tx *gorm.DB) error {
		var last model.TicketMessage
		if err := tx.Where("ticket_id = ?", t.ID).Order("id DESC").First(&last).Error; err != nil {
			return err
		}
		if last.SenderRole != "user" {
			return errSkip // 窗口期内有人回复了，放弃
		}
		msg = &model.TicketMessage{
			TicketID: t.ID, SenderRole: "assistant", SenderName: "AI 助手",
			Content: content, Via: "assistant",
		}
		if err := tx.Create(msg).Error; err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Model(&model.Ticket{}).Where("id = ?", t.ID).Updates(map[string]any{
			"last_reply_at": now, "last_reply_by": "assistant",
			"assistant_replies": gorm.Expr("assistant_replies + 1"),
			"unread_user_count": gorm.Expr("unread_user_count + 1"),
		}).Error; err != nil {
			return err
		}
		return nil
	})
	if err == errSkip {
		return
	}
	if err != nil || msg == nil {
		log.Printf("[assistant] 工单 #%d 落库失败: %v", t.ID, err)
		return
	}
	// outbox：推送用户（TG 已绑定时）
	a.G.Create(&model.Notification{
		Kind: model.NotifyUserAssistant, Status: "pending",
		Payload: mustAgentJSON(t.ID, msg.ID),
	})
}

// errSkip 触发窗口失效，静默放弃本次
var errSkip = &skipError{}

type skipError struct{}

func (*skipError) Error() string { return "skip" }

func mustAgentJSON(ticketID, messageID uint) string {
	b, _ := json.Marshal(map[string]any{"ticket_id": ticketID, "message_id": messageID})
	return string(b)
}
