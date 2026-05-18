// Package version exposes the application's build identity. Its fields are set
// by linker flags from the task runner so every entrypoint can report the same
// git-aware version in logs, startup output, and health responses.
package version

import "strings"

var (
	Describe = "dev"
	Commit   = "unknown"
	Branch   = "unknown"
)

// String returns the concise human-facing build version.
func String() string {
	parts := []string{Describe}
	if Branch != "" && Branch != "unknown" {
		parts = append(parts, "branch="+Branch)
	}
	if Commit != "" && Commit != "unknown" {
		parts = append(parts, "commit="+Commit)
	}
	return strings.Join(parts, " ")
}
