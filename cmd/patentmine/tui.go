package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

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
func runTUI(args []string) int {
	flags := flag.NewFlagSet("tui", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	notesExportDir := flags.String("notes-export-dir", "", "directory for exported notes .md files (overrides PATENTMINE_NOTES_EXPORT_DIR)")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "usage: patentmine tui [--notes-export-dir path]")
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	telemetry, err := openObservability(cfg, "tui")
	if err != nil {
		return fail(err)
	}
	defer func() { _ = telemetry.Close() }()
	telemetry.Logger.Info("tui starting", slog.String("socket_path", string(cfg.SocketPath)))
	fmt.Fprintf(os.Stderr, "patentmine tui %s\n", appversion.String())
	reportPaths(os.Stderr, cfg)

	client, err := rpc.Dial(string(cfg.SocketPath))
	if err != nil {
		telemetry.Logger.Error("dial daemon failed", slog.String("socket_path", string(cfg.SocketPath)), slog.String("error", err.Error()))
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

	exportDir := cfg.NotesExportDir
	if *notesExportDir != "" {
		exportDir = *notesExportDir
	}

	app, err := tui.New(client, registry, keymap.Default(), text.English(),
		tui.WithLastProject(lastProjectID),
		tui.WithTelemetry(telemetry),
		tui.WithActivityMinDuration(time.Duration(cfg.ActivityMinMS)*time.Millisecond),
		tui.WithAIConfig(cfg.AIProvider, cfg.GeminiAPIKey, cfg.OllamaHost, cfg.OllamaModel),
		tui.WithUSPTOKey(cfg.USPTOAPIKey),
		tui.WithBackupConfig(cfg.BackupConfigured(), cfg.BackupRcloneRemote, cfg.BackupBucket),
		tui.WithNotesExportDir(exportDir),
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
