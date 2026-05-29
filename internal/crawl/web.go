package crawl

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
)

// HTTP source tuning.
const (
	httpTimeout  = 30 * time.Second
	maxBodyBytes = 8 << 20 // 8 MiB cap on a response body.
)

// parseFunc turns a provider's raw response body into a Result.
type parseFunc func(ctx context.Context, number domain.PatentNumber, body []byte) (Result, error)

// httpSource is the shared scaffold for web-based sources. It owns the polite
// rate limiting, request building, and status handling; each provider supplies
// only a URL builder and a response parser.
type httpSource struct {
	name    domain.Source
	client  *http.Client
	limiter *limiter
	urlFor  func(number domain.PatentNumber) string
	headers func() http.Header
	parse   parseFunc
	metrics *observability.Metrics
	logger  *slog.Logger
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

// WithMetrics attaches request-level telemetry for this HTTP source.
func (s *httpSource) WithMetrics(metrics *observability.Metrics) { s.metrics = metrics }

// WithLogger attaches structured request logging for this HTTP source.
func (s *httpSource) WithLogger(logger *slog.Logger) { s.logger = logger }

// SetInterval updates the request-spacing interval dynamically.
func (s *httpSource) SetInterval(d time.Duration) {
	if s.limiter != nil {
		s.limiter.SetInterval(d)
	}
}

// Fetch implements Source: it rate-limits, performs the GET, maps the status,
// and delegates body parsing to the provider's parseFunc.
func (s *httpSource) Fetch(ctx context.Context, number domain.PatentNumber) (Result, error) {
	waitStart := time.Now()
	if err := s.limiter.wait(ctx); err != nil {
		s.observeDelay(time.Since(waitStart), true)
		return Result{}, err
	}
	s.observeDelay(time.Since(waitStart), false)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.urlFor(number), nil)
	if err != nil {
		return Result{}, fmt.Errorf("crawl/%s: build request: %w", s.name, err)
	}
	if s.headers != nil {
		h := s.headers()
		for k, vv := range h {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}
	}
	requestBytes := estimateHTTPRequestBytes(req)
	requestStart := time.Now()
	resp, err := s.client.Do(req)
	if err != nil {
		s.observeHTTP(number, time.Since(requestStart), 0, requestBytes, 0, true, err)
		return Result{}, fmt.Errorf("crawl/%s: %w", s.name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	responseHeaderBytes := estimateHTTPHeaderBytes(resp.Header)

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through to parsing
	case http.StatusNotFound:
		bodyBytes, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))
		s.observeHTTP(number, time.Since(requestStart), resp.StatusCode, requestBytes, responseHeaderBytes+bodyBytes, true, ErrNotAvailable)
		return Result{}, ErrNotAvailable
	default:
		bodyBytes, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))
		err := fmt.Errorf("crawl/%s: unexpected status %d", s.name, resp.StatusCode)
		s.observeHTTP(number, time.Since(requestStart), resp.StatusCode, requestBytes, responseHeaderBytes+bodyBytes, true, err)
		return Result{}, fmt.Errorf("crawl/%s: unexpected status %d", s.name, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		s.observeHTTP(number, time.Since(requestStart), resp.StatusCode, requestBytes, responseHeaderBytes+int64(len(body)), true, err)
		return Result{}, fmt.Errorf("crawl/%s: read body: %w", s.name, err)
	}
	s.observeHTTP(number, time.Since(requestStart), resp.StatusCode, requestBytes, responseHeaderBytes+int64(len(body)), false, nil)
	return s.parse(ctx, number, body)
}

func (s *httpSource) observeDelay(d time.Duration, failed bool) {
	if s.metrics != nil {
		s.metrics.ObserveDuration(string(observability.MetricNamespaceCrawl)+".source."+string(s.name)+".delay", d, failed)
	}
}

func (s *httpSource) observeHTTP(number domain.PatentNumber, d time.Duration, statusCode int, sentBytes, receivedBytes int64, failed bool, err error) {
	if s.metrics != nil {
		prefix := string(observability.MetricNamespaceCrawl) + ".source." + string(s.name) + "."
		s.metrics.ObserveDuration(prefix+"http", d, failed)
		s.metrics.IncCounter(prefix+"sent_bytes_total", sentBytes)
		s.metrics.IncCounter(prefix+"received_bytes_total", receivedBytes)
		if statusCode > 0 {
			s.metrics.SetGauge(prefix+"last_status_code", int64(statusCode))
		}
	}
	if s.logger == nil {
		return
	}
	attrs := []any{
		slog.String("source", string(s.name)),
		slog.String("number", number.String()),
		slog.Int64("duration_ms", d.Milliseconds()),
		slog.Int64("sent_bytes", sentBytes),
		slog.Int64("received_bytes", receivedBytes),
		slog.Bool("failed", failed),
	}
	if statusCode > 0 {
		attrs = append(attrs, slog.Int("status_code", statusCode))
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
		s.logger.Warn("crawl source http request failed", attrs...)
		return
	}
	s.logger.Info("crawl source http request completed", attrs...)
}

func estimateHTTPRequestBytes(req *http.Request) int64 {
	if req == nil || req.URL == nil {
		return 0
	}
	return int64(len(req.Method)+len(req.URL.String())) + estimateHTTPHeaderBytes(req.Header)
}

func estimateHTTPHeaderBytes(header http.Header) int64 {
	var n int64
	for name, values := range header {
		n += int64(len(name))
		for _, value := range values {
			n += int64(len(value))
		}
	}
	return n
}
