package tgbot

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/storage"
	"sub2api-ticket/internal/store"
)

// Core TG 消息分发核心：管理员话题群 + 用户私聊两条链路。
type Core struct {
	G             *gorm.DB
	S             *store.Settings
	T             *store.TicketStore
	PublicBaseURL string
	TGProxy       string
	Pool          *UpdateWorkerPool
	Uploads       *SaveImageDeps   // 图片上传钩子（main 装配时注入，可能为 nil）
	groupBuf      mediaGroupBuffer // 相册聚合缓冲

	// 群成员权限缓存，避免每条消息都打一次 getChatMember
	memberMu    sync.Mutex
	memberCache map[memberKey]memberEntry
}

type memberKey struct {
	chatID int64
	userID int64
}

type memberEntry struct {
	admin bool
	exp   time.Time
}

func (c *Core) SubmitTask(task func()) bool {
	if c.Pool != nil {
		return c.Pool.Submit(task)
	}
	go task()
	return true
}

func (c *Core) newBot() *Client { return NewWithProxy(c.S.BotToken(), c.TGProxy) }

// DispatchRaw webhook 异步处理入口
func (c *Core) DispatchRaw(body []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[tgbot] panic: %v", r)
		}
	}()
	var u Update
	if err := json.Unmarshal(body, &u); err != nil {
		log.Printf("[tgbot] update 解析失败: %v", err)
		return
	}
	bot := c.newBot()
	if !bot.Valid() {
		return
	}
	c.HandleUpdate(bot, u)
}

func (c *Core) HandleUpdate(bot *Client, u Update) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[tgbot] handle panic: %v", r)
		}
	}()
	if u.CallbackQuery != nil {
		c.onCallback(bot, u.CallbackQuery)
		return
	}
	if u.Message == nil {
		return
	}
	m := u.Message
	if m.From != nil && m.From.IsBot {
		return // 防回环
	}
	// 相册：同 media_group_id 的多条 update 聚合 1.5s 后作为一条消息处理
	if m.MediaGroupID != "" && len(m.Photo) > 0 {
		c.bufferMediaGroup(u, m)
		return
	}
	if m.Chat.ID == c.S.AdminGroupID() {
		c.onAdminGroup(bot, m)
		return
	}
	if m.Chat.Type == "private" {
		c.onPrivate(bot, m)
	}
}

// ———————— 管理员群 ————————

var topicIDRe = regexp.MustCompile(`^#(\d+)`)

func (c *Core) onAdminGroup(bot *Client, m *Message) {
	// 管理员群里的消息只有群主/管理员本人才能被当成客服回复。
	// Telegram 超级群默认允许成员发言，仅凭 chat_id 相等无法区分身份，
	// 否则任何被拉进群的人都能冒充管理员回复用户、关闭任意工单。
	if !c.isGroupAdmin(bot, m.Chat.ID, m.From) {
		return
	}
	// 手动创建的话题名若以 #工单ID 开头，自动建立映射
	if m.ForumTopicCreated != nil {
		if match := topicIDRe.FindStringSubmatch(m.ForumTopicCreated.Name); match != nil {
			if id, err := strconv.ParseUint(match[1], 10, 64); err == nil {
				var t model.Ticket
				if err := c.G.First(&t, uint(id)).Error; err == nil {
					c.G.Save(&model.TicketTopic{TicketID: t.ID, ChatID: m.Chat.ID, ThreadID: m.MessageThreadID})
				}
			}
		}
		return
	}
	text := strings.TrimSpace(m.Text)
	if strings.HasPrefix(text, "/") {
		c.onAdminCommand(bot, m, text)
		return
	}
	if m.MessageThreadID == 0 || m.IsAutomaticForward {
		return
	}
	var tt model.TicketTopic
	if err := c.G.Where("chat_id = ? AND thread_id = ?", m.Chat.ID, m.MessageThreadID).First(&tt).Error; err != nil {
		return // 非工单话题
	}
	name := senderDisplayName(m.From)
	// 管理员群里的图片：下载后走正常上传链路，作为附件回复
	if atts := c.collectPhotos(bot, m); len(atts) > 0 {
		text := strings.TrimSpace(m.Caption)
		if text != "" && strings.HasPrefix(text, "/") {
			c.onAdminCommand(bot, m, text)
			return
		}
		if _, _, err := c.T.AppendMessage(tt.TicketID, nil, "admin", name, text, "telegram", true, atts); err != nil {
			c.reply(bot, m, "⚠️ 同步失败："+Escape(err.Error()))
			return
		}
		if c.Uploads != nil && c.Uploads.Rebind != nil {
			c.Uploads.Rebind(0, tt.TicketID, atts)
		}
		c.ackPhotos(bot, m, tt.TicketID)
		return
	}
	if text == "" {
		return
	}
	_, _, err := c.T.AppendMessage(tt.TicketID, nil, "admin", name, text, "telegram", true, nil)
	if err != nil {
		bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, MessageThreadID: m.MessageThreadID,
			Text: "⚠️ 同步失败：" + Escape(err.Error())})
	}
}

func (c *Core) onAdminCommand(bot *Client, m *Message, text string) {
	parts := strings.Fields(text)
	cmd := strings.Split(parts[0], "@")[0] // 去掉 /cmd@botname 后缀
	argID := uint(0)
	if len(parts) > 1 {
		if n, err := strconv.ParseUint(strings.TrimPrefix(parts[1], "#"), 10, 64); err == nil {
			argID = uint(n)
		}
	}
	switch cmd {
	case "/id":
		bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, MessageThreadID: m.MessageThreadID,
			Text: fmt.Sprintf("本群 chat_id：<code>%d</code>（is_forum 话题群）", m.Chat.ID)})
	case "/list":
		c.adminList(bot, m)
	case "/close":
		if t := c.ticketForCmd(bot, m, argID); t != nil {
			if changed, _ := c.T.ChangeStatus(t, "admin", senderDisplayName(m.From), "closed"); !changed {
				c.reply(bot, m, "ℹ️ 工单已是关闭状态")
			} else {
				c.reply(bot, m, fmt.Sprintf("⚫️ 工单 #%d 已关闭", t.ID))
			}
		}
	case "/open":
		if t := c.ticketForCmd(bot, m, argID); t != nil {
			if changed, _ := c.T.ChangeStatus(t, "admin", senderDisplayName(m.From), "open"); !changed {
				c.reply(bot, m, "ℹ️ 工单未处于关闭状态")
			} else {
				c.reply(bot, m, fmt.Sprintf("🟢 工单 #%d 已重新开启", t.ID))
			}
		}
	default:
		c.reply(bot, m, "可用命令：/list 进行中工单 · /close [#id] · /open [#id] · /id")
	}
}

func (c *Core) ticketForCmd(bot *Client, m *Message, argID uint) *model.Ticket {
	id := argID
	if id == 0 && m.MessageThreadID > 0 {
		var tt model.TicketTopic
		if err := c.G.Where("chat_id = ? AND thread_id = ?", m.Chat.ID, m.MessageThreadID).First(&tt).Error; err == nil {
			id = tt.TicketID
		}
	}
	if id == 0 {
		c.reply(bot, m, "请提供工单号或在工单话题内使用")
		return nil
	}
	var t model.Ticket
	if err := c.G.First(&t, id).Error; err != nil {
		return nil
	}
	return &t
}

func (c *Core) adminList(bot *Client, m *Message) {
	var tickets []model.Ticket
	c.G.Preload("User").Where("status IN ?", []string{"open", "pending"}).
		Order("last_reply_at ASC").Limit(10).Find(&tickets)
	if len(tickets) == 0 {
		c.reply(bot, m, "✅ 暂无进行中的工单")
		return
	}
	var b strings.Builder
	b.WriteString("📋 进行中的工单：\n")
	for _, t := range tickets {
		uname := ""
		if t.User != nil {
			uname = t.User.Username
		}
		fmt.Fprintf(&b, "#%d %s · %s · %s（%s）\n", t.ID, StatusLabel(t.Status), Escape(Truncate(t.Subject, 40)), uname, t.LastReplyAt.Format("01-02 15:04"))
	}
	c.reply(bot, m, b.String())
}

// EnsureTopic 确保工单对应的管理员群话题存在，返回 chat_id/thread_id
func (c *Core) EnsureTopic(bot *Client, ticket *model.Ticket) (int64, int, error) {
	var tt model.TicketTopic
	if err := c.G.Where("ticket_id = ?", ticket.ID).First(&tt).Error; err == nil {
		return tt.ChatID, tt.ThreadID, nil
	}
	groupID := c.S.AdminGroupID()
	if groupID == 0 {
		return 0, 0, errors.New("未配置管理员群 ID")
	}
	name := Truncate(fmt.Sprintf("#%d %s", ticket.ID, ticket.Subject), 90)
	thread, err := bot.CreateForumTopic(groupID, name)
	if err != nil {
		return 0, 0, fmt.Errorf("创建话题失败: %w", err)
	}
	c.G.Create(&model.TicketTopic{TicketID: ticket.ID, ChatID: groupID, ThreadID: thread})
	return groupID, thread, nil
}

// SyncTopicStatus 工单关闭/重开时同步话题状态
func (c *Core) SyncTopicStatus(bot *Client, ticket *model.Ticket, closed bool) {
	var tt model.TicketTopic
	if err := c.G.Where("ticket_id = ?", ticket.ID).First(&tt).Error; err != nil {
		return
	}
	if closed {
		_ = bot.CloseForumTopic(tt.ChatID, tt.ThreadID)
	} else {
		_ = bot.ReopenForumTopic(tt.ChatID, tt.ThreadID)
	}
}

// ———————— 用户私聊 ————————

func (c *Core) onPrivate(bot *Client, m *Message) {
	text := strings.TrimSpace(m.Text)
	caption := strings.TrimSpace(m.Caption)
	if text == "" && caption == "" && len(m.Photo) == 0 {
		return
	}
	if strings.HasPrefix(text, "/start") {
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/start"))
		if strings.HasPrefix(arg, "bind_") {
			c.bindUser(bot, m, strings.TrimPrefix(arg, "bind_"))
			return
		}
		bot.SendMessage(SendMessageOpts{
			ChatID: m.Chat.ID,
			Text: "👋 你好！我是 sub2api 工单助手。\n\n" +
				"在网页端绑定 Telegram 后，这里会推送你工单的新回复。\n" +
				"直接回复消息即可同步到当前工单；/list 查看进行中的工单。",
		})
		return
	}
	if text == "/list" || text == "/tickets" {
		c.listUserTickets(bot, m)
		return
	}
	if strings.HasPrefix(text, "/") {
		bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, Text: "未知命令。可用：/list 查看进行中的工单"})
		return
	}

	u := c.userByChatID(m.Chat.ID)
	if u == nil {
		bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID,
			Text: "⚠️ 尚未绑定。请先登录工单网页 → 设置 → 绑定 Telegram，再回到这里。"})
		return
	}

	// 图片消息：下载 → 上传服务处理 → 作为附件回复到当前工单
	if atts := c.collectPhotos(bot, m); len(atts) > 0 {
		content := caption
		if content == "" {
			content = "[图片]"
		}
		ticketID := c.pickTicket(u)
		if ticketID == 0 {
			bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID,
				Text: "你当前没有进行中的工单。请到网页端创建工单后，再来这里发送。"})
			return
		}
		_, _, err := c.T.AppendMessage(ticketID, u, "user", u.Username, content, "telegram", false, atts)
		if err == store.ErrTicketClosed {
			bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, Text: "该工单已关闭，请到网页端重新开启后再回复。"})
			return
		}
		if err != nil {
			bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, Text: "⚠️ 图片发送失败，请稍后再试。"})
			return
		}
		if c.Uploads != nil && c.Uploads.Rebind != nil {
			c.Uploads.Rebind(u.ID, ticketID, atts)
		}
		bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID,
			Text: fmt.Sprintf("✅ 图片已发送到工单 #%d（%d 张）", ticketID, len(atts))})
		return
	}
	ticketID := c.pickTicket(u)
	if ticketID == 0 {
		bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID,
			Text: "你当前没有进行中的工单。请到网页端创建工单后，再来这里回复。"})
		return
	}
	_, _, err := c.T.AppendMessage(ticketID, u, "user", u.Username, text, "telegram", false, nil)
	if err == store.ErrTicketClosed {
		bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, Text: "该工单已关闭，请到网页端重新开启后再回复。"})
		return
	}
	if err != nil {
		bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, Text: "⚠️ 发送失败，请稍后再试。"})
		return
	}
	bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, Text: fmt.Sprintf("✅ 已发送到工单 #%d", ticketID)})
}

// bindUser 处理 /start bind_<code>
func (c *Core) bindUser(bot *Client, m *Message, code string) {
	var bc model.BindCode
	if err := c.G.Where("code = ?", code).First(&bc).Error; err != nil || time.Now().After(bc.ExpiresAt) {
		bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, Text: "⚠️ 绑定码无效或已过期，请回到网页重新生成。"})
		return
	}
	var u model.User
	if err := c.G.First(&u, bc.UserID).Error; err != nil {
		bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, Text: "⚠️ 用户不存在。"})
		return
	}
	// 一个 TG 账号只能绑定一个工单用户：顶掉旧绑定
	c.G.Model(&model.User{}).Where("tg_chat_id = ? AND id != ?", m.Chat.ID, u.ID).Update("tg_chat_id", 0)
	c.G.Model(&u).Update("tg_chat_id", m.Chat.ID)
	c.G.Delete(&bc)
	c.G.Save(&model.UserChatState{ChatID: m.Chat.ID, CurrentTicketID: c.pickTicket(&u), UpdatedAt: time.Now()})

	bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID,
		Text: fmt.Sprintf("✅ 绑定成功，%s！\n之后工单有新回复会第一时间推送到这里，直接回复消息即可答复。", Escape(u.Username))})
	c.listUserTickets(bot, m)
}

func (c *Core) listUserTickets(bot *Client, m *Message) {
	u := c.userByChatID(m.Chat.ID)
	if u == nil {
		bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID,
			Text: "⚠️ 尚未绑定。请先登录工单网页 → 设置 → 绑定 Telegram。"})
		return
	}
	var tickets []model.Ticket
	c.G.Where("user_id = ? AND status IN ?", u.ID, []string{"open", "pending"}).
		Order("last_reply_at DESC").Limit(10).Find(&tickets)
	if len(tickets) == 0 {
		bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, Text: "你目前没有进行中的工单。"})
		return
	}
	var kb [][]InlineKeyboardButton
	var b strings.Builder
	b.WriteString("📋 你的进行中工单（点击切换回复目标）：\n")
	for _, t := range tickets {
		fmt.Fprintf(&b, "#%d %s · %s\n", t.ID, StatusLabel(t.Status), Escape(Truncate(t.Subject, 40)))
		kb = append(kb, []InlineKeyboardButton{{
			Text:         fmt.Sprintf("#%d %s", t.ID, Truncate(t.Subject, 30)),
			CallbackData: fmt.Sprintf("pick_ticket:%d", t.ID),
		}})
	}
	bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, Text: b.String(), ReplyMarkup: &InlineKeyboardMarkup{InlineKeyboard: kb}})
}

func (c *Core) onCallback(bot *Client, q *CallbackQuery) {
	if !strings.HasPrefix(q.Data, "pick_ticket:") {
		bot.AnswerCallbackQuery(q.ID, "")
		return
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(q.Data, "pick_ticket:"), 10, 64)
	if err != nil {
		bot.AnswerCallbackQuery(q.ID, "无效工单")
		return
	}
	var t model.Ticket
	if err := c.G.First(&t, uint(id)).Error; err != nil {
		bot.AnswerCallbackQuery(q.ID, "工单不存在")
		return
	}
	// 仅工单所有者可切换
	var u model.User
	if err := c.G.Where("tg_chat_id = ?", q.From.ID).First(&u).Error; err != nil || u.ID != t.UserID {
		bot.AnswerCallbackQuery(q.ID, "无权操作该工单")
		return
	}
	c.G.Save(&model.UserChatState{ChatID: q.Message.Chat.ID, CurrentTicketID: t.ID, UpdatedAt: time.Now()})
	bot.AnswerCallbackQuery(q.ID, fmt.Sprintf("✅ 已切换，回复将发送到工单 #%d", t.ID))
}

// ———————— 辅助 ————————

// isGroupAdmin 判断发言者是否是管理员群的群主/管理员。
// 结果缓存 5 分钟（非管理员/查询失败缓存 30 秒）；查询失败按无权限处理（fail-closed），
// 宁可暂时不响应，也不能让未知身份的人以客服口吻给用户回话。
func (c *Core) isGroupAdmin(bot *Client, chatID int64, u *User) bool {
	if bot == nil || u == nil {
		return false
	}
	key := memberKey{chatID: chatID, userID: u.ID}

	c.memberMu.Lock()
	if e, ok := c.memberCache[key]; ok && time.Now().Before(e.exp) {
		c.memberMu.Unlock()
		return e.admin
	}
	c.memberMu.Unlock()

	member, err := bot.GetChatMember(chatID, u.ID)
	admin := err == nil && member.IsAdmin()
	if err != nil {
		log.Printf("[tgbot] getChatMember 失败 chat=%d user=%d: %v", chatID, u.ID, err)
	}

	ttl := 5 * time.Minute
	if !admin {
		ttl = 30 * time.Second
	}
	c.memberMu.Lock()
	if c.memberCache == nil {
		c.memberCache = map[memberKey]memberEntry{}
	}
	c.memberCache[key] = memberEntry{admin: admin, exp: time.Now().Add(ttl)}
	c.memberMu.Unlock()
	return admin
}

func (c *Core) userByChatID(chatID int64) *model.User {
	var u model.User
	if err := c.G.Where("tg_chat_id = ?", chatID).First(&u).Error; err != nil {
		return nil
	}
	return &u
}

// pickTicket 用户当前回复目标：chat state > 最近活跃的 open/pending 工单
func (c *Core) pickTicket(u *model.User) uint {
	var st model.UserChatState
	if err := c.G.Where("chat_id = ?", u.TGChatID).First(&st).Error; err == nil && st.CurrentTicketID != 0 {
		var t model.Ticket
		if err := c.G.Where("id = ? AND user_id = ? AND status != ?", st.CurrentTicketID, u.ID, "closed").First(&t).Error; err == nil {
			return t.ID
		}
	}
	var t model.Ticket
	if err := c.G.Where("user_id = ? AND status IN ?", u.ID, []string{"open", "pending"}).
		Order("last_reply_at DESC").First(&t).Error; err == nil {
		return t.ID
	}
	return 0
}

func (c *Core) lookupTicketByThread(chatID int64, threadID int) uint {
	var tt model.TicketTopic
	if err := c.G.Where("chat_id = ? AND thread_id = ?", chatID, threadID).First(&tt).Error; err != nil {
		return 0
	}
	return tt.TicketID
}

func (c *Core) reply(bot *Client, m *Message, text string) {
	if bot == nil {
		return
	}
	bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, MessageThreadID: m.MessageThreadID, Text: text})
}

// TicketURL 网页端工单详情链接
func (c *Core) TicketURL(id uint) string {
	if c.PublicBaseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/tickets/%d", c.PublicBaseURL, id)
}

func senderDisplayName(u *User) string {
	if u == nil {
		return "admin"
	}
	name := u.DisplayName()
	if u.Username != "" {
		return name + " (@" + u.Username + ")"
	}
	return name
}

// ———————— 图片消息 ————————

// bufferMediaGroup 聚合相册消息；窗口关闭后合成一条带头消息（caption 在第一条）分发
func (c *Core) bufferMediaGroup(u Update, m *Message) {
	c.groupBuf.mu.Lock()
	if c.groupBuf.items == nil {
		c.groupBuf.items = map[string][]*Message{}
		c.groupBuf.timer = map[string]*time.Timer{}
	}
	key := fmt.Sprintf("%d:%s", m.Chat.ID, m.MediaGroupID)
	c.groupBuf.items[key] = append(c.groupBuf.items[key], m)
	if _, ok := c.groupBuf.timer[key]; !ok {
		g := key
		c.groupBuf.timer[g] = time.AfterFunc(1500*time.Millisecond, func() {
			msgs := c.groupBuf.take(g)
			if len(msgs) == 0 {
				return
			}
			head := msgs[0]
			if head.Caption == "" {
				for _, x := range msgs {
					if x.Caption != "" {
						head.Caption = x.Caption
						break
					}
				}
			}
			for _, x := range msgs[1:] {
				head.Photo = append(head.Photo, x.Photo...)
			}
			c.SubmitTask(func() {
				bot := c.newBot()
				if bot.Valid() {
					c.HandleUpdate(bot, Update{Message: head})
				}
			})
		})
	}
	c.groupBuf.mu.Unlock()
}

// collectPhotos 下载消息里的图片（photo 取每种尺寸中最大；image/* document 也接受），
// 经 Uploads 钩子走完整上传链路（校验/压缩/审计/限速）。失败图片跳过并提示。
func (c *Core) collectPhotos(bot *Client, m *Message) []storage.Attachment {
	if c.Uploads == nil || c.Uploads.SaveImage == nil {
		if len(m.Photo) > 0 {
			log.Printf("[tgbot] Uploads 未注入，忽略图片消息 chat=%d", m.Chat.ID)
		}
		return nil
	}
	// 每张图 TG 返回多组尺寸（不同 file_id）；聚合后按 FileSize 分组，
	// 每组取最大的一个 file_id。同一张图的尺寸 file_id 不同但可按 width 唯一分组：
	// 简化处理 —— 逐个尺寸下载会重复，这里按“每个消息一张图、聚合 Photo 按
	// MessageID 已合并”的前提，取全局最大尺寸数量 = 相册张数不可靠，因此按
	// PhotoSize 中 width 唯一性过滤缩略图（TG 缩略图 width 通常 ≤400）。
	var fileIDs []struct{ id, name string }
	seen := map[string]bool{}
	for i := range m.Photo {
		p := &m.Photo[i]
		if p.Width <= 400 || seen[p.FileID] {
			continue
		}
		seen[p.FileID] = true
		fileIDs = append(fileIDs, struct{ id, name string }{p.FileID, ""})
	}
	if len(fileIDs) == 0 && len(m.Photo) > 0 {
		// 只有缩略图（极端情况）：取最大者
		if p := m.LargestPhoto(); p != nil {
			fileIDs = append(fileIDs, struct{ id, name string }{p.FileID, ""})
		}
	}
	if m.Document != nil && strings.HasPrefix(m.Document.MimeType, "image/") {
		fileIDs = append(fileIDs, struct{ id, name string }{m.Document.FileID, m.Document.FileName})
	}
	if len(fileIDs) == 0 {
		return nil
	}
	var u *model.User
	if m.Chat.Type == "private" {
		u = c.userByChatID(m.Chat.ID)
		if u == nil {
			return nil // 未绑定用户在 onPrivate 前置分支已提示
		}
	} else {
		// 管理员群：无对应用户，构造占位用户落审计（SaveImage 内部有全局限速兜底）
		u = &model.User{Username: senderDisplayName(m.From)}
	}
	var atts []storage.Attachment
	var lastErr error
	for _, f := range fileIDs {
		if len(atts) >= 5 {
			break // 与 Web 端一致：每条消息最多 5 张
		}
		data, err := bot.DownloadFile(f.id)
		if err != nil {
			lastErr = err
			continue
		}
		att, err := c.Uploads.SaveImage(u, data, f.name)
		if err != nil {
			lastErr = err
			continue
		}
		atts = append(atts, *att)
	}
	if len(atts) == 0 && lastErr != nil {
		bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, MessageThreadID: m.MessageThreadID,
			Text: "⚠️ 图片处理失败：" + Escape(lastErr.Error())})
	}
	return atts
}

// ackPhotos 管理员群图片同步成功后的简短确认
func (c *Core) ackPhotos(bot *Client, m *Message, ticketID uint) {
	bot.SendMessage(SendMessageOpts{ChatID: m.Chat.ID, MessageThreadID: m.MessageThreadID,
		Text: fmt.Sprintf("✅ 图片已同步到工单 #%d", ticketID)})
}
