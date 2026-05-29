package domain

import "fmt"

// RelationKind names a directed edge between two patents in the family graph.
type RelationKind string

const (
	// RelationCites: From cites To as prior art.
	RelationCites RelationKind = "cites"
	// RelationCitedBy: From is cited by To (the inverse of RelationCites).
	RelationCitedBy RelationKind = "cited_by"
	// RelationParent: To is a parent application of From (e.g. continuation).
	RelationParent RelationKind = "parent"
	// RelationChild: To is a child application of From.
	RelationChild RelationKind = "child"
)

// Valid reports whether the RelationKind is a known value.
func (k RelationKind) Valid() bool {
	switch k {
	case RelationCites, RelationCitedBy, RelationParent, RelationChild:
		return true
	default:
		return false
	}
}

// Inverse returns the edge as seen from the other endpoint.
func (k RelationKind) Inverse() RelationKind {
	switch k {
	case RelationCites:
		return RelationCitedBy
	case RelationCitedBy:
		return RelationCites
	case RelationParent:
		return RelationChild
	case RelationChild:
		return RelationParent
	default:
		return k
	}
}

// ParseRelationKind converts a string into a RelationKind.
func ParseRelationKind(s string) (RelationKind, error) {
	k := RelationKind(s)
	if !k.Valid() {
		return "", fmt.Errorf("domain: unknown relation kind %q", s)
	}
	return k, nil
}

// Relation is one directed edge of the patent family graph. Source records
// which crawl produced the edge (e.g. google, uspto). The graph keeps one edge
// per (From, To, Kind, Source) so the same citation observed by two sources is
// stored twice and can be compared or reloaded independently.
type Relation struct {
	From   PatentNumber `json:"from"`
	To     PatentNumber `json:"to"`
	Kind   RelationKind `json:"kind"`
	Source Source       `json:"source,omitempty"`
}
