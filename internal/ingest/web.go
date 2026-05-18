package ingest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"patentmine/internal/domain"
)

// HTTP source tuning.
const (
	httpTimeout  = 30 * time.Second
	maxBodyBytes = 8 << 20 // 8 MiB cap on a response body.
)

// parseFunc turns a provider's raw response body into a Result.
type parseFunc func(number domain.PatentNumber, body []byte) (Result, error)

// httpSource is the shared scaffold for web-based sources. It owns the polite
// rate limiting, request building, and status handling; each provider supplies
// only a URL builder and a response parser.
type httpSource struct {
	name    domain.Source
	client  *http.Client
	limiter *limiter
	urlFor  func(number domain.PatentNumber) string
	parse   parseFunc
}

// newHTTPSource builds an httpSource with a polite minimum request interval.
func newHTTPSource(name domain.Source, minInterval time.Duration, urlFor func(domain.PatentNumber) string, parse parseFunc) *httpSource {
	return &httpSource{
		name:    name,
		client:  &http.Client{Timeout: httpTimeout},
		limiter: newLimiter(minInterval),
		urlFor:  urlFor,
		parse:   parse,
	}
}

// Name implements Source.
func (s *httpSource) Name() domain.Source { return s.name }

// Fetch implements Source: it rate-limits, performs the GET, maps the status,
// and delegates body parsing to the provider's parseFunc.
func (s *httpSource) Fetch(ctx context.Context, number domain.PatentNumber) (Result, error) {
	if err := s.limiter.wait(ctx); err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.urlFor(number), nil)
	if err != nil {
		return Result{}, fmt.Errorf("ingest/%s: build request: %w", s.name, err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("ingest/%s: %w", s.name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through to parsing
	case http.StatusNotFound:
		return Result{}, ErrNotAvailable
	default:
		return Result{}, fmt.Errorf("ingest/%s: unexpected status %d", s.name, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return Result{}, fmt.Errorf("ingest/%s: read body: %w", s.name, err)
	}
	return s.parse(number, body)
}
