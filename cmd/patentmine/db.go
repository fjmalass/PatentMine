package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"patentmine/internal/config"
	"patentmine/internal/store/sqlite"
)

const dbUsage = `usage:
  patentmine db backup [--out=<path>] [--rsync=<dest>]   create a consistent hot backup
  patentmine db vacuum                                   compact database file to reclaim space
  patentmine db status                                   show file size, integrity, and row counts
`

func runDB(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, dbUsage)
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "backup":
		return runDBBackup(cfg, subArgs)
	case "vacuum":
		return runDBVacuum(cfg, subArgs)
	case "status":
		return runDBStatus(cfg, subArgs)
	case "help", "-h", "--help":
		fmt.Print(dbUsage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "patentmine db: unknown subcommand %q\n\n%s", sub, dbUsage)
		return 2
	}
}

func runDBBackup(cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("db backup", flag.ExitOnError)
	outFlag := fs.String("out", "", "explicit path for backup destination")
	rsyncFlag := fs.String("rsync", "", "remote rsync destination (e.g. user@host:/path/)")
	_ = fs.Parse(args)

	// Resolve output path
	destPath := *outFlag
	if destPath == "" {
		backupsDir := filepath.Join(string(cfg.HomeDir), "backups")
		if err := os.MkdirAll(backupsDir, 0755); err != nil {
			return fail(fmt.Errorf("create backups directory: %w", err))
		}
		backupName := fmt.Sprintf("patentmine-%s.db", time.Now().Format("20060102-150405"))
		destPath = filepath.Join(backupsDir, backupName)
	}

	// Resolve rsync target
	rsyncDest := *rsyncFlag
	if rsyncDest == "" {
		rsyncDest = os.Getenv("PATENTMINE_DB_BACKUP_RSYNC")
	}

	ctx := context.Background()
	fmt.Printf("Opening source database %s...\n", cfg.DBPath)
	repo, err := sqlite.Open(ctx, string(cfg.DBPath))
	if err != nil {
		return fail(fmt.Errorf("open sqlite database: %w", err))
	}
	defer repo.Close()

	fmt.Printf("Creating consistent hot backup to %s...\n", destPath)
	if err := repo.Backup(ctx, destPath); err != nil {
		return fail(fmt.Errorf("database hot backup failed: %w", err))
	}

	fmt.Printf("Database hot backup created successfully: %s\n", destPath)

	// Execute rsync if configured
	if rsyncDest != "" {
		fmt.Printf("Synchronizing backup file to remote destination %s using rsync...\n", rsyncDest)
		cmd := exec.Command("rsync", "-avz", destPath, rsyncDest)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fail(fmt.Errorf("rsync sync failed: %w", err))
		}
		fmt.Println("Rsync synchronization completed successfully.")
	}

	return 0
}

func runDBVacuum(cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("db vacuum", flag.ExitOnError)
	_ = fs.Parse(args)

	ctx := context.Background()
	fmt.Printf("Opening database %s...\n", cfg.DBPath)
	repo, err := sqlite.Open(ctx, string(cfg.DBPath))
	if err != nil {
		return fail(fmt.Errorf("open sqlite database: %w", err))
	}
	defer repo.Close()

	fmt.Println("Compacting database (vacuuming)...")
	if err := repo.Vacuum(ctx); err != nil {
		return fail(fmt.Errorf("database vacuum failed: %w", err))
	}

	fmt.Println("Database vacuum completed successfully. Unused space reclaimed.")
	return 0
}

func runDBStatus(cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("db status", flag.ExitOnError)
	_ = fs.Parse(args)

	ctx := context.Background()
	repo, err := sqlite.Open(ctx, string(cfg.DBPath))
	if err != nil {
		return fail(fmt.Errorf("open sqlite database: %w", err))
	}
	defer repo.Close()

	stat, err := repo.Status(ctx)
	if err != nil {
		return fail(fmt.Errorf("retrieve database status: %w", err))
	}

	fmt.Println("Database Diagnostic Status")
	fmt.Println(strings.Repeat("=", 55))
	fmt.Printf("  File Path:   %s\n", stat.Path)
	fmt.Printf("  File Size:   %.2f MB\n", stat.SizeMB)
	fmt.Printf("  Integrity:   %s\n", stat.IntegrityCheck)
	fmt.Println("\n  Table Row Counts:")
	fmt.Println("  " + strings.Repeat("-", 35))

	// Find the longest table name for alignment
	maxLen := 0
	for tbl := range stat.TableCounts {
		if len(tbl) > maxLen {
			maxLen = len(tbl)
		}
	}

	for tbl, count := range stat.TableCounts {
		if count >= 0 {
			fmt.Printf("    %-*s : %d\n", maxLen, tbl, count)
		} else {
			fmt.Printf("    %-*s : [table not initialized]\n", maxLen, tbl)
		}
	}
	fmt.Println(strings.Repeat("=", 55))

	return 0
}
