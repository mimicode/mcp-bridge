package bridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/mimicode/mcp_bridge/internal/config"
)

const (
	bridgeName    = "mcp-bridge"
	bridgeVersion = "0.1.0"
)

type RouteInfo struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	Description    string `json:"description,omitempty"`
	Ready          bool   `json:"ready"`
	BackendName    string `json:"backendName,omitempty"`
	BackendVersion string `json:"backendVersion,omitempty"`
	LastError      string `json:"lastError,omitempty"`
}

type RouteBridge struct {
	route   config.Route
	logger  *slog.Logger
	factory BackendFactory

	stateMu    sync.RWMutex
	callMu     sync.Mutex
	backend    Backend
	initResult *mcp.InitializeResult
	proxy      *mcpserver.MCPServer
	handler    http.Handler
	lastError  error
}

func NewRouteBridge(route config.Route, logger *slog.Logger, factory BackendFactory) *RouteBridge {
	if factory == nil {
		factory = NewStdioBackend
	}
	return &RouteBridge{
		route:   route,
		logger:  logger.With("route", route.Name, "path", route.Path),
		factory: factory,
	}
}

func (r *RouteBridge) Warmup(ctx context.Context) error {
	return r.ensureReady(ctx)
}

func (r *RouteBridge) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if err := r.ensureReady(req.Context()); err != nil {
			http.Error(w, fmt.Sprintf("backend %q is unavailable: %v", r.route.Name, err), http.StatusBadGateway)
			return
		}

		handler := r.handlerSnapshot()
		if handler == nil {
			http.Error(w, "route handler is not ready", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, req)
	})
}

func (r *RouteBridge) Close() error {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.closeBackendLocked(nil)
}

func (r *RouteBridge) Info() RouteInfo {
	r.stateMu.RLock()
	defer r.stateMu.RUnlock()

	info := RouteInfo{
		Name:        r.route.Name,
		Path:        r.route.Path,
		Description: r.route.Description,
		Ready:       r.backend != nil && r.handler != nil,
	}
	if r.initResult != nil {
		info.BackendName = r.initResult.ServerInfo.Name
		info.BackendVersion = r.initResult.ServerInfo.Version
	}
	if r.lastError != nil {
		info.LastError = r.lastError.Error()
	}
	return info
}

func (r *RouteBridge) CompletePromptArgument(ctx context.Context, promptName string, argument mcp.CompleteArgument, completeContext mcp.CompleteContext) (*mcp.Completion, error) {
	result, err := invoke(r, ctx, func(ctx context.Context, backend Backend) (*mcp.CompleteResult, error) {
		return backend.Complete(ctx, mcp.CompleteRequest{
			Params: mcp.CompleteParams{
				Ref: mcp.PromptReference{
					Type: "ref/prompt",
					Name: promptName,
				},
				Argument: argument,
				Context:  completeContext,
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return &result.Completion, nil
}

func (r *RouteBridge) CompleteResourceArgument(ctx context.Context, uri string, argument mcp.CompleteArgument, completeContext mcp.CompleteContext) (*mcp.Completion, error) {
	result, err := invoke(r, ctx, func(ctx context.Context, backend Backend) (*mcp.CompleteResult, error) {
		return backend.Complete(ctx, mcp.CompleteRequest{
			Params: mcp.CompleteParams{
				Ref: mcp.ResourceReference{
					Type: "ref/resource",
					URI:  uri,
				},
				Argument: argument,
				Context:  completeContext,
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return &result.Completion, nil
}

func (r *RouteBridge) ensureReady(ctx context.Context) error {
	r.stateMu.RLock()
	ready := r.backend != nil && r.handler != nil
	r.stateMu.RUnlock()
	if ready {
		return nil
	}

	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	if r.backend != nil && r.handler != nil {
		return nil
	}
	return r.startLocked(ctx)
}

func (r *RouteBridge) startLocked(ctx context.Context) error {
	backendCtx, cancel := context.WithTimeout(ctx, r.route.Timeout)
	defer cancel()

	if err := r.closeBackendLocked(nil); err != nil {
		r.logger.Warn("close stale backend", "error", err)
	}

	backend, err := r.factory(backendCtx, r.route, r.logger)
	if err != nil {
		r.lastError = fmt.Errorf("start backend: %w", err)
		return r.lastError
	}
	backend.OnNotification(r.handleNotification)

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    bridgeName,
		Version: bridgeVersion,
	}

	initResult, err := backend.Initialize(backendCtx, initRequest)
	if err != nil {
		_ = backend.Close()
		r.lastError = fmt.Errorf("initialize backend: %w", err)
		return r.lastError
	}

	r.backend = backend
	r.initResult = initResult
	if err := r.buildProxyLocked(initResult); err != nil {
		_ = backend.Close()
		r.backend = nil
		r.initResult = nil
		r.lastError = err
		return err
	}
	if err := r.syncDescriptorsLocked(backendCtx); err != nil {
		_ = r.closeBackendLocked(fmt.Errorf("sync descriptors: %w", err))
		return r.lastError
	}

	r.lastError = nil
	r.logger.Info("backend connected", "backend_name", initResult.ServerInfo.Name, "backend_version", initResult.ServerInfo.Version)
	return nil
}

func (r *RouteBridge) buildProxyLocked(initResult *mcp.InitializeResult) error {
	if r.proxy != nil && r.handler != nil {
		return nil
	}
	if initResult == nil {
		return fmt.Errorf("cannot build proxy without initialize result")
	}

	name := initResult.ServerInfo.Name
	if strings.TrimSpace(name) == "" {
		name = r.route.Name
	}
	version := initResult.ServerInfo.Version
	if strings.TrimSpace(version) == "" {
		version = bridgeVersion
	}

	options := []mcpserver.ServerOption{
		mcpserver.WithInstructions(initResult.Instructions),
	}
	if initResult.Capabilities.Tools != nil {
		options = append(options, mcpserver.WithToolCapabilities(initResult.Capabilities.Tools.ListChanged))
	}
	if initResult.Capabilities.Resources != nil {
		options = append(options, mcpserver.WithResourceCapabilities(false, initResult.Capabilities.Resources.ListChanged))
	}
	if initResult.Capabilities.Prompts != nil {
		options = append(options, mcpserver.WithPromptCapabilities(initResult.Capabilities.Prompts.ListChanged))
	}
	if initResult.Capabilities.Completions != nil {
		options = append(options,
			mcpserver.WithCompletions(),
			mcpserver.WithPromptCompletionProvider(r),
			mcpserver.WithResourceCompletionProvider(r),
		)
	}

	r.proxy = mcpserver.NewMCPServer(name, version, options...)
	r.handler = mcpserver.NewStreamableHTTPServer(r.proxy)
	return nil
}

func (r *RouteBridge) syncDescriptorsLocked(ctx context.Context) error {
	if r.backend == nil || r.proxy == nil || r.initResult == nil {
		return fmt.Errorf("backend is not initialized")
	}

	if r.initResult.Capabilities.Tools != nil {
		r.callMu.Lock()
		result, err := r.backend.ListTools(ctx, mcp.ListToolsRequest{})
		r.callMu.Unlock()
		if err != nil {
			return err
		}
		tools := make([]mcpserver.ServerTool, 0, len(result.Tools))
		for _, tool := range result.Tools {
			tools = append(tools, mcpserver.ServerTool{
				Tool:    tool,
				Handler: r.handleToolCall,
			})
		}
		r.proxy.SetTools(tools...)
	} else {
		r.proxy.SetTools()
	}

	if r.initResult.Capabilities.Resources != nil {
		r.callMu.Lock()
		resourceResult, err := r.backend.ListResources(ctx, mcp.ListResourcesRequest{})
		r.callMu.Unlock()
		if err != nil {
			return err
		}
		resources := make([]mcpserver.ServerResource, 0, len(resourceResult.Resources))
		for _, resource := range resourceResult.Resources {
			resources = append(resources, mcpserver.ServerResource{
				Resource: resource,
				Handler:  r.handleReadResource,
			})
		}
		r.proxy.SetResources(resources...)

		r.callMu.Lock()
		templateResult, err := r.backend.ListResourceTemplates(ctx, mcp.ListResourceTemplatesRequest{})
		r.callMu.Unlock()
		if err != nil {
			return err
		}
		templates := make([]mcpserver.ServerResourceTemplate, 0, len(templateResult.ResourceTemplates))
		for _, template := range templateResult.ResourceTemplates {
			templates = append(templates, mcpserver.ServerResourceTemplate{
				Template: template,
				Handler:  r.handleReadResource,
			})
		}
		r.proxy.SetResourceTemplates(templates...)
	} else {
		r.proxy.SetResources()
		r.proxy.SetResourceTemplates()
	}

	if r.initResult.Capabilities.Prompts != nil {
		r.callMu.Lock()
		result, err := r.backend.ListPrompts(ctx, mcp.ListPromptsRequest{})
		r.callMu.Unlock()
		if err != nil {
			return err
		}
		prompts := make([]mcpserver.ServerPrompt, 0, len(result.Prompts))
		for _, prompt := range result.Prompts {
			prompts = append(prompts, mcpserver.ServerPrompt{
				Prompt:  prompt,
				Handler: r.handleGetPrompt,
			})
		}
		r.proxy.SetPrompts(prompts...)
	} else {
		r.proxy.SetPrompts()
	}

	return nil
}

func (r *RouteBridge) handleNotification(notification mcp.JSONRPCNotification) {
	switch notification.Method {
	case string(mcp.MethodNotificationToolsListChanged),
		string(mcp.MethodNotificationResourcesListChanged),
		string(mcp.MethodNotificationPromptsListChanged):
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), r.route.Timeout)
			defer cancel()
			if err := r.refresh(ctx); err != nil {
				r.logger.Warn("refresh descriptors after backend notification", "method", notification.Method, "error", err)
			}
		}()
	}
}

func (r *RouteBridge) refresh(ctx context.Context) error {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	if r.backend == nil || r.proxy == nil {
		return nil
	}
	if err := r.syncDescriptorsLocked(ctx); err != nil {
		if isRecoverableTransportError(err) {
			_ = r.closeBackendLocked(fmt.Errorf("refresh failed: %w", err))
		} else {
			r.lastError = err
		}
		return err
	}
	r.lastError = nil
	return nil
}

func (r *RouteBridge) handleToolCall(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return invoke(r, ctx, func(ctx context.Context, backend Backend) (*mcp.CallToolResult, error) {
		return backend.CallTool(ctx, request)
	})
}

func (r *RouteBridge) handleReadResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	result, err := invoke(r, ctx, func(ctx context.Context, backend Backend) (*mcp.ReadResourceResult, error) {
		return backend.ReadResource(ctx, request)
	})
	if err != nil {
		return nil, err
	}
	return result.Contents, nil
}

func (r *RouteBridge) handleGetPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return invoke(r, ctx, func(ctx context.Context, backend Backend) (*mcp.GetPromptResult, error) {
		return backend.GetPrompt(ctx, request)
	})
}

func invoke[T any](r *RouteBridge, ctx context.Context, fn func(context.Context, Backend) (T, error)) (T, error) {
	if err := r.ensureReady(ctx); err != nil {
		var zero T
		return zero, err
	}
	return invokeBackend(r, ctx, fn)
}

func invokeBackend[T any](r *RouteBridge, ctx context.Context, fn func(context.Context, Backend) (T, error)) (T, error) {
	var zero T

	for attempt := 0; attempt < 2; attempt++ {
		backend := r.backendSnapshot()
		if backend == nil {
			if err := r.ensureReady(ctx); err != nil {
				return zero, err
			}
			continue
		}

		callCtx, cancel := context.WithTimeout(ctx, r.route.Timeout)
		r.callMu.Lock()
		result, err := fn(callCtx, backend)
		r.callMu.Unlock()
		cancel()

		if err == nil {
			r.clearLastError()
			return result, nil
		}
		if !isRecoverableTransportError(err) {
			r.setLastError(err)
			return zero, err
		}

		r.logger.Warn("recoverable backend error, restarting", "error", err)
		if closeErr := r.invalidateBackend(backend, err); closeErr != nil {
			r.logger.Warn("close backend after transport error", "error", closeErr)
		}
		if err := r.ensureReady(ctx); err != nil {
			return zero, err
		}
	}

	return zero, fmt.Errorf("backend %q failed after retry", r.route.Name)
}

func (r *RouteBridge) invalidateBackend(current Backend, cause error) error {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	if r.backend != current {
		return nil
	}
	return r.closeBackendLocked(cause)
}

func (r *RouteBridge) closeBackendLocked(cause error) error {
	var err error
	if r.backend != nil {
		err = r.backend.Close()
	}
	r.backend = nil
	r.initResult = nil
	if cause != nil {
		r.lastError = cause
	}
	return err
}

func (r *RouteBridge) backendSnapshot() Backend {
	r.stateMu.RLock()
	defer r.stateMu.RUnlock()
	return r.backend
}

func (r *RouteBridge) handlerSnapshot() http.Handler {
	r.stateMu.RLock()
	defer r.stateMu.RUnlock()
	return r.handler
}

func (r *RouteBridge) setLastError(err error) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	r.lastError = err
}

func (r *RouteBridge) clearLastError() {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	r.lastError = nil
}

func isRecoverableTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "transport closed"),
		strings.Contains(text, "broken pipe"),
		strings.Contains(text, "connection reset"),
		strings.Contains(text, "connection refused"),
		strings.Contains(text, "eof"),
		strings.Contains(text, "unexpected shutdown"):
		return true
	default:
		return false
	}
}
