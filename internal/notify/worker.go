package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/storage"
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
		if err != nil {
			return err
		}
		return w.sendAttachments(bot, chatID, threadID, p.MessageID)

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
		if err != nil {
			return err
		}
		return w.sendAttachments(bot, user.TGChatID, 0, p.MessageID)

	case model.NotifyUserAssistant:
		// AI 自动回复 → 推送绑定 TG 的用户（复用 user_new_reply 渲染，带 AI 标识）
		var msg model.TicketMessage
		if err := w.G.First(&msg, p.MessageID).Error; err != nil {
			return errors.New("消息不存在")
		}
		var user model.User
		if err := w.G.First(&user, ticket.UserID).Error; err != nil {
			return nil
		}
		if user.TGChatID == 0 {
			return nil
		}
		text := tgbot.RenderUserNewReply(ticket.ID, ticket.Subject, "🤖 "+msg.SenderName, msg.Content, w.Core.TicketURL(ticket.ID))
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

// sendAttachments 消息带图片时，下载对象后用 sendPhoto/sendMediaGroup 转发到 TG。
// 需要拿到原始字节（S3/本地均可），经 Core.Uploads.OpenByKey 读取。
func (w *Worker) sendAttachments(bot *tgbot.Client, chatID int64, threadID int, messageID uint) error {
	var msg model.TicketMessage
	if err := w.G.First(&msg, messageID).Error; err != nil {
		return nil
	}
	atts := storage.DecodeAttachments(msg.Attachments)
	if len(atts) == 0 {
		return nil
	}
	if w.Core == nil || w.Core.Uploads == nil || w.Core.Uploads.OpenByKey == nil {
		return nil
	}
	// 下载前 5 张（与消息附件上限一致）
	type item struct {
		rc   io.ReadCloser
		mime string
	}
	items := make([]item, 0, len(atts))
	defer func() {
		for _, it := range items {
			it.rc.Close()
		}
	}()
	for _, a := range atts {
		if len(items) >= 5 {
			break
		}
		key, err := attachmentKey(a.URL)
		if err != nil {
			continue
		}
		rc, mime, err := w.Core.Uploads.OpenByKey(context.Background(), key)
		if err != nil {
			continue
		}
		items = append(items, item{rc: rc, mime: mime})
	}
	if len(items) == 0 {
		return nil
	}
	// TG 不接受 URL 直接发图（需可公网访问），这里逐张上传字节。
	// sendPhoto/sendMediaGroup 支持 multipart file 上传。
	if len(items) == 1 {
		data, err := io.ReadAll(items[0].rc)
		if err != nil {
			return nil
		}
		return bot.UploadPhoto(chatID, threadID, data, items[0].mime, "")
	}
	inputs := make([]tgbot.MediaInput, 0, len(items))
	for i, it := range items {
		data, err := io.ReadAll(it.rc)
		if err != nil {
			continue
		}
		inputs = append(inputs, tgbot.MediaInput{Data: data, Mime: it.mime,
			Caption: firstCaption(i, msg.Content)})
	}
	if len(inputs) == 0 {
		return nil
	}
	return bot.UploadMediaGroup(chatID, threadID, inputs)
}

func firstCaption(i int, content string) string {
	if i == 0 && content != "" && content != "[图片]" {
		return content
	}
	return ""
}

// attachmentKey 从附件 URL 提取对象 key（与 api.keyFromURL 同规则）
func attachmentKey(raw string) (string, error) {
	if i := strings.Index(raw, "?"); i >= 0 {
		raw = raw[:i]
	}
	if j := strings.Index(raw, "/uploads/"); j >= 0 {
		return strings.TrimPrefix(raw[j+len("/uploads/"):], "/"), nil
	}
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	if len(parts) >= 3 {
		return strings.Join(parts[len(parts)-3:], "/"), nil
	}
	return "", errors.New("无法解析 key")
}
