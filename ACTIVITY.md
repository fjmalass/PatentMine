# Telemetry, Auditing, and Activity Tracking Guide

PatentMine features an advanced, built-in observability subsystem that journals user interaction events, search actions, and database mutations. This telemetry serves three main purposes:
1. **Activity History & Replay**: Powering the TUI's `[ga]` timeline and supporting point-in-time state reconstruction.
2. **Behavioral Analytics**: Providing high-fidelity logs for data-driven usability analysis.
3. **Audit Logging & Security**: Creating a secure, unalterable trail of patent tagging, IDS status changes, and project switches.

---

## 1. Core Architecture

The activity tracking subsystem uses a high-performance, append-only dated **JSON Lines (JSONL)** journal.
- **Sinks**: Written dynamically under `$PATENTMINE_HOME/logs/` (defaults to `~/.config/patentmine/logs/` on Linux).
- **Format**: Each record is written as a single-line JSON object, permitting lightning-fast, zero-overhead background file streaming.
- **File Naming Pattern**: `activity-YYYY-MM-DD.jsonl`

### Record Data Schema
Every event conforms to the following structural schema defined in `internal/observability/observability.go`:

```go
type Record struct {
    ID        string         `json:"id"`                  // Unique chronological ID
    Timestamp time.Time      `json:"timestamp"`           // UTC timestamp
    Date      string         `json:"date"`                // Local calendar date (YYYY-MM-DD)
    Component string         `json:"component"`           // Creator process ("tui", "daemon", "api")
    Action    string         `json:"action"`              // Semantic action constant (e.g., "ids.entry.save")
    Entity    string         `json:"entity"`              // Target domain type (e.g., "ids_entry", "patent")
    EntityID  string         `json:"entity_id,omitempty"` // Bounded identifier (e.g., "project_id/patent_number")
    Status    string         `json:"status"`              // Event result ("committed", "observed")
    Before    any            `json:"before,omitempty"`    // Prior state snapshot (for undo/audits)
    After     any            `json:"after,omitempty"`     // Resulting state snapshot
    Metadata  map[string]any `json:"metadata,omitempty"`  // Structured variable metadata
}
```

---

## 2. Event Catalog

The full list of semantic actions tracked by the system is enumerated in `internal/observability/actions.go`:

| Action Constant | Entity Type | Details Recorded |
| :--- | :--- | :--- |
| `ids.entry.save` | `ids_entry` | Captures prior status, new status, and full relational properties of an IDS entry. |
| `ids.entry.delete` | `ids_entry` | Logged upon deletion of an entry from a project's Information Disclosure Statement. |
| `membership.set_state` | `membership` | Tracks transition of patent review states (e.g., `pending` $\rightarrow$ `under_review` $\rightarrow$ `active`). |
| `patent.tag_assign` | `patent` | Captures tag creation and allocation to individual patents. |
| `patent.tag_remove` | `patent` | Records removing tags from specific patent records. |
| `ui.focus` | `patent` | UI navigation tracking. Records focus dwell durations (100ms+) on individual patent panels. |
| `filter.apply` | `filter` | Telemetry recording applied search queries, active tags, and filter expressions. |
| `project.switch` | `project` | Logged whenever a user changes their active workspace project. |

---

## 3. History Feed & Noisy-Neighbor Mitigation

The TUI's **Activity History Overlay** displays a collapsed, chronological view of high-signal events.

> [!CAUTION]
> **Noisy-Neighbor Silent Truncation:**
> Highly frequent navigation events like `ui.focus` occur at a vastly larger volume than mutations like `ids.entry.save`. 
> 
> To prevent these focus logs from filling up the history scan window and causing old state mutations to be silently truncated, ensure your retention thresholds (`PATENTMINE_LOG_RETAIN_DAYS` and `PATENTMINE_LOG_MAX_SIZE_BYTES`) are properly configured to support the activity depth of your active projects.

---

## 4. Configuration & Adaptive Retention

Retention is completely configurable and dynamically managed at daemon startup via environment variables.

To customize your retention policies, set the following environment variables in your `.env` or system environment:

```bash
# 1. Configurable Time-Based Pruning (Days of active history to preserve)
# Defaults to 14 days if unset.
export PATENTMINE_LOG_RETAIN_DAYS=30

# 2. Adaptive Size-Based Pruning (Max log directory size in bytes)
# If the logs directory size exceeds this limit, the oldest files are dropped.
# Defaults to 100MB (104857600 bytes) if unset.
export PATENTMINE_LOG_MAX_SIZE_BYTES=52428800 # Capped at 50MB
```

On daemon startup or TUI launch, a background worker scans `$PATENTMINE_HOME/logs/`, prunes any log files whose date suffix is older than `LogRetainDays`, and performs a sorting pass to keep total log storage strictly below `LogMaxSizeBytes` by removing the oldest logs first.

---

## 5. Architectural Retention Tiers for Long-Term Auditing

If you need to keep activity records indefinitely for data science, productivity auditing, or security logs, implement one of the following production-grade patterns:

### Tier 1: Local Gzip Compression (Offline Archiving)
Rather than deleting retired dated files, write a script or cron task to compress retired logs into an offline `archive/` folder:
- **Compression Efficiency**: JSONL is exceptionally repetitive. Gzip compresses it by **$\ge$ 90%**.
- **Yield**: A full year of highly active workflows ($\approx 2.6 \text{ million events}$) compresses from 1.0 GB to **less than 100 MB**.

### Tier 2: Daemon Shipping (Cloud/Remote Data Warehouse)
In corporate settings, write a daemon worker that periodically ships un-uploaded logs to an HTTPS REST telemetry collector:
1. Every hour, query the local database cache for new unsent records.
2. `POST https://telemetry.patentmine.internal/v1/ship` with a compressed batch payload.
3. Upon a successful HTTP `200 OK` handshake, mark the records as synced.
4. Let the local pruning thread delete synced records locally, keeping the user's hard drive clean and fast.

### Tier 3: Columnar Data Extraction (Pandas/ClickHouse Parquet)
For big-data analysis, convert JSONL rows into highly optimized columnar Apache Parquet files:
- Parquet supports dictionary encoding, column-group compression, and run-length encoding.
- Parquet reduces IO footprints by **$\gt 95\%$** when querying specific columns like `action` or `duration` across millions of workflow steps.
