package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"patentmine/internal/config"
)

const cleanDataUsage = `usage:
  patentmine clean-data [--force] [--skip-stop]

Removes local data: database files, logs, runtime files, and the USPTO patent cache.
Backups and exports under PATENTMINE_HOME are left untouched.
Prompts for confirmation unless --force is passed. Type exactly "clean-data"; "y" is refused.
`

func runCleanData(args []string) int {
	args = normalizeCleanDataArgs(args)

	fs := flag.NewFlagSet("clean-data", flag.ExitOnError)
	force := fs.Bool("force", false, "skip the interactive confirmation prompt")
	skipStop := fs.Bool("skip-stop", false, "do not stop the daemon before removing files")
	fs.Usage = func() { fmt.Fprint(fs.Output(), cleanDataUsage) }
	_ = fs.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}

	if !confirmCleanData(*force, os.Stdin, os.Stderr, cfg) {
		return 2
	}

	if !*skipStop {
		if code := runStop(nil); code != 0 {
			return code
		}
	}

	removed := 0
	for _, path := range cleanDataFiles(cfg) {
		ok, err := removePathIfExists(path)
		if err != nil {
			return fail(err)
		}
		if ok {
			removed++
		}
	}

	for _, dir := range []config.Path{cfg.LogsDir, cfg.PatentsDir} {
		count, err := removeDirContents(string(dir))
		if err != nil {
			return fail(err)
		}
		removed += count
	}

	fmt.Printf("cleaned PatentMine local data: removed %d item(s)\n", removed)
	fmt.Printf("home preserved: %s\n", cfg.HomeDir)
	return 0
}

func normalizeCleanDataArgs(args []string) []string {
	var normalized []string
	for _, arg := range args {
		for _, part := range strings.Split(arg, ";") {
			if part == "" || part == "--" {
				continue
			}
			normalized = append(normalized, part)
		}
	}
	return normalized
}

func confirmCleanData(force bool, in io.Reader, out io.Writer, cfg config.Config) bool {
	if force {
		return true
	}

	fmt.Fprintln(out, "This will permanently remove local PatentMine data.")
	fmt.Fprintf(out, "PATENTMINE_HOME: %s\n", cfg.HomeDir)
	fmt.Fprintln(out, "Will remove:")
	fmt.Fprintf(out, "  database files: %s, %s-wal, %s-shm, %s-journal\n", cfg.DBPath, cfg.DBPath, cfg.DBPath, cfg.DBPath)
	fmt.Fprintf(out, "  runtime files: %s, %s\n", cfg.PIDPath, cfg.SocketPath)
	fmt.Fprintf(out, "  logs contents: all entries under %s\n", cfg.LogsDir)
	fmt.Fprintf(out, "  patent cache contents: all entries under %s\n", cfg.PatentsDir)
	fmt.Fprintln(out, "Will preserve:")
	fmt.Fprintf(out, "  backups and exports under %s\n", cfg.HomeDir)
	fmt.Fprint(out, `Type exactly "clean-data" to continue: `)

	scanner := bufio.NewScanner(in)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "clean-data" {
		fmt.Fprintln(out, `confirmation did not match "clean-data"; refusing to delete data. No files were removed.`)
		return false
	}
	return true
}

func cleanDataFiles(cfg config.Config) []string {
	dbPath := string(cfg.DBPath)
	return []string{
		dbPath,
		dbPath + "-wal",
		dbPath + "-shm",
		dbPath + "-journal",
		string(cfg.PIDPath),
		string(cfg.SocketPath),
	}
}

func removePathIfExists(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.RemoveAll(path); err != nil {
		return false, fmt.Errorf("remove %s: %w", path, err)
	}
	return true, nil
}

func removeDirContents(dir string) (int, error) {
	if dir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", dir, err)
	}

	removed := 0
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return removed, fmt.Errorf("remove %s: %w", path, err)
		}
		removed++
	}
	return removed, nil
}
