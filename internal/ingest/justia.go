package ingest

import (
	"fmt"
	"time"

	"patentmine/internal/domain"
)

// justiaMinInterval keeps requests to Justia Patents polite.
const justiaMinInterval = 2 * time.Second

// NewJustiaSource builds a Source backed by Justia Patents.
func NewJustiaSource() Source {
	return newHTTPSource(
		domain.SourceJustia,
		justiaMinInterval,
		func(n domain.PatentNumber) string {
			return "https://patents.justia.com/patent/" + n.Serial
		},
		parseJustia,
	)
}

// parseJustia extracts a patent and its family edges from a Justia Patents
// page. Implementing the HTML parser is the work that makes this provider live.
func parseJustia(domain.PatentNumber, []byte) (Result, error) {
	return Result{}, fmt.Errorf("ingest/justia: response parser not yet implemented")
}
