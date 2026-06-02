# TUI `:add` Execution Flow Sequence Diagram

This document contains the execution flow sequence diagram for the TUI `:add` command across all `source.mode` settings.

Related bulk commands:

- **`:add.file <path>`** (aliases `add-file`, `load`) — reads a plain-text patent list and adds every number to the **currently active project** (the file's `# project:` line is informational only and is ignored), each with `direct` (manual) provenance, exactly as repeated `:add` would. The daemon reads, validates the header, and parses the file; ambiguous USPTO matches are reported as failures rather than prompting, so the bulk run stays non-interactive. When it finishes, a result popup summarizes how many of how many were added and gives a per-number reason for each failure, so a partial import never silently drops patents. REST: `POST /projects/{id}/added`.
- **`:export.added [path]`** (aliases `export-added`, `add.export`) — writes the active project's manually-added patents (memberships with `direct`/`manual` provenance) to a plain-text list file that `:add.file` can reload. With a `path` it writes there directly (warning first if that file already exists); with no argument it opens a confirmation popup showing the proposed default location before writing. Either way a result popup then reports how many patents were exported and where. REST: `GET /projects/{id}/added/export` (count also in the `X-Patent-Count` header).

Both operations emit activity records (`added.import` / `added.export`) and metrics, and each imported patent also records its own `membership.add` activity, so the full history is preserved. The file format carries a mandatory magic header (`# patentmine added-patents v1`) so a wrong-kind file is rejected on import.

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant TUI as TUI App (cmdAddToProject)
    participant RPC as RPC Daemon (membership.add)
    participant Engine as Engine (AddToProject)
    participant DB as SQLite Database
    participant Crawl as Crawl Background Job
    participant Registry as Source Registry (FetchExcluding)

    User->>TUI: Types ":add US11549750B1"
    TUI->>RPC: Method "membership.add" with empty Source string
    RPC->>Engine: calls s.engine.AddToProject()
    
    %% Phase 1: Proactive Local Registry check
    Note over Engine, DB: Phase 1: Proactive Registry Check
    Engine->>DB: RecordOf("US11549750B1") (looks up existing document stages)
    alt document number already mapped to a record
        DB-->>Engine: Returns existing record number
    else document number unknown
        DB-->>Engine: store.ErrNotFound
        
        %% Phase 2: ODP Candidate Resolution
        alt mode is USPTO-preferred
            Engine->>Registry: Query USPTO ODP Candidates
            Registry-->>Engine: Returns candidate wrappers
            alt multiple candidates found
                Engine-->>RPC: Returns candidates list
                RPC-->>TUI: Triggers USPTOCandidatePicker Overlay
                User->>TUI: Selects correct candidate application
                TUI->>RPC: Retries membership.add with selected application number
                RPC->>Engine: calls s.engine.AddToProject()
            else exactly 1 candidate found
                Engine->>Engine: Resolves to canonical Application ID
            end
        end
    end

    %% Phase 3: Ingestion & Crawl
    Engine->>Engine: e.ensureRecord() (creates stub under canonical record in DB if new)
    Engine->>Engine: e.repo.AddMembership() (review_state = "unknown")
    Engine->>Crawl: e.StartFamilyCrawl()
    Crawl->>Registry: c.fetchFromSource() with empty Source string
    
    note over Registry: Evaluates current source.mode policy
    
    alt mode == "compare"
        Registry->>Registry: Try File Cache -> Try USPTO -> Try Google (compares & diffs)
    else mode == "uspto-first"
        Registry->>Registry: Try File Cache -> Try USPTO -> Fallback to Google on 404
    else mode == "uspto-only"
        Registry->>Registry: Try File Cache -> Try USPTO (skips Google entirely)
    else mode == "google-only"
        Registry->>Registry: Try File Cache -> Try Google (skips USPTO entirely)
    end

    Registry-->>Crawl: Returns fetched Result
    Crawl->>Engine: c.ingestNode() (Patent FetchState -> "cached")
    Crawl-->>Engine: Background crawl job completes successfully
    Engine->>Engine: cleanupIfNotFound() event handler triggers
    Engine->>Engine: Promotes root ReviewState to "under_review"
```

## Detailed Execution Steps:

1. **TUI Parsing & Invocation**:
   - The user inputs `:add US11549750B1`.
   - The TUI maps this to `AddToProject` command in `internal/command/catalog.go`.
   - The bubble tea handler `cmdAddToProject` in `internal/tui/app_commands.go` dispatches `pane.AddToProjectFromSourceCmd` with an empty source string (`""`).

2. **RPC Invocation**:
   - The client makes a `membership.add` RPC call using `proto.MembershipParams{Project: project, Patent: number, Source: ""}`.
   - The server handler `s.membershipAdd` in `internal/rpc/server.go` receives the call and delegates to `s.engine.AddToProject(ctx, p.Project, p.Patent)`.

3. **Proactive Registry Check & Candidate Resolution**:
   - Before writing any new record, the engine performs a proactive lookup `RecordOf` in the `document` table. If the document stage is already known to belong to a parent record, it reuses that record number directly.
   - If unknown and in a USPTO-preferred mode, the engine searches the USPTO ODP. If multiple application wrappers match, a `USPTOCandidatePicker` overlay is opened in the TUI so the user can select the correct application.
   - If exactly one application candidate is found, the engine automatically resolves the requested patent to the canonical Application ID before stub creation. This proactive mapping avoids creating duplicate stubs for different stages (publications vs grants) of the same invention.

4. **Engine-Side Ingestion**:
   - `AddToProject` in `internal/engine/engine_project.go` starts a background crawl via `e.StartFamilyCrawl(ctx, record, 0, domain.CrawlProfileAll, false)`.

5. **Background Crawl & `source.mode` Policy Enforcement**:
   - The crawl calls `c.fetchFromSource` which queries the `registry.FetchExcluding` method.
   - The registry enforces the configured `source.mode` (e.g. `compare`, `uspto-first`, `uspto-only`, `google-only`) to fetch from local/external sources.

6. **Post-Crawl Review State Promotion & Merging**:
   - Upon successful crawl, the patent gets saved as `cached`. Discovered citations are saved as `stub`.
   - During ingestion, any overlapping stub records are automatically resolved and deep-merged via `MergeRecords`.
   - The done event is caught by `cleanupIfNotFound` which automatically promotes the root patent's review state to `under_review`.
   - The addition trigger records a `membership_provenance` entry (`direct` for manual adds, `related` for family expansions, and `system`/`auto` for crawler operations).

## Membership Provenance & Icons

Every membership records *how* a patent entered a project in its `added_method` field. The TUI renders each provenance with a glyph (`Theme.ProvenanceGlyph`, defined in `internal/tui/render/theme.go`); the detail pane shows it next to a human-readable label. `:export.added` exports only the **manual** rows; `:add.file` writes new rows back as **manual**.

| Glyph | `added_method` value(s) | Meaning | Exported by `:export.added`? |
|:---:|---|---|:---:|
| 🔤 | `direct`, `manual`, *(empty)* | Manually added via `:add` / `:add.file` (Direct Ingestion) | ✅ Yes |
| 👪 | `related` | Promoted from a family neighbor (parent/child expansion) | ❌ No |
| 🏡 | `neighbors` | Promoted from a depth-1 neighbor auto-assign | ❌ No |
| 🤖 | `system`, `auto…` | Added by a crawler/system operation | ❌ No |
| 🔗 | `cites`, `citation` | Promoted from a citation edge | ❌ No |
| 👈 | `cited_by`, `cited` | Promoted from a cited-by edge | ❌ No |
| ⬆️ | `parent` | Promoted from a parent edge | ❌ No |
| ⬇️ | `child` | Promoted from a child edge | ❌ No |
| 🔄 | `loading` | Stub still loading/being discovered | ❌ No |
| ❓ | *(anything else)* | Unknown provenance | ❌ No |

An empty `added_method` predates provenance tracking and is treated as manual, matching `filterexpr.ParseProvenance` and the engine's `isManualMethod` helper.

---

## Searching & Filtering Added Patents

Once patents are added to a project, they can be searched, highlighted, and filtered dynamically in the **Catalog** view:
* Press **`/`** to open the Find/Filter query prompt.
* Press **`Tab`** while typing to dynamically cycle the search scope:
  * **All Columns**: Broad search matching title, abstract, and patent number.
  * **Number**: Filters by patent number substring.
  * **Title**: Limits full-text search (FTS) to patent titles.
  * **Inventor**: Filters by patent inventors.
  * **Class**: Filters by patent classifications.
  * **Assignee**: Filters by current assignee.
  * **Tags**: Filters by tags assigned in the active project.

