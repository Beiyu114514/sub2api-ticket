package tgbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"sub2api-ticket/internal/storage"
)

// SaveImageDeps 注入到 Core 的上传依赖（tgbot 不能反向依赖 api 包）。
type SaveImageDeps struct {
	// SaveImage 保存图片：u 为 *model.User（用 any 避免引入 model 之外的反向依赖）
	SaveImage func(u any, data []byte, origName string) (*storage.Attachment, error)
	// OpenByKey 读对象内容（管理员群发图转发等场景），返回 reader + mime
	OpenByKey func(ctx context.Context, key string) (io.ReadCloser, string, error)
	// Rebind 消息落库后把附件 Upload 记录挂到工单（审计）
	Rebind func(userID, ticketID uint, atts []storage.Attachment)
}

// mediaGroupBuffer 聚合相册消息：TG 把一次发的多张图拆成多条同 media_group_id 的
// update，短窗口聚合后作为一条消息的附件处理。
type mediaGroupBuffer struct {
	mu    sync.Mutex
	items map[string][]*Message // media_group_id -> messages
	timer map[string]*time.Timer
}

// flush 返回该组聚合的全部消息（按 message_id 排序）
func (b *mediaGroupBuffer) take(group string) []*Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	msgs := b.items[group]
	delete(b.items, group)
	if t := b.timer[group]; t != nil {
		t.Stop()
		delete(b.timer, group)
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].MessageID < msgs[j].MessageID })
	return msgs
}

// DownloadFile 下载 TG 文件：getFile 拿 file_path 后从 file/bot<token>/<path> 拉字节。
// 走 Client 自身的 HTTP 客户端（含代理）。Bot API 限制下载 ≤20MB。
func (c *Client) DownloadFile(fileID string) ([]byte, error) {
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := c.call("getFile", map[string]any{"file_id": fileID}, &out); err != nil {
		return nil, err
	}
	if !out.OK || out.Result.FilePath == "" {
		return nil, fmt.Errorf("tg getFile: 无 file_path")
	}
	resp, err := c.HTTP.Get(fmt.Sprintf("%s/file/bot%s/%s", apiBase, c.Token, out.Result.FilePath))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("tg download: HTTP %d", resp.StatusCode)
	}
	// TG 文件上限 20MB，服务端另有 10MB 处理上限
	buf, err := io.ReadAll(io.LimitReader(resp.Body, 21<<20))
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// extByMime multipart 文件扩展名
func extByMime(mime string) string {
	switch mime {
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	default:
		return "jpg"
	}
}

// UploadPhoto multipart 上传单张图片字节
func (c *Client) UploadPhoto(chatID int64, threadID int, data []byte, mime, caption string) error {
	fname := "photo." + extByMime(mime)
	params := map[string]any{}
	if threadID > 0 {
		params["message_thread_id"] = strconv.Itoa(threadID)
	}
	if caption != "" {
		params["caption"] = caption
	}
	return c.uploadMultipart("sendPhoto", chatID, "photo", fname, data, params)
}

// MediaInput multipart 图组的单张输入
type MediaInput struct {
	Data    []byte
	Mime    string
	Caption string
}

// UploadMediaGroup multipart 上传图组（2-10 张）
func (c *Client) UploadMediaGroup(chatID int64, threadID int, items []MediaInput) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	media := make([]string, 0, len(items))
	for i, it := range items {
		fname := fmt.Sprintf("photo%d.%s", i, extByMime(it.Mime))
		fw, err := w.CreateFormFile("file" + strconv.Itoa(i), fname)
		if err != nil {
			return err
		}
		if _, err := fw.Write(it.Data); err != nil {
			return err
		}
		e := map[string]any{"type": "photo", "media": "attach://file" + strconv.Itoa(i)}
		if it.Caption != "" {
			e["caption"] = it.Caption
		}
		b, _ := json.Marshal(e)
		media = append(media, string(b))
	}
	if threadID > 0 {
		_ = w.WriteField("message_thread_id", strconv.Itoa(threadID))
	}
	_ = w.WriteField("media", "["+strings.Join(media, ",")+"]")
	if err := w.Close(); err != nil {
		return err
	}
	return c.postMultipart("sendMediaGroup", chatID, &buf, w.FormDataContentType())
}

// uploadMultipart 单文件 multipart 请求
func (c *Client) uploadMultipart(method string, chatID int64, field, fname string, data []byte, extra map[string]any) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, fname)
	if err != nil {
		return err
	}
	if _, err := fw.Write(data); err != nil {
		return err
	}
	for k, v := range extra {
		_ = w.WriteField(k, fmt.Sprint(v))
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.postMultipart(method, chatID, &buf, w.FormDataContentType())
}

// postMultipart 发送已构造好的 multipart body
func (c *Client) postMultipart(method string, chatID int64, body io.Reader, contentType string) error {
	var b bytes.Buffer
	if _, err := b.ReadFrom(body); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/bot%s/%s", apiBase, c.Token, method), &b)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
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
	return nil
}
