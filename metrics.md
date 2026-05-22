# Metrics Guide

PatentMine exposes in-process metrics from the daemon and also records user-facing TUI actions in the TUI process when telemetry is attached.

This document explains:

1. how to fetch metrics
2. what the different access paths do
3. what the TUI exposes today
4. what to watch for tag-sorting and list-performance regressions
5. what remains to build

## What metrics are

PatentMine keeps an in-memory metrics registry with three kinds of values:

1. timings: operation duration summaries
2. counters: monotonically increasing event counts
3. gauges: latest point-in-time values

The main producers are:

1. daemon RPC handlers
2. engine operations
3. SQLite store operations
4. TUI actions when telemetry is enabled

## How to get metrics

### HTTP JSON snapshot

Use:

```text
GET /metricsz
```

This returns a JSON snapshot of the current in-memory registry. It is the easiest format to inspect from scripts or during debugging.

What it gives you:

1. current timestamp
2. timing summaries by name
3. counters by name
4. gauges by name

Implementation path:

1. HTTP route: `internal/api/routes.go`
2. RPC method behind it: `metrics.get`
3. snapshot source: `internal/observability/observability.go`

### HTTP Prometheus format

Use:

```text
GET /metrics
```

This exposes the same in-memory snapshot in Prometheus text exposition format.

Use this when:

1. scraping from Prometheus
2. checking metric names quickly with `curl`
3. graphing timing and counter trends outside the app

Implementation path:

1. HTTP route: `internal/api/routes.go`
2. rendering: `internal/observability/observability.go`

### RPC / client access

Use RPC method:

```text
metrics.get
```

This is what the HTTP layer calls internally. Any thin client that can speak the project RPC protocol can request the same metrics snapshot directly.

Implementation path:

1. wire method: `internal/proto/proto.go`
2. RPC server handler: `internal/rpc/server.go`

## What the TUI does

The TUI metrics dashboard is not the source of truth for metrics. The daemon is.

The TUI records additional client-side metrics only when telemetry is attached with:

1. `WithTelemetry(...)` in `internal/tui/app.go`

That means:

1. the TUI process can record user-facing actions into its own local runtime metrics object
2. those metrics are not automatically merged into the daemon's `metrics.get`, `/metricsz`, or `/metrics` snapshots
3. the TUI now exposes a daemon-backed metrics overlay for inspecting daemon metrics live

Examples already recorded by the TUI include:

1. `tui.fulltext.fetch`
2. `tui.clipboard.copy.ok`
3. `tui.clipboard.copy.error`
4. `tui.notes.add`

So if you ask “how do I get daemon metrics in the TUI?”, the practical answer is:

1. start the daemon with `patentmine serve`
2. start the TUI with `patentmine tui`
3. open the metrics overlay with `M` or `:metrics`

If you ask “how do I get TUI-local metrics?”, the practical answer today is:

1. run the TUI with telemetry enabled
2. inspect the TUI process logs/runtime directly
3. do not expect those local counters to appear in the daemon snapshot yet

## TUI metrics overlay

The TUI now includes a daemon-backed metrics overlay.

Open it with:

1. `M`
2. `:metrics`

What it shows:

1. `Timings` tab
2. `Counters` tab
3. `Gauges` tab
4. grouped sections such as `rpc`, `engine`, `store`, `crawl`, `tui`, and `other`
5. compact inline tags derived from metric names
6. recent local history bars built from repeated daemon snapshots

Overlay behavior:

1. size is approximately 90% of the terminal width and height
2. default tab is `Timings`
3. refresh interval is `2s`
4. data source is daemon RPC `metrics.get`
5. history is session-local to the open overlay and is not persisted

Controls:

1. `tab` / `shift+tab` or `l` / `h` to switch tabs
2. `j` / `k` or arrow keys to move through rows
3. `r` to refresh immediately
4. `q` / `esc` to close

Implementation path:

1. command wiring: `internal/command/catalog.go`
2. app command handler: `internal/tui/app_commands.go`
3. key binding: `internal/tui/keymap/default.go`
4. overlay implementation: `internal/tui/overlay/metrics.go`

## What the daemon records

The daemon already records timing/counter data at multiple layers.

Examples:

1. `rpc.method.patent.list`
2. `engine.list_patents`
3. `store.sqlite.list_patents`
4. `store.sqlite.count_patents`

For crawl progress specifically, the daemon also records live gauges such as:

1. `crawl.crawler.crawled`
2. `crawl.crawler.discovered`
3. `crawl.crawler.pending`
4. `crawl.crawler.depth`
5. `crawl.crawler.max_depth`
6. `crawl.crawler.citations`
7. `crawl.crawler.cited_by`
8. `crawl.crawler.parents`
9. `crawl.crawler.children`

These are the latest values reported by the active crawl loop, so they are useful for checking whether a crawl is still making progress and which BFS depth it is currently processing.

This lets you see whether a slowdown is mostly:

1. request handling overhead
2. engine/business logic
3. SQLite query cost

## Tag sorting metrics

The normalized `SortByTags` path now has dedicated observability so you can measure its cost without changing the schema design.

### Store-level tag sort metrics

Recorded in `internal/store/sqlite/patent.go`:

1. `store.sqlite.list_patents.sort_tags_total`
2. `store.sqlite.list_patents.sort_tags`
3. `store.sqlite.list_patents.sort_tags.limit`
4. `store.sqlite.list_patents.sort_tags.offset`
5. `store.sqlite.list_patents.sort_tags.rows`

What they mean:

1. `sort_tags_total`: how often SQLite list queries sorted by tags
2. `sort_tags`: timing summary for the tag-sort list path
3. `limit`: latest requested page size for a tag-sort query
4. `offset`: latest requested offset for a tag-sort query
5. `rows`: latest number of rows returned by that query

### RPC-level tag sort metrics

Recorded in `internal/rpc/server.go`:

1. `patent_list.sort_tags_total`
2. `patent_relations.sort_tags_total`
3. `rpc.method.patent.list.sort_tags`
4. `rpc.method.patent.relations.sort_tags`
5. `rpc.method.patent.list.sort_tags.limit`
6. `rpc.method.patent.list.sort_tags.offset`
7. `rpc.method.patent.relations.sort_tags.limit`
8. `rpc.method.patent.relations.sort_tags.offset`

What they mean:

1. how often clients request tag sorting for the main list
2. how often clients request tag sorting for citations/cited-by/parents/children
3. request-level duration seen at the RPC boundary
4. the latest paging parameters used for those requests

### Warning logs for slow tag sorting

The RPC layer now emits warnings when tag-sorted list requests are slow.

Current warning messages:

1. `slow patent list tag sort`
2. `slow patent relations tag sort`

These logs include:

1. project
2. relation kind when applicable
3. limit
4. offset
5. duration in milliseconds
6. failed status

## Unsupported sort metrics

If a client requests a sort key the current server-defined table contract does not support, the RPC layer rejects it and records observability data.

Metrics:

1. `patent_list.sort_request_total`
2. `patent_list.sort_unsupported_total`
3. `patent_relations.sort_unsupported_total`

Logs:

1. `unsupported patent list sort`
2. `unsupported patent relations sort`

This protects against UI/backend drift.

## Reading the timing summaries

Each timing metric stores a summary, not a full trace. The fields include:

1. count
2. errors
3. total duration
4. min duration
5. max duration
6. last duration

That makes the metrics lightweight and fast, but it also means:

1. you can spot regressions and outliers
2. you cannot reconstruct every individual request from metrics alone
3. if you need exact event history, use logs and the activity journal alongside metrics

## Practical workflows

### Check daemon metrics in the TUI

1. start the daemon with `patentmine serve`
2. start the TUI with `patentmine tui`
3. open the metrics overlay with `M`
4. inspect the `Timings`, `Counters`, and `Gauges` tabs

### Check whether the TUI process is generating local metrics

1. run the app with telemetry enabled
2. perform a user action like full-text fetch or clipboard copy
3. remember those counters are local to the TUI process today
4. do not expect them in daemon `/metricsz` unless a future write-back path is added

### Check whether tag sorting is slow

1. sort a project list by tags in the TUI
2. fetch `/metricsz`
3. inspect:
   `rpc.method.patent.list.sort_tags`
   `store.sqlite.list_patents.sort_tags`
4. compare request-level time vs SQLite-level time

Interpretation:

1. if both are high, the DB query is the main cost
2. if RPC is much higher than store time, extra overhead exists above SQLite

### Check relation-list tag sorting

1. open citations, cited-by, parents, or children
2. sort by tags
3. fetch `/metricsz`
4. inspect:
   `patent_relations.sort_tags_total`
   `rpc.method.patent.relations.sort_tags`

## Benchmark

There is also a benchmark for the normalized tag-sort path in:

1. `internal/store/sqlite/sqlite_test.go`

Benchmark name:

1. `BenchmarkListPatentsSortByTags`

Use it when changing:

1. patent list SQL
2. tag joins
3. ordering expressions
4. paging behavior for sorted lists

## Current limitations

1. The TUI overlay displays daemon metrics only; it does not yet merge TUI-local process metrics.
2. Metrics are in-memory snapshots, not long-term storage.
3. Gauges show the latest observed value, not a historical series by themselves.
4. The overlay history bars are reconstructed from periodic snapshot polling, not from lossless event history.
5. True latency histograms and percentiles are not available from the current metric format.

## TODO

1. add an explicit `Client Session` section or tab for TUI-local metrics
2. add a daemon write-back path if TUI-local metrics should be visible in `metrics.get`
3. add server-side rolling history windows for shared recent-history views across reconnects
4. add true histogram buckets or sample storage for latency percentiles and better timing charts
5. add overlay filtering and sorting controls for large metric sets

## Summary

Use:

1. `/metricsz` for JSON inspection
2. `/metrics` for Prometheus scraping
3. `metrics.get` for direct RPC access
4. `M` or `:metrics` in the TUI for a live daemon metrics dashboard

Remember:

1. the daemon owns the metrics registry
2. the TUI overlay reads daemon metrics from `metrics.get`
3. TUI-local metrics are still process-local and separate today
4. tag sorting now has dedicated timing, counters, gauges, and slow-request warnings
