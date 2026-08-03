package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"patentmine/internal/config"
)

func TestConfirmCleanDataRequiresExactConfirmation(t *testing.T) {
	var out bytes.Buffer
	cfg := config.Config{
		HomeDir:    config.Path(filepath.Join("tmp", "patentmine")),
		DBPath:     config.Path(filepath.Join("tmp", "patentmine", "patentmine.db")),
		LogsDir:    config.Path(filepath.Join("tmp", "patentmine", "logs")),
		PatentsDir: config.Path(filepath.Join("tmp", "patentmine", "patents")),
		PIDPath:    config.Path(filepath.Join("tmp", "patentmine", "patentmine.pid")),
		SocketPath: config.Path(filepath.Join("tmp", "patentmine", "patentmine.sock")),
	}

	if confirmCleanData(false, strings.NewReader("no\n"), &out, cfg) {
		t.Fatal("confirmCleanData accepted wrong confirmation")
	}
	if !strings.Contains(out.String(), `Type exactly "clean-data"`) {
		t.Fatalf("prompt output = %q, want confirmation prompt", out.String())
	}
	if !strings.Contains(out.String(), "database files:") || !strings.Contains(out.String(), "logs contents:") {
		t.Fatalf("prompt output = %q, want deletion locations", out.String())
	}
	if !strings.Contains(out.String(), `confirmation did not match "clean-data"`) {
		t.Fatalf("prompt output = %q, want refusal reason", out.String())
	}
	if !confirmCleanData(false, strings.NewReader("clean-data\n"), &out, cfg) {
		t.Fatal("confirmCleanData rejected exact confirmation")
	}
	if !confirmCleanData(true, strings.NewReader(""), &out, cfg) {
		t.Fatal("confirmCleanData rejected force")
	}
}

func TestNormalizeCleanDataArgsHandlesCargoMakeSeparators(t *testing.T) {
	args := normalizeCleanDataArgs([]string{"--;--force;--skip-stop"})
	want := []string{"--force", "--skip-stop"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("normalizeCleanDataArgs = %v, want %v", args, want)
	}
}

func TestCleanDataRemovesLocalDataButKeepsBackups(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvHome, home)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	paths := []string{
		string(cfg.DBPath),
		string(cfg.DBPath) + "-wal",
		string(cfg.DBPath) + "-shm",
		string(cfg.DBPath) + "-journal",
		string(cfg.PIDPath),
		string(cfg.SocketPath),
		filepath.Join(string(cfg.LogsDir), "log-2026-08-03.jsonl"),
		filepath.Join(string(cfg.PatentsDir), "sample.xml"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	backupPath := filepath.Join(home, "backups", "keep.db")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		t.Fatalf("create backup dir: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte("backup"), 0o644); err != nil {
		t.Fatalf("write backup: %v", err)
	}

	if code := runCleanData([]string{"--force", "--skip-stop"}); code != 0 {
		t.Fatalf("runCleanData exit code = %d, want 0", code)
	}

	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s still exists after clean-data", path)
		}
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup should be preserved: %v", err)
	}
}
