package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// S3 S3 兼容对象存储实现（AWS S3 / MinIO / R2 / B2）。
// 凭据由后台设置注入。只用 V4 签名的 REST 请求，不引 SDK。
type S3 struct {
	Endpoint   string // 如 https://s3.amazonaws.com 或 https://minio.example.com
	Region     string
	Bucket     string
	AccessKey  string
	SecretKey  string
	PathStyle  bool   // MinIO/R2 通常 true
	PublicBase string // 可选：公桶/CDN 前缀，设置后直接拼 URL，不签名
	// presignTTL 预签名有效期（PublicBase 为空时使用）
	presignTTL time.Duration
}

func (s *S3) Name() string { return "s3" }

func (s *S3) ttl() time.Duration {
	if s.presignTTL > 0 {
		return s.presignTTL
	}
	return 7 * 24 * time.Hour
}

func (s *S3) objectURL(key string) string {
	if s.PathStyle {
		return strings.TrimSuffix(s.Endpoint, "/") + "/" + s.Bucket + "/" + key
	}
	// virtual-hosted style：bucket.endpoint
	ep, _ := url.Parse(s.Endpoint)
	return fmt.Sprintf("%s://%s.%s/%s", ep.Scheme, s.Bucket, ep.Host, key)
}

// hmacSHA256 小工具
func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// signedURL 生成 V4 预签名 GET URL（查询参数签名，免 SDK）
func (s *S3) signedURL(key string, now time.Time) string {
	host := strings.TrimSuffix(s.Endpoint, "/")
	if s.PathStyle {
		host = strings.TrimSuffix(s.Endpoint, "/") + "/" + s.Bucket
	} else {
		ep, _ := url.Parse(s.Endpoint)
		host = fmt.Sprintf("%s://%s.%s", ep.Scheme, s.Bucket, ep.Host)
	}
	expire := s.ttl()
	expStr := fmt.Sprint(int64(expire.Seconds()))
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")
	scope := dateStamp + "/" + s.Region + "/s3/aws4_request"

	q := url.Values{}
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", s.AccessKey+"/"+scope)
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", expStr)
	q.Set("X-Amz-SignedHeaders", "host")
	canonicalURI := "/" + key
	// 查询参数按 key 排序
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var cq []string
	for _, k := range keys {
		cq = append(cq, url.QueryEscape(k)+"="+url.QueryEscape(q.Get(k)))
	}
	canonicalQuery := strings.Join(cq, "&")
	canonicalRequest := strings.Join([]string{
		"GET",
		canonicalURI,
		canonicalQuery,
		"host:" + mustHost(host) + "\n",
		"",
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+s.SecretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(s.Region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	sig := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	return host + canonicalURI + "?" + canonicalQuery + "&X-Amz-Signature=" + sig
}

func mustHost(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	return strings.TrimSuffix(u, "/")
}

func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}

// Save 用 V4 签名 PUT 上传
func (s *S3) Save(ctx context.Context, o Object) (string, error) {
	key := o.Key
	// PUT 不走预签名（payload 参与签名），构造标准 V4 SignedHeaders=host;content-type
	now := time.Now()
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")
	scope := dateStamp + "/" + s.Region + "/s3/aws4_request"
	payloadHash := o.PayloadHash // 由调用方（upload 服务）提供；为空则要求 Body 可复读
	if payloadHash == "" {
		return "", fmt.Errorf("s3 上传需要 payload hash")
	}
	host := mustHost(s.Endpoint)
	if s.PathStyle {
		host = mustHost(s.Endpoint) + "/" + s.Bucket
	} else {
		host = s.Bucket + "." + host
	}

	canonicalHeaders := "content-type:" + o.Mime + "\nhost:" + host + "\nx-amz-content-sha256:" + payloadHash + "\nx-amz-date:" + amzDate + "\n"
	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{
		"PUT",
		"/" + key,
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	kDate := hmacSHA256([]byte("AWS4"+s.SecretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(s.Region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	sig := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	scheme := "https"
	if strings.HasPrefix(s.Endpoint, "http://") {
		scheme = "http"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		scheme+"://"+host+"/"+key, o.Body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", o.Mime)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.AccessKey, scope, signedHeaders, sig))
	if o.Size > 0 {
		req.ContentLength = o.Size
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("s3 上传失败: %s %s", resp.Status, string(body))
	}

	if s.PublicBase != "" {
		return strings.TrimSuffix(s.PublicBase, "/") + "/" + key, nil
	}
	return s.signedURL(key, now), nil
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	u := s.signedURL(key, time.Now())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, ErrNotFound
	}
	if resp.StatusCode/100 != 2 {
		resp.Body.Close()
		return nil, fmt.Errorf("s3 读取失败: %s", resp.Status)
	}
	return resp.Body, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	now := time.Now()
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")
	scope := dateStamp + "/" + s.Region + "/s3/aws4_request"
	empty := sha256Hex(nil)
	host := mustHost(s.Endpoint)
	if s.PathStyle {
		host = mustHost(s.Endpoint) + "/" + s.Bucket
	} else {
		host = s.Bucket + "." + host
	}
	canonicalHeaders := "host:" + host + "\nx-amz-content-sha256:" + empty + "\nx-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{"DELETE", "/" + key, "", canonicalHeaders, signedHeaders, empty}, "\n")
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", amzDate, scope, sha256Hex([]byte(canonicalRequest))}, "\n")
	kDate := hmacSHA256([]byte("AWS4"+s.SecretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(s.Region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	sig := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	scheme := "https"
	if strings.HasPrefix(s.Endpoint, "http://") {
		scheme = "http"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, scheme+"://"+host+"/"+key, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", empty)
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.AccessKey, scope, signedHeaders, sig))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode/100 == 2 {
		return nil
	}
	return fmt.Errorf("s3 删除失败: %s", resp.Status)
}
