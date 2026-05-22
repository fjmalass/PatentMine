package importer

import (
	"os"
	"path/filepath"
	"testing"

	"patentmine/internal/config"
)

func TestLiveUSPTOImportViaDotEnv(t *testing.T) {
	if os.Getenv("PATENTMINE_LIVE_USPTO") != "1" {
		t.Skip("set PATENTMINE_LIVE_USPTO=1 to run the live USPTO ODP smoke test")
	}
	root, err := findRepoRoot()
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
	bundle, err := ImportUSPTO("US8164048B2", cfg.USPTO.APIKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Patent.Number == "" || bundle.Patent.Title == "" {
		t.Fatalf("expected imported patent metadata, got number=%q title=%q", bundle.Patent.Number, bundle.Patent.Title)
	}
	if len(bundle.Citations) == 0 {
		t.Fatal("expected live import to return citation edges")
	}
}

func findRepoRoot() (string, error) {
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
