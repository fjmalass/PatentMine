// Full-text search payloads: scanning the stored USPTO bodies of a set of
// patents for a query string.
package proto

import "patentmine/internal/domain"

// FullTextSearchParams searches the locally-stored full-text bodies of the
// given patents for Query (case-insensitive substring). Patents with no
// ingested body are reported back in the result's Missing list rather than
// fetched on the fly.
type FullTextSearchParams struct {
	Numbers []domain.PatentNumber `json:"numbers"`
	Query   string                `json:"query"`
}

// FullTextSearchMatch is one matching section within one patent's body.
// Locator names the section ("Claim 5", "Disclosure ¶0045", "Disclosure
// ¶Abstract") so the viewer can jump straight to it; Snippet is a short window
// of text around the match for display in the result list.
type FullTextSearchMatch struct {
	Number  domain.PatentNumber `json:"number"`
	Kind    USPTOXMLKind        `json:"kind,omitempty"`
	Locator string              `json:"locator"`
	// Occurrence is the 0-based index of this hit among the matches in its
	// section, so the viewer can land on the exact occurrence, not just the
	// section.
	Occurrence int    `json:"occurrence"`
	Snippet    string `json:"snippet"`
}

// FullTextSearchResult carries every match plus coverage bookkeeping: how many
// patents had a local body to scan, and which selected patents had none (so the
// caller can tell "no matches" apart from "nothing was searchable").
type FullTextSearchResult struct {
	Query   string                `json:"query"`
	Matches []FullTextSearchMatch `json:"matches"`
	Scanned int                   `json:"scanned"`
	Missing []domain.PatentNumber `json:"missing,omitempty"`
}
