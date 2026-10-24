package model

import "time"

// User sub2api 用户镜像。role 与 sub2api 保持一致：admin / user
type User struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	// 列名显式固定为 sub2api_id：GORM 对 Sub2APIID 的默认蛇形命名
	// 随版本变化（曾生成 sub_api_id / sub2_api_id），不锁定会导致跨版本升级后
	// AutoMigrate 建出与代码查询不一致的列，SSO 登录 500（见 issue #2）。
	Sub2APIID int64  `gorm:"column:sub2api_id;uniqueIndex" json:"sub2api_id"`
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
	SenderRole string    `json:"sender_role"` // user / admin / system / assistant
	SenderName string    `json:"sender_name"`
	Content    string    `json:"content"`
	// Attachments 附件 JSON 数组，元素结构见 storage.Attachment；空串=无附件
	Attachments string    `json:"attachments"`
	Via        string    `json:"via"` // web / telegram / assistant
	CreatedAt  time.Time `json:"created_at"`
}

// Upload 上传审计记录：谁传的、存在哪种存储、对象 key，供归属校验与孤儿清理
type Upload struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	TicketID  uint      `gorm:"index;default:0" json:"ticket_id"` // 0 = 尚未挂到消息
	Storage   string    `json:"storage"`                         // local / s3
	Key       string    `gorm:"uniqueIndex" json:"key"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
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
