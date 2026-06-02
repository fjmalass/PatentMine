package domain

import (
	"strings"
	"time"
)

// ProjectID is the stable identifier of a project. A distinct type (not a bare
// string) keeps it from being mixed up with patent numbers or other IDs.
type ProjectID string

// ProjectExaminer is one examiner assignment recorded for a project. A patent
// application can be reassigned during prosecution, so examiners are kept as a
// history list ordered by RecordedAt; the latest entry is the current one.
type ProjectExaminer struct {
	Name       string    `json:"name"`
	RecordedAt time.Time `json:"recorded_at"`
}

// Project groups patents for a body of work — typically one patent-office
// filing for which an IDS will be generated.
type Project struct {
	ID        ProjectID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	// IDS header fields written into PTO/SB/08a and PTO/SB/08c when rendering
	// the IDS PDF bundle. Empty until the user fills them via the IDS header
	// overlay.
	ApplicationNumber    string            `json:"application_number,omitempty"`
	FilingDate           time.Time         `json:"filing_date"`
	Inventors            []string          `json:"inventors,omitempty"`
	ArtUnit              string            `json:"art_unit,omitempty"`
	Examiners            []ProjectExaminer `json:"examiners,omitempty"`
	AttorneyDocketNumber string            `json:"attorney_docket_number,omitempty"`
	CachedCount          int               `json:"cached_count"`
}

// FirstInventor returns the first-named inventor for the IDS header, or empty
// when none has been recorded yet.
func (p Project) FirstInventor() string {
	if len(p.Inventors) == 0 {
		return ""
	}
	return p.Inventors[0]
}

// LatestExaminer returns the most recently recorded examiner name, or empty
// when no examiner is on file. When two entries share the same timestamp the
// last one in the slice wins.
func (p Project) LatestExaminer() string {
	if len(p.Examiners) == 0 {
		return ""
	}
	latest := p.Examiners[0]
	for _, e := range p.Examiners[1:] {
		if !e.RecordedAt.Before(latest.RecordedAt) {
			latest = e
		}
	}
	return latest.Name
}

// JoinInventors renders inventors as a comma-separated string suitable for
// editing in a single-line text input. Empty when no inventors are recorded.
func (p Project) JoinInventors() string {
	return strings.Join(p.Inventors, ", ")
}

// ParseInventors splits a comma-separated inventor string into a clean list.
// Whitespace around names is trimmed and empty entries are dropped.
func ParseInventors(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// Membership links one patent to one project and carries its workflow state.
// A patent in several projects has one Membership per project.
type Membership struct {
	Patent             PatentNumber `json:"patent"`
	Project            ProjectID    `json:"project"`
	ReviewState        ReviewState  `json:"review_state"`
	AddedAt            time.Time    `json:"added_at"`
	AddedMethod        string       `json:"added_method,omitempty"`         // 'direct' or 'related'
	ParentPatentNumber PatentNumber `json:"parent_patent_number,omitempty"` // Root patent of relationship
	SourceProvider     string       `json:"source_provider,omitempty"`       // 'uspto', 'google', 'file'
	SourceMode         string       `json:"source_mode,omitempty"`           // 'compare', 'uspto-first', etc.
}
