package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"patentmine/internal/config"
	"patentmine/internal/engine"
	"patentmine/internal/ingest"
	"patentmine/internal/rpc"
	"patentmine/internal/store/sqlite"
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

	repo, err := sqlite.Open(ctx, cfg.DBPath)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = repo.Close() }()

	eng, err := buildEngine(ctx, cfg, repo)
	if err != nil {
		return fail(err)
	}
	defer eng.Close()

	// Clear a socket left behind by a previous run before binding.
	if err := os.Remove(cfg.SocketPath); err != nil && !os.IsNotExist(err) {
		return fail(err)
	}
	ln, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = os.Remove(cfg.SocketPath) }()

	fmt.Printf("patentmine daemon listening on %s\n", cfg.SocketPath)
	if err := rpc.NewServer(eng).Serve(ctx, ln); err != nil {
		return fail(err)
	}
	fmt.Println("patentmine daemon stopped")
	return 0
}

// buildEngine assembles the ingest pipeline and the engine. The default source
// registry is file-only; web sources are added once their parsers land.
func buildEngine(ctx context.Context, cfg config.Config, repo *sqlite.Repo) (*engine.Engine, error) {
	patentsDir := filepath.Join(cfg.HomeDir, patentsDirName)
	if err := os.MkdirAll(patentsDir, 0o755); err != nil {
		return nil, err
	}
	registry := ingest.NewRegistry(ingest.NewFileSource(patentsDir))
	crawler := ingest.NewCrawler(registry, repo, ingest.CrawlConfig{})
	return engine.New(ctx, repo, ingest.Factory(crawler)), nil
}

// fail prints err and returns the failure exit code.
func fail(err error) int {
	fmt.Fprintln(os.Stderr, "patentmine:", err)
	return 1
}
