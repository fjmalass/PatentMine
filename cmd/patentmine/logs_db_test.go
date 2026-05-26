package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"patentmine/internal/config"
	"patentmine/internal/store/sqlite"
)

func TestLogsCommands(t *testing.T) {
	tmpHome, err := os.MkdirTemp("", "patentmine-test-home-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpHome)

	// Set home dir in env
	t.Setenv(config.EnvHome, tmpHome)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	// Create mock log files
	logsDir := string(cfg.LogsDir)
	err = os.MkdirAll(logsDir, 0755)
	if err != nil {
		t.Fatalf("failed to create logs dir: %v", err)
	}

	// 1. Create a log file that is old (for archiving) and one that is current
	nowTime := time.Now()
	oldDateStr := nowTime.AddDate(0, 0, -5).Format("2006-01-02")
	currentDateStr := nowTime.Format("2006-01-02")

	oldLog := filepath.Join(logsDir, "log-"+oldDateStr+".jsonl")
	currentLog := filepath.Join(logsDir, "log-"+currentDateStr+".jsonl")
	oldActivity := filepath.Join(logsDir, "activity-"+oldDateStr+".jsonl")

	_ = os.WriteFile(oldLog, []byte("{\"level\":\"info\",\"msg\":\"old log\"}\n"), 0644)
	_ = os.WriteFile(currentLog, []byte("{\"level\":\"info\",\"msg\":\"new log\"}\n"), 0644)
	_ = os.WriteFile(oldActivity, []byte("{\"action\":\"test\",\"entity\":\"dummy\"}\n"), 0644)

	// Test 1: Logs List
	t.Run("logs list", func(t *testing.T) {
		code := runLogs([]string{"list"})
		if code != 0 {
			t.Errorf("logs list failed with code %d", code)
		}
	})

	// Test 2: Logs Archive
	t.Run("logs archive", func(t *testing.T) {
		// Mock time cutoff to archive files older than current day
		code := runLogs([]string{"archive", "--keep-days=1"})
		if code != 0 {
			t.Errorf("logs archive failed with code %d", code)
		}

		// Ensure old files are archived and removed, and new file is kept
		if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
			t.Errorf("old log %s was not removed after archiving", oldLog)
		}
		if _, err := os.Stat(oldActivity); !os.IsNotExist(err) {
			t.Errorf("old activity %s was not removed after archiving", oldActivity)
		}
		if _, err := os.Stat(currentLog); err != nil {
			t.Errorf("current log %s was incorrectly removed", currentLog)
		}

		// Check that the archive was created in backups
		backupsDir := filepath.Join(tmpHome, "backups")
		entries, err := os.ReadDir(backupsDir)
		if err != nil {
			t.Fatalf("failed to read backups dir: %v", err)
		}
		var foundArchive bool
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "logs-backup-") && strings.HasSuffix(e.Name(), ".tar.gz") {
				foundArchive = true
			}
		}
		if !foundArchive {
			t.Errorf("logs archive failed to create tar.gz backup file")
		}
	})

	// Test 3: Logs Ship
	t.Run("logs ship", func(t *testing.T) {
		var receivedHeader string
		var receivedFilename string
		var receivedBodySize int64

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeader = r.Header.Get("Authorization")
			receivedFilename = r.Header.Get("X-PatentMine-Filename")
			// Read body to count size
			n, _ := ioCopy(ioDiscard{}, r.Body)
			receivedBodySize = n
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		t.Setenv("PATENTMINE_LOG_SHIP_URL", ts.URL)
		t.Setenv("PATENTMINE_LOG_SHIP_AUTH", "secret-token")

		code := runLogs([]string{"ship"})
		if code != 0 {
			t.Errorf("logs ship failed with code %d", code)
		}

		if receivedHeader != "Bearer secret-token" {
			t.Errorf("expected Bearer token, got %q", receivedHeader)
		}
		if !strings.HasPrefix(receivedFilename, "logs-backup-") {
			t.Errorf("expected logs-backup prefix in filename, got %q", receivedFilename)
		}
		if receivedBodySize == 0 {
			t.Errorf("expected non-empty request body size")
		}
	})
}

func TestDBCommands(t *testing.T) {
	tmpHome, err := os.MkdirTemp("", "patentmine-test-db-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpHome)

	t.Setenv(config.EnvHome, tmpHome)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	ctx := context.Background()
	// Open database to initialize the schema
	repo, err := sqlite.Open(ctx, string(cfg.DBPath))
	if err != nil {
		t.Fatalf("failed to open test sqlite: %v", err)
	}
	repo.Close()

	// Test 1: DB Status
	t.Run("db status", func(t *testing.T) {
		code := runDB([]string{"status"})
		if code != 0 {
			t.Errorf("db status failed with code %d", code)
		}
	})

	// Test 2: DB Backup
	t.Run("db backup", func(t *testing.T) {
		backupFile := filepath.Join(tmpHome, "custom-backup.db")
		code := runDB([]string{"backup", "--out", backupFile})
		if code != 0 {
			t.Errorf("db backup failed with code %d", code)
		}

		// Verify backup exists and is queryable
		if _, err := os.Stat(backupFile); err != nil {
			t.Errorf("backup file was not created: %v", err)
		}

		backupRepo, err := sqlite.Open(ctx, backupFile)
		if err != nil {
			t.Errorf("failed to open backed-up database: %v", err)
		} else {
			backupRepo.Close()
		}
	})

	// Test 3: DB Vacuum
	t.Run("db vacuum", func(t *testing.T) {
		code := runDB([]string{"vacuum"})
		if code != 0 {
			t.Errorf("db vacuum failed with code %d", code)
		}
	})
}

// Minimal io copy helpers to avoid external imports in test
type ioDiscard struct{}
func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func ioCopy(dst ioDiscard, src ioReader) (written int64, err error) {
	buf := make([]byte, 32*1024)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = fmt.Errorf("invalid write")
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = fmt.Errorf("short write")
				break
			}
		}
		if er != nil {
			if er.Error() != "EOF" {
				err = er
			}
			break
		}
	}
	return written, err
}

type ioReader interface {
	Read(p []byte) (n int, err error)
}
