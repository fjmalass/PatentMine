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
	"patentmine/internal/crawl"
	"patentmine/internal/observability"
	"patentmine/internal/rpc"
	"patentmine/internal/store"
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
	reportPaths(os.Stdout, cfg)
	telemetry.Logger.InfoContext(ctx, "daemon starting",
		slog.String("build_version", appversion.String()),
		slog.String("db_path", string(cfg.DBPath)),
		slog.String("socket_path", string(cfg.SocketPath)),
		slog.String("logs_dir", string(cfg.LogsDir)))

	repo, err := sqlite.OpenWithMetrics(ctx, string(cfg.DBPath), telemetry.Metrics)
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
		telemetry.Logger.ErrorContext(ctx, "write pid file failed", slog.String("pid_path", string(cfg.PIDPath)), slog.String("error", err.Error()))
		return fail(err)
	}
	defer func() { _ = os.Remove(string(cfg.PIDPath)) }()

	// Clear a socket left behind by a previous run before binding.
	if err := os.Remove(string(cfg.SocketPath)); err != nil && !os.IsNotExist(err) {
		telemetry.Logger.ErrorContext(ctx, "remove stale socket failed", slog.String("socket_path", string(cfg.SocketPath)), slog.String("error", err.Error()))
		return fail(err)
	}
	ln, err := net.Listen("unix", string(cfg.SocketPath))
	if err != nil {
		telemetry.Logger.ErrorContext(ctx, "listen failed", slog.String("socket_path", string(cfg.SocketPath)), slog.String("error", err.Error()))
		return fail(err)
	}
	defer func() { _ = os.Remove(string(cfg.SocketPath)) }()

	fmt.Printf("patentmine daemon %s listening on %s\n", appversion.String(), cfg.SocketPath)
	telemetry.Logger.InfoContext(ctx, "daemon listening", slog.String("socket_path", string(cfg.SocketPath)))
	if err := rpc.NewServer(eng).Serve(ctx, ln); err != nil {
		telemetry.Logger.ErrorContext(ctx, "rpc server stopped with error", slog.String("error", err.Error()))
		return fail(err)
	}
	fmt.Println("patentmine daemon stopped")
	telemetry.Logger.InfoContext(ctx, "daemon stopped")
	return 0
}

// buildEngine assembles the ingest pipeline and the engine.
func buildEngine(ctx context.Context, cfg config.Config, repo *sqlite.Repo, telemetry *observability.Runtime) (*engine.Engine, error) {
	patentsDir := filepath.Join(string(cfg.HomeDir), patentsDirName)
	if err := os.MkdirAll(patentsDir, 0o755); err != nil {
		return nil, err
	}
	registry := crawl.NewRegistry(
		crawl.NewFileSource(patentsDir),
		crawl.NewGoogleSource(),
	).WithMetrics(telemetry.Metrics).WithLogger(telemetry.Logger)
	cachingRepo := store.NewCache(repo)
	crawler := crawl.NewCrawler(registry, cachingRepo, crawl.CrawlConfig{}).WithMetrics(telemetry.Metrics).WithLogger(telemetry.Logger)
	return engine.New(ctx, cachingRepo, crawl.Factory(crawler),
		engine.WithFileImporter(crawler),
		engine.WithLogger(telemetry.Logger),
		engine.WithActivityRecorder(telemetry.Activity),
		engine.WithMetrics(telemetry.Metrics)), nil
}

// fail prints err and returns the failure exit code.
func fail(err error) int {
	fmt.Fprintln(os.Stderr, "patentmine:", err)
	return 1
}

func writePIDFile(path config.Path) error {
	return os.WriteFile(string(path), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
}
