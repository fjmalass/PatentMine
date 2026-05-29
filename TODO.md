# PatentMine TODO & Architecture Notes

Durable record of open work and the reasoning behind deferred architecture
decisions. Sort/pagination telemetry referenced here lives in
[Continuity & Parentage](./CONTINUITY.md) §3 and the [Metrics Guide](./metrics.md).

---

## Decision (recorded): sorting stays server-side

Table sorting is **authoritative on the daemon** — SQL `ORDER BY` + `LIMIT/OFFSET`
in `patentSortExpr` (`internal/store/sqlite/patent.go`), driven by
`SortColumn`/`SortAscending` on the RPC params. The TUI and the REST/website are
both thin clients of the same methods (`patent.relations`, `patent.list`).

Why not sort in the interface:

- **Pagination correctness.** Page size is 10 (`defaultPageSize`); a pane holds
  only the current page. Sorting in the pane reorders 10 rows, not the full set
  — page 2 would sort independently of page 1. To sort client-side correctly you
  must first pull the whole set, which moves I/O rather than saving it.
- **One comparator, two front-ends.** Server sort gives the website identical
  ordering for free (`?sort=&asc=`). A client comparator would be re-implemented
  in JS and drift (empty-code ordering, tie-breaks, collation).
- **DB consistency.** The server re-reads committed state on every query; a
  client snapshot goes stale the moment a crawl/edit/REST write lands.
- **The "snappier client" gain is mostly imaginary here.** The daemon is local
  (socket/localhost), the query is indexed, the page is 10 rows → sub-ms round
  trip. The client-sort speed argument assumes WAN/large-set latency this
  deployment does not have.

Revisit only if the sort telemetry proves a real bottleneck (see last checkbox).

---

## Pagination strategies considered

| Approach | Complexity | Best-fit size | Prior art |
| --- | --- | --- | --- |
| **Offset/limit per page** (current) | Trivial | Small/bounded | SQL `LIMIT/OFFSET` everywhere |
| **Keyset / cursor (seek)** | Moderate; no random page jump | Large / infinite feeds | Stripe `starting_after`, GitHub & Slack cursors, Elasticsearch `search_after` (which deprecated `from/size` deep paging) |
| **Full client cache + local paginate** | Moderate + cache invalidation | Bounded (thousands, not millions) | ag-Grid client-side row model, TanStack Table client mode, DataTables.js, spreadsheets |
| **Sliding-window prefetch + virtualized render** | High (coherence, eviction, prefetch races, scrollbar sizing) | Any size | ag-Grid infinite/server-side models, DataGrip chunked fetch, Kibana log scroll |

Notes:
- Offset/limit's weaknesses: deep `OFFSET` scans-and-discards, and rows shifting
  between fetches cause skipped/duplicated rows across page boundaries.
- "Cache all + paginate locally" does **not** require moving sort to the client:
  the server can sort the full set once, the client caches the *ordered* list and
  flips pages in RAM. That keeps web parity while feeling instant.
- Virtualized rendering is orthogonal — it sits on top of any of the above.

---

## Recommended next steps (not scheduled)

- [ ] **Relations/parents views (small, bounded):** server sorts the full set
  once → client caches the ordered list → paginate/render locally. Generalize
  the existing `loadAllPages` (`internal/tui/pane/citations.go`) into the render
  path. Sort stays server-owned. Invalidate the cache off engine change events
  (`announceChange` / engine `Bus`).
- [ ] **Main patent table (potentially large):** add keyset/cursor pagination to
  kill deep-offset cost; prefetch ±1 page for smooth scroll. Do not materialize
  the whole set.
- [ ] **Selection hardening (prereq for any caching):** make the single TUI cursor
  **patent-number-keyed** across re-sort/reload so the highlighted row survives
  reordering. Visual multi-select already keys by number (`allSelectedNumbers`);
  the single cursor in `Citations` does not yet.
- [ ] **Gate the above on telemetry.** Only build the client-cache/hybrid if a
  sort is measured slow via `store.sqlite.list_patents.sort_*` and
  `rpc.method.patent.relations.sort_*`. Instrument first (done), optimize later.

---

## Done this session

- Parentage `REL` column moved to 3rd position (after NUMBER, before TITLE) and
  made sortable.
- Server-side SQL sort for parentage (`patentSortExpr` correlated subquery over
  `uspto_continuity`), `SortByParentage` wired (`internal/domain`).
- Sort telemetry at both layers: `observeSortDuration` (store) and
  `observeRelationsSort` (rpc), label-driven metric names.
- Removed the useless `.rows` gauge (snapshot of last call's row count) from the
  sort telemetry — both tags and parentage.
- Family-graph Mermaid/DOT generation moved to the daemon
  (`internal/export/familygraph`) and exposed over REST
  (`GET /patents/{number}/family.mmd|.dot`).
