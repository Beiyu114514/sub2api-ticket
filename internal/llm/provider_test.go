package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ChatCompletion 直连 OpenAI 兼容 API：请求体、鉴权头、响应解析
func TestChatCompletion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization = %s", got)
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["model"] != "gpt-test" {
			t.Errorf("model = %v", in["model"])
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"你好，我是 AI 客服"}}]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "sk-test", Model: "gpt-test"}
	out, err := c.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "hi"}}, 0.3)
	if err != nil {
		t.Fatal(err)
	}
	if out != "你好，我是 AI 客服" {
		t.Errorf("回复 = %s", out)
	}
}

// 未配置 / 空回复 / 非 200 报错
func TestChatCompletionErrors(t *testing.T) {
	if _, err := (&Client{}).ChatCompletion(context.Background(), nil, 0); err == nil {
		t.Error("未配置应报错")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, APIKey: "k", Model: "m"}
	if _, err := c.ChatCompletion(context.Background(), nil, 0); err == nil {
		t.Error("401 应报错")
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv2.Close()
	c2 := &Client{BaseURL: srv2.URL, APIKey: "k", Model: "m"}
	if _, err := c2.ChatCompletion(context.Background(), nil, 0); err == nil {
		t.Error("空 choices 应报错")
	}
}
