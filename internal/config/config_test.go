package config

import "testing"

func TestLoadMCPConfig(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "")
	t.Setenv("COOKIE_SAMESITE", "")
	t.Setenv("SUB2API_BASE_URL", "https://router.example.com/")
	t.Setenv("SUB2API_MCP_TOKEN", "tr_mcp_primary")
	t.Setenv("SUB2API_MCP_PATH", "")
	t.Setenv("TOKENROUTER_MCP_TOKEN", "tr_mcp_alias")
	t.Setenv("TOKENROUTER_MCP_PATH", "https://router.example.com/custom-mcp")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Sub2APIBaseURL != "https://router.example.com" {
		t.Fatalf("base URL = %q", cfg.Sub2APIBaseURL)
	}
	if cfg.Sub2APIMCPToken != "tr_mcp_primary" {
		t.Fatalf("MCP token = %q", cfg.Sub2APIMCPToken)
	}
	if cfg.Sub2APIMCPPath != "https://router.example.com/custom-mcp" {
		t.Fatalf("MCP path = %q", cfg.Sub2APIMCPPath)
	}
}

func TestLoadMCPConfigDefaults(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "")
	t.Setenv("COOKIE_SAMESITE", "")
	t.Setenv("SUB2API_MCP_TOKEN", "")
	t.Setenv("SUB2API_MCP_PATH", "")
	t.Setenv("TOKENROUTER_MCP_TOKEN", "")
	t.Setenv("TOKENROUTER_MCP_PATH", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Sub2APIMCPPath != "/mcp" {
		t.Fatalf("MCP path = %q, want /mcp", cfg.Sub2APIMCPPath)
	}
}
