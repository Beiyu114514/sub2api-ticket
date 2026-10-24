package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Message chat/completions 消息
type Message struct {
	Role    string `json:"role"` // system / user / assistant
	Content string `json:"content"`
}

// Client OpenAI 兼容 API 直连客户端（base_url + api_key + model 后台可配）
type Client struct {
	BaseURL string // 如 https://api.openai.com/v1
	APIKey  string
	Model   string
	Timeout time.Duration
}

// ChatCompletion 非流式补全
func (c *Client) ChatCompletion(ctx context.Context, msgs []Message, temperature float64) (string, error) {
	if c.BaseURL == "" || c.APIKey == "" || c.Model == "" {
		return "", fmt.Errorf("LLM 未配置")
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(map[string]any{
		"model":       c.Model,
		"messages":    msgs,
		"temperature": temperature,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, truncateStr(string(raw), 200))
	}
	var out struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("LLM 响应解析失败: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("LLM 无 choices")
	}
	content := out.Choices[0].Message.Content
	if content == "" {
		return "", fmt.Errorf("LLM 空回复")
	}
	return content, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
