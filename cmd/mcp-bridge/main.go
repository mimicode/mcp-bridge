package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mimicode/mcp_bridge/internal/bridge"
	"github.com/mimicode/mcp_bridge/internal/buildinfo"
	"github.com/mimicode/mcp_bridge/internal/config"
)

func main() {
	var (
		configPath = flag.String("config", "config.json", "path to MCP bridge config json")
		listenAddr = flag.String("listen", ":8082", "http listen address")
		basePath   = flag.String("base-path", config.DefaultBasePath, "base path for auto-generated routes")
		printVer   = flag.Bool("version", false, "print build version and exit")
	)
	flag.Parse()

	if *printVer {
		info := buildinfo.Current()
		fmt.Printf("version=%s commit=%s buildTime=%s\n", info.Version, info.Commit, info.BuildTime)
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load(*configPath, *basePath)
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}

	app := bridge.NewApp(cfg, logger, nil)

	warmupCtx, warmupCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	if err := app.Warmup(warmupCtx); err != nil {
		logger.Warn("warmup completed with errors", "error", err)
	}
	warmupCancel()

	server := &http.Server{
		Addr:              *listenAddr,
		Handler:           app,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		info := buildinfo.Current()
		logger.Info("server started", "listen", *listenAddr, "config", cfg.SourcePath, "version", info.Version, "commit", info.Commit, "build_time", info.BuildTime)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	waitForShutdown(logger, server, app)
}

func waitForShutdown(logger *slog.Logger, server *http.Server, app *bridge.App) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	sig := <-stop
	logger.Info("shutdown signal received", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	serverErrCh := make(chan error, 1)
	appErrCh := make(chan error, 1)

	go func() {
		serverErrCh <- server.Shutdown(ctx)
	}()
	go func() {
		appErrCh <- app.Shutdown(ctx)
	}()

	serverErr := <-serverErrCh
	if serverErr != nil {
		logger.Error("graceful http shutdown failed", "error", serverErr)
		if closeErr := server.Close(); closeErr != nil {
			logger.Error("force close http server failed", "error", closeErr)
		}
	}

	if appErr := <-appErrCh; appErr != nil {
		logger.Error("graceful mcp shutdown failed", "error", appErr)
	}

	logger.Info("server stopped", "status", fmt.Sprintf("graceful within %s", 15*time.Second))
}
