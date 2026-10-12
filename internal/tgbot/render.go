package tgbot

import (
	"fmt"
	"html"
	"strings"
)

// TG 消息统一 HTML parse_mode，用户内容必须先转义防注入。
func Escape(s string) string { return html.EscapeString(s) }

func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func PriorityLabel(p string) string {
	switch p {
	case "high":
		return "🔴 高"
	case "low":
		return "⚪️ 低"
	default:
		return "🟡 普通"
	}
}

func StatusLabel(s string) string {
	switch s {
	case "open":
		return "🟢 待处理"
	case "pending":
		return "🟡 等待用户"
	case "closed":
		return "⚫️ 已关闭"
	}
	return s
}

// RenderAdminNewTicket 新工单首条消息
func RenderAdminNewTicket(id uint, subject, category, priority, username, content string, link string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>🎫 新工单 #%d</b>\n", id)
	fmt.Fprintf(&b, "<b>标题：</b>%s\n", Escape(subject))
	fmt.Fprintf(&b, "<b>用户：</b>%s\n", Escape(username))
	fmt.Fprintf(&b, "<b>优先级：</b>%s", PriorityLabel(priority))
	if category != "" {
		fmt.Fprintf(&b, "　<b>分类：</b>%s", Escape(category))
	}
	b.WriteString("\n")
	if content != "" {
		fmt.Fprintf(&b, "<blockquote>%s</blockquote>\n", Escape(Truncate(content, 1500)))
	}
	fmt.Fprintf(&b, "<i>直接在本话题内回复即可答复用户</i>")
	if link != "" {
		fmt.Fprintf(&b, "\n<a href=\"%s\">🌐 在网页中查看</a>", link)
	}
	return b.String()
}

// RenderAdminNewReply 用户在网页/TG 的新回复
func RenderAdminNewReply(id uint, subject, senderName, content, via string, link string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>💬 工单 #%d 新回复</b>（%s）\n", id, Escape(Truncate(subject, 60)))
	fmt.Fprintf(&b, "<b>来自：</b>%s", Escape(senderName))
	if via == "telegram" {
		b.WriteString(" <i>· via TG</i>")
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "<blockquote>%s</blockquote>", Escape(Truncate(content, 1500)))
	if link != "" {
		fmt.Fprintf(&b, "\n<a href=\"%s\">🌐 在网页中查看</a>", link)
	}
	return b.String()
}

// RenderUserNewReply 推送给用户的新回复
func RenderUserNewReply(id uint, subject, senderName, content string, link string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>💬 你的工单 #%d 有新回复</b>\n", id)
	fmt.Fprintf(&b, "<b>标题：</b>%s\n", Escape(Truncate(subject, 60)))
	fmt.Fprintf(&b, "<blockquote>%s</blockquote>", Escape(Truncate(content, 1200)))
	if link != "" {
		fmt.Fprintf(&b, "\n<a href=\"%s\">🌐 查看工单</a>", link)
	}
	return b.String()
}

// RenderUserStatus 状态变化通知
func RenderUserStatus(id uint, subject, status, operator string) string {
	var b strings.Builder
	switch status {
	case "closed":
		fmt.Fprintf(&b, "⚫️ 工单 #%d 已关闭（%s）", id, Escape(operator))
	case "open":
		fmt.Fprintf(&b, "🟢 工单 #%d 已重新开启（%s）", id, Escape(operator))
	default:
		fmt.Fprintf(&b, "ℹ️ 工单 #%d 状态更新为 %s（%s）", id, StatusLabel(status), Escape(operator))
	}
	fmt.Fprintf(&b, "\n<i>标题：%s</i>", Escape(Truncate(subject, 60)))
	return b.String()
}
