package bridge

import (
	"context"
	"log/slog"

	"github.com/mimicode/mcp_bridge/internal/config"
)

func TestRoute(ctx context.Context, route config.Route, logger *slog.Logger, factory BackendFactory) (RouteInfo, error) {
	testBridge := NewRouteBridge(route, logger, factory)
	defer func() {
		_ = testBridge.Close()
	}()

	err := testBridge.Warmup(ctx)
	return testBridge.Info(), err
}
