package tui

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/storage"
	sqliterepo "patentmine/internal/storage/sqlite"
)

func TestLiveUSPTOTUIAddAndRefreshCommandsViaDotEnv(t *testing.T) {
	if os.Getenv("PATENTMINE_LIVE_USPTO") != "1" {
		t.Skip("set PATENTMINE_LIVE_USPTO=1 to run the live USPTO ODP TUI command smoke test")
	}
	root, err := findLiveTUIRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatal(err)
		}
	})

	cfg := config.Load()
	cfg.ImportSource = config.ImportSourceUSPTO
	if cfg.USPTO.APIKey == "" {
		t.Fatal("USPTO API key was not loaded from .env or environment")
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo, err := sqliterepo.Open(filepath.Join(t.TempDir(), "live-tui.sqlite"), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Fatal(err)
		}
	})
	if err := repo.Setup(ctx); err != nil {
		t.Fatal(err)
	}

	model := New(ctx, repo, logger, logger, cfg, "live-test")
	model = runLiveTUICommand(t, model, ParseCommand(":add US8164048B2"))
	if model.err != "" {
		t.Fatalf("unexpected TUI error after add: %s", model.err)
	}
	if model.current.Number == "" || model.current.Title == "" || model.current.ImportSource != ImportSourceUSPTO {
		t.Fatalf("expected USPTO-backed current patent after add, got %+v", model.current)
	}
	if model.mode != viewDetail {
		t.Fatalf("expected detail mode after add, got %q", model.mode)
	}
	if !strings.Contains(model.message, "via USPTO ODP") {
		t.Fatalf("expected add confirmation message to mention USPTO ODP, got %q", model.message)
	}

	cited, err := repo.ListCitations(ctx, model.ProjectID, model.current.Number, domain.RelationCites, storage.ListCitationsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	citing, err := repo.ListCitations(ctx, model.ProjectID, model.current.Number, domain.RelationCitedBy, storage.ListCitationsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cited)+len(citing) == 0 {
		t.Fatalf("expected live TUI add to store citations, cited=%d citing=%d", len(cited), len(citing))
	}

	model = runLiveTUICommand(t, model, ParseCommand(":refresh"))
	if model.err != "" {
		t.Fatalf("unexpected TUI error after refresh: %s", model.err)
	}
	if model.current.Number == "" || model.current.ImportSource != ImportSourceUSPTO {
		t.Fatalf("expected USPTO-backed current patent after refresh, got %+v", model.current)
	}
	if !strings.Contains(model.message, "refresh complete:") {
		t.Fatalf("expected refresh confirmation message, got %q", model.message)
	}
}

func runLiveTUICommand(t *testing.T, model *Model, command Command) *Model {
	t.Helper()
	updated, cmd := model.runCommand(command)
	next, ok := updated.(*Model)
	if !ok {
		t.Fatalf("expected *Model from command %q, got %T", command.Name, updated)
	}
	msgs := collectLiveTUIMessages(t, cmd)
	handled := false
	for _, msg := range msgs {
		switch msg := msg.(type) {
		case refreshResultMsg:
			handled = true
			updated, cmd := next.Update(msg)
			next = updated.(*Model)
			for _, followup := range collectLiveTUIMessages(t, cmd) {
				if _, ok := followup.(refreshDetailsResultMsg); ok {
					updated, _ = next.Update(followup)
					next = updated.(*Model)
				}
			}
		case refreshDetailsResultMsg:
			handled = true
			updated, _ := next.Update(msg)
			next = updated.(*Model)
		}
	}
	if !handled {
		t.Fatalf("command %q did not produce a refresh result", command.Name)
	}
	return next
}

func collectLiveTUIMessages(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := runLiveTUICommandFunc(t, cmd)
	switch msg := msg.(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, collectLiveTUIMessages(t, c)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

func runLiveTUICommandFunc(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	ch := make(chan tea.Msg, 1)
	go func() {
		ch <- cmd()
	}()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(3 * time.Minute):
		t.Fatal("timed out waiting for live TUI command")
		return nil
	}
}

func findLiveTUIRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
