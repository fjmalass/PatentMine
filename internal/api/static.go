package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const webUIPrefix = "/ui/"

// registerWebUI serves the built SPA under /ui/ when a web root is configured.
func (s *Server) registerWebUI() {
	if s.webDir == "" {
		return
	}
	root, err := filepath.Abs(s.webDir)
	if err != nil {
		return
	}
	if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
		return
	}

	s.mux.HandleFunc("GET /ui/{path...}", func(w http.ResponseWriter, r *http.Request) {
		path := r.PathValue("path")

		// Serve index.html for the UI root
		if path == "" || path == "/" {
			http.ServeFile(w, r, filepath.Join(root, "index.html"))
			return
		}

		// Try to serve the requested file
		fullPath := filepath.Join(root, path)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, fullPath)
			return
		}

		// Fallback to index.html for SPA client-side routing
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	})
}

// SecurityFromEnv builds API security settings from environment variables.
func SecurityFromEnv() SecurityConfig {
	origins := strings.TrimSpace(os.Getenv("PATENTMINE_API_CORS_ORIGINS"))
	var cors []string
	if origins != "" {
		for _, o := range strings.Split(origins, ",") {
			if t := strings.TrimSpace(o); t != "" {
				cors = append(cors, t)
			}
		}
	}
	return SecurityConfig{
		APIToken:    strings.TrimSpace(os.Getenv("PATENTMINE_API_TOKEN")),
		CORSOrigins: cors,
		TLSCert:     strings.TrimSpace(os.Getenv("PATENTMINE_API_TLS_CERT")),
		TLSKey:      strings.TrimSpace(os.Getenv("PATENTMINE_API_TLS_KEY")),
	}
}

// AIConfigFromAppConfig maps application config into API AI settings.
func AIConfigFromAppConfig(provider, geminiKey, ollamaHost, ollamaModel string) AIConfig {
	return AIConfig{
		Provider:     provider,
		GeminiAPIKey: geminiKey,
		OllamaHost:   ollamaHost,
		OllamaModel:  ollamaModel,
	}
}
