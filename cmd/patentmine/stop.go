// Command patentmine daemon stop control. This file implements the `stop`
// subcommand, which reads the daemon pid file, signals the running process,
// waits for shutdown, and records control-plane logs for that lifecycle action.
package main

import (
	"errors"
	"fmt"
	"log/slog"
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
	telemetry, err := openObservability(cfg, "control")
	if err != nil {
		return fail(err)
	}
	defer func() { _ = telemetry.Close() }()
	pid, err := readPIDFile(cfg.PIDPath)
	if errors.Is(err, os.ErrNotExist) {
		telemetry.Logger.Info("stop requested but daemon not running")
		fmt.Println("patentmine daemon is not running")
		return 0
	}
	if err != nil {
		telemetry.Logger.Error("read pid file failed", slog.String("pid_path", string(cfg.PIDPath)), slog.String("error", err.Error()))
		return fail(err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fail(err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			_ = os.Remove(string(cfg.PIDPath))
			telemetry.Logger.Info("stale pid file removed", slog.Int("pid", pid), slog.String("pid_path", string(cfg.PIDPath)))
			fmt.Println("patentmine daemon is not running")
			return 0
		}
		telemetry.Logger.Error("signal daemon failed", slog.Int("pid", pid), slog.String("error", err.Error()))
		return fail(err)
	}
	telemetry.Logger.Info("daemon stop signaled", slog.Int("pid", pid))
	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(string(cfg.PIDPath)); errors.Is(err, os.ErrNotExist) {
			telemetry.Logger.Info("daemon stopped", slog.Int("pid", pid))
			fmt.Println("patentmine daemon stopped")
			return 0
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fail(fmt.Errorf("timeout waiting for daemon pid file %s to clear", cfg.PIDPath))
}

func readPIDFile(path config.Path) (int, error) {
	raw, err := os.ReadFile(string(path))
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
