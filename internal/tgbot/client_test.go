package tgbot

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeAPIServer 按真实 Telegram 的 envelope 格式（{"ok":true,"result":...}）应答。
func fakeAPIServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/botTOK/getMe":
			w.Write([]byte(`{"ok":true,"result":{"id":777,"is_bot":true,"username":"demo_bot"}}`))
		case r.URL.Path == "/botTOK/setWebhook":
			w.Write([]byte(`{"ok":true,"result":true,"description":"Webhook was set"}`))
		case r.URL.Path == "/botTOK/sendMessage":
			w.Write([]byte(`{"ok":true,"result":{"message_id":55,"chat":{"id":1,"type":"private"},"text":"hi"}}`))
		case r.URL.Path == "/botTOK/createForumTopic":
			w.Write([]byte(`{"ok":true,"result":{"message_thread_id":42,"name":"x"}}`))
		case r.URL.Path == "/botTOK/getChatMember":
			w.Write([]byte(`{"ok":true,"result":{"status":"administrator"}}`))
		case r.URL.Path == "/botTOK/answerCallbackQuery":
			w.Write([]byte(`{"ok":true,"result":true}`))
		default:
			w.Write([]byte(`{"ok":false,"description":"unknown method"}`))
		}
	}))
}

// TestCallParsesEnvelope 回归：call() 必须把完整 envelope 解析进调用方的结构体。
// 旧实现只反序列化内层 result，导致 GetMe 永远 "getMe failed"、
// SetWebhook 永远解析失败（webhook 注册不上）、CreateForumTopic 永远返回 thread 0
// （所有工单通知都挤进群的 General 话题）。
func TestCallParsesEnvelope(t *testing.T) {
	srv := fakeAPIServer(t)
	defer srv.Close()
	withAPIBase(t, srv.URL)
	bot := New("TOK")

	me, err := bot.GetMe()
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if me.ID != 777 || me.Username != "demo_bot" {
		t.Fatalf("GetMe = %+v", me)
	}

	id, err := bot.SendMessage(SendMessageOpts{ChatID: 1, Text: "hi"})
	if err != nil || id != 55 {
		t.Fatalf("SendMessage = (%d, %v), want (55, nil)", id, err)
	}

	thread, err := bot.CreateForumTopic(1, "x")
	if err != nil || thread != 42 {
		t.Fatalf("CreateForumTopic = (%d, %v), want (42, nil)", thread, err)
	}

	if err := bot.SetWebhook("https://e.x/hook", "s"); err != nil {
		t.Fatalf("SetWebhook: %v", err)
	}

	member, err := bot.GetChatMember(-100, 1)
	if err != nil || !member.IsAdmin() {
		t.Fatalf("GetChatMember = (%+v, %v)", member, err)
	}
}

// TestCallSurfacesTelegramError 上游 ok=false 时必须返回描述性错误。
func TestCallSurfacesTelegramError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":false,"description":"Bad Request: chat not found"}`))
	}))
	defer srv.Close()
	withAPIBase(t, srv.URL)

	if _, err := New("TOK").SendMessage(SendMessageOpts{ChatID: 1, Text: "x"}); err == nil {
		t.Fatal("期望返回错误")
	}
}
