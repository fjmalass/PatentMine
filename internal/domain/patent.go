package domain

import (
	"fmt"
	"time"
)

// Source identifies where a patent record was ingested from.
type Source string

const (
	// SourceFixture is a local test fixture, used before live sources exist.
	SourceFixture Source = "fixture"
	// SourceGoogle is Google Patents.
	SourceGoogle Source = "google"
	// SourceJustia is Justia Patents.
	SourceJustia Source = "justia"
	// SourceUSPTO is the United States Patent and Trademark Office.
	SourceUSPTO Source = "uspto"
	// SourcePCT is the WIPO / PCT international system.
	SourcePCT Source = "pct"
)

// Valid reports whether the Source is a known value.
func (s Source) Valid() bool {
	switch s {
	case SourceFixture, SourceGoogle, SourceJustia, SourceUSPTO, SourcePCT:
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

// Patent is the core business object: one patent document. It carries no I/O
// or UI concerns — persistence lives in package store, display in package tui.
type Patent struct {
	Number          PatentNumber `json:"number"`
	Title           string       `json:"title"`
	Abstract        string       `json:"abstract"`
	Assignee        string       `json:"assignee"`
	Inventors       []string     `json:"inventors"`
	FetchState      FetchState   `json:"fetch_state"`
	Source          Source       `json:"source"`
	ApplicationDate time.Time    `json:"application_date"` // Zero when unknown.
	PublicationDate time.Time    `json:"publication_date"` // Zero when unknown.
	GrantDate       time.Time    `json:"grant_date"`       // Zero when unknown.
	FetchedAt       time.Time    `json:"fetched_at"`       // Zero for a stub never fetched.
}

// IsStub reports whether only a reference exists, without the full body.
func (p Patent) IsStub() bool {
	return p.FetchState == FetchStub
}
