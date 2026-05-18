package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/config"
	"patentmine/internal/rpc"
	"patentmine/internal/tui"
	"patentmine/internal/tui/keymap"
)

// runTUI launches the terminal client. It is a thin frontend: it connects to
// the daemon and renders, holding no database or business logic of its own.
func runTUI(_ []string) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}

	client, err := rpc.Dial(cfg.SocketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "patentmine: cannot reach the daemon at %s\n", cfg.SocketPath)
		fmt.Fprintln(os.Stderr, "start it first in another terminal:  patentmine serve")
		return 1
	}
	defer func() { _ = client.Close() }()

	registry, err := command.Default()
	if err != nil {
		return fail(err)
	}

	app := tui.New(client, registry, keymap.Default())
	if _, err := tea.NewProgram(app, tea.WithAltScreen()).Run(); err != nil {
		return fail(err)
	}
	return 0
}
