package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDatedPathAddsDateBeforeExtension(t *testing.T) {
	now := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
	got := DatedPath("logs/patentmine.log", now)
	want := "logs/patentmine-2026-05-08.log"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}


func TestOpenPrunesOldDatedLogs(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "patentmine.log")
	for _, name := range []string{
		"patentmine-2026-08-15.log",
		"patentmine-2026-08-16.log",
		"patentmine-2026-08-17.log",
		"patentmine-2026-08-18.log",
		"patentmine-2026-08-19.log",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("log\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, closeLog, logPath, err := Open(basePath, 3)
	if err != nil {
		t.Fatal(err)
	}
	closeLog()

	matches, err := filepath.Glob(filepath.Join(dir, "patentmine-*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 3 {
		t.Fatalf("expected 3 retained logs, got %d: %v", len(matches), matches)
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("expected current log %q to exist: %v", logPath, err)
	}
}
