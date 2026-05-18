package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"patentmine/internal/api"
	"patentmine/internal/command"
	"patentmine/internal/config"
	"patentmine/internal/rpc"
)

const (
	envAPIAddr     = "PATENTMINE_API_ADDR"
	defaultAPIAddr = "127.0.0.1:8080"
)

// runAPI starts the web API. Like the TUI it is a thin client that forwards to
// the daemon; it holds no database of its own.
func runAPI(_ []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	telemetry, err := openObservability(cfg, "api")
	if err != nil {
		return fail(err)
	}
	defer func() { _ = telemetry.Close() }()
	telemetry.Logger.InfoContext(ctx, "api starting", slog.String("socket_path", cfg.SocketPath))

	client, err := rpc.Dial(cfg.SocketPath)
	if err != nil {
		telemetry.Logger.ErrorContext(ctx, "dial daemon failed", slog.String("socket_path", cfg.SocketPath), slog.String("error", err.Error()))
		fmt.Fprintf(os.Stderr, "patentmine: cannot reach the daemon at %s\n", cfg.SocketPath)
		fmt.Fprintln(os.Stderr, "start it first in another terminal:  patentmine serve")
		return 1
	}
	defer func() { _ = client.Close() }()

	registry, err := command.Default()
	if err != nil {
		telemetry.Logger.ErrorContext(ctx, "build command registry failed", slog.String("error", err.Error()))
		return fail(err)
	}

	addr := os.Getenv(envAPIAddr)
	if addr == "" {
		addr = defaultAPIAddr
	}

	fmt.Printf("patentmine web API listening on http://%s\n", addr)
	telemetry.Logger.InfoContext(ctx, "api listening", slog.String("addr", addr))
	if err := api.NewServer(client, registry).ListenAndServe(ctx, addr); err != nil {
		telemetry.Logger.ErrorContext(ctx, "api serve failed", slog.String("error", err.Error()))
		return fail(err)
	}
	fmt.Println("patentmine web API stopped")
	telemetry.Logger.InfoContext(ctx, "api stopped")
	return 0
}
