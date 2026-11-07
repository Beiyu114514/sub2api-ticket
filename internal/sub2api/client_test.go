package sub2api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGetAdminUserThroughMCP(t *testing.T) {
	var initializeCalls, toolCalls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			t.Errorf("path = %q, want /mcp", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tr_mcp_test" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("x-api-key") != "" {
			t.Errorf("admin API key header must not be sent")
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  struct {
				Name      string `json:"name"`
				Arguments struct {
					OperationID string            `json:"operation_id"`
					Path        map[string]string `json:"path"`
				} `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode MCP request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.JSONRPC != "2.0" {
			t.Errorf("jsonrpc = %q", req.JSONRPC)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q", got)
		}
		if got := r.Header.Get("Accept"); !strings.Contains(got, "application/json") || !strings.Contains(got, "text/event-stream") {
			t.Errorf("accept = %q", got)
		}
		switch req.Method {
		case "initialize":
			atomic.AddInt64(&initializeCalls, 1)
			writeRPCResult(t, w, req.ID, map[string]any{
				"protocolVersion": "2025-11-25",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]string{"name": "TokenRouter", "version": "test"},
			})
		case "notifications/initialized":
			if got := r.Header.Get("MCP-Protocol-Version"); got != "2025-11-25" {
				t.Errorf("protocol version = %q", got)
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			atomic.AddInt64(&toolCalls, 1)
			if req.Params.Name != "read_operation" {
				t.Errorf("tool name = %q", req.Params.Name)
			}
			if req.Params.Arguments.OperationID != "users.get" {
				t.Errorf("operation_id = %q", req.Params.Arguments.OperationID)
			}
			if got := req.Params.Arguments.Path["id"]; got != "42" {
				t.Errorf("path id = %q", got)
			}
			if got := r.Header.Get("MCP-Protocol-Version"); got != "2025-11-25" {
				t.Errorf("protocol version = %q", got)
			}
			writeRPCResult(t, w, req.ID, map[string]any{
				"content": []map[string]string{{"type": "text", "text": "ok"}},
				"structuredContent": map[string]any{
					"status":      "succeeded",
					"http_status": http.StatusOK,
					"data": map[string]any{
						"code": 0,
						"data": map[string]any{
							"id":       42,
							"username": "alice",
							"email":    "alice@example.com",
							"role":     "admin",
						},
					},
				},
				"isError": false,
			})
		default:
			t.Errorf("unexpected method %q", req.Method)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	client := New(srv.URL, "/api/v1/auth/me", "/mcp", "tr_mcp_test")
	info, err := client.GetAdminUser(42)
	if err != nil {
		t.Fatalf("GetAdminUser: %v", err)
	}
	if info.Sub2APIID != 42 || info.Username != "alice" || info.Email != "alice@example.com" || info.Role != "admin" {
		t.Fatalf("unexpected user info: %+v", info)
	}
	if atomic.LoadInt64(&initializeCalls) != 1 || atomic.LoadInt64(&toolCalls) != 1 {
		t.Fatalf("initialize calls=%d tool calls=%d", initializeCalls, toolCalls)
	}
}

func TestGetAdminUserMCPEventStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch req.Method {
		case "initialize":
			writeRPCResult(t, w, req.ID, map[string]any{"protocolVersion": "2025-11-25"})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			w.Header().Set("Content-Type", "text/event-stream")
			result := map[string]any{
				"content": []map[string]string{{"type": "text", "text": "ok"}},
				"structuredContent": map[string]any{
					"status":      "succeeded",
					"http_status": http.StatusOK,
					"data": map[string]any{
						"code": 0,
						"data": map[string]any{"id": 7, "role": "user"},
					},
				},
			}
			payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
			_, _ = w.Write([]byte("event: message\ndata: " + string(payload) + "\n\n"))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	client := New(srv.URL, "/api/v1/auth/me", "/mcp", "token")
	info, err := client.GetAdminUser(7)
	if err != nil {
		t.Fatalf("GetAdminUser: %v", err)
	}
	if info.Sub2APIID != 7 || info.Role != "user" {
		t.Fatalf("unexpected user info: %+v", info)
	}
}

func TestGetAdminUserMCPRejectsIncompletePayload(t *testing.T) {
	cases := map[string]any{
		"empty object": map[string]any{},
		"code only":    map[string]any{"code": 0},
		"missing role": map[string]any{"id": 42, "username": "alice"},
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newMCPUserDataServer(t, data)
			defer srv.Close()
			client := New(srv.URL, "/api/v1/auth/me", "/mcp", "token")
			if _, err := client.GetAdminUser(42); err == nil {
				t.Fatalf("payload %v should be rejected", data)
			}
		})
	}
}

func newMCPUserDataServer(t *testing.T, data any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch req.Method {
		case "initialize":
			writeRPCResult(t, w, req.ID, map[string]any{"protocolVersion": "2025-11-25"})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			writeRPCResult(t, w, req.ID, map[string]any{
				"content": []map[string]string{{"type": "text", "text": "ok"}},
				"structuredContent": map[string]any{
					"status":      "succeeded",
					"http_status": http.StatusOK,
					"data":        data,
				},
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
}

func TestGetAdminUserMCPToolError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch req.Method {
		case "initialize":
			writeRPCResult(t, w, req.ID, map[string]any{"protocolVersion": "2025-11-25"})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			writeRPCResult(t, w, req.ID, map[string]any{
				"content": []map[string]string{{"type": "text", "text": "operation is not authorized"}},
				"structuredContent": map[string]any{
					"status":  "failed",
					"message": "operation is not authorized",
				},
				"isError": true,
			})
		}
	}))
	defer srv.Close()

	client := New(srv.URL, "/api/v1/auth/me", "/mcp", "token")
	_, err := client.GetAdminUser(42)
	if err == nil || !strings.Contains(err.Error(), "operation is not authorized") {
		t.Fatalf("error = %v", err)
	}
}

func TestGetAdminUserMCPUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := New(srv.URL, "/api/v1/auth/me", "/mcp", "bad-token")
	_, err := client.GetAdminUser(42)
	if err == nil || !strings.Contains(err.Error(), "令牌无效") {
		t.Fatalf("error = %v", err)
	}
}

func TestGetAdminUserRequiresMCPToken(t *testing.T) {
	client := New("https://router.example.com", "/api/v1/auth/me", "/mcp", "")
	if client.MCPConfigured() {
		t.Fatal("MCPConfigured() = true without token")
	}
	if _, err := client.GetAdminUser(42); err == nil {
		t.Fatal("GetAdminUser should fail without MCP token")
	}
}

func TestMCPEndpointConfiguration(t *testing.T) {
	client := New("https://router.example.com/", "/api/v1/auth/me", "", "token")
	endpoint, err := client.mcpEndpoint()
	if err != nil {
		t.Fatalf("mcpEndpoint: %v", err)
	}
	if endpoint != "https://router.example.com/mcp" {
		t.Fatalf("endpoint = %q", endpoint)
	}

	absolute := New("", "/api/v1/auth/me", "https://mcp.example.com/custom", "token")
	if !absolute.MCPConfigured() {
		t.Fatal("absolute MCP URL should be usable without base URL")
	}
}

func TestParseUserJSONUnwrapsNestedData(t *testing.T) {
	info := parseUserJSON([]byte(`{"status":"succeeded","data":{"code":0,"data":{"id":9007199254740993,"username":"bob","role":"admin"}}}`))
	if info.Sub2APIID != 9007199254740993 || info.Username != "bob" || info.Role != "admin" {
		t.Fatalf("unexpected user info: %+v", info)
	}
}

func writeRPCResult(t *testing.T, w http.ResponseWriter, id json.RawMessage, result any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}); err != nil {
		t.Errorf("write MCP response: %v", err)
	}
}
