package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"patentmine/internal/crawl"
	"patentmine/internal/domain"
)

const (
	// EnvHome overrides the home directory; useful for tests and isolation.
	EnvHome = "PATENTMINE_HOME"
	// BackupProviderB2 is the Backblaze B2 provider identifier used in config.
	BackupProviderB2 = "b2"
	// DefaultBackupRcloneRemote is the remote name used when none is configured.
	DefaultBackupRcloneRemote = BackupProviderB2
	// RcloneExecutable is the command used for rclone-backed backup checks.
	RcloneExecutable = "rclone"
	// RcloneListDirCommand lists remote directories during connectivity checks.
	RcloneListDirCommand = "lsd"

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
	HomeDir            Path   // Base directory; created if absent.
	DBPath             Path   // SQLite database file.
	LogsDir            Path   // Runtime logs and activity directory.
	PIDPath            Path   // Daemon pid file.
	SocketPath         Path   // Unix domain socket for the daemon.
	USPTOAPIKey        string // USPTO Open Data Portal API Key.
	SourceMode         domain.SourceMode // Provider policy: compare, uspto-first, uspto-only, google-only.
	GeminiAPIKey       string // Google Gemini Developer API Key.
	AIProvider         string // Chosen AI Provider ("gemini", "ollama")
	OllamaModel        string // Local Ollama Model ("mistral", etc.)
	OllamaHost         string // Local Ollama server host
	CrawlWorkers       int    // Concurrent crawl goroutines; 0 means use engine default.
	ActivityMinMS      int    // Minimum UI look/hover duration to record.
	NotesExportDir     string // Directory for exported notes .md files; empty means user home dir.
	IDSExportDir       string // Directory for exported IDS PDF bundles; empty means $HomeDir/exports.
	BackupProvider     string // Backup provider name, e.g. b2.
	BackupBucket       string // Remote backup bucket/container name.
	BackupB2KeyID      string // Backblaze B2 application key ID.
	BackupB2Secret     string // Backblaze B2 application key secret.
	BackupB2APIURL     string // Backblaze B2 native API base URL.
	BackupRcloneRemote string // rclone remote name used for backup checks.
	LogRetainDays      int    // Number of days of log/activity files to keep.
	LogMaxSizeBytes    int64  // Maximum size limit for the logs directory in bytes.
}

// BackupConfigured reports whether backup settings are present. Connectivity is
// checked separately by the diagnostics command because startup paths should not
// block on external services.
func (c Config) BackupConfigured() bool {
	if strings.TrimSpace(c.BackupBucket) == "" {
		return false
	}
	if strings.TrimSpace(c.BackupRcloneRemote) != "" {
		return true
	}
	return strings.TrimSpace(c.BackupB2KeyID) != "" && strings.TrimSpace(c.BackupB2Secret) != ""
}

// loadDotEnv searches for and loads environment variables from a .env file.
func loadDotEnv(paths ...string) []string {
	var loaded []string
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
				loaded = append(loaded, key)
			}
		}
		_ = f.Close()
	}
	return loaded
}

func resolveEnvRefs(keys []string) {
	for range 10 {
		changed := false
		for _, key := range keys {
			value := os.Getenv(key)
			if !strings.Contains(value, "${") {
				continue
			}
			resolved := expandBraceEnv(value)
			if resolved != value {
				_ = os.Setenv(key, resolved)
				changed = true
			}
		}
		if !changed {
			break
		}
	}
}

func expandBraceEnv(value string) string {
	var b strings.Builder
	for {
		start := strings.Index(value, "${")
		if start == -1 {
			b.WriteString(value)
			return b.String()
		}
		b.WriteString(value[:start])
		rest := value[start+2:]
		end := strings.IndexByte(rest, '}')
		if end == -1 {
			b.WriteString(value[start:])
			return b.String()
		}
		b.WriteString(os.Getenv(rest[:end]))
		value = rest[end+1:]
	}
}

func resolveFileRefs(keys []string) {
	for _, key := range keys {
		value := os.Getenv(key)
		if !strings.HasPrefix(value, "file:") {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(value, "file:"))
		path = expandHomePath(expandBraceEnv(path))
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		_ = os.Setenv(key, strings.TrimSpace(string(data)))
	}
}

func expandHomePath(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
	loaded := loadDotEnv(".env", sshEnvPath, filepath.Join(home, ".env"))
	resolveEnvRefs(loaded)
	resolveFileRefs(loaded)

	logsDir := filepath.Join(home, logsDirName)
	if err := os.MkdirAll(logsDir, dirPerm); err != nil {
		return Config{}, fmt.Errorf("config: create logs dir %q: %w", logsDir, err)
	}

	usptoKey := os.Getenv("PATENTMINE_USPTO_API_KEY")
	if usptoKey == "" {
		usptoKey = os.Getenv("USPTO_API_KEY")
	}
	// PATENTMINE_SOURCE_MODE feeds crawl.NormalizeSourceMode so the canonical
	// names live in one place. An invalid value silently falls back to
	// compare so a typo doesn't refuse to boot.
	sourceMode, err := crawl.NormalizeSourceMode(os.Getenv("PATENTMINE_SOURCE_MODE"))
	if err != nil {
		sourceMode = crawl.SourceModeCompare
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

	notesExportDir := os.Getenv("PATENTMINE_NOTES_EXPORT_DIR")
	idsExportDir := os.Getenv("PATENTMINE_IDS_EXPORT_DIR")
	if idsExportDir == "" {
		idsExportDir = filepath.Join(home, "exports")
	}
	backupProvider := os.Getenv("PATENTMINE_BACKUP_PROVIDER")
	backupBucket := os.Getenv("PATENTMINE_BACKUP_BUCKET")
	backupB2KeyID := firstNonEmpty(
		os.Getenv("PATENTMINE_BACKUP_B2_KEY_ID"),
		os.Getenv("PATENTMINE_BACKUP_ACCESS_KEY_ID"),
		os.Getenv("B2_APPLICATION_KEY_ID"),
	)
	backupB2Secret := firstNonEmpty(
		os.Getenv("PATENTMINE_BACKUP_B2_SECRET"),
		os.Getenv("PATENTMINE_BACKUP_SECRET_ACCESS_KEY"),
		os.Getenv("B2_APPLICATION_KEY"),
	)
	backupB2APIURL := os.Getenv("PATENTMINE_BACKUP_B2_API_URL")
	if backupB2APIURL == "" {
		backupB2APIURL = "https://api.backblazeb2.com"
	}
	backupRcloneRemote := os.Getenv("PATENTMINE_BACKUP_RCLONE_REMOTE")
	if backupRcloneRemote == "" {
		backupRcloneRemote = DefaultBackupRcloneRemote
	}

	logRetainDays, _ := strconv.Atoi(os.Getenv("PATENTMINE_LOG_RETAIN_DAYS"))
	if logRetainDays <= 0 {
		logRetainDays = 14
	}

	logMaxSizeBytesStr := os.Getenv("PATENTMINE_LOG_MAX_SIZE_BYTES")
	var logMaxSizeBytes int64
	if logMaxSizeBytesStr != "" {
		if val, err := strconv.ParseInt(logMaxSizeBytesStr, 10, 64); err == nil && val > 0 {
			logMaxSizeBytes = val
		}
	}
	if logMaxSizeBytes <= 0 {
		logMaxSizeBytes = 100 * 1024 * 1024 // Default: 100MB
	}

	return Config{
		HomeDir:            Path(home),
		DBPath:             Path(filepath.Join(home, dbFileName)),
		LogsDir:            Path(logsDir),
		PIDPath:            Path(filepath.Join(home, pidFileName)),
		SocketPath:         Path(filepath.Join(home, socketFileName)),
		USPTOAPIKey:        usptoKey,
		SourceMode:         sourceMode,
		GeminiAPIKey:       geminiKey,
		AIProvider:         aiProvider,
		OllamaModel:        ollamaModel,
		OllamaHost:         ollamaHost,
		CrawlWorkers:       crawlWorkers,
		ActivityMinMS:      activityMinMS,
		NotesExportDir:     notesExportDir,
		IDSExportDir:       idsExportDir,
		BackupProvider:     backupProvider,
		BackupBucket:       backupBucket,
		BackupB2KeyID:      backupB2KeyID,
		BackupB2Secret:     backupB2Secret,
		BackupB2APIURL:     backupB2APIURL,
		BackupRcloneRemote: backupRcloneRemote,
		LogRetainDays:      logRetainDays,
		LogMaxSizeBytes:    logMaxSizeBytes,
	}, nil
}
