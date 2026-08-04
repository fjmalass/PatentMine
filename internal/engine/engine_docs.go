package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"patentmine/internal/proto"
	"patentmine/internal/store"
)

// ListDocs returns the project-level markdown documents visible to clients.
func (e *Engine) ListDocs(ctx context.Context) (proto.DocsListResult, error) {
	if strings.TrimSpace(e.docsDir) == "" {
		return proto.DocsListResult{}, errors.New("engine: docs directory not configured")
	}
	if err := ctx.Err(); err != nil {
		return proto.DocsListResult{}, err
	}

	var docs []proto.DocSummary
	for _, name := range []string{"README.md", "CHANGELOG.md"} {
		path := filepath.Join(e.docsDir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			docs = append(docs, proto.DocSummary{ID: name, Title: markdownTitle(path, name)})
		}
	}

	docsDir := filepath.Join(e.docsDir, "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil && !os.IsNotExist(err) {
		return proto.DocsListResult{}, fmt.Errorf("engine: list docs: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		id := filepath.ToSlash(filepath.Join("docs", entry.Name()))
		docs = append(docs, proto.DocSummary{ID: id, Title: markdownTitle(filepath.Join(docsDir, entry.Name()), entry.Name())})
	}
	slices.SortStableFunc(docs, func(a, b proto.DocSummary) int {
		return strings.Compare(docSortKey(a.ID), docSortKey(b.ID))
	})
	return proto.DocsListResult{Docs: docs}, nil
}

// GetDoc returns one project-level markdown document by ID.
func (e *Engine) GetDoc(ctx context.Context, params proto.DocsGetParams) (proto.DocsGetResult, error) {
	if strings.TrimSpace(e.docsDir) == "" {
		return proto.DocsGetResult{}, errors.New("engine: docs directory not configured")
	}
	if err := ctx.Err(); err != nil {
		return proto.DocsGetResult{}, err
	}
	id := cleanDocID(params.ID)
	path, ok := e.docPath(id)
	if !ok {
		return proto.DocsGetResult{}, store.ErrNotFound
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return proto.DocsGetResult{}, store.ErrNotFound
		}
		return proto.DocsGetResult{}, fmt.Errorf("engine: read doc: %w", err)
	}
	mode := strings.ToLower(strings.TrimSpace(params.Mode))
	if mode == "" {
		mode = "preview"
	}
	if mode != "preview" && mode != "raw" {
		return proto.DocsGetResult{}, fmt.Errorf("engine: unsupported docs mode %q", params.Mode)
	}
	return proto.DocsGetResult{
		Doc:     proto.DocSummary{ID: id, Title: markdownTitle(path, filepath.Base(id))},
		Content: string(data),
		Mode:    mode,
	}, nil
}

func (e *Engine) docPath(id string) (string, bool) {
	if id == "README.md" || id == "CHANGELOG.md" {
		return filepath.Join(e.docsDir, id), true
	}
	if strings.HasPrefix(id, "docs/") && strings.Count(id, "/") == 1 && strings.EqualFold(filepath.Ext(id), ".md") {
		return filepath.Join(e.docsDir, filepath.FromSlash(id)), true
	}
	return "", false
}

func cleanDocID(id string) string {
	id = filepath.ToSlash(strings.TrimSpace(id))
	id = strings.TrimPrefix(id, "/")
	if id == "README" || id == "CHANGELOG" {
		id += ".md"
	}
	return id
}

func markdownTitle(path, fallback string) string {
	data, err := os.ReadFile(path)
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				return strings.TrimSpace(strings.TrimPrefix(line, "# "))
			}
		}
	}
	fallback = strings.TrimSuffix(filepath.Base(fallback), filepath.Ext(fallback))
	fallback = strings.ReplaceAll(fallback, "_", " ")
	fallback = strings.ReplaceAll(fallback, "-", " ")
	return fallback
}

func docSortKey(id string) string {
	switch id {
	case "README.md":
		return "00"
	case "CHANGELOG.md":
		return "01"
	default:
		return "02/" + id
	}
}
