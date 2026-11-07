package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config 全部来自环境变量，基础设施类配置不走数据库。
type Config struct {
	ListenAddr string
	DBPath     string

	// 对外基础 URL，用于 TG webhook 注册与消息里的跳转链接，如 https://ticket.example.com
	PublicBaseURL string
	// cookie SameSite：same-site 嵌入（同主域子域）用 lax；跨站 iframe 必须 none
	CookieSameSite string

	Sub2APIBaseURL string
	// 用于校验透传 JWT 的用户级接口路径（不同版本可能不同，可覆盖）
	Sub2APIVerifyPath string
	// TokenRouter MCP 专用访问令牌，需具备 users.get 权限，用于查询用户资料
	Sub2APIMCPToken string
	// MCP Streamable HTTP 端点路径（TokenRouter 默认 /mcp），也可填写完整 URL
	Sub2APIMCPPath string

	// 本地兜底管理员（首次启动时若无该账号则创建）
	AdminEmail    string
	AdminPassword string

	// TG 接收方式：true 时用 long polling（本地调试），默认 webhook
	TGPolling bool

	// 访问 api.telegram.org 的代理（http:// 或 socks5://），部署环境被墙时必配
	TGProxy string
}

func Load() (*Config, error) {
	c := &Config{
		ListenAddr:        env("LISTEN_ADDR", ":8080"),
		DBPath:            env("DB_PATH", "data/ticket.db"),
		PublicBaseURL:     strings.TrimRight(env("PUBLIC_BASE_URL", ""), "/"),
		CookieSameSite:    strings.ToLower(env("COOKIE_SAMESITE", "lax")),
		Sub2APIBaseURL:    strings.TrimRight(env("SUB2API_BASE_URL", ""), "/"),
		Sub2APIVerifyPath: env("SUB2API_VERIFY_PATH", "/api/v1/auth/me"),
		Sub2APIMCPToken:   envAny("", "SUB2API_MCP_TOKEN", "TOKENROUTER_MCP_TOKEN"),
		Sub2APIMCPPath:    envAny("/mcp", "SUB2API_MCP_PATH", "TOKENROUTER_MCP_PATH"),
		AdminEmail:        env("ADMIN_EMAIL", ""),
		AdminPassword:     env("ADMIN_PASSWORD", ""),
		TGPolling:         env("TG_POLLING", "false") == "true",
		TGProxy:           env("TG_PROXY", ""),
	}
	if c.PublicBaseURL != "" {
		if _, err := url.Parse(c.PublicBaseURL); err != nil {
			return nil, fmt.Errorf("PUBLIC_BASE_URL 无效: %w", err)
		}
	}
	if c.CookieSameSite != "lax" && c.CookieSameSite != "none" && c.CookieSameSite != "strict" {
		return nil, fmt.Errorf("COOKIE_SAMESITE 只支持 lax/none/strict，当前 %q", c.CookieSameSite)
	}
	return c, nil
}

// PublicHTTPSPort 供 webhook URL 判断是否 https
func (c *Config) SecureCookie() bool {
	return strings.HasPrefix(c.PublicBaseURL, "https://")
}

func env(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func envAny(def string, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
