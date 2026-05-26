package domain

// SourceMode is the daemon-wide policy for choosing crawl sources.
// It is typed so handlers, configuration, and the registry can never confuse
// it with another string parameter — the compiler refuses bare-string passes.
type SourceMode string

const (
	// SourceModeCompare fetches from USPTO and then also from Google so the
	// caller can diff the two records side-by-side. The slowest mode.
	SourceModeCompare SourceMode = "compare"

	// SourceModeUSPTOFirst tries USPTO first and only falls back to Google
	// when USPTO has no record for the requested patent.
	SourceModeUSPTOFirst SourceMode = "uspto-first"

	// SourceModeUSPTOOnly never consults Google; USPTO failures surface as
	// errors instead of silent Google substitutions.
	SourceModeUSPTOOnly SourceMode = "uspto-only"

	// SourceModeGoogleOnly never consults USPTO; useful when only Google
	// data is wanted (foreign patents, legacy records, USPTO outages).
	SourceModeGoogleOnly SourceMode = "google-only"
)

// String returns the canonical wire-form name of the mode.
func (m SourceMode) String() string { return string(m) }

// IsValid reports whether m is one of the four canonical modes.
func (m SourceMode) IsValid() bool {
	switch m {
	case SourceModeCompare, SourceModeUSPTOFirst, SourceModeUSPTOOnly, SourceModeGoogleOnly:
		return true
	}
	return false
}
