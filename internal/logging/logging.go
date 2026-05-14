package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// EnvDebug is the environment variable that enables debug-level logging and render tracing.
const EnvDebug = "PATENT_DEBUG"

func logLevel() slog.Level {
	if os.Getenv(EnvDebug) == "1" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

func Open(path string, maxLogs int) (*slog.Logger, func(), string, error) {
	logPath := DatedPath(path, time.Now())
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return slog.Default(), func() {}, logPath, err
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return slog.Default(), func() {}, logPath, err
	}
	if err := pruneDatedLogs(path, maxLogs, logPath); err != nil {
		_ = file.Close()
		return slog.Default(), func() {}, logPath, err
	}
	logger := slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{Level: logLevel()}))
	closeFn := func() {
		_ = file.Close()
	}
	return logger, closeFn, logPath, nil
}

func DatedPath(path string, now time.Time) string {
	if strings.TrimSpace(path) == "" {
		path = "patentmine.log"
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", stem, now.Format(time.DateOnly), ext))
}

func pruneDatedLogs(path string, maxLogs int, keepPath string) error {
	if maxLogs <= 0 {
		return nil
	}
	pattern := datedGlob(path)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	if len(matches) <= maxLogs {
		return nil
	}
	sort.Strings(matches)
	removeCount := len(matches) - maxLogs
	keepPath = filepath.Clean(keepPath)
	for _, match := range matches {
		if removeCount == 0 {
			return nil
		}
		if filepath.Clean(match) == keepPath {
			continue
		}
		if err := os.Remove(match); err != nil {
			return err
		}
		removeCount--
	}
	return nil
}

func datedGlob(path string) string {
	if strings.TrimSpace(path) == "" {
		path = "patentmine.log"
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, stem+"-[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]"+ext)
}

// OpenActivity opens (or creates) a monthly-rotated JSON-lines activity log.
// Path is used as a stem: activity.jsonl → activity_2026_05.jsonl.
// Returns the logger, a close function, and any open error.
func OpenActivity(path string) (*slog.Logger, func(), error) {
	if strings.TrimSpace(path) == "" {
		path = "logs/activity.jsonl"
	}
	monthlyPath := monthlyPath(path, time.Now())
	if err := os.MkdirAll(filepath.Dir(monthlyPath), 0o755); err != nil {
		return slog.Default(), func() {}, err
	}
	file, err := os.OpenFile(monthlyPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return slog.Default(), func() {}, err
	}
	logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{Level: logLevel()}))
	return logger, func() { _ = file.Close() }, nil
}

// monthlyPath derives logs/activity_2026_05.jsonl from logs/activity.jsonl.
func monthlyPath(path string, now time.Time) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, fmt.Sprintf("%s_%s%s", stem, now.Format("2006_01"), ext))
}
