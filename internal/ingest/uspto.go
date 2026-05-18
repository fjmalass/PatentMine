package ingest

import (
	"fmt"
	"time"

	"patentmine/internal/domain"
)

// usptoMinInterval keeps requests to the USPTO endpoint polite.
const usptoMinInterval = 1 * time.Second

// NewUSPTOSource builds a Source backed by the USPTO patent data endpoint.
func NewUSPTOSource() Source {
	return newHTTPSource(
		domain.SourceUSPTO,
		usptoMinInterval,
		func(n domain.PatentNumber) string {
			return "https://ped.uspto.gov/api/queries/patent/" + n.Normalized()
		},
		parseUSPTO,
	)
}

// parseUSPTO extracts a patent and its family edges from a USPTO API response.
// Implementing the JSON parser is the work that makes this provider live.
func parseUSPTO(domain.PatentNumber, []byte) (Result, error) {
	return Result{}, fmt.Errorf("ingest/uspto: response parser not yet implemented")
}
