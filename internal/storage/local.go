package storage

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Local 本地磁盘实现：对象写入 root 目录，通过 /uploads/<key>?e=<expire>&s=<sig> 签名 URL 访问。
// 上传目录由 http.FileServer 纯静态提供，签名防跨用户枚举。
type Local struct {
	Root string // 如 data/uploads
	// URLBase 对外前缀，如 https://ticket.example.com/uploads；留空则相对路径 /uploads/<key>
	URLBase string
	// SignKey HMAC 密钥；为空时生成随机（进程重启后旧签名失效，仅测试用）
	SignKey string
	// URLTTL 签名有效期
	URLTTL time.Duration
}

func (l *Local) Name() string { return "local" }

func (l *Local) signKey() string {
	if l.SignKey != "" {
		return l.SignKey
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (l *Local) ttl() time.Duration {
	if l.URLTTL > 0 {
		return l.URLTTL
	}
	return 7 * 24 * time.Hour
}

// SignedURL 生成带过期与 HMAC 签名的访问 URL
func (l *Local) SignedURL(key string, now time.Time) string {
	exp := now.Add(l.ttl()).Unix()
	msg := key + "|" + fmt.Sprint(exp)
	mac := hmac.New(sha256.New, []byte(l.signKey()))
	mac.Write([]byte(msg))
	sig := hex.EncodeToString(mac.Sum(nil))[:32]
	base := strings.TrimSuffix(l.URLBase, "/")
	if base == "" {
		base = "/uploads"
	}
	return fmt.Sprintf("%s/%s?e=%d&s=%s", base, key, exp, sig)
}

// Verify 校验签名 URL 参数，key 必须与服务端取出的对象 key 一致
func (l *Local) Verify(key string, expire int64, sig string, now time.Time) error {
	if expire < now.Unix() {
		return errors.New("链接已过期")
	}
	msg := key + "|" + fmt.Sprint(expire)
	mac := hmac.New(sha256.New, []byte(l.signKey()))
	mac.Write([]byte(msg))
	want := hex.EncodeToString(mac.Sum(nil))[:32]
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return errors.New("签名无效")
	}
	return nil
}

func (l *Local) path(key string) (string, error) {
	p := filepath.Join(l.Root, filepath.FromSlash(key))
	// 防穿越：解析后必须仍在 Root 内
	if !strings.HasPrefix(p, filepath.Clean(l.Root)+string(os.PathSeparator)) {
		return "", errors.New("非法对象 key")
	}
	return p, nil
}

func (l *Local) Save(_ context.Context, o Object) (string, error) {
	p, err := l.path(o.Key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, o.Body); err != nil {
		os.Remove(p)
		return "", err
	}
	return l.SignedURL(o.Key, time.Now()), nil
}

func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (l *Local) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return nil
}
