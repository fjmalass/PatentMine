package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"patentmine/internal/api"
)

func TestAPISecurityBearerToken(t *testing.T) {
	h := testAPIWithOpts(t, api.WithSecurity(api.SecurityConfig{APIToken: "secret"}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/patents", nil)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /patents without token = %d, want 401", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/patents", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /patents with token = %d, want 200: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /healthz without token = %d, want 200", w.Code)
	}
}

func TestAPICORSPreflight(t *testing.T) {
	h := testAPIWithOpts(t, api.WithSecurity(api.SecurityConfig{
		CORSOrigins: []string{"https://ipad.example"},
	}))

	req := httptest.NewRequest(http.MethodOptions, "/patents", nil)
	req.Header.Set("Origin", "https://ipad.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS = %d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://ipad.example" {
		t.Fatalf("CORS origin = %q", got)
	}
}

func TestAPINewGapRoutesRegistered(t *testing.T) {
	h := testAPI(t)
	w := do(t, h, http.MethodGet, "/session", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /session = %d", w.Code)
	}
	w = do(t, h, http.MethodPost, "/filters/validate", `{"expression":"state:active","project_active":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /filters/validate = %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "canonical") {
		t.Fatalf("validate body = %s", w.Body.String())
	}
}

func TestAPICrawlAcceptsProfile(t *testing.T) {
	h := testAPI(t)
	w := do(t, h, http.MethodPost, "/crawl", `{"root":"US11611785B2","depth":0,"profile":"citations","force":false}`)
	if w.Code == http.StatusNotFound {
		t.Fatalf("POST /crawl missing: %s", w.Body.String())
	}
}
