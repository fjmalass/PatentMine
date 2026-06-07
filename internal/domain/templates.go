package domain

import "fmt"

// Template/document-set layout. The user-managed templates root is laid out as
//
//	<templates-root>/<TemplatesOfficeActionsSubdir>/<DocumentSet>/<files…>
//
// These names are pinned here, in one place, so the path is never spelled out as
// loose string literals across the engine, RPC, and TUI.
const TemplatesOfficeActionsSubdir = "officeactions"

// DocumentSet names a template-driven set of documents a matter is built from.
// Each set has its own folder of templates under TemplatesOfficeActionsSubdir
// and is also used as the matter's per-type output subdirectory.
type DocumentSet string

const (
	// DocSetProvisional is the provisional-application document set (the SB/16
	// cover sheet plus the written description, drawings, and ADS templates).
	DocSetProvisional DocumentSet = "provisional"
	// DocSetApplication is the non-provisional application document set generated
	// when a provisional is submitted.
	DocSetApplication DocumentSet = "application"
)

// Valid reports whether the DocumentSet is a known value.
func (d DocumentSet) Valid() bool {
	switch d {
	case DocSetProvisional, DocSetApplication:
		return true
	default:
		return false
	}
}

// Label returns the human-readable name of a document set for the TUI.
func (d DocumentSet) Label() string {
	switch d {
	case DocSetProvisional:
		return "Provisional"
	case DocSetApplication:
		return "Application"
	default:
		return string(d)
	}
}

// ParseDocumentSet converts a string into a DocumentSet.
func ParseDocumentSet(s string) (DocumentSet, error) {
	d := DocumentSet(s)
	if !d.Valid() {
		return "", fmt.Errorf("domain: unknown document set %q", s)
	}
	return d, nil
}
