package observability

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const replayHistoryFileName = "replay-history.jsonl"

var replayHistoryMu sync.Mutex

// ActivityQuery filters persisted activity records.
type ActivityQuery struct {
	Limit     int
	Component string
	Action    string
	Entity    string
	Since     time.Time
}

// ReplayHistoryEntry records one time a replay/activity record was opened.
type ReplayHistoryEntry struct {
	PlayedAt          time.Time      `json:"played_at"`
	ActivityID        string         `json:"activity_id,omitempty"`
	ActivityTimestamp time.Time      `json:"activity_timestamp,omitempty"`
	Action            string         `json:"action"`
	Entity            string         `json:"entity"`
	EntityID          string         `json:"entity_id,omitempty"`
	Status            string         `json:"status,omitempty"`
	Attributes        map[string]any `json:"attributes,omitempty"`
}

// ReadActivityRecords returns activity records newest-first from the JSONL log
// files under logsDir. It intentionally reads the append-only journal directly:
// activity is process observability, not domain state that needs a DB table.
func ReadActivityRecords(logsDir string, q ActivityQuery) ([]Record, error) {
	if q.Limit <= 0 {
		q.Limit = 100
	}
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return nil, fmt.Errorf("observability: read activity dir: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "activity-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		files = append(files, filepath.Join(logsDir, name))
	}
	slices.Sort(files)
	slices.Reverse(files)

	records := make([]Record, 0, q.Limit)
	for _, file := range files {
		batch, err := readActivityFile(file)
		if err != nil {
			return nil, err
		}
		for i := len(batch) - 1; i >= 0; i-- {
			rec := batch[i]
			if !activityMatches(rec, q) {
				continue
			}
			records = append(records, rec)
			if len(records) >= q.Limit {
				return records, nil
			}
		}
	}
	return records, nil
}

func readActivityFile(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("observability: open activity file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var out []Record
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			// Gracefully skip corrupted or incomplete JSON lines (e.g., from abrupt process termination)
			continue
		}
		out = append(out, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("observability: scan activity %s: %w", path, err)
	}
	return out, nil
}

func activityMatches(rec Record, q ActivityQuery) bool {
	if q.Component != "" && rec.Component != q.Component {
		return false
	}
	if q.Action != "" && rec.Action != q.Action {
		return false
	}
	if q.Entity != "" && rec.Entity != q.Entity {
		return false
	}
	if !q.Since.IsZero() && rec.Timestamp.Before(q.Since) {
		return false
	}
	return true
}

// AppendReplayHistory appends one replay-history entry and caps the JSONL file
// to the newest cap entries. The cap keeps long-running review sessions bounded.
func AppendReplayHistory(logsDir string, entry ReplayHistoryEntry, cap int) error {
	if cap <= 0 {
		cap = 10000
	}
	replayHistoryMu.Lock()
	defer replayHistoryMu.Unlock()
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return fmt.Errorf("observability: create logs dir: %w", err)
	}
	path := filepath.Join(logsDir, replayHistoryFileName)
	lines, err := readJSONLLines(path)
	if err != nil {
		return err
	}
	if entry.PlayedAt.IsZero() {
		entry.PlayedAt = time.Now().UTC()
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("observability: marshal replay history: %w", err)
	}
	lines = append(lines, string(encoded))
	if len(lines) > cap {
		lines = lines[len(lines)-cap:]
	}
	return writeJSONLLines(path, lines)
}

// ReadReplayHistory returns replay-history entries newest-first.
func ReadReplayHistory(logsDir string, limit int) ([]ReplayHistoryEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	path := filepath.Join(logsDir, replayHistoryFileName)
	lines, err := readJSONLLines(path)
	if err != nil {
		return nil, err
	}
	out := make([]ReplayHistoryEntry, 0, min(limit, len(lines)))
	for i := len(lines) - 1; i >= 0 && len(out) < limit; i-- {
		var entry ReplayHistoryEntry
		if err := json.Unmarshal([]byte(lines[i]), &entry); err != nil {
			// Gracefully skip corrupted replay history entries
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

func readJSONLLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("observability: open jsonl %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("observability: scan jsonl %s: %w", path, err)
	}
	return lines, nil
}

func writeJSONLLines(path string, lines []string) error {
	tmp := path + ".tmp"
	body := strings.Join(lines, "\n")
	if body != "" {
		body += "\n"
	}
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return fmt.Errorf("observability: write replay history: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("observability: replace replay history: %w", err)
	}
	return nil
}
