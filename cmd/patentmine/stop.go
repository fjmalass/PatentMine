// Command patentmine lifecycle stop control. This file implements the
// stop-server, stop-api, and stop-tui subcommands, which read the matching pid
// file, signal the running process, wait for shutdown, and record control-plane
// logs for that lifecycle action.
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
	return stopService(cfg, cfg.PIDPath, "daemon")
}

// runStopAPI stops the background web API server, if one is recorded in the pid file.
func runStopAPI(_ []string) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	return stopService(cfg, cfg.APIPIDPath, "API")
}

// runStopTUI stops the terminal UI, if one is recorded in the pid file.
func runStopTUI(_ []string) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	return stopService(cfg, cfg.TUIPIDPath, "TUI")
}

// stopService signals the process recorded in pidPath and waits for it to clear
// the file. label names the service for human-facing output and logs. Each
// start-* subcommand removes its own pid file on exit, so a cleared file is the
// signal that shutdown finished.
func stopService(cfg config.Config, pidPath config.Path, label string) int {
	telemetry, err := openObservability(cfg, "control")
	if err != nil {
		return fail(err)
	}
	defer func() { _ = telemetry.Close() }()
	pid, err := readPIDFile(pidPath)
	if errors.Is(err, os.ErrNotExist) {
		telemetry.Logger.Info("stop requested but service not running", slog.String("service", label))
		fmt.Printf("patentmine %s is not running\n", label)
		return 0
	}
	if err != nil {
		telemetry.Logger.Error("read pid file failed", slog.String("service", label), slog.String("pid_path", string(pidPath)), slog.String("error", err.Error()))
		return fail(err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fail(err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			_ = os.Remove(string(pidPath))
			telemetry.Logger.Info("stale pid file removed", slog.String("service", label), slog.Int("pid", pid), slog.String("pid_path", string(pidPath)))
			fmt.Printf("patentmine %s is not running\n", label)
			return 0
		}
		telemetry.Logger.Error("signal service failed", slog.String("service", label), slog.Int("pid", pid), slog.String("error", err.Error()))
		return fail(err)
	}
	telemetry.Logger.Info("stop signaled", slog.String("service", label), slog.Int("pid", pid))
	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(string(pidPath)); errors.Is(err, os.ErrNotExist) {
			telemetry.Logger.Info("service stopped", slog.String("service", label), slog.Int("pid", pid))
			fmt.Printf("patentmine %s stopped\n", label)
			return 0
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fail(fmt.Errorf("timeout waiting for %s pid file %s to clear", label, pidPath))
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
