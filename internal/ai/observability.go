package ai

import (
	"net/http"
	"net/url"
)

// RedactedRoundTripper intercepts HTTP requests and redacts sensitive credentials.
type RedactedRoundTripper struct {
	next http.RoundTripper
}

// NewRedactedRoundTripper builds a new RoundTripper wrapping the provided transport.
func NewRedactedRoundTripper(next http.RoundTripper) *RedactedRoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &RedactedRoundTripper{next: next}
}

// RoundTrip clones the request, redacts credential headers/query variables, and forwards it.
func (r *RedactedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid mutating request objects shared concurrently
	cloned := req.Clone(req.Context())

	// Scrub sensitive headers
	sensitiveHeaders := []string{"Authorization", "x-api-key", "x-goog-api-key"}
	for _, h := range sensitiveHeaders {
		if cloned.Header.Get(h) != "" {
			cloned.Header.Set(h, "[REDACTED]")
		}
	}

	// Scrub ?key= query parameters in URLs (e.g. Gemini URL authentication)
	if cloned.URL != nil && cloned.URL.Query().Get("key") != "" {
		q := cloned.URL.Query()
		q.Set("key", "[REDACTED]")
		cloned.URL.RawQuery = q.Encode()
	}

	return r.next.RoundTrip(cloned)
}

// ScrubURL removes sensitive keys from a URL string for safe logging.
func ScrubURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if u.Query().Get("key") != "" {
		q := u.Query()
		q.Set("key", "[REDACTED]")
		u.RawQuery = q.Encode()
	}
	return u.String()
}
