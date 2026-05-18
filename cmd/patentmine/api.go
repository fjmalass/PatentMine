package main

import (
	"context"
	"fmt"
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

	client, err := rpc.Dial(cfg.SocketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "patentmine: cannot reach the daemon at %s\n", cfg.SocketPath)
		fmt.Fprintln(os.Stderr, "start it first in another terminal:  patentmine serve")
		return 1
	}
	defer func() { _ = client.Close() }()

	registry, err := command.Default()
	if err != nil {
		return fail(err)
	}

	addr := os.Getenv(envAPIAddr)
	if addr == "" {
		addr = defaultAPIAddr
	}

	fmt.Printf("patentmine web API listening on http://%s\n", addr)
	if err := api.NewServer(client, registry).ListenAndServe(ctx, addr); err != nil {
		return fail(err)
	}
	fmt.Println("patentmine web API stopped")
	return 0
}
