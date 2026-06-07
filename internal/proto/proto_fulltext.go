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
	// Number is the canonical record number — the key the viewer loads the body
	// by. Display is the stage's public number (the "…B2" grant or "…A1"
	// publication) shown in the result row; loading by Display would fail because
	// it names a document, not the record.
	Number  domain.PatentNumber `json:"number"`
	Display domain.PatentNumber `json:"display,omitempty"`
	Kind    USPTOXMLKind        `json:"kind,omitempty"`
	Locator string              `json:"locator"`
	// Occurrence is the 0-based index of this hit among the matches in its
	// section, so the viewer can land on the exact occurrence, not just the
	// section.
	Occurrence int    `json:"occurrence"`
	Snippet    string `json:"snippet"`
}

// FullTextCoverageParams asks how much stored full text the given patents have.
type FullTextCoverageParams struct {
	Numbers []domain.PatentNumber `json:"numbers"`
}

// FullTextCoverageEntry reports one patent's stored full-text body: whether one
// exists, which document (grant/pgpub) it came from, and its plain-text size.
type FullTextCoverageEntry struct {
	Number  domain.PatentNumber `json:"number"`
	Present bool                `json:"present"`
	Kind    USPTOXMLKind        `json:"kind,omitempty"`
	Bytes   int                 `json:"bytes,omitempty"`
}

// FullTextCoverageResult carries per-patent coverage plus project-wide totals.
type FullTextCoverageResult struct {
	Entries    []FullTextCoverageEntry `json:"entries"`
	Bodies     int                     `json:"bodies"`      // patents with a stored body
	TotalBytes int64                   `json:"total_bytes"` // summed plain-text size, bytes
}

// FullTextSearchResult carries every match plus coverage bookkeeping: how many
// patents had a local body to scan, and which selected patents had none (so the
// caller can tell "no matches" apart from "nothing was searchable").
type FullTextSearchResult struct {
	Query   string                `json:"query"`
	Matches []FullTextSearchMatch `json:"matches"`
	Scanned int                   `json:"scanned"`
	// NoMatch lists patents that had an ingested body but no hit; Missing lists
	// patents with no ingested body at all. Together with the matched patents
	// they account for every selected patent.
	NoMatch []domain.PatentNumber `json:"no_match,omitempty"`
	Missing []domain.PatentNumber `json:"missing,omitempty"`
}
