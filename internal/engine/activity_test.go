// Package engine activity tests. These checks verify that engine mutations emit
// semantic activity records suitable for later replay or undo/redo tooling.
package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/store/sqlite"
)

func TestEngineWritesSemanticActivityRecords(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "logs")
	obs, err := observability.Open(logsDir, "test", "test-version")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = obs.Close() })

	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "engine.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	eng := New(ctx, repo, nil, WithActivityRecorder(obs.Activity), WithLogger(obs.Logger))
	t.Cleanup(eng.Close)

	patent := domain.Patent{Number: domain.MustParsePatentNumber("US11611785B2"), FetchState: domain.FetchCached}
	if err := eng.SavePatent(context.Background(), patent); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	if err := repo.SaveDocument(context.Background(), patent.Number, domain.Document{
		Number: patent.Number,
		Stage:  domain.GuessStage(patent.Number),
	}); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	project, err := eng.CreateProject(context.Background(), "P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := eng.AddToProject(context.Background(), project.ID, patent.Number); err != nil {
		t.Fatalf("AddToProject: %v", err)
	}
	if err := eng.SetReviewState(context.Background(), project.ID, []domain.PatentNumber{patent.Number}, domain.ReviewStateUnderReview); err != nil {
		t.Fatalf("SetReviewState: %v", err)
	}

	date := time.Now().In(time.Local).Format("2006-01-02")
	body, err := os.ReadFile(filepath.Join(logsDir, "activity-"+date+".jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, action := range []string{`"action":"patent.save"`, `"action":"project.create"`, `"action":"membership.add"`, `"action":"membership.set_state"`, `"mutation_group_id"`} {
		if !strings.Contains(string(body), action) {
			t.Fatalf("activity log missing %s: %s", action, body)
		}
	}
}
