// Package config resolves filesystem locations shared by every entrypoint:
// the daemon socket and the database file. Centralizing them keeps paths out
// of magic literals scattered across subcommands.
package config

import (
	"fmt"
	"os"
	"path/filepath"
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
	HomeDir    Path // Base directory; created if absent.
	DBPath     Path // SQLite database file.
	LogsDir    Path // Runtime logs and activity directory.
	PIDPath    Path // Daemon pid file.
	SocketPath Path // Unix domain socket for the daemon.
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
	logsDir := filepath.Join(home, logsDirName)
	if err := os.MkdirAll(logsDir, dirPerm); err != nil {
		return Config{}, fmt.Errorf("config: create logs dir %q: %w", logsDir, err)
	}
	return Config{
		HomeDir:    Path(home),
		DBPath:     Path(filepath.Join(home, dbFileName)),
		LogsDir:    Path(logsDir),
		PIDPath:    Path(filepath.Join(home, pidFileName)),
		SocketPath: Path(filepath.Join(home, socketFileName)),
	}, nil
}
