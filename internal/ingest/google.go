package ingest

import (
	"fmt"
	"time"

	"patentmine/internal/domain"
)

// googleMinInterval keeps requests to Google Patents polite.
const googleMinInterval = 2 * time.Second

// NewGoogleSource builds a Source backed by Google Patents.
func NewGoogleSource() Source {
	return newHTTPSource(
		domain.SourceGoogle,
		googleMinInterval,
		func(n domain.PatentNumber) string {
			return "https://patents.google.com/patent/" + n.Normalized() + "/en"
		},
		parseGoogle,
	)
}

// parseGoogle extracts a patent and its family edges from a Google Patents
// page. Implementing the HTML parser is the work that makes this provider live.
func parseGoogle(domain.PatentNumber, []byte) (Result, error) {
	return Result{}, fmt.Errorf("ingest/google: response parser not yet implemented")
}
