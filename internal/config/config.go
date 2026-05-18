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

// Config holds the resolved paths the process needs.
type Config struct {
	HomeDir    string // Base directory; created if absent.
	DBPath     string // SQLite database file.
	LogsDir    string // Runtime logs and activity directory.
	PIDPath    string // Daemon pid file.
	SocketPath string // Unix domain socket for the daemon.
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
	return Config{
		HomeDir:    home,
		DBPath:     filepath.Join(home, dbFileName),
		LogsDir:    filepath.Join(home, logsDirName),
		PIDPath:    filepath.Join(home, pidFileName),
		SocketPath: filepath.Join(home, socketFileName),
	}, nil
}
