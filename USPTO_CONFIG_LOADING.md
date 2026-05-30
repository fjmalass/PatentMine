# USPTO Loading & Source Configuration

This document covers everything needed to load patents from USPTO into
PatentMine: API key setup, the four provider-policy modes, the `:add` family
of commands (single and batch), and the XML download / parse / full-text
pipeline that sits behind them.

Related docs:

- [README.md](./README.md) — architectural overview, identity & data model.
- [ACTIVITY.md](./ACTIVITY.md) — telemetry & journal.
- [metrics.md](./metrics.md) — counters and timings.

---

## 1. API Key Setup

PatentMine talks to the USPTO Open Data Portal (ODP) for bibliographic
searches and to the USPTO bulk dataset for grant / pre-grant XML files. Both
endpoints accept the same `x-api-key` header. PatentMine reads the key from
the `PATENTMINE_USPTO_API_KEY` environment variable.

`PATENTMINE_USPTO_API_KEY` is the only supported USPTO key variable; legacy
generic names are intentionally ignored so credential provenance is explicit.

### 1.1 Where configuration is loaded from

At startup, PatentMine loads configuration from the shell environment and from
`.env` files. The loader checks these files in order:

1. `./.env` in the current working directory.
2. `~/.ssh/patentmine/.env`.
3. `$PATENTMINE_HOME/.env`, where `PATENTMINE_HOME` defaults to the user's
   config directory plus `patentmine`.

If a variable is already set in the shell, the `.env` value does not overwrite
it. This lets you temporarily override a saved setup for one terminal session.

`.env` values support simple `${VAR}` expansion, which is useful for keeping a
single credentials directory variable:

```dotenv
PATENTMINE_CREDENTIALS_DIR=~/.ssh/patentmine
PATENTMINE_USPTO_API_KEY=file:${PATENTMINE_CREDENTIALS_DIR}/uspto_odp_key
PATENTMINE_SOURCE_MODE=uspto-first
```

### 1.2 Direct value

```bash
export PATENTMINE_USPTO_API_KEY=ABCDEFGHIJ1234567890
```

This is fine for one-off testing, but it leaves the secret in shell history or
process environment. Use `file:` for regular use.

### 1.3 `file:` indirection (recommended)

`file:` makes PatentMine read the key from a file on disk instead of carrying
it through process environment. `~/` is expanded to `$HOME`, and whitespace is
trimmed from the file contents.

Create the key file:

```bash
mkdir -p ~/.ssh/patentmine
printf '%s\n' 'ABCDEFGHIJ1234567890' > ~/.ssh/patentmine/uspto_odp_key
chmod 600 ~/.ssh/patentmine/uspto_odp_key
```

Use it directly from the shell:

```bash
export PATENTMINE_USPTO_API_KEY=file:~/.ssh/patentmine/uspto_odp_key
```

The file should contain only the key (whitespace trimmed). This is the
shape `.env` uses out of the box:

```dotenv
# .env
PATENTMINE_CREDENTIALS_DIR=~/.ssh/patentmine
PATENTMINE_USPTO_API_KEY=file:${PATENTMINE_CREDENTIALS_DIR}/uspto_odp_key
```

### 1.4 Full minimal `.env`

For a USPTO-first workstation setup, use:

```dotenv
PATENTMINE_HOME=~/.config/patentmine
PATENTMINE_CREDENTIALS_DIR=~/.ssh/patentmine
PATENTMINE_USPTO_API_KEY=file:${PATENTMINE_CREDENTIALS_DIR}/uspto_odp_key
PATENTMINE_SOURCE_MODE=uspto-first
```

`PATENTMINE_HOME` is optional. Set it only when you want the database, socket,
logs, activity journal, and exports under a specific directory.

### 1.5 Verifying

The daemon logs `service check uspto connected` when the startup probe
succeeds. From a running daemon you can also run:

```bash
patentmine check uspto
```

which prints the resolved status code, request/response bytes, and the
rate-limit headers returned by ODP. The TUI surfaces the same state in the
status bar shortly after launch.

Troubleshooting checklist:

- `patentmine check uspto` reports HTTP 401/403: the key is missing, wrong, or
  expired according to USPTO.
- The command reports `read ...`: the `file:` path is wrong or permissions block
  the process from reading it.
- `:add.uspto` works but XML fetch fails: the ODP search endpoint is reachable,
  but the saved USPTO record may not have a grant/pgpub XML URL, or the bulk
  file endpoint is temporarily unavailable.
- A shell export appears ignored: check whether another shell variable is
  already set. Shell values win over `.env` values.

### 1.6 Where the key is used

| Surface | Endpoint | Notes |
| --- | --- | --- |
| `:add.uspto` / `:lookup` | `api.uspto.gov/api/v1/patent/applications/search` | One request per patent. Subject to USPTO's 1 s minimum interval. |
| Auto / manual XML fetch | `api.uspto.gov/api/v1/datasets/products/files/...` | Bulk dataset. **No** minimum-interval rate limit on PatentMine's side — parallel downloads OK. |
| Browser open of USPTO Source URL | URL is opened with `?api_key=` appended so the browser can hit ODP directly. |

Do not commit `.env` files or key files. Keep the actual key in a private file
and store only the `file:` reference in local config.

---

## 2. Source Mode Policy

`SourceMode` decides which providers PatentMine consults on every crawl /
lookup. Set it once at boot time:

```dotenv
# .env
PATENTMINE_SOURCE_MODE=uspto-first
```

Change it at runtime from the TUI command palette:

```
:source.mode uspto-first
```

Bare `:source.mode` (no arg) prints the current setting.

### 2.1 The four modes

| Mode | Behavior |
| --- | --- |
| `compare` | Fetch USPTO **and** Google for every patent. Slowest. The USPTO record stays authoritative; Google is stored as a `source_diff` row so divergences are queryable. Use this when researching data quality across providers. |
| `uspto-first` (recommended default) | Try USPTO first. Fall back to Google only when USPTO returns no record (foreign patents, very old grants). The patent's `Source` field reflects who actually answered. |
| `uspto-only` | USPTO is the **only** source consulted. A missing record surfaces as an error instead of being silently substituted with Google data. Choose this when you want strict provenance. |
| `google-only` | USPTO is never consulted. Useful for foreign patents, legacy records, or USPTO outages. The API key is not used in this mode. |

The mode is enforced inside `Registry.FetchExcluding` in `internal/crawl/source.go`.
Type-safety: the value is a `domain.SourceMode` typed string, so the four
constants are the only valid identifiers in code.

### 2.2 Choosing the right mode

Use `uspto-first` for normal US patent work. It gives USPTO provenance when a
record exists and still lets Google cover gaps such as foreign records, older
records, or USPTO outages.

Use `uspto-only` when provenance matters more than convenience. This is the
right mode when you are validating USPTO data quality or want failures to be
visible instead of silently filled by Google.

Use `compare` when you want to investigate source differences. The USPTO record
is still the saved authoritative patent row, but Google is fetched too and field
differences are recorded as `source_diff` rows.

Use `google-only` when the USPTO key is unavailable or when most of the current
work is non-US prior art.

Source-specific commands override the mode:

- `:add.uspto` always uses USPTO and does not fall back to Google.
- `:add.google` always uses Google and does not consult USPTO.
- `:add` uses the current `:source.mode` policy.

### 2.3 Telemetry

Each mode emits a gauge on switch so dashboards can pin live state:

- `engine.source_mode.compare`
- `engine.source_mode.uspto-first`
- `engine.source_mode.uspto-only`
- `engine.source_mode.google-only`

Exactly one is `1` at any time; the others are `0`. The counter
`engine.source_mode.set_total` bumps on every change.

---

## 3. `:add` Command Family

All three commands link a patent to the active project, then arm a
single-patent fetch. They differ in **which** provider performs that fetch.

| Command | Source override | Effect |
| --- | --- | --- |
| `:add` | none | Use whatever `:source.mode` is configured. |
| `:add.uspto` | USPTO | Force USPTO. No Google fallback, regardless of mode. |
| `:add.google` | Google | Force Google. No USPTO. |

### 3.1 Forms accepted

All three commands accept three input forms:

1. **Cursor / selection** — no arguments, single patent under the cursor or
   the current visual selection. Multi-patent selections dispatch one RPC per
   patent in parallel.

   ```
   :add.uspto
   ```

2. **Single typed number**:

   ```
   :add.uspto 17730671
   ```

3. **Batch typed numbers** — space-separated:

   ```
   :add.uspto 17730671 17696256 18493058
   ```

   Each number is parsed up-front; a single typo aborts the batch before any
   RPC is dispatched. Successful parses fire one `membership.add` RPC each in
   parallel. The status line shows `adding N patents via uspto in parallel…`.
   Counters: `tui.add.batch.started`, `tui.add.batch.total`.

### 3.2 What happens after `:add.uspto`

```
  membership.add  ──►  USPTO search (ODP)
                       │
                       │  match found
                       ▼
                  uspto_application row saved
                       │
                       ▼  (after crawl completes)
                  autoFetchUSPTOXMLAfterCrawl  ──► grant XML if present,
                                                   else pgpub XML
                       │
                       ▼
                   uspto_xml_download bump  +  parse  +  uspto_grant_body /
                                                        drawings /
                                                        citations /
                                                        classifications /
                                                        relations
                       │
                       ▼
                  patent citations normalized into relation graph
```

The auto-fetch listener is armed by every `startFamilyCrawl`, so `:lookup`
on a USPTO-resolvable patent ingests its grant XML on completion too.

The ODP file-wrapper search does not carry prior-art citations. Citations are
loaded only after the XML path runs. PatentMine still stores every parsed
reference in `uspto_grant_citation`; patent references are also converted into
normal `relation` rows so `:open.citations`, citation counts, highlighting, and
project graph workflows see them. Non-patent literature references remain in
the USPTO citation table for downstream use.

### 3.3 Normal load recipes

Fresh USPTO load of one patent:

```
:add.uspto 17730671
```

Batch USPTO load:

```
:add.uspto 17730671 17696256 18493058
```

Use configured provider policy:

```
:source.mode uspto-first
:add 17730671
```

Load selected rows from the catalog:

1. Move the cursor to one patent, or press `v` and select multiple rows.
2. Run `:add.uspto` with no arguments.
3. Watch the status line for the add and auto XML-fetch progress.

If the record is already present as a stub, the USPTO fetch fills it in. If the
record is already cached, use a force lookup/crawl path or manual XML fetch when
you specifically need to refresh side data.

### 3.4 Candidate picker

If the ODP search returns multiple matching applications for the requested
number, the daemon returns the list and the TUI opens the
`USPTOCandidatePicker` overlay (80% × 80%) so the user picks one. The
selected candidate's application number is then used for the actual add.

The broad search checks these fields where available:

- `applicationNumberText`, for application numbers such as `17812078` or
  `17/812,078`.
- `patentNumberText` / `patentNumber`, for grant numbers such as `12614626` or
  `US12614626B2`.
- `publicationNumberText` / `publicationNumber`, for publication numbers such
  as `20230021336` or `US20230021336A1`.

In the picker, use `up` / `down` or `j` / `k`, then press `Enter` to select the
correct application. If none is correct, close the overlay and try a more exact
application, publication, or grant number.

### 3.5 Identifier Matching & Life-Stage Resolution

When searching or adding a patent via the USPTO, the daemon retrieves a candidate ODP wrapper list. Each candidate wrapper (`usptoWrapperData`) contains multiple identifier fields representing the same underlying invention at different stages of its life cycle:
- `w.ApplicationNumberText` (e.g., `14/283,408`)
- `w.ApplicationMetaData.PatentNumber` / `w.ApplicationMetaData.PatentNumberText` (e.g., `US11611785B2`)
- `w.ApplicationMetaData.PublicationNumber` (e.g., `US20220252571A1`)

To reliably identify if a candidate wrapper matches the target `domain.PatentNumber` requested by the user, the crawler uses a unified parsing and matching approach:

1. **Serial-Based Identity Matching (`matchesPatent`)**:
   - Compares the candidate raw identifiers directly against the target `domain.PatentNumber`.
   - Uses the unified parser `domain.ParsePatentNumber` to strip separators (such as `/`, `-`, commas, and spaces) and extract the core serial digits.
   - Compares the normalized serial digits (after stripping leading zeros). This avoids matching failures caused by different prefixes or suffixes (like kind codes `A1` vs `B2`), which represent different stages of the *same* invention record.

2. **Life-Stage Classification**:
   - The kind codes determine a patent document's exact stage with 100% certainty:
     - Kind code starts with **`B`** (e.g., `B1`, `B2`) -> **Grant** (100% granted patent).
     - Kind code starts with **`A`** (e.g., `A1`, `A2`) -> **Pre-grant publication** (published application).
     - Empty kind code -> **Application** (un-published application serial).
   - The helper `domain.GuessStage` parses the kind code to assign the correct stage to stub records (e.g., during citation walks). This maps them under their correct stage in the database (e.g. `publication US20100282272A1`) and displays them correctly in the TUI detail pane.

3. **Telemetry & Logs**:
   - The parsing and matching process is tracked in-process using these telemetry counters on `observability.Metrics`:
     - `crawl.uspto.matches_patent.calls_total`: Total match attempts.
     - `crawl.uspto.matches_patent.parse_success_total`: Successful parses through `domain.ParsePatentNumber`.
     - `crawl.uspto.matches_patent.parse_fallback_total`: Fallbacks to raw digit stripping.
     - `crawl.uspto.matches_patent.matched_total`: Successful matches.
   - Structured debug logs (`slog.Debug`) record the raw input, the target patent, the parsed serial, and match success/failure for precise tracing.

---

## 4. Manual XML Fetch

Beyond the auto-fetch wired into `:add.uspto`, two commands let you
explicitly download a patent's XML:

| Command | Source | Notes |
| --- | --- | --- |
| `:fetch.uspto.pgpub` | Pre-grant publication XML | Useful before grant. |
| `:fetch.uspto.grant` | Grant XML | Always preferred when available. |

Both honor multi-selection and run downloads in parallel. The daemon serves
each request on its own goroutine and USPTO's bulk dataset has no
minimum-interval limit, so dozens of concurrent fetches are fine.

### 4.1 Cache semantics

Each `(application_number, kind)` pair is tracked in `uspto_xml_download`:

```
application_number    TEXT
kind                  TEXT  -- 'pgpub' or 'grant'
source_url            TEXT
local_path            TEXT  -- absolute path on disk
bytes                 INTEGER
download_count        INTEGER
first_downloaded_at   TEXT
last_downloaded_at    TEXT
last_accessed_at      TEXT
```

`download_count` increments on **every** request — cache hits included — so
the tally reflects how often each document has been wanted. `bytes` and
`last_downloaded_at` only refresh on a real network download.

### 4.2 Detail-pane shortcut

In the detail view, place the cursor on either the `PGPub URL` /
`Grant URL` row (or the `PGPub XML` / `Grant XML` filename row above it) and
press **Enter**. PatentMine pops a spinner overlay, fetches (or serves the
cached copy), and writes the saved path into the status line. xdg-open is
intentionally **not** invoked — on Linux it would open the `.xml` in the
default browser, which is not the user goal.

### 4.3 Viewing USPTO links in the TUI

PatentMine exposes source and XML links in three places:

| Location | What you see | Action |
| --- | --- | --- |
| Catalog / citations table | Patent rows and citation counts | Select a row and press `w`, or run `:browse`, to open the provider URL in the browser. |
| Detail pane | `Source URL`, `PGPub URL`, `Grant URL`, `PGPub XML`, and `Grant XML` rows when known | Use these rows to confirm which USPTO endpoints and XML filenames are attached to the record. |
| Full-text pane | Parsed claims, abstract, and description | Run `:open.fulltext` or press `T` from Detail. USPTO XML is preferred when present. |

Browser behavior:

- For USPTO-loaded records, `:browse` opens the saved ODP source URL.
- If the URL host is `api.uspto.gov` and a USPTO key is configured, PatentMine
  appends `api_key=<key>` to the opened browser URL because the browser cannot
  send the `x-api-key` header that the daemon uses internally.
- For non-USPTO records, `:browse` falls back to the saved provider URL or a
  Google Patents URL for the selected number.

Explicit browse commands:

| Command | Behavior |
| --- | --- |
| `:browse` | Open the saved source URL when present; otherwise fall back to Google Patents. |
| `:browse.uspto` | Open the USPTO grant XML URL when present; otherwise open the USPTO pre-grant publication XML URL. |
| `:browse.uspto.grant` | Open only the USPTO grant XML URL. If missing, report a status-line error. |
| `:browse.uspto.pgpub` | Open only the USPTO pre-grant publication XML URL. |
| `:browse.uspto.pub` | Alias for `:browse.uspto.pgpub`. |
| `:browse.google` | Open Google Patents, regardless of saved source. |

Each browse command accepts typed patent numbers, just like `:browse`, for
example `:browse.uspto.grant 17730671` or `:browse.google US11921100B2`.
PatentMine records browse metrics, logs, and activity telemetry with the
requested target (`default`, `uspto`, `uspto_grant`, `uspto_pgpub`, `google`),
the actual provider/kind opened, and success/error status. URLs in activity
records have browser API keys redacted.

Detail-pane XML fetch behavior:

1. Open Detail with `Enter` / `l`.
2. Move the cursor to `Grant URL`, `PGPub URL`, `Grant XML`, or `PGPub XML`.
3. Press `Enter`.
4. PatentMine fetches the corresponding XML, parses it, updates parsed body
   tables, saves citations/classifications/drawings, and adds patent citations
   to the normal relation graph.

Citation viewing:

- Press `c` or run `:open.citations` to show patents cited by the selected
  patent.
- Press `b` or open the cited-by view to show patents that cite the selected
  patent, when those edges are already known locally.
- USPTO XML patent citations become normal `relation` rows after XML ingest.
  NPL references are preserved in `uspto_grant_citation` but are not shown as
  patent graph rows.

### 4.4 Telemetry around fetch

| Counter | When |
| --- | --- |
| `uspto.fetch_xml.count` | Every call (hit + miss). |
| `uspto.fetch_xml.cache_hit` | File found on disk; no network call. |
| `uspto.fetch_xml.cache_miss` | Downloaded over the network. |
| `uspto.fetch_xml.bytes` | Bytes written on a miss. |
| `uspto.auto_fetch_xml.attempt` / `.ok` / `.error` | Auto-fetch after `:add` / `:lookup`. |
| `uspto.xml.parse.count` / `.error` | XML parse pass. |
| `uspto.xml.save.error` | Database save pass. |
| `uspto.xml.ingest.{claims,citations,classifications,drawings,relations}` | Per-section row counts on a successful ingest. |
| `uspto.xml.ingest.citation_relations` | Patent citations normalized into the regular relation graph. |
| `uspto.xml.ingest.{abstract,description,claims}_bytes` | Body sizes. |
| Timings | `uspto.fetch_xml`, `uspto.xml.parse`, `uspto.xml.map`, `uspto.xml.save`, `uspto.xml.ingest`. |

The activity journal records `uspto.fetch_xml` and `uspto.xml.ingest` rows
with `cached`, per-section counts, and patent number.

---

## 5. Parsed Body & Full-Text Viewer

When the XML is ingested, body content lands in:

- `uspto_grant_body` — abstract text + XML, description text + XML, claim
  statement, claims text, claims JSON.
- `uspto_drawing` — figure-by-figure file/dimensions.
- `uspto_grant_citation` — patent + NPL references with category, cited
  document id, CPC text, national class.
- `uspto_grant_classification` — IPCR + CPC rows tagged by role
  (`main` / `further` / `search`).
- `uspto_grant_relation` — continuation, continuation-in-part, division,
  reissue, provisional, related-publication rows.

Patent citations from `uspto_grant_citation` are additionally normalized into
the regular `relation` table as `cites` edges. This is what drives the citation
pane, cited-by lookups, inline citation highlighting, and citation count
columns. Cited patent documents that are not already known are inserted as
stubs so the edge has a local endpoint and can be crawled later.

The single-row summary (`grant_doc_number`, `grant_kind`, `grant_date`,
`term_extension_days`, `number_of_claims`, `number_of_drawing_sheets`,
`number_of_figures`, `primary_examiner_*`, `attorney_org`, `attorney_type`,
`field_of_search_json`, …) lives on `uspto_application` so it travels with
the existing application row.

### 5.1 `:open.fulltext`

The full-text viewer (`T` / `:open.fulltext` in detail scope) now prefers
the USPTO-parsed body when present. On open it asks the daemon for the body
via `uspto.grant_body`; when one is on hand the pane renders that — claims
listed individually, then abstract + description. Only when no XML body
exists does the viewer fall back to the live Google fetch via
`crawl.FetchFullText`.

---

## 6. Quick Reference

```
# .env
PATENTMINE_CREDENTIALS_DIR=~/.ssh/patentmine
PATENTMINE_USPTO_API_KEY=file:${PATENTMINE_CREDENTIALS_DIR}/uspto_odp_key
PATENTMINE_SOURCE_MODE=uspto-first
```

```
# TUI commands
:source.mode uspto-first          # change policy
:source.mode                      # print current policy

:add 17730671                     # honors :source.mode
:add.uspto 17730671 17696256      # forces USPTO, batch ok
:add.google US12345678            # forces Google

:fetch.uspto.pgpub                # cursor / visual-selection patents
:fetch.uspto.grant                # cursor / visual-selection patents

:browse                           # open Source URL / provider URL in browser
:browse.uspto                     # open grant XML URL, fallback to publication XML URL
:browse.uspto.grant               # force USPTO grant XML URL
:browse.uspto.pgpub               # force USPTO publication XML URL
:browse.google                    # force Google Patents URL
:open.citations                   # view patents cited by current patent
:open.fulltext                    # prefers parsed XML body
```

Detail-pane Enter on a `PGPub URL` / `Grant URL` row also fetches and
ingests the matching XML.
