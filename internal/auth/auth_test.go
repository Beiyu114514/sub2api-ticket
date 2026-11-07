package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/glebarez/sqlite"

	"sub2api-ticket/internal/config"
	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/sub2api"
)

func newAuthTestService(t *testing.T, upstreamURL string) (*Service, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	// 每次测试独立临时库：count>1 重跑时避免唯一索引冲突
	g, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := g.AutoMigrate(&model.User{}, &model.Session{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := &config.Config{CookieSameSite: "lax", Sub2APIMCPToken: "mcp-token"}
	c := sub2api.New(upstreamURL, "/api/v1/auth/me", "/mcp", "mcp-token")
	return NewService(g, cfg, c), g
}

// newMCPUserServer 模拟 TokenRouter MCP：完成 initialize 后，users.get 返回
// lookup 指定的 HTTP 状态和用户 JSON。hits 只统计真正的 tools/call。
func newMCPUserServer(t *testing.T, lookup func(id string) (int, string)) (*httptest.Server, *int64) {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Name      string `json:"name"`
				Arguments struct {
					OperationID string            `json:"operation_id"`
					Path        map[string]string `json:"path"`
				} `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		switch req.Method {
		case "initialize":
			writeMCPResult(t, w, req.ID, map[string]any{
				"protocolVersion": "2025-11-25",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]string{"name": "TokenRouter", "version": "test"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			atomic.AddInt64(&hits, 1)
			if req.Params.Name != "read_operation" || req.Params.Arguments.OperationID != "users.get" {
				http.Error(w, "unexpected MCP operation", http.StatusBadRequest)
				return
			}
			status, userJSON := lookup(req.Params.Arguments.Path["id"])
			if status >= http.StatusMultipleChoices {
				writeMCPResult(t, w, req.ID, map[string]any{
					"content": []map[string]string{{"type": "text", "text": "user lookup failed"}},
					"structuredContent": map[string]any{
						"status":      "failed",
						"http_status": status,
						"data":        map[string]string{"message": "user lookup failed"},
					},
					"isError": true,
				})
				return
			}
			writeMCPResult(t, w, req.ID, map[string]any{
				"content": []map[string]string{{"type": "text", "text": "ok"}},
				"structuredContent": map[string]any{
					"status":      "succeeded",
					"http_status": http.StatusOK,
					"data": map[string]any{
						"code": 0,
						"data": json.RawMessage(userJSON),
					},
				},
				"isError": false,
			})
		default:
			http.Error(w, "unknown method", http.StatusBadRequest)
		}
	}))
	return srv, &hits
}

func writeMCPResult(t *testing.T, w http.ResponseWriter, id json.RawMessage, result any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	if err := json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}); err != nil {
		t.Errorf("write MCP response: %v", err)
	}
}

// runRequireAdmin 用给定 user 跑一遍 RequireAdmin 中间件，返回是否放行。
func runRequireAdmin(t *testing.T, s *Service, u *model.User) (bool, int) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	c.Set("user", u)
	s.RequireAdmin()(c)
	return !c.IsAborted(), w.Code
}

// TestRequireAdminRevalidatesUpstream 回归 ISSUE-04：源站角色变更/账号删除必须在
// 一个复验周期内阻断本地会话，而不是继续信任 7 天的本地 session。
func TestRequireAdminRevalidatesUpstream(t *testing.T) {
	t.Run("upstream downgraded -> 403 + local role synced", func(t *testing.T) {
		srv, hits := newMCPUserServer(t, func(string) (int, string) {
			return http.StatusOK, `{"id":42,"role":"user"}`
		})
		defer srv.Close()
		s, g := newAuthTestService(t, srv.URL)

		u := &model.User{Sub2APIID: 42, Username: "u", Role: "admin", LastSeenAt: time.Now().Add(-30 * time.Minute)}
		if err := g.Create(u).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
		ok, code := runRequireAdmin(t, s, u)
		if ok || code != http.StatusForbidden {
			t.Fatalf("ok=%v code=%d, want 403", ok, code)
		}
		if atomic.LoadInt64(hits) != 1 {
			t.Fatalf("upstream hits = %d, want 1", atomic.LoadInt64(hits))
		}
		var got model.User
		g.First(&got, u.ID)
		if got.Role != "user" {
			t.Fatalf("local role = %q, want user（应同步降级）", got.Role)
		}
	})

	t.Run("upstream 404 -> 403", func(t *testing.T) {
		srv, _ := newMCPUserServer(t, func(string) (int, string) {
			return http.StatusNotFound, ""
		})
		defer srv.Close()
		s, g := newAuthTestService(t, srv.URL)
		u := &model.User{Sub2APIID: 7, Username: "u", Role: "admin", LastSeenAt: time.Now().Add(-time.Hour)}
		if err := g.Create(u).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
		ok, code := runRequireAdmin(t, s, u)
		if ok || code != http.StatusForbidden {
			t.Fatalf("ok=%v code=%d, want 403", ok, code)
		}
	})

	t.Run("upstream payload missing role -> 403, local role unchanged", func(t *testing.T) {
		srv, _ := newMCPUserServer(t, func(string) (int, string) {
			// 上游只回了 ID/用户名：不能据此认定角色被撤销并写库
			return http.StatusOK, `{"id":8,"username":"u"}`
		})
		defer srv.Close()
		s, g := newAuthTestService(t, srv.URL)
		u := &model.User{Sub2APIID: 8, Username: "u", Role: "admin", LastSeenAt: time.Now().Add(-time.Hour)}
		if err := g.Create(u).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
		ok, code := runRequireAdmin(t, s, u)
		if ok || code != http.StatusForbidden {
			t.Fatalf("ok=%v code=%d, want 403", ok, code)
		}
		var got model.User
		g.First(&got, u.ID)
		if got.Role != "admin" {
			t.Fatalf("local role = %q, want admin（角色字段缺失时不应持久化降级）", got.Role)
		}
	})

	t.Run("still admin upstream -> pass + last_seen refreshed", func(t *testing.T) {
		srv, _ := newMCPUserServer(t, func(string) (int, string) {
			return http.StatusOK, `{"id":9,"role":"admin"}`
		})
		defer srv.Close()
		s, g := newAuthTestService(t, srv.URL)
		stale := time.Now().Add(-30 * time.Minute)
		u := &model.User{Sub2APIID: 9, Username: "u", Role: "admin", LastSeenAt: stale}
		if err := g.Create(u).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
		ok, code := runRequireAdmin(t, s, u)
		if !ok || code != http.StatusOK {
			t.Fatalf("ok=%v code=%d, want pass", ok, code)
		}
		var got model.User
		g.First(&got, u.ID)
		if !got.LastSeenAt.After(stale) {
			t.Fatalf("last_seen_at 未刷新: %v", got.LastSeenAt)
		}
	})

	t.Run("recently seen -> no upstream call", func(t *testing.T) {
		srv, hits := newMCPUserServer(t, func(string) (int, string) {
			return http.StatusOK, `{"role":"admin"}`
		})
		defer srv.Close()
		s, g := newAuthTestService(t, srv.URL)
		u := &model.User{Sub2APIID: 11, Username: "u", Role: "admin", LastSeenAt: time.Now().Add(-2 * time.Minute)}
		if err := g.Create(u).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
		ok, _ := runRequireAdmin(t, s, u)
		if !ok {
			t.Fatal("近期活跃的管理员应直接放行")
		}
		if atomic.LoadInt64(hits) != 0 {
			t.Fatalf("upstream hits = %d, want 0（复验周期内不应打源站）", atomic.LoadInt64(hits))
		}
	})

	t.Run("local fallback admin bypasses upstream", func(t *testing.T) {
		srv, hits := newMCPUserServer(t, func(string) (int, string) {
			return http.StatusOK, `{"role":"user"}`
		})
		defer srv.Close()
		s, g := newAuthTestService(t, srv.URL)
		// Sub2APIID = -1 是本地兜底管理员，与上游无映射
		u := &model.User{Sub2APIID: -1, Username: "admin", Role: "admin", LastSeenAt: time.Now().Add(-99 * time.Hour)}
		if err := g.Create(u).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
		ok, _ := runRequireAdmin(t, s, u)
		if !ok {
			t.Fatal("本地兜底管理员不应被上游复验拦截")
		}
		if atomic.LoadInt64(hits) != 0 {
			t.Fatalf("upstream hits = %d, want 0", atomic.LoadInt64(hits))
		}
	})

	t.Run("plain user rejected without upstream call", func(t *testing.T) {
		srv, hits := newMCPUserServer(t, func(string) (int, string) {
			return http.StatusOK, `{"role":"admin"}`
		})
		defer srv.Close()
		s, g := newAuthTestService(t, srv.URL)
		u := &model.User{Sub2APIID: 5, Username: "u", Role: "user", LastSeenAt: time.Now().Add(-time.Hour)}
		if err := g.Create(u).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
		ok, code := runRequireAdmin(t, s, u)
		if ok || code != http.StatusForbidden {
			t.Fatalf("ok=%v code=%d, want 403", ok, code)
		}
		if atomic.LoadInt64(hits) != 0 {
			t.Fatalf("upstream hits = %d, want 0", atomic.LoadInt64(hits))
		}
	})
}
