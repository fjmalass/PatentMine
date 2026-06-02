# TUI vs Web GUI performance comparison

Compare **daemon work**, not terminal paint vs DOM layout.

## 1. Daemon timings (authoritative)

```bash
# Before scenario
curl -s http://127.0.0.1:18080/metricsz | jq '.timings'

# Run scenario (see scripts/bench-api.sh)
./scripts/bench-api.sh

# After scenario — inspect timing deltas
curl -s http://127.0.0.1:18080/metricsz | jq '.timings'
```

Key metrics: `store.sqlite.patent.list.*`, crawl, USPTO fetch (see [metrics.md](metrics.md)).

## 2. REST end-to-end

`scripts/bench-api.sh` records wall-clock for:

- patent list (cold + warm)
- patent get
- relations
- family graph
- crawl start
- metrics snapshot

Use the same `project`, `filter`, and `limit` as the TUI session.

## 3. Web GUI perceived latency

In Safari dev tools → Network, note `fetch` duration for each action. Add render time in Performance panel if needed.

The web app logs no separate metrics today; use browser timestamps vs `/metricsz` RPC timings.

## 4. Fair comparison rules

- Warm the DB: run each list query twice; report the second.
- Match query params: `project`, `filter`, `sort_column`, `sort_ascending`, `limit`, `offset`.
- Remote iPad: measure LAN and VPN separately; expect extra RTT on HTTP vs local Unix socket TUI.

## 5. TUI-local metrics

TUI-only counters (clipboard, etc.) are **not** in `/metricsz` unless pushed via `POST /metrics`. See [metrics.md](metrics.md).
