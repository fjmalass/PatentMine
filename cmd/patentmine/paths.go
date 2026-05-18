package main

import (
	"fmt"

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
	return 0
}
