package bridge

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/mimicode/mcp_bridge/internal/config"
)

type fakeBackend struct {
	initResult        *mcp.InitializeResult
	tools             []mcp.Tool
	resources         []mcp.Resource
	resourceTemplates []mcp.ResourceTemplate
	prompts           []mcp.Prompt
	notifications     func(notification mcp.JSONRPCNotification)
}

func (f *fakeBackend) Initialize(context.Context, mcp.InitializeRequest) (*mcp.InitializeResult, error) {
	return f.initResult, nil
}

func (f *fakeBackend) Ping(context.Context) error { return nil }

func (f *fakeBackend) ListResources(context.Context, mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error) {
	return &mcp.ListResourcesResult{Resources: append([]mcp.Resource(nil), f.resources...)}, nil
}

func (f *fakeBackend) ListResourceTemplates(context.Context, mcp.ListResourceTemplatesRequest) (*mcp.ListResourceTemplatesResult, error) {
	return &mcp.ListResourceTemplatesResult{ResourceTemplates: append([]mcp.ResourceTemplate(nil), f.resourceTemplates...)}, nil
}

func (f *fakeBackend) ReadResource(context.Context, mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return &mcp.ReadResourceResult{}, nil
}

func (f *fakeBackend) ListPrompts(context.Context, mcp.ListPromptsRequest) (*mcp.ListPromptsResult, error) {
	return &mcp.ListPromptsResult{Prompts: append([]mcp.Prompt(nil), f.prompts...)}, nil
}

func (f *fakeBackend) GetPrompt(context.Context, mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{}, nil
}

func (f *fakeBackend) ListTools(context.Context, mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	return &mcp.ListToolsResult{Tools: append([]mcp.Tool(nil), f.tools...)}, nil
}

func (f *fakeBackend) CallTool(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{}, nil
}

func (f *fakeBackend) Complete(context.Context, mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	return &mcp.CompleteResult{}, nil
}

func (f *fakeBackend) OnNotification(handler func(notification mcp.JSONRPCNotification)) {
	f.notifications = handler
}

func (f *fakeBackend) Close() error { return nil }

func TestRouteBridgeWarmupMirrorsBackendDescriptors(t *testing.T) {
	backend := &fakeBackend{
		initResult: &mcp.InitializeResult{
			ServerInfo: mcp.Implementation{
				Name:    "fake-backend",
				Version: "1.0.0",
			},
			Capabilities: mcp.ServerCapabilities{
				Tools: &struct {
					ListChanged bool "json:\"listChanged,omitempty\""
				}{ListChanged: true},
				Resources: &struct {
					Subscribe   bool "json:\"subscribe,omitempty\""
					ListChanged bool "json:\"listChanged,omitempty\""
				}{ListChanged: true},
				Prompts: &struct {
					ListChanged bool "json:\"listChanged,omitempty\""
				}{ListChanged: true},
				Completions: &struct{}{},
			},
		},
		tools: []mcp.Tool{
			{Name: "echo", InputSchema: mcp.ToolInputSchema{Type: "object"}},
		},
		resources: []mcp.Resource{
			{URI: "file:///tmp/a.txt", Name: "a.txt"},
		},
		resourceTemplates: []mcp.ResourceTemplate{
			mcp.NewResourceTemplate("file:///{path}", "files"),
		},
		prompts: []mcp.Prompt{
			{Name: "summarize"},
		},
	}

	factory := func(context.Context, config.Route, *slog.Logger) (Backend, error) {
		return backend, nil
	}

	route := NewRouteBridge(config.Route{
		Name:    "fake",
		Path:    "/mcp/fake",
		Command: "unused",
		Timeout: config.DefaultTimeout,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), factory)

	if err := route.Warmup(context.Background()); err != nil {
		t.Fatalf("Warmup() error = %v", err)
	}

	if route.proxy == nil {
		t.Fatal("expected proxy server to be created")
	}
	if len(route.proxy.ListTools()) != 1 {
		t.Fatalf("expected 1 mirrored tool, got %d", len(route.proxy.ListTools()))
	}
	if len(route.proxy.ListResources()) != 1 {
		t.Fatalf("expected 1 mirrored resource, got %d", len(route.proxy.ListResources()))
	}
	if len(route.proxy.ListPrompts()) != 1 {
		t.Fatalf("expected 1 mirrored prompt, got %d", len(route.proxy.ListPrompts()))
	}

	info := route.Info()
	if !info.Ready {
		t.Fatal("expected route to be ready after warmup")
	}
	if info.BackendName != "fake-backend" {
		t.Fatalf("unexpected backend name: %q", info.BackendName)
	}
}
