package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"patentmine/internal/config"
	"patentmine/internal/engine"
	"patentmine/internal/ingest"
	"patentmine/internal/observability"
	"patentmine/internal/rpc"
	"patentmine/internal/store/sqlite"
	appversion "patentmine/internal/version"
)

// patentsDirName holds local JSON patent files consulted before web sources.
const patentsDirName = "patents"

// runServe starts the engine daemon: it owns the database and serves every
// thin client over a unix socket until it receives an interrupt.
func runServe(_ []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	telemetry, err := openObservability(cfg, "daemon")
	if err != nil {
		return fail(err)
	}
	defer func() { _ = telemetry.Close() }()
	fmt.Printf("patentmine daemon %s starting\n", appversion.String())
	telemetry.Logger.InfoContext(ctx, "daemon starting",
		slog.String("build_version", appversion.String()),
		slog.String("db_path", cfg.DBPath),
		slog.String("socket_path", cfg.SocketPath),
		slog.String("logs_dir", cfg.LogsDir))

	repo, err := sqlite.OpenWithMetrics(ctx, cfg.DBPath, telemetry.Metrics)
	if err != nil {
		telemetry.Logger.ErrorContext(ctx, "open sqlite failed", slog.String("error", err.Error()))
		return fail(err)
	}
	defer func() { _ = repo.Close() }()

	eng, err := buildEngine(ctx, cfg, repo, telemetry)
	if err != nil {
		telemetry.Logger.ErrorContext(ctx, "build engine failed", slog.String("error", err.Error()))
		return fail(err)
	}
	defer eng.Close()

	if err := writePIDFile(cfg.PIDPath); err != nil {
		telemetry.Logger.ErrorContext(ctx, "write pid file failed", slog.String("pid_path", cfg.PIDPath), slog.String("error", err.Error()))
		return fail(err)
	}
	defer func() { _ = os.Remove(cfg.PIDPath) }()

	// Clear a socket left behind by a previous run before binding.
	if err := os.Remove(cfg.SocketPath); err != nil && !os.IsNotExist(err) {
		telemetry.Logger.ErrorContext(ctx, "remove stale socket failed", slog.String("socket_path", cfg.SocketPath), slog.String("error", err.Error()))
		return fail(err)
	}
	ln, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		telemetry.Logger.ErrorContext(ctx, "listen failed", slog.String("socket_path", cfg.SocketPath), slog.String("error", err.Error()))
		return fail(err)
	}
	defer func() { _ = os.Remove(cfg.SocketPath) }()

	fmt.Printf("patentmine daemon %s listening on %s\n", appversion.String(), cfg.SocketPath)
	telemetry.Logger.InfoContext(ctx, "daemon listening", slog.String("socket_path", cfg.SocketPath))
	if err := rpc.NewServer(eng).Serve(ctx, ln); err != nil {
		telemetry.Logger.ErrorContext(ctx, "rpc server stopped with error", slog.String("error", err.Error()))
		return fail(err)
	}
	fmt.Println("patentmine daemon stopped")
	telemetry.Logger.InfoContext(ctx, "daemon stopped")
	return 0
}

// buildEngine assembles the ingest pipeline and the engine. The default source
// registry is file-only; web sources are added once their parsers land.
func buildEngine(ctx context.Context, cfg config.Config, repo *sqlite.Repo, telemetry *observability.Runtime) (*engine.Engine, error) {
	patentsDir := filepath.Join(cfg.HomeDir, patentsDirName)
	if err := os.MkdirAll(patentsDir, 0o755); err != nil {
		return nil, err
	}
	registry := ingest.NewRegistry(ingest.NewFileSource(patentsDir)).WithMetrics(telemetry.Metrics).WithLogger(telemetry.Logger)
	crawler := ingest.NewCrawler(registry, repo, ingest.CrawlConfig{}).WithMetrics(telemetry.Metrics).WithLogger(telemetry.Logger)
	return engine.New(ctx, repo, ingest.Factory(crawler),
		engine.WithLogger(telemetry.Logger),
		engine.WithActivityRecorder(telemetry.Activity),
		engine.WithMetrics(telemetry.Metrics)), nil
}

// fail prints err and returns the failure exit code.
func fail(err error) int {
	fmt.Fprintln(os.Stderr, "patentmine:", err)
	return 1
}

func writePIDFile(path string) error {
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
}
