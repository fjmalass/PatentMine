package crawl

import (
	"fmt"
	"time"

	"patentmine/internal/domain"
)

// pctMinInterval keeps requests to WIPO PATENTSCOPE polite.
const pctMinInterval = 2 * time.Second

// NewPCTSource builds a Source backed by WIPO PATENTSCOPE (the PCT system).
func NewPCTSource() Source {
	return newHTTPSource(
		domain.SourcePCT,
		pctMinInterval,
		func(n domain.PatentNumber) string {
			return "https://patentscope.wipo.int/search/en/detail.jsf?docId=" + n.Normalized()
		},
		parsePCT,
	)
}

// parsePCT extracts a patent and its family edges from a PATENTSCOPE response.
// Implementing the parser is the work that makes this provider live.
func parsePCT(domain.PatentNumber, []byte) (Result, error) {
	return Result{}, fmt.Errorf("crawl/pct: response parser not yet implemented")
}
