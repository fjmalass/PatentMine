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
	logger := slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo}))
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
