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
// use as a map key and for deduplication across ingestion sources.
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

// patentNumberPattern matches an optional 2-letter country code, a run of
// serial digits (which may contain spaces, commas or hyphens), and an optional
// kind code. Examples: "US11611785B2", "US-11,611,785-B2", "EP1234567A1".
var patentNumberPattern = regexp.MustCompile(
	`^([A-Z]{2})?[\s\-]*([0-9][0-9\s,\-]*?)[\s\-]*([A-Z][0-9]?)?$`,
)

var nonDigit = regexp.MustCompile(`[^0-9]`)

// ParsePatentNumber normalizes raw input into a PatentNumber. It uppercases,
// trims, and strips separators so the same patent written differently by
// different sources parses to an equal value.
func ParsePatentNumber(raw string) (PatentNumber, error) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" {
		return PatentNumber{}, ErrEmptyNumber
	}
	m := patentNumberPattern.FindStringSubmatch(s)
	if m == nil {
		return PatentNumber{}, fmt.Errorf("domain: cannot parse patent number %q", raw)
	}
	serial := nonDigit.ReplaceAllString(m[2], "")
	if serial == "" {
		return PatentNumber{}, ErrNoSerial
	}
	return PatentNumber{Country: m[1], Serial: serial, Kind: m[3]}, nil
}

// MustParsePatentNumber is ParsePatentNumber for compile-time-known literals
// (tests, fixtures). It panics on error.
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

// String implements fmt.Stringer.
func (n PatentNumber) String() string {
	return n.Normalized()
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
