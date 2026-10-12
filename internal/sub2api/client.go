package sub2api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Client 调用 sub2api 的两类接口：
//  1. 用户级接口（Bearer 透传 JWT）验证 token 有效性
//  2. Admin API（x-api-key）查询用户资料
type Client struct {
	BaseURL     string
	VerifyPath  string
	UserPath    string
	AdminAPIKey string
	HTTP        *http.Client
}

func New(baseURL, verifyPath, userPath, adminAPIKey string) *Client {
	return &Client{
		BaseURL:     baseURL,
		VerifyPath:  verifyPath,
		UserPath:    userPath,
		AdminAPIKey: adminAPIKey,
		HTTP:        &http.Client{Timeout: 10 * time.Second},
	}
}

// UserInfo 宽松解析的用户信息
type UserInfo struct {
	Sub2APIID int64
	Username  string
	Email     string
	Role      string
}

// VerifyToken 用透传的 JWT 调 sub2api 用户接口，200 视为有效并尽量解析出用户信息。
func (c *Client) VerifyToken(token string, expectUserID int64) (*UserInfo, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("未配置 SUB2API_BASE_URL")
	}
	body, status, err := c.request(http.MethodGet, c.VerifyPath, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	})
	if err != nil {
		return nil, fmt.Errorf("连接 sub2api 失败: %w", err)
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, fmt.Errorf("token 无效或已过期")
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("sub2api 校验接口返回 %d", status)
	}

	info := parseUserJSON(body)
	if expectUserID != 0 && info.Sub2APIID != 0 && info.Sub2APIID != expectUserID {
		return nil, fmt.Errorf("token 与 user_id 不匹配")
	}
	if info.Sub2APIID == 0 {
		info.Sub2APIID = expectUserID
	}
	if info.Sub2APIID == 0 {
		return nil, fmt.Errorf("无法确定用户 ID")
	}
	return &info, nil
}

// GetAdminUser 通过管理员 API Key 查询用户资料（best effort，失败不阻断）
func (c *Client) GetAdminUser(id int64) (*UserInfo, error) {
	if c.AdminAPIKey == "" || c.BaseURL == "" {
		return nil, fmt.Errorf("未配置管理员 API Key")
	}
	body, status, err := c.request(http.MethodGet, c.UserPath+"/"+strconv.FormatInt(id, 10), func(r *http.Request) {
		r.Header.Set("x-api-key", c.AdminAPIKey)
	})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("sub2api 用户接口返回 %d", status)
	}
	info := parseUserJSON(body)
	info.Sub2APIID = id
	return &info, nil
}

func (c *Client) request(method, path string, set func(*http.Request)) ([]byte, int, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if set != nil {
		set(req)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return body, resp.StatusCode, nil
}

// parseUserJSON 宽松解析：兼容顶层或 data 包裹、字段名差异（id/user_id、role/role_name）
func parseUserJSON(body []byte) UserInfo {
	var raw map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(body), &raw); err != nil {
		return UserInfo{}
	}
	if d, ok := raw["data"].(map[string]any); ok {
		raw = d
	}
	var info UserInfo
	info.Sub2APIID = pickInt64(raw, "id", "user_id", "uid")
	info.Username = pickString(raw, "username", "name", "nickname")
	info.Email = pickString(raw, "email", "mail")
	info.Role = pickString(raw, "role", "role_name", "user_role")
	if info.Role == "" {
		if n := pickInt64(raw, "role", "role_id"); n == 1 {
			// 部分实现用数字 role：1 视为普通用户，不猜测 admin
			info.Role = "user"
		}
	}
	return info
}

func pickString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				if t != "" {
					return t
				}
			case float64:
				return strconv.FormatFloat(t, 'f', -1, 64)
			}
		}
	}
	return ""
}

func pickInt64(m map[string]any, keys ...string) int64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case float64:
				return int64(t)
			case string:
				if n, err := strconv.ParseInt(t, 10, 64); err == nil {
					return n
				}
			}
		}
	}
	return 0
}

// NormalizeRole admin 判定：字符串含 admin / root / super
func (u *UserInfo) NormalizeRole() string {
	r := u.Role
	if r == "admin" || r == "root" || r == "super_admin" || r == "administrator" {
		return "admin"
	}
	return "user"
}
