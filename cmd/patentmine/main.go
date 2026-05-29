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
  patentmine start-server       start the engine daemon
  patentmine stop-server        stop the engine daemon
  patentmine start-tui          launch the terminal UI
  patentmine stop-tui           stop a running terminal UI
  patentmine start-api          start the web API server (use --addr host:port)
  patentmine stop-api           stop the web API server
  patentmine paths              print resolved runtime paths
  patentmine logs               manage log and activity files (list, archive, ship)
  patentmine db                 manage database tasks (backup, vacuum, status)
  patentmine uspto-manage       manage the patents/XML cache directory (list, archive, clean)
  patentmine check-connectivity check configured external service connectivity
  patentmine lookup             look up a patent by USPTO application number
  patentmine expiration-date    compute and compare estimated patent expiration dates
  patentmine uspto-citations    parse and save citations from a USPTO grant/pgpub XML file
  patentmine version            print the build version
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
	case "start-server":
		return runServe(args[1:])
	case "stop-server":
		return runStop(args[1:])
	case "start-tui":
		return runTUI(args[1:])
	case "stop-tui":
		return runStopTUI(args[1:])
	case "start-api":
		return runAPI(args[1:])
	case "stop-api":
		return runStopAPI(args[1:])
	case "paths":
		return runPaths(args[1:])
	case "logs":
		return runLogs(args[1:])
	case "db":
		return runDB(args[1:])
	case "uspto-manage":
		return runPatents(args[1:])
	case "check-connectivity":
		return runCheck(args[1:])
	case "lookup":
		return runLookup(args[1:])
	case "expiration-date":
		return runExpirationDate(args[1:])
	case "uspto-citations":
		return runCitations(args[1:])
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
