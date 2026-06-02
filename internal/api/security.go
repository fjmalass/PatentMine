package api

import (
	"net/http"
	"strings"
)

// SecurityConfig configures HTTP hardening for remote and browser clients.
type SecurityConfig struct {
	// APIToken when non-empty requires Authorization: Bearer <token> on all routes
	// except /healthz and CORS preflight.
	APIToken string
	// CORSOrigins lists allowed Origin values. Empty disables CORS headers.
	CORSOrigins []string
	// TLSCert and TLSKey enable HTTPS directly on the API process when both are set.
	TLSCert string
	TLSKey  string
}

func (c SecurityConfig) corsAllowed(origin string) bool {
	if origin == "" || len(c.CORSOrigins) == 0 {
		return false
	}
	for _, allowed := range c.CORSOrigins {
		if allowed == "*" || strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}

// wrapSecurity applies CORS and optional bearer-token auth around the API mux.
func wrapSecurity(next http.Handler, sec SecurityConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if sec.corsAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			if sec.corsAllowed(origin) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		if sec.APIToken != "" && !isPublicRoute(r) {
			if !bearerAuthorized(r, sec.APIToken) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isPublicRoute(r *http.Request) bool {
	if r.Method == http.MethodOptions {
		return true
	}
	switch r.URL.Path {
	case "/healthz":
		return true
	default:
		return false
	}
}

func bearerAuthorized(r *http.Request, token string) bool {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if strings.HasPrefix(auth, prefix) && strings.TrimSpace(auth[len(prefix):]) == token {
		return true
	}
	// EventSource cannot set Authorization; allow token query on the events stream only.
	if r.URL.Path == "/events" && r.URL.Query().Get("token") == token {
		return true
	}
	return false
}
