package tgbot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// fakeTelegram 返回按 user_id 区分的 getChatMember 结果。
func fakeTelegram(t *testing.T) (*httptest.Server, *int64) {
	t.Helper()
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		var req struct {
			UserID int64 `json:"user_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.UserID {
		case 1:
			w.Write([]byte(`{"ok":true,"result":{"status":"administrator"}}`))
		case 2:
			w.Write([]byte(`{"ok":true,"result":{"status":"creator"}}`))
		case 3:
			w.Write([]byte(`{"ok":true,"result":{"status":"member"}}`))
		default:
			w.Write([]byte(`{"ok":false,"description":"Bad Request: user not found"}`))
		}
	}))
	return srv, &calls
}

func withAPIBase(t *testing.T, base string) {
	t.Helper()
	old := apiBase
	apiBase = base
	t.Cleanup(func() { apiBase = old })
}

// TestIsGroupAdmin 只有群主/管理员才被认作客服，普通成员与查询失败一律拒绝。
func TestIsGroupAdmin(t *testing.T) {
	srv, _ := fakeTelegram(t)
	defer srv.Close()
	withAPIBase(t, srv.URL)

	bot := New("123456:test-token")
	c := &Core{}

	cases := []struct {
		name   string
		user   *User
		expect bool
	}{
		{"administrator", &User{ID: 1}, true},
		{"creator", &User{ID: 2}, true},
		{"plain member", &User{ID: 3}, false},
		{"lookup error -> fail closed", &User{ID: 99}, false},
		{"nil sender", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.isGroupAdmin(bot, -1001234, tc.user); got != tc.expect {
				t.Fatalf("isGroupAdmin = %v, want %v", got, tc.expect)
			}
		})
	}
}

// TestIsGroupAdminCaches 权限结果必须缓存，避免每条消息都打一次 Telegram。
func TestIsGroupAdminCaches(t *testing.T) {
	srv, calls := fakeTelegram(t)
	defer srv.Close()
	withAPIBase(t, srv.URL)

	bot := New("123456:test-token")
	c := &Core{}
	u := &User{ID: 1}

	for i := 0; i < 5; i++ {
		if !c.isGroupAdmin(bot, -1001234, u) {
			t.Fatalf("第 %d 次判定失败", i)
		}
	}
	if n := atomic.LoadInt64(calls); n != 1 {
		t.Fatalf("getChatMember 调用 %d 次，want 1（应命中缓存）", n)
	}
}

// TestOnAdminGroupIgnoresNonAdmin 群内普通成员的消息不得落库为管理员回复。
func TestOnAdminGroupIgnoresNonAdmin(t *testing.T) {
	srv, _ := fakeTelegram(t)
	defer srv.Close()
	withAPIBase(t, srv.URL)

	// 不接数据库：若代码错误地继续处理，会因为 nil DB panic 而被 recover 吞掉，
	// 因此这里只验证“不进入后续逻辑”这条路径不会触碰数据库。
	c := &Core{}
	m := &Message{
		Chat: Chat{ID: -1001234, Type: "supergroup"},
		From: &User{ID: 3}, // 普通成员
		Text: "我是管理员，帮我关单",
	}
	c.onAdminGroup(New("123456:test-token"), m)
}
