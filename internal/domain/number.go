package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// PatentNumber is a parsed, normalized patent identifier. It is a value type:
// two PatentNumbers compare equal when every field matches, so it is safe to
// use as a map key and for deduplication across crawl sources.
type PatentNumber struct {
	Country string // ISO-2 country code, uppercase; empty when the source omits it.
	Serial  string // Serial digits only, separators stripped.
	Kind    string // Kind code (e.g. "B2", "A1"); empty when absent.
}



var (
	// ErrEmptyNumber is returned when the input has no content.
	ErrEmptyNumber = errors.New("domain: empty patent number")
	// ErrNoSerial is returned when no serial digits could be found.
	ErrNoSerial = errors.New("domain: patent number has no serial digits")
)

// patentNumberPattern matches an optional 2-letter country code, optional spaces/hyphens,
// an optional alphabetical prefix (like RE, D, PP) followed by serial digits, and an optional kind code.
// Examples: "US11611785B2", "US-11,611,785-B2", "EP1234567A1", "US16/123456", "RE48372", "USD890123".
var patentNumberPattern = regexp.MustCompile(
	`^([A-Z]{2})?[\s\-]*([A-Z]*[0-9][0-9\s,\-/]*?)[\s\-]*([A-Z][0-9]?)?$`,
)

var nonAlphanumeric = regexp.MustCompile(`[^A-Z0-9]`)

// ParsePatentNumber normalizes raw input into a PatentNumber. It uppercases,
// trims, and strips separators so the same patent written differently by
// different sources parses to an equal value.
func ParsePatentNumber(raw string) (PatentNumber, error) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.Trim(s, "[]")
	s = strings.ReplaceAll(s, ":", " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return PatentNumber{}, ErrEmptyNumber
	}

	// For US reissue patents parsed without country prefix (e.g. RE48372), treat as US
	if strings.HasPrefix(s, "RE") && len(s) > 2 && s[2] >= '0' && s[2] <= '9' {
		s = "US" + s
	}

	m := patentNumberPattern.FindStringSubmatch(s)
	if m == nil {
		return PatentNumber{}, fmt.Errorf("domain: cannot parse patent number %q", raw)
	}
	serial := nonAlphanumeric.ReplaceAllString(m[2], "")
	if serial == "" {
		return PatentNumber{}, ErrNoSerial
	}
	kind := m[3]
	// A document number that carries a kind code (e.g. "US09658068B2") is a
	// grant or publication identifier whose leading zeros are insignificant
	// zero-padding: Google's canonical page is "US9658068B2", so the padded
	// form 404s and the patent silently vanishes after its not-found cleanup.
	// Strip the padding so "US09658068B2" and "US9658068B2" are one identity
	// that resolves cleanly. A bare application serial (no kind code, e.g.
	// "09/658,068") is left intact: there the leading "09" is a significant
	// series code, not padding.
	if kind != "" {
		serial = strings.TrimLeft(serial, "0")
		if serial == "" {
			serial = "0"
		}
	}
	return PatentNumber{Country: m[1], Serial: serial, Kind: kind}, nil
}

// MustParsePatentNumber is ParsePatentNumber for compile-time-known literals
// (tests, samples). It panics on error.
func MustParsePatentNumber(raw string) PatentNumber {
	n, err := ParsePatentNumber(raw)
	if err != nil {
		panic(err)
	}
	return n
}

// IsZero reports whether the number is the empty value.
func (n PatentNumber) IsZero() bool {
	return n == PatentNumber{}
}

// Normalized returns the canonical string form: country + serial + kind.
func (n PatentNumber) Normalized() string {
	return n.Country + n.Serial + n.Kind
}

// GooglePatentURLPrefix is the base of a Google Patents patent page URL. It
// lives in domain so every layer that links to Google Patents (crawl Source,
// detail view) builds the URL from one literal rather than repeating it.
const GooglePatentURLPrefix = "https://patents.google.com/patent/"

// GooglePatentURL returns the Google Patents page URL for this number. Google's
// canonical page for a granted patent is its grant document (e.g. US…B2), so
// callers that want the default Google version should build this from the
// record's granted/latest number.
func (n PatentNumber) GooglePatentURL() string {
	return GooglePatentURLPrefix + n.Normalized() + "/en"
}

// String implements fmt.Stringer.
func (n PatentNumber) String() string {
	return n.Normalized()
}

// HasExplicitGrantKind reports whether the patent number has an explicit kind code representing a grant.
func (n PatentNumber) HasExplicitGrantKind() bool {
	if strings.HasPrefix(n.Kind, "B") || strings.HasPrefix(n.Kind, "E") || strings.HasPrefix(n.Kind, "S") {
		return true
	}
	if strings.HasPrefix(n.Kind, "P") && n.Kind != "PL" {
		return true
	}
	return false
}

// HasExplicitPublicationKind reports whether the patent number has an explicit kind code representing a publication.
func (n PatentNumber) HasExplicitPublicationKind() bool {
	if n.Kind != "" && !strings.HasPrefix(n.Kind, "B") && !strings.HasPrefix(n.Kind, "E") && !strings.HasPrefix(n.Kind, "S") {
		if strings.HasPrefix(n.Kind, "P") && n.Kind != "PL" {
			return false
		}
		// Exclude legacy US grants which have bare "A" kind codes
		if n.Country == "US" && n.Kind == "A" {
			return false
		}
		return true
	}
	return false
}

// IsGrant reports whether the patent number represents a granted patent.
func (n PatentNumber) IsGrant() bool {
	if n.HasExplicitGrantKind() {
		return true
	}
	// US legacy grants sometimes carry a bare "A" kind code (e.g. US1234567A)
	if n.Country == "US" && n.Kind == "A" {
		return true
	}
	// Fallback stage detection for US granted patents queried without kind codes
	if n.Country == "US" && n.Kind == "" {
		digitsOnly := getDigitsOnly(n.Serial)
		if len(digitsOnly) <= 7 {
			return true
		}
	}
	return false
}

// IsUSGrant reports whether the patent number represents a US granted patent.
func (n PatentNumber) IsUSGrant() bool {
	return n.Country == "US" && n.IsGrant()
}

// IsPublication reports whether the patent number represents a published pre-grant application.
func (n PatentNumber) IsPublication() bool {
	if n.HasExplicitPublicationKind() {
		return true
	}
	// Fallback stage detection for US publications queried without kind codes (11 digits)
	if n.Country == "US" && n.Kind == "" {
		digitsOnly := getDigitsOnly(n.Serial)
		if len(digitsOnly) == 11 {
			return true
		}
	}
	return false
}

// IsApplication reports whether the patent number represents an application.
func (n PatentNumber) IsApplication() bool {
	return !n.IsGrant() && !n.IsPublication()
}

// Stage returns the lifecycle stage of this patent number.
func (n PatentNumber) Stage() Stage {
	if n.IsGrant() {
		return StageGrant
	}
	if n.IsPublication() {
		return StagePublication
	}
	return StageApplication
}

// getDigitsOnly returns only the numerical digits in the string.
func getDigitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// formatUSApplication formats an 8-digit US application number as "XX/YYY,YYY".
func formatUSApplication(serial string) string {
	digits := getDigitsOnly(serial)
	if len(digits) != 8 {
		return serial
	}
	return digits[:2] + "/" + digits[2:5] + "," + digits[5:]
}

// formatUSPublication formats an 11-digit US publication number as "YYYY/NNNNNNN".
func formatUSPublication(serial string) string {
	digits := getDigitsOnly(serial)
	if len(digits) != 11 {
		return serial
	}
	return digits[:4] + "/" + digits[4:]
}

// formatCommas formats numerical values with thousands commas, leaving alphabetical prefixes intact.
func formatCommas(s string) string {
	prefix := ""
	digits := s
	for i, r := range s {
		if r >= '0' && r <= '9' {
			prefix = s[:i]
			digits = s[i:]
			break
		}
	}

	n := len(digits)
	if n <= 3 {
		return prefix + digits
	}
	var b strings.Builder
	b.WriteString(prefix)
	first := n % 3
	if first == 0 {
		first = 3
	}
	b.WriteString(digits[:first])
	for i := first; i < n; i += 3 {
		b.WriteByte(',')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// DisplayString returns a human-friendly string for the patent number.
func (n PatentNumber) DisplayString() string {
	if n.IsZero() {
		return ""
	}

	serialDisp := n.Serial

	// Coordinate US-specific visual formatting
	if n.Country == "US" {
		if n.IsGrant() {
			serialDisp = formatCommas(n.Serial)
		} else if n.IsApplication() {
			serialDisp = formatUSApplication(n.Serial)
		} else if n.IsPublication() {
			serialDisp = formatUSPublication(n.Serial)
		}
	}

	return n.Country + serialDisp + n.Kind
}

// Equal reports whether two numbers refer to the same patent document.
func (n PatentNumber) Equal(other PatentNumber) bool {
	return n == other
}

// MarshalJSON encodes the number as its normalized string form so the wire
// protocol and the web API carry a plain "US11611785B2" rather than an object.
func (n PatentNumber) MarshalJSON() ([]byte, error) {
	return json.Marshal(n.Normalized())
}

// UnmarshalJSON parses a normalized string form back into a PatentNumber.
func (n *PatentNumber) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "" {
		*n = PatentNumber{}
		return nil
	}
	parsed, err := ParsePatentNumber(s)
	if err != nil {
		return err
	}
	*n = parsed
	return nil
}
