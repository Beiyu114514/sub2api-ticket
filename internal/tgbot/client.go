package tgbot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// apiBase 抽成变量以便测试指向本地假 Telegram 服务。
var apiBase = "https://api.telegram.org"

// Client Telegram Bot API 轻量客户端，只封装本项目用到的方法。
type Client struct {
	Token string
	HTTP  *http.Client
}

func New(token string) *Client {
	return &Client{Token: token, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// NewWithProxy 支持经 http/socks5 代理访问 Bot API（部署环境无法直连 api.telegram.org 时使用）
func NewWithProxy(token, proxy string) *Client {
	if proxy == "" {
		return New(token)
	}
	tr := &http.Transport{Proxy: http.ProxyURL(mustURL(proxy))}
	return &Client{Token: token, HTTP: &http.Client{Timeout: 20 * time.Second, Transport: tr}}
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic("TG_PROXY 无效: " + s)
	}
	return u
}

func (c *Client) Valid() bool { return c != nil && c.Token != "" }

// —— API 类型 ——

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

func (u User) DisplayName() string {
	if u.FirstName != "" {
		return u.FirstName
	}
	return u.Username
}

// ChatMember getChatMember 返回的群成员信息，仅关心权限状态。
type ChatMember struct {
	Status string `json:"status"`
	User   *User  `json:"user"`
}

// IsAdmin 是否为群主或管理员。
func (m *ChatMember) IsAdmin() bool {
	return m != nil && (m.Status == "creator" || m.Status == "administrator")
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type Message struct {
	MessageID          int                `json:"message_id"`
	From               *User              `json:"from"`
	Chat               Chat               `json:"chat"`
	Date               int64              `json:"date"`
	Text               string             `json:"text"`
	Caption            string             `json:"caption"`
	MediaGroupID       string             `json:"media_group_id"`
	Photo              []PhotoSize        `json:"photo"`
	Document           *Document          `json:"document"`
	MessageThreadID    int                `json:"message_thread_id"`
	IsAutomaticForward bool               `json:"is_automatic_forward"`
	ForumTopicCreated  *ForumTopicCreated `json:"forum_topic_created"`
	ForumTopicClosed   *map[string]any    `json:"forum_topic_closed"`
	ForumTopicReopened *map[string]any    `json:"forum_topic_reopened"`
	ReplyToMessage     *Message           `json:"reply_to_message"`
}

type PhotoSize struct {
	FileID       string `json:"file_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int64  `json:"file_size"`
}

type Document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

// LargestPhoto 取最大尺寸的 photo（发图消息）
func (m *Message) LargestPhoto() *PhotoSize {
	if len(m.Photo) == 0 {
		return nil
	}
	best := &m.Photo[0]
	for i := range m.Photo {
		if m.Photo[i].FileSize > best.FileSize {
			best = &m.Photo[i]
		}
	}
	return best
}

type ForumTopicCreated struct {
	Name string `json:"name"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type SendMessageOpts struct {
	ChatID                int64
	MessageThreadID       int
	Text                  string
	ParseMode             string
	ReplyMarkup           *InlineKeyboardMarkup
	DisableWebPagePreview bool
}

// —— Update 来源 ——

// GetUpdates long polling
func (c *Client) GetUpdates(offset int64, timeout int) ([]Update, error) {
	var out struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	err := c.call("getUpdates", map[string]any{
		"offset": offset, "timeout": timeout, "allowed_updates": []string{"message", "callback_query"},
	}, &out)
	if err != nil {
		return nil, err
	}
	return out.Result, nil
}

// —— 方法 ——

func (c *Client) GetMe() (*User, error) {
	var out struct {
		OK     bool `json:"ok"`
		Result User `json:"result"`
	}
	if err := c.call("getMe", nil, &out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("getMe failed")
	}
	return &out.Result, nil
}

// GetChatMember 查询群成员状态，用于确认发言者是否真的具备管理员权限。
func (c *Client) GetChatMember(chatID, userID int64) (*ChatMember, error) {
	var out struct {
		OK     bool       `json:"ok"`
		Result ChatMember `json:"result"`
	}
	if err := c.call("getChatMember", map[string]any{
		"chat_id": chatID, "user_id": userID,
	}, &out); err != nil {
		return nil, err
	}
	return &out.Result, nil
}

func (c *Client) SetWebhook(url, secret string) error {
	var out struct {
		OK bool `json:"ok"`
	}
	return c.call("setWebhook", map[string]any{
		"url": url, "secret_token": secret,
		"allowed_updates": []string{"message", "callback_query"},
	}, &out)
}

func (c *Client) DeleteWebhook() error {
	var out struct {
		OK bool `json:"ok"`
	}
	return c.call("deleteWebhook", map[string]any{"drop_pending_updates": false}, &out)
}

func (c *Client) GetWebhookInfo() (map[string]any, error) {
	var out struct {
		OK     bool           `json:"ok"`
		Result map[string]any `json:"result"`
	}
	if err := c.call("getWebhookInfo", nil, &out); err != nil {
		return nil, err
	}
	return out.Result, nil
}

func (c *Client) SendMessage(o SendMessageOpts) (int, error) {
	params := map[string]any{
		"chat_id": o.ChatID, "text": o.Text, "parse_mode": "HTML",
		"disable_web_page_preview": o.DisableWebPagePreview,
	}
	if o.MessageThreadID > 0 {
		params["message_thread_id"] = o.MessageThreadID
	}
	if o.ReplyMarkup != nil {
		params["reply_markup"] = o.ReplyMarkup
	}
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := c.call("sendMessage", params, &out); err != nil {
		return 0, err
	}
	return out.Result.MessageID, nil
}

func (c *Client) CreateForumTopic(chatID int64, name string) (int, error) {
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageThreadID int `json:"message_thread_id"`
		} `json:"result"`
	}
	err := c.call("createForumTopic", map[string]any{"chat_id": chatID, "name": name}, &out)
	if err != nil {
		return 0, err
	}
	return out.Result.MessageThreadID, nil
}

func (c *Client) CloseForumTopic(chatID int64, threadID int) error {
	var out struct {
		OK bool `json:"ok"`
	}
	return c.call("closeForumTopic", map[string]any{"chat_id": chatID, "message_thread_id": threadID}, &out)
}

func (c *Client) ReopenForumTopic(chatID int64, threadID int) error {
	var out struct {
		OK bool `json:"ok"`
	}
	return c.call("reopenForumTopic", map[string]any{"chat_id": chatID, "message_thread_id": threadID}, &out)
}

func (c *Client) AnswerCallbackQuery(id, text string) {
	_ = c.call("answerCallbackQuery", map[string]any{"callback_query_id": id, "text": text}, &struct{}{})
}

// SendPhoto 发送单张图片（可带 caption）
func (c *Client) SendPhoto(chatID int64, threadID int, fileID, caption string) (int, error) {
	params := map[string]any{
		"chat_id": chatID, "photo": fileID, "parse_mode": "HTML",
	}
	if threadID > 0 {
		params["message_thread_id"] = threadID
	}
	if caption != "" {
		params["caption"] = caption
	}
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := c.call("sendPhoto", params, &out); err != nil {
		return 0, err
	}
	return out.Result.MessageID, nil
}

// SendMediaGroupInput sendMediaGroup 单条输入（photo 或 file_id 引用）
type SendMediaGroupInput struct {
	FileID  string
	Caption string
}

// SendMediaGroup 发送图组（2-10 张）
func (c *Client) SendMediaGroup(chatID int64, threadID int, items []SendMediaGroupInput) error {
	media := make([]map[string]any, 0, len(items))
	for i, it := range items {
		e := map[string]any{"type": "photo", "media": it.FileID}
		if i == 0 && it.Caption != "" {
			e["caption"] = it.Caption
			e["parse_mode"] = "HTML"
		}
		media = append(media, e)
	}
	params := map[string]any{"chat_id": chatID, "media": media}
	if threadID > 0 {
		params["message_thread_id"] = threadID
	}
	return c.call("sendMediaGroup", params, &struct{ OK bool }{})
}

// call 统一请求：POST JSON，解析 {"ok":bool,"description":string}
func (c *Client) call(method string, params any, result any) error {
	var body io.Reader
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	} else {
		body = bytes.NewReader([]byte("{}"))
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/bot%s/%s", apiBase, c.Token, method), body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var envelope struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("tg %s: 响应解析失败: %s", method, truncate(string(raw), 200))
	}
	if !envelope.OK {
		return fmt.Errorf("tg %s: %s", method, envelope.Description)
	}
	if result != nil {
		// 直接解析完整 envelope（{"ok":true,"result":...}）：调用方传入的都是
		// 带 OK / Result 字段的结构体。此前只把内层 result 反序列化进这些结构体，
		// 导致 GetMe 永远失败、SetWebhook 永远报解析错误、CreateForumTopic 永远返回 0。
		if err := json.Unmarshal(raw, result); err != nil {
			return fmt.Errorf("tg %s: result 解析失败: %w", method, err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
