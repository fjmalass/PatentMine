package main

import (
	"context"
	"fmt"

	"patentmine/internal/config"
	"patentmine/internal/store/sqlite"
)

// runMigrate opens the database, which applies any pending migrations, then
// exits. It is handy in CI and first-run setup before starting the daemon.
func runMigrate(_ []string) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	repo, err := sqlite.Open(context.Background(), cfg.DBPath)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = repo.Close() }()

	fmt.Printf("patentmine: database migrated at %s\n", cfg.DBPath)
	return 0
}
