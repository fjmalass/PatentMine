package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ImportSource controls which data source is used to fetch patent data.
type ImportSource string

const (
	ImportSourceGoogle ImportSource = "google"
	ImportSourceUSPTO  ImportSource = "uspto"
)

// Config holds runtime configuration for PatentMine.
type Config struct {
	// ImportSource selects the patent data provider (google or uspto).
	ImportSource ImportSource
	// USPTOAPIKey is the X-API-KEY for the USPTO Open Data Portal.
	USPTOAPIKey string
}

// Load reads config from (lowest to highest priority):
//  1. ~/.config/patentmine/config.toml
//  2. USPTO_API_KEY_FILE env var (path to file containing the key)
//  3. USPTO_API_KEY env var (raw key value)
//  4. PATENTMINE_IMPORT_SOURCE env var
func Load() Config {
	cfg := Config{ImportSource: ImportSourceGoogle}

	if file, err := loadFile(); err == nil {
		if v := file["import_source"]; v != "" {
			cfg.ImportSource = ImportSource(v)
		}
		if v := file["uspto_api_key"]; v != "" {
			cfg.USPTOAPIKey = v
		}
		if path := file["uspto_api_key_file"]; path != "" {
			if key := readFile(path); key != "" {
				cfg.USPTOAPIKey = key
			}
		}
	}

	if path := os.Getenv("USPTO_API_KEY_FILE"); path != "" {
		if key := readFile(path); key != "" {
			cfg.USPTOAPIKey = key
		}
	}
	if v := os.Getenv("USPTO_API_KEY"); v != "" {
		cfg.USPTOAPIKey = v
	}
	if v := os.Getenv("PATENTMINE_IMPORT_SOURCE"); v != "" {
		cfg.ImportSource = ImportSource(v)
	}

	return cfg
}

func loadFile() (map[string]string, error) {
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

	result := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		result[key] = val
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
