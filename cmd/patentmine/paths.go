// Command patentmine resolved path reporting. This file implements the `paths`
// subcommand used by automation and task runners to discover the active home,
// database, pid, socket, and logs locations chosen by config.Load.
package main

import (
	"fmt"
	"io"

	"patentmine/internal/config"
)

// runPaths prints resolved runtime paths for automation and debugging.
func runPaths(_ []string) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	fmt.Printf("HOME=%s\nDB=%s\nPID=%s\nSOCKET=%s\n",
		cfg.HomeDir, cfg.DBPath, cfg.PIDPath, cfg.SocketPath)
	fmt.Printf("LOGS=%s\n", cfg.LogsDir)
	return 0
}

// reportPaths writes the resolved runtime locations in human-readable form.
// Entrypoints print it on startup so a user always knows where their database
// and logs live.
func reportPaths(w io.Writer, cfg config.Config) {
	fmt.Fprintf(w, "  home    %s\n", cfg.HomeDir)
	fmt.Fprintf(w, "  db      %s\n", cfg.DBPath)
	fmt.Fprintf(w, "  logs    %s\n", cfg.LogsDir)
	fmt.Fprintf(w, "  socket  %s\n", cfg.SocketPath)
}
