package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyCLI(t *testing.T) {
	tests := []struct {
		name           string
		initialSource  ImportSource
		cliSource      string
		cliKey         string
		expectedSource ImportSource
		expectedKey    string
	}{
		{
			name:           "CLI overrides default",
			initialSource:  ImportSourceGoogle,
			cliSource:      "uspto",
			cliKey:         "test-key",
			expectedSource: ImportSourceUSPTO,
			expectedKey:    "test-key",
		},
		{
			name:           "Empty CLI source keeps initial",
			initialSource:  ImportSourceUSPTO,
			cliSource:      "",
			cliKey:         "new-key",
			expectedSource: ImportSourceUSPTO,
			expectedKey:    "new-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{ImportSource: tt.initialSource}
			ApplyCLI(&cfg, tt.cliSource, tt.cliKey, "")
			if cfg.ImportSource != tt.expectedSource {
				t.Errorf("expected source %v, got %v", tt.expectedSource, cfg.ImportSource)
			}
			if cfg.USPTO.APIKey != tt.expectedKey {
				t.Errorf("expected key %v, got %v", tt.expectedKey, cfg.USPTO.APIKey)
			}
		})
	}
}

func TestLoadReadsDotEnv(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := os.WriteFile(".env", []byte("PATENTMINE_IMPORT_SOURCE=uspto\nUSPTO_API_KEY=dotenv-key\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.ImportSource != ImportSourceUSPTO {
		t.Fatalf("expected .env import source %q, got %q", ImportSourceUSPTO, cfg.ImportSource)
	}
	if cfg.USPTO.APIKey != "dotenv-key" {
		t.Fatalf("expected .env API key, got %q", cfg.USPTO.APIKey)
	}
}

func TestLoadRealEnvOverridesDotEnv(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := os.WriteFile(".env", []byte("PATENTMINE_IMPORT_SOURCE=uspto\nUSPTO_API_KEY=dotenv-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATENTMINE_IMPORT_SOURCE", "google")
	t.Setenv("USPTO_API_KEY", "real-env-key")

	cfg := Load()
	if cfg.ImportSource != ImportSourceGoogle {
		t.Fatalf("expected process env import source %q, got %q", ImportSourceGoogle, cfg.ImportSource)
	}
	if cfg.USPTO.APIKey != "real-env-key" {
		t.Fatalf("expected process env API key, got %q", cfg.USPTO.APIKey)
	}
}

func TestLoadDotEnvCanReadKeyFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	keyPath := filepath.Join(dir, "uspto.key")
	if err := os.WriteFile(keyPath, []byte("file-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".env", []byte("USPTO_API_KEY_FILE="+keyPath+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.USPTO.APIKey != "file-key" {
		t.Fatalf("expected key from .env key file, got %q", cfg.USPTO.APIKey)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatal(err)
		}
	})
}
