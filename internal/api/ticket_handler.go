package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/store"
)

type TicketHandler struct{ Deps }

// List 工单列表：普通用户只看自己的；管理员看全部
func (h *TicketHandler) List(c *gin.Context) {
	u := c.MustGet("user").(*model.User)
	status := c.Query("status")
	q := strings.TrimSpace(c.Query("q"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	db := h.G.Model(&model.Ticket{})
	if !u.IsAdmin() {
		db = db.Where("user_id = ?", u.ID)
	}
	if store.ValidStatus(status) {
		db = db.Where("status = ?", status)
	}
	if q != "" {
		like := "%" + q + "%"
		db = db.Where("subject LIKE ?", like)
	}

	var total int64
	db.Count(&total)

	var tickets []model.Ticket
	if err := db.Preload("User").
		Order("last_reply_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&tickets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	// 管理员视图才暴露完整用户信息；普通用户隐藏关联用户
	if !u.IsAdmin() {
		for i := range tickets {
			tickets[i].User = nil
			tickets[i].UnreadAdminCount = 0
		}
	}
	// 统计各状态数量（管理员/用户各自范围）
	statusCount := map[string]int64{}
	var rows []struct {
		Status string
		Cnt    int64
	}
	cntDB := h.G.Model(&model.Ticket{}).Select("status, COUNT(*) as cnt")
	if !u.IsAdmin() {
		cntDB = cntDB.Where("user_id = ?", u.ID)
	}
	cntDB.Group("status").Scan(&rows)
	for _, r := range rows {
		statusCount[r.Status] = r.Cnt
	}

	c.JSON(http.StatusOK, gin.H{
		"items": tickets, "total": total, "page": page, "page_size": pageSize,
		"status_count": statusCount,
	})
}

// Create 新建工单
func (h *TicketHandler) Create(c *gin.Context) {
	u := c.MustGet("user").(*model.User)
	var req struct {
		Subject  string `json:"subject" binding:"required"`
		Category string `json:"category"`
		Priority string `json:"priority"`
		Content  string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题和内容必填"})
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.Content = strings.TrimSpace(req.Content)
	if req.Subject == "" || req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题和内容不能为空"})
		return
	}
	if len(req.Subject) > 200 {
		req.Subject = req.Subject[:200]
	}
	if len(req.Content) > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "内容过长（上限 10000 字符）"})
		return
	}
	if !store.ValidPriority(req.Priority) {
		req.Priority = "normal"
	}
	// 简单限流：每用户 60 秒内最多 3 单
	var recent int64
	h.G.Model(&model.Ticket{}).
		Where("user_id = ? AND created_at > ?", u.ID, time.Now().Add(-time.Minute)).
		Count(&recent)
	if recent >= 3 {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "创建过于频繁，请稍后再试"})
		return
	}

	t, msg, err := h.Tickets.CreateTicket(u, req.Subject, req.Category, req.Priority, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败"})
		return
	}
	msg.Content = "" // 首条消息在详情里看
	c.JSON(http.StatusOK, gin.H{"ticket": t, "first_message": msg})
}

// Get 工单详情 + 消息流；同时清对应方向的未读
func (h *TicketHandler) Get(c *gin.Context) {
	u := c.MustGet("user").(*model.User)
	t, err := h.loadTicket(c, u)
	if err != nil {
		return // loadTicket 已写响应
	}

	var messages []model.TicketMessage
	h.G.Where("ticket_id = ?", t.ID).Order("id ASC").Limit(500).Find(&messages)

	updates := map[string]any{}
	if u.IsAdmin() {
		if t.UnreadAdminCount > 0 {
			updates["unread_admin_count"] = 0
		}
	} else {
		if t.UnreadUserCount > 0 {
			updates["unread_user_count"] = 0
		}
	}
	if len(updates) > 0 {
		h.G.Model(t).Updates(updates)
		if u.IsAdmin() {
			t.UnreadAdminCount = 0
		} else {
			t.UnreadUserCount = 0
		}
	}
	if !u.IsAdmin() {
		t.User = nil
	}
	c.JSON(http.StatusOK, gin.H{"ticket": t, "messages": messages})
}

// Reply 追加回复
func (h *TicketHandler) Reply(c *gin.Context) {
	u := c.MustGet("user").(*model.User)
	t, err := h.loadTicket(c, u)
	if err != nil {
		return
	}
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "内容不能为空"})
		return
	}
	if len(req.Content) > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "内容过长（上限 10000 字符）"})
		return
	}
	role := "user"
	if u.IsAdmin() {
		role = "admin"
	}
	_, msg, err := h.Tickets.AppendMessage(t.ID, u, role, u.Username, strings.TrimSpace(req.Content), "web", false)
	if err == store.ErrTicketClosed {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "发送失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": msg})
}

// ChangeStatus 状态流转
func (h *TicketHandler) ChangeStatus(c *gin.Context) {
	u := c.MustGet("user").(*model.User)
	t, err := h.loadTicket(c, u)
	if err != nil {
		return
	}
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !store.ValidStatus(req.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效状态"})
		return
	}
	if !u.IsAdmin() && req.Status == "pending" {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权设置该状态"})
		return
	}
	role := "user"
	name := u.Username
	if u.IsAdmin() {
		role = "admin"
	}
	_, err = h.Tickets.ChangeStatus(t, role, name, req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "操作失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ticket": t})
}

// loadTicket 按 :id 加载并校验权限
func (h *TicketHandler) loadTicket(c *gin.Context, u *model.User) (*model.Ticket, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效工单 ID"})
		return nil, err
	}
	var t model.Ticket
	if err := h.G.Preload("User").First(&t, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "工单不存在"})
		return nil, err
	}
	if !u.IsAdmin() && t.UserID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权访问该工单"})
		return nil, store.ErrNoPermission
	}
	return &t, nil
}
