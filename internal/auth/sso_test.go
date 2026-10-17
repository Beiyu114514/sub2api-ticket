package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"sub2api-ticket/internal/model"
)

func postSSO(t *testing.T, s *Service, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/sso", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	s.SSO(c)
	return w
}

// TestSSOFailsClosedWithoutUpstreamID 回归：上游 /auth/me 响应解析不出用户 ID 时必须拒绝登录。
// 旧实现会回退使用请求体里的 user_id，导致任意持有有效 token 的用户都能伪造成任意账号
// （包括管理员）——身份只能由上游决定。
func TestSSOFailsClosedWithoutUpstreamID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 只有用户名和角色，没有任何 id 字段
		w.Write([]byte(`{"username":"attacker","role":"user"}`))
	}))
	defer srv.Close()
	s, g := newAuthTestService(t, srv.URL)

	w := postSSO(t, s, `{"token":"attacker-valid-token","user_id":42}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401（不能信任客户端提交的 user_id）", w.Code)
	}

	var count int64
	g.Model(&model.User{}).Count(&count)
	if count != 0 {
		t.Fatalf("用户被创建了 %d 个，应保持 0", count)
	}
}

// TestSSORejectsUserIDMismatch token 属于 A 却声明 user_id=B 时必须拒绝。
func TestSSORejectsUserIDMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":7,"username":"alice","role":"user"}`))
	}))
	defer srv.Close()
	s, _ := newAuthTestService(t, srv.URL)

	w := postSSO(t, s, `{"token":"alice-token","user_id":42}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", w.Code)
	}
}

// TestSSOAcceptsMatchingID 正常链路：上游给出的 ID 与声明一致时放行。
func TestSSOAcceptsMatchingID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":42,"username":"alice","email":"a@example.com","role":"admin"}`))
	}))
	defer srv.Close()
	s, _ := newAuthTestService(t, srv.URL)

	w := postSSO(t, s, `{"token":"alice-token","user_id":42}`)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"sub2api_id":42`) {
		t.Fatalf("响应未包含期望用户: %s", w.Body.String())
	}
}

// TestSSOWithoutDeclaredUserID 不传 user_id 时完全以上游为准（推荐调用方式）。
func TestSSOWithoutDeclaredUserID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":9,"username":"bob","role":"user"}`))
	}))
	defer srv.Close()
	s, _ := newAuthTestService(t, srv.URL)

	w := postSSO(t, s, `{"token":"bob-token"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}
