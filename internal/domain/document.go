package domain

import (
	"fmt"
	"strings"
	"time"
)

// Stage is one point in a patent's life cycle. A record holds an open-ended
// set of documents, each tagged with the stage it represents — so the
// application, the pre-grant publication, and the grant of one invention are
// one record with three documents.
type Stage string

const (
	// StageApplication is the filed application.
	StageApplication Stage = "application"
	// StagePublication is a pre-grant publication.
	StagePublication Stage = "publication"
	// StageGrant is the granted patent.
	StageGrant Stage = "grant"
)

// Valid reports whether the Stage is a known value.
func (s Stage) Valid() bool {
	switch s {
	case StageApplication, StagePublication, StageGrant:
		return true
	default:
		return false
	}
}

// ParseStage converts a string into a Stage.
func ParseStage(s string) (Stage, error) {
	st := Stage(s)
	if !st.Valid() {
		return "", fmt.Errorf("domain: unknown stage %q", s)
	}
	return st, nil
}

// rank orders stages for headline selection: a later stage outranks an earlier
// one, so a granted patent is shown by its grant number.
func (s Stage) rank() int {
	switch s {
	case StageGrant:
		return 3
	case StagePublication:
		return 2
	case StageApplication:
		return 1
	default:
		return 0
	}
}

// Document is one official number a patent record carries at one life stage.
type Document struct {
	Number PatentNumber `json:"number"`
	Stage  Stage        `json:"stage"`
	Dated  time.Time    `json:"dated"` // Zero when the stage date is unknown.
}

// GuessStage infers a life stage from a number's kind code, for a document
// discovered (e.g. as a citation) without an explicit stage. A "B" kind is a
// grant; anything else is treated as a publication. An application is only
// recognised when stated explicitly — application numbers carry no kind code.
func GuessStage(n PatentNumber) Stage {
	if strings.HasPrefix(n.Kind, "B") {
		return StageGrant
	}
	if n.Country == "US" && n.Kind == "" {
		return StageApplication
	}
	return StagePublication
}
