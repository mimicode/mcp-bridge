package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/mimicode/mcp_bridge/internal/buildinfo"
	"github.com/mimicode/mcp_bridge/internal/config"
	"github.com/mimicode/mcp_bridge/internal/ui"
)

type App struct {
	logger     *slog.Logger
	factory    BackendFactory
	configPath string
	basePath   string
	uiHandler  http.Handler

	mu     sync.RWMutex
	routes map[string]*RouteBridge
	order  []*RouteBridge
}

func NewApp(cfg *config.Runtime, logger *slog.Logger, factory BackendFactory) *App {
	uiHandler, err := ui.Handler()
	if err != nil {
		panic(err)
	}

	app := &App{
		logger:     logger,
		factory:    factory,
		configPath: cfg.SourcePath,
		basePath:   cfg.BasePath,
		uiHandler:  uiHandler,
		routes:     make(map[string]*RouteBridge, len(cfg.Servers)),
	}
	for _, route := range cfg.Servers {
		bridge := NewRouteBridge(route, logger, factory)
		app.routes[route.Path] = bridge
		app.order = append(app.order, bridge)
	}
	sortRoutes(app.order)
	return app
}

func (a *App) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch {
	case req.URL.Path == "/":
		a.handleIndex(w, req)
		return
	case req.URL.Path == "/healthz":
		a.handleHealth(w, req)
		return
	case req.URL.Path == "/_admin/api/config":
		a.handleConfigAPI(w, req)
		return
	case req.URL.Path == "/_admin/api/test":
		a.handleTestAPI(w, req)
		return
	case req.URL.Path == "/_admin":
		http.Redirect(w, req, "/_admin/", http.StatusPermanentRedirect)
		return
	case strings.HasPrefix(req.URL.Path, "/_admin/"):
		http.StripPrefix("/_admin/", a.uiHandler).ServeHTTP(w, req)
		return
	}

	if bridge, canonical, ok := a.lookupRoute(req.URL.Path); ok {
		if canonical != req.URL.Path {
			http.Redirect(w, req, canonical, http.StatusPermanentRedirect)
			return
		}
		bridge.Handler().ServeHTTP(w, req)
		return
	}

	http.NotFound(w, req)
}

func (a *App) Warmup(ctx context.Context) error {
	var joined error
	for _, route := range a.routeSnapshot() {
		if err := route.Warmup(ctx); err != nil {
			a.logger.Warn("route warmup failed", "route", route.route.Name, "path", route.route.Path, "error", err)
			joined = errors.Join(joined, err)
		}
	}
	return joined
}

func (a *App) Close() error {
	var joined error
	for _, route := range a.routeSnapshot() {
		if err := route.Close(); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	return joined
}

func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	ready := true
	for _, route := range a.routeSnapshot() {
		if !route.Info().Ready {
			ready = false
			break
		}
	}

	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ready":  ready,
		"routes": a.routeInfos(),
	})
}

func (a *App) handleIndex(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/" {
		http.NotFound(w, req)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":      bridgeName,
		"version":   buildinfo.Current(),
		"health":    "/healthz",
		"routes":    a.routeInfos(),
		"admin_api": "/_admin/api/config",
	})
}

func (a *App) routeInfos() []RouteInfo {
	routes := a.routeSnapshot()
	infos := make([]RouteInfo, 0, len(routes))
	for _, route := range routes {
		infos = append(infos, route.Info())
	}
	return infos
}

func (a *App) Reload(ctx context.Context, cfg *config.Runtime) error {
	oldRoutes := a.routeMapSnapshot()
	nextMap := make(map[string]*RouteBridge, len(cfg.Servers))
	nextOrder := make([]*RouteBridge, 0, len(cfg.Servers))
	keep := make(map[string]struct{}, len(cfg.Servers))

	for _, routeCfg := range cfg.Servers {
		if current, ok := oldRoutes[routeCfg.Path]; ok && current.route.Equal(routeCfg) {
			nextMap[routeCfg.Path] = current
			nextOrder = append(nextOrder, current)
			keep[routeCfg.Path] = struct{}{}
			continue
		}

		bridge := NewRouteBridge(routeCfg, a.logger, a.factory)
		nextMap[routeCfg.Path] = bridge
		nextOrder = append(nextOrder, bridge)
	}

	sortRoutes(nextOrder)

	a.mu.Lock()
	a.routes = nextMap
	a.order = nextOrder
	a.configPath = cfg.SourcePath
	a.basePath = cfg.BasePath
	a.mu.Unlock()

	var joined error
	for _, route := range nextOrder {
		if _, exists := keep[route.route.Path]; exists {
			continue
		}
		if err := route.Warmup(ctx); err != nil {
			joined = errors.Join(joined, err)
		}
	}

	for path, route := range oldRoutes {
		if _, exists := keep[path]; exists {
			continue
		}
		if err := route.Close(); err != nil {
			joined = errors.Join(joined, err)
		}
	}

	return joined
}

func (a *App) lookupRoute(path string) (*RouteBridge, string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if route, ok := a.routes[path]; ok {
		return route, path, true
	}
	if path != "/" && strings.HasSuffix(path, "/") {
		canonical := strings.TrimRight(path, "/")
		if route, ok := a.routes[canonical]; ok {
			return route, canonical, true
		}
	}
	return nil, "", false
}

func (a *App) routeSnapshot() []*RouteBridge {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]*RouteBridge(nil), a.order...)
}

func (a *App) routeMapSnapshot() map[string]*RouteBridge {
	a.mu.RLock()
	defer a.mu.RUnlock()

	cloned := make(map[string]*RouteBridge, len(a.routes))
	for path, route := range a.routes {
		cloned[path] = route
	}
	return cloned
}

func (a *App) handleConfigAPI(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		a.handleGetConfig(w)
	case http.MethodPost:
		a.handleSaveConfig(w, req)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleTestAPI(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("read request body failed: %v", err),
		})
		return
	}

	var payload struct {
		Name        string            `json:"name"`
		Command     string            `json:"command"`
		Args        []string          `json:"args"`
		Env         map[string]string `json:"env"`
		Description string            `json:"description"`
		Timeout     int               `json:"timeout"`
		Path        string            `json:"path"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("parse request failed: %v", err),
		})
		return
	}

	serverName := strings.TrimSpace(payload.Name)
	if serverName == "" {
		serverName = "test-server"
	}

	cfg, err := config.File{
		MCPServers: map[string]config.Server{
			serverName: {
				Enabled:     true,
				Command:     payload.Command,
				Args:        append([]string(nil), payload.Args...),
				Env:         payload.Env,
				Description: payload.Description,
				TimeoutMS:   payload.Timeout,
				Path:        payload.Path,
			},
		},
	}.Normalize(a.configFilePath(), a.currentBasePath())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": err.Error(),
		})
		return
	}

	testCtx, cancel := context.WithTimeout(req.Context(), 2*config.DefaultTimeout)
	info, testErr := TestRoute(testCtx, cfg.Servers[0], a.logger.With("test", serverName), a.factory)
	cancel()

	response := map[string]any{
		"ok":      testErr == nil,
		"info":    info,
		"version": buildinfo.Current(),
	}
	if testErr != nil {
		response["error"] = testErr.Error()
		writeJSON(w, http.StatusBadGateway, response)
		return
	}
	response["message"] = "MCP 启动测试成功"
	writeJSON(w, http.StatusOK, response)
}

func (a *App) handleGetConfig(w http.ResponseWriter) {
	content, err := os.ReadFile(a.configFilePath())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("read config failed: %v", err),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"configPath": a.configFilePath(),
		"content":    string(content),
		"routes":     a.routeInfos(),
		"version":    buildinfo.Current(),
	})
}

func (a *App) handleSaveConfig(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("read request body failed: %v", err),
		})
		return
	}

	var payload struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("parse request failed: %v", err),
		})
		return
	}

	formatted, err := config.Format([]byte(payload.Content))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": err.Error(),
		})
		return
	}

	cfg, err := config.Parse(formatted, a.configFilePath(), a.currentBasePath())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": err.Error(),
		})
		return
	}

	if err := writeFileAtomically(a.configFilePath(), formatted); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("write config failed: %v", err),
		})
		return
	}

	reloadCtx, cancel := context.WithTimeout(req.Context(), 2*config.DefaultTimeout)
	reloadErr := a.Reload(reloadCtx, cfg)
	cancel()

	response := map[string]any{
		"message":    "配置已保存，MCP 配置已重新加载",
		"content":    string(formatted),
		"routes":     a.routeInfos(),
		"configPath": a.configFilePath(),
		"reloaded":   reloadErr == nil,
		"version":    buildinfo.Current(),
	}
	if reloadErr != nil {
		response["message"] = "配置已保存，但部分 MCP 热更新失败，可查看各路由错误信息"
		response["warning"] = reloadErr.Error()
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) configFilePath() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.configPath
}

func (a *App) currentBasePath() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.basePath
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeFileAtomically(path string, content []byte) error {
	dir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(dir, "mcp-bridge-config-*.json")
	if err != nil {
		return err
	}

	tempPath := tempFile.Name()
	cleanup := func() {
		_ = os.Remove(tempPath)
	}

	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		cleanup()
		return err
	}
	if err := tempFile.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

func sortRoutes(routes []*RouteBridge) {
	slices.SortFunc(routes, func(a *RouteBridge, b *RouteBridge) int {
		switch {
		case a.route.Path < b.route.Path:
			return -1
		case a.route.Path > b.route.Path:
			return 1
		default:
			return 0
		}
	})
}
