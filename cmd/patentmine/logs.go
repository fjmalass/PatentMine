package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"patentmine/internal/config"
)

const logsUsage = `usage:
  patentmine logs list                         list all log and activity files
  patentmine logs archive [--keep-days=N]      compress older logs into a backup archive
  patentmine logs ship --url=<url> [--auth=<>] ship a log file or latest archive to remote server
`

func runLogs(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, logsUsage)
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "list":
		return runLogsList(cfg, subArgs)
	case "archive":
		return runLogsArchive(cfg, subArgs)
	case "ship":
		return runLogsShip(cfg, subArgs)
	case "help", "-h", "--help":
		fmt.Print(logsUsage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "patentmine logs: unknown subcommand %q\n\n%s", sub, logsUsage)
		return 2
	}
}

func runLogsList(cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("logs list", flag.ExitOnError)
	_ = fs.Parse(args)

	logsDir := string(cfg.LogsDir)
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return fail(fmt.Errorf("read logs directory: %w", err))
	}

	var found bool
	fmt.Printf("%-32s %-12s %-12s %s\n", "FILE NAME", "SIZE (KB)", "TYPE", "MODIFIED")
	fmt.Println(strings.Repeat("-", 75))

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		var typ string
		switch {
		case strings.HasPrefix(name, "log-"):
			typ = "System Log"
		case strings.HasPrefix(name, "activity-"):
			typ = "Activity Log"
		case name == "replay-history.jsonl":
			typ = "Replay Hist"
		default:
			continue
		}

		found = true
		info, err := entry.Info()
		var szStr string
		var modStr string
		if err == nil {
			szStr = fmt.Sprintf("%.2f", float64(info.Size())/1024.0)
			modStr = info.ModTime().Local().Format("2006-01-02 15:04:05")
		} else {
			szStr = "unknown"
			modStr = "unknown"
		}
		fmt.Printf("%-32s %-12s %-12s %s\n", name, szStr, typ, modStr)
	}

	if !found {
		fmt.Println("No log or activity files found.")
	}
	return 0
}

func runLogsArchive(cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("logs archive", flag.ExitOnError)
	keepDays := fs.Int("keep-days", 1, "number of days of logs to keep uncompressed")
	_ = fs.Parse(args)

	logsDir := string(cfg.LogsDir)
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return fail(fmt.Errorf("read logs directory: %w", err))
	}

	now := time.Now()
	cutoff := now.AddDate(0, 0, -*keepDays)
	cutoffDateStr := cutoff.Format("2006-01-02")

	var filesToArchive []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		var datePart string
		if strings.HasPrefix(name, "log-") && strings.HasSuffix(name, ".jsonl") {
			datePart = name[4 : len(name)-6]
		} else if strings.HasPrefix(name, "activity-") && strings.HasSuffix(name, ".jsonl") {
			datePart = name[9 : len(name)-6]
		} else {
			continue
		}

		// Archive if it is before cutoff date
		if datePart < cutoffDateStr {
			filesToArchive = append(filesToArchive, filepath.Join(logsDir, name))
		}
	}

	if len(filesToArchive) == 0 {
		fmt.Println("No files found older than cutoff date:", cutoffDateStr)
		return 0
	}

	backupsDir := filepath.Join(string(cfg.HomeDir), "backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		return fail(fmt.Errorf("create backups directory: %w", err))
	}

	archiveName := fmt.Sprintf("logs-backup-%s.tar.gz", time.Now().Format("20060102-150405"))
	archivePath := filepath.Join(backupsDir, archiveName)

	fmt.Printf("Archiving %d log files to %s...\n", len(filesToArchive), archivePath)
	if err := compressFiles(filesToArchive, archivePath); err != nil {
		return fail(fmt.Errorf("create log archive: %w", err))
	}

	// Delete archived files
	for _, path := range filesToArchive {
		if err := os.Remove(path); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove archived log file %s: %v\n", path, err)
		}
	}

	fmt.Printf("Archival completed successfully. Created: %s\n", archivePath)
	return 0
}

func compressFiles(files []string, dest string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}
		header.Name = filepath.Base(path)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		fileToTar, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(tw, fileToTar)
		_ = fileToTar.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func runLogsShip(cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("logs ship", flag.ExitOnError)
	urlFlag := fs.String("url", "", "remote HTTP log ingestion URL")
	authFlag := fs.String("auth", "", "authorization header/token")
	fileFlag := fs.String("file", "", "specific file path to ship")
	_ = fs.Parse(args)

	// Resolve URL
	url := *urlFlag
	if url == "" {
		url = os.Getenv("PATENTMINE_LOG_SHIP_URL")
	}
	if url == "" {
		return fail(errors.New("shipping destination URL must be specified via --url or PATENTMINE_LOG_SHIP_URL environment variable"))
	}

	// Resolve Auth
	auth := *authFlag
	if auth == "" {
		auth = os.Getenv("PATENTMINE_LOG_SHIP_AUTH")
	}

	// Resolve Target File
	targetPath := *fileFlag
	if targetPath == "" {
		// Find latest log archive in backups folder
		backupsDir := filepath.Join(string(cfg.HomeDir), "backups")
		entries, err := os.ReadDir(backupsDir)
		if err != nil && !os.IsNotExist(err) {
			return fail(fmt.Errorf("read backups directory: %w", err))
		}

		var latestArchive string
		var latestTime time.Time
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasPrefix(name, "logs-backup-") || !strings.HasSuffix(name, ".tar.gz") {
				continue
			}
			info, err := entry.Info()
			if err == nil && info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestArchive = filepath.Join(backupsDir, name)
			}
		}
		if latestArchive == "" {
			return fail(errors.New("no logs-backup-*.tar.gz archive found; please specify file explicitly using --file"))
		}
		targetPath = latestArchive
	}

	f, err := os.Open(targetPath)
	if err != nil {
		return fail(fmt.Errorf("open file to ship: %w", err))
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fail(fmt.Errorf("stat file to ship: %w", err))
	}

	fmt.Printf("Shipping %s (%.2f KB) to %s...\n", filepath.Base(targetPath), float64(info.Size())/1024.0, url)

	req, err := http.NewRequest(http.MethodPost, url, f)
	if err != nil {
		return fail(fmt.Errorf("build shipping request: %w", err))
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-PatentMine-Filename", filepath.Base(targetPath))
	req.Header.Set("X-PatentMine-Component", "cli-shipper")
	if auth != "" {
		if !strings.HasPrefix(strings.ToLower(auth), "bearer ") && !strings.HasPrefix(strings.ToLower(auth), "basic ") {
			req.Header.Set("Authorization", "Bearer "+auth)
		} else {
			req.Header.Set("Authorization", auth)
		}
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fail(fmt.Errorf("dispatch shipping request: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fail(fmt.Errorf("shipping failed: server returned status %s: %s", resp.Status, string(body)))
	}

	fmt.Println("Logs successfully shipped.")
	return 0
}
