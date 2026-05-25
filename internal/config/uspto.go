package config

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	usptoCheckURL    = "https://api.uspto.gov/api/v1/patent/applications/search?patentApplicationNumber=%s"
	usptoAppNumber   = "16123456"
	usptoCheckTimeout = 10 * time.Second
)

// USPTOConnectionResult holds the result of a USPTO API connectivity check.
type USPTOConnectionResult struct {
	Connected bool
	Remaining string
	Limit     string
	Message   string
}

// ResolveAPIKey resolves a raw API key string. If the key starts with "file:",
// it reads the key from the specified file path. Supports ~/ for home directory.
func ResolveAPIKey(raw string) (string, error) {
	if !strings.HasPrefix(raw, "file:") {
		return raw, nil
	}
	path := strings.TrimSpace(strings.TrimPrefix(raw, "file:"))
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand ~: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// CheckUSPTOConnection tests connectivity to the USPTO API using the given key.
func CheckUSPTOConnection(ctx context.Context, key string) USPTOConnectionResult {
	url := fmt.Sprintf(usptoCheckURL, usptoAppNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return USPTOConnectionResult{Message: err.Error()}
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: usptoCheckTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return USPTOConnectionResult{
			Connected: false,
			Message:   fmt.Sprintf("connection failed: %s", err.Error()),
		}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	remaining := resp.Header.Get("X-RateLimit-Remaining")
	limit := resp.Header.Get("X-RateLimit-Limit")

	msg := "key loaded and API reachable"
	if remaining != "" || limit != "" {
		msg += fmt.Sprintf(" (rate limit: %s/%s remaining)", remaining, limit)
	}

	return USPTOConnectionResult{
		Connected: resp.StatusCode >= 200 && resp.StatusCode < 300,
		Remaining: remaining,
		Limit:     limit,
		Message:   msg,
	}
}
