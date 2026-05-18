// Command patentmine is the single binary for the PatentMine system. It hosts
// every entrypoint as a subcommand: the engine daemon and each thin client.
package main

import (
	"fmt"
	"os"

	appversion "patentmine/internal/version"
)

const usageText = `patentmine — patent tracking

usage:
  patentmine serve     start the engine daemon
  patentmine stop      stop the engine daemon
  patentmine tui       launch the terminal UI
  patentmine api       start the web API server
  patentmine paths     print resolved runtime paths
  patentmine version   print the build version
`

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches to a subcommand and returns the process exit code. Keeping the
// logic out of main keeps it testable.
func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:])
	case "stop":
		return runStop(args[1:])
	case "tui":
		return runTUI(args[1:])
	case "api":
		return runAPI(args[1:])
	case "paths":
		return runPaths(args[1:])
	case "version":
		fmt.Println(appversion.String())
		return 0
	case "help", "-h", "--help":
		fmt.Print(usageText)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "patentmine: unknown subcommand %q\n\n%s", args[0], usageText)
		return 2
	}
}
