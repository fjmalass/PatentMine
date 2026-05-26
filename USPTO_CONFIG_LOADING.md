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

### 1.1 Direct value

```bash
export PATENTMINE_USPTO_API_KEY=ABCDEFGHIJ1234567890
```

### 1.2 `file:` indirection (recommended)

`file:` makes PatentMine read the key from a file on disk instead of carrying
it through process environment. `~/` is expanded to `$HOME`.

```bash
export PATENTMINE_USPTO_API_KEY=file:~/.ssh/uspto_odp_key
```

The file should contain only the key (whitespace trimmed). This is the
shape `.env` uses out of the box:

```dotenv
# .env
PATENTMINE_CREDENTIALS_DIR=~/.ssh
PATENTMINE_USPTO_API_KEY=file:${PATENTMINE_CREDENTIALS_DIR}/uspto_odp_key
```

### 1.3 Verifying

The daemon logs `service check uspto connected` when the startup probe
succeeds. From a running daemon you can also run:

```bash
patentmine check uspto
```

which prints the resolved status code, request/response bytes, and the
rate-limit headers returned by ODP. The TUI surfaces the same state in the
status bar shortly after launch.

### 1.4 Where the key is used

| Surface | Endpoint | Notes |
| --- | --- | --- |
| `:add.uspto` / `:lookup` | `api.uspto.gov/api/v1/patent/applications/search` | One request per patent. Subject to USPTO's 1 s minimum interval. |
| Auto / manual XML fetch | `api.uspto.gov/api/v1/datasets/products/files/...` | Bulk dataset. **No** minimum-interval rate limit on PatentMine's side — parallel downloads OK. |
| Browser open of USPTO Source URL | URL is opened with `?api_key=` appended so the browser can hit ODP directly. |

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

### 2.2 Telemetry

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
```

The auto-fetch listener is armed by every `startFamilyCrawl`, so `:lookup`
on a USPTO-resolvable patent ingests its grant XML on completion too.

### 3.3 Candidate picker

If the ODP search returns multiple matching applications for the requested
number, the daemon returns the list and the TUI opens the
`USPTOCandidatePicker` overlay (80% × 80%) so the user picks one. The
selected candidate's application number is then used for the actual add.

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

### 4.3 Telemetry around fetch

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
PATENTMINE_CREDENTIALS_DIR=~/.ssh
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

:open.fulltext                    # prefers parsed XML body
```

Detail-pane Enter on a `PGPub URL` / `Grant URL` row also fetches and
ingests the matching XML.
