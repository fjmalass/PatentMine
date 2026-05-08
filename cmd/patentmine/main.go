package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/importer"
	"patentmine/internal/logging"
	sqliterepo "patentmine/internal/storage/sqlite"
	"patentmine/internal/tui"
)

func main() {
	ctx := context.Background()
	logger, closeLog, err := logging.Open("./patentmine.log")
	if err != nil {
		exit(err)
	}
	defer closeLog()
	logger.Info("starting patentmine")
	repo, err := sqliterepo.Open("./patentmine.db")
	if err != nil {
		logger.Error("open sqlite failed", "error", err)
		exit(err)
	}
	defer repo.Close()
	if err := repo.Setup(ctx); err != nil {
		logger.Error("sqlite setup failed", "error", err)
		exit(err)
	}
	if err := seedFixture(ctx, repo); err != nil {
		logger.Error("fixture seed failed", "error", err)
		exit(err)
	}
	model := tui.New(ctx, repo, logger)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		logger.Error("tui failed", "error", err)
		exit(err)
	}
	logger.Info("stopped patentmine")
}

func seedFixture(ctx context.Context, repo *sqliterepo.Repository) error {
	bundle, err := importer.LoadFixture("fixtures/US11611785B2.json")
	if err != nil {
		return err
	}
	existing, err := repo.GetPatent(ctx, "US11611785B2")
	if err == nil && existing.ExpirationDate != "" && existing.ExpirationEstimated == bundle.Patent.ExpirationEstimated {
		return nil
	}
	return repo.UpsertPatentBundle(ctx, bundle)
}

func exit(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
