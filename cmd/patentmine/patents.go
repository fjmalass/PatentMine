// Command patentmine patents subcommand. This file implements management for the
// patents (XML cache) directory: list, archive (for backup), and clean (for
// removal of downloaded USPTO grant XML files).
package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"patentmine/internal/config"
	"patentmine/internal/observability"
)

const patentsUsage = `usage:
  patentmine uspto-manage list                         list files in the patents (XML) cache dir
  patentmine uspto-manage archive [options]            compress patents dir into a timestamped backup archive
  patentmine uspto-manage clean                        remove all files from the patents cache directory
`

func runPatents(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, patentsUsage)
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}

	// Open shared observability (writes structured JSONL logs + activity journal under the app logs dir).
	// This gives us proper telemetry for timing, counts, compression, deletes, etc.
	telemetry, err := openObservability(cfg, "cli")
	if err != nil {
		// Best-effort: continue with human output only if logs can't be opened.
		fmt.Fprintf(os.Stderr, "warning: could not open observability: %v\n", err)
	} else {
		defer func() { _ = telemetry.Close() }()
	}

	sub := args[0]
	subArgs := args[1:]

	start := time.Now()
	var code int
	switch sub {
	case "list":
		code = runPatentsList(cfg, subArgs, telemetry)
	case "archive":
		code = runPatentsArchive(cfg, subArgs, telemetry)
	case "clean":
		code = runPatentsClean(cfg, subArgs, telemetry)
	case "help", "-h", "--help":
		fmt.Print(patentsUsage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "patentmine uspto-manage: unknown subcommand %q\n\n%s", sub, patentsUsage)
		return 2
	}
	if telemetry != nil {
		telemetry.Logger.Info("patents command completed",
			slog.String("subcommand", sub),
			slog.Int("exit_code", code),
			slog.Duration("total_duration", time.Since(start)))
	}
	return code
}

func runPatentsList(cfg config.Config, args []string, telemetry *observability.Runtime) int {
	fs := flag.NewFlagSet("uspto-manage list", flag.ExitOnError)
	_ = fs.Parse(args)

	dir := string(cfg.PatentsDir)
	start := time.Now()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Patents directory does not exist:", dir)
			return 0
		}
		return fail(fmt.Errorf("read patents directory: %w", err))
	}

	var totalBytes int64
	found := false
	fmt.Printf("%-40s %12s %s\n", "FILE NAME", "SIZE (KB)", "MODIFIED")
	fmt.Println(strings.Repeat("-", 70))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		found = true
		info, err := entry.Info()
		var szStr, modStr string
		var sz int64
		if err == nil && info != nil {
			sz = info.Size()
			totalBytes += sz
			szStr = fmt.Sprintf("%.2f", float64(sz)/1024.0)
			modStr = info.ModTime().Local().Format("2006-01-02 15:04:05")
		} else {
			szStr = "?"
			modStr = "?"
		}
		fmt.Printf("%-40s %12s %s\n", entry.Name(), szStr, modStr)
	}

	dur := time.Since(start)
	if !found {
		fmt.Println("No files in patents directory.")
	} else {
		fmt.Printf("\nTotal: %d file(s), %.2f KB (%.2f MB)  |  duration: %v\n",
			len(entries), float64(totalBytes)/1024.0, float64(totalBytes)/1024.0/1024.0, dur)
	}

	// Structured telemetry
	if telemetry != nil {
		telemetry.Logger.Info("patents list",
			slog.String("dir", dir),
			slog.Int("file_count", len(entries)),
			slog.Int64("total_bytes", totalBytes),
			slog.Duration("duration", dur))
		if telemetry.Activity != nil {
			_ = telemetry.Activity.Record(context.Background(), observability.Record{
				Action:   observability.ActionCLIPatentsList,
				Entity:   "patents",
				Status:   "ok",
				Attributes: map[string]any{
					"dir":         dir,
					"file_count":  len(entries),
					"total_bytes": totalBytes,
					"duration_ms": dur.Milliseconds(),
				},
			})
		}
	}
	return 0
}

func runPatentsClean(cfg config.Config, args []string, telemetry *observability.Runtime) int {
	fs := flag.NewFlagSet("uspto-manage clean", flag.ExitOnError)
	_ = fs.Parse(args)

	dir := string(cfg.PatentsDir)
	start := time.Now()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Patents directory does not exist (nothing to clean).")
			return 0
		}
		return fail(fmt.Errorf("read patents directory: %w", err))
	}

	var removed, deletedBytes int64
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		var sz int64
		if info, statErr := os.Stat(p); statErr == nil {
			sz = info.Size()
		}
		if err := os.RemoveAll(p); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove %s: %v\n", p, err)
		} else {
			removed++
			deletedBytes += sz
		}
	}
	dur := time.Since(start)

	fmt.Printf("Cleaned patents directory: removed %d item(s), freed ~%.2f KB from %s  |  duration: %v\n",
		removed, float64(deletedBytes)/1024.0, dir, dur)

	// Structured telemetry + activity
	if telemetry != nil {
		telemetry.Logger.Info("patents clean",
			slog.String("dir", dir),
			slog.Int64("removed_count", removed),
			slog.Int64("freed_bytes", deletedBytes),
			slog.Duration("duration", dur))
		if telemetry.Activity != nil {
			_ = telemetry.Activity.Record(context.Background(), observability.Record{
				Action:   observability.ActionCLIPatentsClean,
				Entity:   "patents",
				Status:   "ok",
				Attributes: map[string]any{
					"dir":           dir,
					"removed_count": removed,
					"freed_bytes":   deletedBytes,
					"duration_ms":   dur.Milliseconds(),
				},
			})
		}
	}
	return 0
}

func runPatentsArchive(cfg config.Config, args []string, telemetry *observability.Runtime) int {
	fs := flag.NewFlagSet("uspto-manage archive", flag.ExitOnError)
	keepDays := fs.Int("keep-days", 0, "reserved for future (currently archives everything)")
	outFlag := fs.String("out", "", "explicit path for the archive file")
	prune := fs.Bool("prune", false, "remove files from patents dir after successful archiving")
	_ = fs.Parse(args)

	_ = *keepDays // unused for now; present for CLI compatibility with logs archive

	patentsDir := string(cfg.PatentsDir)
	start := time.Now()

	// Collect all files recursively (supports subdirs if future layout changes)
	var filesToArchive []string
	var bytesIn int64
	walkErr := filepath.WalkDir(patentsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			filesToArchive = append(filesToArchive, path)
			if info, statErr := d.Info(); statErr == nil {
				bytesIn += info.Size()
			}
		}
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return fail(fmt.Errorf("scan patents directory: %w", walkErr))
	}

	if len(filesToArchive) == 0 {
		fmt.Println("No files found in patents directory to archive:", patentsDir)
		return 0
	}

	// Resolve destination
	destPath := *outFlag
	if destPath == "" {
		backupsDir := filepath.Join(string(cfg.HomeDir), "backups")
		if err := os.MkdirAll(backupsDir, 0755); err != nil {
			return fail(fmt.Errorf("create backups directory: %w", err))
		}
		archiveName := fmt.Sprintf("patents-backup-%s.tar.gz", time.Now().Format("20060102-150405"))
		destPath = filepath.Join(backupsDir, archiveName)
	}

	fmt.Printf("Archiving %d file(s) from %s to %s...\n", len(filesToArchive), patentsDir, destPath)

	archiveStart := time.Now()
	if err := tarPaths(filesToArchive, patentsDir, destPath); err != nil {
		return fail(fmt.Errorf("create patents archive: %w", err))
	}
	archiveDur := time.Since(archiveStart)

	// Capture output size + compute compression stats
	var bytesOut int64
	if info, err := os.Stat(destPath); err == nil {
		bytesOut = info.Size()
	}
	var ratio float64
	if bytesIn > 0 {
		ratio = (1 - float64(bytesOut)/float64(bytesIn)) * 100
	}
	savings := bytesIn - bytesOut
	dur := time.Since(start)

	// Nice human summary with all the numbers the user asked for
	fmt.Printf("Patents archive created successfully: %s\n", destPath)
	fmt.Printf("  files: %d  |  input: %.2f KB (%.2f MB)  |  output: %.2f KB (%.2f MB)\n",
		len(filesToArchive),
		float64(bytesIn)/1024.0, float64(bytesIn)/1024.0/1024.0,
		float64(bytesOut)/1024.0, float64(bytesOut)/1024.0/1024.0)
	fmt.Printf("  compression: %.1f%%  |  space saved: %.2f KB  |  archive time: %v  |  total: %v\n",
		ratio, float64(savings)/1024.0, archiveDur, dur)

	prunedCount := int64(0)
	if *prune {
		for _, p := range filesToArchive {
			if err := os.Remove(p); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to prune %s after archive: %v\n", p, err)
			} else {
				prunedCount++
			}
		}
		fmt.Printf("Pruned %d archived file(s) from patents directory.\n", prunedCount)
	}

	// Rich structured telemetry (logs + activity + local metrics)
	if telemetry != nil {
		telemetry.Logger.Info("patents archive",
			slog.String("dir", patentsDir),
			slog.String("archive_path", destPath),
			slog.Int("file_count", len(filesToArchive)),
			slog.Int64("bytes_in", bytesIn),
			slog.Int64("bytes_out", bytesOut),
			slog.Float64("compression_ratio", ratio),
			slog.Int64("bytes_saved", savings),
			slog.Duration("archive_duration", archiveDur),
			slog.Duration("total_duration", dur),
			slog.Bool("prune", *prune),
			slog.Int64("pruned_count", prunedCount))

		if telemetry.Activity != nil {
			_ = telemetry.Activity.Record(context.Background(), observability.Record{
				Action:   observability.ActionCLIPatentsArchive,
				Entity:   "patents",
				Status:   "ok",
				Attributes: map[string]any{
					"dir":               patentsDir,
					"archive_path":      destPath,
					"file_count":        len(filesToArchive),
					"bytes_in":          bytesIn,
					"bytes_out":         bytesOut,
					"compression_ratio": ratio,
					"bytes_saved":       savings,
					"duration_ms":       dur.Milliseconds(),
					"prune":             *prune,
					"pruned_count":      prunedCount,
				},
			})
		}
	}
	return 0
}

// tarPaths writes the given files into a gzipped tar archive at dest.
// Names inside the tar are relative to rootDir and prefixed with "patents/".
func tarPaths(files []string, rootDir, dest string) error {
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

		rel, err := filepath.Rel(rootDir, path)
		if err != nil {
			rel = filepath.Base(path)
		}
		// Use forward slashes in tar and nest under patents/
		tarName := filepath.ToSlash(filepath.Join("patents", rel))

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = tarName

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			continue
		}

		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, in)
		in.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}
