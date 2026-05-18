package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"patentmine/internal/config"
)

const stopTimeout = 5 * time.Second

// runStop stops a running daemon, if one is recorded in the pid file.
func runStop(_ []string) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	pid, err := readPIDFile(cfg.PIDPath)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("patentmine daemon is not running")
		return 0
	}
	if err != nil {
		return fail(err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fail(err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			_ = os.Remove(cfg.PIDPath)
			fmt.Println("patentmine daemon is not running")
			return 0
		}
		return fail(err)
	}
	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(cfg.PIDPath); errors.Is(err, os.ErrNotExist) {
			fmt.Println("patentmine daemon stopped")
			return 0
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fail(fmt.Errorf("timeout waiting for daemon pid file %s to clear", cfg.PIDPath))
}

func readPIDFile(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0, fmt.Errorf("parse pid file %s: %w", path, err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("parse pid file %s: invalid pid %d", path, pid)
	}
	return pid, nil
}
