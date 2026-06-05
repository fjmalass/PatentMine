package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"patentmine/internal/api"
	"patentmine/internal/command"
	"patentmine/internal/config"
	"patentmine/internal/rpc"
	appversion "patentmine/internal/version"
)

const (
	envAPIAddr     = "PATENTMINE_API_ADDR"
	defaultAPIAddr = "127.0.0.1:18080"
)

// runAPI starts the web API. Like the TUI it is a thin client that forwards to
// the daemon; it holds no database of its own.
func runAPI(args []string) int {
	flags := flag.NewFlagSet("api", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	addrFlag := flags.String("addr", "", "HTTP listen address")
	webDirFlag := flags.String("web-dir", "", "directory with built SPA (served at /ui/)")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "usage: patentmine api [--addr host:port] [--web-dir path]")
		fmt.Fprintln(flags.Output(), "  env: PATENTMINE_API_TOKEN, PATENTMINE_API_CORS_ORIGINS, PATENTMINE_API_TLS_CERT/KEY, PATENTMINE_WEB_DIR")
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return 2
	}

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
	telemetry.Logger.InfoContext(ctx, "api starting", slog.String("socket_path", string(cfg.SocketPath)))

	client, err := rpc.Dial(string(cfg.SocketPath))
	if err != nil {
		telemetry.Logger.ErrorContext(ctx, "dial daemon failed", slog.String("socket_path", string(cfg.SocketPath)), slog.String("error", err.Error()))
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

	addr := *addrFlag
	if addr == "" {
		addr = os.Getenv(envAPIAddr)
		if addr == "" {
			addr = defaultAPIAddr
		}
	}

	webDir := *webDirFlag
	if webDir == "" {
		webDir = os.Getenv("PATENTMINE_WEB_DIR")
	}
	sec := api.SecurityFromEnv()
	opts := []api.Option{
		api.WithActivity(telemetry, time.Duration(cfg.ActivityMinMS)*time.Millisecond),

		api.WithSecurity(sec),
		api.WithAI(api.AIConfigFromAppConfig(cfg.AIProvider, cfg.GeminiAPIKey, cfg.OllamaHost, cfg.OllamaModel, cfg.OpenAIAPIKey, cfg.OpenAIModel)),
	}
	if webDir != "" {
		opts = append(opts, api.WithWebDir(webDir))
	}
	scheme := "http"
	if sec.TLSCert != "" && sec.TLSKey != "" {
		scheme = "https"
	}
	fmt.Printf("patentmine api %s listening on %s://%s\n", appversion.String(), scheme, addr)
	if webDir != "" {
		fmt.Printf("  web UI: %s://%s/ui/\n", scheme, addr)
	}
	telemetry.Logger.InfoContext(ctx, "api listening", slog.String("addr", addr))
	if err := api.NewServer(client, registry, opts...).ListenAndServe(ctx, addr); err != nil {
		telemetry.Logger.ErrorContext(ctx, "api serve failed", slog.String("error", err.Error()))
		return fail(err)
	}
	fmt.Println("patentmine web API stopped")
	telemetry.Logger.InfoContext(ctx, "api stopped")
	return 0
}
