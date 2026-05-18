// Package ingest pulls patents and family-graph edges from the web. A Source
// fetches one patent from one provider; the Crawler walks the family graph
// across sources and writes the result to the store.
package ingest

import (
	"context"
	"errors"

	"patentmine/internal/domain"
)

// ErrNotAvailable means a Source has no record for the requested patent, so
// the Crawler should try the next source.
var ErrNotAvailable = errors.New("ingest: patent not available from this source")

// Result is one patent fetched from a Source: the record's bibliographic data,
// its life-stage documents (application, publication, grant), and the
// family-graph edges discovered alongside it.
type Result struct {
	Patent    domain.Patent
	Documents []domain.Document
	Relations []domain.Relation
}

// Source fetches a single patent from one provider.
type Source interface {
	// Name reports which provider this is.
	Name() domain.Source
	// Fetch retrieves one patent; it returns ErrNotAvailable when the provider
	// has no record for the number.
	Fetch(ctx context.Context, number domain.PatentNumber) (Result, error)
}

// Registry holds an ordered set of sources. Fetch tries them in turn so a
// preferred provider can be consulted before a fallback.
type Registry struct {
	sources []Source
}

// NewRegistry builds a Registry from sources, consulted in the given order.
func NewRegistry(sources ...Source) *Registry {
	return &Registry{sources: sources}
}

// Fetch returns the first successful result. A source reporting ErrNotAvailable
// is skipped; a hard error is remembered but the next source is still tried.
func (r *Registry) Fetch(ctx context.Context, number domain.PatentNumber) (Result, error) {
	lastErr := error(ErrNotAvailable)
	for _, s := range r.sources {
		res, err := s.Fetch(ctx, number)
		if err == nil {
			return res, nil
		}
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		if !errors.Is(err, ErrNotAvailable) {
			lastErr = err
		}
	}
	return Result{}, lastErr
}
