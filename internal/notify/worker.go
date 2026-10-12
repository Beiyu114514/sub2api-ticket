package notify

import (
	"encoding/json"
	"errors"
	"log"
	"time"

	"gorm.io/gorm"

	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/store"
	"sub2api-ticket/internal/tgbot"
)

// Worker 通知 outbox 消费者：轮询 pending 通知 → 发送 → 失败指数退避重试。
type Worker struct {
	G       *gorm.DB
	S       *store.Settings
	T       *store.TicketStore
	Core    *tgbot.Core
	TGProxy string
}

func (w *Worker) Run() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		w.tick()
	}
}

func (w *Worker) tick() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[notify] panic: %v", r)
		}
	}()
	var notes []model.Notification
	if err := w.G.Where("status = ? AND next_attempt_at <= ?", "pending", time.Now()).
		Order("id ASC").Limit(10).Find(&notes).Error; err != nil {
		return
	}
	for i := range notes {
		err := w.dispatch(&notes[i])
		if err == nil {
			now := time.Now()
			w.G.Model(&notes[i]).Updates(map[string]any{"status": "sent", "sent_at": now, "last_error": ""})
		} else {
			retries := notes[i].Retries + 1
			if retries >= 8 {
				w.G.Model(&notes[i]).Updates(map[string]any{"status": "failed", "retries": retries, "last_error": err.Error()})
				log.Printf("[notify] #%d %s 放弃: %v", notes[i].ID, notes[i].Kind, err)
			} else {
				w.G.Model(&notes[i]).Updates(map[string]any{
					"retries": retries, "next_attempt_at": time.Now().Add(backoff(retries)), "last_error": err.Error(),
				})
			}
		}
	}
}

func backoff(retries int) time.Duration {
	switch retries {
	case 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	case 3:
		return 15 * time.Minute
	case 4:
		return 30 * time.Minute
	default:
		return 2 * time.Hour
	}
}

type notifyPayload struct {
	TicketID  uint   `json:"ticket_id"`
	MessageID uint   `json:"message_id"`
	Status    string `json:"status"`
	To        string `json:"to"`
	Operator  string `json:"operator"`
}

func (w *Worker) dispatch(n *model.Notification) error {
	var p notifyPayload
	if err := json.Unmarshal([]byte(n.Payload), &p); err != nil {
		return errors.New("payload 解析失败: " + err.Error())
	}
	bot := tgbot.NewWithProxy(w.S.BotToken(), w.TGProxy)
	if !bot.Valid() {
		return errors.New("bot token 未配置")
	}

	var ticket model.Ticket
	if err := w.G.Preload("User").First(&ticket, p.TicketID).Error; err != nil {
		return nil // 工单已删，视为完成
	}

	switch n.Kind {
	case model.NotifyAdminNewTicket, model.NotifyAdminNewReply:
		chatID, threadID, err := w.Core.EnsureTopic(bot, &ticket)
		if err != nil {
			return err
		}
		uname := ""
		if ticket.User != nil {
			uname = ticket.User.Username
		}
		var text string
		if n.Kind == model.NotifyAdminNewTicket {
			var msg model.TicketMessage
			content := ""
			if w.G.First(&msg, p.MessageID).Error == nil {
				content = msg.Content
			}
			text = tgbot.RenderAdminNewTicket(ticket.ID, ticket.Subject, ticket.Category, ticket.Priority, uname, content, w.Core.TicketURL(ticket.ID))
		} else {
			var msg model.TicketMessage
			if err := w.G.First(&msg, p.MessageID).Error; err != nil {
				return errors.New("消息不存在")
			}
			text = tgbot.RenderAdminNewReply(ticket.ID, ticket.Subject, msg.SenderName, msg.Content, msg.Via, w.Core.TicketURL(ticket.ID))
		}
		_, err = bot.SendMessage(tgbot.SendMessageOpts{
			ChatID: chatID, MessageThreadID: threadID, Text: text, DisableWebPagePreview: true,
		})
		return err

	case model.NotifyUserNewReply:
		var msg model.TicketMessage
		if err := w.G.First(&msg, p.MessageID).Error; err != nil {
			return errors.New("消息不存在")
		}
		var user model.User
		if err := w.G.First(&user, ticket.UserID).Error; err != nil {
			return nil
		}
		if user.TGChatID == 0 {
			return nil // 未绑定 TG，视为已处理
		}
		text := tgbot.RenderUserNewReply(ticket.ID, ticket.Subject, msg.SenderName, msg.Content, w.Core.TicketURL(ticket.ID))
		kb := &tgbot.InlineKeyboardMarkup{InlineKeyboard: [][]tgbot.InlineKeyboardButton{{
			{Text: "↩️ 回复此工单", CallbackData: "pick_ticket:" + itoa(ticket.ID)},
		}}}
		_, err := bot.SendMessage(tgbot.SendMessageOpts{
			ChatID: user.TGChatID, Text: text, ReplyMarkup: kb, DisableWebPagePreview: true,
		})
		return err

	case model.NotifyUserStatus:
		// 话题状态同步始终执行（best effort）
		w.Core.SyncTopicStatus(bot, &ticket, p.Status == "closed")
		if p.To == "admin" {
			// 用户主动操作（如自行关闭）→ 通知管理员群
			chatID, threadID, err := w.Core.EnsureTopic(bot, &ticket)
			if err != nil {
				return err
			}
			_, err = bot.SendMessage(tgbot.SendMessageOpts{
				ChatID: chatID, MessageThreadID: threadID,
				Text: tgbot.RenderUserStatus(ticket.ID, ticket.Subject, p.Status, "用户 "+p.Operator),
			})
			return err
		}
		// 通知用户
		var user model.User
		if err := w.G.First(&user, ticket.UserID).Error; err != nil || user.TGChatID == 0 {
			return nil
		}
		_, err := bot.SendMessage(tgbot.SendMessageOpts{
			ChatID: user.TGChatID,
			Text:   tgbot.RenderUserStatus(ticket.ID, ticket.Subject, p.Status, p.Operator),
		})
		return err
	}
	return nil
}

func itoa(n uint) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
