package model

import "time"

// User sub2api 用户镜像。role 与 sub2api 保持一致：admin / user
type User struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	Sub2APIID int64  `gorm:"uniqueIndex" json:"sub2api_id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	TGChatID  int64  `gorm:"index;default:0" json:"tg_chat_id"` // 0 = 未绑定

	// 本地兜底管理员（非 SSO）时有密码
	PasswordHash string `json:"-"`

	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

func (u *User) IsAdmin() bool { return u.Role == "admin" }

// Ticket 工单
type Ticket struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	UserID   uint   `gorm:"index" json:"user_id"`
	User     *User  `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Subject  string `json:"subject"`
	Category string `json:"category"`
	Priority string `json:"priority"`            // low / normal / high
	Status   string `gorm:"index" json:"status"` // open / pending / closed

	UnreadAdminCount int `json:"unread_admin_count"`
	UnreadUserCount  int `json:"unread_user_count"`

	LastReplyAt time.Time  `json:"last_reply_at"`
	LastReplyBy string     `json:"last_reply_by"` // user / admin / system
	ClosedAt    *time.Time `json:"closed_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TicketMessage 工单消息流
type TicketMessage struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	TicketID   uint      `gorm:"index" json:"ticket_id"`
	SenderRole string    `json:"sender_role"` // user / admin / system
	SenderName string    `json:"sender_name"`
	Content    string    `json:"content"`
	Via        string    `json:"via"` // web / telegram
	CreatedAt  time.Time `json:"created_at"`
}

// TicketTopic 管理员群话题映射：每个工单一个 forum topic
type TicketTopic struct {
	TicketID  uint      `gorm:"primaryKey" json:"ticket_id"`
	ChatID    int64     `json:"chat_id"`
	ThreadID  int       `json:"thread_id"`
	CreatedAt time.Time `json:"created_at"`
}

// UserChatState 用户私聊回复时的当前工单上下文
type UserChatState struct {
	ChatID          int64     `gorm:"primaryKey" json:"chat_id"`
	CurrentTicketID uint      `json:"current_ticket_id"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Session SSO/登录后签发的会话
type Session struct {
	Token     string    `gorm:"primaryKey" json:"token"`
	UserID    uint      `gorm:"index" json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// Notification 通知 outbox：先落库，worker 异步发送并重试
type Notification struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Kind          string     `json:"kind"`                // admin_new_ticket / admin_new_reply / user_new_reply / user_status
	Payload       string     `json:"payload"`             // JSON
	Status        string     `gorm:"index" json:"status"` // pending / sent / failed
	Retries       int        `json:"retries"`
	NextAttemptAt time.Time  `gorm:"index" json:"next_attempt_at"`
	LastError     string     `json:"last_error"`
	CreatedAt     time.Time  `json:"created_at"`
	SentAt        *time.Time `json:"sent_at"`
}

// BindCode 用户绑定 TG 的一次性码
type BindCode struct {
	Code      string    `gorm:"primaryKey" json:"code"`
	UserID    uint      `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Setting 运行时配置（网页后台可改）
type Setting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `json:"value"`
}

// 通知 kind 常量
const (
	NotifyAdminNewTicket = "admin_new_ticket"
	NotifyAdminNewReply  = "admin_new_reply"
	NotifyUserNewReply   = "user_new_reply"
	NotifyUserStatus     = "user_status"
)
