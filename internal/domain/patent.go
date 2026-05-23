package domain

import (
	"fmt"
	"time"
)

// SortColumn names a column that patent listings can be ordered by.
type SortColumn string

const (
	SortByNumber         SortColumn = "number"
	SortByTitle          SortColumn = "title"
	SortByInventor       SortColumn = "inventor"
	SortByExpires        SortColumn = "expires"
	SortByReviewState    SortColumn = "review_state"
	SortByIDS            SortColumn = "ids"
	SortByTags           SortColumn = "tags"
	SortByClassification SortColumn = "classification"

	// UI-only sort keys used by the per-patent classification overlay. They
	// never travel over RPC — the overlay sorts its in-memory rows directly.
	// The full word "Classification" is used to avoid collision with the CPC
	// hierarchy field called "Class" inside the Classification struct.
	SortByClassificationSystem      SortColumn = "classification.system"
	SortByClassificationCode        SortColumn = "classification.code"
	SortByClassificationDescription SortColumn = "classification.description"
)

// CrawlProfile defines which family-graph edges to follow during a crawl.
type CrawlProfile string

const (
	CrawlProfileCitations CrawlProfile = "citations" // Follow cites only (depth 0 only)
	CrawlProfileCitedBy   CrawlProfile = "citedby"   // Follow cited_by only (depth 0 only)
	CrawlProfileFamily    CrawlProfile = "family"    // Follow parent/child recursion
	CrawlProfileAll       CrawlProfile = "all"       // Combination of the above
)

// Valid reports whether the profile is a known value.
func (p CrawlProfile) Valid() bool {
	switch p {
	case CrawlProfileCitations, CrawlProfileCitedBy, CrawlProfileFamily, CrawlProfileAll:
		return true
	default:
		return false
	}
}

// Source identifies where a patent record was crawled from.
type Source string

const (
	// SourceFile is the local file source, used before live sources exist.
	SourceFile Source = "file"
	// SourceGoogle is Google Patents.
	SourceGoogle Source = "google"
	// SourceJustia is Justia Patents.
	SourceJustia Source = "justia"
	// SourceUSPTO is the United States Patent and Trademark Office.
	SourceUSPTO Source = "uspto"
	// SourcePCT is the WIPO / PCT international system.
	SourcePCT Source = "pct"
)

// ExpirationEstimated marks an ExpirationDate derived by the +20-year rule
// rather than read from an authoritative source.
const ExpirationEstimated = "estimated"

// Valid reports whether the Source is a known value.
func (s Source) Valid() bool {
	switch s {
	case SourceFile, SourceGoogle, SourceJustia, SourceUSPTO, SourcePCT:
		return true
	default:
		return false
	}
}

// ParseSource converts a string into a Source.
func ParseSource(s string) (Source, error) {
	src := Source(s)
	if !src.Valid() {
		return "", fmt.Errorf("domain: unknown source %q", s)
	}
	return src, nil
}

// Inventor is one human creator of a patented invention.
type Inventor string

// InventorStats carries aggregated database statistics for an inventor.
type InventorStats struct {
	Inventor string         `json:"inventor"`
	Total    int            `json:"total"`
	States   map[string]int `json:"states"`
	Tags     map[string]int `json:"tags"`
}

// Patent is the core business object: one patent record. One record spans the
// invention's whole life — its application, publication, and grant are all
// Documents of the same record. It carries no I/O or UI concerns: persistence
// lives in package store, display in package tui.
type Patent struct {
	// Number is the record's permanent number — the first document number
	// ever seen for it. It never changes, even after the patent later
	// publishes, so rows that point at a record stay valid.
	Number PatentNumber `json:"number"`
	// DisplayNumber is the number the record should be shown by: the
	// latest-stage document (grant, else publication, else application).
	DisplayNumber   PatentNumber `json:"display_number"`
	Title           string       `json:"title"`
	Abstract        string       `json:"abstract"`
	Assignee        string       `json:"assignee"`
	Inventors       []Inventor   `json:"inventors"`
	FetchState      FetchState   `json:"fetch_state"`
	Source          Source       `json:"source"`
	ApplicationDate time.Time    `json:"application_date"` // Zero when unknown.
	PublicationDate time.Time    `json:"publication_date"` // Zero when unknown.
	GrantDate       time.Time    `json:"grant_date"`       // Zero when unknown.
	FetchedAt       time.Time    `json:"fetched_at"`       // Zero for a stub never fetched.
	// FirstClaim is the patent's claim 1, the body text shown in the detail view.
	FirstClaim string `json:"first_claim,omitempty"`
	// ExpirationDate is when the patent's protection ends; zero when unknown.
	ExpirationDate time.Time `json:"expiration_date"`
	// ExpirationSource records how ExpirationDate was determined, e.g. "estimated".
	ExpirationSource string `json:"expiration_source,omitempty"`
	// SourceURL is the provider page the record was fetched from.
	SourceURL string `json:"source_url,omitempty"`
	// Classifications is the set of classification codes (e.g. CPC, IPC) for this patent.
	Classifications []string `json:"classifications"`
	// Documents is the open-ended set of life-stage documents for this record.
	Documents []Document `json:"documents"`
}

// PatentRow is the lightweight listing shape used by paged views. It keeps the
// record key and display number separate so list UIs stay cheap without losing
// the application/publication/grant distinction.
type PatentRow struct {
	Number          PatentNumber `json:"number"`
	DisplayNumber   PatentNumber `json:"display_number"`
	Title           string       `json:"title"`
	Inventors       []Inventor   `json:"inventors"`
	PublicationDate time.Time    `json:"publication_date"`
	ExpirationDate  time.Time    `json:"expiration_date"`
	Tags            []string     `json:"tags"`
	Classifications []string     `json:"classifications"`
	FetchState      FetchState   `json:"fetch_state"`
	ReviewState     ReviewState  `json:"review_state,omitempty"`
	IDSEntry        *IDSEntry    `json:"ids_entry,omitempty"`
	CitationsCount  int          `json:"citations_count,omitempty"`
	CitedByCount    int          `json:"cited_by_count,omitempty"`
	ParentsCount    int          `json:"parents_count,omitempty"`
}

// IsStub reports whether only a reference exists, without the full body.
func (p Patent) IsStub() bool {
	return p.FetchState == FetchStub
}

// LatestDocument returns the record's furthest-along document — grant over
// publication over application, breaking ties by date. ok is false when the
// record has no documents.
func (p Patent) LatestDocument() (latest Document, ok bool) {
	for _, d := range p.Documents {
		switch {
		case !ok,
			d.Stage.rank() > latest.Stage.rank(),
			d.Stage.rank() == latest.Stage.rank() && d.Dated.After(latest.Dated):
			latest, ok = d, true
		}
	}
	return latest, ok
}

// NumberToShow returns the number the record should be displayed by: its
// latest document's number, or the record number when it has no documents.
func (p Patent) NumberToShow() PatentNumber {
	if latest, ok := p.LatestDocument(); ok {
		return latest.Number
	}
	return p.Number
}

// DocumentFor returns the document of the given stage, if the record has one.
func (p Patent) DocumentFor(stage Stage) (Document, bool) {
	for _, d := range p.Documents {
		if d.Stage == stage {
			return d, true
		}
	}
	return Document{}, false
}
