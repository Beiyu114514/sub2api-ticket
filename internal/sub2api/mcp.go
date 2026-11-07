package sub2api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMCPPath       = "/mcp"
	mcpProtocolVersion   = "2025-11-25"
	mcpMaxResponseBytes  = 2 << 20
	mcpCallTimeout       = 15 * time.Second
	mcpCloseTimeout      = 2 * time.Second
	mcpOperationUserGet  = "users.get"
	mcpReadOperationName = "read_operation"
)

// MCPConfigured 表示是否具备通过 MCP 查询用户资料的必要配置。
func (c *Client) MCPConfigured() bool {
	if c == nil || strings.TrimSpace(c.MCPToken) == "" {
		return false
	}
	_, err := c.mcpEndpoint()
	return err == nil
}

// GetAdminUser 通过 TokenRouter MCP 的 users.get 只读操作查询用户资料。
// best effort：调用方自行决定失败时是否阻断请求。
func (c *Client) GetAdminUser(id int64) (*UserInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mcpCallTimeout)
	defer cancel()
	return c.GetAdminUserContext(ctx, id)
}

// GetAdminUserContext 与 GetAdminUser 相同，但允许调用方控制超时和取消。
func (c *Client) GetAdminUserContext(ctx context.Context, id int64) (*UserInfo, error) {
	if id <= 0 {
		return nil, fmt.Errorf("用户 ID 无效")
	}
	if !c.MCPConfigured() {
		return nil, fmt.Errorf("未配置 TokenRouter MCP Token（SUB2API_MCP_TOKEN）")
	}
	raw, err := c.callMCPTool(ctx, mcpReadOperationName, map[string]any{
		"operation_id": mcpOperationUserGet,
		"path": map[string]string{
			"id": strconv.FormatInt(id, 10),
		},
	})
	if err != nil {
		return nil, err
	}

	var operation struct {
		Status     string          `json:"status"`
		HTTPStatus int             `json:"http_status"`
		Data       json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &operation); err != nil {
		return nil, fmt.Errorf("解析 TokenRouter MCP 用户查询结果失败: %w", err)
	}
	if operation.HTTPStatus >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("TokenRouter MCP 查询用户返回 HTTP %d: %s", operation.HTTPStatus, mcpOperationMessage(operation.Data))
	}
	if operation.Status != "" && operation.Status != "succeeded" {
		return nil, fmt.Errorf("TokenRouter MCP 查询用户失败: %s", mcpOperationMessage(operation.Data))
	}
	if isEmptyJSON(operation.Data) {
		return nil, fmt.Errorf("TokenRouter MCP 查询用户未返回资料")
	}

	info := parseUserJSON(operation.Data)
	// 必须先判空再回填 ID：否则空响应会因回填而“非空”，
	// 调用方会把查不到角色误判为角色被撤销并持久化降级。
	if info.empty() {
		return nil, fmt.Errorf("TokenRouter MCP 未从响应中解析出用户资料")
	}
	if strings.TrimSpace(info.Role) == "" {
		return nil, fmt.Errorf("TokenRouter MCP 响应缺少角色字段，无法确认管理员身份")
	}
	if info.Sub2APIID == 0 {
		// users.get 的目标 ID 由请求确定；响应省略 ID 时沿用请求值。
		info.Sub2APIID = id
	} else if info.Sub2APIID != id {
		return nil, fmt.Errorf("TokenRouter MCP 返回的用户 ID %d 与请求 ID %d 不一致", info.Sub2APIID, id)
	}
	return &info, nil
}

func (c *Client) callMCPTool(ctx context.Context, name string, args any) (json.RawMessage, error) {
	endpoint, err := c.mcpEndpoint()
	if err != nil {
		return nil, err
	}
	session := &mcpSession{
		endpoint: endpoint,
		token:    strings.TrimSpace(c.MCPToken),
		http:     c.HTTP,
	}
	if err := session.initialize(ctx); err != nil {
		return nil, err
	}
	defer session.close()
	return session.callTool(ctx, name, args)
}

func (c *Client) mcpEndpoint() (string, error) {
	path := strings.TrimSpace(c.MCPPath)
	if path == "" {
		path = defaultMCPPath
	}
	if parsed, err := url.Parse(path); err == nil && parsed.IsAbs() {
		if (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
			return path, nil
		}
		return "", fmt.Errorf("SUB2API_MCP_PATH 必须是 http(s) URL 或以 / 开头的路径")
	}
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return "", fmt.Errorf("未配置 SUB2API_BASE_URL")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path, nil
}

func mcpOperationMessage(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "未知错误"
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err == nil {
		if message := findMessage(value); message != "" {
			return message
		}
	}
	return truncateText(string(trimmed), 300)
}

func findMessage(value any) string {
	m, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"message", "error", "reason"} {
		if text, ok := m[key].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	if nested, ok := m["data"]; ok {
		if message := findMessage(nested); message != "" {
			return message
		}
	}
	return ""
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (e *rpcError) Error() string {
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = "未知 MCP 错误"
	}
	return fmt.Sprintf("MCP 错误 %d: %s", e.Code, message)
}

type mcpSession struct {
	endpoint        string
	token           string
	http            *http.Client
	protocolVersion string
	sessionID       string
	nextID          int64
}

func (s *mcpSession) initialize(ctx context.Context) error {
	s.nextID++
	raw, err := s.post(ctx, rpcRequest{
		JSONRPC: "2.0",
		ID:      s.nextID,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo": map[string]string{
				"name":    "sub2api-ticket",
				"version": "1.0.0",
			},
		},
	}, true, false)
	if err != nil {
		return fmt.Errorf("TokenRouter MCP 初始化失败: %w", err)
	}
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("解析 TokenRouter MCP 初始化结果失败: %w", err)
	}
	s.protocolVersion = strings.TrimSpace(result.ProtocolVersion)
	if s.protocolVersion == "" {
		s.protocolVersion = mcpProtocolVersion
	}
	if _, err := s.post(ctx, rpcRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}, false, true); err != nil {
		return fmt.Errorf("TokenRouter MCP 初始化确认失败: %w", err)
	}
	return nil
}

func (s *mcpSession) callTool(ctx context.Context, name string, args any) (json.RawMessage, error) {
	s.nextID++
	raw, err := s.post(ctx, rpcRequest{
		JSONRPC: "2.0",
		ID:      s.nextID,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      name,
			"arguments": args,
		},
	}, true, true)
	if err != nil {
		return nil, err
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		IsError           bool            `json:"isError"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("解析 TokenRouter MCP 工具结果失败: %w", err)
	}
	text := ""
	for _, content := range result.Content {
		if strings.TrimSpace(content.Text) != "" {
			text = strings.TrimSpace(content.Text)
			break
		}
	}
	payload := result.StructuredContent
	if result.IsError {
		message := mcpOperationMessage(payload)
		if text != "" {
			message = text
		}
		return nil, fmt.Errorf("TokenRouter MCP 工具 %s 调用失败: %s", name, message)
	}
	if isEmptyJSON(payload) && text != "" && json.Valid([]byte(text)) {
		payload = json.RawMessage(text)
	}
	if isEmptyJSON(payload) {
		return nil, fmt.Errorf("TokenRouter MCP 工具 %s 未返回结构化结果", name)
	}
	return payload, nil
}

func (s *mcpSession) post(ctx context.Context, message rpcRequest, wantResponse, includeProtocolVersion bool) (json.RawMessage, error) {
	body, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("编码 MCP 请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建 MCP 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("User-Agent", "sub2api-ticket/1.0.0")
	if includeProtocolVersion && s.protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", s.protocolVersion)
	}
	if s.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", s.sessionID)
	}

	client := s.http
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 TokenRouter MCP 失败: %w", err)
	}
	defer resp.Body.Close()
	if sessionID := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")); sessionID != "" {
		s.sessionID = sessionID
	}
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, mcpMaxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取 TokenRouter MCP 响应失败: %w", err)
	}
	if len(responseBody) > mcpMaxResponseBytes {
		return nil, fmt.Errorf("TokenRouter MCP 响应超过 %d 字节限制", mcpMaxResponseBytes)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, mcpHTTPStatusError(resp.StatusCode, responseBody)
	}
	if !wantResponse {
		return nil, nil
	}
	return decodeMCPResponse(responseBody, resp.Header.Get("Content-Type"), message.ID)
}

func (s *mcpSession) close() {
	if s.sessionID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), mcpCloseTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.endpoint, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	if s.protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", s.protocolVersion)
	}
	req.Header.Set("Mcp-Session-Id", s.sessionID)
	client := s.http
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

func decodeMCPResponse(body []byte, contentType string, id int64) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(body)
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "text/event-stream" || bytes.HasPrefix(trimmed, []byte("data:")) {
		return decodeMCPEventStream(trimmed, id)
	}
	if mediaType == "application/json" || mediaType == "" || strings.HasSuffix(mediaType, "+json") {
		return decodeMCPJSON(trimmed, id)
	}
	return nil, fmt.Errorf("TokenRouter MCP 返回不支持的 Content-Type %q", contentType)
}

func decodeMCPJSON(body []byte, id int64) (json.RawMessage, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("TokenRouter MCP 返回空响应")
	}
	var response rpcResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析 TokenRouter MCP 响应失败: %w", err)
	}
	if response.Error != nil {
		return nil, response.Error
	}
	if len(response.ID) > 0 && !rpcIDMatches(response.ID, id) {
		return nil, fmt.Errorf("TokenRouter MCP 响应 ID 与请求不匹配")
	}
	if len(response.Result) == 0 {
		return nil, fmt.Errorf("TokenRouter MCP 响应缺少 result")
	}
	return response.Result, nil
}

func decodeMCPEventStream(body []byte, id int64) (json.RawMessage, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), mcpMaxResponseBytes)
	var dataLines []string
	var result json.RawMessage

	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = nil
		if payload == "" || payload == "[DONE]" {
			return nil
		}
		var response rpcResponse
		if err := json.Unmarshal([]byte(payload), &response); err != nil {
			return fmt.Errorf("解析 TokenRouter MCP SSE 响应失败: %w", err)
		}
		if len(response.ID) > 0 && !rpcIDMatches(response.ID, id) {
			return nil
		}
		if response.Error != nil {
			return response.Error
		}
		if len(response.Result) > 0 {
			result = response.Result
		}
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			if len(result) > 0 {
				return result, nil
			}
			continue
		}
		if data, ok := strings.CutPrefix(line, "data:"); ok {
			dataLines = append(dataLines, strings.TrimPrefix(data, " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 TokenRouter MCP SSE 响应失败: %w", err)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("TokenRouter MCP SSE 响应中没有匹配的 JSON-RPC 结果")
	}
	return result, nil
}

func rpcIDMatches(raw json.RawMessage, id int64) bool {
	got := strings.TrimSpace(string(raw))
	want := strconv.FormatInt(id, 10)
	return got == want || got == `"`+want+`"`
}

func mcpHTTPStatusError(status int, body []byte) error {
	detail := truncateText(strings.TrimSpace(string(body)), 300)
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("TokenRouter MCP 令牌无效或已过期")
	case http.StatusForbidden:
		return fmt.Errorf("TokenRouter MCP 访问被拒绝，请确认令牌具备 users.get 权限")
	case http.StatusNotFound:
		return fmt.Errorf("TokenRouter MCP 接口不存在，请检查 SUB2API_MCP_PATH（默认 /mcp）")
	case http.StatusTooManyRequests:
		return fmt.Errorf("TokenRouter MCP 请求过于频繁")
	}
	if detail != "" {
		return fmt.Errorf("TokenRouter MCP 返回 HTTP %d: %s", status, detail)
	}
	return fmt.Errorf("TokenRouter MCP 返回 HTTP %d", status)
}

func isEmptyJSON(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

func truncateText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
