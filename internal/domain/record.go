package domain

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// RecordID is the opaque surrogate primary key for a patent record. It is
// generated as a UUIDv7 so the values are globally unique (multi-database
// safe) and roughly time-ordered for cache-friendly inserts.
type RecordID string

// NewRecordID returns a freshly minted RecordID. UUIDv7 carries a millisecond
// timestamp prefix; if generation fails the function falls back to UUIDv4 so a
// caller never observes an empty id.
func NewRecordID() RecordID {
	if id, err := uuid.NewV7(); err == nil {
		return RecordID(id.String())
	}
	return RecordID(uuid.NewString())
}

// ParseRecordID validates and normalizes a string form. It accepts any UUID
// variant so values backfilled by the SQLite migration (which uses
// hex(randomblob(16))) remain interchangeable with UUIDv7s minted in Go.
func ParseRecordID(s string) (RecordID, error) {
	if s == "" {
		return "", nil
	}
	if _, err := uuid.Parse(s); err != nil {
		return "", fmt.Errorf("domain: invalid record id %q: %w", s, err)
	}
	return RecordID(s), nil
}

// IsZero reports whether the id is empty.
func (r RecordID) IsZero() bool { return r == "" }

// String implements fmt.Stringer.
func (r RecordID) String() string { return string(r) }

// MarshalJSON encodes a RecordID as a plain JSON string.
func (r RecordID) MarshalJSON() ([]byte, error) { return json.Marshal(string(r)) }

// UnmarshalJSON parses a plain JSON string back into a RecordID.
func (r *RecordID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*r = RecordID(s)
	return nil
}

// AuthorityRef names one external identifier that maps to a patent record. It
// is the input to the cross-source resolver (authority_identifier table).
//
// Authority is the source/issuing body ("US", "GOOGLE", "PCT", "WO", a foreign
// office name). IdentifierType is the role of the value ("application",
// "publication", "grant", "priority"). Identifier is the bare value, no
// country prefix.
type AuthorityRef struct {
	Authority      string `json:"authority"`
	IdentifierType string `json:"identifier_type"`
	Identifier     string `json:"identifier"`
}

// IsZero reports whether the ref is unusable (any required field empty).
func (a AuthorityRef) IsZero() bool {
	return a.Authority == "" || a.IdentifierType == "" || a.Identifier == ""
}

// String implements fmt.Stringer in a stable, log-safe form.
func (a AuthorityRef) String() string {
	return a.Authority + "/" + a.IdentifierType + "/" + a.Identifier
}
