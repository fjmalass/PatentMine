package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// EnvHome overrides the home directory; useful for tests and isolation.
	EnvHome = "PATENTMINE_HOME"

	dbFileName     = "patentmine.db"
	logsDirName    = "logs"
	pidFileName    = "patentmine.pid"
	socketFileName = "patentmine.sock"
	dirPerm        = 0o755
)

// Path is a filesystem path, typed to prevent accidental assignment of plain
// strings to path fields.
type Path string

// String implements fmt.Stringer.
func (p Path) String() string { return string(p) }

// Config holds the resolved paths the process needs.
type Config struct {
	HomeDir       Path   // Base directory; created if absent.
	DBPath        Path   // SQLite database file.
	LogsDir       Path   // Runtime logs and activity directory.
	PIDPath       Path   // Daemon pid file.
	SocketPath    Path   // Unix domain socket for the daemon.
	USPTOAPIKey   string // USPTO Open Data Portal API Key.
	GeminiAPIKey  string // Google Gemini Developer API Key.
	AIProvider    string // Chosen AI Provider ("gemini", "ollama")
	OllamaModel   string // Local Ollama Model ("mistral", etc.)
	OllamaHost    string // Local Ollama server host
	CrawlWorkers  int    // Concurrent crawl goroutines; 0 means use engine default.
	ActivityMinMS int    // Minimum UI look/hover duration to record.
}

// loadDotEnv searches for and loads environment variables from a .env file.
func loadDotEnv(paths ...string) {
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
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
			value := strings.TrimSpace(parts[1])
			if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
				value = value[1 : len(value)-1]
			}
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, value)
			}
		}
		_ = f.Close()
	}
}

// Load resolves the configuration, creating the home directory if needed.
// The home directory is $PATENTMINE_HOME, or <user-config-dir>/patentmine.
func Load() (Config, error) {
	home := os.Getenv(EnvHome)
	if home == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return Config{}, fmt.Errorf("config: locate config dir: %w", err)
		}
		home = filepath.Join(base, "patentmine")
	}
	if err := os.MkdirAll(home, dirPerm); err != nil {
		return Config{}, fmt.Errorf("config: create home %q: %w", home, err)
	}

	// Try loading .env from current directory first, ~/.ssh/patentmine/.env, then home directory
	var sshEnvPath string
	if userHome, err := os.UserHomeDir(); err == nil {
		sshDir := filepath.Join(userHome, ".ssh", "patentmine")
		_ = os.MkdirAll(sshDir, 0o700) // Ensure secure ssh keys subfolder exists
		sshEnvPath = filepath.Join(sshDir, ".env")
	}
	loadDotEnv(".env", sshEnvPath, filepath.Join(home, ".env"))

	logsDir := filepath.Join(home, logsDirName)
	if err := os.MkdirAll(logsDir, dirPerm); err != nil {
		return Config{}, fmt.Errorf("config: create logs dir %q: %w", logsDir, err)
	}

	usptoKey := os.Getenv("PATENTMINE_USPTO_API_KEY")
	if usptoKey == "" {
		usptoKey = os.Getenv("USPTO_API_KEY")
	}
	geminiKey := os.Getenv("GEMINI_API_KEY")

	aiProvider := os.Getenv("PATENTMINE_AI_PROVIDER")
	if aiProvider == "" {
		aiProvider = os.Getenv("AI_PROVIDER")
	}
	if aiProvider == "" {
		aiProvider = "gemini"
	}

	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "mistral"
	}

	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}

	crawlWorkers, _ := strconv.Atoi(os.Getenv("PATENTMINE_CRAWL_WORKERS"))
	activityMinMS, _ := strconv.Atoi(os.Getenv("PATENTMINE_ACTIVITY_MIN_MS"))
	if activityMinMS <= 0 {
		activityMinMS = 100
	}

	return Config{
		HomeDir:       Path(home),
		DBPath:        Path(filepath.Join(home, dbFileName)),
		LogsDir:       Path(logsDir),
		PIDPath:       Path(filepath.Join(home, pidFileName)),
		SocketPath:    Path(filepath.Join(home, socketFileName)),
		USPTOAPIKey:   usptoKey,
		GeminiAPIKey:  geminiKey,
		AIProvider:    aiProvider,
		OllamaModel:   ollamaModel,
		OllamaHost:    ollamaHost,
		CrawlWorkers:  crawlWorkers,
		ActivityMinMS: activityMinMS,
	}, nil
}
