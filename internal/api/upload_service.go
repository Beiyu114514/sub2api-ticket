package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/storage"
)

// UploadService 上传核心：校验、压缩、落存储、写审计、限速
type UploadService struct {
	Deps
}

// S3Config 后台配置的 S3 参数（存 Setting，JSON 序列化）
type S3Config struct {
	Endpoint   string `json:"endpoint"`
	Region     string `json:"region"`
	Bucket     string `json:"bucket"`
	AccessKey  string `json:"access_key"`
	SecretKey  string `json:"secret_key"`
	PathStyle  bool   `json:"use_path_style"`
	PublicBase string `json:"public_base_url"`
}

// backend 按设置构造当前存储后端
func (s *UploadService) backend() storage.Backend {
	if s.Settings.StorageMode() == "s3" {
		if b := s.s3Backend(); b != nil {
			return b
		}
		// S3 配置不完整时降级 local，避免上传全挂
	}
	return s.localBackend()
}

// s3Backend 按设置构造 S3 后端；配置不完整返回 nil
func (s *UploadService) s3Backend() *storage.S3 {
	var cfg S3Config
	if err := json.Unmarshal([]byte(s.Settings.StorageS3JSON()), &cfg); err != nil || cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil
	}
	b := &storage.S3{
		Endpoint:   cfg.Endpoint,
		Region:     cfg.Region,
		Bucket:     cfg.Bucket,
		AccessKey:  cfg.AccessKey,
		SecretKey:  cfg.SecretKey,
		PathStyle:  cfg.PathStyle,
		PublicBase: cfg.PublicBase,
	}
	// 预签名有效期与本地模式共用同一 TTL 设置（SetPresignTTL 内部按 S3 7 天上限截断）
	b.SetPresignTTL(time.Duration(s.Settings.StorageURLTTLHours()) * time.Hour)
	return b
}

// localBackend 按设置构造本地磁盘后端
func (s *UploadService) localBackend() *storage.Local {
	return &storage.Local{
		Root:    s.Settings.StorageLocalDir(),
		SignKey: s.Settings.StorageSignKey(),
		URLTTL:  time.Duration(s.Settings.StorageURLTTLHours()) * time.Hour,
	}
}

// ResignAttachments 刷新附件访问 URL：上传时落库的签名 URL 会过期，
// 按 Upload 记录的实际存储后端用当前时间重签。key 解析或记录缺失时保留原 URL。
func (s *UploadService) ResignAttachments(list []storage.Attachment) []storage.Attachment {
	if list == nil {
		return nil
	}
	out := make([]storage.Attachment, len(list))
	for i, a := range list {
		out[i] = a
		key, err := keyFromURL(a.URL)
		if err != nil {
			continue
		}
		var rec model.Upload
		if err := s.G.Where("key = ?", key).First(&rec).Error; err != nil {
			continue
		}
		if rec.Storage == "s3" {
			if b := s.s3Backend(); b != nil {
				if b.PublicBase != "" {
					out[i].URL = strings.TrimSuffix(b.PublicBase, "/") + "/" + key
				} else {
					out[i].URL = b.Presign(key)
				}
			}
			continue
		}
		out[i].URL = s.localBackend().SignedURL(key, time.Now())
	}
	return out
}

// 简单内存限速：每用户每分钟最多 10 次
var uploadRate = struct {
	sync.Mutex
	m map[uint][]time.Time
}{m: map[uint][]time.Time{}}

func allowUpload(userID uint) bool {
	uploadRate.Lock()
	defer uploadRate.Unlock()
	now := time.Now()
	list := uploadRate.m[userID]
	keep := list[:0]
	for _, t := range list {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= 10 {
		uploadRate.m[userID] = keep
		return false
	}
	uploadRate.m[userID] = append(keep, now)
	return true
}

// extByMime 存储扩展名（仅用于展示，安全不依赖它）
func extByMime(mime string) string {
	switch mime {
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	}
	return "bin"
}

// SaveImage 处理一次上传：原始名仅用于展示，key 服务端随机生成
func (s *UploadService) SaveImage(ctx context.Context, u *model.User, data []byte, origName string) (*storage.Attachment, error) {
	if !allowUpload(u.ID) {
		return nil, fmt.Errorf("上传过于频繁，请稍后再试")
	}
	out, mime, w, h, err := storage.ProcessImage(data)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%s/%s.%s", storage.KeyPrefix(time.Now()), randomID(), extByMime(mime))
	sum := sha256.Sum256(out)
	b := s.backend()
	url, err := b.Save(ctx, storage.Object{
		Key:         key,
		Mime:        mime,
		Size:        int64(len(out)),
		Body:        strings.NewReader(string(out)),
		PayloadHash: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		return nil, fmt.Errorf("存储失败: %w", err)
	}
	rec := &model.Upload{
		UserID:  u.ID,
		Storage: b.Name(),
		Key:     key,
		Mime:    mime,
		Size:    int64(len(out)),
	}
	if err := s.G.Create(rec).Error; err != nil {
		_ = b.Delete(ctx, key) // 审计写不进就回滚对象，避免孤儿
		return nil, err
	}
	return &storage.Attachment{
		URL: url, Mime: mime, Size: int64(len(out)),
		Width: w, Height: h, Name: sanitizeName(origName),
	}, nil
}

// SaveImageAny tgbot 钩子入口（u 必须是 *model.User；用 any 解耦包依赖）
func (s *UploadService) SaveImageAny(u any, data []byte, origName string) (*storage.Attachment, error) {
	mu, ok := u.(*model.User)
	if !ok || mu == nil {
		return nil, fmt.Errorf("无效用户")
	}
	return s.SaveImage(context.Background(), mu, data, origName)
}

// OpenImageByKey 按 key 读取对象（TG 管理员群转发附件场景）
func (s *UploadService) OpenImageByKey(ctx context.Context, key string) (io.ReadCloser, string, error) {
	var rec model.Upload
	if err := s.G.Where("key = ?", key).First(&rec).Error; err != nil {
		return nil, "", fmt.Errorf("对象不存在")
	}
	rc, err := s.backend().Open(ctx, key)
	return rc, rec.Mime, err
}

// ResolveAttachments 校验 attachments 引用归属（key 必须是上传者本人 Upload 记录），
// 返回规范化后的附件列表。max 默认 5。
func (s *UploadService) ResolveAttachments(u *model.User, urls []string, max int) ([]storage.Attachment, error) {
	if max <= 0 {
		max = 5
	}
	if len(urls) > max {
		return nil, fmt.Errorf("附件数量超限（最多 %d 张）", max)
	}
	if len(urls) == 0 {
		return nil, nil
	}
	var out []storage.Attachment
	for _, raw := range urls {
		key, err := keyFromURL(raw)
		if err != nil {
			return nil, fmt.Errorf("附件 URL 无效")
		}
		var rec model.Upload
		if err := s.G.Where("key = ? AND user_id = ?", key, u.ID).First(&rec).Error; err != nil {
			return nil, fmt.Errorf("附件不存在或无权引用")
		}
		out = append(out, storage.Attachment{
			URL: raw, Mime: rec.Mime, Size: rec.Size,
			Width: 0, Height: 0, // 尺寸在详情里重新签名 URL 时不再需要
		})
	}
	return out, nil
}

// keyFromURL 从返回给前端的 URL 中提取对象 key（path 部分去掉前缀与 query）
func keyFromURL(raw string) (string, error) {
	i := strings.Index(raw, "?")
	if i >= 0 {
		raw = raw[:i]
	}
	// 取路径段：/uploads/<key> 或 https://.../bucket/<key> —— 统一取 /uploads/ 之后的全部
	if j := strings.Index(raw, "/uploads/"); j >= 0 {
		return strings.TrimPrefix(raw[j+len("/uploads/"):], "/"), nil
	}
	// 兜底：最后两段（yyyy/mm 不够，key 是 yyyy/mm/id.ext）
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	if len(parts) >= 3 {
		return strings.Join(parts[len(parts)-3:], "/"), nil
	}
	return "", fmt.Errorf("无法解析 key")
}

func sanitizeName(n string) string {
	n = strings.TrimSpace(n)
	if len(n) > 80 {
		n = n[:80]
	}
	return n
}

func randomID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}

// RebindTicketToUploads 消息落库后把附件 Upload 记录挂到工单（审计用）
func (s *UploadService) RebindTicketToUploads(userID, ticketID uint, atts []storage.Attachment) {
	if len(atts) == 0 {
		return
	}
	keys := make([]string, 0, len(atts))
	for _, a := range atts {
		if k, err := keyFromURL(a.URL); err == nil {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return
	}
	q := s.G.Model(&model.Upload{}).
		Where("ticket_id = 0 AND key IN ?", keys)
	if userID != 0 {
		// TG 管理员群上传等无归属用户场景传 0：跳过归属过滤
		q = q.Where("user_id = ?", userID)
	}
	q.Update("ticket_id", ticketID)
}
