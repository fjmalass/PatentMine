package tui

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/importer"
	"patentmine/internal/storage"
	sqliterepo "patentmine/internal/storage/sqlite"
)

func TestLiveUSPTOCitationTargetRefreshPush(t *testing.T) {
	if os.Getenv("PATENTMINE_LIVE_USPTO_PUSH") != "1" {
		t.Skip("set PATENTMINE_LIVE_USPTO_PUSH=1 to run the deeper live USPTO citation-target push test")
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
	if cfg.USPTO.APIKey == "" {
		t.Fatal("USPTO API key was not loaded from .env or environment")
	}
	limit := livePushLimit()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo, err := sqliterepo.Open(filepath.Join(t.TempDir(), "live-push.sqlite"), logger)
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

	started := time.Now()
	rootBundle, err := importer.ImportUSPTO("US8164048B2", cfg.USPTO.APIKey, logger)
	if err != nil {
		t.Fatal(err)
	}
	rootBundle.Patent.ImportSource = ImportSourceUSPTO
	rootBundle.Patent.ReviewState = domain.ReviewStateStored
	if err := repo.UpsertPatentBundle(ctx, "default", rootBundle); err != nil {
		t.Fatal(err)
	}

	cited, err := repo.ListCitations(ctx, "default", rootBundle.Patent.Number, domain.RelationCites, storage.ListCitationsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	citing, err := repo.ListCitations(ctx, "default", rootBundle.Patent.Number, domain.RelationCitedBy, storage.ListCitationsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	targets := uniqueLivePushTargets(cited, citing)
	if len(targets) == 0 {
		t.Fatal("root patent produced no citation targets to push")
	}
	if limit > len(targets) {
		limit = len(targets)
	}

	var succeeded, importFailed, storageFailed int
	var failures []string
	for _, edge := range targets[:limit] {
		bundle, err := importer.ImportUSPTO(string(edge.TargetPatent), cfg.USPTO.APIKey, logger)
		if err != nil {
			importFailed++
			failures = append(failures, string(edge.TargetPatent)+": import: "+err.Error())
			continue
		}
		bundle = bundleForCitationTarget(bundle, edge.TargetPatent)
		bundle.Patent.ImportSource = ImportSourceUSPTO
		bundle.Patent.ReviewState = domain.ReviewStateCached
		if err := repo.UpsertPatentBundle(ctx, "default", bundle); err != nil {
			storageFailed++
			failures = append(failures, string(edge.TargetPatent)+": storage: "+err.Error())
			continue
		}
		if err := repo.UpdateCitationReviewState(ctx, "default", edge, domain.ReviewStateCached); err != nil {
			storageFailed++
			failures = append(failures, string(edge.TargetPatent)+": review state: "+err.Error())
			continue
		}
		p, err := repo.GetPatent(ctx, "default", edge.TargetPatent)
		if err != nil || p.Title == "" || p.Title == string(edge.TargetPatent) {
			storageFailed++
			failures = append(failures, string(edge.TargetPatent)+": metadata not materialized")
			continue
		}
		succeeded++
	}

	patents, err := repo.ListPatents(ctx, "default", storage.ListPatentsOptions{ReviewStateFilter: storage.ReviewStateFilterNone})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("root=%s stored_as=%s root_cites=%d root_cited_by=%d unique_targets=%d attempted=%d succeeded=%d import_failed=%d storage_failed=%d stored_patents=%d elapsed=%s",
		"US8164048B2", rootBundle.Patent.Number, len(cited), len(citing), len(targets), limit, succeeded, importFailed, storageFailed, len(patents), time.Since(started).Round(time.Second))
	if len(failures) > 0 {
		t.Logf("first_failures=%s", strings.Join(failures[:min(len(failures), 5)], " | "))
	}
	if succeeded == 0 {
		t.Fatalf("expected at least one citation target refresh to succeed; attempted=%d failures=%d", limit, len(failures))
	}
}

func livePushLimit() int {
	value := strings.TrimSpace(os.Getenv("PATENTMINE_LIVE_USPTO_PUSH_LIMIT"))
	if value == "" {
		return 10
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 10
	}
	return n
}

func uniqueLivePushTargets(groups ...[]domain.CitationEdge) []domain.CitationEdge {
	seen := map[domain.PatentNumber]bool{}
	var out []domain.CitationEdge
	for _, group := range groups {
		for _, edge := range group {
			if edge.TargetPatent == "" || seen[edge.TargetPatent] {
				continue
			}
			seen[edge.TargetPatent] = true
			out = append(out, edge)
		}
	}
	return out
}
