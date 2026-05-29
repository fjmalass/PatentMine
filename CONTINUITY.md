# Patent Continuity, Parentage & Family-Graph Export

This document explains how PatentMine models patent **continuity** (the parent/child lineage between related applications), how that data differs between the **USPTO** and **Google** sources, how the claim-parentage *type* is surfaced in the Parents view, and how to export a patent's family graph to Mermaid/Graphviz files.

---

## 1. What continuity is

A patent rarely stands alone. Through **continuity**, an application claims the benefit of an earlier application's filing date:

- **Continuation (CON)** — same disclosure, new claims.
- **Continuation-in-part (CIP)** — adds new matter to a parent.
- **Division (DIV)** — split out of a parent in response to a restriction requirement.
- **Provisional (PRO / PROV)** — a 12-month placeholder that does *not* start the 20-year term clock.
- **National Stage (371 / NST)**, **Reissue (REI)**, **Substitute (SUB)**, **Continued prosecution (CPA)**, and others.

These relationships form a directed graph (parent → child). It is usually a near-linear tree, but shared parents and re-filings can introduce diamonds and, occasionally, cycles. See also [U.S. Patent Expiration Date Computation](./EXPIRATION_DATE.md), which walks the same chain to find the **earliest term filing date**.

---

## 2. USPTO vs. Google: two sources of lineage

PatentMine resolves continuity from whichever source loaded the record (controlled by `PATENTMINE_SOURCE_MODE`; see [USPTO Loading & Source Configuration](./USPTO_CONFIG_LOADING.md)).

| Aspect | USPTO (Open Data Portal) | Google Patents |
| --- | --- | --- |
| Source field | `parentContinuityBag` from the ODP application API | Family / related-application edges scraped from the patent page |
| Stored in | `uspto_continuity` table | `relation` table (`relation_kind = parent`/`child`) |
| Claim-parentage **type** | **Yes** — `claimParentageTypeCode` + description (e.g. `CON` / "Continuation of") | **No** — Google exposes the link but not a typed code |
| Parent application number | `parentApplicationNumberText` | Inferred from the related document number |
| Granularity | Application-level, with status and filing dates | Document-level |

**Key difference:** only the **USPTO** source carries the *typed* claim-parentage code. A Google-loaded record still shows parents and children, but the **REL** column (below) will be blank for edges that have no stored USPTO continuity row. To get the parentage type, load the record with USPTO in the source policy (`uspto-first` or `uspto-only`) and supply an API key.

Both sources feed the **same** relation graph, so the family-graph view, citation panes, and counts behave identically regardless of origin.

---

## 3. The Parents view and the `REL` column

Open a patent and press **`p`** (`:open.parents`) to list its parents. The table adds a compact **`REL`** column showing the claim-parentage *code*:

```
#    NUMBER          ...   PARENTS  REL
1    US8126680        ...   7        CON
2    US60258246       ...   0        PRO
```

- The column shows the **code** (`CON`, `CIP`, `DIV`, `PRO`, …) to stay narrow.
- When a parent row is focused, the **status line** shows the full phrase ("Continuation of"), resolved through a canonical lookup table (`internal/uspto/parentage.go`). The table is the source of truth; if a code is unknown, PatentMine falls back to the description text the USPTO API supplied.
- States: a real code, or blank when no USPTO continuity row links that parent (e.g. Google-only edges).

### Observability (visible under the `M` overlay)
The engine records, per Parents load:

- `engine.relations.parentage` — attach timing.
- `engine.parents.<category>` counters — `continuation`, `continuation_in_part`, `division`, `provisional`, `national_stage`, `reissue`, `other`.
- `engine.parents.total` gauge.
- A `"loaded continuity parents"` log line with per-category counts.

---

## 4. Family graph: DFS layout & cycles

Press **`f`** / **`g f`** (`:open.family-graph`) to open the bounded parent/child graph rooted at the selected patent.

- **Traversal** is depth-bounded and node-capped, cycle-safe via a `visited` set.
- **Layout** is a **depth-first forest**: each lineage is fully expanded before its siblings, with indentation reflecting DFS nesting. The focus patent's line leads.
- Because the graph may not be a tree, a node reached a second time (shared parent or cycle) is emitted as a **back-reference** row — marked `↩` and colored — instead of being expanded again. This shows the linkage and guarantees the layout terminates.
- Row legend: `R`=root, `M`=merge (multiple parents), `B`=branch (multiple children), `X`=cross-edge, `↩`=cycle/back-reference.
- Adjust depth with `[` / `]` or `-` / `+`; filter by country.

DFS layout timing is recorded as `tui.family.dfs_layout`, with `tui.family.dfs_nodes` and `tui.family.dfs_back_edges` gauges (all visible under `M`).

---

## 5. Exporting the family graph

Press **`y`** (`:family.mermaid`) in the family-graph view. PatentMine:

1. Generates a **Mermaid** (`flowchart TD`) diagram and a **Graphviz DOT** (`digraph`) diagram of the current graph.
2. Writes **both** files to the patents cache directory.
3. Copies the Mermaid source to the clipboard as well.

In both formats, cross/back edges (cycles, depth-inconsistent links) are drawn **dashed and red**, and the root node is highlighted, so a non-tree family is easy to read in a laid-out render.

### Export location

Files are written to the patents directory — by default:

```
~/.config/patentmine/patents/<ROOT>_<YYYYMMDD-HHMMSS>.mmd
~/.config/patentmine/patents/<ROOT>_<YYYYMMDD-HHMMSS>.dot
```

`<ROOT>` is the focus patent number (e.g. `US12945822`) and the timestamp is the generation time, so repeated exports never overwrite. The directory is `cfg.PatentsDir` (`$PATENTMINE_HOME/patents`); on most systems `$PATENTMINE_HOME` resolves to `~/.config/patentmine`. The status line reports the written path.

Render the files with any Mermaid tool (e.g. the Mermaid Live Editor, or VS Code's Mermaid preview) or Graphviz (`dot -Tsvg US12945822_20260529-141500.dot -o family.svg`).

### Observability (visible under the `M` overlay)
- `tui.family.mermaid_gen`, `tui.family.dot_gen`, `tui.family.export_write` — generation and write timings.
- `tui.family.export.ok` / `tui.family.export.error` — counters.
- `tui.family.export.nodes` / `tui.family.export.back_edges` — gauges.
- A `"family graph exported"` log line with both paths and node/back-edge counts.

---

## 6. Code map

| Concern | File | Notes |
| --- | --- | --- |
| Parentage code lookup table & categories | `internal/uspto/parentage.go` | `ParentageLabel`, `ParentageCategory`, code constants |
| Continuity storage | `internal/store/sqlite/source_data.go` | `uspto_continuity` table; `USPTOContinuities`, `ParentageForChild` |
| Parents attach + metrics | `internal/engine/engine_crawl.go` | `Relations` → `attachParentage` |
| Parents `REL` column | `internal/tui/pane/table.go`, `internal/tui/pane/citations.go` | `PatentColumnParentage` |
| Family graph DFS + export | `internal/tui/pane/family_graph.go` | `dfsRows`, `mermaidGraph`, `dotGraph`, `exportGraph` |
| Family graph traversal | `internal/engine/engine_family_graph.go` | `FamilyGraph` (cycle-safe walk) |
