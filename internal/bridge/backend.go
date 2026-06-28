package bridge

import (
	"bufio"
	"context"
	"io"
	"log/slog"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/mimicode/mcp_bridge/internal/config"
)

type Backend interface {
	Initialize(ctx context.Context, request mcp.InitializeRequest) (*mcp.InitializeResult, error)
	Ping(ctx context.Context) error
	ListResources(ctx context.Context, request mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error)
	ListResourceTemplates(ctx context.Context, request mcp.ListResourceTemplatesRequest) (*mcp.ListResourceTemplatesResult, error)
	ReadResource(ctx context.Context, request mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error)
	ListPrompts(ctx context.Context, request mcp.ListPromptsRequest) (*mcp.ListPromptsResult, error)
	GetPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error)
	ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error)
	CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	Complete(ctx context.Context, request mcp.CompleteRequest) (*mcp.CompleteResult, error)
	OnNotification(handler func(notification mcp.JSONRPCNotification))
	Close() error
}

type BackendFactory func(ctx context.Context, route config.Route, logger *slog.Logger) (Backend, error)

func NewStdioBackend(ctx context.Context, route config.Route, logger *slog.Logger) (Backend, error) {
	client, err := mcpclient.NewStdioMCPClient(route.Command, route.EnvPairs(), route.Args...)
	if err != nil {
		return nil, err
	}

	if stderr, ok := mcpclient.GetStderr(client); ok {
		go streamStderr(route.Name, stderr, logger)
	}

	return client, nil
}

func streamStderr(routeName string, stderr io.Reader, logger *slog.Logger) {
	scanner := bufio.NewScanner(stderr)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		logger.Warn("backend stderr", "route", routeName, "line", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		logger.Warn("backend stderr reader stopped", "route", routeName, "error", err)
	}
}
