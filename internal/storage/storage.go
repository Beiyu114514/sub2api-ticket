// Package storage 提供附件存储抽象：本地磁盘与 S3 兼容对象存储两种实现，
// 由后台设置 storage.mode 决定，运行时可切换（只影响新上传）。
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// Attachment 消息附件的对外表示，序列化后存入 TicketMessage.Attachments
type Attachment struct {
	URL    string `json:"url"`
	Mime   string `json:"mime"`
	Size   int64  `json:"size"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Name   string `json:"name,omitempty"`
}

// DecodeAttachments 解析消息附件 JSON；空串返回 nil
func DecodeAttachments(s string) []Attachment {
	if s == "" {
		return nil
	}
	var out []Attachment
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// EncodeAttachments 序列化附件；空列表返回空串
func EncodeAttachments(list []Attachment) string {
	if len(list) == 0 {
		return ""
	}
	b, err := json.Marshal(list)
	if err != nil {
		return ""
	}
	return string(b)
}

var ErrNotFound = errors.New("对象不存在")

// Object 一次保存请求
type Object struct {
	Key         string // 存储内唯一 key，如 2026/10/uuid.webp
	Mime        string
	Size        int64
	Body        io.Reader
	PayloadHash string // 可选：SHA256 hex；S3 V4 签名 PUT 需要
}

// Backend 存储后端
type Backend interface {
	// Save 保存对象并返回对外访问 URL
	Save(ctx context.Context, o Object) (url string, err error)
	// Open 读取对象内容（TG 转发等场景需要原始字节）
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete 删除对象，不存在时返回 ErrNotFound
	Delete(ctx context.Context, key string) error
	// Name 实现名：local / s3
	Name() string
}

// KeyPrefix 按 y/m 分目录，避免单目录文件过多
func KeyPrefix(now time.Time) string {
	return fmt.Sprintf("%04d/%02d", now.Year(), int(now.Month()))
}
