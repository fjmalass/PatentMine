package sqlite

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Backup creates an online hot backup copy of the SQLite database at destPath.
// If the destination file already exists, it is overwritten.
// An in-memory database (used by tests) has no on-disk path and is skipped.
func (r *Repo) Backup(ctx context.Context, destPath string) error {
	if r.path == "" || r.path == ":memory:" {
		return nil
	}

	start := time.Now()
	var failed bool

	if r.metrics != nil {
		r.metrics.IncCounter("store.sqlite.backup.total", 1)
	}

	slog.InfoContext(ctx, "starting database backup",
		slog.String("src", r.path),
		slog.String("dest", destPath))

	if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
		failed = true
		if r.metrics != nil {
			r.metrics.IncCounter("store.sqlite.backup.error_total", 1)
		}
		slog.ErrorContext(ctx, "backup failed to remove existing file",
			slog.String("dest", destPath),
			slog.String("error", err.Error()))
		return fmt.Errorf("store/sqlite: backup: remove existing %s: %w", destPath, err)
	}

	// VACUUM INTO takes an SQL string literal, not a bind parameter.
	literal := "'" + strings.ReplaceAll(destPath, "'", "''") + "'"
	if _, err := r.writer.ExecContext(ctx, "VACUUM INTO "+literal); err != nil {
		failed = true
		if r.metrics != nil {
			r.metrics.IncCounter("store.sqlite.backup.error_total", 1)
		}
		slog.ErrorContext(ctx, "backup ExecContext failed",
			slog.String("dest", destPath),
			slog.String("error", err.Error()))
		return fmt.Errorf("store/sqlite: backup: VACUUM INTO: %w", err)
	}

	duration := time.Since(start)
	if r.metrics != nil {
		r.metrics.ObserveDuration("store.sqlite.backup.duration", duration, failed)
	}

	// Get backup file size for telemetry
	var sizeBytes int64
	if info, err := os.Stat(destPath); err == nil {
		sizeBytes = info.Size()
		if r.metrics != nil {
			r.metrics.SetGauge("store.sqlite.backup.size_bytes", sizeBytes)
		}
	}

	slog.InfoContext(ctx, "database backup completed successfully",
		slog.String("dest", destPath),
		slog.Duration("duration", duration),
		slog.Int64("size_bytes", sizeBytes))

	return nil
}

// Vacuum compacts the SQLite database to reclaim free space.
func (r *Repo) Vacuum(ctx context.Context) error {
	if _, err := r.writer.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("store/sqlite: vacuum: %w", err)
	}
	return nil
}

// DBStatus holds database statistics and integrity information.
type DBStatus struct {
	Path           string
	SizeMB         float64
	IntegrityCheck string
	TableCounts    map[string]int
}

// Status returns statistics and checks integrity of the SQLite database.
func (r *Repo) Status(ctx context.Context) (*DBStatus, error) {
	stat := &DBStatus{
		Path:        r.path,
		TableCounts: make(map[string]int),
	}

	// Calculate size
	if r.path != "" && r.path != ":memory:" {
		info, err := os.Stat(r.path)
		if err == nil {
			stat.SizeMB = float64(info.Size()) / (1024.0 * 1024.0)
		}
	}

	// Run integrity check
	var integrity string
	if err := r.reader.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		stat.IntegrityCheck = fmt.Sprintf("FAILED: %v", err)
	} else {
		stat.IntegrityCheck = integrity
	}

	// Query counts for primary tables
	tables := []string{"record", "document", "relation", "project", "membership", "patent_tag", "project_patent_note", "source_bib"}
	for _, tbl := range tables {
		var count int
		query := fmt.Sprintf("SELECT count(*) FROM %s", tbl)
		if err := r.reader.QueryRowContext(ctx, query).Scan(&count); err == nil {
			stat.TableCounts[tbl] = count
		} else {
			stat.TableCounts[tbl] = -1 // table doesn't exist or query failed
		}
	}

	return stat, nil
}
