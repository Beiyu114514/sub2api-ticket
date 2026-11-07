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

// Client 调用 TokenRouter / sub2api 的两类接口：
//  1. 用户级接口（Bearer 透传 JWT）验证 token 有效性
//  2. MCP 管理接口（Bearer MCP Token）查询用户资料
type Client struct {
	BaseURL    string
	VerifyPath string
	MCPPath    string
	MCPToken   string
	HTTP       *http.Client
}

func New(baseURL, verifyPath, mcpPath, mcpToken string) *Client {
	return &Client{
		BaseURL:    baseURL,
		VerifyPath: verifyPath,
		MCPPath:    mcpPath,
		MCPToken:   mcpToken,
		HTTP:       &http.Client{Timeout: 10 * time.Second},
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
	// 身份必须由上游响应确定，绝不能回退到客户端传入的 expectUserID：
	// 一旦上游响应解析不出 ID（字段改名、返回 200 的 HTML 登录页等），
	// 任何持有有效 token 的用户都能伪造成任意 sub2api_id（含管理员）。
	if info.Sub2APIID == 0 {
		return nil, fmt.Errorf("无法从 sub2api 响应确定用户 ID，拒绝登录")
	}
	if expectUserID != 0 && info.Sub2APIID != expectUserID {
		return nil, fmt.Errorf("token 与 user_id 不匹配")
	}
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

// parseUserJSON 宽松解析：兼容顶层或多层 data 包裹、字段名差异（id/user_id、role/role_name）。
func parseUserJSON(body []byte) UserInfo {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return UserInfo{}
	}
	return parseUserValue(raw)
}

func parseUserValue(raw any) UserInfo {
	m, ok := raw.(map[string]any)
	if !ok {
		return UserInfo{}
	}
	// MCP 结果和普通 HTTP 响应都可能再包一层 data；优先采用真正含用户字段的
	// 内层对象，避免把外层 request_id 之类字段误当作用户 ID。
	if d, ok := m["data"]; ok {
		if nested := parseUserValue(d); !nested.empty() {
			return nested
		}
	}
	return userInfoFromMap(m)
}

func (u UserInfo) empty() bool {
	return u.Sub2APIID == 0 && u.Username == "" && u.Email == "" && u.Role == ""
}

func userInfoFromMap(raw map[string]any) UserInfo {
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
			case json.Number:
				return t.String()
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
			case json.Number:
				if n, err := t.Int64(); err == nil {
					return n
				}
			case int64:
				return t
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
