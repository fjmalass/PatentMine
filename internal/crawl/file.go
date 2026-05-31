package crawl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"patentmine/internal/domain"
)

// dateLayout is the date format used in local patent files.
const dateLayout = domain.DateLayout

// fileDocument is the JSON shape of one life-stage document.
type fileDocument struct {
	Number string `json:"number"`
	Stage  string `json:"stage"`
	Date   string `json:"date"`
}

// patentFile is the on-disk JSON shape of a saved patent record.
type patentFile struct {
	Number          string           `json:"number"`
	Title           string           `json:"title"`
	Abstract        string           `json:"abstract"`
	Assignee        string           `json:"assignee"`
	Inventors       []domain.Inventor `json:"inventors"`
	ApplicationDate string           `json:"application_date"`
	PublicationDate string           `json:"publication_date"`
	GrantDate       string           `json:"grant_date"`
	FirstClaim      string           `json:"first_claim"`
	Documents       []fileDocument   `json:"documents"`
	Cites           []string         `json:"cites"`
	CitedBy         []string         `json:"cited_by"`
	Parents         []string         `json:"parents"`
	Children        []string         `json:"children"`
}

// FileSource serves patents from local JSON files on disk. It is the first
// source implemented, so the crawler and engine can be exercised without the
// network.
type FileSource struct {
	dir string
}

// NewFileSource reads patents from dir, where each file is named
// <normalized-number>.json.
func NewFileSource(dir string) *FileSource {
	return &FileSource{dir: dir}
}

// Name implements Source.
func (s *FileSource) Name() domain.Source { return domain.SourceFile }

// Fetch implements Source by reading <dir>/<number>.json.
func (s *FileSource) Fetch(_ context.Context, number domain.PatentNumber) (Result, error) {
	path := filepath.Join(s.dir, number.Normalized()+".json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Result{}, ErrNotAvailable
	}
	if err != nil {
		return Result{}, fmt.Errorf("crawl/file: read %s: %w", path, err)
	}
	var file patentFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return Result{}, fmt.Errorf("crawl/file: decode %s: %w", path, err)
	}
	return fileToResult(number, file)
}

// fileToResult converts a decoded patent file into a Result.
func fileToResult(number domain.PatentNumber, file patentFile) (Result, error) {
	patent := domain.Patent{
		Number:     number,
		Title:      file.Title,
		Abstract:   file.Abstract,
		Assignee:   file.Assignee,
		Inventors:  file.Inventors,
		FirstClaim: file.FirstClaim,
		FetchState: domain.FetchCached,
		Source:     domain.SourceFile,
		FetchedAt:  time.Now().UTC(),
	}
	var err error
	if patent.ApplicationDate, err = parseDate(file.ApplicationDate); err != nil {
		return Result{}, err
	}
	if patent.PublicationDate, err = parseDate(file.PublicationDate); err != nil {
		return Result{}, err
	}
	if patent.GrantDate, err = parseDate(file.GrantDate); err != nil {
		return Result{}, err
	}

	documents, err := fileDocuments(number, file)
	if err != nil {
		return Result{}, err
	}
	relations, err := buildRelations(number, file)
	if err != nil {
		return Result{}, err
	}
	return Result{Patent: patent, Documents: documents, Relations: relations}, nil
}

// ImportFile loads a patent record from a local fixture file (the patentFile
// JSON shape) into the store, including its citation and family edges. It
// satisfies engine.FileImporter, so the engine can import a file without
// importing the crawl package.
func (c *Crawler) ImportFile(ctx context.Context, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("crawl: read fixture %s: %w", path, err)
	}
	var file patentFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return fmt.Errorf("crawl: decode fixture %s: %w", path, err)
	}
	number, err := domain.ParsePatentNumber(file.Number)
	if err != nil {
		return fmt.Errorf("crawl: fixture %s has an invalid number %q: %w", path, file.Number, err)
	}
	res, err := fileToResult(number, file)
	if err != nil {
		return err
	}
	// depth 0 with limit 0: the record, its documents, neighbour stubs, and
	// edges are written in one batch, but no neighbour is queued for crawling.
	queue := []node{}
	_, _, _, err = c.ingestNode(ctx, res, 0, 0, "", map[domain.PatentNumber]bool{}, &queue)
	return err
}

// fileDocuments reads the life-stage documents. When a file lists none, one is
// synthesized from the record number so every record has at least one document.
func fileDocuments(number domain.PatentNumber, file patentFile) ([]domain.Document, error) {
	if len(file.Documents) == 0 {
		dated, err := parseDate(firstNonEmpty(file.GrantDate, file.PublicationDate, file.ApplicationDate))
		if err != nil {
			return nil, err
		}
		return []domain.Document{{
			Number: number,
			Stage:  domain.GuessStage(number),
			Dated:  dated,
		}}, nil
	}
	var docs []domain.Document
	for _, fd := range file.Documents {
		num, err := domain.ParsePatentNumber(fd.Number)
		if err != nil {
			return nil, fmt.Errorf("crawl/file: document %q: %w", fd.Number, err)
		}
		stage, err := domain.ParseStage(fd.Stage)
		if err != nil {
			return nil, fmt.Errorf("crawl/file: document %s: %w", fd.Number, err)
		}
		dated, err := parseDate(fd.Date)
		if err != nil {
			return nil, err
		}
		docs = append(docs, domain.Document{Number: num, Stage: stage, Dated: dated})
	}
	return docs, nil
}

// buildRelations turns a file's neighbor lists into directed edges.
func buildRelations(from domain.PatentNumber, file patentFile) ([]domain.Relation, error) {
	groups := []struct {
		kind    domain.RelationKind
		numbers []string
	}{
		{domain.RelationCites, file.Cites},
		{domain.RelationCitedBy, file.CitedBy},
		{domain.RelationParent, file.Parents},
		{domain.RelationChild, file.Children},
	}
	var relations []domain.Relation
	for _, g := range groups {
		for _, raw := range g.numbers {
			to, err := domain.ParsePatentNumber(raw)
			if err != nil {
				return nil, fmt.Errorf("crawl/file: %s neighbor %q: %w", g.kind, raw, err)
			}
			relations = append(relations, domain.Relation{From: from, To: to, Kind: g.kind})
		}
	}
	return relations, nil
}

// parseDate parses a YYYY-MM-DD date, treating "" as the zero time.
func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("crawl/file: bad date %q: %w", s, err)
	}
	return t, nil
}

// firstNonEmpty returns the first non-empty string, or "".
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
