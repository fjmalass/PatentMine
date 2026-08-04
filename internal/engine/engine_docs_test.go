package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"patentmine/internal/proto"
	"patentmine/internal/store"
)

func TestDocsListAndGet(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Root Readme\nbody")
	mustWriteFile(t, filepath.Join(root, "CHANGELOG.md"), "# Changes\nbody")
	mustWriteFile(t, filepath.Join(root, "docs", "00_HOW_TO_TUI.md"), "# TUI Guide\nbody")
	mustWriteFile(t, filepath.Join(root, "docs", "nested", "ignore.md"), "# Ignore\nbody")
	mustWriteFile(t, filepath.Join(root, "docs", "ignore.txt"), "not markdown")

	eng := New(context.Background(), nil, nil, WithDocsDir(root))
	defer eng.Close()

	list, err := eng.ListDocs(context.Background())
	if err != nil {
		t.Fatalf("ListDocs: %v", err)
	}
	got := make([]string, 0, len(list.Docs))
	for _, doc := range list.Docs {
		got = append(got, doc.ID)
	}
	want := []string{"README.md", "CHANGELOG.md", "docs/00_HOW_TO_TUI.md"}
	if len(got) != len(want) {
		t.Fatalf("doc IDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("doc IDs = %v, want %v", got, want)
		}
	}

	res, err := eng.GetDoc(context.Background(), proto.DocsGetParams{ID: "README"})
	if err != nil {
		t.Fatalf("GetDoc README: %v", err)
	}
	if res.Doc.ID != "README.md" || res.Doc.Title != "Root Readme" || res.Mode != "preview" {
		t.Fatalf("GetDoc README = %+v", res)
	}

	if _, err := eng.GetDoc(context.Background(), proto.DocsGetParams{ID: "docs/nested/ignore.md"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetDoc nested err = %v, want ErrNotFound", err)
	}
	if _, err := eng.GetDoc(context.Background(), proto.DocsGetParams{ID: "../README.md"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetDoc traversal err = %v, want ErrNotFound", err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
