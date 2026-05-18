// Command patentmine observability helpers. This file centralizes the setup of
// structured logs and activity journaling for each CLI entrypoint, so serve,
// tui, api, and control commands all open the same dated log sinks the same way.
package main

import (
	"patentmine/internal/config"
	"patentmine/internal/observability"
)

func openObservability(cfg config.Config, component string) (*observability.Runtime, error) {
	return observability.Open(cfg.LogsDir, component)
}
