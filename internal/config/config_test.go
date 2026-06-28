package config

import "testing"

func TestNormalizeAssignsStablePaths(t *testing.T) {
	cfg := File{
		MCPServers: map[string]Server{
			"Filesystem MCP": {
				Enabled: true,
				Command: "npx",
			},
			"Filesystem-MCP": {
				Enabled: true,
				Command: "npx",
			},
			"custom": {
				Enabled: true,
				Command: "uvx",
				Path:    "/bridge/custom",
			},
			"disabled": {
				Enabled: false,
				Command: "ignored",
			},
		},
	}

	runtime, err := cfg.Normalize("config.json", "/mcp")
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if len(runtime.Servers) != 3 {
		t.Fatalf("expected 3 enabled routes, got %d", len(runtime.Servers))
	}

	got := map[string]string{}
	for _, route := range runtime.Servers {
		got[route.Name] = route.Path
	}

	if got["Filesystem MCP"] != "/mcp/filesystem-mcp" {
		t.Fatalf("unexpected auto path: %q", got["Filesystem MCP"])
	}
	if got["Filesystem-MCP"] != "/mcp/filesystem-mcp-2" {
		t.Fatalf("unexpected collision-resolved path: %q", got["Filesystem-MCP"])
	}
	if got["custom"] != "/bridge/custom" {
		t.Fatalf("unexpected custom path: %q", got["custom"])
	}
}

func TestNormalizeRejectsDuplicateExplicitPath(t *testing.T) {
	cfg := File{
		MCPServers: map[string]Server{
			"a": {
				Enabled: true,
				Command: "cmd-a",
				Path:    "/same",
			},
			"b": {
				Enabled: true,
				Command: "cmd-b",
				Path:    "/same",
			},
		},
	}

	if _, err := cfg.Normalize("config.json", "/mcp"); err == nil {
		t.Fatal("expected duplicate explicit path error")
	}
}

func TestNormalizeRejectsReservedPath(t *testing.T) {
	cfg := File{
		MCPServers: map[string]Server{
			"bad": {
				Enabled: true,
				Command: "cmd",
				Path:    "/_admin/api/config",
			},
		},
	}

	if _, err := cfg.Normalize("config.json", "/mcp"); err == nil {
		t.Fatal("expected reserved path error")
	}
}

func TestNormalizeAllowsAllServersDisabled(t *testing.T) {
	cfg := File{
		MCPServers: map[string]Server{
			"disabled-a": {
				Enabled: false,
				Command: "ignored-a",
			},
			"disabled-b": {
				Enabled: false,
				Command: "ignored-b",
			},
		},
	}

	runtime, err := cfg.Normalize("config.json", "/mcp")
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if len(runtime.Servers) != 0 {
		t.Fatalf("expected 0 enabled routes, got %d", len(runtime.Servers))
	}
}

func TestNormalizeAllowsEmptyConfig(t *testing.T) {
	cfg := File{
		MCPServers: map[string]Server{},
	}

	runtime, err := cfg.Normalize("config.json", "/mcp")
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if len(runtime.Servers) != 0 {
		t.Fatalf("expected 0 enabled routes, got %d", len(runtime.Servers))
	}
}
