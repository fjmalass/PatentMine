package ingest

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

// fixtureDateLayout is the date format used in fixture JSON files.
const fixtureDateLayout = "2006-01-02"

// fixtureFile is the on-disk JSON shape of a fixture patent.
type fixtureFile struct {
	Number          string   `json:"number"`
	Title           string   `json:"title"`
	Abstract        string   `json:"abstract"`
	Assignee        string   `json:"assignee"`
	Inventors       []string `json:"inventors"`
	ApplicationDate string   `json:"application_date"`
	PublicationDate string   `json:"publication_date"`
	GrantDate       string   `json:"grant_date"`
	Cites           []string `json:"cites"`
	CitedBy         []string `json:"cited_by"`
	Parents         []string `json:"parents"`
	Children        []string `json:"children"`
}

// FixtureSource serves patents from local JSON files. It is the first source
// implemented, so the crawler and engine can be exercised without the network.
type FixtureSource struct {
	dir string
}

// NewFixtureSource reads fixtures from dir, where each file is named
// <normalized-number>.json.
func NewFixtureSource(dir string) *FixtureSource {
	return &FixtureSource{dir: dir}
}

// Name implements Source.
func (s *FixtureSource) Name() domain.Source { return domain.SourceFixture }

// Fetch implements Source by reading <dir>/<number>.json.
func (s *FixtureSource) Fetch(_ context.Context, number domain.PatentNumber) (Result, error) {
	path := filepath.Join(s.dir, number.Normalized()+".json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Result{}, ErrNotAvailable
	}
	if err != nil {
		return Result{}, fmt.Errorf("ingest/fixture: read %s: %w", path, err)
	}
	var file fixtureFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return Result{}, fmt.Errorf("ingest/fixture: decode %s: %w", path, err)
	}
	return fixtureToResult(number, file)
}

// fixtureToResult converts a decoded fixture into a Result.
func fixtureToResult(number domain.PatentNumber, file fixtureFile) (Result, error) {
	patent := domain.Patent{
		Number:     number,
		Title:      file.Title,
		Abstract:   file.Abstract,
		Assignee:   file.Assignee,
		Inventors:  file.Inventors,
		FetchState: domain.FetchCached,
		Source:     domain.SourceFixture,
		FetchedAt:  time.Now().UTC(),
	}
	var err error
	if patent.ApplicationDate, err = parseFixtureDate(file.ApplicationDate); err != nil {
		return Result{}, err
	}
	if patent.PublicationDate, err = parseFixtureDate(file.PublicationDate); err != nil {
		return Result{}, err
	}
	if patent.GrantDate, err = parseFixtureDate(file.GrantDate); err != nil {
		return Result{}, err
	}

	relations, err := buildRelations(number, file)
	if err != nil {
		return Result{}, err
	}
	return Result{Patent: patent, Relations: relations}, nil
}

// buildRelations turns the fixture's neighbor lists into directed edges.
func buildRelations(from domain.PatentNumber, file fixtureFile) ([]domain.Relation, error) {
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
				return nil, fmt.Errorf("ingest/fixture: %s neighbor %q: %w", g.kind, raw, err)
			}
			relations = append(relations, domain.Relation{From: from, To: to, Kind: g.kind})
		}
	}
	return relations, nil
}

// parseFixtureDate parses a YYYY-MM-DD date, treating "" as the zero time.
func parseFixtureDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(fixtureDateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("ingest/fixture: bad date %q: %w", s, err)
	}
	return t, nil
}
