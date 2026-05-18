package main

import (
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui"
	"patentmine/internal/tui/keymap"
	appversion "patentmine/internal/version"
)

// runTUI launches the terminal client. It is a thin frontend: it connects to
// the daemon and renders, holding no database or business logic of its own.
func runTUI(_ []string) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	telemetry, err := openObservability(cfg, "tui")
	if err != nil {
		return fail(err)
	}
	defer func() { _ = telemetry.Close() }()
	telemetry.Logger.Info("tui starting", slog.String("socket_path", cfg.SocketPath))
	fmt.Fprintf(os.Stderr, "patentmine tui %s\n", appversion.String())

	client, err := rpc.Dial(cfg.SocketPath)
	if err != nil {
		telemetry.Logger.Error("dial daemon failed", slog.String("socket_path", cfg.SocketPath), slog.String("error", err.Error()))
		fmt.Fprintf(os.Stderr, "patentmine: cannot reach the daemon at %s\n", cfg.SocketPath)
		fmt.Fprintln(os.Stderr, "start it first in another terminal:  patentmine serve")
		return 1
	}
	defer func() { _ = client.Close() }()

	registry, err := command.Default()
	if err != nil {
		telemetry.Logger.Error("build command registry failed", slog.String("error", err.Error()))
		return fail(err)
	}
	lastProjectID, err := loadLastProject(cfg.HomeDir)
	if err != nil {
		telemetry.Logger.Error("load last project failed", slog.String("error", err.Error()))
	}

	app, err := tui.New(client, registry, keymap.Default(), text.English(),
		tui.WithLastProject(lastProjectID),
		tui.WithLastProjectSaver(func(id domain.ProjectID) error { return saveLastProject(cfg.HomeDir, id) }))
	if err != nil {
		telemetry.Logger.Error("build tui failed", slog.String("error", err.Error()))
		return fail(err)
	}
	if _, err := tea.NewProgram(app, tea.WithAltScreen()).Run(); err != nil {
		telemetry.Logger.Error("tui run failed", slog.String("error", err.Error()))
		return fail(err)
	}
	telemetry.Logger.Info("tui stopped")
	return 0
}
