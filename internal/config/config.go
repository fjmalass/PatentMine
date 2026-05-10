package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ImportSource selects the patent data provider.
type ImportSource string

const (
	ImportSourceGoogle ImportSource = "google"
	ImportSourceUSPTO  ImportSource = "uspto"
)

// USPTOConfig holds configuration for the USPTO Open Data Portal.
type USPTOConfig struct {
	// APIKey is the resolved X-API-KEY value (loaded from api_key or api_key_file).
	APIKey string
}

// GoogleConfig holds configuration for the Google Patents scraper.
// Reserved for future use (no auth required currently).
type GoogleConfig struct{}

// Config is the top-level application configuration.
type Config struct {
	ImportSource ImportSource
	USPTO        USPTOConfig
	Google       GoogleConfig
}

// Load reads config with the following priority (highest wins):
//
//  1. PATENTMINE_IMPORT_SOURCE / USPTO_API_KEY / USPTO_API_KEY_FILE env vars
//  2. ~/.config/patentmine/config.toml sections
//  3. Built-in defaults (ImportSource = google)
func Load() Config {
	cfg := Config{ImportSource: ImportSourceGoogle}
	applyFile(&cfg)
	applyEnv(&cfg)
	return cfg
}

// ApplyCLI overlays explicit CLI flag values onto an already-loaded Config.
// Empty string means "not set by the user" and is skipped.
func ApplyCLI(cfg *Config, importSource, apiKey, apiKeyFile string) {
	if importSource != "" {
		cfg.ImportSource = ImportSource(importSource)
	}
	if apiKey != "" {
		cfg.USPTO.APIKey = apiKey
	}
	if apiKeyFile != "" {
		if key := readFile(apiKeyFile); key != "" {
			cfg.USPTO.APIKey = key
		}
	}
}

func applyFile(cfg *Config) {
	sections, err := loadFile()
	if err != nil {
		return
	}
	top := sections[""]
	if v := top["import_source"]; v != "" {
		cfg.ImportSource = ImportSource(v)
	}
	if uspto, ok := sections["uspto"]; ok {
		if v := uspto["api_key"]; v != "" {
			cfg.USPTO.APIKey = v
		}
		if path := uspto["api_key_file"]; path != "" {
			if key := readFile(path); key != "" {
				cfg.USPTO.APIKey = key
			}
		}
	}
}

func applyEnv(cfg *Config) {
	if path := os.Getenv("USPTO_API_KEY_FILE"); path != "" {
		if key := readFile(path); key != "" {
			cfg.USPTO.APIKey = key
		}
	}
	if v := os.Getenv("USPTO_API_KEY"); v != "" {
		cfg.USPTO.APIKey = v
	}
	if v := os.Getenv("PATENTMINE_IMPORT_SOURCE"); v != "" {
		cfg.ImportSource = ImportSource(v)
	}
}

// loadFile parses ~/.config/patentmine/config.toml into a map of sections.
// The top-level (pre-section) keys live under the "" key.
func loadFile() (map[string]map[string]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".config", "patentmine", "config.toml")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := map[string]map[string]string{"": {}}
	section := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			if result[section] == nil {
				result[section] = map[string]string{}
			}
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		result[section][key] = val
	}
	return result, scanner.Err()
}

func readFile(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
