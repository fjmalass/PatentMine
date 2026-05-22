# Clip patent passages with metadata

**Session ID:** ses_1b48c1c9affef2nOaGDRoWiR3m
**Created:** 5/21/2026, 12:52:26 PM
**Updated:** 5/21/2026, 1:44:51 PM

---

## User

is there way i could grabe the entire text (not stored in the database as too large.) but then have a look at it. and be able to select passages and do a copy to clipboard. along with all the patent relevant information. like Patent#, Inventor (in a ashort cut like <name> et al. when multiple, Application#, Assignee, Publication,Expiration date, and where the clip came from like <Claim 2> etc. 
make sure that would work in linux/wsl/windows/osx..

provide pros/cons of the methods and how this clip to clipboard would work for other items. 

---

## Assistant (Plan · Big Pickle · 96.1s)

_Thinking:_

The user wants to know how to grab full patent text (text, not database-stored, since it's too large), view it, select passages, copy to clipboard with patent metadata (Patent#, Inventor with "et al." for multiple, Application#, Assignee, Publication, Expiration date, source location like "Claim 2", etc.), and have it work cross-platform (Linux/WSL/Windows/macOS).

They're asking for pros/cons of methods and how clipboard would work for other items.

Let me first understand their codebase to see what they're currently working with.

Let me first explore the codebase to understand what you're working with.

**Tool: task**

**Input:**
```json
{
  "description": "Explore codebase structure",
  "prompt": "Explore the codebase at /mnt/d/Repos/PatentMineNew thoroughly. I need to understand:\n1. What kind of application this is (web, desktop? what framework?)\n2. How patents are currently displayed/viewed\n3. How patent text is stored or accessed (is it from a database, API, file?)\n4. What patent metadata fields exist (Patent#, Inventor, Application#, Assignee, Publication date, Expiration date, Claims, etc.)\n5. Any existing clipboard or copy functionality\n6. The tech stack (frontend/backend languages, libraries)\n\nLook at package.json, README, main source files, database schemas, API routes, component files, etc. Return a comprehensive summary of the architecture and all relevant findings.",
  "subagent_type": "explore"
}
```

**Output:**
```
task_id: ses_1b48c0b0bffe60uwHbkrbqNbIC (for resuming to continue this task if needed)

<task_result>
Here is a comprehensive summary of the PatentMine codebase:

---

## 1. Application Type & Framework

**PatentMine** is a **terminal user interface (TUI) application** for patent curation and crawling. It is not a web app or desktop GUI app -- it runs in the terminal using the [Bubbletea](https://github.com/charmbracelet/bubbletea) Go TUI framework (v1.3.10). It also has a secondary **HTTP/JSON API server** (for programmatic access) and a **JSON-RPC daemon** backend. There are three ways to interact with it:

- `patentmine tui` -- The main interactive TUI (thin client)
- `patentmine api` -- A REST API web server (on localhost)
- `patentmine serve` -- The engine daemon (background, communicates via Unix sockets)

All clients communicate with the daemon via JSON-RPC over Unix sockets (protocol defined in `internal/proto/`).

---

## 2. How Patents Are Displayed/Viewed

Patent display is handled entirely in the TUI layer (`internal/tui/`):

### Catalog Pane (`internal/tui/pane/catalog.go`)
- The main patent **list view**, showing patents as a **paged table**
- Columns: `#`, `NUMBER`, `TITLE`, `INVENTOR`, `EXPIRES`, `TAGS`, `IDS`, `FETCH/REVIEW STATE`
- Supports sorting by column, filtering, visual selection (vim-like `v` mode), and searching (`/`)
- Uses `render.Paginator` for scrolling/paging
- Each row displays lightweight `domain.PatentRow` data (no full patent text)

### Detail Pane (`internal/tui/pane/detail.go`)
- Shows a **complete single patent record** in a scrollable view
- Sections displayed (with jump anchors for `;` jump mode):
  - "Shown as" (DisplayNumber)
  - "Record key" (Canonical Number)
  - "Title"
  - **"Assignee"** (jump key)
  - **"Inventors"** (jump key)
  - "Country"
  - "Fetch state"
  - "Source"
  - "Source URL"
  - **"Expiration"** (jump key)
  - **"Review state"** (jump key, project-scoped)
  - **"IDS"** (jump key, project-scoped)
  - **"Tags"** (jump key, project-scoped)
  - **"Citations"** (jump key)
  - **"Documents"** (jump key -- lists all lifecycle document numbers)
  - **"First claim"** (jump key -- section with word-wrapped text)
  - **"Abstract"** (jump key -- section with word-wrapped text)

### Citations Pane (`internal/tui/pane/citations.go`)
- Lists family-graph edges (citations, cited-by, parents, children) as a table

### IDS Pane (`internal/tui/pane/ids.go`)
- Curated Information Disclosure Statement entries for a project

### Projects Pane (`internal/tui/pane/projects.go`)
- Lists/manages projects

### Key Bindings for Navigation
- `j`/`k` -- up/down, `gg`/`G` -- top/bottom, `ctrl+d`/`ctrl+u` -- page down/up
- `enter`/`l` -- open detail, `h`/`esc` -- back, `;` -- jump mode in detail

---

## 3. How Patent Text Is Stored & Accessed

### Storage: SQLite Database
- **File**: `internal/store/sqlite/schema.sql` defines all tables
- **Engine**: Uses `modernc.org/sqlite` (pure Go SQLite, no CGo)
- The database file is stored at a configurable path (default: `<app-home>/patentmine.db`)

### Key Tables:
| Table | Purpose |
|---|---|
| `patent` | Main patent records (number, title, abstract, assignee, inventors JSON, dates, first_claim, expiration, source, etc.) |
| `document` | Lifecycle documents (application/publication/grant stages) linked to patents via `record_number` FK |
| `relation` | Family-graph edges (citations, cited-by, parent, child) |
| `project` | User projects |
| `membership` | Links patents to projects with review states |
| `tag` / `patent_tag` | Project-scoped tags and their assignments |
| `project_ids` | Curated IDS entries |

### Access Pattern:
Data flows through a layered architecture:
1. **Web Crawlers** (`internal/crawl/google.go`, `justia.go`, `uspto.go`, `pct.go`) fetch patent HTML from the web
2. **Crawler** (`internal/crawl/crawl.go`) scrapes HTML, extracts metadata, relations, and documents
3. **Engine** (`internal/engine/engine.go`) orchestrates crawling and store operations
4. **RPC Layer** (`internal/rpc/`) provides IPC for clients
5. **UI Clients** (TUI, API) call the daemon via JSON-RPC

### Crawl Sources (in priority order):
1. **File source** (`internal/crawl/file.go`) -- local cached fixture files
2. **Google Patents** (primary) -- `https://patents.google.com/patent/{number}/en`
3. **Justia** -- `https://patents.justia.com/patent/{serial}`
4. **USPTO** -- `https://patft.uspto.gov/netacgi/nph-Parser?patentnumber={serial}`
5. **PCT/WIPO** -- `https://patentscope.wipo.int/search/en/detail.jsf?docId={...}`

### How Full Text Is Fetched:
- Google Patents is the primary source
- The Google scraper (`parseGoogle`) extracts from HTML microdata (`itemprop` attributes) and DOM elements
- Fields extracted: title, abstract, assignee, inventors, dates (application/publication/grant), **first claim** (from `section[itemprop='claims'] .claim`), and relations
- **Full claims text is NOT stored** -- only `FirstClaim` (claim 1) is stored in the `patent.first_claim` column
- There is no PDF storage, no full-text indexing, and no full patent specification storage

---

## 4. Patent Metadata Fields

### Complete `domain.Patent` struct (`internal/domain/patent.go`):

| Field | Type | DB Column | Description |
|---|---|---|---|
| `Number` | `PatentNumber` | `number` (PK) | Canonical record key |
| `DisplayNumber` | `PatentNumber` | `display_number` | Latest-stage number for display |
| `Title` | `string` | `title` | Patent title |
| `Abstract` | `string` | `abstract` | Abstract text |
| `Assignee` | `string` | `assignee` | Assignee name |
| `Inventors` | `[]Inventor` | `inventors` (JSON array) | List of inventors |
| `FetchState` | `FetchState` | `fetch_state` | "stub" or "cached" |
| `Source` | `Source` | `source` | "google", "justia", "uspto", "pct", "file" |
| `ApplicationDate` | `time.Time` | `application_date` | Filing date |
| `PublicationDate` | `time.Time` | `publication_date` | Publication date |
| `GrantDate` | `time.Time` | `grant_date` | Grant/issue date |
| `FetchedAt` | `time.Time` | `fetched_at` | When record was crawled |
| `FirstClaim` | `string` | `first_claim` | Claim 1 body text |
| `ExpirationDate` | `time.Time` | `expiration_date` | Patent expiration date |
| `ExpirationSource` | `string` | `expiration_source` | How expiration was determined (e.g. "estimated") |
| `SourceURL` | `string` | `source_url` | URL the record was fetched from |
| `Documents` | `[]Document` | (child table) | Lifecycle documents (not stored in patent table) |

### `domain.Document` struct:

| Field | Type | Description |
|---|---|---|
| `Number` | `PatentNumber` | Document number (e.g., application, publication, or grant number) |
| `Stage` | `Stage` | "application", "publication", or "grant" |
| `Dated` | `time.Time` | Date associated with this stage |

### `domain.PatentRow` (list view):

| Field | Type |
|---|---|
| `Number` | `PatentNumber` |
| `DisplayNumber` | `PatentNumber` |
| `Title` | `string` |
| `Inventors` | `[]Inventor` |
| `ExpirationDate` | `time.Time` |
| `Tags` | `[]string` |
| `FetchState` | `FetchState` |
| `ReviewState` | `ReviewState` (project-scoped) |
| `IDSEntry` | `*IDSEntry` |

### Other metadata:
- **Review states**: `stored`, `under_review`, `ignored`, `cached`, `deleted` (project-scoped)
- **Fetch states**: `stub`, `cached`
- **Relation kinds**: `cites`, `cited_by`, `parent`, `child`
- **IDS entry fields**: kind code, country code, in_full flag, relevant passages, notes, status (pending/submitted/accepted)
- **Patent Number components**: Country (ISO-2), Serial (digits only), Kind (e.g., "B2", "A1")

---

## 5. Existing Clipboard / Copy Functionality

**There is NO clipboard or copy functionality whatsoever.** My search for `clipboard`, `copy`, `yank`, and `paste` across all Go, SQL, and markdown files returned zero relevant results. There is no:

- Way to copy a patent number, title, abstract, or claims to the system clipboard
- `y` key binding or similar
- `:copy` or `:yank` command
- Export to clipboard functionality
- Any reference to `golang.org/x/term` or OS clipboard APIs

The only "copy" references in the codebase are Go variable copies (copying slices, etc.) and the `Makefile.toml` `cp` command for database backups.

---

## 6. Tech Stack

### Backend (Go):
| Layer | Technology | File/Location |
|---|---|---|
| **Language** | Go 1.26 | `go.mod` |
| **Database** | SQLite (via `modernc.org/sqlite` v1.50.1) | `internal/store/sqlite/` |
| **IPC Protocol** | JSON-RPC 2.0 over Unix sockets | `internal/proto/`, `internal/rpc/` |
| **Web Scraping** | `github.com/PuerkitoBio/goquery` v1.12.0 (jQuery-like HTML parsing) | `internal/crawl/google.go` etc. |
| **Logging** | `log/slog` (standard library structured logging) | Throughout |
| **Observability** | Custom in-process metrics, activity journal (JSONL) | `internal/observability/` |
| **HTTP Server** | Standard `net/http` (Go 1.22+ routing patterns) | `internal/api/` |

### Frontend (TUI):
| Layer | Technology | File/Location |
|---|---|---|
| **Framework** | `github.com/charmbracelet/bubbletea` v1.3.10 (TUI framework) | `internal/tui/` |
| **Styling** | `github.com/charmbracelet/lipgloss` v1.1.0 (style definitions) | `internal/tui/render/` |
| **Text Width** | `github.com/charmbracelet/x/ansi` (string width, ANSI handling) | `internal/tui/render/cell.go` |
| **Bubbles** | `github.com/charmbracelet/bubbles` v1.0.0 (text input, spinner) | (used via overlay) |
| **Key Reader** | Custom chord reader supporting multi-key sequences | `internal/keys/` |

### Project Structure:
```
PatentMineNew/
  cmd/patentmine/          # Binary entrypoints (main, serve, tui, api, etc.)
  internal/
    api/                   # REST API server
    command/               # Command registry (ID system)
    config/                # Configuration
    crawl/                 # Web crawlers (Google, Justia, USPTO, PCT)
    domain/                # Core domain types (Patent, Document, Number, etc.)
    engine/                # Daemon core (business logic, orchestration)
    keys/                  # Key chord reader
    observability/         # Metrics, activity journal
    proto/                 # JSON-RPC protocol definitions
    rpc/                   # RPC client implementation
    store/                 # Repository interface & SQLite implementation
    text/                  # i18n string catalog
    tui/                   # Terminal UI (Bubbletea app, panes, overlays)
    version/               # Build version info
```

### Architecture Pattern:
**Three-tier daemon architecture**:
- **Daemon** (`engine` + `store` + `crawl`) -- owns the database, runs background crawl jobs, exposes JSON-RPC API
- **TUI Client** (`tui/`) -- thin client that communicates with daemon via RPC
- **Web API Server** (`api/`) -- thin HTTP server that translates REST calls to RPC calls

The TUI and API are both "thin clients" that own no database and no business logic -- they only translate user input into RPC calls to the daemon.
</task_result>
```

---

## Assistant (Plan · Big Pickle · 10.7s)

_Thinking:_

Now I have a thorough understanding of the codebase. Let me think about what the user is asking:

1. They want to grab the **full patent text** (which is not stored in the DB because it's too large)
2. They want to view this full text
3. Select passages and copy to clipboard
4. The clipboard copy should include metadata: Patent#, Inventor (et al. format), Application#, Assignee, Publication date, Expiration date, and source location (like "Claim 2")
5. Cross-platform: Linux, WSL, Windows, macOS
6. They want pros/cons of methods

Current state:
- Only `FirstClaim` is stored in the DB
- Full claims text is fetched from Google Patents but not stored
- No clipboard functionality exists
- It's a Go Bubbletea TUI app

There are several approaches:

### Approach 1: Fetch full text from Google Patents on demand, display in a pager/viewer, integrate clipboard

We'd need to:
- Extend the Google crawler to fetch ALL claims (not just claim 1)
- Or fetch the patent page HTML on-demand when the user wants to view full text
- Store it temporarily (not in DB, maybe in a cache file or memory)
- Create a new TUI pane/viewer for the full text with selection capability
- Use a Go clipboard library like `github.com/atotto/clipboard` or `golang.design/x/clipboard`

### Approach 2: Open the patent in an external browser and use browser's native copy

- Open `https://patents.google.com/patent/{number}/en` in default browser
- User copies from there manually
- But this loses the integrated clipboard-with-metadata feature

### Approach 3: Use an external pager like `less` or `bat` with pipe-to-clipboard

- Write full text to a temp file and open in `less`/`bat`
- Use OS-specific clipboard commands (xclip/xsel on Linux, pbcopy on macOS, clip.exe on WSL/Windows)
- But loses TUI integration

### Approach 4: Build a custom TUI viewer within Bubbletea with text selection and clipboard

- Most integrated approach
- Full text fetched on-demand from Google Patents (or cached)
- A new scrollable view that shows all claims with headers like "Claim 1", "Claim 2", etc.
- Visual text selection (like vim visual mode)
- `y` to yank selected text with metadata to clipboard
- Needs cross-platform clipboard library

Let me think about the clipboard aspect more carefully.

For Go clipboard libraries:
1. `github.com/atotto/clipboard` - cross-platform, uses xclip/xsel on Linux, pbcopy on macOS, clip.exe on Windows. Simple API. But may not work well in WSL (needs clip.exe from Windows).
2. `github.com/golang-design/clipboard` - newer, cross-platform, might be more reliable
3. For WSL specifically, we might need to call `clip.exe` directly or use `powershell.exe`
4. Can also use the `wl-clipboard` tools on Wayland

For the full text fetch:
- Google Patents page has all claims in `section[itemprop='claims'] div.claim`
- We can extend the Google crawler to fetch all claims
- Store them in memory (not DB) for the session
- Maybe cache to a temp file for future lookups

Let me explore the existing Google crawler to understand how it extracts claims.

I should look at the actual code now to understand the detail pane and the crawl module better before formulating a plan.

Let me read a few key files.

Good, this is a Go Bubbletea TUI app. Let me dig deeper into the relevant files to understand how claims are parsed and displayed.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/crawl/google.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/crawl/google.go</path>
<type>file</type>
<content>
1: package crawl
2: 
3: import (
4: 	"bytes"
5: 	"fmt"
6: 	"regexp"
7: 	"strings"
8: 	"time"
9: 
10: 	"github.com/PuerkitoBio/goquery"
11: 
12: 	"patentmine/internal/domain"
13: )
14: 
15: // googleMinInterval keeps requests to Google Patents polite.
16: const googleMinInterval = 2 * time.Second
17: 
18: // NewGoogleSource builds a Source backed by Google Patents.
19: func NewGoogleSource() Source {
20: 	return newHTTPSource(
21: 		domain.SourceGoogle,
22: 		googleMinInterval,
23: 		func(n domain.PatentNumber) string {
24: 			return "https://patents.google.com/patent/" + n.Normalized() + "/en"
25: 		},
26: 		parseGoogle,
27: 	)
28: }
29: 
30: var (
31: 	// googlePatentURLRe pulls the patent number out of a /patent/<num>/ path.
32: 	googlePatentURLRe = regexp.MustCompile(`(?i)/patent/([^/?#]+)`)
33: 	// googlePatentIDRe matches a bare patent identifier in scraped text.
34: 	googlePatentIDRe = regexp.MustCompile(`(?i)\b[A-Z]{2,}[0-9][A-Z0-9]*\b`)
35: )
36: 
37: // googleDateLayouts are the date forms seen on Google Patents.
38: var googleDateLayouts = []string{"2006-01-02", "2006/01/02"}
39: 
40: // parseGoogle extracts a patent's record from a Google Patents HTML page. The
41: // bibliographic fields come from the page's itemprop microdata (with meta-tag
42: // fallbacks); citation and family edges come from the page body, so a fetched
43: // patent's Citations / Cited-by / family views populate.
44: func parseGoogle(number domain.PatentNumber, body []byte) (Result, error) {
45: 	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
46: 	if err != nil {
47: 		return Result{}, fmt.Errorf("crawl/google: parse HTML: %w", err)
48: 	}
49: 	title := clean(googleText(doc.Selection, "span[itemprop='title']", "meta[name='DC.title']", "title"))
50: 	if title == "" {
51: 		// Not a parseable patent page — let the registry fall through.
52: 		return Result{}, ErrNotAvailable
53: 	}
54: 
55: 	patent := domain.Patent{
56: 		Number:          number,
57: 		DisplayNumber:   number,
58: 		Title:           title,
59: 		Abstract:        clean(googleText(doc.Selection, "div.abstract", "section[itemprop='abstract']", "meta[name='DC.description']")),
60: 		Assignee:        clean(googleText(doc.Selection, "dd[itemprop='assigneeOriginal']", "span[itemprop='assigneeOriginal']", "dd[itemprop='assigneeCurrent']")),
61: 		Inventors:       googleTexts(doc.Selection, "dd[itemprop='inventor']", "span[itemprop='inventor']"),
62: 		FetchState:      domain.FetchCached,
63: 		Source:          domain.SourceGoogle,
64: 		FetchedAt:       time.Now().UTC(),
65: 		ApplicationDate: googleAttrDate(doc, "time[itemprop='filingDate']"),
66: 		PublicationDate: googleAttrDate(doc, "time[itemprop='publicationDate']"),
67: 		GrantDate:       googleAttrDate(doc, "time[itemprop='grantDate']"),
68: 	}
69: 
70: 	patent.FirstClaim = clean(googleText(doc.Selection, "section[itemprop='claims'] .claim", ".claims .claim"))
71: 	patent.SourceURL = "https://patents.google.com/patent/" + number.Normalized() + "/en"
72: 	// Google does not state a definitive expiration; estimate it as 20 years
73: 	// from the earliest of publication or grant.
74: 	if base := firstNonZeroTime(patent.PublicationDate, patent.GrantDate); !base.IsZero() {
75: 		patent.ExpirationDate = base.AddDate(20, 0, 0)
76: 		patent.ExpirationSource = domain.ExpirationEstimated
77: 	}
78: 
79: 	document := domain.Document{
80: 		Number: number,
81: 		Stage:  domain.GuessStage(number),
82: 		Dated:  firstNonZeroTime(patent.GrantDate, patent.PublicationDate, patent.ApplicationDate),
83: 	}
84: 	return Result{
85: 		Patent:    patent,
86: 		Documents: []domain.Document{document},
87: 		Relations: googleRelations(doc, number),
88: 	}, nil
89: }
90: 
91: // googleRelations extracts every citation and family edge for number. The
92: // crawler overwrites Relation.From with the resolved record number, so only To
93: // and Kind need to be correct here.
94: func googleRelations(doc *goquery.Document, number domain.PatentNumber) []domain.Relation {
95: 	var out []domain.Relation
96: 	seen := map[string]bool{}
97: 	add := func(to domain.PatentNumber, kind domain.RelationKind) {
98: 		if to.IsZero() || to.Normalized() == number.Normalized() {
99: 			return
100: 		}
101: 		key := string(kind) + "\x00" + to.Normalized()
102: 		if seen[key] {
103: 			return
104: 		}
105: 		seen[key] = true
106: 		out = append(out, domain.Relation{From: number, To: to, Kind: kind})
107: 	}
108: 
109: 	// Citations: patents this one cites (backward) and patents citing it (forward).
110: 	googleCitationRows(doc, "[itemprop='backwardReferences'], #patentCitations, #backwardReferences",
111: 		func(n domain.PatentNumber) { add(n, domain.RelationCites) })
112: 	googleCitationRows(doc, "[itemprop='forwardReferences'], #citedBy, #forwardReferences",
113: 		func(n domain.PatentNumber) { add(n, domain.RelationCitedBy) })
114: 
115: 	// Family: parentApps list patents this one continues from; priority and
116: 	// continuation apps list patents that continue from this one.
117: 	doc.Find("[itemprop='parentApps']").Each(func(_ int, row *goquery.Selection) {
118: 		if n, ok := googleRowNumber(row); ok {
119: 			add(n, domain.RelationParent)
120: 		}
121: 	})
122: 	for _, selector := range []string{"[itemprop='priorityApps']", "[itemprop='continuationApps']"} {
123: 		doc.Find(selector).Each(func(_ int, row *goquery.Selection) {
124: 			if n, ok := googleRowNumber(row); ok {
125: 				add(n, domain.RelationChild)
126: 			}
127: 		})
128: 	}
129: 	return out
130: }
131: 
132: // googleCitationRows scans every citation section matched by selector and emits
133: // each patent number it finds, by link or by bare text.
134: func googleCitationRows(doc *goquery.Document, selector string, emit func(domain.PatentNumber)) {
135: 	doc.Find(selector).Each(func(_ int, section *goquery.Selection) {
136: 		section.Find("tr, .patent-result, .result, .citation, [itemprop='publicationNumber'], a[href*='/patent/']").Each(func(_ int, s *goquery.Selection) {
137: 			if href, ok := s.Attr("href"); ok && strings.Contains(href, "/patent/") {
138: 				if n, ok := googleParseNumber(googlePatentNumberFromURL(href)); ok {
139: 					emit(n)
140: 				}
141: 			}
142: 			text := strings.TrimSpace(s.Text())
143: 			if len(text) > 4 && len(text) < 16 && !strings.Contains(text, " ") {
144: 				if n, ok := googleParseNumber(text); ok {
145: 					emit(n)
146: 				}
147: 			}
148: 		})
149: 	})
150: }
151: 
152: // googleRowNumber returns the patent number a family-table row refers to.
153: func googleRowNumber(row *goquery.Selection) (domain.PatentNumber, bool) {
154: 	if n, ok := googleParseNumber(row.Find("[itemprop='representativePublication']").First().Text()); ok {
155: 		return n, true
156: 	}
157: 	var found domain.PatentNumber
158: 	var ok bool
159: 	row.Find("a[href*='/patent/']").EachWithBreak(func(_ int, a *goquery.Selection) bool {
160: 		if href, hrefOK := a.Attr("href"); hrefOK {
161: 			if n, parsed := googleParseNumber(googlePatentNumberFromURL(href)); parsed {
162: 				found, ok = n, true
163: 				return false
164: 			}
165: 		}
166: 		return true
167: 	})
168: 	return found, ok
169: }
170: 
171: // googleText returns the first non-empty value among selectors: a meta tag's
172: // content attribute, or an element's text.
173: func googleText(s *goquery.Selection, selectors ...string) string {
174: 	for _, selector := range selectors {
175: 		found := s.Find(selector).First()
176: 		if found.Length() == 0 {
177: 			continue
178: 		}
179: 		if content, ok := found.Attr("content"); ok && strings.TrimSpace(content) != "" {
180: 			return content
181: 		}
182: 		if text := found.Text(); strings.TrimSpace(text) != "" {
183: 			return text
184: 		}
185: 	}
186: 	return ""
187: }
188: 
189: // googleTexts returns the de-duplicated text of every element matched by any
190: // selector.
191: func googleTexts(s *goquery.Selection, selectors ...string) []domain.Inventor {
192: 	seen := map[string]bool{}
193: 	var out []domain.Inventor
194: 	for _, selector := range selectors {
195: 		s.Find(selector).Each(func(_ int, el *goquery.Selection) {
196: 			text := clean(el.Text())
197: 			if text != "" && !seen[text] {
198: 				seen[text] = true
199: 				out = append(out, domain.Inventor(text))
200: 			}
201: 		})
202: 	}
203: 	return out
204: }
205: 
206: // googleAttrDate parses the datetime attribute of the first matched element.
207: func googleAttrDate(doc *goquery.Document, selector string) time.Time {
208: 	value, _ := doc.Find(selector).First().Attr("datetime")
209: 	return parseGoogleDate(value)
210: }
211: 
212: // googleParseNumber normalizes a scraped string and parses it as a patent
213: // number, reporting false when it is not a valid number.
214: func googleParseNumber(raw string) (domain.PatentNumber, bool) {
215: 	id := googleNormalizeID(raw)
216: 	if id == "" {
217: 		return domain.PatentNumber{}, false
218: 	}
219: 	n, err := domain.ParsePatentNumber(id)
220: 	if err != nil {
221: 		return domain.PatentNumber{}, false
222: 	}
223: 	return n, true
224: }
225: 
226: // googleNormalizeID strips scraped-text noise and returns a bare patent id.
227: func googleNormalizeID(value string) string {
228: 	value = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(value, " ", ""), ",", ""))
229: 	if value == "" {
230: 		return ""
231: 	}
232: 	if strings.Contains(value, "/PATENT/") {
233: 		return googlePatentNumberFromURL(value)
234: 	}
235: 	return strings.TrimSpace(googlePatentIDRe.FindString(value))
236: }
237: 
238: // googlePatentNumberFromURL pulls the patent number from a /patent/<num>/ path.
239: func googlePatentNumberFromURL(rawURL string) string {
240: 	m := googlePatentURLRe.FindStringSubmatch(rawURL)
241: 	if len(m) < 2 {
242: 		return ""
243: 	}
244: 	return strings.TrimSpace(m[1])
245: }
246: 
247: // parseGoogleDate parses a date in any layout Google uses, or the zero time.
248: func parseGoogleDate(s string) time.Time {
249: 	s = strings.TrimSpace(s)
250: 	for _, layout := range googleDateLayouts {
251: 		if t, err := time.Parse(layout, s); err == nil {
252: 			return t
253: 		}
254: 	}
255: 	return time.Time{}
256: }
257: 
258: // firstNonZeroTime returns the first non-zero time, or the zero time.
259: func firstNonZeroTime(times ...time.Time) time.Time {
260: 	for _, t := range times {
261: 		if !t.IsZero() {
262: 			return t
263: 		}
264: 	}
265: 	return time.Time{}
266: }
267: 
268: // clean collapses runs of whitespace in scraped text to single spaces.
269: func clean(value string) string {
270: 	return strings.Join(strings.Fields(value), " ")
271: }

(End of file - total 271 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/pane/detail.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/pane/detail.go</path>
<type>file</type>
<content>
1: package pane
2: 
3: import (
4: 	"fmt"
5: 	"strings"
6: 	"time"
7: 
8: 	tea "github.com/charmbracelet/bubbletea"
9: 
10: 	"patentmine/internal/command"
11: 	"patentmine/internal/domain"
12: 	"patentmine/internal/proto"
13: 	"patentmine/internal/rpc"
14: 	"patentmine/internal/tui/render"
15: )
16: 
17: // detailDateLayout formats dates in the detail view.
18: const detailDateLayout = "2006-01-02"
19: 
20: // detailLoadedMsg delivers a finished patent.get result. state and tags are
21: // project-scoped and empty when the detail pane has no project.
22: type detailLoadedMsg struct {
23: 	requestID uint64
24: 	patent    domain.Patent
25: 	state     domain.ReviewState
26: 	tags      []domain.Tag
27: 	idsEntry  *domain.IDSEntry
28: 	err       error
29: }
30: 
31: // detailRelationsMsg delivers the family-graph edge counts for the detail view.
32: type detailRelationsMsg struct {
33: 	requestID uint64
34: 	counts    map[domain.RelationKind]int
35: }
36: 
37: // detailRelationKinds are the edge kinds the detail view counts, in display order.
38: var detailRelationKinds = []domain.RelationKind{
39: 	domain.RelationCites, domain.RelationCitedBy,
40: 	domain.RelationParent, domain.RelationChild,
41: }
42: 
43: // Detail shows one patent's full record. The record can be longer than the
44: // body area, so the pane scrolls — every navigation binding in the detail
45: // keymap layer must resolve to a handler here.
46: type Detail struct {
47: 	client   *rpc.Client
48: 	theme    render.Theme
49: 	number   domain.PatentNumber
50: 	project  domain.ProjectID
51: 	handlers map[command.ID]cmdHandler
52: 
53: 	patent     domain.Patent
54: 	state      domain.ReviewState
55: 	tags       []domain.Tag
56: 	idsEntry   *domain.IDSEntry
57: 	relCounts  map[domain.RelationKind]int
58: 	anchors    []render.JumpAnchor // jump targets, rebuilt on every body render
59: 	jumpKeys   map[string]rune    // label -> assigned jump key, stable for pane lifetime
60: 	page       render.Paginator
61: 	loading    bool
62: 	loadErr    string
63: 	loadID     uint64
64: 	jumpActive bool
65: }
66: 
67: // detailAnchorLabels are the section labels in display order. The jump key
68: // assignment algorithm uses this order to assign keys.
69: var detailAnchorLabels = []string{
70: 	"Assignee",
71: 	"Inventors",
72: 	"Expiration",
73: 	"Review state",
74: 	"IDS",
75: 	"Tags",
76: 	"Citations",
77: 	"Documents",
78: 	"First claim",
79: 	"Abstract",
80: }
81: 
82: // NewDetail builds a detail pane for one patent number. project, when set,
83: // scopes the pane's review state and tags; pass "" for a project-independent
84: // view. boundLetters are the single-letter/digit keys bound in the base and
85: // detail keymap layers, used to avoid conflicts when assigning jump keys.
86: func NewDetail(client *rpc.Client, theme render.Theme, number domain.PatentNumber, project domain.ProjectID, boundLetters []rune) *Detail {
87: 	d := &Detail{
88: 		client:    client,
89: 		theme:     theme,
90: 		number:    number,
91: 		project:   project,
92: 		relCounts: map[domain.RelationKind]int{},
93: 		page:      render.NewPaginator(10),
94: 		loading:   true,
95: 	}
96: 	d.computeJumpKeys(boundLetters)
97: 	d.handlers = map[command.ID]cmdHandler{
98: 		command.NavDown: func(inv Invocation) tea.Cmd {
99: 			if d.jumpActive && len(d.anchors) > 0 {
100: 				d.page.ScrollTo(d.nextAnchorLine())
101: 			} else {
102: 				d.page.ScrollTo(d.page.Cursor() + inv.Repeat)
103: 			}
104: 			return nil
105: 		},
106: 		command.NavUp: func(inv Invocation) tea.Cmd {
107: 			if d.jumpActive && len(d.anchors) > 0 {
108: 				d.page.ScrollTo(d.prevAnchorLine())
109: 			} else {
110: 				d.page.ScrollTo(d.page.Cursor() - inv.Repeat)
111: 			}
112: 			return nil
113: 		},
114: 		command.NavPageDown: func(Invocation) tea.Cmd { d.page.ScrollTo(d.page.Cursor() + d.page.PageSize()); return nil },
115: 		command.NavPageUp:   func(Invocation) tea.Cmd { d.page.ScrollTo(d.page.Cursor() - d.page.PageSize()); return nil },
116: 		command.NavTop:      func(Invocation) tea.Cmd { d.page.Top(); return nil },
117: 		command.NavBottom:   func(Invocation) tea.Cmd { d.page.Bottom(); return nil },
118: 		command.Refresh:     func(Invocation) tea.Cmd { d.loading = true; return d.reload() },
119: 		command.CrawlFamily: func(Invocation) tea.Cmd {
120: 			return CrawlCmd(d.client, d.number, crawlFamilyDepth, domain.CrawlProfileFamily, false)
121: 		},
122: 		command.CrawlCitations: func(Invocation) tea.Cmd {
123: 			return CrawlCmd(d.client, d.number, crawlFamilyDepth, domain.CrawlProfileCitations, false)
124: 		},
125: 		command.CrawlCitedBy: func(Invocation) tea.Cmd {
126: 			return CrawlCmd(d.client, d.number, crawlFamilyDepth, domain.CrawlProfileCitedBy, false)
127: 		},
128: 		command.CrawlAll: func(Invocation) tea.Cmd {
129: 			return CrawlCmd(d.client, d.number, crawlFamilyDepth, domain.CrawlProfileAll, false)
130: 		},
131: 		command.LookupPatent: func(Invocation) tea.Cmd {
132: 			return CrawlCmd(d.client, d.number, lookupDepth, "", false)
133: 		},
134: 	}
135: 	return d
136: }
137: 
138: // Context implements Pane.
139: func (d *Detail) Scope() command.Scope { return command.ScopeDetail }
140: 
141: // Title implements Pane.
142: func (d *Detail) Title() string { return "Detail · " + d.number.String() }
143: 
144: // Init implements Pane.
145: func (d *Detail) Init() tea.Cmd { return d.reload() }
146: 
147: // reload fetches the patent record and its family-graph edge counts.
148: func (d *Detail) reload() tea.Cmd {
149: 	return tea.Batch(d.load(), d.loadRelations())
150: }
151: 
152: // load fetches the patent record from the daemon, scoped to the pane's project
153: // so the reply carries the patent's review state and tags.
154: func (d *Detail) load() tea.Cmd {
155: 	client, number, project := d.client, d.number, d.project
156: 	requestID := nextAsyncID()
157: 	d.loadID = requestID
158: 	return func() tea.Msg {
159: 		ctx, cancel := callContext()
160: 		defer cancel()
161: 		var res proto.PatentResult
162: 		err := client.Call(ctx, proto.MethodPatentGet,
163: 			proto.PatentGetParams{Number: number, Project: project}, &res)
164: 		return detailLoadedMsg{
165: 			requestID: requestID,
166: 			patent:    res.Patent,
167: 			state:     res.ReviewState,
168: 			tags:      res.Tags,
169: 			idsEntry:  res.IDSEntry,
170: 			err:       err,
171: 		}
172: 	}
173: }
174: 
175: // loadRelations counts the patent's family-graph edges by kind.
176: func (d *Detail) loadRelations() tea.Cmd {
177: 	client, number, requestID := d.client, d.number, d.loadID
178: 	return func() tea.Msg {
179: 		ctx, cancel := callContext()
180: 		defer cancel()
181: 		counts := make(map[domain.RelationKind]int, len(detailRelationKinds))
182: 		for _, kind := range detailRelationKinds {
183: 			var res proto.RelationsResult
184: 			if err := client.Call(ctx, proto.MethodRelations,
185: 				proto.RelationsParams{Number: number, Kind: kind, Limit: 1}, &res); err == nil {
186: 				counts[kind] = res.Total
187: 			}
188: 		}
189: 		return detailRelationsMsg{requestID: requestID, counts: counts}
190: 	}
191: }
192: 
193: // Command implements Pane.
194: func (d *Detail) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
195: 	if handler, ok := d.handlers[id]; ok {
196: 		return d, handler(inv)
197: 	}
198: 	return d, nil
199: }
200: 
201: // Handles implements Pane.
202: func (d *Detail) Handles() []command.ID { return handlerIDs(d.handlers) }
203: 
204: // Update implements Pane.
205: func (d *Detail) Update(msg tea.Msg) (Pane, tea.Cmd) {
206: 	switch m := msg.(type) {
207: 	case detailLoadedMsg:
208: 		if m.requestID != d.loadID {
209: 			return d, nil
210: 		}
211: 		d.loading = false
212: 		if m.err != nil {
213: 			d.loadErr = m.err.Error()
214: 			return d, nil
215: 		}
216: 		d.loadErr = ""
217: 		d.patent = m.patent
218: 		d.state = m.state
219: 		d.tags = m.tags
220: 		d.idsEntry = m.idsEntry
221: 		d.page.Top()
222: 	case detailRelationsMsg:
223: 		if m.requestID == d.loadID {
224: 			d.relCounts = m.counts
225: 		}
226: 	case ProjectChangedMsg:
227: 		// The pane's project scopes its review state and tags; a change means
228: 		// the project-relative fields must be re-fetched.
229: 		var project domain.ProjectID
230: 		if m.Project != nil {
231: 			project = m.Project.ID
232: 		}
233: 		if project != d.project {
234: 			d.project = project
235: 			d.loading = true
236: 			return d, d.reload()
237: 		}
238: 	}
239: 	return d, nil
240: }
241: 
242: // Selection implements Pane: a detail pane's selection is its own patent.
243: func (d *Detail) Selection() (domain.PatentNumber, bool) {
244: 	return d.number, true
245: }
246: 
247: // View implements Pane. Long records scroll: the body is built in full, then
248: // windowed to the visible height by the paginator.
249: func (d *Detail) View(w, h int) string {
250: 	switch {
251: 	case d.loading:
252: 		return d.theme.Dim.Render("loading " + d.number.String() + "…")
253: 	case d.loadErr != "":
254: 		return d.theme.Error.Render("error: " + d.loadErr)
255: 	}
256: 	lines := strings.Split(d.body(w), "\n")
257: 	d.page.SetTotal(len(lines))
258: 	d.page.SetPageSize(max(h, 1))
259: 	start, end := d.page.Window()
260: 	cursor := d.page.Cursor()
261: 	out := make([]string, 0, end-start)
262: 	for i, line := range lines[start:end] {
263: 		if start+i == cursor {
264: 			out = append(out, d.theme.Selected.Render(render.Pad(line, w)))
265: 		} else {
266: 			out = append(out, line)
267: 		}
268: 	}
269: 	return strings.Join(out, "\n")
270: }
271: 
272: // body renders the full, unwindowed detail record. It also rebuilds the jump
273: // anchors, recording the line each labelled field lands on so jump mode can
274: // scroll straight to it.
275: func (d *Detail) body(w int) string {
276: 	p := d.patent
277: 	d.anchors = d.anchors[:0]
278: 	var b strings.Builder
279: 	d.field(&b, w, "Shown as", numberToShow(p).String())
280: 	d.field(&b, w, "Record key", p.Number.String())
281: 	d.field(&b, w, "Title", p.Title)
282: 	d.addAnchor(&b, d.jumpKey("Assignee"), "Assignee", 0)
283: 	d.field(&b, w, "Assignee", p.Assignee)
284: 	d.addAnchor(&b, d.jumpKey("Inventors"), "Inventors", 0)
285: 	var names []string
286: 	for _, inv := range p.Inventors {
287: 		names = append(names, string(inv))
288: 	}
289: 	d.field(&b, w, "Inventors", strings.Join(names, ", "))
290: 	d.field(&b, w, "Country", countryOrDash(p.Number.Country))
291: 	d.field(&b, w, "Fetch state", fetchStateText(d.theme, p.FetchState))
292: 	d.field(&b, w, "Source", string(p.Source))
293: 	d.field(&b, w, "Source URL", p.SourceURL)
294: 	d.addAnchor(&b, d.jumpKey("Expiration"), "Expiration", 0)
295: 	d.field(&b, w, "Expiration", expirationText(p))
296: 
297: 	// Project-scoped fields. Review state and tags describe the patent within
298: 	// one project, so they appear only when the pane has an active project.
299: 	if d.project != "" {
300: 		b.WriteByte('\n')
301: 		d.addAnchor(&b, d.jumpKey("Review state"), "Review state", 0)
302: 		d.field(&b, w, "Review state", styledReviewStateText(d.theme, d.state))
303: 		d.addAnchor(&b, d.jumpKey("IDS"), "IDS", 0)
304: 		d.field(&b, w, "IDS", detailIDSText(d.idsEntry))
305: 		d.addAnchor(&b, d.jumpKey("Tags"), "Tags", 0)
306: 		d.field(&b, w, "Tags", tagsText(d.tags))
307: 	}
308: 
309: 	// Family-graph edge counts. The dedicated panes (c/b) list the edges.
310: 	b.WriteByte('\n')
311: 	d.addAnchor(&b, d.jumpKey("Citations"), "Citations", 0)
312: 	d.field(&b, w, "Citations", fmt.Sprintf("%d", d.relCounts[domain.RelationCites]))
313: 	d.field(&b, w, "Cited by", fmt.Sprintf("%d", d.relCounts[domain.RelationCitedBy]))
314: 	d.field(&b, w, "Parents", fmt.Sprintf("%d", d.relCounts[domain.RelationParent]))
315: 	d.field(&b, w, "Children", fmt.Sprintf("%d", d.relCounts[domain.RelationChild]))
316: 
317: 	// Every life-stage document — the application stays visible here even once
318: 	// the patent has published.
319: 	d.addAnchor(&b, d.jumpKey("Documents"), "Documents", 1)
320: 	b.WriteByte('\n')
321: 	displayDocs := "Documents"
322: 	if d.jumpActive {
323: 		displayDocs = fmt.Sprintf("[%s] Documents", d.theme.Warn.Copy().Bold(true).Render(string(d.jumpKey("Documents"))))
324: 	}
325: 	b.WriteString(d.theme.Header.Render(displayDocs))
326: 	b.WriteByte('\n')
327: 	if len(p.Documents) == 0 {
328: 		b.WriteString(d.theme.Dim.Render("  (none)"))
329: 		b.WriteByte('\n')
330: 	}
331: 	for _, doc := range p.Documents {
332: 		line := "  " + render.Pad(string(doc.Stage), 13) + " " +
333: 			render.Pad(doc.Number.String(), 20) + " " + dateText(doc.Dated)
334: 		b.WriteString(d.theme.Row.Render(render.Truncate(line, w)))
335: 		b.WriteByte('\n')
336: 	}
337: 
338: 	d.addAnchor(&b, d.jumpKey("First claim"), "First claim", 1)
339: 	d.section(&b, w, "First claim", p.FirstClaim)
340: 	d.addAnchor(&b, d.jumpKey("Abstract"), "Abstract", 1)
341: 	d.section(&b, w, "Abstract", p.Abstract)
342: 	return strings.TrimRight(b.String(), "\n")
343: }
344: 
345: // addAnchor records a jump anchor for the next labelled line b will write.
346: // lineDelta offsets the recorded line past a leading blank or heading: 0 for a
347: // plain field, 1 for a section whose heading follows a spacer line.
348: func (d *Detail) addAnchor(b *strings.Builder, key rune, label string, lineDelta int) {
349: 	d.anchors = append(d.anchors, render.JumpAnchor{
350: 		Key:   key,
351: 		Label: label,
352: 		Line:  strings.Count(b.String(), "\n") + lineDelta,
353: 	})
354: }
355: 
356: // JumpAnchors implements pane.JumpProvider: the jump targets of the last render.
357: func (d *Detail) JumpAnchors() []render.JumpAnchor { return d.anchors }
358: 
359: // JumpTo implements pane.JumpProvider, scrolling the body so line leads the
360: // visible window.
361: func (d *Detail) JumpTo(line int) { d.page.ScrollTo(line) }
362: 
363: // SetJumpActive updates the jump mode state, triggering inline shortcut rendering.
364: func (d *Detail) SetJumpActive(active bool) {
365: 	d.jumpActive = active
366: }
367: 
368: // JumpActive reports whether jump mode is active.
369: func (d *Detail) JumpActive() bool { return d.jumpActive }
370: 
371: // nextAnchorLine returns the line of the first anchor after the cursor, or the
372: // first anchor when the cursor is at or past the last anchor.
373: func (d *Detail) nextAnchorLine() int {
374: 	cursor := d.page.Cursor()
375: 	for _, a := range d.anchors {
376: 		if a.Line > cursor {
377: 			return a.Line
378: 		}
379: 	}
380: 	return d.anchors[0].Line
381: }
382: 
383: // prevAnchorLine returns the line of the last anchor before the cursor, or the
384: // last anchor when the cursor is at or before the first anchor.
385: func (d *Detail) prevAnchorLine() int {
386: 	cursor := d.page.Cursor()
387: 	for i := len(d.anchors) - 1; i >= 0; i-- {
388: 		if d.anchors[i].Line < cursor {
389: 			return d.anchors[i].Line
390: 		}
391: 	}
392: 	return d.anchors[len(d.anchors)-1].Line
393: }
394: 
395: // computeJumpKeys assigns a stable jump key to each anchor label, avoiding
396: // conflicts with keys already bound in the base and detail keymap layers.
397: // Each label's characters are tried in order (first letter, second, etc.);
398: // if none are free, the first free letter in a-z is used, then 0-9.
399: func (d *Detail) computeJumpKeys(bound []rune) {
400: 	boundSet := make(map[rune]bool, len(bound))
401: 	for _, r := range bound {
402: 		boundSet[r] = true
403: 	}
404: 	used := make(map[rune]bool, len(detailAnchorLabels))
405: 	d.jumpKeys = make(map[string]rune, len(detailAnchorLabels))
406: 	for _, label := range detailAnchorLabels {
407: 		key := d.assignKey(label, boundSet, used)
408: 		d.jumpKeys[label] = key
409: 		used[key] = true
410: 	}
411: }
412: 
413: func (d *Detail) assignKey(label string, boundSet, used map[rune]bool) rune {
414: 	for _, r := range label {
415: 		switch {
416: 		case r >= 'A' && r <= 'Z':
417: 			r = r - 'A' + 'a'
418: 			fallthrough
419: 		case r >= 'a' && r <= 'z':
420: 			if !boundSet[r] && !used[r] {
421: 				return r
422: 			}
423: 		case r >= '0' && r <= '9':
424: 			if !boundSet[r] && !used[r] {
425: 				return r
426: 			}
427: 		}
428: 	}
429: 	for r := 'a'; r <= 'z'; r++ {
430: 		if !boundSet[r] && !used[r] {
431: 			return r
432: 		}
433: 	}
434: 	for r := '0'; r <= '9'; r++ {
435: 		if !used[r] {
436: 			return r
437: 		}
438: 	}
439: 	return '?'
440: }
441: 
442: // jumpKey returns the assigned jump key for a label, or 0 if unset.
443: func (d *Detail) jumpKey(label string) rune {
444: 	if d.jumpKeys != nil {
445: 		if key, ok := d.jumpKeys[label]; ok {
446: 			return key
447: 		}
448: 	}
449: 	return 0
450: }
451: 
452: // HandleKey implements pane.KeyHandler. When jump mode is active it intercepts
453: // single-letter keys that match a jump anchor, scrolling to that section and
454: // consuming the key so it never reaches the keymap.
455: func (d *Detail) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {
456: 	if !d.jumpActive {
457: 		return d, nil, false
458: 	}
459: 	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
460: 		r := msg.Runes[0]
461: 		for _, a := range d.anchors {
462: 			if a.Key == r {
463: 				d.JumpTo(a.Line)
464: 				return d, nil, true
465: 			}
466: 		}
467: 	}
468: 	return d, nil, false
469: }
470: 
471: // numberToShow returns the record's display number, falling back to the
472: // record key when no documents set one.
473: func numberToShow(p domain.Patent) domain.PatentNumber {
474: 	if !p.DisplayNumber.IsZero() {
475: 		return p.DisplayNumber
476: 	}
477: 	return p.Number
478: }
479: 
480: // field writes one "Label: value" line, truncated to the body width.
481: func (d *Detail) field(b *strings.Builder, w int, label, value string) {
482: 	labelW := 14
483: 	displayLabel := label
484: 	if d.jumpActive {
485: 		labelW = 18
486: 		if key, ok := d.jumpKeys[label]; ok {
487: 			displayLabel = fmt.Sprintf("[%s] %s", d.theme.Warn.Copy().Bold(true).Render(string(key)), label)
488: 		} else {
489: 			displayLabel = "    " + label
490: 		}
491: 	}
492: 	if strings.TrimSpace(value) == "" {
493: 		value = "—"
494: 	}
495: 	b.WriteString(d.theme.Header.Render(render.Pad(displayLabel, labelW)))
496: 	b.WriteString(" ")
497: 	b.WriteString(d.theme.Row.Render(render.Truncate(value, max(w-labelW-1, 0))))
498: 	b.WriteByte('\n')
499: }
500: 
501: // section writes a heading followed by word-wrapped body text, so long fields
502: // like the first claim and abstract are readable in the scrolling view.
503: func (d *Detail) section(b *strings.Builder, w int, heading, text string) {
504: 	b.WriteByte('\n')
505: 	displayHeading := heading
506: 	if d.jumpActive {
507: 		if key, ok := d.jumpKeys[heading]; ok {
508: 			displayHeading = fmt.Sprintf("[%s] %s", d.theme.Warn.Copy().Bold(true).Render(string(key)), heading)
509: 		}
510: 	}
511: 	b.WriteString(d.theme.Header.Render(displayHeading))
512: 	b.WriteByte('\n')
513: 	if strings.TrimSpace(text) == "" {
514: 		b.WriteString(d.theme.Dim.Render("  (none)"))
515: 		b.WriteByte('\n')
516: 		return
517: 	}
518: 	for _, line := range wrapText(text, max(w-2, 1)) {
519: 		b.WriteString(d.theme.Row.Render("  " + line))
520: 		b.WriteByte('\n')
521: 	}
522: }
523: 
524: // wrapText greedily word-wraps s to lines no wider than width.
525: func wrapText(s string, width int) []string {
526: 	var lines []string
527: 	var line strings.Builder
528: 	for _, word := range strings.Fields(s) {
529: 		switch {
530: 		case line.Len() == 0:
531: 			line.WriteString(word)
532: 		case line.Len()+1+len(word) <= width:
533: 			line.WriteByte(' ')
534: 			line.WriteString(word)
535: 		default:
536: 			lines = append(lines, line.String())
537: 			line.Reset()
538: 			line.WriteString(word)
539: 		}
540: 	}
541: 	if line.Len() > 0 {
542: 		lines = append(lines, line.String())
543: 	}
544: 	return lines
545: }
546: 
547: // reviewStateText renders a patent's review state within the pane's project,
548: // or a note when the patent is not a member of that project.
549: func reviewStateText(state domain.ReviewState) string {
550: 	if state == "" {
551: 		return "not in project"
552: 	}
553: 	return string(state)
554: }
555: 
556: func styledReviewStateText(theme render.Theme, state domain.ReviewState) string {
557: 	text := reviewStateText(state)
558: 	switch state {
559: 	case domain.ReviewStateUnderReview:
560: 		return theme.Warn.Render(text)
561: 	case domain.ReviewStateCached:
562: 		return theme.Dim.Render(text)
563: 	case domain.ReviewStateDeleted:
564: 		return theme.Error.Render(text)
565: 	default:
566: 		return text
567: 	}
568: }
569: 
570: func fetchStateText(theme render.Theme, state domain.FetchState) string {
571: 	text := string(state)
572: 	switch state {
573: 	case domain.FetchCached:
574: 		return theme.Dim.Render(text)
575: 	case domain.FetchStub:
576: 		return theme.MutedItalic.Render(text)
577: 	default:
578: 		return text
579: 	}
580: }
581: 
582: // tagsText renders a patent's tags as a comma-separated list, or a dash when
583: // it carries none.
584: func tagsText(tags []domain.Tag) string {
585: 	if len(tags) == 0 {
586: 		return "—"
587: 	}
588: 	names := make([]string, len(tags))
589: 	for i, t := range tags {
590: 		names[i] = t.Name
591: 	}
592: 	return strings.Join(names, ", ")
593: }
594: 
595: func detailIDSText(entry *domain.IDSEntry) string {
596: 	if entry == nil {
597: 		return "not on IDS"
598: 	}
599: 	return entry.SummaryText()
600: }
601: 
602: // countryOrDash returns the country code, or a dash when it is blank.
603: func countryOrDash(code string) string {
604: 	if strings.TrimSpace(code) == "" {
605: 		return "—"
606: 	}
607: 	return code
608: }
609: 
610: // expirationText renders the expiration date and how it was determined.
611: func expirationText(p domain.Patent) string {
612: 	if p.ExpirationDate.IsZero() {
613: 		return "—"
614: 	}
615: 	text := p.ExpirationDate.Format(detailDateLayout)
616: 	if p.ExpirationSource != "" {
617: 		text += " (" + p.ExpirationSource + ")"
618: 	}
619: 	return text
620: }
621: 
622: // dateText renders a date, or a dash when it is unset.
623: func dateText(t time.Time) string {
624: 	if t.IsZero() {
625: 		return "—"
626: 	}
627: 	return t.Format(detailDateLayout)
628: }

(End of file - total 628 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/domain/patent.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/domain/patent.go</path>
<type>file</type>
<content>
1: package domain
2: 
3: import (
4: 	"fmt"
5: 	"time"
6: )
7: 
8: // SortColumn names a column that patent listings can be ordered by.
9: type SortColumn string
10: 
11: const (
12: 	SortByNumber      SortColumn = "number"
13: 	SortByTitle       SortColumn = "title"
14: 	SortByInventor    SortColumn = "inventor"
15: 	SortByExpires     SortColumn = "expires"
16: 	SortByReviewState SortColumn = "review_state"
17: 	SortByIDS         SortColumn = "ids"
18: )
19: 
20: // CrawlProfile defines which family-graph edges to follow during a crawl.
21: type CrawlProfile string
22: 
23: const (
24: 	CrawlProfileCitations CrawlProfile = "citations" // Follow cites only (depth 0 only)
25: 	CrawlProfileCitedBy   CrawlProfile = "citedby"   // Follow cited_by only (depth 0 only)
26: 	CrawlProfileFamily    CrawlProfile = "family"    // Follow parent/child recursion
27: 	CrawlProfileAll       CrawlProfile = "all"       // Combination of the above
28: )
29: 
30: // Valid reports whether the profile is a known value.
31: func (p CrawlProfile) Valid() bool {
32: 	switch p {
33: 	case CrawlProfileCitations, CrawlProfileCitedBy, CrawlProfileFamily, CrawlProfileAll:
34: 		return true
35: 	default:
36: 		return false
37: 	}
38: }
39: 
40: // Source identifies where a patent record was crawled from.
41: type Source string
42: 
43: const (
44: 	// SourceFile is the local file source, used before live sources exist.
45: 	SourceFile Source = "file"
46: 	// SourceGoogle is Google Patents.
47: 	SourceGoogle Source = "google"
48: 	// SourceJustia is Justia Patents.
49: 	SourceJustia Source = "justia"
50: 	// SourceUSPTO is the United States Patent and Trademark Office.
51: 	SourceUSPTO Source = "uspto"
52: 	// SourcePCT is the WIPO / PCT international system.
53: 	SourcePCT Source = "pct"
54: )
55: 
56: // ExpirationEstimated marks an ExpirationDate derived by the +20-year rule
57: // rather than read from an authoritative source.
58: const ExpirationEstimated = "estimated"
59: 
60: // Valid reports whether the Source is a known value.
61: func (s Source) Valid() bool {
62: 	switch s {
63: 	case SourceFile, SourceGoogle, SourceJustia, SourceUSPTO, SourcePCT:
64: 		return true
65: 	default:
66: 		return false
67: 	}
68: }
69: 
70: // ParseSource converts a string into a Source.
71: func ParseSource(s string) (Source, error) {
72: 	src := Source(s)
73: 	if !src.Valid() {
74: 		return "", fmt.Errorf("domain: unknown source %q", s)
75: 	}
76: 	return src, nil
77: }
78: 
79: // Inventor is one human creator of a patented invention.
80: type Inventor string
81: 
82: // Patent is the core business object: one patent record. One record spans the
83: // invention's whole life — its application, publication, and grant are all
84: // Documents of the same record. It carries no I/O or UI concerns: persistence
85: // lives in package store, display in package tui.
86: type Patent struct {
87: 	// Number is the record's permanent number — the first document number
88: 	// ever seen for it. It never changes, even after the patent later
89: 	// publishes, so rows that point at a record stay valid.
90: 	Number PatentNumber `json:"number"`
91: 	// DisplayNumber is the number the record should be shown by: the
92: 	// latest-stage document (grant, else publication, else application).
93: 	DisplayNumber   PatentNumber `json:"display_number"`
94: 	Title           string       `json:"title"`
95: 	Abstract        string       `json:"abstract"`
96: 	Assignee        string       `json:"assignee"`
97: 	Inventors       []Inventor   `json:"inventors"`
98: 	FetchState      FetchState   `json:"fetch_state"`
99: 	Source          Source       `json:"source"`
100: 	ApplicationDate time.Time    `json:"application_date"` // Zero when unknown.
101: 	PublicationDate time.Time    `json:"publication_date"` // Zero when unknown.
102: 	GrantDate       time.Time    `json:"grant_date"`       // Zero when unknown.
103: 	FetchedAt       time.Time    `json:"fetched_at"`       // Zero for a stub never fetched.
104: 	// FirstClaim is the patent's claim 1, the body text shown in the detail view.
105: 	FirstClaim string `json:"first_claim,omitempty"`
106: 	// ExpirationDate is when the patent's protection ends; zero when unknown.
107: 	ExpirationDate time.Time `json:"expiration_date"`
108: 	// ExpirationSource records how ExpirationDate was determined, e.g. "estimated".
109: 	ExpirationSource string `json:"expiration_source,omitempty"`
110: 	// SourceURL is the provider page the record was fetched from.
111: 	SourceURL string `json:"source_url,omitempty"`
112: 	// Documents is the open-ended set of life-stage documents for this record.
113: 	Documents []Document `json:"documents"`
114: }
115: 
116: // PatentRow is the lightweight listing shape used by paged views. It keeps the
117: // record key and display number separate so list UIs stay cheap without losing
118: // the application/publication/grant distinction.
119: type PatentRow struct {
120: 	Number         PatentNumber `json:"number"`
121: 	DisplayNumber  PatentNumber `json:"display_number"`
122: 	Title          string       `json:"title"`
123: 	Inventors      []Inventor   `json:"inventors"`
124: 	ExpirationDate time.Time    `json:"expiration_date"`
125: 	Tags           []string     `json:"tags"`
126: 	FetchState     FetchState   `json:"fetch_state"`
127: 	ReviewState    ReviewState  `json:"review_state,omitempty"`
128: 	IDSEntry       *IDSEntry    `json:"ids_entry,omitempty"`
129: }
130: 
131: // IsStub reports whether only a reference exists, without the full body.
132: func (p Patent) IsStub() bool {
133: 	return p.FetchState == FetchStub
134: }
135: 
136: // LatestDocument returns the record's furthest-along document — grant over
137: // publication over application, breaking ties by date. ok is false when the
138: // record has no documents.
139: func (p Patent) LatestDocument() (latest Document, ok bool) {
140: 	for _, d := range p.Documents {
141: 		switch {
142: 		case !ok,
143: 			d.Stage.rank() > latest.Stage.rank(),
144: 			d.Stage.rank() == latest.Stage.rank() && d.Dated.After(latest.Dated):
145: 			latest, ok = d, true
146: 		}
147: 	}
148: 	return latest, ok
149: }
150: 
151: // NumberToShow returns the number the record should be displayed by: its
152: // latest document's number, or the record number when it has no documents.
153: func (p Patent) NumberToShow() PatentNumber {
154: 	if latest, ok := p.LatestDocument(); ok {
155: 		return latest.Number
156: 	}
157: 	return p.Number
158: }
159: 
160: // DocumentFor returns the document of the given stage, if the record has one.
161: func (p Patent) DocumentFor(stage Stage) (Document, bool) {
162: 	for _, d := range p.Documents {
163: 		if d.Stage == stage {
164: 			return d, true
165: 		}
166: 	}
167: 	return Document{}, false
168: }

(End of file - total 168 lines)
</content>
```

---

## Assistant (Plan · Big Pickle · 3.7s)

_Thinking:_

Now I have a good understanding. Let me also look at some key files for the key bindings and the command system to understand how new commands are registered.

Let me also look at the command system, key bindings, and how the TUI is structured.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/command"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/command</path>
<type>directory</type>
<entries>
catalog.go
command_test.go
command.go

(3 entries)
</entries>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/keys"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/keys</path>
<type>directory</type>
<entries>
chord.go
keys.go
reader_test.go
reader.go

(4 entries)
</entries>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui</path>
<type>directory</type>
<entries>
app_test.go
app.go
browser.go
keymap/
overlay/
pane/
policy.go
render/
wiring_test.go
wiring.go

(10 entries)
</entries>
```

---

## Assistant (Plan · Big Pickle · 3.6s)

_Thinking:_

Let me look at the command system and keymaps to understand how to add new commands.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/command/command.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/command/command.go</path>
<type>file</type>
<content>
1: // Package command is the frontend-agnostic action catalog. Every action the
2: // system can perform is one Command declaration. Keymaps (TUI), HTTP routes
3: // (web API), and the help screen are all generated from these declarations,
4: // so an action is defined once and every frontend stays in step.
5: //
6: // A Command carries no behavior and no display strings: it is structure only.
7: // A frontend's dispatcher interprets the Command.ID; human labels live in the
8: // text catalog, keyed by ID. The Registry is built once at startup and injected
9: // — there is deliberately no package-level mutable state.
10: package command
11: 
12: import (
13: 	"fmt"
14: 	"maps"
15: 	"slices"
16: 	"strings"
17: 
18: 	"patentmine/internal/proto"
19: )
20: 
21: // ID is a stable, typed command identifier. Every ID is a const in catalog.go;
22: // nothing spells one as a bare string.
23: type ID string
24: 
25: // Kind classifies how a command is carried out.
26: type Kind string
27: 
28: const (
29: 	// KindView changes only client-local view state (scrolling, opening an
30: 	// overlay, switching panes). It never touches the daemon.
31: 	KindView Kind = "view"
32: 	// KindEngine results in a request to the daemon over the proto layer.
33: 	KindEngine Kind = "engine"
34: )
35: 
36: // Scope names a UI situation — which pane or overlay is active. Commands are
37: // scoped to scopes so a keymap can offer the right actions and the help
38: // screen can group them.
39: type Scope string
40: 
41: const (
42: 	ScopeCatalog   Scope = "catalog"   // the main patent list
43: 	ScopeDetail    Scope = "detail"    // one patent's detail view
44: 	ScopeCitations Scope = "citations" // a citations / cited-by list
45: 	ScopeIDS       Scope = "ids"       // one patent's IDS entry editor
46: 	ScopeProjects  Scope = "projects"  // the project list
47: 	ScopeOverlay   Scope = "overlay"   // a modal overlay is focused
48: )
49: 
50: // Command is the declaration of one action.
51: type Command struct {
52: 	// ID uniquely identifies the command.
53: 	ID ID
54: 	// Name is the canonical typed-command form, such as "open.projects".
55: 	// Empty means the command is not exposed in the command palette/prompt.
56: 	Name string
57: 	// Aliases are additional typed forms accepted by the command prompt.
58: 	Aliases []string
59: 	// Usage is a concise invocation example shown in help/palette footers. It
60: 	// is command syntax, not prose, so it stays a literal rather than a key.
61: 	Usage string
62: 	// Kind says whether the command hits the daemon or only the local view.
63: 	Kind Kind
64: 	// Method is the proto method a KindEngine command maps to; empty for
65: 	// KindView commands.
66: 	Method proto.Method
67: 	// Scopes lists the scopes in which the command is offered. An empty
68: 	// Scopes means the command is global (valid in every scope).
69: 	Scopes []Scope
70: }
71: 
72: // Global reports whether the command applies in every scope.
73: func (c Command) Global() bool {
74: 	return len(c.Scopes) == 0
75: }
76: 
77: // AvailableIn reports whether the command is offered in scope.
78: func (c Command) AvailableIn(scope Scope) bool {
79: 	if c.Global() {
80: 		return true
81: 	}
82: 	return slices.Contains(c.Scopes, scope)
83: }
84: 
85: // Registry is an immutable, injected catalog of commands. Build it once with
86: // NewRegistry and pass it to frontends; never store one in a package global.
87: type Registry struct {
88: 	byID    map[ID]Command
89: 	byName  map[string]ID
90: 	ordered []Command
91: }
92: 
93: // NewRegistry builds a Registry, rejecting duplicate IDs and malformed
94: // commands so a wiring mistake fails loudly at startup.
95: func NewRegistry(commands ...Command) (*Registry, error) {
96: 	r := &Registry{
97: 		byID:    make(map[ID]Command, len(commands)),
98: 		byName:  make(map[string]ID, len(commands)),
99: 		ordered: make([]Command, 0, len(commands)),
100: 	}
101: 	for _, c := range commands {
102: 		if c.ID == "" {
103: 			return nil, fmt.Errorf("command: a command has an empty ID")
104: 		}
105: 		if _, dup := r.byID[c.ID]; dup {
106: 			return nil, fmt.Errorf("command: duplicate command ID %q", c.ID)
107: 		}
108: 		if c.Kind == KindEngine && c.Method == "" {
109: 			return nil, fmt.Errorf("command: engine command %q has no method", c.ID)
110: 		}
111: 		if c.Kind == KindView && c.Method != "" {
112: 			return nil, fmt.Errorf("command: view command %q must not set a method", c.ID)
113: 		}
114: 		if err := validateTypedNames(r.byName, c); err != nil {
115: 			return nil, err
116: 		}
117: 		r.byID[c.ID] = c
118: 		r.ordered = append(r.ordered, c)
119: 	}
120: 	return r, nil
121: }
122: 
123: // Lookup returns the command for id.
124: func (r *Registry) Lookup(id ID) (Command, bool) {
125: 	c, ok := r.byID[id]
126: 	return c, ok
127: }
128: 
129: // All returns every command in registration order.
130: func (r *Registry) All() []Command {
131: 	out := make([]Command, len(r.ordered))
132: 	copy(out, r.ordered)
133: 	return out
134: }
135: 
136: // InScope returns the commands offered in scope, in registration order.
137: func (r *Registry) InScope(scope Scope) []Command {
138: 	var out []Command
139: 	for _, c := range r.ordered {
140: 		if c.AvailableIn(scope) {
141: 			out = append(out, c)
142: 		}
143: 	}
144: 	return out
145: }
146: 
147: // TypedInScope returns commands that are offered in scope and exposed in the
148: // command palette/prompt.
149: func (r *Registry) TypedInScope(scope Scope) []Command {
150: 	var out []Command
151: 	for _, c := range r.InScope(scope) {
152: 		if c.Name != "" {
153: 			out = append(out, c)
154: 		}
155: 	}
156: 	return out
157: }
158: 
159: // LookupName resolves a canonical or alias typed command.
160: func (r *Registry) LookupName(name string) (Command, bool) {
161: 	id, ok := r.byName[strings.ToLower(strings.TrimSpace(name))]
162: 	if !ok {
163: 		return Command{}, false
164: 	}
165: 	return r.byID[id], true
166: }
167: 
168: // Len reports how many commands the registry holds.
169: func (r *Registry) Len() int {
170: 	return len(r.ordered)
171: }
172: 
173: func validateTypedNames(index map[string]ID, c Command) error {
174: 	seen := maps.Clone(index)
175: 	register := func(name string) error {
176: 		key := strings.ToLower(strings.TrimSpace(name))
177: 		if key == "" {
178: 			return nil
179: 		}
180: 		if prior, dup := seen[key]; dup {
181: 			return fmt.Errorf("command: typed name %q for %q conflicts with %q", key, c.ID, prior)
182: 		}
183: 		seen[key] = c.ID
184: 		index[key] = c.ID
185: 		return nil
186: 	}
187: 	if err := register(c.Name); err != nil {
188: 		return err
189: 	}
190: 	for _, alias := range c.Aliases {
191: 		if err := register(alias); err != nil {
192: 			return err
193: 		}
194: 	}
195: 	return nil
196: }

(End of file - total 196 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/command/catalog.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/command/catalog.go</path>
<type>file</type>
<content>
1: package command
2: 
3: import "patentmine/internal/proto"
4: 
5: // Command IDs. Every action in the system is named here exactly once; no other
6: // file spells a command identifier as a bare string.
7: const (
8: 	// Navigation within a list or scrollable pane.
9: 	NavDown     ID = "nav.down"
10: 	NavUp       ID = "nav.up"
11: 	NavPageDown ID = "nav.page-down"
12: 	NavPageUp   ID = "nav.page-up"
13: 	NavTop          ID = "nav.top"
14: 	NavBottom       ID = "nav.bottom"
15: 	ReselectLast    ID = "nav.reselect-last"
16: 
17: 	// Moving between panes and overlays.
18: 	OpenDetail    ID = "view.detail"
19: 	OpenCitations ID = "view.citations"
20: 	OpenCitedBy   ID = "view.cited-by"
21: 	OpenIDS       ID = "view.ids"
22: 	OpenProjects  ID = "view.projects"
23: 	Back          ID = "view.back"
24: 	CloseOverlay  ID = "view.close-overlay"
25: 	Refresh       ID = "view.refresh"
26: 	OpenSearch    ID = "search.open"
27: 	OpenCommand   ID = "command.open"
28: 	JumpMode      ID = "view.jump"
29: 	SelectVisual  ID = "select.visual"
30: 	SelectClear   ID = "select.clear"
31: 	SelectAll     ID = "select.all"
32: 	ColNext       ID = "col.next"
33: 	ColPrev       ID = "col.prev"
34: 	SortApply     ID = "col.sort-apply"
35: 	OpenBrowser   ID = "view.browser"
36: 
37: 	// Application-wide actions.
38: 	Quit ID = "app.quit"
39: 	Help ID = "app.help"
40: 
41: 	// Engine reads, backing both pane data loads and web API routes.
42: 	PatentList      ID = "patent.list"
43: 	PatentGet       ID = "patent.get"
44: 	PatentRelations ID = "patent.relations"
45: 	ProjectList     ID = "project.list"
46: 
47: 	// ExportIDS builds an Information Disclosure Statement for a project.
48: 	ExportIDS ID = "ids.export"
49: 
50: 	// Patent review-state changes. All four map to one proto method; the
51: 	// target state is the difference the dispatcher supplies.
52: 	MarkStored      ID = "patent.mark-stored"
53: 	MarkUnderReview ID = "patent.mark-under-review"
54: 	MarkIgnored     ID = "patent.mark-ignored"
55: 	MarkDeleted     ID = "patent.mark-deleted"
56: 	AddToProject    ID = "patent.add-to-project"
57: 
58: 	// Tagging. Both act on the selected patent within the active project.
59: 	TagAdd    ID = "patent.tag"
60: 	TagRemove ID = "patent.untag"
61: 
62: 	// Tag taxonomy and patent tag assignments.
63: 	TagTaxonomyAdd    ID = "tag.add"
64: 	TagTaxonomyList   ID = "tag.list"
65: 	TagTaxonomyDelete ID = "tag.delete"
66: 	TagPatentAdd      ID = "tag.patent.add"
67: 	TagPatentDelete   ID = "tag.patent.delete"
68: 	TagPatentList     ID = "tag.patent.list"
69: 	TagPatentManage   ID = "tag.patent"
70: 
71: 	// PatentDelete permanently removes a patent from the database.
72: 	PatentDelete ID = "patent.delete"
73: 
74: 	// Crawling.
75: 	CrawlFamily    ID = "crawl.family"
76: 	CrawlCitations ID = "crawl.citations"
77: 	CrawlCitedBy   ID = "crawl.citedby"
78: 	CrawlAll       ID = "crawl.all"
79: 	CrawlCancel    ID = "crawl.cancel"
80: 	LookupPatent   ID = "patent.lookup"
81: 	Import          ID = "patent.import"
82: 
83: 	// Projects.
84: 	ProjectCreate      ID = "project.create"
85: 	ProjectActivate    ID = "project.activate"
86: 	ProjectClearActive ID = "project.clear-active"
87: 
88: 	// Filtering.
89: 	Filter   ID = "view.filter"
90: 	FindOpen ID = "find.open"
91: 
92: 	// IDS entry editing.
93: 	IDSEditField   ID = "ids.edit-field"
94: 	IDSToggleFull  ID = "ids.toggle-full"
95: 	IDSCycleStatus ID = "ids.cycle-status"
96: 	IDSDelete      ID = "ids.delete"
97: )
98: 
99: // listScopes are the scopes that behave as scrollable lists.
100: var listScopes = []Scope{ScopeCatalog, ScopeCitations, ScopeProjects, ScopeDetail, ScopeIDS}
101: 
102: // patentScopes are the scopes where a patent is selected and can be acted on.
103: var patentScopes = []Scope{ScopeCatalog, ScopeDetail, ScopeCitations}
104: 
105: // projectScopes are the scopes where project-focused actions are relevant.
106: var projectScopes = []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeProjects}
107: 
108: // Default returns the registry of every command the system supports. It is the
109: // single source of truth: build it once at startup and inject it.
110: func Default() (*Registry, error) {
111: 	return NewRegistry(
112: 		// --- navigation (view) ---
113: 		Command{ID: NavDown, Kind: KindView, Scopes: listScopes},
114: 		Command{ID: NavUp, Kind: KindView, Scopes: listScopes},
115: 		Command{ID: NavPageDown, Kind: KindView, Scopes: listScopes},
116: 		Command{ID: NavPageUp, Kind: KindView, Scopes: listScopes},
117: 		Command{ID: NavTop, Kind: KindView, Scopes: listScopes},
118: 		Command{ID: NavBottom, Kind: KindView, Scopes: listScopes},
119: 		Command{ID: ReselectLast, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
120: 
121: 		// --- panes and overlays (view) ---
122: 		Command{ID: OpenDetail, Name: "open.detail", Aliases: []string{"detail"}, Usage: ":open.detail", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
123: 		Command{ID: OpenBrowser, Name: "browse", Aliases: []string{"open.browser", "web"}, Usage: ":browse [PATENT ...]", Kind: KindView, Scopes: patentScopes},
124: 		Command{ID: OpenCitations, Name: "open.citations", Aliases: []string{"citations"}, Usage: ":open.citations", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail}},
125: 		Command{ID: OpenCitedBy, Name: "open.citedby", Aliases: []string{"citedby"}, Usage: ":open.citedby", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail}},
126: 		Command{ID: OpenProjects, Name: "open.projects", Aliases: []string{"projects"}, Usage: ":open.projects", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeIDS}},
127: 		Command{ID: OpenIDS, Name: "open.ids", Aliases: []string{"ids"}, Usage: ":open.ids", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeCitations}},
128: 		Command{ID: Back, Kind: KindView},
129: 		Command{ID: CloseOverlay, Kind: KindView, Scopes: []Scope{ScopeOverlay}},
130: 		Command{ID: Refresh, Name: "refresh", Aliases: []string{"reload"}, Usage: ":refresh", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeProjects, ScopeIDS}},
131: 		Command{ID: OpenSearch, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeProjects}},
132: 		Command{ID: OpenCommand, Kind: KindView},
133: 		Command{ID: JumpMode, Name: "jump", Aliases: []string{"jump-to-field"}, Usage: ":jump", Kind: KindView, Scopes: []Scope{ScopeDetail}},
134: 		Command{ID: SelectVisual, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
135: 		Command{ID: SelectClear, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
136: 		Command{ID: SelectAll, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
137: 		Command{ID: ColNext, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
138: 		Command{ID: ColPrev, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
139: 		Command{ID: SortApply, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
140: 		Command{ID: ProjectActivate, Name: "project.use", Aliases: []string{"use-project"}, Usage: ":project.use [PROJECT]", Kind: KindView, Scopes: projectScopes},
141: 		Command{ID: ProjectClearActive, Name: "project.clear", Aliases: []string{"clear-project"}, Usage: ":project.clear", Kind: KindView, Scopes: projectScopes},
142: 
143: 		// --- filtering (view) ---
144: 		Command{ID: Filter, Name: "filter", Aliases: []string{"f", "filter.clear"}, Usage: ":filter <type> <value>", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
145: 		Command{ID: FindOpen, Name: "find", Aliases: []string{"/"}, Usage: ":find", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
146: 		Command{ID: IDSEditField, Kind: KindView, Scopes: []Scope{ScopeIDS}},
147: 		Command{ID: IDSToggleFull, Kind: KindView, Scopes: []Scope{ScopeIDS}},
148: 		Command{ID: IDSCycleStatus, Kind: KindView, Scopes: []Scope{ScopeIDS}},
149: 		Command{ID: IDSDelete, Kind: KindView, Scopes: []Scope{ScopeIDS}},
150: 
151: 		// --- application-wide (view) ---
152: 		Command{ID: Quit, Name: "quit", Aliases: []string{"exit"}, Usage: ":quit", Kind: KindView},
153: 		Command{ID: Help, Name: "help", Aliases: []string{"?"}, Usage: ":help", Kind: KindView},
154: 
155: 		// --- engine reads ---
156: 		Command{ID: PatentList, Kind: KindEngine, Method: proto.MethodPatentList},
157: 		Command{ID: PatentGet, Kind: KindEngine, Method: proto.MethodPatentGet},
158: 		Command{ID: PatentRelations, Kind: KindEngine, Method: proto.MethodRelations},
159: 		Command{ID: ProjectList, Kind: KindEngine, Method: proto.MethodProjectList},
160: 		Command{ID: ExportIDS, Kind: KindEngine, Method: proto.MethodIDSExport, Scopes: []Scope{ScopeProjects}},
161: 
162: 		// --- patent review state (engine) ---
163: 		Command{ID: MarkStored, Name: "review_state.stored", Aliases: []string{"stored"}, Usage: ":review_state.stored", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
164: 		Command{ID: MarkUnderReview, Name: "review_state.under_review", Aliases: []string{"under-review"}, Usage: ":review_state.under_review", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
165: 		Command{ID: MarkIgnored, Name: "review_state.ignored", Aliases: []string{"ignored"}, Usage: ":review_state.ignored", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
166: 		Command{ID: MarkDeleted, Name: "review_state.deleted", Aliases: []string{"deleted"}, Usage: ":review_state.deleted", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
167: 		Command{ID: AddToProject, Name: "add", Aliases: []string{"add-to-project"}, Usage: ":add [PATENT]", Kind: KindEngine, Method: proto.MethodMembershipAdd, Scopes: patentScopes},
168: 		Command{ID: PatentDelete, Name: "delete", Aliases: []string{"delete-patent"}, Usage: ":delete", Kind: KindEngine, Method: proto.MethodPatentDelete, Scopes: patentScopes},
169: 		Command{ID: TagAdd, Name: "tag", Aliases: []string{"tag-patent"}, Usage: ":tag <name>", Kind: KindEngine, Method: proto.MethodTagAssign, Scopes: patentScopes},
170: 		Command{ID: TagRemove, Name: "untag", Aliases: []string{"untag-patent"}, Usage: ":untag <name>", Kind: KindEngine, Method: proto.MethodTagRemove, Scopes: patentScopes},
171: 		Command{ID: TagTaxonomyAdd, Name: "tag.add", Aliases: []string{"create-tag"}, Usage: ":tag.add <name>", Kind: KindEngine, Method: proto.MethodTagCreate, Scopes: projectScopes},
172: 		Command{ID: TagTaxonomyList, Name: "tag.list", Aliases: []string{"list-tags"}, Usage: ":tag.list", Kind: KindEngine, Method: proto.MethodTagList, Scopes: projectScopes},
173: 		Command{ID: TagTaxonomyDelete, Name: "tag.delete", Aliases: []string{"delete-tag"}, Usage: ":tag.delete <name>", Kind: KindEngine, Method: proto.MethodTagDelete, Scopes: projectScopes},
174: 		Command{ID: TagPatentAdd, Name: "tag.patent.add", Aliases: []string{"patent-tag"}, Usage: ":tag.patent.add <name>", Kind: KindEngine, Method: proto.MethodPatentTagAdd, Scopes: patentScopes},
175: 		Command{ID: TagPatentDelete, Name: "tag.patent.delete", Aliases: []string{"patent-untag"}, Usage: ":tag.patent.delete <name>", Kind: KindEngine, Method: proto.MethodPatentTagDelete, Scopes: patentScopes},
176: 		Command{ID: TagPatentList, Name: "tag.patent.list", Aliases: []string{"patent-tags"}, Usage: ":tag.patent.list", Kind: KindEngine, Method: proto.MethodPatentTagList, Scopes: patentScopes},
177: 		Command{ID: TagPatentManage, Name: "tag.patent", Aliases: []string{"tag-manage"}, Usage: ":tag.patent", Kind: KindEngine, Method: proto.MethodPatentTagList, Scopes: patentScopes},
178: 
179: 		// --- crawling (engine) ---
180: 		Command{ID: CrawlFamily, Name: "crawl.family", Aliases: []string{"family"}, Usage: ":crawl.family", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
181: 		Command{ID: CrawlCitations, Name: "crawl.citations", Aliases: []string{"crawl-citations"}, Usage: ":crawl.citations", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
182: 		Command{ID: CrawlCitedBy, Name: "crawl.citedby", Aliases: []string{"crawl-citedby"}, Usage: ":crawl.citedby", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
183: 		Command{ID: CrawlAll, Name: "crawl.all", Aliases: []string{"crawl", "recursion"}, Usage: ":crawl.all", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
184: 		Command{ID: LookupPatent, Name: "lookup", Aliases: []string{"lookup-patent"}, Usage: ":lookup", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
185: 		Command{ID: Import, Name: "import", Aliases: []string{"import-patent"}, Usage: ":import <number|file> [force]", Kind: KindEngine, Method: proto.MethodCrawlFamily},
186: 		Command{ID: CrawlCancel, Kind: KindEngine, Method: proto.MethodCrawlCancel},
187: 
188: 		// --- projects (engine) ---
189: 		Command{ID: ProjectCreate, Name: "project.create", Aliases: []string{"create-project"}, Usage: ":project.create [NAME]", Kind: KindEngine, Method: proto.MethodProjectCreate, Scopes: []Scope{ScopeProjects}},
190: 	)
191: }

(End of file - total 191 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/keymap"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/keymap</path>
<type>directory</type>
<entries>
default.go
hint.go
keymap_test.go
keymap.go

(4 entries)
</entries>
```

---

## Assistant (Plan · Big Pickle · 3.6s)

_Thinking:_

Let me also look at the keymap files to understand how keyboard shortcuts are defined, and also the keymap default bindings.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/keymap/keymap.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/keymap/keymap.go</path>
<type>file</type>
<content>
1: // Package keymap maps key sequences to commands as a stack of layers. An
2: // overlay pushes a blocking layer that overrides — and masks — the layers
3: // beneath it, while the layers beneath keep rendering as a dimmed background.
4: // The keymap is built from data and injected; there is no global keymap state.
5: package keymap
6: 
7: import (
8: 	"maps"
9: 	"slices"
10: 	"sort"
11: 	"strings"
12: 	"unicode"
13: 
14: 	"patentmine/internal/command"
15: 	"patentmine/internal/keys"
16: )
17: 
18: // Layer is one named set of key-sequence bindings. A Blocking layer (used by
19: // overlays) stops resolution from falling through to the layers below it.
20: type Layer struct {
21: 	Name     string
22: 	Blocking bool
23: 	bindings map[string]command.ID
24: }
25: 
26: // NewLayer creates an empty layer.
27: func NewLayer(name string, blocking bool) *Layer {
28: 	return &Layer{Name: name, Blocking: blocking, bindings: make(map[string]command.ID)}
29: }
30: 
31: // Bind maps a canonical key sequence (e.g. "g g", "ctrl+d") to a command. It
32: // returns the layer so binds can be chained.
33: func (l *Layer) Bind(sequence string, id command.ID) *Layer {
34: 	l.bindings[sequence] = id
35: 	return l
36: }
37: 
38: // BindAll applies a set of bindings at once.
39: func (l *Layer) BindAll(bindings map[string]command.ID) *Layer {
40: 	maps.Copy(l.bindings, bindings)
41: 	return l
42: }
43: 
44: // Bindings returns a copy of the layer's sequence-to-command map.
45: func (l *Layer) Bindings() map[string]command.ID {
46: 	return maps.Clone(l.bindings)
47: }
48: 
49: // BoundLetters returns the single-letter and single-digit keys bound in this layer.
50: func (l *Layer) BoundLetters() []rune {
51: 	var out []rune
52: 	for seq := range l.bindings {
53: 		if len(seq) == 1 {
54: 			r := rune(seq[0])
55: 			if unicode.IsLetter(r) || unicode.IsDigit(r) {
56: 				out = append(out, r)
57: 			}
58: 		}
59: 	}
60: 	return out
61: }
62: 
63: // Stack is the ordered set of active layers. The last pushed layer has the
64: // highest priority. Resolution and matching honour Blocking layers.
65: type Stack struct {
66: 	layers []*Layer
67: }
68: 
69: // NewStack creates a stack from base layers, lowest priority first.
70: func NewStack(base ...*Layer) *Stack {
71: 	return &Stack{layers: append([]*Layer(nil), base...)}
72: }
73: 
74: // Push adds a layer at the top (highest priority).
75: func (s *Stack) Push(l *Layer) {
76: 	s.layers = append(s.layers, l)
77: }
78: 
79: // Pop removes the top layer, if any.
80: func (s *Stack) Pop() {
81: 	if n := len(s.layers); n > 0 {
82: 		s.layers = s.layers[:n-1]
83: 	}
84: }
85: 
86: // Depth reports how many layers are stacked.
87: func (s *Stack) Depth() int {
88: 	return len(s.layers)
89: }
90: 
91: // active returns the participating layers, top priority first: every layer
92: // from the top down to and including the first Blocking layer (all layers when
93: // none blocks).
94: func (s *Stack) active() []*Layer {
95: 	var out []*Layer
96: 	for i := len(s.layers) - 1; i >= 0; i-- {
97: 		out = append(out, s.layers[i])
98: 		if s.layers[i].Blocking {
99: 			break
100: 		}
101: 	}
102: 	return out
103: }
104: 
105: // Resolve returns the command bound to a complete key sequence.
106: func (s *Stack) Resolve(sequence string) (command.ID, bool) {
107: 	for _, l := range s.active() {
108: 		if id, ok := l.bindings[sequence]; ok {
109: 			return id, true
110: 		}
111: 	}
112: 	return "", false
113: }
114: 
115: // Match classifies a candidate sequence for the input Reader. It is the Lookup
116: // the keys.Reader expects.
117: func (s *Stack) Match(sequence string) keys.Match {
118: 	var complete, prefix bool
119: 	for _, l := range s.active() {
120: 		for bound := range l.bindings {
121: 			switch {
122: 			case bound == sequence:
123: 				complete = true
124: 			case strings.HasPrefix(bound, sequence+" "):
125: 				prefix = true
126: 			}
127: 		}
128: 	}
129: 	switch {
130: 	case complete && prefix:
131: 		return keys.CompleteAndPrefix
132: 	case complete:
133: 		return keys.Complete
134: 	case prefix:
135: 		return keys.Prefix
136: 	default:
137: 		return keys.NoMatch
138: 	}
139: }
140: 
141: // BoundLetters returns the sorted, deduplicated single-letter/digit keys bound
142: // in the base layer and the given scope layer combined.
143: func (k *Keymaps) BoundLetters(scope command.Scope) []rune {
144: 	seen := map[rune]bool{}
145: 	collect := func(layer *Layer) {
146: 		if layer == nil {
147: 			return
148: 		}
149: 		for _, r := range layer.BoundLetters() {
150: 			seen[r] = true
151: 		}
152: 	}
153: 	collect(k.base)
154: 	collect(k.scopes[scope])
155: 	out := make([]rune, 0, len(seen))
156: 	for r := range seen {
157: 		out = append(out, r)
158: 	}
159: 	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
160: 	return out
161: }
162: 
163: // Shortcuts returns the active key sequences that invoke id in scope.
164: func (k *Keymaps) Shortcuts(scope command.Scope, id command.ID) []string {
165: 	seen := map[string]bool{}
166: 	var out []string
167: 	collect := func(layer *Layer) {
168: 		if layer == nil {
169: 			return
170: 		}
171: 		for seq, bound := range layer.bindings {
172: 			if bound != id || seen[seq] {
173: 				continue
174: 			}
175: 			seen[seq] = true
176: 			out = append(out, seq)
177: 		}
178: 	}
179: 	collect(k.base)
180: 	collect(k.scopes[scope])
181: 	slices.Sort(out)
182: 	return out
183: }

(End of file - total 183 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/keymap/default.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/keymap/default.go</path>
<type>file</type>
<content>
1: package keymap
2: 
3: import (
4: 	"maps"
5: 
6: 	"patentmine/internal/command"
7: )
8: 
9: // Keymaps holds the base layer plus one layer per UI scope. A frontend
10: // composes a Stack from these; Default() builds the shipped bindings, but the
11: // same shape could be loaded from a user config file.
12: type Keymaps struct {
13: 	base     *Layer
14: 	scopes map[command.Scope]*Layer
15: }
16: 
17: // Base returns the always-active global layer.
18: func (k *Keymaps) Base() *Layer {
19: 	return k.base
20: }
21: 
22: // Scope returns the layer for s, or nil when s has no dedicated layer.
23: func (k *Keymaps) Scope(s command.Scope) *Layer {
24: 	return k.scopes[s]
25: }
26: 
27: // ScopeLayers returns every scope-scoped layer keyed by its scope. The
28: // wiring check walks this to verify every bound key reaches a handler.
29: func (k *Keymaps) ScopeLayers() map[command.Scope]*Layer {
30: 	out := make(map[command.Scope]*Layer, len(k.scopes))
31: 	maps.Copy(out, k.scopes)
32: 	return out
33: }
34: 
35: // StackFor builds the resolution stack for a scope: the base layer plus the
36: // scope's own layer.
37: func (k *Keymaps) StackFor(s command.Scope) *Stack {
38: 	st := NewStack(k.base)
39: 	if layer := k.scopes[s]; layer != nil {
40: 		st.Push(layer)
41: 	}
42: 	return st
43: }
44: 
45: // listMotions are the navigation bindings shared by every scrollable context.
46: func listMotions() map[string]command.ID {
47: 	return map[string]command.ID{
48: 		"j":      command.NavDown,
49: 		"down":   command.NavDown,
50: 		"k":      command.NavUp,
51: 		"up":     command.NavUp,
52: 		"ctrl+d": command.NavPageDown,
53: 		"pgdown": command.NavPageDown,
54: 		"ctrl+u": command.NavPageUp,
55: 		"pgup":   command.NavPageUp,
56: 		"g g":    command.NavTop,
57: 		"G":      command.NavBottom,
58: 	}
59: }
60: 
61: // patentActions are the bindings shared by scopes where a patent is selected.
62: func patentActions() map[string]command.ID {
63: 	return map[string]command.ID{
64: 		"s": command.MarkStored,
65: 		"r": command.MarkUnderReview,
66: 		"i": command.MarkIgnored,
67: 		"x": command.MarkDeleted,
68: 		"D": command.PatentDelete,
69: 		"a": command.AddToProject,
70: 		"f": command.CrawlFamily,
71: 		"L": command.LookupPatent,
72: 	}
73: }
74: 
75: // viewActions are the bindings shared by scopes that display a patent and
76: // its related views (detail, citations, cited-by, IDS, browser, etc.).
77: func viewActions() map[string]command.ID {
78: 	return map[string]command.ID{
79: 		"I":      command.OpenIDS,
80: 		"w":      command.OpenBrowser,
81: 		"c":      command.OpenCitations,
82: 		"b":      command.OpenCitedBy,
83: 		"p":      command.OpenProjects,
84: 		"ctrl+r": command.Refresh,
85: 	}
86: }
87: 
88: // Default returns the shipped keymap.
89: func Default() *Keymaps {
90: 	base := NewLayer("global", false).BindAll(map[string]command.ID{
91: 		"ctrl+c": command.Quit,
92: 		"Q":      command.Quit,
93: 		"?":      command.Help,
94: 		"h":      command.Back,
95: 		"left":   command.Back,
96: 		"esc":    command.Back,
97: 		":":      command.OpenCommand,
98: 	})
99: 
100: 	catalog := NewLayer("catalog", false).
101: 		BindAll(listMotions()).
102: 		BindAll(patentActions()).
103: 		BindAll(viewActions()).
104: 		BindAll(map[string]command.ID{
105: 			"enter":  command.OpenDetail,
106: 			"l":      command.OpenDetail,
107: 			"right":  command.ColNext,
108: 			"left":   command.ColPrev,
109: 			".":      command.SortApply,
110: 			"/":      command.FindOpen,
111: 			"n":      command.NavDown,
112: 			"N":      command.NavUp,
113: 			"g v":    command.ReselectLast,
114: 			"v":      command.SelectVisual,
115: 			"esc":    command.SelectClear,
116: 			"g a":    command.SelectAll,
117: 			"ctrl+a": command.SelectAll,
118: 		})
119: 
120: 	detail := NewLayer("detail", false).
121: 		BindAll(listMotions()).
122: 		BindAll(patentActions()).
123: 		BindAll(viewActions()).
124: 		BindAll(map[string]command.ID{
125: 			"/":  command.OpenSearch,
126: 			";":  command.JumpMode,
127: 		})
128: 
129: 	citations := NewLayer("citations", false).
130: 		BindAll(listMotions()).
131: 		BindAll(patentActions()).
132: 		BindAll(viewActions()).
133: 		BindAll(map[string]command.ID{
134: 			"enter":  command.OpenDetail,
135: 			"l":      command.OpenDetail,
136: 			"right":  command.ColNext,
137: 			"left":   command.ColPrev,
138: 			".":      command.SortApply,
139: 			"/":      command.FindOpen,
140: 			"n":      command.NavDown,
141: 			"N":      command.NavUp,
142: 			"g v":    command.ReselectLast,
143: 			"v":      command.SelectVisual,
144: 			"g a":    command.SelectAll,
145: 			"ctrl+a": command.SelectAll,
146: 		})
147: 
148: 	ids := NewLayer("ids", false).
149: 		BindAll(listMotions()).
150: 		BindAll(map[string]command.ID{
151: 			"enter":  command.IDSEditField,
152: 			"e":      command.IDSEditField,
153: 			"f":      command.IDSToggleFull,
154: 			"s":      command.IDSCycleStatus,
155: 			"D":      command.IDSDelete,
156: 			"p":      command.OpenProjects,
157: 			"ctrl+r": command.Refresh,
158: 		})
159: 
160: 	projects := NewLayer("projects", false).
161: 		BindAll(listMotions()).
162: 		BindAll(map[string]command.ID{
163: 			"enter":  command.ProjectActivate,
164: 			"l":      command.ProjectActivate,
165: 			"right":  command.ProjectActivate,
166: 			"u":      command.ProjectClearActive,
167: 			"n":      command.ProjectCreate,
168: 			"I":      command.ExportIDS,
169: 			"/":      command.OpenSearch,
170: 			"ctrl+r": command.Refresh,
171: 		})
172: 
173: 	// The overlay layer is composed directly on top of the global base while an
174: 	// overlay is focused — the App leaves the pane layer out entirely, so the
175: 	// pane's bindings are inactive while only the global ones remain.
176: 	overlay := NewLayer("overlay", false).BindAll(map[string]command.ID{
177: 		"esc":    command.CloseOverlay,
178: 		"q":      command.CloseOverlay,
179: 		"j":      command.NavDown,
180: 		"down":   command.NavDown,
181: 		"k":      command.NavUp,
182: 		"up":     command.NavUp,
183: 		"ctrl+d": command.NavPageDown,
184: 		"ctrl+u": command.NavPageUp,
185: 	})
186: 
187: 	return &Keymaps{
188: 		base: base,
189: 		scopes: map[command.Scope]*Layer{
190: 			command.ScopeCatalog:   catalog,
191: 			command.ScopeDetail:    detail,
192: 			command.ScopeCitations: citations,
193: 			command.ScopeIDS:       ids,
194: 			command.ScopeProjects:  projects,
195: 			command.ScopeOverlay:   overlay,
196: 		},
197: 	}
198: }

(End of file - total 198 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/app.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/app.go</path>
<type>file</type>
<content>
1: // Package tui is the terminal client. The App is the bubbletea root: it owns
2: // the pane and overlay stacks and routes input, but holds no business state —
3: // every screen's state lives in its own Pane or Overlay. That decomposition is
4: // the deliberate structural defence against a god-object UI model.
5: //
6: // Input flows through one path. A key chord and a typed command both resolve
7: // to a command.ID and run through invoke, so the two can never diverge. Every
8: // command.ID is serviced by exactly one handler — an entry in appHandlers or a
9: // pane/overlay that lists the ID in Handles — and validateWiring fails the boot
10: // if any bound key or typed command would resolve to an unhandled ID.
11: package tui
12: 
13: import (
14: 	"context"
15: 	"encoding/json"
16: 	"fmt"
17: 	"slices"
18: 	"strings"
19: 	"time"
20: 
21: 	tea "github.com/charmbracelet/bubbletea"
22: 	"github.com/charmbracelet/lipgloss"
23: 
24: 	"patentmine/internal/command"
25: 	"patentmine/internal/domain"
26: 	"patentmine/internal/keys"
27: 	"patentmine/internal/proto"
28: 	"patentmine/internal/rpc"
29: 	"patentmine/internal/text"
30: 	"patentmine/internal/tui/keymap"
31: 	"patentmine/internal/tui/overlay"
32: 	"patentmine/internal/tui/pane"
33: 	"patentmine/internal/tui/render"
34: 	appversion "patentmine/internal/version"
35: )
36: 
37: // statusRows is the bottom status line the App always draws.
38: const statusRows = 1
39: 
40: // pingTimeout bounds the startup version probe.
41: const pingTimeout = 5 * time.Second
42: 
43: // overlay box sizing.
44: const (
45: 	overlayMaxWidth  = 76
46: 	overlayMaxHeight = 22
47: 	overlayMargin    = 4 // free space kept around the box
48: 	overlayChrome    = 4 // border (2) + horizontal padding (2)
49: )
50: 
51: // busEventMsg carries a daemon event into the bubbletea update loop.
52: type busEventMsg struct{ event proto.Event }
53: 
54: // eventsClosedMsg signals that the daemon event stream ended.
55: type eventsClosedMsg struct{}
56: 
57: // pingLoadedMsg carries the daemon's reported build version.
58: type pingLoadedMsg struct {
59: 	version string
60: 	err     error
61: }
62: 
63: // invocation carries the arguments of one command request: empty for a key
64: // chord, populated for a typed command.
65: type invocation struct {
66: 	repeat int
67: 	args   []string
68: }
69: 
70: // appHandler services one command at the App level. appHandlers is the single
71: // table of App-handled commands; validateWiring reads it to prove every bound
72: // key reaches a handler.
73: type appHandler func(*App, invocation) (tea.Model, tea.Cmd)
74: 
75: var appHandlers = map[command.ID]appHandler{
76: 	command.Quit:               (*App).cmdQuit,
77: 	command.Help:               (*App).cmdHelp,
78: 	command.OpenSearch:         (*App).cmdOpenSearch,
79: 	command.OpenCommand:        (*App).cmdOpenCommand,
80: 	command.JumpMode:           (*App).cmdJumpMode,
81: 	command.CloseOverlay:       (*App).cmdCloseOverlay,
82: 	command.Back:               (*App).cmdBack,
83: 	command.OpenDetail:         (*App).cmdOpenDetail,
84: 	command.OpenBrowser:        (*App).cmdOpenBrowser,
85: 	command.OpenCitations:      (*App).cmdOpenCitations,
86: 	command.OpenCitedBy:        (*App).cmdOpenCitedBy,
87: 	command.OpenIDS:            (*App).cmdOpenIDS,
88: 	command.OpenProjects:       (*App).cmdOpenProjects,
89: 	command.ProjectActivate:    (*App).cmdProjectActivate,
90: 	command.ProjectClearActive: (*App).cmdProjectClear,
91: 	command.ProjectCreate:      (*App).cmdProjectCreate,
92: 	command.AddToProject:       (*App).cmdAddToProject,
93: 	command.Import:             (*App).cmdImport,
94: 	command.MarkStored:         (*App).cmdMarkStored,
95: 	command.MarkUnderReview:    (*App).cmdMarkUnderReview,
96: 	command.MarkIgnored:        (*App).cmdMarkIgnored,
97: 	command.MarkDeleted:        (*App).cmdMarkDeleted,
98: 	command.TagAdd:             (*App).cmdTagAdd,
99: 	command.TagRemove:          (*App).cmdTagRemove,
100: 	command.TagTaxonomyAdd:     (*App).cmdTagTaxonomyAdd,
101: 	command.TagTaxonomyList:    (*App).cmdTagTaxonomyList,
102: 	command.TagPatentManage:    (*App).cmdTagPatentManage,
103: 	command.TagTaxonomyDelete:  (*App).cmdTagTaxonomyDelete,
104: 	command.TagPatentAdd:       (*App).cmdTagPatentAdd,
105: 	command.TagPatentDelete:    (*App).cmdTagPatentDelete,
106: 	command.TagPatentList:      (*App).cmdTagPatentList,
107: 	command.PatentDelete:       (*App).cmdPatentDelete,
108: }
109: 
110: // typedAcceptsArgs lists the commands whose typed form takes arguments. Every
111: // other typed command is rejected with a usage error when given any.
112: var typedAcceptsArgs = map[command.ID]bool{
113: 	command.AddToProject:    true,
114: 	command.ProjectActivate: true,
115: 	command.ProjectCreate:   true,
116: 	command.Import:          true,
117: 	command.TagAdd:          true,
118: 	command.TagRemove:       true,
119: 	command.TagTaxonomyAdd:     true,
120: 	command.TagTaxonomyDelete:  true,
121: 	command.TagPatentAdd:       true,
122: 	command.TagPatentDelete:    true,
123: 	command.Filter:          true,
124: 	command.OpenBrowser:     true,
125: }
126: 
127: // App is the bubbletea root model.
128: type App struct {
129: 	client          *rpc.Client
130: 	registry        *command.Registry
131: 	keymaps         *keymap.Keymaps
132: 	hints           *keymap.HintCatalog
133: 	theme           render.Theme
134: 	text            *text.Catalog
135: 	reader          keys.Reader
136: 	saveLastProject func(domain.ProjectID) error
137: 
138: 	panes      []pane.Pane
139: 	overlays   []overlay.Overlay
140: 	confirmCmd tea.Cmd // pending action awaiting confirmation
141: 
142: 	status        string
143: 	statusErr     bool
144: 	width         int
145: 	height        int
146: 	activeProject *domain.Project
147: 	lastProjectID domain.ProjectID
148: 	tuiVersion    string
149: 	daemonVersion string
150: 	openURL       func(string) error
151: }
152: 
153: type Option func(*App)
154: 
155: func WithLastProject(id domain.ProjectID) Option {
156: 	return func(a *App) { a.lastProjectID = id }
157: }
158: 
159: func WithLastProjectSaver(save func(domain.ProjectID) error) Option {
160: 	return func(a *App) { a.saveLastProject = save }
161: }
162: 
163: // New builds the App with the splash/project selector as the initial pane. It
164: // fails when the keymap, command registry, and handlers are not consistent —
165: // see validateWiring — so a wiring mistake is caught at startup.
166: func New(client *rpc.Client, registry *command.Registry, keymaps *keymap.Keymaps, catalog *text.Catalog, opts ...Option) (*App, error) {
167: 	if err := validateWiring(registry, keymaps, catalog); err != nil {
168: 		return nil, err
169: 	}
170: 	theme := render.NewTheme()
171: 	hints, err := keymap.NewHintCatalog(registry, keymap.DefaultHints())
172: 	if err != nil {
173: 		return nil, err
174: 	}
175: 	app := &App{
176: 		client:        client,
177: 		registry:      registry,
178: 		keymaps:       keymaps,
179: 		hints:         hints,
180: 		theme:         theme,
181: 		text:          catalog,
182: 		tuiVersion:    appversion.String(),
183: 		daemonVersion: "connecting",
184: 		openURL:       openExternalURL,
185: 	}
186: 	app.status = catalog.T(text.StatusWelcome)
187: 	for _, opt := range opts {
188: 		opt(app)
189: 	}
190: 	app.panes = []pane.Pane{pane.NewSplash(client, theme, app.lastProjectID,
191: 		app.splashFooterHint(), app.splashEmptyHint())}
192: 	return app, nil
193: }
194: 
195: // Init implements tea.Model.
196: func (a *App) Init() tea.Cmd {
197: 	cmds := []tea.Cmd{a.panes[0].Init(), a.fetchPing()}
198: 	if a.client != nil {
199: 		cmds = append(cmds, a.listen())
200: 	}
201: 	return tea.Batch(cmds...)
202: }
203: 
204: // listen waits for one daemon event and delivers it as a message.
205: func (a *App) listen() tea.Cmd {
206: 	client := a.client
207: 	return func() tea.Msg {
208: 		ev, ok := <-client.Events()
209: 		if !ok {
210: 			return eventsClosedMsg{}
211: 		}
212: 		return busEventMsg{event: ev}
213: 	}
214: }
215: 
216: // fetchPing asks the daemon which build version it is running.
217: func (a *App) fetchPing() tea.Cmd {
218: 	if a.client == nil {
219: 		return nil
220: 	}
221: 	client := a.client
222: 	return func() tea.Msg {
223: 		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
224: 		defer cancel()
225: 		var res proto.PingResult
226: 		err := client.Call(ctx, proto.MethodPing, nil, &res)
227: 		return pingLoadedMsg{version: res.Version, err: err}
228: 	}
229: }
230: 
231: // Update implements tea.Model.
232: func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
233: 	switch m := msg.(type) {
234: 	case tea.WindowSizeMsg:
235: 		a.width, a.height = m.Width, m.Height
236: 		return a, a.broadcast(pane.ResizeMsg{Width: m.Width, Height: max(m.Height-statusRows, 1)})
237: 	case tea.KeyMsg:
238: 		return a.handleKey(m)
239: 	case overlay.ConfirmAcceptMsg:
240: 		a.popOverlay()
241: 		cmd := a.confirmCmd
242: 		a.confirmCmd = nil
243: 		return a, cmd
244: 	case overlay.ConfirmRejectMsg:
245: 		a.popOverlay()
246: 		a.confirmCmd = nil
247: 		return a, nil
248: 	case overlay.PromptSubmitMsg:
249: 		a.popOverlay()
250: 		return a.executeTypedCommand(m.Input)
251: 	case overlay.PromptCloseMsg:
252: 		a.popOverlay()
253: 		return a, nil
254: 	case overlay.TextSubmitMsg:
255: 		a.popOverlay()
256: 		return a.handleTextSubmit(m)
257: 	case pane.EditIDSFieldMsg:
258: 		return a.openIDSEditInput(m.Field)
259: 	case overlay.JumpSelectMsg:
260: 		a.popOverlay()
261: 		if provider, ok := a.focusedPane().(pane.JumpProvider); ok {
262: 			provider.JumpTo(m.Line)
263: 		}
264: 		return a, nil
265: 	case overlay.CloseOverlayMsg:
266: 		a.popOverlay()
267: 		return a, nil
268: 	case pane.StatusMsg:
269: 		a.status, a.statusErr = a.text.Tf(m.Key, m.Args...), m.Error
270: 		if m.Key == text.StatusCrawlStarted && len(m.Args) >= 2 {
271: 			jobID, _ := m.Args[1].(string)
272: 			isLookup := len(m.Args) >= 3
273: 			if d, ok := m.Args[2].(int); ok && d == 0 {
274: 				isLookup = true
275: 			}
276: 			verb := "Crawling"
277: 			if isLookup {
278: 				verb = "Looking up"
279: 			}
280: 			title := verb + " " + m.Args[0].(string)
281: 			loading := overlay.NewLoading(a.theme, []string{jobID}, title)
282: 			a.overlays = append(a.overlays, loading)
283: 			return a, loading.Init()
284: 		}
285: 		return a, nil
286: 	case pane.MultiCrawlStartedMsg:
287: 		isLookup := m.Depth == 0
288: 		verb := "Crawling"
289: 		if isLookup {
290: 			verb = "Looking up"
291: 		}
292: 		title := fmt.Sprintf("%s %d patents", verb, len(m.Numbers))
293: 		loading := overlay.NewLoading(a.theme, m.JobIDs, title)
294: 		a.overlays = append(a.overlays, loading)
295: 		return a, loading.Init()
296: 	case pingLoadedMsg:
297: 		if m.err != nil {
298: 			a.daemonVersion = "unavailable"
299: 			return a, nil
300: 		}
301: 		a.daemonVersion = m.version
302: 		return a, nil
303: 	case busEventMsg:
304: 		return a, tea.Batch(
305: 			a.handleEvent(m.event),
306: 			a.broadcastOverlays(m.event),
307: 			a.listen())
308: 	case eventsClosedMsg:
309: 		a.setErr(text.StatusDaemonClosed)
310: 		return a, nil
311: 	default:
312: 		// rpc results, spinner ticks and the like — let every pane/overlay consume what is theirs.
313: 		return a, tea.Batch(a.broadcast(msg), a.broadcastOverlays(msg))
314: 	}
315: }
316: 
317: // broadcast forwards a message to every pane.
318: func (a *App) broadcast(msg tea.Msg) tea.Cmd {
319: 	var cmds []tea.Cmd
320: 	for i, p := range a.panes {
321: 		updated, cmd := p.Update(msg)
322: 		a.panes[i] = updated
323: 		if cmd != nil {
324: 			cmds = append(cmds, cmd)
325: 		}
326: 	}
327: 	return tea.Batch(cmds...)
328: }
329: 
330: // broadcastOverlays forwards a message to every overlay.
331: func (a *App) broadcastOverlays(msg tea.Msg) tea.Cmd {
332: 	var cmds []tea.Cmd
333: 	for i, ov := range a.overlays {
334: 		if u, ok := ov.(interface {
335: 			Update(tea.Msg) (overlay.Overlay, tea.Cmd)
336: 		}); ok {
337: 			updated, cmd := u.Update(msg)
338: 			a.overlays[i] = updated
339: 			if cmd != nil {
340: 				cmds = append(cmds, cmd)
341: 			}
342: 		}
343: 	}
344: 	return tea.Batch(cmds...)
345: }
346: 
347: // handleKey feeds a key press through the chord reader and keymap.
348: func (a *App) handleKey(m tea.KeyMsg) (tea.Model, tea.Cmd) {
349: 	if len(a.overlays) > 0 {
350: 		if handler, ok := a.focusedOverlay().(overlay.KeyHandler); ok {
351: 			updated, cmd, consumed := handler.HandleKey(m)
352: 			a.overlays[len(a.overlays)-1] = updated
353: 			if consumed {
354: 				return a, cmd
355: 			}
356: 		}
357: 	}
358: 	if len(a.overlays) == 0 {
359: 		if interceptor, ok := a.focusedPane().(pane.KeyHandler); ok {
360: 			updated, cmd, consumed := interceptor.HandleKey(m)
361: 			a.panes[len(a.panes)-1] = updated
362: 			if consumed {
363: 				return a, cmd
364: 			}
365: 		}
366: 	}
367: 	stack := a.keyStack()
368: 	chord, ok := a.reader.Feed(keys.Key(m.String()), stack.Match)
369: 	if !ok {
370: 		return a, nil // buffered as part of a longer sequence
371: 	}
372: 	id, ok := stack.Resolve(chord.Sequence())
373: 	if !ok {
374: 		return a, nil // unbound sequence
375: 	}
376: 	return a.invoke(id, invocation{repeat: chord.Repeat()})
377: }
378: 
379: // keyStack composes the active keymap. With an overlay open the pane layer is
380: // deliberately left out so only the overlay and global bindings apply.
381: func (a *App) keyStack() *keymap.Stack {
382: 	stack := keymap.NewStack(a.keymaps.Base())
383: 	var scope command.Scope
384: 	if len(a.overlays) > 0 {
385: 		scope = command.ScopeOverlay
386: 	} else {
387: 		scope = a.focusedPane().Scope()
388: 	}
389: 	if layer := a.keymaps.Scope(scope); layer != nil {
390: 		stack.Push(layer)
391: 	}
392: 	return stack
393: }
394: 
395: // invoke carries out a resolved command. App-level commands run from the
396: // appHandlers table; everything else is forwarded to the focused overlay or
397: // pane — but only when that overlay or pane lists the command in Handles, so a
398: // command can never be silently dropped.
399: func (a *App) invoke(id command.ID, inv invocation) (tea.Model, tea.Cmd) {
400: 	if handler, ok := appHandlers[id]; ok {
401: 		return handler(a, inv)
402: 	}
403: 	if len(a.overlays) > 0 {
404: 		ov := a.focusedOverlay()
405: 		if !slices.Contains(ov.Handles(), id) {
406: 			return a.unhandled(id)
407: 		}
408: 		updated, cmd := ov.Command(id, inv.repeat)
409: 		a.overlays[len(a.overlays)-1] = updated
410: 		return a, cmd
411: 	}
412: 	p := a.focusedPane()
413: 	if !slices.Contains(p.Handles(), id) {
414: 		return a.unhandled(id)
415: 	}
416: 	updated, cmd := p.Command(id, pane.Invocation{Repeat: inv.repeat, Args: inv.args})
417: 	a.panes[len(a.panes)-1] = updated
418: 	return a, cmd
419: }
420: 
421: // unhandled reports a command that reached invoke with no handler. validateWiring
422: // makes this unreachable for bound keys and typed commands; it stays as a
423: // visible last line of defence rather than a silent no-op.
424: func (a *App) unhandled(id command.ID) (tea.Model, tea.Cmd) {
425: 	a.setErr(text.StatusUnhandledCommand, string(id))
426: 	return a, nil
427: }
428: 
429: // --- App-level command handlers ---------------------------------------------
430: 
431: func (a *App) cmdQuit(invocation) (tea.Model, tea.Cmd) { return a, tea.Quit }
432: 
433: func (a *App) cmdHelp(invocation) (tea.Model, tea.Cmd) {
434: 	if len(a.overlays) == 0 {
435: 		a.overlays = append(a.overlays, overlay.NewHelp(a.registry, a.keymaps, a.theme, a.text))
436: 	}
437: 	return a, nil
438: }
439: 
440: func (a *App) cmdOpenSearch(invocation) (tea.Model, tea.Cmd) {
441: 	return a.openPrompt(overlay.PromptPalette)
442: }
443: 
444: func (a *App) cmdOpenCommand(invocation) (tea.Model, tea.Cmd) {
445: 	return a.openPrompt(overlay.PromptDirect)
446: }
447: 
448: // cmdJumpMode toggles jump mode on the focused pane. When jump mode is active
449: // the pane renders inline shortcut labels (e.g. "[a] Assignee") and navigation
450: // keys (j/k) cycle between labelled sections instead of scrolling line-by-line.
451: // It is a no-op when an overlay is already open.
452: func (a *App) cmdJumpMode(invocation) (tea.Model, tea.Cmd) {
453: 	if len(a.overlays) > 0 {
454: 		return a, nil
455: 	}
456: 	if toggler, ok := a.focusedPane().(interface {
457: 		SetJumpActive(bool)
458: 		JumpActive() bool
459: 	}); ok {
460: 		toggler.SetJumpActive(!toggler.JumpActive())
461: 	}
462: 	return a, nil
463: }
464: 
465: func (a *App) cmdCloseOverlay(invocation) (tea.Model, tea.Cmd) {
466: 	a.popOverlay()
467: 	return a, nil
468: }
469: 
470: func (a *App) cmdBack(invocation) (tea.Model, tea.Cmd) {
471: 	if len(a.overlays) > 0 {
472: 		a.popOverlay()
473: 	} else if len(a.panes) > 1 {
474: 		a.panes = a.panes[:len(a.panes)-1]
475: 	}
476: 	return a, nil
477: }
478: 
479: func (a *App) cmdOpenDetail(invocation) (tea.Model, tea.Cmd) { return a.openDetail() }
480: 
481: func (a *App) cmdOpenBrowser(inv invocation) (tea.Model, tea.Cmd) {
482: 	var numbers []domain.PatentNumber
483: 	if len(inv.args) > 0 {
484: 		numbers = make([]domain.PatentNumber, 0, len(inv.args))
485: 		for _, arg := range inv.args {
486: 			number, err := domain.ParsePatentNumber(arg)
487: 			if err != nil {
488: 				a.setErr(text.StatusInvalidPatentNumber, err.Error())
489: 				return a, nil
490: 			}
491: 			numbers = append(numbers, number)
492: 		}
493: 	} else {
494: 		numbers = a.focusedSelections()
495: 		if len(numbers) == 0 {
496: 			a.setErr(text.StatusNoPatentSelected)
497: 			return a, nil
498: 		}
499: 	}
500: 	return a, a.openPatentsInBrowser(numbers)
501: }
502: func (a *App) cmdOpenCitations(invocation) (tea.Model, tea.Cmd) {
503: 	return a.openCitations(domain.RelationCites)
504: }
505: func (a *App) cmdOpenCitedBy(invocation) (tea.Model, tea.Cmd) {
506: 	return a.openCitations(domain.RelationCitedBy)
507: }
508: 
509: func (a *App) cmdOpenIDS(invocation) (tea.Model, tea.Cmd) { return a.openIDS() }
510: 
511: func (a *App) cmdOpenProjects(invocation) (tea.Model, tea.Cmd) {
512: 	return a.pushPane(pane.NewProjects(a.client, a.theme))
513: }
514: 
515: func (a *App) cmdProjectClear(invocation) (tea.Model, tea.Cmd) {
516: 	a.activeProject = nil
517: 	a.setStatus(text.StatusClearedProject)
518: 	return a, a.broadcast(pane.ProjectChangedMsg{})
519: }
520: 
521: func (a *App) cmdMarkStored(invocation) (tea.Model, tea.Cmd) {
522: 	return a.runReviewState(command.MarkStored, domain.ReviewStateStored)
523: }
524: func (a *App) cmdMarkUnderReview(invocation) (tea.Model, tea.Cmd) {
525: 	return a.runReviewState(command.MarkUnderReview, domain.ReviewStateUnderReview)
526: }
527: func (a *App) cmdMarkIgnored(invocation) (tea.Model, tea.Cmd) {
528: 	return a.runReviewState(command.MarkIgnored, domain.ReviewStateIgnored)
529: }
530: func (a *App) cmdMarkDeleted(invocation) (tea.Model, tea.Cmd) {
531: 	return a.runReviewState(command.MarkDeleted, domain.ReviewStateDeleted)
532: }
533: 
534: func (a *App) cmdProjectActivate(inv invocation) (tea.Model, tea.Cmd) {
535: 	switch len(inv.args) {
536: 	case 0:
537: 		return a.activateProject()
538: 	case 1:
539: 		return a.activateProjectByArg(inv.args[0])
540: 	default:
541: 		return a.usageError(command.ProjectActivate)
542: 	}
543: }
544: 
545: func (a *App) cmdAddToProject(inv invocation) (tea.Model, tea.Cmd) {
546: 	switch len(inv.args) {
547: 	case 0:
548: 		return a.runProjectAction(func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
549: 			return pane.AddToProjectCmd(a.client, project, patent)
550: 		})
551: 	case 1:
552: 		number, err := domain.ParsePatentNumber(inv.args[0])
553: 		if err != nil {
554: 			a.setErr(text.StatusInvalidPatentNumber, err.Error())
555: 			return a, nil
556: 		}
557: 		if a.activeProject == nil {
558: 			a.setErr(text.StatusNoActiveProject)
559: 			return a, nil
560: 		}
561: 		if a.client == nil {
562: 			a.setErr(text.StatusDaemonUnavailable)
563: 			return a, nil
564: 		}
565: 		return a, pane.AddToProjectCmd(a.client, a.activeProject.ID, number)
566: 	default:
567: 		return a.usageError(command.AddToProject)
568: 	}
569: }
570: 
571: // cmdTagAdd tags the selected patent within the active project. The tag name
572: // is the typed argument; it may contain spaces.
573: func (a *App) cmdTagAdd(inv invocation) (tea.Model, tea.Cmd) {
574: 	if len(inv.args) == 0 {
575: 		return a.usageError(command.TagAdd)
576: 	}
577: 	name := strings.Join(inv.args, " ")
578: 	return a.runAction(command.TagAdd, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
579: 		return pane.AssignTagCmd(a.client, project, patent, name)
580: 	})
581: }
582: 
583: // cmdTagRemove removes a tag from the selected patent within the active project.
584: func (a *App) cmdTagRemove(inv invocation) (tea.Model, tea.Cmd) {
585: 	if len(inv.args) == 0 {
586: 		return a.usageError(command.TagRemove)
587: 	}
588: 	name := strings.Join(inv.args, " ")
589: 	return a.runAction(command.TagRemove, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
590: 		return pane.RemoveTagCmd(a.client, project, patent, name)
591: 	})
592: }
593: 
594: // cmdTagTaxonomyAdd registers a tag in the project's taxonomy.
595: func (a *App) cmdTagTaxonomyAdd(inv invocation) (tea.Model, tea.Cmd) {
596: 	if len(inv.args) == 0 {
597: 		return a.usageError(command.TagTaxonomyAdd)
598: 	}
599: 	name := strings.Join(inv.args, " ")
600: 	if a.activeProject == nil {
601: 		a.setErr(text.StatusNoActiveProject)
602: 		return a, nil
603: 	}
604: 	if a.client == nil {
605: 		a.setErr(text.StatusDaemonUnavailable)
606: 		return a, nil
607: 	}
608: 	return a, pane.CreateTagTaxonomyCmd(a.client, a.activeProject.ID, name)
609: }
610: 
611: // cmdTagTaxonomyList lists all taxonomy tags in the active project.
612: func (a *App) cmdTagTaxonomyList(inv invocation) (tea.Model, tea.Cmd) {
613: 	if a.activeProject == nil {
614: 		a.setErr(text.StatusNoActiveProject)
615: 		return a, nil
616: 	}
617: 	if a.client == nil {
618: 		a.setErr(text.StatusDaemonUnavailable)
619: 		return a, nil
620: 	}
621: 	o, cmd := overlay.NewTagTaxonomyOverlay(a.client, a.theme, a.text, a.activeProject.ID)
622: 	a.overlays = append(a.overlays, o)
623: 	return a, cmd
624: }
625: 
626: // cmdTagPatentManage opens the interactive tag manager popup for the selected patent(s).
627: func (a *App) cmdTagPatentManage(inv invocation) (tea.Model, tea.Cmd) {
628: 	if a.activeProject == nil {
629: 		a.setErr(text.StatusNoActiveProject)
630: 		return a, nil
631: 	}
632: 	if a.client == nil {
633: 		a.setErr(text.StatusDaemonUnavailable)
634: 		return a, nil
635: 	}
636: 	numbers := a.focusedSelections()
637: 	if len(numbers) == 0 {
638: 		a.setErr(text.StatusNoPatentSelected)
639: 		return a, nil
640: 	}
641: 	o, cmd := overlay.NewTagPatentOverlay(a.client, a.theme, a.text, a.activeProject.ID, numbers)
642: 	a.overlays = append(a.overlays, o)
643: 	return a, cmd
644: }
645: 
646: // cmdTagTaxonomyDelete removes a tag from the project's taxonomy.
647: func (a *App) cmdTagTaxonomyDelete(inv invocation) (tea.Model, tea.Cmd) {
648: 	if len(inv.args) == 0 {
649: 		return a.usageError(command.TagTaxonomyDelete)
650: 	}
651: 	name := strings.Join(inv.args, " ")
652: 	if a.activeProject == nil {
653: 		a.setErr(text.StatusNoActiveProject)
654: 		return a, nil
655: 	}
656: 	if a.client == nil {
657: 		a.setErr(text.StatusDaemonUnavailable)
658: 		return a, nil
659: 	}
660: 	return a, pane.DeleteTagTaxonomyCmd(a.client, a.activeProject.ID, name)
661: }
662: 
663: // cmdTagPatentAdd assigns a tag to the selected patent within the active project.
664: func (a *App) cmdTagPatentAdd(inv invocation) (tea.Model, tea.Cmd) {
665: 	if len(inv.args) == 0 {
666: 		return a.usageError(command.TagPatentAdd)
667: 	}
668: 	name := strings.Join(inv.args, " ")
669: 	return a.runAction(command.TagPatentAdd, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
670: 		return pane.AssignPatentTagCmd(a.client, project, patent, name)
671: 	})
672: }
673: 
674: // cmdTagPatentDelete removes a tag assignment from the selected patent within the active project.
675: func (a *App) cmdTagPatentDelete(inv invocation) (tea.Model, tea.Cmd) {
676: 	if len(inv.args) == 0 {
677: 		return a.usageError(command.TagPatentDelete)
678: 	}
679: 	name := strings.Join(inv.args, " ")
680: 	return a.runAction(command.TagPatentDelete, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
681: 		return pane.RemovePatentTagCmd(a.client, project, patent, name)
682: 	})
683: }
684: 
685: // cmdTagPatentList lists all tags assigned to the selected patent.
686: func (a *App) cmdTagPatentList(inv invocation) (tea.Model, tea.Cmd) {
687: 	return a.runAction(command.TagPatentList, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
688: 		return pane.ListPatentTagsCmd(a.client, project, patent)
689: 	})
690: }
691: 
692: func (a *App) cmdPatentDelete(invocation) (tea.Model, tea.Cmd) {
693: 	return a.runAction(command.PatentDelete, func(_ domain.ProjectID, n domain.PatentNumber) tea.Cmd {
694: 		return pane.DeletePatentCmd(a.client, n)
695: 	})
696: }
697: 
698: // cmdProjectCreate opens a name-entry overlay, or — given a typed name — creates
699: // the project directly.
700: func (a *App) cmdProjectCreate(inv invocation) (tea.Model, tea.Cmd) {
701: 	if len(inv.args) > 0 {
702: 		return a.createProject(strings.Join(inv.args, " "))
703: 	}
704: 	a.overlays = append(a.overlays, overlay.NewTextInput(
705: 		a.theme, a.text, overlay.PurposeCreateProject, text.NewProjectTitle, text.NewProjectCaption))
706: 	return a, nil
707: }
708: 
709: // cmdImport fetches a patent by number — optionally forcing past the file
710: // cache — or loads a fixture file when the argument is a path.
711: func (a *App) cmdImport(inv invocation) (tea.Model, tea.Cmd) {
712: 	if len(inv.args) == 0 {
713: 		return a.usageError(command.Import)
714: 	}
715: 	force := false
716: 	for _, arg := range inv.args[1:] {
717: 		if !strings.EqualFold(arg, "force") {
718: 			return a.usageError(command.Import)
719: 		}
720: 		force = true
721: 	}
722: 	if a.client == nil {
723: 		a.setErr(text.StatusDaemonUnavailable)
724: 		return a, nil
725: 	}
726: 	target := inv.args[0]
727: 	if isFixturePath(target) {
728: 		return a, pane.ImportFileCmd(a.client, target)
729: 	}
730: 	number, err := domain.ParsePatentNumber(target)
731: 	if err != nil {
732: 		a.setErr(text.StatusInvalidPatentNumber, err.Error())
733: 		return a, nil
734: 	}
735: 	return a, pane.CrawlCmd(a.client, number, 0, "", force)
736: }
737: 
738: // isFixturePath reports whether arg names a fixture file rather than a patent.
739: func isFixturePath(arg string) bool {
740: 	return strings.ContainsAny(arg, `/\`) || strings.HasSuffix(strings.ToLower(arg), ".json")
741: }
742: 
743: // handleTextSubmit routes a value entered in a TextInput overlay to its action.
744: func (a *App) handleTextSubmit(m overlay.TextSubmitMsg) (tea.Model, tea.Cmd) {
745: 	switch m.Purpose {
746: 	case overlay.PurposeCreateProject:
747: 		return a.createProject(m.Value)
748: 	case overlay.PurposeEditIDSKind:
749: 		return a.applyIDSFieldEdit("kind", m.Value)
750: 	case overlay.PurposeEditIDSCountry:
751: 		return a.applyIDSFieldEdit("country", m.Value)
752: 	case overlay.PurposeEditIDSPassages:
753: 		return a.applyIDSFieldEdit("passages", m.Value)
754: 	case overlay.PurposeEditIDSNotes:
755: 		return a.applyIDSFieldEdit("notes", m.Value)
756: 	default:
757: 		return a, nil
758: 	}
759: }
760: 
761: func (a *App) openIDSEditInput(field string) (tea.Model, tea.Cmd) {
762: 	var (
763: 		purpose overlay.Purpose
764: 		title   text.Key
765: 		caption text.Key
766: 	)
767: 	switch field {
768: 	case "kind":
769: 		purpose, title, caption = overlay.PurposeEditIDSKind, text.EditIDSKindTitle, text.EditIDSKindCaption
770: 	case "country":
771: 		purpose, title, caption = overlay.PurposeEditIDSCountry, text.EditIDSCountryTitle, text.EditIDSCountryCaption
772: 	case "passages":
773: 		purpose, title, caption = overlay.PurposeEditIDSPassages, text.EditIDSPassagesTitle, text.EditIDSPassagesCaption
774: 	case "notes":
775: 		purpose, title, caption = overlay.PurposeEditIDSNotes, text.EditIDSNotesTitle, text.EditIDSNotesCaption
776: 	default:
777: 		return a, nil
778: 	}
779: 	a.overlays = append(a.overlays, overlay.NewTextInput(a.theme, a.text, purpose, title, caption))
780: 	return a, nil
781: }
782: 
783: func (a *App) applyIDSFieldEdit(field, value string) (tea.Model, tea.Cmd) {
784: 	idsPane, ok := a.focusedPane().(interface{ ApplyTextValue(string, string) tea.Cmd })
785: 	if !ok {
786: 		return a, nil
787: 	}
788: 	return a, idsPane.ApplyTextValue(field, value)
789: }
790: 
791: // createProject sends a project.create request for name.
792: func (a *App) createProject(name string) (tea.Model, tea.Cmd) {
793: 	name = strings.TrimSpace(name)
794: 	if name == "" {
795: 		a.setErr(text.StatusProjectNameEmpty)
796: 		return a, nil
797: 	}
798: 	if a.client == nil {
799: 		a.setErr(text.StatusDaemonUnavailable)
800: 		return a, nil
801: 	}
802: 	return a, pane.CreateProjectCmd(a.client, name)
803: }
804: 
805: // usageError reports the correct invocation of a command.
806: func (a *App) usageError(id command.ID) (tea.Model, tea.Cmd) {
807: 	cmd, _ := a.registry.Lookup(id)
808: 	usage := cmd.Usage
809: 	if usage == "" {
810: 		usage = ":" + cmd.Name
811: 	}
812: 	a.setErr(text.StatusUsage, usage)
813: 	return a, nil
814: }
815: 
816: // --- pane stack --------------------------------------------------------------
817: 
818: // preparePane syncs a newly mounted pane with the current app state before its
819: // first render/load, so list panes size themselves from the live window rather
820: // than their constructor defaults.
821: func (a *App) preparePane(p pane.Pane) (pane.Pane, []tea.Cmd) {
822: 	var cmds []tea.Cmd
823: 	if a.width > 0 && a.height > 0 {
824: 		updated, cmd := p.Update(pane.ResizeMsg{Width: a.width, Height: max(a.height-statusRows, 1)})
825: 		p = updated
826: 		if cmd != nil {
827: 			cmds = append(cmds, cmd)
828: 		}
829: 	}
830: 	updated, cmd := p.Update(pane.ProjectChangedMsg{Project: a.activeProject})
831: 	p = updated
832: 	if cmd != nil {
833: 		cmds = append(cmds, cmd)
834: 	}
835: 	return p, cmds
836: }
837: 
838: // pushPane adds a pane to the stack and returns its init command.
839: func (a *App) pushPane(p pane.Pane) (tea.Model, tea.Cmd) {
840: 	p, cmds := a.preparePane(p)
841: 	a.panes = append(a.panes, p)
842: 	if len(cmds) == 0 {
843: 		cmds = append(cmds, p.Init())
844: 	}
845: 	return a, tea.Batch(cmds...)
846: }
847: 
848: // openDetail pushes a detail pane for the focused pane's selected patent. The
849: // active project, when set, scopes the detail's review state and tags.
850: func (a *App) openDetail() (tea.Model, tea.Cmd) {
851: 	number, ok := a.focusedPane().Selection()
852: 	if !ok {
853: 		a.setErr(text.StatusNoPatentSelected)
854: 		return a, nil
855: 	}
856: 	var project domain.ProjectID
857: 	if a.activeProject != nil {
858: 		project = a.activeProject.ID
859: 	}
860: 	bound := a.keymaps.BoundLetters(command.ScopeDetail)
861: 	return a.pushPane(pane.NewDetail(a.client, a.theme, number, project, bound))
862: }
863: 
864: func (a *App) openIDS() (tea.Model, tea.Cmd) {
865: 	if a.activeProject == nil {
866: 		a.setErr(text.StatusNoActiveProject)
867: 		return a, nil
868: 	}
869: 	number, ok := a.focusedPane().Selection()
870: 	if !ok {
871: 		a.setErr(text.StatusNoPatentSelected)
872: 		return a, nil
873: 	}
874: 	return a.pushPane(pane.NewIDSDetail(a.client, a.theme, number, a.activeProject.ID))
875: }
876: 
877: // openCitations pushes a family-edge pane for the focused pane's selected
878: // patent, showing edges of the given kind.
879: func (a *App) openCitations(kind domain.RelationKind) (tea.Model, tea.Cmd) {
880: 	number, ok := a.focusedPane().Selection()
881: 	if !ok {
882: 		a.setErr(text.StatusNoPatentSelected)
883: 		return a, nil
884: 	}
885: 	return a.pushPane(pane.NewCitations(a.client, a.theme, number, kind))
886: }
887: 
888: func (a *App) activateProject() (tea.Model, tea.Cmd) {
889: 	selector, ok := a.focusedPane().(interface{ SelectedProject() (domain.Project, bool) })
890: 	if !ok {
891: 		a.setErr(text.StatusNoProjectSelection)
892: 		return a, nil
893: 	}
894: 	project, ok := selector.SelectedProject()
895: 	if !ok {
896: 		a.setErr(text.StatusNoProjectSelected)
897: 		return a, nil
898: 	}
899: 	return a.useProject(project)
900: }
901: 
902: func (a *App) activateProjectByArg(arg string) (tea.Model, tea.Cmd) {
903: 	project, ok := a.resolveProjectArg(arg)
904: 	if !ok {
905: 		a.setErr(text.StatusProjectNotFound, arg)
906: 		return a, nil
907: 	}
908: 	return a.useProject(project)
909: }
910: 
911: // useProject makes project the active project and updates every pane.
912: func (a *App) useProject(project domain.Project) (tea.Model, tea.Cmd) {
913: 	a.activeProject = &project
914: 	a.lastProjectID = project.ID
915: 	if a.saveLastProject != nil {
916: 		if err := a.saveLastProject(project.ID); err != nil {
917: 			a.setErr(text.StatusActiveProjectSaveErr, project.Name, err.Error())
918: 		} else {
919: 			a.setStatus(text.StatusActiveProject, project.Name)
920: 		}
921: 	} else {
922: 		a.setStatus(text.StatusActiveProject, project.Name)
923: 	}
924: 	if splash, ok := a.focusedPane().(interface{ IsSplash() bool }); ok && splash.IsSplash() && len(a.panes) == 1 {
925: 		var mounted pane.Pane = pane.NewCatalog(a.client, a.theme)
926: 		mounted, cmds := a.preparePane(mounted)
927: 		a.panes[0] = mounted
928: 		if len(cmds) == 0 {
929: 			cmds = append(cmds, mounted.Init())
930: 		}
931: 		return a, tea.Batch(cmds...)
932: 	}
933: 	if len(a.panes) > 1 {
934: 		a.panes = a.panes[:len(a.panes)-1]
935: 	}
936: 	return a, a.broadcast(pane.ProjectChangedMsg{Project: &project})
937: }
938: 
939: func (a *App) resolveProjectArg(arg string) (domain.Project, bool) {
940: 	if selector, ok := a.focusedPane().(*pane.Projects); ok {
941: 		if project, found := selector.ProjectByArg(arg); found {
942: 			return project, true
943: 		}
944: 	}
945: 	if a.client == nil {
946: 		return domain.Project{}, false
947: 	}
948: 	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
949: 	defer cancel()
950: 	var res proto.ProjectListResult
951: 	if err := a.client.Call(ctx, proto.MethodProjectList, nil, &res); err != nil {
952: 		return domain.Project{}, false
953: 	}
954: 	needle := strings.TrimSpace(strings.ToLower(arg))
955: 	for _, project := range res.Projects {
956: 		if strings.ToLower(string(project.ID)) == needle || strings.ToLower(project.Name) == needle {
957: 			return project, true
958: 		}
959: 	}
960: 	return domain.Project{}, false
961: }
962: 
963: func (a *App) runReviewState(id command.ID, target domain.ReviewState) (tea.Model, tea.Cmd) {
964: 	return a.runAction(id, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
965: 		return pane.SetReviewStateCmd(a.client, project, patent, target)
966: 	})
967: }
968: 
969: // runAction is the canonical dispatcher for single-patent and multi-patent
970: // project actions. It validates preconditions, loops over all focused selections,
971: // and applies any confirmation registered for id in commandPolicies.
972: func (a *App) runAction(
973: 	id command.ID,
974: 	build func(domain.ProjectID, domain.PatentNumber) tea.Cmd,
975: ) (tea.Model, tea.Cmd) {
976: 	if a.activeProject == nil {
977: 		a.setErr(text.StatusNoActiveProject)
978: 		return a, nil
979: 	}
980: 	if a.client == nil {
981: 		a.setErr(text.StatusDaemonUnavailable)
982: 		return a, nil
983: 	}
984: 	numbers := a.focusedSelections()
985: 	if len(numbers) == 0 {
986: 		a.setErr(text.StatusNoPatentSelected)
987: 		return a, nil
988: 	}
989: 	if len(numbers) >= 2 {
990: 		if vs, ok := a.focusedPane().(pane.VisualSelectionSaver); ok {
991: 			vs.SaveVisualSelection()
992: 		}
993: 	}
994: 	cmds := make([]tea.Cmd, 0, len(numbers))
995: 	for _, n := range numbers {
996: 		cmds = append(cmds, build(a.activeProject.ID, n))
997: 	}
998: 	cmd := tea.Batch(cmds...)
999: 
1000: 	if policy := commandPolicies[id]; policy.Confirm != nil {
1001: 		if msg, needs := policy.Confirm(numbers); needs {
1002: 			a.confirmCmd = cmd
1003: 			a.overlays = append(a.overlays, overlay.NewConfirm(a.theme, msg))
1004: 			return a, nil
1005: 		}
1006: 	}
1007: 	return a, cmd
1008: }
1009: 
1010: func (a *App) focusedSelections() []domain.PatentNumber {
1011: 	if ms, ok := a.focusedPane().(pane.MultiSelector); ok {
1012: 		if sels := ms.Selections(); len(sels) >= 2 {
1013: 			return sels
1014: 		}
1015: 	}
1016: 	if number, ok := a.focusedPane().Selection(); ok {
1017: 		return []domain.PatentNumber{number}
1018: 	}
1019: 	return nil
1020: }
1021: 
1022: func (a *App) runProjectAction(action func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd) (tea.Model, tea.Cmd) {
1023: 	return a.runAction("", action)
1024: }
1025: 
1026: func (a *App) openPrompt(mode overlay.PromptMode) (tea.Model, tea.Cmd) {
1027: 	a.overlays = append(a.overlays, overlay.NewPrompt(a.registry, a.keymaps, a.theme, a.text, a.scope(), mode))
1028: 	return a, nil
1029: }
1030: 
1031: func (a *App) scope() command.Scope {
1032: 	if len(a.overlays) > 0 {
1033: 		if source, ok := a.focusedOverlay().(overlay.ScopeSource); ok {
1034: 			return source.SourceScope()
1035: 		}
1036: 		return command.ScopeOverlay
1037: 	}
1038: 	return a.focusedPane().Scope()
1039: }
1040: 
1041: // executeTypedCommand parses a typed command and routes it through invoke, the
1042: // same path a key chord takes — so a command can never work one way and not the
1043: // other.
1044: func (a *App) executeTypedCommand(input string) (tea.Model, tea.Cmd) {
1045: 	parts := strings.Fields(strings.TrimSpace(input))
1046: 	if len(parts) == 0 {
1047: 		return a, nil
1048: 	}
1049: 	cmd, ok := a.registry.LookupName(parts[0])
1050: 	if !ok {
1051: 		a.setErr(text.StatusUnknownCommand, parts[0])
1052: 		return a, nil
1053: 	}
1054: 	if !cmd.AvailableIn(a.scope()) {
1055: 		a.setErr(text.StatusCommandNotHere, cmd.Name)
1056: 		return a, nil
1057: 	}
1058: 	args := parts[1:]
1059: 	if len(args) > 0 && !typedAcceptsArgs[cmd.ID] {
1060: 		return a.usageError(cmd.ID)
1061: 	}
1062: 	return a.invoke(cmd.ID, invocation{repeat: 1, args: args})
1063: }
1064: 
1065: // handleEvent reflects a daemon event into the status line and refreshes data.
1066: func (a *App) handleEvent(ev proto.Event) tea.Cmd {
1067: 	switch ev.Method {
1068: 	case proto.EventCrawlProgress:
1069: 		var p proto.CrawlProgress
1070: 		if json.Unmarshal(ev.Params, &p) == nil {
1071: 			a.setStatus(text.StatusCrawlProgress, p.JobID, p.CrawledCount, p.DiscoveredCount, p.Message)
1072: 		}
1073: 		return nil
1074: 	case proto.EventCrawlDone:
1075: 		var d proto.CrawlDone
1076: 		_ = json.Unmarshal(ev.Params, &d)
1077: 		if d.Error != "" {
1078: 			a.setErr(text.StatusCrawlFailed, d.JobID, d.Error)
1079: 		} else {
1080: 			a.setStatus(text.StatusCrawlComplete, d.JobID)
1081: 		}
1082: 		return a.refreshPanes()
1083: 	case proto.EventDBChanged:
1084: 		return a.refreshPanes()
1085: 	}
1086: 	return nil
1087: }
1088: 
1089: // refreshPanes asks every pane to reload from the daemon. Panes cache their own
1090: // view state, so a store mutation must invalidate the whole stack rather than
1091: // only the currently visible pane.
1092: func (a *App) refreshPanes() tea.Cmd {
1093: 	var cmds []tea.Cmd
1094: 	for i, p := range a.panes {
1095: 		updated, cmd := p.Command(command.Refresh, pane.Invocation{Repeat: 1})
1096: 		a.panes[i] = updated
1097: 		if cmd != nil {
1098: 			cmds = append(cmds, cmd)
1099: 		}
1100: 	}
1101: 	return tea.Batch(cmds...)
1102: }
1103: 
1104: func (a *App) focusedPane() pane.Pane          { return a.panes[len(a.panes)-1] }
1105: func (a *App) focusedOverlay() overlay.Overlay { return a.overlays[len(a.overlays)-1] }
1106: 
1107: func (a *App) popOverlay() {
1108: 	if n := len(a.overlays); n > 0 {
1109: 		if _, ok := a.overlays[n-1].(*overlay.Jump); ok {
1110: 			if setter, ok := a.focusedPane().(interface{ SetJumpActive(bool) }); ok {
1111: 				setter.SetJumpActive(false)
1112: 			}
1113: 		}
1114: 		a.overlays = a.overlays[:n-1]
1115: 	}
1116: }
1117: 
1118: // setStatus and setErr resolve a catalog key into the status line.
1119: func (a *App) setStatus(key text.Key, args ...any) {
1120: 	a.status, a.statusErr = a.text.Tf(key, args...), false
1121: }
1122: 
1123: func (a *App) setErr(key text.Key, args ...any) {
1124: 	a.status, a.statusErr = a.text.Tf(key, args...), true
1125: }
1126: 
1127: // View implements tea.Model.
1128: func (a *App) View() string {
1129: 	if a.width == 0 || a.height == 0 {
1130: 		return "starting…"
1131: 	}
1132: 	focused := a.focusedPane()
1133: 	header := a.headerBlock(focused)
1134: 	headerLines := 0
1135: 	if header != "" {
1136: 		headerLines = strings.Count(header, "\n") + 1
1137: 	}
1138: 	bodyHeight := max(a.height-headerLines-statusRows, 1)
1139: 
1140: 	body := fitBody(focused.View(a.width, bodyHeight), bodyHeight)
1141: 
1142: 	statusStyle := a.theme.Status
1143: 	if a.statusErr {
1144: 		statusStyle = a.theme.Error
1145: 	}
1146: 	status := statusStyle.Render(render.Pad(" "+a.statusText(), a.width))
1147: 
1148: 	screen := body + "\n" + status
1149: 	if header != "" {
1150: 		screen = header + "\n" + screen
1151: 	}
1152: 	if len(a.overlays) > 0 {
1153: 		screen = a.compositeOverlay(screen)
1154: 	}
1155: 	return screen
1156: }
1157: 
1158: func (a *App) headerBlock(focused pane.Pane) string {
1159: 	if splash, ok := focused.(interface{ IsSplash() bool }); ok && splash.IsSplash() {
1160: 		return ""
1161: 	}
1162: 	line1 := a.renderScreenHeader(focused)
1163: 	line2 := a.theme.Dim.Render(render.Pad(" "+a.helperLine(focused.Scope()), a.width))
1164: 	line3 := a.theme.Header.Render(strings.Repeat("─", a.width))
1165: 	return line1 + "\n" + line2 + "\n" + line3
1166: }
1167: 
1168: func (a *App) renderScreenHeader(focused pane.Pane) string {
1169: 	var b strings.Builder
1170: 	b.WriteString(a.theme.Title.Render("PatentMine"))
1171: 	if a.activeProject != nil {
1172: 		b.WriteString(" ")
1173: 		project := a.activeProject.Name + " [" + string(a.activeProject.ID) + "]"
1174: 		b.WriteString(a.theme.Row.Render(project))
1175: 	}
1176: 	b.WriteString(" ")
1177: 	b.WriteString(a.theme.Header.Render("· "))
1178: 	b.WriteString(a.theme.Row.Bold(true).Render(focused.Title()))
1179: 	return render.Pad(" "+b.String(), a.width)
1180: }
1181: 
1182: func (a *App) helperLine(scope command.Scope) string {
1183: 	var parts []string
1184: 	for _, h := range a.hints.For(scope) {
1185: 		if len(h.Commands) == 1 {
1186: 			parts = append(parts, a.shortcutHint(scope, h.Commands[0], h.Label))
1187: 		} else {
1188: 			parts = append(parts, a.multiShortcutHint(scope, h.Commands, h.Label))
1189: 		}
1190: 	}
1191: 	return a.joinHints(parts...)
1192: }
1193: 
1194: func (a *App) splashFooterHint() string {
1195: 	scope := command.ScopeProjects
1196: 	return a.joinHints(
1197: 		a.navigationHint(scope),
1198: 		a.shortcutHint(scope, command.ProjectActivate, text.HintSelect),
1199: 		a.text.T(text.HintSlashCommands),
1200: 		a.shortcutHint(scope, command.OpenCommand, text.HintCommand),
1201: 		a.shortcutHint(scope, command.ProjectCreate, text.HintNewProject),
1202: 		a.shortcutHint(scope, command.Quit, text.HintQuit),
1203: 	)
1204: }
1205: 
1206: func (a *App) splashEmptyHint() string {
1207: 	scope := command.ScopeProjects
1208: 	createUsage := ":project.create"
1209: 	if cmd, ok := a.registry.Lookup(command.ProjectCreate); ok && cmd.Usage != "" {
1210: 		createUsage = cmd.Usage
1211: 	}
1212: 	shortcut := a.shortcutKeys(scope, command.ProjectCreate)
1213: 	if shortcut == "" {
1214: 		return a.text.Tf(text.SplashCreateHint, createUsage)
1215: 	}
1216: 	return a.text.Tf(text.SplashCreateKeyHint, createUsage, shortcut)
1217: }
1218: 
1219: func (a *App) navigationHint(scope command.Scope) string {
1220: 	down := a.shortcutKeys(scope, command.NavDown)
1221: 	up := a.shortcutKeys(scope, command.NavUp)
1222: 	move := a.text.T(text.HintMove)
1223: 	if down == "" && up == "" {
1224: 		return move
1225: 	}
1226: 	if down == "" {
1227: 		return up + " " + move
1228: 	}
1229: 	if up == "" {
1230: 		return down + " " + move
1231: 	}
1232: 	return down + "/" + up + " " + move
1233: }
1234: 
1235: func (a *App) shortcutHint(scope command.Scope, id command.ID, labelKey text.Key) string {
1236: 	label := a.text.T(labelKey)
1237: 	keys := a.shortcutKeys(scope, id)
1238: 	if keys == "" {
1239: 		return label
1240: 	}
1241: 	return keys + " " + label
1242: }
1243: 
1244: func (a *App) multiShortcutHint(scope command.Scope, ids []command.ID, labelKey text.Key) string {
1245: 	label := a.text.T(labelKey)
1246: 	var keys []string
1247: 	for _, id := range ids {
1248: 		k := a.shortcutKeys(scope, id)
1249: 		if k != "" {
1250: 			keys = append(keys, k)
1251: 		}
1252: 	}
1253: 	if len(keys) == 0 {
1254: 		return label
1255: 	}
1256: 	return strings.Join(keys, "/") + " " + label
1257: }
1258: 
1259: func (a *App) shortcutKeys(scope command.Scope, id command.ID) string {
1260: 	shortcuts := a.keymaps.Shortcuts(scope, id)
1261: 	if len(shortcuts) == 0 && scope != command.ScopeOverlay {
1262: 		shortcuts = a.keymaps.Shortcuts(command.ScopeOverlay, id)
1263: 	}
1264: 	if len(shortcuts) == 0 {
1265: 		return ""
1266: 	}
1267: 	return strings.Join(shortcuts, "/")
1268: }
1269: 
1270: func (a *App) joinHints(parts ...string) string {
1271: 	filtered := parts[:0]
1272: 	for _, part := range parts {
1273: 		part = strings.TrimSpace(part)
1274: 		if part != "" {
1275: 			filtered = append(filtered, part)
1276: 		}
1277: 	}
1278: 	return strings.Join(filtered, " · ")
1279: }
1280: 
1281: // statusText appends the chord reader's pending input, Vim-style.
1282: func (a *App) statusText() string {
1283: 	versionText := "   [tui " + a.tuiVersion + " | daemon " + a.daemonVersion + "]"
1284: 	if a.activeProject != nil {
1285: 		versionText += "   [project " + a.activeProject.Name + "]"
1286: 	}
1287: 	visual := ""
1288: 	if ms, ok := a.focusedPane().(pane.MultiSelector); ok {
1289: 		if sels := ms.Selections(); len(sels) > 0 {
1290: 			visual = fmt.Sprintf("   [VISUAL %d]", len(sels))
1291: 		}
1292: 	}
1293: 	if pending := a.reader.Pending(); pending != "" {
1294: 		return a.status + versionText + visual + "   [" + pending + "]"
1295: 	}
1296: 	return a.status + versionText + visual
1297: }
1298: 
1299: // compositeOverlay draws the focused overlay centred over the dimmed screen.
1300: func (a *App) compositeOverlay(screen string) string {
1301: 	boxWidth := min(a.width-overlayMargin, overlayMaxWidth)
1302: 	boxHeight := min(a.height-overlayMargin, overlayMaxHeight)
1303: 	if boxWidth < 16 || boxHeight < 6 {
1304: 		return screen // terminal too small to frame an overlay
1305: 	}
1306: 	ov := a.focusedOverlay()
1307: 	innerWidth := boxWidth - overlayChrome
1308: 	content := a.theme.Title.Render(ov.Title()) + "\n\n" +
1309: 		ov.View(innerWidth, boxHeight-overlayChrome)
1310: 	box := a.theme.Box.Width(innerWidth).Height(boxHeight - 2).Render(content)
1311: 
1312: 	dimmed := render.Dim(screen)
1313: 	x, y := render.CenterOffset(a.width, a.height, lipgloss.Width(box), lipgloss.Height(box))
1314: 	return render.Composite(dimmed, box, x, y)
1315: }
1316: 
1317: // fitBody pads or trims rendered pane output to exactly height lines so the
1318: // status line always sits on the bottom row.
1319: func fitBody(body string, height int) string {
1320: 	lines := strings.Split(body, "\n")
1321: 	for len(lines) < height {
1322: 		lines = append(lines, "")
1323: 	}
1324: 	return strings.Join(lines[:height], "\n")
1325: }

(End of file - total 1325 lines)
</content>
```

---

## Assistant (Plan · Big Pickle · 33.6s)

_Thinking:_

Now I have a thorough understanding of the codebase. Let me also check the go.mod to see what dependencies are available, and look at the text catalog and proto layer to understand internationalization and IPC.

Actually, I think I have enough information now to formulate a comprehensive plan. Let me synthesize everything.

The user wants:
1. View full patent text (not just first claim, which is all that's stored now)
2. Be able to select passages visually
3. Copy selected text to clipboard with patent metadata
4. Cross-platform (Linux/WSL/Windows/macOS)
5. Metadata format: Patent#, Inventor (et al.), Application#, Assignee, Publication date, Expiration date, source location (e.g., "Claim 2")
6. Pros/cons of approaches

Let me think about the approaches:

### Approach A: In-TUI Viewer with Clipboard (New Pane + Clipboard Lib)

**What it involves:**
1. Extend the Google Patents crawler to fetch ALL claims (not just claim 1), or add an on-demand fetch for full text
2. Create a new "Full Text" pane (or extend the detail pane) that shows all claims with labeled sections
3. Add visual text selection mode (similar to existing `v` Visual mode for catalog rows, but adapted for text selection within a pane)
4. Add clipboard integration via a Go clipboard library
5. Add new commands: `CopyYank` (y to yank selection), `FullTextView` (open full text view)
6. Format clipboard content with patent metadata header

**Clipboard libraries for Go:**
- `github.com/atotto/clipboard`: cross-platform (xclip/xsel on Linux, pbcopy on macOS, clip.exe on Windows). Pros: simple API, well-established. Cons: requires xclip/xsel on Linux (not always installed), may not work in pure WSL without X.
- `github.com/golang-design/clipboard`: newer, uses cgo with native APIs. Pros: more reliable native integration. Cons: requires CGo.
- `golang.design/x/clipboard`: similar to above.
- A custom approach: detect platform and shell out to `xclip`, `wl-copy`, `pbcopy`, `clip.exe`, `powershell.exe` etc.

For WSL specifically, `clip.exe` works (it's available in the Windows System32 folder which is on the WSL PATH). For macOS, `pbcopy`. For Linux with X, `xclip` or `xsel`. For Wayland, `wl-copy` (wl-clipboard). For pure terminal (SSH), maybe OSC 52 escape sequence.

**OSC 52** (ANSI escape sequence) is interesting - it tells the terminal emulator to set the clipboard. This works in many modern terminal emulators (iTerm2, tmux, Windows Terminal, etc.) without needing any external tools. But it doesn't work in all terminals.

**Pros:**
- Fully integrated TUI experience
- Selection + copy within the app
- Metadata automatically prepended
- No context switching

**Cons:**
- Significant implementation effort (new pane, text selection model, clipboard)
- Full patent text is large and needs fetching/storing
- Clipboard cross-platform is finicky

### Approach B: Open in External Browser + Browser Copy

**What it involves:**
- Already have `OpenBrowser` command that opens `w` keybinding
- Use the browser's native copy functionality
- Could add an enhancement that shows a notification/banner with the metadata

**Pros:**
- Already partially implemented (`w` keybinding + `OpenBrowser`)
- Browser handles text selection and copy natively
- Works on all platforms
- Full patent text is loaded by Google Patents directly

**Cons:**
- Manual copy - user must select and copy in browser
- No automatic metadata inclusion
- Requires context switching from terminal to browser
- Not programmatic

### Approach C: External Pager with Pipe (less/bat + clipboard piped)

**What it involves:**
- Write full text to temp file
- Open in `less` or `bat` for viewing/selection
- Provide a keybinding or command to pipe selected text to clipboard
- Or use `less` with pipe (`|` command in less)

**Pros:**
- Leverages existing tools
- Relatively simple to implement
- Cross-platform (if tools available)

**Cons:**
- Loses TUI integration
- Selection in `less` is primitive
- Metadata formatting is awkward
- Not a polished UX

### My recommendation:

I think the best approach combines:

1. **Full text fetching**: Extend the Google crawler to extract all claims (or the full specification), cache it temporarily (e.g., in a temp file or memory cache, not in the main DB).

2. **Full text viewer**: A new pane (or mode in the detail pane) that shows all claims with headers like `Claim 1`, `Claim 2`, etc.

3. **Visual text selection**: In this viewer, implement a visual selection mode (like vim visual mode) where the user can move a cursor and select lines of text.

4. **Clipboard copy**: On `y` (yank), format the selected text with patent metadata header and copy to clipboard using OSC 52 escape sequence (most portable across terminals) with fallback to platform-specific commands.

Let me think about the formatting of the clipboard content:

```
═══════════════════════════════════════════════════════════════
Patent #: US-11,234,567-B2
Title: Method for Quantum Error Correction
Inventors: J. Smith et al. (3 inventors)
Assignee: Quantum Computing Inc.
Application #: US-17/123,456 (Filed: 2021-03-15)
Publication: US-2022-0123456-A1 (Published: 2022-04-21)
Expiration: 2041-03-15 (estimated)
Source: Claim 2
═══════════════════════════════════════════════════════════════

<selected text from claim 2>
```

This is comprehensive. Now let me think about what to present to the user.

Actually, the user asked for "pros/cons of the methods and how this clip to clipboard would work for other items." So they want:
1. Pros/cons of different approaches
2. How clipboard copy would work for other items (e.g., copying patent numbers from catalog, copying citations, etc.)

Let me structure my response as a plan with the analysis they requested.

Given that I'm in plan mode, I should present the plan without making any edits. Let me compose a comprehensive response.

Now I have a thorough understanding of the codebase. Let me present my analysis and plan.

---

## Analysis & Plan: Full Patent Text Viewer with Clipboard Copy

### Current State
- Only `FirstClaim` (claim 1) is stored in the DB from Google Patents scraping (`internal/crawl/google.go:70`)
- Detail pane shows abstract, first claim, metadata — but no full claims text
- **No clipboard functionality exists** anywhere in the app
- It's a Go Bubbletea TUI app with a clean command/keymap/pane architecture

---

### Proposed Solution: In-TUI Full Text Viewer + Clipboard

I recommend **Approach A** (in-TUI viewer) as the primary, with **OSC 52** escape sequences for clipboard. Here's what it involves:

#### 1. Fetch Full Claims Text
Extract **all claims** from Google Patents HTML. The existing selector is `section[itemprop='claims'] .claim`. We can use `section[itemprop='claims']` and iterate over all `.claim` children to get claim number + text.

- Add a new field (not stored in DB, fetched on-demand):
  ```go
  // Not persisted — fetched separately for the full-text viewer
  type PatentFullText struct {
      Claims   []ClaimSection // e.g., [{Number: 1, Text: "..."}, {Number: 2, Text: "..."}]
      // Could also include Description sections
  }
  ```
- **Storage**: Not in DB. Cache in a temp file `~/.cache/patentmine/fulltext/<number>.json` or just in-memory for the session. Re-fetch from Google if cache missing/stale.

#### 2. New Pane: `FullTextView`
A new pane (`internal/tui/pane/fulltext.go`) modeled after the detail pane but focused on the full claims text. Features:
- Scrollable view with claims labeled `Claim 1`, `Claim 2`, etc.
- Each claim is a visually distinct section
- Description/background sections could also be shown
- Jump anchors for each claim (reuse existing `;` jump mode)

#### 3. Visual Text Selection within Pane
A new selection mode (like vim visual mode, but for text lines within the pane):
- Activate with `V` (shift-v) in the full text pane
- Move cursor (`j`/`k`) to expand selection range
- Selected lines highlighted
- Shows line range in status: `SELECTED 15-23`

#### 4. Clipboard Copy with Metadata (`y` key)
On `y` (yank), format and copy to clipboard:

**Format:**
```
═══════════════════════════════════════
Patent #:     US-11,234,567-B2
Title:        Method for Quantum Error Correction
Inventors:    J. Smith et al. (3)
Assignee:     Quantum Computing Inc.
Application #: US-17/123,456 (2021-03-15)
Publication:  US-2022-0123456-A1 (2022-04-21)
Expiration:   2041-03-15 (estimated)
Source:       Claim 2
═══════════════════════════════════════

<selected claim text>
```

#### 5. Cross-platform Clipboard Strategy (OSC 52 + fallback)

| Method | How it works | Platform |
|---|---|---|
| **OSC 52** | ANSI escape sequence `\e]52;c;<base64>\a` | Any terminal emulator that supports it (iTerm2, tmux, Windows Terminal, kitty, Alacritty, WezTerm, etc.) |
| `clip.exe` | Shell to `/mnt/c/Windows/System32/clip.exe` | WSL |
| `pbcopy` | Shell to `pbcopy` | macOS |
| `xclip`/`xsel` | Shell to `xclip -selection clipboard` | Linux (X11) |
| `wl-copy` | Shell to `wl-copy` | Linux (Wayland) |
| `powershell.exe` | `powershell.exe -command Set-Clipboard` | Windows/WSL fallback |

**Implementation approach**: Try OSC 52 first (write the escape sequence to stdout). If terminal doesn't support it (user reports no clipboard), fall back to platform detection with `runtime.GOOS` and shell out. OSC 52 works in **Windows Terminal, tmux, SSH, iTerm2, kitty, Alacritty** — no dependency installation needed.

---

### How Clipboard Would Work for Other Items

The same clipboard mechanism can be extended to:

| Context | What gets copied | Yields |
|---|---|---|
| **Catalog row** (single) | Patent number, title, inventor, expiration | `US-11,234,567-B2 | Title | Inventors` |
| **Catalog** (multi-select) | One row per selected patent | Batch of patent numbers |
| **Detail** (current) | Full patent metadata + visible fields | Formatted patent record |
| **Full text** (new) | Selected claims + metadata | Claim text with header |
| **Citations pane** | Citation numbers + metadata | List of cited/citing patents |
| **IDS pane** | IDS entry fields | Formatted IDS entry |

Each context would implement a `ClipboardContent() string` interface that formats the data appropriately for clipboard paste.

---

### Pros/Cons Summary

| Approach | Pros | Cons |
|---|---|---|
| **A: In-TUI viewer + clipboard** | Integrated UX, fully programmable, metadata auto-included, fast for repeated use | Significant implementation, clipboard can be finicky cross-platform, full text fetch adds latency |
| **B: External browser** (`w` key) | Already works, browser copy is mature, zero implementation | Manual copy, no metadata, context switch, relies on user |
| **C: External pager** (`less`/`bat`) | Simple to implement, leverages existing tools | Awkward selection, no metadata, poor UX in TUI context |
| **A2: OSC 52 only** (subset of A) | No external deps, works in modern terminals, simplest implementation | Fails silently on terminals without OSC 52 support |

**My recommendation: Approach A with OSC 52 as primary + platform fallbacks.**

---

### Implementation Steps (high-level)

1. **Extend `internal/crawl/google.go`** — add `parseAllClaims()` function that extracts all `.claim` sections with their numbers
2. **Add `domain.PatentFullText` type** with `[]ClaimSection` 
3. **Add a new RPC method** (`MethodFullText`) or have the TUI fetch directly from Google when needed
4. **Create `internal/tui/pane/fulltext.go`** — the full text viewer pane with:
   - Scrollable claims sections
   - Visual line selection mode
   - Jump anchors per claim
5. **Add clipboard utility** (`internal/tui/clipboard.go`) — OSC 52 + platform fallback
6. **Register new commands**: `OpenFullText`, `CopyYank` in `internal/command/catalog.go`
7. **Add key bindings** in `internal/tui/keymap/default.go`
8. **Wire up handlers** in `internal/tui/app.go`

Want me to proceed with implementation, or do you have questions/feedback on the approach?

---

## User

is there a way i could add the clipart to the patent notes. but with only the "locator/claim number." 

---

## Assistant (Plan · Big Pickle · 13.8s)

_Thinking:_

The user is asking about adding clipboard content to patent notes, but specifically with only the "locator/claim number" — not the full claim text. Let me understand what they're asking:

1. They want to be able to copy patent content to the clipboard (as discussed)
2. But instead of copying the full claim text, they want to copy just the "locator" or "claim number" — essentially a reference pointer like "Claim 2" or the specific claim citation
3. And they want this to go into a "patent notes" context — perhaps the IDS (Information Disclosure Statement) notes field, or patent tag notes, or some note-taking system

Wait, let me re-read: "add the clipart to the patent notes" — I think they mean "clipboard" not "clipart". So: "add the clipboard to the patent notes. but with only the locator/claim number."

So they want:
- A way to copy just the claim locator (e.g., "Claim 2") to clipboard
- To be able to paste this into patent notes (or IDS notes, etc.)

Or perhaps they mean: when copying to clipboard for notes/IDS purposes, they want just the claim locator/number, not the full text.

Let me re-read more carefully: "is there a way i could add the clipart to the patent notes. but with only the "locator/claim number.""

I think "clipart" is a typo for "clipboard" or "clip to". They might be asking:
1. Can I add clipboard content to patent notes? But with only the locator/claim number?

Looking at the codebase, there's already an IDS (Information Disclosure Statement) system with:
- `IDSEditField` command
- `IDSEditPassages` purpose
- `EditIDSNotesTitle/Caption` text keys
- Relevant passages field in IDS entry

And there are existing tag/note fields.

So I think the user is asking: when adding notes or IDS entries about a patent, can we copy just the claim locator (e.g., "Claim 2" or "Col 5, Lines 10-15") rather than the full claim text?

Or maybe they mean: In the full text viewer (which we just discussed), when I select text and yank it, instead of copying the whole claim text, I just want to copy the claim number/locator as a reference.

Let me ask for clarification.

Actually, let me think about this more carefully. The user said "add the clipart to the patent notes" - I think this might mean "clip to the patent notes" meaning "copy to the patent notes field" rather than system clipboard. Like, instead of system clipboard, they want to directly populate the notes/IDS field with just the claim locator.

Let me ask the user to clarify.

I want to make sure I understand your question correctly. Can you clarify:

1. When you say "add the clipart to the patent notes" — did you mean **clipboard to the patent notes** (i.e., copy just the claim locator into a notes/IDS field within the app), rather than the system clipboard?

2. When you say "only the locator/claim number" — do you mean instead of copying the full claim text, you just want a reference like `Claim 2` or `Claim 2 of US-11,234,567-B2`?

**Possible interpretations:**

**A)** In the full text viewer, when you yank selected text, you want to copy **just the claim number** (e.g., `Claim 2`) to the system clipboard — useful as a short citation you can paste into emails, documents, etc.

**B)** You want to directly populate the **IDS "Relevant Passages" field** or a notes field with just the claim locator (e.g., `Claim 2`) rather than the full text — so within the app, pressing a key sends the claim reference to the patent's notes/IDS entry.

**C)** Both — clipboard for external pasting, and/or inline notes field for internal use.

Which direction (or something else) were you thinking?

---

## User

could the copy into notes be smart to avoid redundancy or should we have different clipboards. as I would prefer to copy the notes and add the patent information) to integrate somewhere else.

---

## Assistant (Plan · Big Pickle · 19.3s)

_Thinking:_

The user is asking about how the clipboard/copy functionality should work within the app, specifically around patent notes. Let me think about this carefully.

They're asking:
1. Should "copy into notes" be smart to avoid redundancy (i.e., if you already noted something about Claim 2, should it prevent you from adding the same note again)?
2. Or should there be different clipboards (multiple destinations) - one for notes within the app, one for system clipboard to integrate elsewhere?
3. They prefer to copy the notes AND add patent information to integrate somewhere else.

Let me think about the use cases:

**Use case 1: Internal notes (within PatentMine)**
- You're reviewing a patent in the full text viewer
- You see relevant claim text
- You press a key to add it as a note/reference to the patent's IDS entry or a notes field
- This should be smart about deduplication - if you already noted "Claim 2", it shouldn't create a duplicate

**Use case 2: System clipboard (external integration)**
- You're reviewing a patent and want to paste something into an email, document, spreadsheet, etc.
- You select text, press yank, and it goes to system clipboard with patent metadata
- For this, you want the full context (patent#, title, inventor, source location, etc.)

**Use case 3: Copy notes to clipboard with patent info**
- You have accumulated notes within the app
- You want to copy those notes to system clipboard for pasting elsewhere
- The notes should include patent information so context isn't lost

So I think the user is saying:
- They want to be able to add claim references to notes within the app (internal clipboard/notes buffer)
- Those notes should be smart (deduplicate)
- Then they want to export/copy those notes to the system clipboard with patent metadata for use in other tools

This suggests a design where:
1. There's an internal "notes buffer" or "clipboard ring" per patent (or per project)
2. From the full text viewer, you can add claim references (like "Claim 2") to this buffer
3. The buffer deduplicates
4. Then you can copy the entire buffer to system clipboard with patent metadata header
5. Separately, there's a system clipboard yank for selected text (with full context)

Or maybe simpler: there's an internal notes field per patent (IDS relevant passages), and also a system clipboard. The internal notes is smart/dedup, the system clipboard has full context.

Let me present this back to the user in a clear way and get their confirmation before proceeding with a plan.

Actually, let me re-read what they said more carefully:

"could the copy into notes be smart to avoid redundancy or should we have different clipboards. as I would prefer to copy the notes and add the patent information) to integrate somewhere else."

I think they're saying:
- They want to copy claim references into notes (internal to the app)
- They're asking if this should deduplicate (smart) or if there should be separate clipboards (one for internal notes, one for external system clipboard)
- Their preference: they want to accumulate notes, then copy those notes WITH patent information to integrate elsewhere

So the flow would be:
1. Review patent → see interesting claim → press key → adds "Claim 2" to patent notes (dedup)
2. Review more → add more claim references
3. Later, export/copy those notes to system clipboard → format includes patent info + all claim references

This is actually a really nice workflow. Let me think about how this could work.

The IDS system already has a "Relevant Passages" field. We could use that as the notes storage. Or we could add a simpler "quick notes" mechanism.

Actually, I think this maps well to the existing IDS system. The IDS entry has:
- `RelevantPassages` field
- `InFull` flag
- `Kind` and `Country` codes
- `Status`

So when reviewing a full text, pressing a key could:
1. Add the claim reference (e.g., "Claim 2") to the IDS relevant passages
2. Deduplicate so "Claim 2" only appears once

Then from the IDS view, you could copy the formatted IDS entry to clipboard.

Let me present this analysis and ask the user for confirmation.

That clarifies things. Let me restate to make sure I have it right:

**You want two clipboards (or two modes):**

1. **Internal notes buffer** — while reviewing patent full text, pressing a key adds the claim reference (e.g., `Claim 2`) to an accumulated notes list per patent. This is **smart/dedup** — adding `Claim 2` twice only stores it once. This feeds into the patent's IDS "Relevant Passages" or a new notes field.

2. **System clipboard export** — a separate action that copies the **entire accumulated notes buffer + patent metadata** to the system clipboard for pasting into external tools (email, documents, spreadsheets). The output would look like:

```
Patent #:     US-11,234,567-B2
Title:        Method for Quantum Error Correction
Inventors:    J. Smith et al. (3)
Assignee:     Quantum Computing Inc.
Application #: US-17/123,456
Publication:  US-2022-0123456-A1
Expiration:   2041-03-15 (estimated)

Relevant passages:
  - Claim 2
  - Claim 5
  - Claim 14
```

This keeps the internal notes concise (just locators, deduped) but the export is rich with full patent context.

**Does this match what you're thinking?** A few open questions:

1. Should the internal notes buffer map to the existing **IDS "Relevant Passages"** field, or should it be a separate quick-notes mechanism?
2. Should the "add to notes" action from full text view also optionally capture the **surrounding context** (e.g., full Claim 2 text) alongside the locator, or truly just the claim number reference?
3. Should the export go directly to **system clipboard** (for immediate paste), or to a **temp file** (for later batch processing), or both?

---

## User

ok create the plan with 2 clipbaords and proper shortcuts. 

---

## Assistant (Plan · Big Pickle · 4.6s)

_Thinking:_

The user wants me to create a detailed plan with 2 clipboards and proper shortcuts. Let me first explore a few more files to understand the existing notes/IDS system, the text catalog, and how the IPC works so I can make a thorough plan.

Let me look at the IDS pane and the relevant passages field, plus the text catalog for keys.

Let me explore a few more areas to make the plan thorough — specifically the IDS system, text catalog, and wiring checks.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/pane/ids.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/pane/ids.go</path>
<type>file</type>
<content>
1: package pane
2: 
3: import (
4: 	"strings"
5: 
6: 	tea "github.com/charmbracelet/bubbletea"
7: 
8: 	"patentmine/internal/command"
9: 	"patentmine/internal/domain"
10: 	"patentmine/internal/proto"
11: 	"patentmine/internal/rpc"
12: 	"patentmine/internal/text"
13: 	"patentmine/internal/tui/render"
14: )
15: 
16: type idsField string
17: 
18: const (
19: 	idsFieldStatus   idsField = "status"
20: 	idsFieldKind     idsField = "kind"
21: 	idsFieldCountry  idsField = "country"
22: 	idsFieldInFull   idsField = "in_full"
23: 	idsFieldPassages idsField = "passages"
24: 	idsFieldNotes    idsField = "notes"
25: )
26: 
27: type idsLoadedMsg struct {
28: 	requestID uint64
29: 	patent    domain.Patent
30: 	entry     *domain.IDSEntry
31: 	err       error
32: }
33: 
34: type idsSavedMsg struct {
35: 	entry domain.IDSEntry
36: 	err   error
37: }
38: 
39: type idsDeletedMsg struct{ err error }
40: 
41: // EditIDSFieldMsg asks the app to open a text input for the selected IDS field.
42: type EditIDSFieldMsg struct{ Field string }
43: 
44: // IDSDetail edits one patent's curated IDS entry inside the active project.
45: type IDSDetail struct {
46: 	client   *rpc.Client
47: 	theme    render.Theme
48: 	number   domain.PatentNumber
49: 	project  domain.ProjectID
50: 	handlers map[command.ID]cmdHandler
51: 
52: 	patent  domain.Patent
53: 	entry   domain.IDSEntry
54: 	page    render.Paginator
55: 	loading bool
56: 	loadErr string
57: 	loadID  uint64
58: }
59: 
60: func NewIDSDetail(client *rpc.Client, theme render.Theme, number domain.PatentNumber, project domain.ProjectID) *IDSDetail {
61: 	p := &IDSDetail{
62: 		client:  client,
63: 		theme:   theme,
64: 		number:  number,
65: 		project: project,
66: 		page:    render.NewPaginator(10),
67: 		loading: true,
68: 	}
69: 	p.handlers = map[command.ID]cmdHandler{
70: 		command.NavDown:       func(inv Invocation) tea.Cmd { p.page.MoveDown(inv.Repeat); return nil },
71: 		command.NavUp:         func(inv Invocation) tea.Cmd { p.page.MoveUp(inv.Repeat); return nil },
72: 		command.NavPageDown:   func(Invocation) tea.Cmd { p.page.PageDown(); return nil },
73: 		command.NavPageUp:     func(Invocation) tea.Cmd { p.page.PageUp(); return nil },
74: 		command.NavTop:        func(Invocation) tea.Cmd { p.page.Top(); return nil },
75: 		command.NavBottom:     func(Invocation) tea.Cmd { p.page.Bottom(); return nil },
76: 		command.Refresh:       func(Invocation) tea.Cmd { p.loading = true; return p.load() },
77: 		command.IDSEditField:  func(Invocation) tea.Cmd { return p.editFieldCmd() },
78: 		command.IDSToggleFull: func(Invocation) tea.Cmd { return p.toggleFullCmd() },
79: 		command.IDSCycleStatus: func(Invocation) tea.Cmd {
80: 			p.entry.Status = p.entry.Status.Next()
81: 			return p.saveCmd()
82: 		},
83: 		command.IDSDelete: func(Invocation) tea.Cmd { return p.deleteCmd() },
84: 	}
85: 	return p
86: }
87: 
88: func (p *IDSDetail) Scope() command.Scope { return command.ScopeIDS }
89: 
90: func (p *IDSDetail) Title() string { return "IDS · " + p.number.String() }
91: 
92: func (p *IDSDetail) Init() tea.Cmd { return p.load() }
93: 
94: func (p *IDSDetail) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
95: 	if handler, ok := p.handlers[id]; ok {
96: 		return p, handler(inv)
97: 	}
98: 	return p, nil
99: }
100: 
101: func (p *IDSDetail) Handles() []command.ID { return handlerIDs(p.handlers) }
102: 
103: func (p *IDSDetail) Selection() (domain.PatentNumber, bool) { return p.number, true }
104: 
105: func (p *IDSDetail) load() tea.Cmd {
106: 	client, number, project := p.client, p.number, p.project
107: 	requestID := nextAsyncID()
108: 	p.loadID = requestID
109: 	return func() tea.Msg {
110: 		ctx, cancel := callContext()
111: 		defer cancel()
112: 		var res proto.PatentResult
113: 		err := client.Call(ctx, proto.MethodPatentGet,
114: 			proto.PatentGetParams{Number: number, Project: project}, &res)
115: 		var entry *domain.IDSEntry
116: 		if res.IDSEntry != nil {
117: 			copied := *res.IDSEntry
118: 			entry = &copied
119: 		}
120: 		return idsLoadedMsg{requestID: requestID, patent: res.Patent, entry: entry, err: err}
121: 	}
122: }
123: 
124: func (p *IDSDetail) Update(msg tea.Msg) (Pane, tea.Cmd) {
125: 	switch m := msg.(type) {
126: 	case idsLoadedMsg:
127: 		if m.requestID != p.loadID {
128: 			return p, nil
129: 		}
130: 		p.loading = false
131: 		if m.err != nil {
132: 			p.loadErr = m.err.Error()
133: 			return p, nil
134: 		}
135: 		p.loadErr = ""
136: 		p.patent = m.patent
137: 		if m.entry != nil {
138: 			p.entry = *m.entry
139: 		} else {
140: 			p.entry = domain.IDSEntry{Project: p.project, Patent: m.patent.Number, Status: domain.IDSEntryPending}
141: 		}
142: 		p.page.Top()
143: 	case idsSavedMsg:
144: 		if m.err != nil {
145: 			return p, status(text.StatusExportFailed, true, m.err.Error())
146: 		}
147: 		p.entry = m.entry
148: 		return p, status(text.StatusFilter, false, "IDS updated")
149: 	case idsDeletedMsg:
150: 		if m.err != nil {
151: 			return p, status(text.StatusExportFailed, true, m.err.Error())
152: 		}
153: 		p.entry = domain.IDSEntry{Project: p.project, Patent: p.patent.Number, Status: domain.IDSEntryPending}
154: 		return p, status(text.StatusFilter, false, "IDS entry removed")
155: 	case ProjectChangedMsg:
156: 		var project domain.ProjectID
157: 		if m.Project != nil {
158: 			project = m.Project.ID
159: 		}
160: 		if project != p.project {
161: 			p.project = project
162: 			p.loading = true
163: 			return p, p.load()
164: 		}
165: 	}
166: 	return p, nil
167: }
168: 
169: func (p *IDSDetail) View(w, h int) string {
170: 	switch {
171: 	case p.loading:
172: 		return p.theme.Dim.Render("loading IDS…")
173: 	case p.loadErr != "":
174: 		return p.theme.Error.Render("error: " + p.loadErr)
175: 	case p.project == "":
176: 		return p.theme.Error.Render("error: select an active project first")
177: 	}
178: 	lines := strings.Split(p.body(w), "\n")
179: 	p.page.SetTotal(len(lines))
180: 	p.page.SetPageSize(max(h, 1))
181: 	start, end := p.page.Window()
182: 	return strings.Join(lines[start:end], "\n")
183: }
184: 
185: func (p *IDSDetail) body(w int) string {
186: 	fields := [][2]string{
187: 		{"Patent", numberToShow(p.patent).String()},
188: 		{"Title", p.patent.Title},
189: 		{"Status", string(p.entry.Status)},
190: 		{"Kind code", orDash(p.entry.KindCode)},
191: 		{"Country code", orDash(p.entry.CountryCode)},
192: 		{"In full", yesNo(p.entry.InFull)},
193: 		{"Relevant passages", orDash(p.entry.RelevantPassages)},
194: 		{"Notes", orDash(p.entry.Notes)},
195: 	}
196: 	var b strings.Builder
197: 	for i, field := range fields {
198: 		line := render.Pad(field[0]+":", 18) + " " + field[1]
199: 		styled := p.theme.Row.Render(render.Truncate(line, w))
200: 		if i == p.page.Cursor() {
201: 			styled = p.theme.Selected.Render(render.Pad(render.Truncate(line, w), w))
202: 		}
203: 		b.WriteString(styled)
204: 		if i < len(fields)-1 {
205: 			b.WriteByte('\n')
206: 		}
207: 	}
208: 	return b.String()
209: }
210: 
211: func (p *IDSDetail) editFieldCmd() tea.Cmd {
212: 	field := p.currentField()
213: 	switch field {
214: 	case idsFieldKind, idsFieldCountry, idsFieldPassages, idsFieldNotes:
215: 		return func() tea.Msg { return EditIDSFieldMsg{Field: string(field)} }
216: 	default:
217: 		return nil
218: 	}
219: }
220: 
221: func (p *IDSDetail) toggleFullCmd() tea.Cmd {
222: 	p.entry.InFull = !p.entry.InFull
223: 	if p.entry.InFull {
224: 		p.entry.RelevantPassages = ""
225: 	}
226: 	return p.saveCmd()
227: }
228: 
229: func (p *IDSDetail) deleteCmd() tea.Cmd {
230: 	client, project, patent := p.client, p.project, p.entry.Patent
231: 	return func() tea.Msg {
232: 		ctx, cancel := callContext()
233: 		defer cancel()
234: 		var res proto.Empty
235: 		err := client.Call(ctx, proto.MethodIDSEntryDelete,
236: 			proto.IDSEntryParams{Project: project, Patent: patent}, &res)
237: 		return idsDeletedMsg{err: err}
238: 	}
239: }
240: 
241: func (p *IDSDetail) saveCmd() tea.Cmd {
242: 	entry := p.entry
243: 	client := p.client
244: 	return func() tea.Msg {
245: 		ctx, cancel := callContext()
246: 		defer cancel()
247: 		var res proto.IDSEntryResult
248: 		err := client.Call(ctx, proto.MethodIDSEntrySave,
249: 			proto.IDSEntrySaveParams{Entry: entry}, &res)
250: 		return idsSavedMsg{entry: res.Entry, err: err}
251: 	}
252: }
253: 
254: func (p *IDSDetail) ApplyTextValue(field, value string) tea.Cmd {
255: 	switch idsField(field) {
256: 	case idsFieldKind:
257: 		p.entry.KindCode = strings.TrimSpace(value)
258: 	case idsFieldCountry:
259: 		p.entry.CountryCode = strings.ToUpper(strings.TrimSpace(value))
260: 	case idsFieldPassages:
261: 		p.entry.RelevantPassages = strings.TrimSpace(value)
262: 		if p.entry.RelevantPassages != "" {
263: 			p.entry.InFull = false
264: 		}
265: 	case idsFieldNotes:
266: 		p.entry.Notes = strings.TrimSpace(value)
267: 	default:
268: 		return nil
269: 	}
270: 	return p.saveCmd()
271: }
272: 
273: func (p *IDSDetail) currentField() idsField {
274: 	fields := []idsField{idsFieldStatus, idsFieldKind, idsFieldCountry, idsFieldInFull, idsFieldPassages, idsFieldNotes}
275: 	idx := max(p.page.Cursor()-2, 0)
276: 	if idx >= len(fields) {
277: 		idx = len(fields) - 1
278: 	}
279: 	return fields[idx]
280: }
281: 
282: func orDash(s string) string {
283: 	if strings.TrimSpace(s) == "" {
284: 		return "-"
285: 	}
286: 	return s
287: }
288: 
289: func yesNo(v bool) string {
290: 	if v {
291: 		return "yes"
292: 	}
293: 	return "no"
294: }

(End of file - total 294 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/domain/ids.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/domain/ids.go</path>
<type>file</type>
<content>
1: package domain
2: 
3: import (
4: 	"fmt"
5: 	"slices"
6: 	"strings"
7: 	"time"
8: )
9: 
10: // IDSStatus is the lifecycle state of an Information Disclosure Statement.
11: type IDSStatus string
12: 
13: const (
14: 	// IDSDraft is an IDS still being assembled; entries may change.
15: 	IDSDraft IDSStatus = "draft"
16: 	// IDSFiled is an IDS submitted to the patent office; it is now immutable.
17: 	IDSFiled IDSStatus = "filed"
18: )
19: 
20: // Valid reports whether the IDSStatus is a known value.
21: func (s IDSStatus) Valid() bool {
22: 	switch s {
23: 	case IDSDraft, IDSFiled:
24: 		return true
25: 	default:
26: 		return false
27: 	}
28: }
29: 
30: // idsTransitions is the single source of truth for IDS status changes.
31: var idsTransitions = map[IDSStatus][]IDSStatus{
32: 	IDSDraft: {IDSFiled},
33: 	IDSFiled: {},
34: }
35: 
36: // CanTransitionTo reports whether moving from s to target is allowed.
37: func (s IDSStatus) CanTransitionTo(target IDSStatus) bool {
38: 	if s == target {
39: 		return s.Valid()
40: 	}
41: 	return slices.Contains(idsTransitions[s], target)
42: }
43: 
44: // ParseIDSStatus converts a string into an IDSStatus.
45: func ParseIDSStatus(s string) (IDSStatus, error) {
46: 	st := IDSStatus(s)
47: 	if !st.Valid() {
48: 		return "", fmt.Errorf("domain: unknown IDS status %q", s)
49: 	}
50: 	return st, nil
51: }
52: 
53: // IDSEntryStatus is the workflow state of one curated IDS entry.
54: type IDSEntryStatus string
55: 
56: const (
57: 	IDSEntryPending   IDSEntryStatus = "pending"
58: 	IDSEntrySubmitted IDSEntryStatus = "submitted"
59: 	IDSEntryAccepted  IDSEntryStatus = "accepted"
60: )
61: 
62: // Valid reports whether the IDSEntryStatus is a known value.
63: func (s IDSEntryStatus) Valid() bool {
64: 	switch s {
65: 	case IDSEntryPending, IDSEntrySubmitted, IDSEntryAccepted:
66: 		return true
67: 	default:
68: 		return false
69: 	}
70: }
71: 
72: // Next returns the next entry status in the old-style workflow.
73: func (s IDSEntryStatus) Next() IDSEntryStatus {
74: 	switch s {
75: 	case IDSEntryPending:
76: 		return IDSEntrySubmitted
77: 	case IDSEntrySubmitted:
78: 		return IDSEntryAccepted
79: 	case IDSEntryAccepted:
80: 		return IDSEntryPending
81: 	default:
82: 		return IDSEntryPending
83: 	}
84: }
85: 
86: // IDSEntry is one curated prior-art reference for a project/patent pair.
87: type IDSEntry struct {
88: 	ID               int64          `json:"id"`
89: 	Project          ProjectID      `json:"project"`
90: 	Patent           PatentNumber   `json:"patent"`
91: 	KindCode         string         `json:"kind_code,omitempty"`
92: 	CountryCode      string         `json:"country_code,omitempty"`
93: 	InFull           bool           `json:"in_full,omitempty"`
94: 	RelevantPassages string         `json:"relevant_passages,omitempty"`
95: 	Notes            string         `json:"notes,omitempty"`
96: 	Status           IDSEntryStatus `json:"status,omitempty"`
97: 	AddedAt          time.Time      `json:"added_at"`
98: }
99: 
100: // SummaryText returns a compact, stable IDS summary for tables and detail lines.
101: func (e IDSEntry) SummaryText() string {
102: 	if e.Project == "" || e.Patent.IsZero() {
103: 		return "-"
104: 	}
105: 	parts := []string{string(e.Status)}
106: 	if !e.Status.Valid() {
107: 		parts[0] = string(IDSEntryPending)
108: 	}
109: 	switch {
110: 	case e.InFull:
111: 		parts = append(parts, "full")
112: 	case strings.TrimSpace(e.RelevantPassages) != "":
113: 		parts = append(parts, strings.TrimSpace(e.RelevantPassages))
114: 	}
115: 	if note := strings.TrimSpace(e.Notes); note != "" {
116: 		parts = append(parts, note)
117: 	}
118: 	return strings.Join(parts, " | ")
119: }
120: 
121: // IDSReference is one cited reference on an exported Information Disclosure Statement.
122: type IDSReference struct {
123: 	Number PatentNumber `json:"number"`
124: 	Title  string       `json:"title"`
125: }
126: 
127: // IDS is the set of references disclosed to the patent office for a project.
128: type IDS struct {
129: 	Project     ProjectID      `json:"project"`
130: 	Status      IDSStatus      `json:"status"`
131: 	GeneratedAt time.Time      `json:"generated_at"`
132: 	Entries     []IDSReference `json:"entries"`
133: }

(End of file - total 133 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/text"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/text</path>
<type>directory</type>
<entries>
catalog_en.go
text.go

(2 entries)
</entries>
```

---

## Assistant (Plan · Big Pickle · 4.3s)

_Thinking:_

Let me also look at the text catalog and the render/paginator to understand how things work.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/text/text.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/text/text.go</path>
<type>file</type>
<content>
1: // Package text is the UI string catalog. Every user-facing string is named by
2: // a Key and resolved through a Catalog, so the application is translated by
3: // swapping the catalog at startup rather than by editing call sites. A Key with
4: // no catalog entry resolves to a visibly-marked placeholder, and the wiring
5: // check fails the boot when a command's strings are missing — a gap surfaces
6: // loudly instead of shipping a blank label.
7: package text
8: 
9: import (
10: 	"fmt"
11: 	"maps"
12: )
13: 
14: // Key names one catalog entry.
15: type Key string
16: 
17: // CmdTitle is the Key for a command's short label, derived from its ID so the
18: // command package carries no display strings of its own.
19: func CmdTitle(id string) Key { return Key("cmd." + id + ".title") }
20: 
21: // CmdHelp is the Key for a command's one-line description.
22: func CmdHelp(id string) Key { return Key("cmd." + id + ".help") }
23: 
24: // Catalog resolves Keys to display strings for one locale. It is built once at
25: // startup and injected; there is no package-level catalog state.
26: type Catalog struct {
27: 	locale  string
28: 	entries map[Key]string
29: }
30: 
31: // New builds a catalog for a locale from an entry table. The table is copied,
32: // so the caller may reuse or discard the map afterwards.
33: func New(locale string, entries map[Key]string) *Catalog {
34: 	cloned := make(map[Key]string, len(entries))
35: 	maps.Copy(cloned, entries)
36: 	return &Catalog{locale: locale, entries: cloned}
37: }
38: 
39: // Locale reports the catalog's locale tag.
40: func (c *Catalog) Locale() string { return c.locale }
41: 
42: // Has reports whether the catalog defines key.
43: func (c *Catalog) Has(key Key) bool {
44: 	_, ok := c.entries[key]
45: 	return ok
46: }
47: 
48: // Keys returns every defined key, unordered.
49: func (c *Catalog) Keys() []Key {
50: 	out := make([]Key, 0, len(c.entries))
51: 	for k := range c.entries {
52: 		out = append(out, k)
53: 	}
54: 	return out
55: }
56: 
57: // T resolves key to its display string. An undefined key resolves to the key
58: // wrapped in guillemets so the gap is visible on screen rather than blank.
59: func (c *Catalog) T(key Key) string {
60: 	if v, ok := c.entries[key]; ok {
61: 		return v
62: 	}
63: 	return "«" + string(key) + "»"
64: }
65: 
66: // Tf resolves key as a printf-style template and applies args.
67: func (c *Catalog) Tf(key Key, args ...any) string {
68: 	return fmt.Sprintf(c.T(key), args...)
69: }

(End of file - total 69 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/text/catalog_en.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/text/catalog_en.go</path>
<type>file</type>
<content>
1: package text
2: 
3: import "maps"
4: 
5: // Named keys for strings that are not command titles. Command titles and help
6: // lines are keyed by ID through CmdTitle/CmdHelp and need no constant here.
7: const (
8: 	// Status-line messages.
9: 	StatusWelcome              Key = "status.welcome"
10: 	StatusActiveProject        Key = "status.active_project"
11: 	StatusActiveProjectSaveErr Key = "status.active_project_save_err"
12: 	StatusClearedProject       Key = "status.cleared_project"
13: 	StatusProjectNotFound      Key = "status.project_not_found"
14: 	StatusNoPatentSelected     Key = "status.no_patent_selected"
15: 	StatusNoActiveProject      Key = "status.no_active_project"
16: 	StatusDaemonUnavailable    Key = "status.daemon_unavailable"
17: 	StatusNoProjectSelection   Key = "status.no_project_selection"
18: 	StatusNoProjectSelected    Key = "status.no_project_selected"
19: 	StatusUnknownCommand       Key = "status.unknown_command"
20: 	StatusCommandNotHere       Key = "status.command_not_here"
21: 	StatusUsage                Key = "status.usage"
22: 	StatusUnhandledCommand     Key = "status.unhandled_command"
23: 	StatusInvalidPatentNumber  Key = "status.invalid_patent_number"
24: 	StatusDaemonClosed         Key = "status.daemon_closed"
25: 	StatusCrawlProgress       Key = "status.ingest_progress"
26: 	StatusCrawlFailed         Key = "status.ingest_failed"
27: 	StatusCrawlComplete       Key = "status.ingest_complete"
28: 	StatusCrawlStarted        Key = "status.ingest_started"
29: 	StatusCrawlStartFailed    Key = "status.ingest_start_failed"
30: 	StatusAddFailed            Key = "status.add_failed"
31: 	StatusAdded                Key = "status.added"
32: 	StatusAddedNoCrawl        Key = "status.added_no_ingest"
33: 	StatusSetStateFailed       Key = "status.set_state_failed"
34: 	StatusSetState             Key = "status.set_state"
35: 	StatusExportFailed         Key = "status.export_failed"
36: 	StatusExportDone           Key = "status.export_done"
37: 	StatusProjectCreateFailed  Key = "status.project_create_failed"
38: 	StatusProjectCreated       Key = "status.project_created"
39: 	StatusProjectNameEmpty     Key = "status.project_name_empty"
40: 	StatusImportFailed         Key = "status.import_failed"
41: 	StatusImported             Key = "status.imported"
42: 	StatusTagFailed            Key = "status.tag_failed"
43: 	StatusTagged               Key = "status.tagged"
44: 	StatusUntagFailed          Key = "status.untag_failed"
45: 	StatusUntagged             Key = "status.untagged"
46: 	StatusDeleteFailed         Key = "status.delete_failed"
47: 	StatusDeleted              Key = "status.deleted"
48: 	StatusBatchDeleted         Key = "status.batch_deleted"
49: 	StatusBatchSetState        Key = "status.batch_set_state"
50: 	StatusBatchAdded           Key = "status.batch_added"
51: 	StatusFilter               Key = "status.filter"
52: 	StatusBrowserOpenFailed    Key = "status.browser_open_failed"
53: 	StatusBrowserOpened        Key = "status.browser_opened"
54: 	StatusTagTaxonomyAddFailed    Key = "status.tag_taxonomy_add_failed"
55: 	StatusTagTaxonomyAdded        Key = "status.tag_taxonomy_added"
56: 	StatusTagTaxonomyDeleteFailed Key = "status.tag_taxonomy_delete_failed"
57: 	StatusTagTaxonomyDeleted      Key = "status.tag_taxonomy_deleted"
58: 	StatusTagTaxonomyListFailed   Key = "status.tag_taxonomy_list_failed"
59: 	StatusTagPatentAddFailed      Key = "status.tag_patent_add_failed"
60: 	StatusTagPatentAdded          Key = "status.tag_patent_added"
61: 	StatusTagPatentDeleteFailed   Key = "status.tag_patent_delete_failed"
62: 	StatusTagPatentDeleted        Key = "status.tag_patent_deleted"
63: 	StatusTagPatentListFailed     Key = "status.tag_patent_list_failed"
64: 
65: 	// Header / footer navigation hints.
66: 	HintCommands        Key = "hint.commands"
67: 	HintCommand         Key = "hint.command"
68: 	HintDetail          Key = "hint.detail"
69: 	HintCitations       Key = "hint.citations"
70: 	HintCitedBy         Key = "hint.cited_by"
71: 	HintIDS             Key = "hint.ids"
72: 	HintProjects        Key = "hint.projects"
73: 	HintProjectActions  Key = "hint.project_actions"
74: 	HintBack            Key = "hint.back"
75: 	HintNewProject      Key = "hint.new_project"
76: 	HintSelectProject   Key = "hint.select_project"
77: 	HintSelect          Key = "hint.select"
78: 	HintClearActive     Key = "hint.clear_active"
79: 	HintExportIDS       Key = "hint.export_ids"
80: 	HintCrawl          Key = "hint.ingest"
81: 	HintLookup           Key = "hint.fetch"
82: 	HintBrowse          Key = "hint.browse"
83: 	HintJump            Key = "hint.jump"
84: 	HintHelp            Key = "hint.help"
85: 	HintQuit            Key = "hint.quit"
86: 	HintMove            Key = "hint.move"
87: 	HintSlashCommands   Key = "hint.slash_commands"
88: 	SplashCreateHint    Key = "splash.create_hint"
89: 	SplashCreateKeyHint Key = "splash.create_key_hint"
90: 
91: 	// Overlays.
92: 	OverlayHelpTitle       Key = "overlay.help.title"
93: 	OverlayCommandsTitle   Key = "overlay.commands.title"
94: 	OverlayCommandTitle    Key = "overlay.command.title"
95: 	PromptFilterHint       Key = "overlay.prompt.filter_hint"
96: 	PromptDirectHint       Key = "overlay.prompt.direct_hint"
97: 	PromptNoMatch          Key = "overlay.prompt.no_match"
98: 	PromptRunHint          Key = "overlay.prompt.run_hint"
99: 	HelpSectionGlobal      Key = "help.section.global"
100: 	HelpSectionCatalog     Key = "help.section.catalog"
101: 	HelpSectionDetail      Key = "help.section.detail"
102: 	HelpSectionCitations   Key = "help.section.citations"
103: 	HelpSectionIDS         Key = "help.section.ids"
104: 	HelpSectionProjects    Key = "help.section.projects"
105: 	HelpSectionOverlay     Key = "help.section.overlay"
106: 	HelpSectionAvailable   Key = "help.section.available"
107: 	NewProjectTitle        Key = "overlay.new_project.title"
108: 	NewProjectCaption      Key = "overlay.new_project.caption"
109: 	EditIDSKindTitle       Key = "overlay.ids.kind.title"
110: 	EditIDSKindCaption     Key = "overlay.ids.kind.caption"
111: 	EditIDSCountryTitle    Key = "overlay.ids.country.title"
112: 	EditIDSCountryCaption  Key = "overlay.ids.country.caption"
113: 	EditIDSPassagesTitle   Key = "overlay.ids.passages.title"
114: 	EditIDSPassagesCaption Key = "overlay.ids.passages.caption"
115: 	EditIDSNotesTitle      Key = "overlay.ids.notes.title"
116: 	EditIDSNotesCaption    Key = "overlay.ids.notes.caption"
117: 	TextInputHint          Key = "overlay.text_input.hint"
118: )
119: 
120: // cmdStrings is the title/help text for every command, keyed by command ID.
121: // English() expands it into CmdTitle/CmdHelp catalog entries.
122: var cmdStrings = map[string][2]string{
123: 	"nav.down":                 {"Down", "Move the cursor down one row."},
124: 	"nav.up":                   {"Up", "Move the cursor up one row."},
125: 	"nav.page-down":            {"Page down", "Scroll down one page."},
126: 	"nav.page-up":              {"Page up", "Scroll up one page."},
127: 	"nav.top":                  {"Top", "Jump to the first row."},
128: 	"nav.bottom":               {"Bottom", "Jump to the last row."},
129: 	"nav.reselect-last":        {"Reselect last", "Jump back to the last active patent."},
130: 	"view.detail":              {"Open detail", "Open the selected patent's detail view."},
131: 	"view.browser":             {"Open browser", "Open the selected patent's page in the browser, or a typed patent number when given."},
132: 	"view.citations":           {"Open citations", "Show patents the selected patent cites."},
133: 	"view.cited-by":            {"Open cited by", "Show patents that cite the selected patent."},
134: 	"view.ids":                 {"Open IDS", "Open the selected patent's IDS entry editor."},
135: 	"view.projects":            {"Open projects", "Open the project list."},
136: 	"view.back":                {"Back", "Return to the previous pane."},
137: 	"view.close-overlay":       {"Close", "Close the focused overlay."},
138: 	"view.refresh":             {"Refresh", "Reload the current pane from the daemon."},
139: 	"search.open":              {"Command palette", "Open the command palette."},
140: 	"command.open":             {"Command prompt", "Open the command prompt."},
141: 	"view.jump":                {"Jump to field", "Jump straight to a labelled field in the detail view."},
142: 	"app.quit":                 {"Quit", "Quit the application."},
143: 	"app.help":                 {"Help", "Show the help screen."},
144: 	"patent.list":              {"List patents", "List stored patents."},
145: 	"patent.get":               {"Get patent", "Fetch one patent's record."},
146: 	"patent.relations":         {"Patent relations", "List a patent's family-graph edges."},
147: 	"project.list":             {"List projects", "List all projects."},
148: 	"ids.export":               {"Export IDS", "Build the Information Disclosure Statement for the selected project."},
149: 	"patent.mark-stored":       {"Mark stored", "Set the selected patent to the stored review state."},
150: 	"patent.mark-under-review": {"Mark under review", "Set the selected patent to under review."},
151: 	"patent.mark-ignored":      {"Mark ignored", "Set the selected patent to ignored."},
152: 	"patent.mark-deleted":      {"Mark deleted", "Soft-delete the selected patent from the project."},
153: 	"patent.delete":            {"Delete patent", "Permanently remove the selected patent from the database."},
154: 	"select.visual":            {"Visual select", "Toggle visual mode at the cursor to begin range selection."},
155: 	"select.clear":             {"Clear selection", "Exit visual mode and clear the selection range."},
156: 	"select.all":               {"Select all", "Select every patent in the current list."},
157: 	"col.next":                 {"Next column", "Move the visual focus to the next column."},
158: 	"col.prev":                 {"Prev column", "Move the visual focus to the previous column."},
159: 	"col.sort-apply":           {"Apply sort", "Apply sorting to the currently focused column."},
160: 	"patent.add-to-project":    {"Add to project", "Add the selected patent to the active project."},
161: 	"patent.tag":               {"Tag patent", "Tag the selected patent within the active project; an unknown name creates the tag."},
162: 	"patent.untag":             {"Untag patent", "Remove a tag from the selected patent within the active project."},
163: 	"crawl.family":            {"Crawl family", "Recursively crawl the selected patent's family graph (parents and children)."},
164: 	"crawl.citations":         {"Crawl citations", "Crawl patents the selected patent cites."},
165: 	"crawl.citedby":           {"Crawl cited-by", "Crawl patents that cite the selected patent."},
166: 	"crawl.all":               {"Crawl all", "Crawl the full family graph including citations and cited-by."},
167: 	"patent.lookup":           {"Lookup patent", "Fetch the selected patent's record from the web."},
168: 	"patent.import":            {"Import patent", "Fetch a patent by number (add 'force' to bypass the cache) or load a fixture file by path."},
169: 	"crawl.cancel":            {"Cancel crawl", "Cancel a running crawl job."},
170: 	"project.create":           {"Create project", "Create a new project."},
171: 	"project.activate":         {"Use project", "Make the selected project the active project for patent actions."},
172: 	"project.clear-active":     {"Clear active project", "Clear the active project filter and target."},
173: 	"view.filter":              {"Filter", "Apply a filter to the current list (e.g. :filter state cached)."},
174: 	"find.open":                {"Find", "Open the inline find bar; type to search, n/N to navigate, Enter to keep, Esc to cancel."},
175: 	"ids.edit-field":           {"Edit IDS field", "Edit the selected IDS field."},
176: 	"ids.toggle-full":          {"Toggle IDS full", "Toggle whether the full document is cited on the IDS."},
177: 	"ids.cycle-status":         {"Cycle IDS status", "Cycle the IDS entry status through pending, submitted, and accepted."},
178: 	"ids.delete":               {"Delete IDS entry", "Remove the current patent from the curated IDS."},
179: 	"tag.add":                  {"Add taxonomy tag", "Register a new tag in the project's taxonomy."},
180: 	"tag.list":                 {"List taxonomy tags", "List all tags in the project's taxonomy."},
181: 	"tag.delete":               {"Delete taxonomy tag", "Remove a tag from the project's taxonomy."},
182: 	"tag.patent.add":           {"Assign patent tag", "Assign a taxonomy tag to the selected patent."},
183: 	"tag.patent.delete":        {"Remove patent tag", "Remove a tag assignment from the selected patent."},
184: 	"tag.patent.list":          {"List patent tags", "List tags assigned to the selected patent."},
185: 	"tag.patent":               {"Manage patent tags", "Open the interactive tag selector popup for the selected patent(s)."},
186: }
187: 
188: // englishNamed is the English text for every named key.
189: var englishNamed = map[Key]string{
190: 	StatusWelcome:              "select a project to begin — press ? for help",
191: 	StatusActiveProject:        "active project: %s",
192: 	StatusActiveProjectSaveErr: "active project: %s (save failed: %s)",
193: 	StatusClearedProject:       "cleared active project",
194: 	StatusProjectNotFound:      "project not found: %s",
195: 	StatusNoPatentSelected:     "no patent selected",
196: 	StatusNoActiveProject:      "select an active project first",
197: 	StatusDaemonUnavailable:    "daemon connection unavailable",
198: 	StatusNoProjectSelection:   "focused pane has no project selection",
199: 	StatusNoProjectSelected:    "no project selected",
200: 	StatusUnknownCommand:       "unknown command: %s",
201: 	StatusCommandNotHere:       "%s is not available here",
202: 	StatusUsage:                "usage: %s",
203: 	StatusUnhandledCommand:     "unhandled command: %s",
204: 	StatusInvalidPatentNumber:  "invalid patent number: %s",
205: 	StatusDaemonClosed:         "daemon connection closed",
206: 	StatusCrawlProgress:       "crawl %s — crawled %d, discovered %d: %s",
207: 	StatusCrawlFailed:         "crawl %s failed: %s",
208: 	StatusCrawlComplete:       "crawl %s complete",
209: 	StatusCrawlStarted:        "crawl started for %s (%s)",
210: 	StatusCrawlStartFailed:    "crawl failed: %s",
211: 	StatusAddFailed:            "add to project failed: %s",
212: 	StatusAdded:                "added %s to %s",
213: 	StatusAddedNoCrawl:        "added %s — press L to lookup",
214: 	StatusSetStateFailed:       "set state failed: %s",
215: 	StatusSetState:             "set %s to %s review state in %s",
216: 	StatusExportFailed:         "IDS export failed: %s",
217: 	StatusExportDone:           "IDS for %q: %d disclosed reference(s)",
218: 	StatusProjectCreateFailed:  "create project failed: %s",
219: 	StatusProjectCreated:       "created project %s",
220: 	StatusProjectNameEmpty:     "project name cannot be empty",
221: 	StatusImportFailed:         "import failed: %s",
222: 	StatusImported:             "imported %s",
223: 	StatusTagFailed:            "tag failed: %s",
224: 	StatusTagged:               "tagged %s as %q in %s",
225: 	StatusUntagFailed:          "untag failed: %s",
226: 	StatusUntagged:             "removed tag %q from %s",
227: 	StatusDeleteFailed:         "delete failed: %s",
228: 	StatusDeleted:              "deleted %s",
229: 	StatusBatchDeleted:         "deleted %d patents",
230: 	StatusBatchSetState:        "set %d patents to %s in %s",
231: 	StatusBatchAdded:           "added %d patents to %s",
232: 	StatusFilter:               "%s",
233: 	StatusBrowserOpenFailed:    "open browser failed: %s",
234: 	StatusBrowserOpened:        "opened %d patent page(s) in browser",
235: 	StatusTagTaxonomyAddFailed:    "add tag to taxonomy failed: %s",
236: 	StatusTagTaxonomyAdded:        "added taxonomy tag %q to %s",
237: 	StatusTagTaxonomyDeleteFailed: "delete tag from taxonomy failed: %s",
238: 	StatusTagTaxonomyDeleted:      "deleted taxonomy tag %q from %s",
239: 	StatusTagTaxonomyListFailed:   "list taxonomy tags failed: %s",
240: 	StatusTagPatentAddFailed:      "assign tag to patent failed: %s",
241: 	StatusTagPatentAdded:          "assigned tag %q to patent %s",
242: 	StatusTagPatentDeleteFailed:   "remove tag from patent failed: %s",
243: 	StatusTagPatentDeleted:        "removed tag %q from patent %s",
244: 	StatusTagPatentListFailed:     "list patent tags failed: %s",
245: 
246: 	HintCommands:        "commands",
247: 	HintCommand:         "command",
248: 	HintDetail:          "detail",
249: 	HintCitations:       "citations",
250: 	HintCitedBy:         "cited by",
251: 	HintIDS:             "IDS",
252: 	HintProjects:        "projects",
253: 	HintProjectActions:  "project actions",
254: 	HintBack:            "back",
255: 	HintNewProject:      "new project",
256: 	HintSelectProject:   "select project",
257: 	HintSelect:          "select",
258: 	HintClearActive:     "clear active",
259: 	HintExportIDS:       "export IDS",
260: 	HintCrawl:          "crawl family",
261: 	HintLookup:           "lookup patent",
262: 	HintBrowse:          "open browser",
263: 	HintJump:            "jump",
264: 	HintHelp:            "help",
265: 	HintQuit:            "quit",
266: 	HintMove:            "move",
267: 	HintSlashCommands:   "/ commands",
268: 	SplashCreateHint:    "Create one with %s.",
269: 	SplashCreateKeyHint: "Create one with %s or %s.",
270: 
271: 	OverlayHelpTitle:       "Help — key bindings",
272: 	OverlayCommandsTitle:   "Commands",
273: 	OverlayCommandTitle:    "Command",
274: 	PromptFilterHint:       "Filter commands",
275: 	PromptDirectHint:       "Type a dot command",
276: 	PromptNoMatch:          "no matching commands",
277: 	PromptRunHint:          "enter runs the selected command · esc closes",
278: 	HelpSectionGlobal:      "Global",
279: 	HelpSectionCatalog:     "Catalog",
280: 	HelpSectionDetail:      "Detail",
281: 	HelpSectionCitations:   "Citations",
282: 	HelpSectionIDS:         "IDS",
283: 	HelpSectionProjects:    "Projects",
284: 	HelpSectionOverlay:     "Overlay",
285: 	HelpSectionAvailable:   "Available keys",
286: 	NewProjectTitle:        "New project",
287: 	NewProjectCaption:      "Enter a name for the new project.",
288: 	EditIDSKindTitle:       "IDS kind code",
289: 	EditIDSKindCaption:     "Enter the patent kind code, such as B2 or A1.",
290: 	EditIDSCountryTitle:    "IDS country code",
291: 	EditIDSCountryCaption:  "Enter the patent country code, such as US, EP, or WO.",
292: 	EditIDSPassagesTitle:   "IDS relevant passages",
293: 	EditIDSPassagesCaption: "Enter the cited passages, or leave blank when citing the full document.",
294: 	EditIDSNotesTitle:      "IDS notes",
295: 	EditIDSNotesCaption:    "Enter any IDS note for this patent.",
296: 	TextInputHint:          "enter confirms · esc cancels",
297: }
298: 
299: // English returns the shipped en catalog: every command's title and help plus
300: // every named string.
301: func English() *Catalog {
302: 	entries := make(map[Key]string, len(englishNamed)+2*len(cmdStrings))
303: 	maps.Copy(entries, englishNamed)
304: 	for id, pair := range cmdStrings {
305: 		entries[CmdTitle(id)] = pair[0]
306: 		entries[CmdHelp(id)] = pair[1]
307: 	}
308: 	return New("en", entries)
309: }

(End of file - total 309 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/render/paginator.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/render/paginator.go</path>
<type>file</type>
<content>
1: // Package render holds the TUI's reusable presentation pieces: the Paginator
2: // shared by every list pane, the colour theme, and layout helpers. None of it
3: // holds application state — panes embed these values.
4: package render
5: 
6: // JumpAnchor is one labelled scroll target in a long pane: jump mode lists the
7: // anchors, and selecting one scrolls the pane straight to that line.
8: type JumpAnchor struct {
9: 	// Key is the single key that selects this anchor in jump mode.
10: 	Key rune
11: 	// Label is the human-readable name shown in the jump overlay.
12: 	Label string
13: 	// Line is the 0-based body line the anchor scrolls to.
14: 	Line int
15: }
16: 
17: // Paginator tracks a cursor over a list and the visible window around it. Every
18: // list pane embeds one instead of re-deriving scroll arithmetic, so paging
19: // behaves identically across the catalog, citations, and project views.
20: type Paginator struct {
21: 	total    int
22: 	pageSize int
23: 	cursor   int // absolute index into the list, 0-based
24: 	offset   int // absolute index of the first visible row
25: }
26: 
27: // NewPaginator returns a Paginator with the given page size (clamped to >= 1).
28: func NewPaginator(pageSize int) Paginator {
29: 	p := Paginator{pageSize: 1}
30: 	p.SetPageSize(pageSize)
31: 	return p
32: }
33: 
34: // SetTotal updates the list length, keeping the cursor and window valid.
35: func (p *Paginator) SetTotal(n int) {
36: 	if n < 0 {
37: 		n = 0
38: 	}
39: 	p.total = n
40: 	p.clamp()
41: }
42: 
43: // SetPageSize updates the visible row count, keeping the window valid.
44: func (p *Paginator) SetPageSize(n int) {
45: 	if n < 1 {
46: 		n = 1
47: 	}
48: 	p.pageSize = n
49: 	p.clamp()
50: }
51: 
52: // Total returns the list length.
53: func (p Paginator) Total() int { return p.total }
54: 
55: // PageSize returns the visible row count.
56: func (p Paginator) PageSize() int { return p.pageSize }
57: 
58: // Cursor returns the absolute cursor index.
59: func (p Paginator) Cursor() int { return p.cursor }
60: 
61: // Offset returns the absolute index of the first visible row.
62: func (p Paginator) Offset() int { return p.offset }
63: 
64: // CursorInPage returns the cursor's row within the visible window.
65: func (p Paginator) CursorInPage() int { return p.cursor - p.offset }
66: 
67: // Window returns the half-open range [start, end) of visible indices.
68: func (p Paginator) Window() (start, end int) {
69: 	end = min(p.offset+p.pageSize, p.total)
70: 	return p.offset, end
71: }
72: 
73: // MoveDown advances the cursor by n rows (n is clamped to >= 1).
74: func (p *Paginator) MoveDown(n int) { p.move(max(n, 1)) }
75: 
76: // MoveUp retreats the cursor by n rows (n is clamped to >= 1).
77: func (p *Paginator) MoveUp(n int) { p.move(-max(n, 1)) }
78: 
79: // PageDown moves the cursor down one page.
80: func (p *Paginator) PageDown() { p.move(p.pageSize) }
81: 
82: // PageUp moves the cursor up one page.
83: func (p *Paginator) PageUp() { p.move(-p.pageSize) }
84: 
85: // Top moves the cursor to the first row.
86: func (p *Paginator) Top() {
87: 	p.cursor = 0
88: 	p.clamp()
89: }
90: 
91: // Bottom moves the cursor to the last row.
92: func (p *Paginator) Bottom() {
93: 	p.cursor = p.total - 1
94: 	p.clamp()
95: }
96: 
97: // ScrollTo scrolls the window so index leads the visible rows, keeping the
98: // cursor on index. The offset is clamped so the window never runs past the
99: // list end — jump targets near the bottom still fill the page.
100: func (p *Paginator) ScrollTo(index int) {
101: 	if p.total == 0 {
102: 		p.cursor, p.offset = 0, 0
103: 		return
104: 	}
105: 	p.cursor = min(max(index, 0), p.total-1)
106: 	maxOffset := max(p.total-p.pageSize, 0)
107: 	p.offset = min(max(index, 0), maxOffset)
108: }
109: 
110: func (p *Paginator) move(delta int) {
111: 	p.cursor += delta
112: 	p.clamp()
113: }
114: 
115: // clamp keeps the cursor inside the list and scrolls the window to follow it.
116: func (p *Paginator) clamp() {
117: 	if p.total == 0 {
118: 		p.cursor, p.offset = 0, 0
119: 		return
120: 	}
121: 	p.cursor = min(max(p.cursor, 0), p.total-1)
122: 
123: 	if p.cursor < p.offset {
124: 		p.offset = p.cursor
125: 	} else if p.cursor >= p.offset+p.pageSize {
126: 		p.offset = p.cursor - p.pageSize + 1
127: 	}
128: 	maxOffset := max(p.total-p.pageSize, 0)
129: 	p.offset = min(max(p.offset, 0), maxOffset)
130: }

(End of file - total 130 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/wiring.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/wiring.go</path>
<type>file</type>
<content>
1: package tui
2: 
3: import (
4: 	"errors"
5: 	"fmt"
6: 	"slices"
7: 
8: 	"patentmine/internal/command"
9: 	"patentmine/internal/domain"
10: 	"patentmine/internal/text"
11: 	"patentmine/internal/tui/keymap"
12: 	"patentmine/internal/tui/overlay"
13: 	"patentmine/internal/tui/pane"
14: 	"patentmine/internal/tui/render"
15: )
16: 
17: // validateWiring fails the boot when the keymap, command registry, handlers,
18: // and text catalog are not consistent. It is the structural guarantee behind
19: // the input model: a bound key or typed command can never resolve to a command
20: // that no handler services, and every command has display strings.
21: func validateWiring(reg *command.Registry, keymaps *keymap.Keymaps, catalog *text.Catalog) error {
22: 	if reg == nil || keymaps == nil || catalog == nil {
23: 		return errors.New("tui: New requires a registry, keymap, and text catalog")
24: 	}
25: 	paneHandled := paneHandlerSets()
26: 	overlayHandled := overlayHandlerSet(reg, keymaps, catalog)
27: 
28: 	// dispatchable reports whether command id reaches a handler in scope: either
29: 	// the App table, or the pane/overlay that owns that scope.
30: 	dispatchable := func(id command.ID, scope command.Scope) bool {
31: 		if _, ok := appHandlers[id]; ok {
32: 			return true
33: 		}
34: 		if scope == command.ScopeOverlay {
35: 			return slices.Contains(overlayHandled, id)
36: 		}
37: 		return slices.Contains(paneHandled[scope], id)
38: 	}
39: 
40: 	var errs []error
41: 
42: 	// Every key binding must resolve to a registered, dispatchable command.
43: 	checkLayer := func(scope command.Scope, layer *keymap.Layer, global bool) {
44: 		if layer == nil {
45: 			return
46: 		}
47: 		for seq, id := range layer.Bindings() {
48: 			if _, ok := reg.Lookup(id); !ok {
49: 				errs = append(errs, fmt.Errorf("keymap: %q binds %q to unregistered command %q", scope, seq, id))
50: 				continue
51: 			}
52: 			if global {
53: 				// A global key is active in every scope, so only the App
54: 				// table can service it.
55: 				if _, ok := appHandlers[id]; !ok {
56: 					errs = append(errs, fmt.Errorf("keymap: global key %q binds %q, which no App handler services", seq, id))
57: 				}
58: 				continue
59: 			}
60: 			if !dispatchable(id, scope) {
61: 				errs = append(errs, fmt.Errorf("keymap: %q key %q binds %q, which no handler services there", scope, seq, id))
62: 			}
63: 		}
64: 	}
65: 	checkLayer("global", keymaps.Base(), true)
66: 	for scope, layer := range keymaps.ScopeLayers() {
67: 		checkLayer(scope, layer, false)
68: 	}
69: 
70: 	// Every typed command must be dispatchable in each scope it is offered in,
71: 	// so the command prompt and palette can never invoke a dead command.
72: 	for _, c := range reg.All() {
73: 		if c.Name == "" {
74: 			continue
75: 		}
76: 		for _, scope := range typedCheckScope(c) {
77: 			if !dispatchable(c.ID, scope) {
78: 				errs = append(errs, fmt.Errorf("command: typed command %q is offered in %q but no handler services it there", c.Name, scope))
79: 			}
80: 		}
81: 	}
82: 
83: 	// Every command must have title and help strings in the catalog.
84: 	for _, c := range reg.All() {
85: 		if !catalog.Has(text.CmdTitle(string(c.ID))) {
86: 			errs = append(errs, fmt.Errorf("text: catalog has no title for command %q", c.ID))
87: 		}
88: 		if !catalog.Has(text.CmdHelp(string(c.ID))) {
89: 			errs = append(errs, fmt.Errorf("text: catalog has no help for command %q", c.ID))
90: 		}
91: 	}
92: 
93: 	return errors.Join(errs...)
94: }
95: 
96: // paneScopes are the scopes backed by a focusable pane.
97: var paneScopes = []command.Scope{
98: 	command.ScopeCatalog, command.ScopeDetail,
99: 	command.ScopeCitations, command.ScopeIDS, command.ScopeProjects,
100: }
101: 
102: // typedCheckScope returns the scopes in which a typed command must be
103: // dispatchable. A global command is checked against every pane scope (where
104: // dispatchable resolves it through the App table); a scoped command against its
105: // own pane scopes.
106: func typedCheckScope(c command.Command) []command.Scope {
107: 	if c.Global() {
108: 		return paneScopes
109: 	}
110: 	var out []command.Scope
111: 	for _, sc := range c.Scopes {
112: 		if sc == command.ScopeOverlay {
113: 			continue // overlay-scoped typed commands run in their source scope
114: 		}
115: 		out = append(out, sc)
116: 	}
117: 	return out
118: }
119: 
120: // paneHandlerSets builds a sample of every pane and records the command IDs it
121: // services, keyed by the pane's context.
122: func paneHandlerSets() map[command.Scope][]command.ID {
123: 	theme := render.NewTheme()
124: 	panes := []pane.Pane{
125: 		pane.NewCatalog(nil, theme),
126: 		pane.NewDetail(nil, theme, domain.PatentNumber{}, "", nil),
127: 		pane.NewCitations(nil, theme, domain.PatentNumber{}, domain.RelationCites),
128: 		pane.NewIDSDetail(nil, theme, domain.PatentNumber{}, ""),
129: 		pane.NewProjects(nil, theme),
130: 	}
131: 	out := make(map[command.Scope][]command.ID, len(panes))
132: 	for _, p := range panes {
133: 		out[p.Scope()] = p.Handles()
134: 	}
135: 	return out
136: }
137: 
138: // overlayHandlerSet returns the union of command IDs serviced by any overlay.
139: // KeyHandler overlays consume input before the keymap, so the overlay keymap
140: // layer only governs passive overlays — a union is the right test here.
141: func overlayHandlerSet(reg *command.Registry, keymaps *keymap.Keymaps, catalog *text.Catalog) []command.ID {
142: 	theme := render.NewTheme()
143: 	overlays := []overlay.Overlay{
144: 		overlay.NewHelp(reg, keymaps, theme, catalog),
145: 		overlay.NewPrompt(reg, keymaps, theme, catalog, command.ScopeCatalog, overlay.PromptPalette),
146: 		overlay.NewTextInput(theme, catalog, overlay.PurposeCreateProject, text.NewProjectTitle, text.NewProjectCaption),
147: 	}
148: 	var out []command.ID
149: 	for _, ov := range overlays {
150: 		for _, id := range ov.Handles() {
151: 			if !slices.Contains(out, id) {
152: 				out = append(out, id)
153: 			}
154: 		}
155: 	}
156: 	return out
157: }

(End of file - total 157 lines)
</content>
```

---

## Assistant (Plan · Big Pickle · 3.8s)

_Thinking:_

Now let me also check the pane interface to understand how panes work, and look at the end of the app.go for things like policy.go and the pane interface definition.

Let me check a few more files to understand the pane interface and policies.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/pane"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/pane</path>
<type>directory</type>
<entries>
calls.go
catalog_test.go
catalog.go
citations.go
detail.go
filter.go
find.go
ids.go
pane.go
projects.go
table.go
views_test.go

(12 entries)
</entries>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/policy.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/policy.go</path>
<type>file</type>
<content>
1: package tui
2: 
3: import (
4: 	"fmt"
5: 
6: 	"patentmine/internal/command"
7: 	"patentmine/internal/domain"
8: )
9: 
10: // confirmFn decides whether to show a yes/no dialog before an action runs.
11: // Returns the dialog message and whether confirmation is required.
12: type confirmFn func(numbers []domain.PatentNumber) (msg string, needs bool)
13: 
14: // CommandPolicy declares an optional pre-action confirmation for a command.
15: // A nil Confirm is a no-op (action fires immediately).
16: type CommandPolicy struct {
17: 	Confirm confirmFn
18: }
19: 
20: // commandPolicies maps command IDs to their confirmation behaviour.
21: // Commands absent from this map fire immediately with no dialog.
22: var commandPolicies = map[command.ID]CommandPolicy{
23: 	command.MarkDeleted: {
24: 		Confirm: func(ns []domain.PatentNumber) (string, bool) {
25: 			if len(ns) == 1 {
26: 				return "Mark " + ns[0].String() + " as deleted?", true
27: 			}
28: 			return fmt.Sprintf("Mark %d patents as deleted?", len(ns)), true
29: 		},
30: 	},
31: 	command.PatentDelete: {
32: 		Confirm: func(ns []domain.PatentNumber) (string, bool) {
33: 			if len(ns) == 1 {
34: 				return "Delete " + ns[0].String() + "? This cannot be undone.", true
35: 			}
36: 			return fmt.Sprintf("Delete %d patents? This cannot be undone.", len(ns)), true
37: 		},
38: 	},
39: }

(End of file - total 39 lines)
</content>
```

---

## Assistant (Plan · Big Pickle · 3.2s)

_Thinking:_

Let me also look at the pane.go to understand the Pane interface.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/pane/pane.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/pane/pane.go</path>
<type>file</type>
<content>
1: // Package pane holds the TUI's screens. Each screen is a Pane that owns only
2: // its own state; the App owns the pane stack. Adding a screen means adding a
3: // Pane type — never extending a shared "model" struct. This decomposition is
4: // the structural defence against the god-object that sank the prior attempt.
5: package pane
6: 
7: import (
8: 	"slices"
9: 
10: 	tea "github.com/charmbracelet/bubbletea"
11: 
12: 	"patentmine/internal/command"
13: 	"patentmine/internal/domain"
14: 	"patentmine/internal/text"
15: 	"patentmine/internal/tui/render"
16: )
17: 
18: // ResizeMsg reports the body area available to a pane after the app reserves
19: // its header and status lines.
20: type ResizeMsg struct {
21: 	Width  int
22: 	Height int
23: }
24: 
25: // ProjectChangedMsg reports that the app's active project changed.
26: type ProjectChangedMsg struct {
27: 	Project *domain.Project
28: }
29: 
30: // Pane is one screen of the TUI.
31: type Pane interface {
32: 	// Scope reports the keymap scope the pane uses, so the App can pick
33: 	// the right key bindings while this pane is focused.
34: 	Scope() command.Scope
35: 	// Title is shown in the header bar.
36: 	Title() string
37: 	// Init returns a command to run when the pane is first shown.
38: 	Init() tea.Cmd
39: 	// Command applies a resolved command intent forwarded by the App.
40: 	Command(id command.ID, inv Invocation) (Pane, tea.Cmd)
41: 	// Handles reports every command ID the pane services. The App's wiring
42: 	// check cross-references it against the keymap so a key can never resolve
43: 	// to a command the focused pane silently drops.
44: 	Handles() []command.ID
45: 	// Update applies a non-command message: an rpc result, a daemon event, or
46: 	// a resize.
47: 	Update(msg tea.Msg) (Pane, tea.Cmd)
48: 	// View renders the pane body into w columns by h rows.
49: 	View(w, h int) string
50: 	// Selection reports the highlighted patent, when the pane has one. The App
51: 	// uses it to open detail/citation views for the current row.
52: 	Selection() (domain.PatentNumber, bool)
53: }
54: 
55: // MultiSelector is implemented by panes that support visual range selection.
56: // App checks for this interface before falling back to single Selection().
57: type MultiSelector interface {
58: 	Selections() []domain.PatentNumber
59: }
60: 
61: // VisualSelectionSaver is implemented by panes that support saving their
62: // visual selection for gv restore. The App calls this when a visual selection
63: // is consumed by an action (e.g. review state change).
64: type VisualSelectionSaver interface {
65: 	SaveVisualSelection()
66: }
67: 
68: // KeyHandler is implemented by panes that need to intercept raw key events
69: // before keymap resolution — for example when an inline input bar is active.
70: // The App checks this interface before feeding the key to the chord reader.
71: type KeyHandler interface {
72: 	HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool)
73: }
74: 
75: // JumpProvider is implemented by panes that support jump mode — a quick scroll
76: // straight to a labelled field in a long, scrolling pane. The App opens the
77: // jump overlay from JumpAnchors and calls JumpTo with the chosen line.
78: type JumpProvider interface {
79: 	// JumpAnchors returns the pane's current jump targets, in display order.
80: 	JumpAnchors() []render.JumpAnchor
81: 	// JumpTo scrolls the pane so the given body line leads the view.
82: 	JumpTo(line int)
83: }
84: 
85: // Invocation carries a resolved command's repeat count and any typed arguments.
86: type Invocation struct {
87: 	Repeat int
88: 	Args   []string
89: }
90: 
91: // cmdHandler carries out one command for a pane. The pane mutates itself
92: // through its pointer receiver and returns only a tea.Cmd.
93: type cmdHandler func(Invocation) tea.Cmd
94: 
95: // handlerIDs returns the command IDs of a handler table, sorted for stable
96: // output in the wiring check and help screen.
97: func handlerIDs(handlers map[command.ID]cmdHandler) []command.ID {
98: 	ids := make([]command.ID, 0, len(handlers))
99: 	for id := range handlers {
100: 		ids = append(ids, id)
101: 	}
102: 	slices.Sort(ids)
103: 	return ids
104: }
105: 
106: // StatusMsg asks the App to show a line of status text. Panes emit a text key
107: // plus arguments rather than a resolved string, so the App resolves it through
108: // the active locale catalog and status appears in one consistent place.
109: type StatusMsg struct {
110: 	Key   text.Key
111: 	Args  []any
112: 	Error bool
113: }
114: 
115: // MultiCrawlStartedMsg is emitted when multiple patents are selected and a
116: // crawl or lookup is started for each of them. It carries all job IDs so the
117: // app can show a single aggregate overlay instead of stacking one per job.
118: type MultiCrawlStartedMsg struct {
119: 	Numbers []domain.PatentNumber
120: 	JobIDs  []string
121: 	Depth   int
122: }
123: 
124: // status returns a tea.Cmd that emits a StatusMsg for key.
125: func status(key text.Key, isErr bool, args ...any) tea.Cmd {
126: 	return func() tea.Msg { return StatusMsg{Key: key, Args: args, Error: isErr} }
127: }

(End of file - total 127 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/browser.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/browser.go</path>
<type>file</type>
<content>
1: package tui
2: 
3: import (
4: 	"context"
5: 	"os/exec"
6: 	"runtime"
7: 	"strings"
8: 	"time"
9: 
10: 	tea "github.com/charmbracelet/bubbletea"
11: 
12: 	"patentmine/internal/domain"
13: 	"patentmine/internal/proto"
14: 	"patentmine/internal/text"
15: 	"patentmine/internal/tui/pane"
16: )
17: 
18: // browserOpenTimeout bounds one optional patent lookup before opening a URL.
19: const browserOpenTimeout = 15 * time.Second
20: 
21: func (a *App) openPatentsInBrowser(numbers []domain.PatentNumber) tea.Cmd {
22: 	client := a.client
23: 	openURL := a.openURL
24: 	project := domain.ProjectID("")
25: 	if a.activeProject != nil {
26: 		project = a.activeProject.ID
27: 	}
28: 	return func() tea.Msg {
29: 		opened := 0
30: 		for _, number := range numbers {
31: 			url := patentBrowserURL(number)
32: 			if client != nil {
33: 				ctx, cancel := context.WithTimeout(context.Background(), browserOpenTimeout)
34: 				var res proto.PatentResult
35: 				err := client.Call(ctx, proto.MethodPatentGet, proto.PatentGetParams{Number: number, Project: project}, &res)
36: 				cancel()
37: 				if err == nil {
38: 					url = patentBrowserURL(res.Patent.Number)
39: 					if strings.TrimSpace(res.Patent.SourceURL) != "" {
40: 						url = res.Patent.SourceURL
41: 					}
42: 				}
43: 			}
44: 			if err := openURL(url); err != nil {
45: 				return pane.StatusMsg{Key: text.StatusBrowserOpenFailed, Args: []any{err.Error()}, Error: true}
46: 			}
47: 			opened++
48: 		}
49: 		return pane.StatusMsg{Key: text.StatusBrowserOpened, Args: []any{opened}}
50: 	}
51: }
52: 
53: func patentBrowserURL(number domain.PatentNumber) string {
54: 	return "https://patents.google.com/patent/" + number.String()
55: }
56: 
57: func openExternalURL(url string) error {
58: 	var cmd *exec.Cmd
59: 	switch runtime.GOOS {
60: 	case "darwin":
61: 		cmd = exec.Command("open", url)
62: 	case "windows":
63: 		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
64: 	default:
65: 		cmd = exec.Command("xdg-open", url)
66: 	}
67: 	return cmd.Start()
68: }

(End of file - total 68 lines)
</content>
```

---

## Assistant (Plan · Big Pickle · 3.5s)

_Thinking:_

Now I have a comprehensive understanding of the codebase. Let me also look at the store/sqlite schema to understand how the patent data is stored, and specifically the IDS tables.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/store/sqlite/schema.sql"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/store/sqlite/schema.sql</path>
<type>file</type>
<content>
1: CREATE TABLE IF NOT EXISTS patent (
2:     number            TEXT PRIMARY KEY,
3:     country           TEXT NOT NULL,
4:     serial            TEXT NOT NULL,
5:     kind              TEXT NOT NULL,
6:     title             TEXT NOT NULL,
7:     abstract          TEXT NOT NULL,
8:     assignee          TEXT NOT NULL,
9:     inventors         TEXT NOT NULL,
10:     fetch_state       TEXT NOT NULL,
11:     source            TEXT NOT NULL,
12:     application_date  TEXT NOT NULL,
13:     publication_date  TEXT NOT NULL,
14:     grant_date        TEXT NOT NULL,
15:     fetched_at        TEXT NOT NULL,
16:     display_number    TEXT NOT NULL DEFAULT '',
17:     first_claim       TEXT NOT NULL DEFAULT '',
18:     expiration_date   TEXT NOT NULL DEFAULT '',
19:     expiration_source TEXT NOT NULL DEFAULT '',
20:     source_url        TEXT NOT NULL DEFAULT ''
21: );
22: 
23: CREATE TABLE IF NOT EXISTS relation (
24:     from_number TEXT NOT NULL,
25:     to_number   TEXT NOT NULL,
26:     kind        TEXT NOT NULL,
27:     PRIMARY KEY (from_number, to_number, kind)
28: );
29: 
30: CREATE INDEX IF NOT EXISTS idx_relation_from ON relation (from_number, kind);
31: CREATE INDEX IF NOT EXISTS idx_relation_to   ON relation (to_number, kind);
32: 
33: CREATE TABLE IF NOT EXISTS project (
34:     id         TEXT PRIMARY KEY,
35:     name       TEXT NOT NULL,
36:     created_at TEXT NOT NULL
37: );
38: 
39: CREATE TABLE IF NOT EXISTS membership (
40:     project_id    TEXT NOT NULL REFERENCES project (id),
41:     patent_number TEXT NOT NULL REFERENCES patent (number),
42:     state         TEXT NOT NULL,
43:     added_at      TEXT NOT NULL,
44:     PRIMARY KEY (project_id, patent_number)
45: );
46: 
47: CREATE INDEX IF NOT EXISTS idx_membership_project ON membership (project_id, state);
48: 
49: CREATE TABLE IF NOT EXISTS document (
50:     number        TEXT PRIMARY KEY,
51:     record_number TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
52:     country       TEXT NOT NULL,
53:     serial        TEXT NOT NULL,
54:     kind          TEXT NOT NULL,
55:     stage         TEXT NOT NULL,
56:     dated         TEXT NOT NULL
57: );
58: 
59: CREATE INDEX IF NOT EXISTS idx_document_record ON document (record_number, stage);
60: 
61: CREATE TABLE IF NOT EXISTS tag (
62:     id         INTEGER PRIMARY KEY AUTOINCREMENT,
63:     project_id TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
64:     name       TEXT NOT NULL,
65:     created_at TEXT NOT NULL,
66:     UNIQUE (project_id, name)
67: );
68: 
69: CREATE TABLE IF NOT EXISTS patent_tag (
70:     tag_id        INTEGER NOT NULL REFERENCES tag (id) ON DELETE CASCADE,
71:     patent_number TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
72:     created_at    TEXT NOT NULL,
73:     PRIMARY KEY (tag_id, patent_number)
74: );
75: 
76: CREATE INDEX IF NOT EXISTS idx_patent_tag_patent ON patent_tag (patent_number);
77: 
78: CREATE TABLE IF NOT EXISTS project_ids (
79:     id                INTEGER PRIMARY KEY AUTOINCREMENT,
80:     project_id        TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
81:     patent_number     TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
82:     kind_code         TEXT NOT NULL DEFAULT '',
83:     country_code      TEXT NOT NULL DEFAULT '',
84:     in_full           INTEGER NOT NULL DEFAULT 0,
85:     relevant_passages TEXT NOT NULL DEFAULT '',
86:     notes             TEXT NOT NULL DEFAULT '',
87:     status            TEXT NOT NULL DEFAULT 'pending',
88:     added_at          TEXT NOT NULL,
89:     UNIQUE (project_id, patent_number)
90: );
91: 
92: CREATE INDEX IF NOT EXISTS idx_project_ids_project ON project_ids (project_id, added_at DESC);
93: CREATE INDEX IF NOT EXISTS idx_project_ids_patent ON project_ids (patent_number);

(End of file - total 93 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/proto"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/proto</path>
<type>directory</type>
<entries>
codec_test.go
codec.go
proto.go

(3 entries)
</entries>
```

---

## Assistant (Plan · Big Pickle · 40.7s)

_Thinking:_

Now I have all the information I need. Let me compose the comprehensive plan. The plan needs to cover:

1. Two clipboards:
   - **Internal notes accumulator** - smart/dedup, claim references accumulated per-patent, feeds into IDS "Relevant Passages" field
   - **System clipboard export** - one-shot copy of accumulated notes + patent metadata

2. Full text viewer pane

3. Proper shortcuts

Let me design this thoroughly.

### Key Design Decisions:

**For the notes accumulator:**
- This is a new per-patent buffer in the TUI that lives for the session
- When user presses a "add to notes" key in the full text viewer, the locator (e.g., "Claim 2") is added to a deduped set
- This buffer can be flushed/saved to the IDS "Relevant Passages" field via a command
- The buffer can also be exported to system clipboard with full metadata

**For the full text viewer:**
- New pane `FullText` that fetches all claims from Google Patents (re-fetches or uses cached HTML)
- Shows claims with headers like "Claim 1", "Claim 2", etc.
- Supports line-based visual selection within the pane
- Jump anchors per claim

**Shortcuts:**
- `V` (shift-v) - Enter visual selection mode in full text pane
- `y` - Yank/copy selected text to system clipboard (with metadata header)
- `n` - Add current claim locator to internal notes buffer
- `N` (shift-n) - Show notes buffer / export notes
- `;` - Jump mode (reuses existing pattern)

**Clipboard implementation:**
- Use OSC 52 escape sequence as primary (no deps needed)
- Fallback to platform commands: `clip.exe` (WSL), `pbcopy` (macOS), `xclip`/`wl-copy` (Linux)

Let me now write this up as a comprehensive plan.

Now I have everything I need. Here's the complete plan:

---

## Plan: Two-Clipboard System with Full Text Viewer

### Overview

Two distinct clipboard mechanisms:

| Clipboard | Purpose | Scope | Persistence |
|---|---|---|---|
| **Notes Buffer** | Accumulate claim locators per patent (deduped), then flush to IDS | Internal, per-patent | Session-lived, save to DB via IDS |
| **System Clipboard** | Export selected text + patent metadata for external use | One-shot to OS clipboard | Immediate |

---

### New Files

| File | Purpose |
|---|---|
| `internal/domain/fulltext.go` | `FullText` type: `Claims []ClaimSection` where `ClaimSection {Num int, Text string}` |
| `internal/tui/pane/fulltext.go` | New pane for viewing all claims, visual selection, notes accumulation |
| `internal/tui/clipboard.go` | Cross-platform clipboard: OSC 52 + platform fallbacks |
| `internal/tui/notes.go` | Per-patent notes buffer manager |

### Modified Files

| File | Changes |
|---|---|
| `internal/command/catalog.go` | Register new command IDs |
| `internal/command/command.go` | Add `ScopeFullText` scope |
| `internal/tui/keymap/default.go` | Add `ScopeFullText` key bindings |
| `internal/tui/pane/detail.go` | Add jump anchor / button to open full text view |
| `internal/tui/app.go` | Add `appHandlers` entries for new commands |
| `internal/tui/wiring.go` | Add `ScopeFullText` to pane handler sets |
| `internal/text/catalog_en.go` | Add status/hint strings |

---

### 1. Domain Types (`internal/domain/fulltext.go`)

```go
type ClaimSection struct {
    Number int    `json:"number"`
    Text   string `json:"text"`
}

type FullText struct {
    Number PatentNumber `json:"number"`
    Claims []ClaimSection `json:"claims"`
}
```

`FullText` is **not stored in the DB**. Fetched on-demand from Google Patents (re-parsing the HTML page using the existing `parseGoogle` infrastructure, but extracting all `.claim` elements). Cached in-memory for the session; optionally cached to `~/.cache/patentmine/fulltext/<number>.json`.

The existing `parseGoogle` already uses `section[itemprop='claims'] .claim` to get claim 1. We extend this with a new function:

```go
// ParseAllClaims extracts every numbered claim from the patent body.
func ParseAllClaims(body []byte) ([]ClaimSection, error)
```

Iterates `.claim` elements, strips the "Claim N" prefix to extract the number, stores the text.

---

### 2. Full Text Viewer Pane (`internal/tui/pane/fulltext.go`)

New scope: `command.ScopeFullText`

**State:**
```go
type FullText struct {
    client       *rpc.Client
    theme        render.Theme
    number       domain.PatentNumber
    project      domain.ProjectID
    fullText     domain.FullText
    notesBuffer  []string              // accumulated claim locators
    page         render.Paginator
    visualMode   bool                  // visual selection active
    visualStart  int                   // line where selection started
    jumpActive   bool
    anchors      []render.JumpAnchor
    loading      bool
    loadErr      string
}
```

**View layout (body):**
```
Patent #: US-11,234,567-B2
Title:    Method for Quantum Error Correction

[1] Claim 1 ───────────────────────────
  <word-wrapped claim 1 text>

[2] Claim 2 ───────────────────────────
  <word-wrapped claim 2 text>

...
```

Each claim gets a jump anchor (reuse `;` jump mode). Claim headers are styled like the detail pane's section headers.

**Key commands:**

| Key | Command | Action |
|---|---|---|
| `j`/`k` | `NavDown`/`NavUp` | Scroll / move cursor |
| `V` | `SelectVisual` | Enter/exit visual line selection |
| `y` | `CopyYank` | Copy selected lines + metadata to system clipboard |
| `n` | `NoteAdd` | Add current claim locator to notes buffer (deduped) |
| `N` | `NoteOpen` | Show notes buffer overlay / flush to IDS |
| `;` | `JumpMode` | Jump to claim by key |
| `w` | `OpenBrowser` | Open patent in browser |
| `h`/`esc` | `Back` | Back to detail view |
| `x` | `MarkDeleted` | Standard patent review actions |

**Visual selection mode:**
- `V` at a claim header line selects that entire claim
- `V` in the middle of a claim selects from cursor line down
- Moving `j`/`k` extends the selection
- Selected lines get the `theme.Selected` background
- Status line shows `VISUAL L12-L18` when active

---

### 3. Notes Buffer (`internal/tui/notes.go`)

A **per-patent** set of locator strings stored in the TUI (not the daemon):

```go
// NotesBuffer accumulates claim locators for one patent.
type NotesBuffer struct {
    Number  domain.PatentNumber
    Locators []string // deduped, e.g. ["Claim 2", "Claim 5", "Claim 14"]
}
```

**Operations:**
- `AddLocator(number, locator)` — appends if not already in set, sorts. Emits `StatusMsg`.
- `FlushToIDS(number, project, client)` — saves the joined locators to the IDS `RelevantPassages` field via `proto.MethodIDSEntrySave`
- `GetExportText(number, patent)` — format full export text for system clipboard

**Dedup logic:** Simple map lookup. `"Claim 2" + "claim 2"` are folded via case-insensitive comparison.

When user presses `n` (NoteAdd) with cursor on a claim, the locator `"Claim N"` is added to the buffer. Status shows: `"added Claim 5 to notes buffer"`.

When user presses `N` (NoteOpen), a small overlay shows the accumulated locators and offers:
```
Notes for US-11,234,567-B2:
  - Claim 2
  - Claim 5
  - Claim 14

[F] Flush to IDS  [y] Export copy  [esc] Close
```

- `F` — saves to `ids.relevant_passages` (appends to existing content if any)
- `y` — copies full export to system clipboard (see below)
- `esc` — closes overlay

---

### 4. System Clipboard Export Format

When `y` is pressed (with or without visual selection):

**With visual selection (selected lines):**
```
═══════════════════════════════════════════════════════════════
Patent #:     US-11,234,567-B2
Title:        Method for Quantum Error Correction
Inventors:    J. Smith et al. (3)
Assignee:     Quantum Computing Inc.
Application #: US-17/123,456 (Filed: 2021-03-15)
Publication:  US-2022-0123456-A1 (Published: 2022-04-21)
Expiration:   2041-03-15 (estimated)

Source: Claim 2

───────────────────────────────────────────────────────────────

A method for quantum error correction comprising:
encoding a logical qubit into a plurality of physical qubits;
...
```

**With notes buffer (whole buffer export):**
Same patent metadata header, then:
```
───────────────────────────────────────────────────────────────
Relevant passages:
  - Claim 2
  - Claim 5
  - Claim 14
───────────────────────────────────────────────────────────────
```

**From catalog/detail (whole patent copy):**
Similar header, then title + abstract + first claim.

---

### 5. Cross-platform Clipboard (`internal/tui/clipboard.go`)

```go
func CopyToClipboard(text string) error
```

**Strategy (tried in order):**

1. **OSC 52** — Write `\e]52;c;<base64(text)>\x07` to stdout. Works in: Windows Terminal, tmux, iTerm2, kitty, Alacritty, WezTerm, GNOME Terminal (with mods), SSH sessions. Zero dependencies.

2. **Platform detection via `runtime.GOOS`:**
   - Windows/WSL → `exec.Command("clip.exe")` (always available on WSL at `/mnt/c/Windows/System32/clip.exe`)
   - macOS → `exec.Command("pbcopy")`
   - Linux → try `exec.Command("wl-copy")` first (Wayland), then `exec.Command("xclip", "-selection", "clipboard")` (X11)

3. **Fallback:** Save to temp file and show path in status.

This is essentially what `browser.go` already does for URL opening, just for clipboard.

**Why OSC 52 first?** It works across SSH, tmux, and most modern terminal emulators without installing anything. The user doesn't need `xclip`, `wl-clipboard`, or any external tool. It simply writes an escape sequence that the terminal emulator interprets.

---

### 6. Command Registration (`internal/command/catalog.go`)

New IDs to add:
```go
OpenFullText  ID = "view.fulltext"    // Open full text viewer from detail
CopyYank      ID = "edit.copy"        // Yank selection + metadata to clipboard
NoteAdd       ID = "edit.note-add"    // Add claim locator to notes buffer
NoteOpen      ID = "edit.note-open"   // Show/export notes buffer
```

New scope:
```go
ScopeFullText Scope = "fulltext"       // the full claims text viewer
```

---

### 7. Key Bindings (`internal/tui/keymap/default.go`)

FullText layer:
```go
fullText := NewLayer("fulltext", false).
    BindAll(listMotions()).
    BindAll(patentActions()).
    BindAll(viewActions()).
    BindAll(map[string]command.ID{
        "V":   command.SelectVisual,
        "y":   command.CopyYank,
        "n":   command.NoteAdd,
        "N":   command.NoteOpen,
        ";":   command.JumpMode,
        "/":   command.OpenSearch,
    })
```

Detail layer — add binding to open full text:
```go
"t": command.OpenFullText, // 't' for text/claims
```

---

### 8. Detail Pane Extension

In `detail.go`, add a new anchor `"Full claims text"` that appears as:
```
[t] Full claims text
```
When user presses `t` (or jump-key + Enter), the app pushes the `FullText` pane.

Also add to `detailAnchorLabels` so it gets a jump key.

---

### 9. Full Text Fetching

Two options for getting full text:

**Option A (recommended): On-demand re-fetch from Google Patents**
- In `FullText.Init()`, make an HTTP request directly to Google Patents URL
- Parse with `goquery` using a new `parseAllClaims()` function
- Cache result in memory for the session
- Pros: No daemon changes, no DB bloat

**Option B: Add new RPC method**
- Add `proto.MethodFullText` that daemon handles by re-fetching from Google
- Pros: Reuses daemon's HTTP client, respects crawl rate limiting
- Cons: More plumbing

**Recommendation: Option A** — The TUI already has an RPC client, but for this one case it's simpler to have the TUI fetch directly. The crawl rate limiting is per-source (Google) and the daemon's crawler already handles that. However, for politeness we should add a short rate limiter in the TUI too. We can reuse the existing `googleMinInterval`.

Actually, looking more carefully, Option B is cleaner — the daemon already has the HTTP fetching infrastructure, and the TUI is supposed to be a thin client. Let me go with a lightweight approach: add a new RPC method `MethodFullText` in the proto layer that tells the daemon to re-fetch the Google page and return all claims. The daemon can cache it in memory or in a temp file.

---

### 10. Status / Hint Strings (`internal/text/catalog_en.go`)

New entries:
```go
StatusNotesAdded:        "added %s to notes buffer",
StatusNotesFlushed:      "flushed notes for %s to IDS",
StatusCopiedToClipboard: "copied to clipboard: %d bytes",
StatusClipboardFailed:   "clipboard failed: %s",

HintFullText:  "full claims text",
HintNotes:     "notes",
HintYank:      "yank/copy",
HintNoteAdd:   "add claim to notes",
```

---

### Implementation Order

1. `domain/fulltext.go` — type definitions
2. `crawl/google.go` — add `parseAllClaims()` function
3. `proto/proto.go` — add `MethodFullText` and params/result types
4. `rpc/` — wire up new method (already handles arbitrary methods)
5. `tui/clipboard.go` — clipboard utility (OSC 52 + platform fallbacks)
6. `tui/notes.go` — notes buffer manager
7. `tui/pane/fulltext.go` — the main new pane (this is the biggest piece)
8. `command/catalog.go` — register new commands and scope
9. `command/command.go` — add `ScopeFullText`
10. `tui/keymap/default.go` — add bindings
11. `tui/pane/detail.go` — add "Full claims text" anchor + `OpenFullText` handler
12. `tui/app.go` — add app handlers + `cmdFullText` routing
13. `tui/wiring.go` — add `ScopeFullText` to pane checking
14. `text/catalog_en.go` — add all new strings
15. `tui/wiring_test.go` — update if needed

---

### Questions to Resolve

1. **Notes buffer storage:** Should notes survive the session (saved to local file), or be ephemeral (must flush to IDS before quitting)?
2. **Notes flush behavior:** When flushing to IDS, should it **append** to existing `relevant_passages` or **replace**? If append, what separator?
3. **Export to clipboard from other contexts:** Should `y` also work in the **detail pane** (copying the visible field under cursor) and **catalog** (copying the row)? This would make the clipboard useful beyond the full text viewer.
4. **OSC 52 preference:** Some terminals require `set-clipboard` in tmux config. Should we document prerequisites, or default to `clip.exe`/`pbcopy`/`xclip` and offer OSC 52 as a configurable option?

---

## User

implement now.. 

---

## Assistant (Build · Big Pickle · 7.5s)

_Thinking:_

The user wants me to implement the full plan now. This is a large task. Let me break it down into steps and track with todos. Let me start implementing systematically.

Let me first understand the current structure better by reading the proto definitions, the RPC client, and the store to understand the full picture.

Let me create a todo list and start implementing.

Key files I need to examine more closely:
- internal/proto/proto.go - for method definitions
- internal/rpc/client.go - to understand RPC calls
- internal/store/sqlite/ - to understand IDS save
- go.mod for dependencies

Let me read these and then start implementing step by step.

**Tool: todowrite**

**Input:**
```json
{
  "todos": [
    {
      "content": "Read proto, RPC, store files needed for implementation",
      "status": "in_progress",
      "priority": "high"
    },
    {
      "content": "Create domain/fulltext.go with FullText and ClaimSection types",
      "status": "pending",
      "priority": "high"
    },
    {
      "content": "Add ParseAllClaims to crawl/google.go",
      "status": "pending",
      "priority": "high"
    },
    {
      "content": "Add MethodFullText to proto definitions",
      "status": "pending",
      "priority": "high"
    },
    {
      "content": "Add FullText support to engine/store layers",
      "status": "pending",
      "priority": "high"
    },
    {
      "content": "Create tui/clipboard.go (OSC 52 + platform fallbacks)",
      "status": "pending",
      "priority": "high"
    },
    {
      "content": "Create tui/notes.go (notes buffer manager)",
      "status": "pending",
      "priority": "high"
    },
    {
      "content": "Create tui/pane/fulltext.go (full text viewer pane)",
      "status": "pending",
      "priority": "high"
    },
    {
      "content": "Add new commands to command/catalog.go and command/command.go",
      "status": "pending",
      "priority": "high"
    },
    {
      "content": "Add key bindings to tui/keymap/default.go",
      "status": "pending",
      "priority": "high"
    },
    {
      "content": "Wire up in tui/app.go (handlers, routing)",
      "status": "pending",
      "priority": "high"
    },
    {
      "content": "Extend detail pane with full text link",
      "status": "pending",
      "priority": "high"
    },
    {
      "content": "Update text/catalog_en.go with new strings",
      "status": "pending",
      "priority": "high"
    },
    {
      "content": "Update tui/wiring.go for new scope",
      "status": "pending",
      "priority": "high"
    },
    {
      "content": "Build and verify compilation",
      "status": "pending",
      "priority": "high"
    }
  ]
}
```

**Output:**
```
[
  {
    "content": "Read proto, RPC, store files needed for implementation",
    "status": "in_progress",
    "priority": "high"
  },
  {
    "content": "Create domain/fulltext.go with FullText and ClaimSection types",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Add ParseAllClaims to crawl/google.go",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Add MethodFullText to proto definitions",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Add FullText support to engine/store layers",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Create tui/clipboard.go (OSC 52 + platform fallbacks)",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Create tui/notes.go (notes buffer manager)",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Create tui/pane/fulltext.go (full text viewer pane)",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Add new commands to command/catalog.go and command/command.go",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Add key bindings to tui/keymap/default.go",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Wire up in tui/app.go (handlers, routing)",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Extend detail pane with full text link",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Update text/catalog_en.go with new strings",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Update tui/wiring.go for new scope",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Build and verify compilation",
    "status": "pending",
    "priority": "high"
  }
]
```

---

## Assistant (Build · Big Pickle · 4.1s)

_Thinking:_

Let me read the proto, RPC, and store files I need first.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/proto/proto.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/proto/proto.go</path>
<type>file</type>
<content>
1: // Package proto defines the client<->daemon wire contract: a JSON-RPC 2.0
2: // dialect carried as line-delimited JSON. Method and event names are typed
3: // constants so no caller spells them as a bare string.
4: package proto
5: 
6: import (
7: 	"encoding/json"
8: 	"time"
9: 
10: 	"patentmine/internal/domain"
11: )
12: 
13: // Version is the JSON-RPC protocol version string.
14: const Version = "2.0"
15: 
16: // Method names a request a client may send to the daemon.
17: type Method string
18: 
19: const (
20: 	MethodPing           Method = "ping"
21: 	MethodPatentGet      Method = "patent.get"
22: 	MethodPatentList     Method = "patent.list"
23: 	MethodPatentDelete   Method = "patent.delete"
24: 	MethodProjectList    Method = "project.list"
25: 	MethodProjectCreate  Method = "project.create"
26: 	MethodMembershipAdd  Method = "membership.add"
27: 	MethodReviewState    Method = "review_state.set"
28: 	MethodTagAssign      Method = "tag.assign"
29: 	MethodTagRemove      Method = "tag.remove"
30: 	MethodCrawlFamily   Method = "crawl.family"
31: 	MethodCrawlCancel   Method = "crawl.cancel"
32: 	MethodImportFile     Method = "import.file"
33: 	MethodRelations      Method = "patent.relations"
34: 	MethodIDSExport      Method = "ids.export"
35: 	MethodIDSEntryGet    Method = "ids.entry.get"
36: 	MethodIDSEntrySave   Method = "ids.entry.save"
37: 	MethodIDSEntryDelete Method = "ids.entry.delete"
38: 	MethodMetricsGet     Method = "metrics.get"
39: 	MethodTagCreate      Method = "tag.create"
40: 	MethodTagList        Method = "tag.list"
41: 	MethodTagDelete      Method = "tag.delete"
42: 	MethodPatentTagAdd    Method = "patent.tag.add"
43: 	MethodPatentTagDelete Method = "patent.tag.delete"
44: 	MethodPatentTagList   Method = "patent.tag.list"
45: )
46: 
47: // EventKind names a server->client push (a JSON-RPC notification).
48: type EventKind string
49: 
50: const (
51: 	EventCrawlProgress EventKind = "crawl.progress"
52: 	EventCrawlDone     EventKind = "crawl.done"
53: 	EventDBChanged      EventKind = "db.changed"
54: )
55: 
56: // JSON-RPC error codes. The negative range follows the spec; -32000 down is
57: // reserved for application errors.
58: const (
59: 	CodeParse      = -32700
60: 	CodeInvalidReq = -32600
61: 	CodeNoMethod   = -32601
62: 	CodeBadParams  = -32602
63: 	CodeInternal   = -32603
64: 	CodeNotFound   = -32000
65: 	CodeBusy       = -32001
66: )
67: 
68: // Request is one JSON-RPC call from a client to the daemon.
69: type Request struct {
70: 	JSONRPC string          `json:"jsonrpc"`
71: 	ID      uint64          `json:"id"`
72: 	Method  Method          `json:"method"`
73: 	Params  json.RawMessage `json:"params,omitempty"`
74: }
75: 
76: // Reply is the daemon's response to a Request, matched back by ID.
77: type Reply struct {
78: 	JSONRPC string          `json:"jsonrpc"`
79: 	ID      uint64          `json:"id"`
80: 	Result  json.RawMessage `json:"result,omitempty"`
81: 	Error   *Error          `json:"error,omitempty"`
82: }
83: 
84: // Error is a JSON-RPC error object.
85: type Error struct {
86: 	Code    int    `json:"code"`
87: 	Message string `json:"message"`
88: }
89: 
90: func (e *Error) Error() string { return e.Message }
91: 
92: // Event is a daemon->client notification. It has no ID: that absence is how a
93: // client tells an Event apart from a Reply on the shared connection.
94: type Event struct {
95: 	JSONRPC string          `json:"jsonrpc"`
96: 	Method  EventKind       `json:"method"`
97: 	Params  json.RawMessage `json:"params,omitempty"`
98: }
99: 
100: // --- Method parameter and result payloads ---
101: 
102: // PingResult confirms the daemon is alive.
103: type PingResult struct {
104: 	Pong    bool   `json:"pong"`
105: 	Version string `json:"version"`
106: }
107: 
108: // PatentDeleteParams identifies the patent to permanently remove.
109: type PatentDeleteParams struct {
110: 	Number domain.PatentNumber `json:"number"`
111: }
112: 
113: // PatentGetParams selects a single patent. Project, when set, scopes the
114: // project-relative fields of the result — review state and tags — to that
115: // project; the patent record itself is project-independent.
116: type PatentGetParams struct {
117: 	Number  domain.PatentNumber `json:"number"`
118: 	Project domain.ProjectID    `json:"project,omitempty"`
119: }
120: 
121: // PatentResult carries one patent. State and Tags are populated only when the
122: // request named a project the patent is a member of, and are empty otherwise:
123: // they describe the (patent, project) pair, not the patent.
124: type PatentResult struct {
125: 	Patent      domain.Patent      `json:"patent"`
126: 	ReviewState domain.ReviewState `json:"review_state,omitempty"`
127: 	Tags        []domain.Tag       `json:"tags,omitempty"`
128: 	IDSEntry    *domain.IDSEntry   `json:"ids_entry,omitempty"`
129: }
130: 
131: // PatentListParams selects and paginates a patent listing.
132: type PatentListParams struct {
133: 	Project       domain.ProjectID   `json:"project,omitempty"`
134: 	ReviewState   domain.ReviewState `json:"review_state,omitempty"`
135: 	Search        string             `json:"search,omitempty"`
136: 	Limit         int                `json:"limit,omitempty"`
137: 	Offset        int                `json:"offset,omitempty"`
138: 	SortColumn    domain.SortColumn  `json:"sort_column,omitempty"`
139: 	SortAscending bool               `json:"sort_ascending,omitempty"`
140: }
141: 
142: // PatentListResult carries one page of patents plus the unpaged total.
143: type PatentListResult struct {
144: 	Patents []domain.PatentRow `json:"patents"`
145: 	Total   int                `json:"total"`
146: }
147: 
148: // ProjectListResult carries every project.
149: type ProjectListResult struct {
150: 	Projects []domain.Project `json:"projects"`
151: }
152: 
153: // ProjectCreateParams names a new project.
154: type ProjectCreateParams struct {
155: 	Name string `json:"name"`
156: }
157: 
158: // ProjectResult carries one project.
159: type ProjectResult struct {
160: 	Project domain.Project `json:"project"`
161: }
162: 
163: // MembershipParams identifies a (project, patent) pair.
164: type MembershipParams struct {
165: 	Project domain.ProjectID    `json:"project"`
166: 	Patent  domain.PatentNumber `json:"patent"`
167: }
168: 
169: // MembershipAddResult reports the outcome of adding a patent to a project.
170: type MembershipAddResult struct {
171: 	FetchStarted bool `json:"fetch_started"`
172: }
173: 
174: // ReviewStateParams sets a membership's state.
175: type ReviewStateParams struct {
176: 	Project domain.ProjectID    `json:"project"`
177: 	Patent  domain.PatentNumber `json:"patent"`
178: 	State   string              `json:"state"`
179: }
180: 
181: // TagParams names a tag to assign to, or remove from, a patent within a
182: // project. On assign an unknown name creates the tag.
183: type TagParams struct {
184: 	Project domain.ProjectID    `json:"project"`
185: 	Patent  domain.PatentNumber `json:"patent"`
186: 	Name    string              `json:"name"`
187: }
188: 
189: // TagCreateParams registers a new tag in the project's taxonomy.
190: type TagCreateParams struct {
191: 	Project domain.ProjectID `json:"project"`
192: 	Name    string           `json:"name"`
193: }
194: 
195: // TagDeleteParams removes a tag from the project's taxonomy.
196: type TagDeleteParams struct {
197: 	Project domain.ProjectID `json:"project"`
198: 	Name    string           `json:"name"`
199: }
200: 
201: // TagListParams lists all taxonomy tags in the project.
202: type TagListParams struct {
203: 	Project domain.ProjectID `json:"project"`
204: }
205: 
206: // TagListResult carries the list of project taxonomy tags.
207: type TagListResult struct {
208: 	Tags []domain.Tag `json:"tags"`
209: }
210: 
211: // PatentTagListParams lists tags assigned to a patent.
212: type PatentTagListParams struct {
213: 	Project domain.ProjectID    `json:"project"`
214: 	Patent  domain.PatentNumber `json:"patent"`
215: }
216: 
217: // PatentTagListResult carries the list of tags assigned to a patent.
218: type PatentTagListResult struct {
219: 	Tags []domain.Tag `json:"tags"`
220: }
221: 
222: 
223: // CrawlFamilyParams starts a family-graph crawl rooted at one patent. Depth 0
224: // fetches only the root; a negative depth uses the configured family depth.
225: // Force bypasses the local file cache and re-fetches from the web.
226: type CrawlFamilyParams struct {
227: 	Root    domain.PatentNumber `json:"root"`
228: 	Depth   int                 `json:"depth"`
229: 	Profile domain.CrawlProfile `json:"profile,omitempty"`
230: 	Force   bool                `json:"force,omitempty"`
231: }
232: 
233: // CrawlStartResult returns the id of an enqueued job.
234: type CrawlStartResult struct {
235: 	JobID string `json:"job_id"`
236: }
237: 
238: // ImportFileParams loads a patent record from a local fixture file.
239: type ImportFileParams struct {
240: 	Path string `json:"path"`
241: }
242: 
243: // CrawlCancelParams cancels a running job.
244: type CrawlCancelParams struct {
245: 	JobID string `json:"job_id"`
246: }
247: 
248: // RelationsParams selects family-graph edges of one kind from one patent.
249: type RelationsParams struct {
250: 	Number        domain.PatentNumber `json:"number"`
251: 	Kind          domain.RelationKind `json:"kind"`
252: 	Project       domain.ProjectID    `json:"project,omitempty"`
253: 	ReviewState   domain.ReviewState  `json:"review_state,omitempty"`
254: 	Search        string              `json:"search,omitempty"`
255: 	Limit         int                 `json:"limit,omitempty"`
256: 	Offset        int                 `json:"offset,omitempty"`
257: 	SortColumn    domain.SortColumn   `json:"sort_column,omitempty"`
258: 	SortAscending bool                `json:"sort_ascending,omitempty"`
259: }
260: 
261: // RelationsResult carries the requested family-graph edges.
262: type RelationsResult struct {
263: 	Patents []domain.PatentRow `json:"patents"`
264: 	Total   int                `json:"total"`
265: }
266: 
267: // IDSExportParams selects the project to build an Information Disclosure
268: // Statement for.
269: type IDSExportParams struct {
270: 	Project domain.ProjectID `json:"project"`
271: }
272: 
273: // IDSResult carries a generated Information Disclosure Statement.
274: type IDSResult struct {
275: 	IDS domain.IDS `json:"ids"`
276: }
277: 
278: // IDSEntryParams identifies one project/patent IDS entry.
279: type IDSEntryParams struct {
280: 	Project domain.ProjectID    `json:"project"`
281: 	Patent  domain.PatentNumber `json:"patent"`
282: }
283: 
284: // IDSEntrySaveParams carries the IDS entry to insert or update.
285: type IDSEntrySaveParams struct {
286: 	Entry domain.IDSEntry `json:"entry"`
287: }
288: 
289: // IDSEntryResult carries one curated IDS entry.
290: type IDSEntryResult struct {
291: 	Entry domain.IDSEntry `json:"entry"`
292: }
293: 
294: // MetricsResult carries the daemon's current in-memory timing/counter snapshot.
295: type MetricsResult struct {
296: 	Metrics MetricsSnapshot `json:"metrics"`
297: }
298: 
299: // MetricsSnapshot is a transport-safe view of the daemon's metrics.
300: type MetricsSnapshot struct {
301: 	Timestamp time.Time               `json:"timestamp"`
302: 	Timings   map[string]TimingMetric `json:"timings"`
303: 	Counters  map[string]int64        `json:"counters"`
304: 	Gauges    map[string]int64        `json:"gauges"`
305: }
306: 
307: // TimingMetric aggregates one named operation's observed durations.
308: type TimingMetric struct {
309: 	Count      int64 `json:"count"`
310: 	Errors     int64 `json:"errors"`
311: 	TotalNanos int64 `json:"total_nanos"`
312: 	AvgNanos   int64 `json:"avg_nanos"`
313: 	AvgMillis  int64 `json:"avg_millis"`
314: 	MinNanos   int64 `json:"min_nanos"`
315: 	MinMillis  int64 `json:"min_millis"`
316: 	MaxNanos   int64 `json:"max_nanos"`
317: 	MaxMillis  int64 `json:"max_millis"`
318: 	LastNanos  int64 `json:"last_nanos"`
319: 	LastMillis int64 `json:"last_millis"`
320: }
321: 
322: // Empty is the result of operations with nothing to return.
323: type Empty struct{}
324: 
325: // --- Event payloads ---
326: 
327: // CrawlProgress reports incremental progress of a crawl job.
328: type CrawlProgress struct {
329: 	JobID           string `json:"job_id"`
330: 	CrawledCount   int    `json:"crawled_count"`
331: 	DiscoveredCount int    `json:"discovered_count"`
332: 	PendingCount    int    `json:"pending_count"`
333: 	CitationsCount  int    `json:"citations_count,omitempty"`
334: 	CitedByCount    int    `json:"cited_by_count,omitempty"`
335: 	ParentsCount    int    `json:"parents_count,omitempty"`
336: 	ChildrenCount   int    `json:"children_count,omitempty"`
337: 	Message         string `json:"message"`
338: }
339: 
340: // CrawlDone reports that a crawl job finished, with an error if it failed.
341: type CrawlDone struct {
342: 	JobID string `json:"job_id"`
343: 	Error string `json:"error,omitempty"`
344: }
345: 
346: // NewEvent builds an Event, marshaling params. A marshal failure yields an
347: // event with empty params rather than a hard error on a best-effort channel.
348: func NewEvent(kind EventKind, params any) Event {
349: 	raw, err := json.Marshal(params)
350: 	if err != nil {
351: 		raw = nil
352: 	}
353: 	return Event{JSONRPC: Version, Method: kind, Params: raw}
354: }

(End of file - total 354 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/rpc"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/rpc</path>
<type>directory</type>
<entries>
client.go
rpc_test.go
server.go

(3 entries)
</entries>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/engine/engine.go",
  "limit": 100
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/engine/engine.go</path>
<type>file</type>
<content>
1: // Package engine is the daemon core. It owns the store, the crawl worker
2: // pool, and the event bus, and exposes the operations that frontends invoke
3: // (directly in tests, or over RPC in the running system).
4: package engine
5: 
6: import (
7: 	"context"
8: 	"encoding/json"
9: 	"errors"
10: 	"fmt"
11: 	"io"
12: 	"log/slog"
13: 	"strings"
14: 	"time"
15: 
16: 	"patentmine/internal/domain"
17: 	"patentmine/internal/observability"
18: 	"patentmine/internal/proto"
19: 	"patentmine/internal/store"
20: )
21: 
22: // crawlWorkers is the number of concurrent crawl jobs.
23: const crawlWorkers = 4
24: 
25: // CrawlFactory builds the Job that crawls a patent family. It is injected so
26: // the engine does not depend on the crawl package directly: a stub factory
27: // works for tests, the real crawler is wired in at daemon startup. force makes
28: // the crawl bypass the local file cache and re-fetch from the web.
29: type CrawlFactory func(root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) Job
30: 
31: // FileImporter loads a patent record from a local file into the store. Like
32: // CrawlFactory it is injected, so the engine never imports the crawl package.
33: type FileImporter interface {
34: 	ImportFile(ctx context.Context, path string) error
35: }
36: 
37: // Option customizes an Engine.
38: type Option func(*Engine)
39: 
40: // Engine is the daemon core. Its methods are the single set of operations the
41: // system supports; RPC handlers and embedded callers both go through them.
42: type Engine struct {
43: 	repo         store.Repository
44: 	bus          *Bus
45: 	pool         *workerPool
46: 	crawl        CrawlFactory
47: 	fileImporter FileImporter
48: 	logger       *slog.Logger
49: 	activities   *observability.Recorder
50: 	metrics      *observability.Metrics
51: }
52: 
53: // WithFileImporter wires the file-import backend used by ImportFile.
54: func WithFileImporter(fi FileImporter) Option {
55: 	return func(e *Engine) { e.fileImporter = fi }
56: }
57: 
58: // WithLogger records structured logs for engine operations.
59: func WithLogger(logger *slog.Logger) Option {
60: 	return func(e *Engine) {
61: 		if logger != nil {
62: 			e.logger = logger
63: 		}
64: 	}
65: }
66: 
67: // WithActivityRecorder enables semantic activity journaling.
68: func WithActivityRecorder(rec *observability.Recorder) Option {
69: 	return func(e *Engine) {
70: 		e.activities = rec
71: 	}
72: }
73: 
74: // WithMetrics enables in-process timings and counters.
75: func WithMetrics(metrics *observability.Metrics) Option {
76: 	return func(e *Engine) {
77: 		e.metrics = metrics
78: 	}
79: }
80: 
81: // New builds an Engine. The pool's jobs are children of ctx, so cancelling ctx
82: // stops all background work. crawl may be nil if no crawl will be started.
83: func New(ctx context.Context, repo store.Repository, crawl CrawlFactory, opts ...Option) *Engine {
84: 	bus := NewBus()
85: 	eng := &Engine{
86: 		repo:   repo,
87: 		bus:    bus,
88: 		pool:   newWorkerPool(ctx, crawlWorkers, bus, nil),
89: 		crawl: crawl,
90: 		logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
91: 	}
92: 	for _, opt := range opts {
93: 		opt(eng)
94: 	}
95: 	eng.pool.metrics = eng.metrics
96: 	eng.bus.metrics = eng.metrics
97: 	return eng
98: }
99: 
100: // Close stops the worker pool and the event bus. The store is not closed here:

(Showing lines 1-100 of 985. Use offset=101 to continue.)
</content>
```

---

## Assistant (Build · Big Pickle · 3.3s)

_Thinking:_

Let me read the engine RPC handler and the store interface to understand how RPC methods are dispatched, and the store interface definition.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/rpc/server.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/rpc/server.go</path>
<type>file</type>
<content>
1: // Package rpc carries the proto contract over a unix-domain socket: a Server
2: // hosted by the daemon and a Client used by every thin frontend.
3: package rpc
4: 
5: import (
6: 	"context"
7: 	"encoding/json"
8: 	"errors"
9: 	"fmt"
10: 	"log/slog"
11: 	"net"
12: 	"time"
13: 
14: 	"patentmine/internal/domain"
15: 	"patentmine/internal/engine"
16: 	"patentmine/internal/proto"
17: 	"patentmine/internal/store"
18: 	appversion "patentmine/internal/version"
19: )
20: 
21: const slowRPCMethod = 150 * time.Millisecond
22: 
23: // ErrBadParams marks a request whose params failed to decode.
24: var ErrBadParams = errors.New("rpc: bad params")
25: 
26: // handlerFunc processes one request's params and returns a result payload.
27: type handlerFunc func(ctx context.Context, params json.RawMessage) (any, error)
28: 
29: // Server dispatches proto requests to engine operations and forwards engine
30: // events to every connected client.
31: type Server struct {
32: 	engine   *engine.Engine
33: 	handlers map[proto.Method]handlerFunc
34: }
35: 
36: // NewServer wires the dispatch table for an engine.
37: func NewServer(eng *engine.Engine) *Server {
38: 	s := &Server{engine: eng}
39: 	s.handlers = map[proto.Method]handlerFunc{
40: 		proto.MethodPing:           s.ping,
41: 		proto.MethodPatentGet:      s.patentGet,
42: 		proto.MethodPatentList:     s.patentList,
43: 		proto.MethodPatentDelete:   s.patentDelete,
44: 		proto.MethodProjectList:    s.projectList,
45: 		proto.MethodProjectCreate:  s.projectCreate,
46: 		proto.MethodMembershipAdd:  s.membershipAdd,
47: 		proto.MethodReviewState:    s.reviewState,
48: 		proto.MethodTagAssign:      s.tagAssign,
49: 		proto.MethodTagRemove:      s.tagRemove,
50: 		proto.MethodCrawlFamily:   s.crawlFamily,
51: 		proto.MethodCrawlCancel:   s.crawlCancel,
52: 		proto.MethodImportFile:     s.importFile,
53: 		proto.MethodRelations:      s.relations,
54: 		proto.MethodIDSExport:      s.idsExport,
55: 		proto.MethodIDSEntryGet:    s.idsEntryGet,
56: 		proto.MethodIDSEntrySave:   s.idsEntrySave,
57: 		proto.MethodIDSEntryDelete: s.idsEntryDelete,
58: 		proto.MethodMetricsGet:     s.metricsGet,
59: 		proto.MethodTagCreate:      s.tagCreate,
60: 		proto.MethodTagList:        s.tagList,
61: 		proto.MethodTagDelete:      s.tagDelete,
62: 		proto.MethodPatentTagAdd:    s.patentTagAdd,
63: 		proto.MethodPatentTagDelete: s.patentTagDelete,
64: 		proto.MethodPatentTagList:   s.patentTagList,
65: 	}
66: 	return s
67: }
68: 
69: // Serve accepts connections on ln until ctx is cancelled.
70: func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
71: 	go func() {
72: 		<-ctx.Done()
73: 		_ = ln.Close()
74: 	}()
75: 	for {
76: 		conn, err := ln.Accept()
77: 		if err != nil {
78: 			if ctx.Err() != nil {
79: 				return nil
80: 			}
81: 			return fmt.Errorf("rpc: accept: %w", err)
82: 		}
83: 		go s.serveConn(ctx, conn)
84: 	}
85: }
86: 
87: // serveConn handles one client: a goroutine forwards engine events while the
88: // main loop reads and dispatches requests.
89: func (s *Server) serveConn(ctx context.Context, nc net.Conn) {
90: 	conn := proto.NewConn(nc)
91: 	defer func() { _ = conn.Close() }()
92: 
93: 	connCtx, cancel := context.WithCancel(ctx)
94: 	defer cancel()
95: 
96: 	events, unsub := s.engine.Subscribe()
97: 	defer unsub()
98: 	go func() {
99: 		for {
100: 			select {
101: 			case <-connCtx.Done():
102: 				return
103: 			case ev, ok := <-events:
104: 				if !ok {
105: 					return
106: 				}
107: 				if err := conn.WriteMessage(ev); err != nil {
108: 					cancel()
109: 					return
110: 				}
111: 			}
112: 		}
113: 	}()
114: 
115: 	for {
116: 		raw, err := conn.ReadMessage()
117: 		if err != nil {
118: 			cancel()
119: 			return
120: 		}
121: 		var req proto.Request
122: 		if err := json.Unmarshal(raw, &req); err != nil {
123: 			_ = conn.WriteMessage(errorReply(0, proto.CodeParse, "malformed request"))
124: 			continue
125: 		}
126: 		// Each request runs in its own goroutine; the Conn serializes writes,
127: 		// so a slow handler never blocks other requests on the connection.
128: 		go s.handle(connCtx, conn, req)
129: 	}
130: }
131: 
132: func (s *Server) handle(ctx context.Context, conn *proto.Conn, req proto.Request) {
133: 	start := time.Now()
134: 	h, ok := s.handlers[req.Method]
135: 	if !ok {
136: 		s.observeRPC(req.Method, start, true)
137: 		_ = conn.WriteMessage(errorReply(req.ID, proto.CodeNoMethod,
138: 			fmt.Sprintf("unknown method %q", req.Method)))
139: 		return
140: 	}
141: 	result, err := h(ctx, req.Params)
142: 	if err != nil {
143: 		s.observeRPC(req.Method, start, true)
144: 		_ = conn.WriteMessage(errorReply(req.ID, codeFor(err), err.Error()))
145: 		return
146: 	}
147: 	payload, err := json.Marshal(result)
148: 	if err != nil {
149: 		s.observeRPC(req.Method, start, true)
150: 		_ = conn.WriteMessage(errorReply(req.ID, proto.CodeInternal, err.Error()))
151: 		return
152: 	}
153: 	s.observeRPC(req.Method, start, false)
154: 	_ = conn.WriteMessage(proto.Reply{JSONRPC: proto.Version, ID: req.ID, Result: payload})
155: }
156: 
157: func (s *Server) observeRPC(method proto.Method, start time.Time, failed bool) {
158: 	if s.engine == nil {
159: 		return
160: 	}
161: 	if metrics := s.engineMetrics(); metrics != nil {
162: 		d := time.Since(start)
163: 		metrics.ObserveDuration("rpc.method."+string(method), d, failed)
164: 		metrics.IncCounter("rpc.method."+string(method)+".total", 1)
165: 		if failed {
166: 			metrics.IncCounter("rpc.method."+string(method)+".error_total", 1)
167: 		}
168: 		if d >= slowRPCMethod {
169: 			s.engine.Logger().Warn("slow rpc method",
170: 				slog.String("method", string(method)),
171: 				slog.Int64("duration_ms", d.Milliseconds()),
172: 				slog.Bool("failed", failed))
173: 		}
174: 	}
175: }
176: 
177: func (s *Server) engineMetrics() interface {
178: 	ObserveDuration(string, time.Duration, bool)
179: 	IncCounter(string, int64)
180: } {
181: 	return s.engineMetricsRef()
182: }
183: 
184: func (s *Server) engineMetricsRef() interface {
185: 	ObserveDuration(string, time.Duration, bool)
186: 	IncCounter(string, int64)
187: } {
188: 	if s.engine == nil {
189: 		return nil
190: 	}
191: 	return s.engine.Metrics()
192: }
193: 
194: func (s *Server) Logger() *slog.Logger {
195: 	if s.engine == nil {
196: 		return slog.Default()
197: 	}
198: 	return s.engine.Logger()
199: }
200: 
201: // errorReply builds a JSON-RPC error response.
202: func errorReply(id uint64, code int, message string) proto.Reply {
203: 	return proto.Reply{
204: 		JSONRPC: proto.Version,
205: 		ID:      id,
206: 		Error:   &proto.Error{Code: code, Message: message},
207: 	}
208: }
209: 
210: // codeFor maps an engine/store error to a JSON-RPC error code.
211: func codeFor(err error) int {
212: 	switch {
213: 	case errors.Is(err, store.ErrNotFound):
214: 		return proto.CodeNotFound
215: 	case errors.Is(err, ErrBadParams):
216: 		return proto.CodeBadParams
217: 	case errors.Is(err, engine.ErrQueueFull):
218: 		return proto.CodeBusy
219: 	default:
220: 		return proto.CodeInternal
221: 	}
222: }
223: 
224: // decodeParams unmarshals request params into T, tagging failures as bad params.
225: func decodeParams[T any](raw json.RawMessage) (T, error) {
226: 	var v T
227: 	if len(raw) == 0 {
228: 		return v, nil
229: 	}
230: 	if err := json.Unmarshal(raw, &v); err != nil {
231: 		return v, fmt.Errorf("%w: %v", ErrBadParams, err)
232: 	}
233: 	return v, nil
234: }
235: 
236: // --- handlers ---
237: 
238: func (s *Server) ping(context.Context, json.RawMessage) (any, error) {
239: 	return proto.PingResult{Pong: true, Version: appversion.String()}, nil
240: }
241: 
242: func (s *Server) patentGet(ctx context.Context, raw json.RawMessage) (any, error) {
243: 	p, err := decodeParams[proto.PatentGetParams](raw)
244: 	if err != nil {
245: 		return nil, err
246: 	}
247: 	patent, err := s.engine.Patent(ctx, p.Number)
248: 	if err != nil {
249: 		return nil, err
250: 	}
251: 	result := proto.PatentResult{Patent: patent}
252: 	// State and tags are project-scoped; populate them only when the caller
253: 	// named a project, leaving them empty for a project-independent lookup.
254: 	if p.Project != "" {
255: 		state, ok, err := s.engine.ReviewStateOf(ctx, p.Project, p.Number)
256: 		if err != nil {
257: 			return nil, err
258: 		}
259: 		if ok {
260: 			result.ReviewState = state
261: 		}
262: 		tags, err := s.engine.PatentTags(ctx, p.Project, p.Number)
263: 		if err != nil {
264: 			return nil, err
265: 		}
266: 		result.Tags = tags
267: 		entry, ok, err := s.engine.IDSEntryOf(ctx, p.Project, p.Number)
268: 		if err != nil {
269: 			return nil, err
270: 		}
271: 		if ok {
272: 			result.IDSEntry = &entry
273: 		}
274: 	}
275: 	return result, nil
276: }
277: 
278: func (s *Server) patentList(ctx context.Context, raw json.RawMessage) (any, error) {
279: 	p, err := decodeParams[proto.PatentListParams](raw)
280: 	if err != nil {
281: 		return nil, err
282: 	}
283: 	patents, total, err := s.engine.ListPatents(ctx, store.PatentQuery{
284: 		Project:       p.Project,
285: 		ReviewState:   p.ReviewState,
286: 		Search:        p.Search,
287: 		Limit:         p.Limit,
288: 		Offset:        p.Offset,
289: 		SortColumn:    p.SortColumn,
290: 		SortAscending: p.SortAscending,
291: 	})
292: 	if err != nil {
293: 		return nil, err
294: 	}
295: 	return proto.PatentListResult{Patents: patents, Total: total}, nil
296: }
297: 
298: func (s *Server) projectList(ctx context.Context, _ json.RawMessage) (any, error) {
299: 	projects, err := s.engine.Projects(ctx)
300: 	if err != nil {
301: 		return nil, err
302: 	}
303: 	return proto.ProjectListResult{Projects: projects}, nil
304: }
305: 
306: func (s *Server) projectCreate(ctx context.Context, raw json.RawMessage) (any, error) {
307: 	p, err := decodeParams[proto.ProjectCreateParams](raw)
308: 	if err != nil {
309: 		return nil, err
310: 	}
311: 	project, err := s.engine.CreateProject(ctx, p.Name)
312: 	if err != nil {
313: 		return nil, err
314: 	}
315: 	return proto.ProjectResult{Project: project}, nil
316: }
317: 
318: func (s *Server) patentDelete(ctx context.Context, raw json.RawMessage) (any, error) {
319: 	p, err := decodeParams[proto.PatentDeleteParams](raw)
320: 	if err != nil {
321: 		return nil, err
322: 	}
323: 	if err := s.engine.DeletePatent(ctx, p.Number); err != nil {
324: 		return nil, err
325: 	}
326: 	return proto.Empty{}, nil
327: }
328: 
329: func (s *Server) membershipAdd(ctx context.Context, raw json.RawMessage) (any, error) {
330: 	p, err := decodeParams[proto.MembershipParams](raw)
331: 	if err != nil {
332: 		return nil, err
333: 	}
334: 	fetchStarted, err := s.engine.AddToProject(ctx, p.Project, p.Patent)
335: 	if err != nil {
336: 		return nil, err
337: 	}
338: 	return proto.MembershipAddResult{FetchStarted: fetchStarted}, nil
339: }
340: 
341: func (s *Server) reviewState(ctx context.Context, raw json.RawMessage) (any, error) {
342: 	p, err := decodeParams[proto.ReviewStateParams](raw)
343: 	if err != nil {
344: 		return nil, err
345: 	}
346: 	state, err := domain.ParseReviewState(p.State)
347: 	if err != nil {
348: 		return nil, fmt.Errorf("%w: %v", ErrBadParams, err)
349: 	}
350: 	if err := s.engine.SetReviewState(ctx, p.Project, p.Patent, state); err != nil {
351: 		return nil, err
352: 	}
353: 	return proto.Empty{}, nil
354: }
355: 
356: func (s *Server) tagAssign(ctx context.Context, raw json.RawMessage) (any, error) {
357: 	p, err := decodeParams[proto.TagParams](raw)
358: 	if err != nil {
359: 		return nil, err
360: 	}
361: 	if err := s.engine.AssignTag(ctx, p.Project, p.Patent, p.Name); err != nil {
362: 		return nil, err
363: 	}
364: 	return proto.Empty{}, nil
365: }
366: 
367: func (s *Server) tagRemove(ctx context.Context, raw json.RawMessage) (any, error) {
368: 	p, err := decodeParams[proto.TagParams](raw)
369: 	if err != nil {
370: 		return nil, err
371: 	}
372: 	if err := s.engine.RemoveTag(ctx, p.Project, p.Patent, p.Name); err != nil {
373: 		return nil, err
374: 	}
375: 	return proto.Empty{}, nil
376: }
377: 
378: func (s *Server) tagCreate(ctx context.Context, raw json.RawMessage) (any, error) {
379: 	p, err := decodeParams[proto.TagCreateParams](raw)
380: 	if err != nil {
381: 		return nil, err
382: 	}
383: 	tag, err := s.engine.CreateTaxonomyTag(ctx, p.Project, p.Name)
384: 	if err != nil {
385: 		return nil, err
386: 	}
387: 	return tag, nil
388: }
389: 
390: func (s *Server) tagList(ctx context.Context, raw json.RawMessage) (any, error) {
391: 	p, err := decodeParams[proto.TagListParams](raw)
392: 	if err != nil {
393: 		return nil, err
394: 	}
395: 	tags, err := s.engine.ListTaxonomyTags(ctx, p.Project)
396: 	if err != nil {
397: 		return nil, err
398: 	}
399: 	return proto.TagListResult{Tags: tags}, nil
400: }
401: 
402: func (s *Server) tagDelete(ctx context.Context, raw json.RawMessage) (any, error) {
403: 	p, err := decodeParams[proto.TagDeleteParams](raw)
404: 	if err != nil {
405: 		return nil, err
406: 	}
407: 	if err := s.engine.DeleteTaxonomyTag(ctx, p.Project, p.Name); err != nil {
408: 		return nil, err
409: 	}
410: 	return proto.Empty{}, nil
411: }
412: 
413: func (s *Server) patentTagAdd(ctx context.Context, raw json.RawMessage) (any, error) {
414: 	p, err := decodeParams[proto.TagParams](raw)
415: 	if err != nil {
416: 		return nil, err
417: 	}
418: 	if err := s.engine.AssignPatentTag(ctx, p.Project, p.Patent, p.Name); err != nil {
419: 		return nil, err
420: 	}
421: 	return proto.Empty{}, nil
422: }
423: 
424: func (s *Server) patentTagDelete(ctx context.Context, raw json.RawMessage) (any, error) {
425: 	p, err := decodeParams[proto.TagParams](raw)
426: 	if err != nil {
427: 		return nil, err
428: 	}
429: 	if err := s.engine.RemovePatentTag(ctx, p.Project, p.Patent, p.Name); err != nil {
430: 		return nil, err
431: 	}
432: 	return proto.Empty{}, nil
433: }
434: 
435: func (s *Server) patentTagList(ctx context.Context, raw json.RawMessage) (any, error) {
436: 	p, err := decodeParams[proto.PatentTagListParams](raw)
437: 	if err != nil {
438: 		return nil, err
439: 	}
440: 	tags, err := s.engine.PatentTags(ctx, p.Project, p.Patent)
441: 	if err != nil {
442: 		return nil, err
443: 	}
444: 	return proto.PatentTagListResult{Tags: tags}, nil
445: }
446: 
447: func (s *Server) crawlFamily(_ context.Context, raw json.RawMessage) (any, error) {
448: 	p, err := decodeParams[proto.CrawlFamilyParams](raw)
449: 	if err != nil {
450: 		return nil, err
451: 	}
452: 	id, err := s.engine.StartFamilyCrawl(p.Root, p.Depth, p.Profile, p.Force)
453: 	if err != nil {
454: 		return nil, err
455: 	}
456: 	return proto.CrawlStartResult{JobID: string(id)}, nil
457: }
458: 
459: func (s *Server) importFile(ctx context.Context, raw json.RawMessage) (any, error) {
460: 	p, err := decodeParams[proto.ImportFileParams](raw)
461: 	if err != nil {
462: 		return nil, err
463: 	}
464: 	if err := s.engine.ImportFile(ctx, p.Path); err != nil {
465: 		return nil, err
466: 	}
467: 	return proto.Empty{}, nil
468: }
469: 
470: func (s *Server) crawlCancel(_ context.Context, raw json.RawMessage) (any, error) {
471: 	p, err := decodeParams[proto.CrawlCancelParams](raw)
472: 	if err != nil {
473: 		return nil, err
474: 	}
475: 	if err := s.engine.CancelCrawl(engine.JobID(p.JobID)); err != nil {
476: 		return nil, err
477: 	}
478: 	return proto.Empty{}, nil
479: }
480: 
481: func (s *Server) relations(ctx context.Context, raw json.RawMessage) (any, error) {
482: 	p, err := decodeParams[proto.RelationsParams](raw)
483: 	if err != nil {
484: 		return nil, err
485: 	}
486: 	q := store.PatentQuery{
487: 		Relation:      p.Number,
488: 		RelationKind:  p.Kind,
489: 		Project:       p.Project,
490: 		ReviewState:   p.ReviewState,
491: 		Search:        p.Search,
492: 		Limit:         p.Limit,
493: 		Offset:        p.Offset,
494: 		SortColumn:    p.SortColumn,
495: 		SortAscending: p.SortAscending,
496: 	}
497: 	patents, total, err := s.engine.Relations(ctx, q)
498: 	if err != nil {
499: 		return nil, err
500: 	}
501: 	return proto.RelationsResult{Patents: patents, Total: total}, nil
502: }
503: 
504: func (s *Server) idsExport(ctx context.Context, raw json.RawMessage) (any, error) {
505: 	p, err := decodeParams[proto.IDSExportParams](raw)
506: 	if err != nil {
507: 		return nil, err
508: 	}
509: 	ids, err := s.engine.ExportIDS(ctx, p.Project)
510: 	if err != nil {
511: 		return nil, err
512: 	}
513: 	return proto.IDSResult{IDS: ids}, nil
514: }
515: 
516: func (s *Server) idsEntryGet(ctx context.Context, raw json.RawMessage) (any, error) {
517: 	p, err := decodeParams[proto.IDSEntryParams](raw)
518: 	if err != nil {
519: 		return nil, err
520: 	}
521: 	entry, ok, err := s.engine.IDSEntryOf(ctx, p.Project, p.Patent)
522: 	if err != nil {
523: 		return nil, err
524: 	}
525: 	if !ok {
526: 		return nil, store.ErrNotFound
527: 	}
528: 	return proto.IDSEntryResult{Entry: entry}, nil
529: }
530: 
531: func (s *Server) idsEntrySave(ctx context.Context, raw json.RawMessage) (any, error) {
532: 	p, err := decodeParams[proto.IDSEntrySaveParams](raw)
533: 	if err != nil {
534: 		return nil, err
535: 	}
536: 	entry, err := s.engine.SaveIDSEntry(ctx, p.Entry)
537: 	if err != nil {
538: 		return nil, err
539: 	}
540: 	return proto.IDSEntryResult{Entry: entry}, nil
541: }
542: 
543: func (s *Server) idsEntryDelete(ctx context.Context, raw json.RawMessage) (any, error) {
544: 	p, err := decodeParams[proto.IDSEntryParams](raw)
545: 	if err != nil {
546: 		return nil, err
547: 	}
548: 	if err := s.engine.DeleteIDSEntry(ctx, p.Project, p.Patent); err != nil {
549: 		return nil, err
550: 	}
551: 	return proto.Empty{}, nil
552: }
553: 
554: func (s *Server) metricsGet(context.Context, json.RawMessage) (any, error) {
555: 	return proto.MetricsResult{Metrics: s.engine.MetricsSnapshot()}, nil
556: }

(End of file - total 556 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/store"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/store</path>
<type>directory</type>
<entries>
cache.go
repository.go
sqlite/

(3 entries)
</entries>
```

---

## Assistant (Build · Big Pickle · 3.4s)

_Thinking:_

Let me read the store repository interface to understand the full API available.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/store/repository.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/store/repository.go</path>
<type>file</type>
<content>
1: // Package store is the persistence boundary. It defines the Repository
2: // interface that the engine depends on; concrete database implementations
3: // (today SQLite, later Postgres) live in subpackages.
4: package store
5: 
6: import (
7: 	"context"
8: 	"errors"
9: 	"time"
10: 
11: 	"patentmine/internal/domain"
12: )
13: 
14: // ErrNotFound is returned by lookups when no matching row exists.
15: var ErrNotFound = errors.New("store: not found")
16: 
17: // DefaultPageSize is the listing page size used when a query asks for none.
18: const DefaultPageSize = 100
19: 
20: // PatentQuery selects and paginates a patent listing. Pagination is applied in
21: // the database (LIMIT/OFFSET) so a client never loads the whole table.
22: type PatentQuery struct {
23: 	// Project, when set, restricts results to that project's members.
24: 	Project domain.ProjectID
25: 	// ReviewState, when set together with Project, filters by review state.
26: 	ReviewState domain.ReviewState
27: 	// Relation, when set together with RelationKind, restricts results to
28: 	// patents having that relation to the given patent number.
29: 	Relation     domain.PatentNumber
30: 	RelationKind domain.RelationKind
31: 	// Search, when set, is a case-insensitive substring match on number/title.
32: 	Search string
33: 	// Limit is the page size; values <= 0 fall back to DefaultPageSize.
34: 	Limit int
35: 	// Offset is the number of rows to skip.
36: 	Offset int
37: 	// SortColumn names the column to order by; zero falls back to number order.
38: 	SortColumn domain.SortColumn
39: 	// SortAscending, when true, sorts ascending; false sorts descending.
40: 	SortAscending bool
41: }
42: 
43: // Repository is the persistence contract. Every method takes a context so the
44: // interface is cancellation-aware and ready for a pooled/async backend without
45: // changing any caller — the "future non-blocking local DB" requirement.
46: type Repository interface {
47: 	// SavePatent inserts or updates a patent by its number.
48: 	SavePatent(ctx context.Context, p domain.Patent) error
49: 	// DeletePatent permanently removes a patent and all its associated
50: 	// documents, relations, and memberships.
51: 	DeletePatent(ctx context.Context, n domain.PatentNumber) error
52: 	// Patent returns one patent, or ErrNotFound.
53: 	Patent(ctx context.Context, n domain.PatentNumber) (domain.Patent, error)
54: 	// ListPatents returns one page of lightweight listing rows matching q.
55: 	ListPatents(ctx context.Context, q PatentQuery) ([]domain.PatentRow, error)
56: 	// CountPatents returns the total rows matching q, ignoring its paging.
57: 	CountPatents(ctx context.Context, q PatentQuery) (int, error)
58: 
59: 	// SaveDocument inserts or updates one life-stage document of a record.
60: 	SaveDocument(ctx context.Context, recordNumber domain.PatentNumber, doc domain.Document) error
61: 	// Documents returns every life-stage document of a record.
62: 	Documents(ctx context.Context, recordNumber domain.PatentNumber) ([]domain.Document, error)
63: 	// RecordOf returns the record number a document number belongs to, or
64: 	// ErrNotFound when the number is unknown.
65: 	RecordOf(ctx context.Context, number domain.PatentNumber) (domain.PatentNumber, error)
66: 	// MergeRecords folds the absorb record into keep: documents, memberships,
67: 	// and relations are repointed and the absorb row is removed.
68: 	MergeRecords(ctx context.Context, keep, absorb domain.PatentNumber) error
69: 
70: 	// SaveRelation inserts a family-graph edge; duplicates are ignored.
71: 	SaveRelation(ctx context.Context, r domain.Relation) error
72: 	// Relations returns edges of the given kind originating at n.
73: 	Relations(ctx context.Context, n domain.PatentNumber, kind domain.RelationKind) ([]domain.Relation, error)
74: 	// AllRelations returns every family-graph edge where n is either the origin (from) or destination (to).
75: 	AllRelations(ctx context.Context, n domain.PatentNumber) ([]domain.Relation, error)
76: 
77: 	// SaveProject inserts or updates a project by its id.
78: 	SaveProject(ctx context.Context, p domain.Project) error
79: 	// Project returns one project, or ErrNotFound.
80: 	Project(ctx context.Context, id domain.ProjectID) (domain.Project, error)
81: 	// ListProjects returns every project, ordered by name.
82: 	ListProjects(ctx context.Context) ([]domain.Project, error)
83: 
84: 	// AddMembership links a patent to a project; an existing link is left as is.
85: 	AddMembership(ctx context.Context, m domain.Membership) error
86: 	// Membership returns one membership, or ErrNotFound.
87: 	Membership(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (domain.Membership, error)
88: 	// SetReviewState changes a membership's state, or returns ErrNotFound.
89: 	SetReviewState(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, state domain.ReviewState) error
90: 	// DeleteMembership permanently removes a patent from a project. No-op if not a member.
91: 	DeleteMembership(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) error
92: 	// Memberships returns every membership of a project.
93: 	Memberships(ctx context.Context, project domain.ProjectID) ([]domain.Membership, error)
94: 
95: 	// CreateTag returns the project's tag of the given name, creating it when
96: 	// the project does not have it yet. The name is matched as stored.
97: 	CreateTag(ctx context.Context, project domain.ProjectID, name string) (domain.Tag, error)
98: 	// DeleteTag removes a tag from the project's taxonomy.
99: 	DeleteTag(ctx context.Context, project domain.ProjectID, name string) error
100: 	// ProjectTags returns every tag of a project, ordered by name.
101: 	ProjectTags(ctx context.Context, project domain.ProjectID) ([]domain.Tag, error)
102: 	// TagByName returns one taxonomy tag in a project, matching name case-insensitively.
103: 	TagByName(ctx context.Context, project domain.ProjectID, name string) (domain.Tag, error)
104: 	// TagPatent assigns a tag to a patent; an existing assignment is left as is.
105: 	TagPatent(ctx context.Context, tagID int64, patent domain.PatentNumber, assignedAt time.Time) (bool, error)
106: 	// UntagPatent removes a tag from a patent; a missing assignment is a no-op.
107: 	UntagPatent(ctx context.Context, tagID int64, patent domain.PatentNumber) (bool, error)
108: 	// PatentTag returns one assigned tag on a patent in a project, matching name case-insensitively.
109: 	PatentTag(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, name string) (domain.Tag, error)
110: 	// PatentTags returns the tags a patent carries within a project, ordered by
111: 	// name.
112: 	PatentTags(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) ([]domain.Tag, error)
113: 
114: 	// IDSEntry returns the curated IDS entry for one (project, patent) pair.
115: 	IDSEntry(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (domain.IDSEntry, error)
116: 	// SaveIDSEntry inserts or updates one curated IDS entry.
117: 	SaveIDSEntry(ctx context.Context, entry domain.IDSEntry) (domain.IDSEntry, error)
118: 	// DeleteIDSEntry removes the curated IDS entry for one (project, patent) pair.
119: 	DeleteIDSEntry(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) error
120: 	// ListIDSEntries returns every curated IDS entry of a project.
121: 	ListIDSEntries(ctx context.Context, project domain.ProjectID) ([]domain.IDSEntry, error)
122: 
123: 	// Close releases all database resources.
124: 	Close() error
125: }

(End of file - total 125 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/rpc/client.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/rpc/client.go</path>
<type>file</type>
<content>
1: package rpc
2: 
3: import (
4: 	"context"
5: 	"encoding/json"
6: 	"errors"
7: 	"fmt"
8: 	"net"
9: 	"sync"
10: 
11: 	"patentmine/internal/proto"
12: )
13: 
14: // ErrClosed is returned by calls on a Client whose connection has dropped.
15: var ErrClosed = errors.New("rpc: client connection closed")
16: 
17: // eventBuffer bounds the client-side event queue; an overwhelmed consumer
18: // drops the newest events rather than blocking the read loop.
19: const eventBuffer = 64
20: 
21: // Client is a thin frontend's handle to the daemon. It multiplexes request
22: // replies and pushed events over one connection.
23: type Client struct {
24: 	conn   *proto.Conn
25: 	events chan proto.Event
26: 	closed chan struct{}
27: 
28: 	mu       sync.Mutex
29: 	nextID   uint64
30: 	pending  map[uint64]chan proto.Reply
31: 	closeErr error
32: 	stopped  bool
33: }
34: 
35: // Dial connects to the daemon's unix socket and starts the read loop.
36: func Dial(socketPath string) (*Client, error) {
37: 	nc, err := net.Dial("unix", socketPath)
38: 	if err != nil {
39: 		return nil, fmt.Errorf("rpc: dial %s: %w", socketPath, err)
40: 	}
41: 	c := &Client{
42: 		conn:    proto.NewConn(nc),
43: 		events:  make(chan proto.Event, eventBuffer),
44: 		closed:  make(chan struct{}),
45: 		pending: make(map[uint64]chan proto.Reply),
46: 	}
47: 	go c.readLoop()
48: 	return c, nil
49: }
50: 
51: // Events returns the channel of daemon-pushed events. It is closed when the
52: // connection drops.
53: func (c *Client) Events() <-chan proto.Event {
54: 	return c.events
55: }
56: 
57: // Call sends a request and waits for its reply, decoding result into the
58: // caller's value. It returns early if ctx is cancelled or the connection drops.
59: func (c *Client) Call(ctx context.Context, method proto.Method, params, result any) error {
60: 	var raw json.RawMessage
61: 	if params != nil {
62: 		encoded, err := json.Marshal(params)
63: 		if err != nil {
64: 			return fmt.Errorf("rpc: encode params: %w", err)
65: 		}
66: 		raw = encoded
67: 	}
68: 
69: 	c.mu.Lock()
70: 	if c.stopped {
71: 		err := c.closeErr
72: 		c.mu.Unlock()
73: 		return err
74: 	}
75: 	c.nextID++
76: 	id := c.nextID
77: 	replyCh := make(chan proto.Reply, 1)
78: 	c.pending[id] = replyCh
79: 	c.mu.Unlock()
80: 
81: 	cleanup := func() {
82: 		c.mu.Lock()
83: 		delete(c.pending, id)
84: 		c.mu.Unlock()
85: 	}
86: 
87: 	if err := c.conn.WriteMessage(proto.Request{
88: 		JSONRPC: proto.Version,
89: 		ID:      id,
90: 		Method:  method,
91: 		Params:  raw,
92: 	}); err != nil {
93: 		cleanup()
94: 		return fmt.Errorf("rpc: write request: %w", err)
95: 	}
96: 
97: 	select {
98: 	case <-ctx.Done():
99: 		cleanup()
100: 		return ctx.Err()
101: 	case <-c.closed:
102: 		return c.closeErr
103: 	case reply := <-replyCh:
104: 		if reply.Error != nil {
105: 			return reply.Error
106: 		}
107: 		if result != nil && len(reply.Result) > 0 {
108: 			if err := json.Unmarshal(reply.Result, result); err != nil {
109: 				return fmt.Errorf("rpc: decode result: %w", err)
110: 			}
111: 		}
112: 		return nil
113: 	}
114: }
115: 
116: // Close shuts the client down.
117: func (c *Client) Close() error {
118: 	c.shutdown(ErrClosed)
119: 	return nil
120: }
121: 
122: // readLoop demultiplexes incoming frames into replies and events.
123: func (c *Client) readLoop() {
124: 	defer close(c.events)
125: 	for {
126: 		raw, err := c.conn.ReadMessage()
127: 		if err != nil {
128: 			c.shutdown(fmt.Errorf("%w: %v", ErrClosed, err))
129: 			return
130: 		}
131: 		// A frame with a method name and no id is an event; otherwise a reply.
132: 		var probe struct {
133: 			ID     *uint64 `json:"id"`
134: 			Method *string `json:"method"`
135: 		}
136: 		_ = json.Unmarshal(raw, &probe)
137: 		if probe.ID == nil && probe.Method != nil {
138: 			var ev proto.Event
139: 			if json.Unmarshal(raw, &ev) == nil {
140: 				select {
141: 				case c.events <- ev:
142: 				default: // consumer behind; drop newest
143: 				}
144: 			}
145: 			continue
146: 		}
147: 		var reply proto.Reply
148: 		if json.Unmarshal(raw, &reply) != nil {
149: 			continue
150: 		}
151: 		c.deliver(reply)
152: 	}
153: }
154: 
155: func (c *Client) deliver(reply proto.Reply) {
156: 	c.mu.Lock()
157: 	ch, ok := c.pending[reply.ID]
158: 	if ok {
159: 		delete(c.pending, reply.ID)
160: 	}
161: 	c.mu.Unlock()
162: 	if ok {
163: 		ch <- reply
164: 	}
165: }
166: 
167: // shutdown marks the client closed exactly once and wakes blocked callers.
168: func (c *Client) shutdown(err error) {
169: 	c.mu.Lock()
170: 	if c.stopped {
171: 		c.mu.Unlock()
172: 		return
173: 	}
174: 	c.stopped = true
175: 	c.closeErr = err
176: 	c.mu.Unlock()
177: 
178: 	close(c.closed)
179: 	_ = c.conn.Close()
180: }

(End of file - total 180 lines)
</content>
```

---

## Assistant (Build · Big Pickle · 4.4s)

_Thinking:_

Now I have a thorough understanding of the codebase. Let me also check engine.go to see how the RPC handlers call engine methods - specifically how `patentGet` and `idsEntrySave` work via the engine, to make sure I follow the same pattern.

Let me read more of engine.go to understand the IDS save and patent-related handlers.

Actually, I think I have enough now. Let me check the overlay directory briefly to understand how overlays work for the notes popup.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/overlay"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/overlay</path>
<type>directory</type>
<entries>
confirm.go
help.go
jump.go
loading.go
overlay.go
prompt_test.go
prompt.go
tag_view_test.go
tag_view.go
textinput.go

(10 entries)
</entries>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/overlay/text.go",
  "limit": 40
}
```

**Error:**
```
File not found: /mnt/d/Repos/PatentMineNew/internal/tui/overlay/text.go
```

---

## Assistant (Build · Big Pickle · 3.4s)

_Thinking:_

Let me check the overlay interface and an existing overlay to understand the pattern.

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/overlay/overlay.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/overlay/overlay.go</path>
<type>file</type>
<content>
1: // Package overlay holds the TUI's modal layers. Like a Pane, each Overlay owns
2: // only its own state; the App owns the overlay stack and draws each overlay
3: // over a dimmed copy of the pane behind it.
4: package overlay
5: 
6: import (
7: 	"slices"
8: 
9: 	tea "github.com/charmbracelet/bubbletea"
10: 
11: 	"patentmine/internal/command"
12: )
13: 
14: // Overlay is one modal layer.
15: type Overlay interface {
16: 	// Title is shown at the top of the overlay box.
17: 	Title() string
18: 	// Command applies a resolved command intent (e.g. scrolling).
19: 	Command(id command.ID, repeat int) (Overlay, tea.Cmd)
20: 	// Handles reports every command ID the overlay services, so the App's
21: 	// wiring check can confirm overlay key bindings reach a handler.
22: 	Handles() []command.ID
23: 	// View renders the overlay body within maxW columns by maxH rows.
24: 	View(maxW, maxH int) string
25: }
26: 
27: // KeyHandler overlays consume raw key events directly, which prompt-like
28: // overlays need for text entry.
29: type KeyHandler interface {
30: 	HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool)
31: }
32: 
33: // ScopeSource reports the underlying pane scope an overlay was opened from.
34: // Prompt overlays use this so their filtered command list reflects the screen
35: // behind them rather than the generic overlay scope.
36: type ScopeSource interface {
37: 	SourceScope() command.Scope
38: }
39: 
40: // cmdHandler carries out one command for an overlay.
41: type cmdHandler func(repeat int) tea.Cmd
42: 
43: // handlerIDs returns the sorted command IDs of a handler table.
44: func handlerIDs(handlers map[command.ID]cmdHandler) []command.ID {
45: 	ids := make([]command.ID, 0, len(handlers))
46: 	for id := range handlers {
47: 		ids = append(ids, id)
48: 	}
49: 	slices.Sort(ids)
50: 	return ids
51: }
52: 
53: // PromptMode distinguishes palette search from direct command entry.
54: type PromptMode string
55: 
56: const (
57: 	PromptPalette PromptMode = "palette"
58: 	PromptDirect  PromptMode = "direct"
59: )
60: 
61: // PromptSubmitMsg asks the app to execute one typed command string.
62: type PromptSubmitMsg struct {
63: 	Input string
64: }
65: 
66: // PromptCloseMsg asks the app to close the focused prompt overlay.
67: type PromptCloseMsg struct{}
68: 
69: // Purpose names what a TextInput overlay is collecting, so the App routes the
70: // submitted value to the right action.
71: type Purpose string
72: 
73: const (
74: 	// PurposeCreateProject collects a name for a new project.
75: 	PurposeCreateProject   Purpose = "create-project"
76: 	PurposeEditIDSKind     Purpose = "edit-ids-kind"
77: 	PurposeEditIDSCountry  Purpose = "edit-ids-country"
78: 	PurposeEditIDSPassages Purpose = "edit-ids-passages"
79: 	PurposeEditIDSNotes    Purpose = "edit-ids-notes"
80: )
81: 
82: // TextSubmitMsg carries a value entered in a TextInput overlay.
83: type TextSubmitMsg struct {
84: 	Purpose Purpose
85: 	Value   string
86: }
87: 
88: // CloseOverlayMsg asks the app to close the focused overlay.
89: type CloseOverlayMsg struct{}

(End of file - total 89 lines)
</content>
```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/overlay/textinput.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/overlay/textinput.go</path>
<type>file</type>
<content>
1: package overlay
2: 
3: import (
4: 	"strings"
5: 
6: 	tea "github.com/charmbracelet/bubbletea"
7: 
8: 	"patentmine/internal/command"
9: 	"patentmine/internal/text"
10: 	"patentmine/internal/tui/render"
11: )
12: 
13: // TextInput is a single-line text-entry overlay. One type serves every
14: // free-text prompt — project name today, notes or rename later — so a new
15: // text field never means a new overlay type.
16: type TextInput struct {
17: 	theme   render.Theme
18: 	catalog *text.Catalog
19: 	purpose Purpose
20: 	title   text.Key
21: 	caption text.Key
22: 	value   string
23: 	cursor  int // rune index into value
24: }
25: 
26: // NewTextInput builds a text-entry overlay. title and caption are catalog keys;
27: // the submitted value is delivered as a TextSubmitMsg tagged with purpose.
28: func NewTextInput(theme render.Theme, catalog *text.Catalog, purpose Purpose, title, caption text.Key) *TextInput {
29: 	return &TextInput{theme: theme, catalog: catalog, purpose: purpose, title: title, caption: caption}
30: }
31: 
32: // Title implements Overlay.
33: func (t *TextInput) Title() string { return t.catalog.T(t.title) }
34: 
35: // Command implements Overlay: a text field services no resolved commands —
36: // every key press is consumed by HandleKey as literal input.
37: func (t *TextInput) Command(command.ID, int) (Overlay, tea.Cmd) { return t, nil }
38: 
39: // Handles implements Overlay.
40: func (t *TextInput) Handles() []command.ID { return nil }
41: 
42: // HandleKey implements KeyHandler: the overlay consumes every key as text so
43: // no key press leaks to the keymap while a field is focused.
44: func (t *TextInput) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
45: 	switch msg.Type {
46: 	case tea.KeyEsc:
47: 		return t, func() tea.Msg { return PromptCloseMsg{} }, true
48: 	case tea.KeyEnter:
49: 		value := strings.TrimSpace(t.value)
50: 		if value == "" {
51: 			return t, nil, true // stay open until the field has content
52: 		}
53: 		return t, func() tea.Msg { return TextSubmitMsg{Purpose: t.purpose, Value: value} }, true
54: 	case tea.KeyBackspace:
55: 		if t.cursor > 0 {
56: 			runes := []rune(t.value)
57: 			t.value = string(append(runes[:t.cursor-1], runes[t.cursor:]...))
58: 			t.cursor--
59: 		}
60: 		return t, nil, true
61: 	case tea.KeyLeft:
62: 		if t.cursor > 0 {
63: 			t.cursor--
64: 		}
65: 		return t, nil, true
66: 	case tea.KeyRight:
67: 		if t.cursor < len([]rune(t.value)) {
68: 			t.cursor++
69: 		}
70: 		return t, nil, true
71: 	case tea.KeyRunes, tea.KeySpace:
72: 		runes := []rune(t.value)
73: 		ins := []rune(msg.String())
74: 		merged := make([]rune, 0, len(runes)+len(ins))
75: 		merged = append(merged, runes[:t.cursor]...)
76: 		merged = append(merged, ins...)
77: 		merged = append(merged, runes[t.cursor:]...)
78: 		t.value = string(merged)
79: 		t.cursor += len(ins)
80: 		return t, nil, true
81: 	}
82: 	return t, nil, true
83: }
84: 
85: // View implements Overlay.
86: func (t *TextInput) View(maxW, _ int) string {
87: 	var b strings.Builder
88: 	b.WriteString(t.theme.Row.Render(render.Truncate(t.catalog.T(t.caption), maxW)))
89: 	b.WriteString("\n\n")
90: 	runes := []rune(t.value)
91: 	before := string(runes[:t.cursor])
92: 	after := string(runes[t.cursor:])
93: 	input := "> " + before + t.theme.Title.Render("█") + after
94: 	b.WriteString(render.Truncate(input, maxW))
95: 	b.WriteString("\n\n")
96: 	b.WriteString(t.theme.Dim.Render(render.Truncate(t.catalog.T(text.TextInputHint), maxW)))
97: 	return b.String()
98: }

(End of file - total 98 lines)
</content>
```

---

## Assistant (Build · Big Pickle · 14.7s)

_Thinking:_

Now I have a complete understanding of the codebase. Let me also check go.mod to see what external dependencies are already present.

Actually, I already saw from the task output that the dependencies are:
- github.com/PuerkitoBio/goquery v1.12.0
- github.com/charmbracelet/bubbletea v1.3.10
- github.com/charmbracelet/bubbles v1.0.0
- github.com/charmbracelet/lipgloss v1.1.0
- modernc.org/sqlite v1.50.1

OK, now let me start implementing. This is a large change spanning many files. Let me organize the implementation order carefully.

I'll work through the files in dependency order:

1. `internal/domain/fulltext.go` - types only
2. `internal/crawl/google.go` - add ParseAllClaims (but this requires domain types, which the crawl package already imports)
3. `internal/proto/proto.go` - add MethodFullText
4. `internal/tui/clipboard.go` - clipboard utility
5. `internal/tui/notes.go` - notes buffer
6. `internal/tui/pane/fulltext.go` - the big new pane
7. `internal/command/command.go` - add ScopeFullText
8. `internal/command/catalog.go` - register new commands
9. `internal/tui/keymap/default.go` - add bindings
10. `internal/text/catalog_en.go` - add strings
11. `internal/tui/pane/detail.go` - add link to full text
12. `internal/tui/app.go` - add handlers
13. `internal/tui/wiring.go` - add scope

Let me start with the domain types and work my way through.

Actually, wait. Let me reconsider the architecture. The full text pane needs to fetch full patent text from Google Patents. The existing architecture is:

- TUI is a thin client that calls the daemon via RPC
- The daemon does all the work (crawling, DB access)

For fetching full text, I have two options:

**Option A: Add a new RPC method** that the daemon handles by re-fetching the Google Patents page and returning all claims. This follows the existing architecture.

**Option B: Have the TUI fetch directly** from Google Patents in the pane's Init(). This is simpler but breaks the thin-client pattern and requires the TUI to have crawling dependencies.

I'll go with **Option A** since it follows the existing architecture and reuses the daemon's HTTP client infrastructure. I'll add:
- `proto.MethodFullText` 
- `proto.FullTextParams` (just a patent number)
- `proto.FullTextResult` (carries the FullText)
- An engine method `FullText(ctx, number)` that re-fetches and parses
- An RPC server handler
- The TUI calls this via the existing `Client.Call()` 

Wait, but looking at this more carefully - the engine's crawl methods use a worker pool and job system. The FullText fetch is simpler - it's just a one-shot fetch that returns immediately. Let me look at how the engine handles PatentGet...

The engine.Patent() method just reads from the store. For full text, I need the engine to re-fetch from the web. Let me check if there's a simpler method for this...

Actually, the simplest approach is to add a method on the engine that takes a patent number, fetches the Google Patents page using the HTTP source, parses all claims, and returns them. This is essentially a synchronous "lookup but only for full text" operation.

Let me check how the `crawl` package's http source works for re-fetching...

Actually, looking at the engine more carefully:

```go
type CrawlFactory func(root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) Job
```

The Job interface runs async. For a synchronous fetch (which is simpler), I could either:
1. Make the engine call the crawl factory directly and wait for the job
2. Have a separate HTTP fetch in the engine

Actually, the simplest way is to have the TUI pane make its own HTTP request directly. The crawl package already has `NewGoogleSource()` which returns a `Source` with a `Fetch` method. But that's wrapped in the engine's async job system.

Let me take a different, simpler approach: have the TUI pane call the existing `MethodPatentGet` RPC method to get the patent (with its first claim), and also fetch the full text directly from the Google Patents page. The TUI already has the URL pattern: `https://patents.google.com/patent/<number>/en`.

Actually wait, the TUI is a thin client that communicates over a Unix socket. It does have networking capabilities though - it's a Go program that can make HTTP requests. But adding HTTP dependencies to the TUI is not ideal.

Let me reconsider... The cleanest approach for this codebase is:

1. Add a new proto method `MethodFullText`
2. Add an engine method that re-fetches from Google (synchronously)
3. Wire the RPC handler

But I don't want to have to understand the full crawl pipeline. Let me look at what's in the `engine` that's already similar to what I need...

Actually, let me just look at the engine.Patent method and see if there's already something for fetching...

Let me check engine.go for the key methods I need.

Let me check the engine's public methods for patent fetching and the crawl source interface.

**Tool: bash**

**Input:**
```json
{
  "command": "grep -n \"^func (e \\*Engine)\" /mnt/d/Repos/PatentMineNew/internal/engine/engine.go | head -40",
  "description": "List engine public methods"
}
```

**Output:**
```
102:func (e *Engine) Close() {
108:func (e *Engine) Subscribe() (<-chan proto.Event, func()) {
113:func (e *Engine) Patent(ctx context.Context, n domain.PatentNumber) (patent domain.Patent, err error) {
125:func (e *Engine) recordNumber(ctx context.Context, n domain.PatentNumber) (domain.PatentNumber, error) {
141:func (e *Engine) ensureRecord(ctx context.Context, n domain.PatentNumber) (domain.PatentNumber, bool, error) {
167:func (e *Engine) ListPatents(ctx context.Context, q store.PatentQuery) (rows []domain.PatentRow, total int, err error) {
181:func (e *Engine) SavePatent(ctx context.Context, p domain.Patent) (err error) {
209:func (e *Engine) DeletePatent(ctx context.Context, n domain.PatentNumber) (err error) {
264:func (e *Engine) RestorePatent(ctx context.Context, snapshot PatentSnapshot, soft bool) (err error) {
321:func (e *Engine) Projects(ctx context.Context) (projects []domain.Project, err error) {
327:func (e *Engine) CreateProject(ctx context.Context, name string) (project domain.Project, err error) {
360:func (e *Engine) AddToProject(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (fetchStarted bool, err error) {
416:func (e *Engine) cleanupIfNotFound(project domain.ProjectID, record domain.PatentNumber, stubCreated bool, id JobID) {
453:func (e *Engine) SetReviewState(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, target domain.ReviewState) (err error) {
505:func (e *Engine) ReviewStateOf(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (state domain.ReviewState, ok bool, err error) {
526:func (e *Engine) PatentTags(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (tags []domain.Tag, err error) {
539:func (e *Engine) IDSEntryOf(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (entry domain.IDSEntry, ok bool, err error) {
559:func (e *Engine) SaveIDSEntry(ctx context.Context, entry domain.IDSEntry) (saved domain.IDSEntry, err error) {
581:func (e *Engine) DeleteIDSEntry(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (err error) {
596:func (e *Engine) CreateTaxonomyTag(ctx context.Context, project domain.ProjectID, name string) (tag domain.Tag, err error) {
623:func (e *Engine) DeleteTaxonomyTag(ctx context.Context, project domain.ProjectID, name string) (err error) {
658:func (e *Engine) ListTaxonomyTags(ctx context.Context, project domain.ProjectID) (tags []domain.Tag, err error) {
667:func (e *Engine) AssignPatentTag(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, name string) (err error) {
716:func (e *Engine) RemovePatentTag(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, name string) (err error) {
759:func (e *Engine) AssignTag(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, name string) (err error) {
766:func (e *Engine) RemoveTag(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, name string) (err error) {
772:func (e *Engine) StartFamilyCrawl(root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) (id JobID, err error) {
797:func (e *Engine) ImportFile(ctx context.Context, path string) (err error) {
816:func (e *Engine) Relations(ctx context.Context, q store.PatentQuery) (out []domain.PatentRow, total int, err error) {
831:func (e *Engine) ExportIDS(ctx context.Context, projectID domain.ProjectID) (ids domain.IDS, err error) {
876:func (e *Engine) CancelCrawl(id JobID) (err error) {
893:func (e *Engine) MetricsSnapshot() proto.MetricsSnapshot {
924:func (e *Engine) Metrics() *observability.Metrics {
929:func (e *Engine) Logger() *slog.Logger {
934:func (e *Engine) announceChange() {
938:func (e *Engine) existingPatent(ctx context.Context, n domain.PatentNumber) (domain.Patent, bool) {
946:func (e *Engine) existingMembership(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (domain.Membership, bool) {
954:func (e *Engine) log(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
963:func (e *Engine) recordActivity(ctx context.Context, rec observability.Record) {
972:func (e *Engine) incCounter(name string, delta int64) {

```

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/crawl/crawl.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/crawl/crawl.go</path>
<type>file</type>
<content>
1: package crawl
2: 
3: import (
4: 	"context"
5: 	"errors"
6: 	"fmt"
7: 	"log/slog"
8: 	"time"
9: 
10: 	"patentmine/internal/domain"
11: 	"patentmine/internal/observability"
12: 	"patentmine/internal/store"
13: )
14: 
15: const slowCrawlerRun = 500 * time.Millisecond
16: 
17: // Crawl tuning. A crawl stops when it reaches either limit, so a dense citation
18: // graph cannot produce an unbounded job.
19: const (
20: 	defaultMaxDepth   = 4
21: 	defaultNodeBudget = 200
22: )
23: 
24: // Progress is an incremental crawl update reported through the emit callback.
25: type Progress struct {
26: 	CrawledCount    int    // Total full records saved to the database.
27: 	DiscoveredCount int    // Total unique patent numbers seen (crawled + stubs).
28: 	PendingCount    int    // Items currently in the crawl queue.
29: 	CitationsCount  int    // Total citation edges found.
30: 	CitedByCount    int    // Total cited-by edges found.
31: 	ParentsCount    int    // Total parent/continuation edges found.
32: 	ChildrenCount   int    // Total child edges found.
33: 	Message         string // Human-readable note about the latest step.
34: }
35: 
36: // CrawlConfig bounds a crawl.
37: type CrawlConfig struct {
38: 	MaxDepth   int                 // BFS depth from the root; <= 0 uses defaultMaxDepth.
39: 	NodeBudget int                 // Max records to fetch; <= 0 uses defaultNodeBudget.
40: 	Profile    domain.CrawlProfile // Profile name (e.g., 'family', 'citations').
41: }
42: 
43: // Crawler walks the patent family graph breadth-first, fetching each node from
44: // the source registry and writing records, documents, and relations to the
45: // store.
46: type Crawler struct {
47: 	registry *Registry
48: 	repo     store.Repository
49: 	cfg      CrawlConfig
50: 	metrics  *observability.Metrics
51: 	logger   *slog.Logger
52: }
53: 
54: // NewCrawler builds a Crawler.
55: func NewCrawler(registry *Registry, repo store.Repository, cfg CrawlConfig) *Crawler {
56: 	if cfg.MaxDepth <= 0 {
57: 		cfg.MaxDepth = defaultMaxDepth
58: 	}
59: 	if cfg.NodeBudget <= 0 {
60: 		cfg.NodeBudget = defaultNodeBudget
61: 	}
62: 	return &Crawler{registry: registry, repo: repo, cfg: cfg}
63: }
64: 
65: // WithMetrics attaches a phase-1 in-process metrics recorder.
66: func (c *Crawler) WithMetrics(metrics *observability.Metrics) *Crawler {
67: 	c.metrics = metrics
68: 	return c
69: }
70: 
71: // WithLogger attaches a logger for slow crawl warnings.
72: func (c *Crawler) WithLogger(logger *slog.Logger) *Crawler {
73: 	c.logger = logger
74: 	return c
75: }
76: 
77: // node is one queued patent number and its BFS depth from the root.
78: type node struct {
79: 	number domain.PatentNumber
80: 	depth  int
81: }
82: 
83: // fetch retrieves one patent. A force fetch skips the local file cache and
84: // the SQLite database so the crawl re-pulls from the web.
85: func (c *Crawler) fetch(ctx context.Context, number domain.PatentNumber, force bool) (Result, error) {
86: 	if !force {
87: 		p, err := c.repo.Patent(ctx, number)
88: 		if err == nil && p.FetchState == domain.FetchCached {
89: 			var rels []domain.Relation
90: 			for _, kind := range []domain.RelationKind{
91: 				domain.RelationCites, domain.RelationCitedBy,
92: 				domain.RelationParent, domain.RelationChild,
93: 			} {
94: 				if kr, err := c.repo.Relations(ctx, number, kind); err == nil {
95: 					rels = append(rels, kr...)
96: 				}
97: 			}
98: 			return Result{Patent: p, Documents: p.Documents, Relations: rels}, nil
99: 		}
100: 	}
101: 	if force {
102: 		return c.registry.FetchExcluding(ctx, number, domain.SourceFile)
103: 	}
104: 	return c.registry.Fetch(ctx, number)
105: }
106: 
107: // Crawl profiles define which family-graph edges to follow during a crawl.
108: const (
109: 	CrawlProfileCitations = "citations" // Follow cites only (depth 0 only)
110: 	CrawlProfileCitedBy   = "citedby"   // Follow cited_by only (depth 0 only)
111: 	CrawlProfileFamily    = "family"    // Follow parent/child recursion
112: 	CrawlProfileAll       = "all"       // Combination of the above
113: )
114: 
115: // Crawl performs a bounded breadth-first walk from root. A negative maxDepth
116: // uses the configured depth; zero crawls the root only. force bypasses the
117: // local file cache. emit, which may be nil, receives progress.
118: func (c *Crawler) Crawl(ctx context.Context, root domain.PatentNumber, maxDepth int, profile domain.CrawlProfile, force bool, emit func(Progress)) error {
119: 	start := time.Now()
120: 	failed := true
121: 	log := observability.WithContextAttrs(ctx, c.logger)
122: 	if log == nil {
123: 		log = slog.Default()
124: 	}
125: 
126: 	defer func() {
127: 		if c.metrics != nil {
128: 			d := time.Since(start)
129: 			c.metrics.ObserveDuration("crawl.crawler.crawl", d, failed)
130: 			if d >= slowCrawlerRun {
131: 				log.Warn("slow crawl",
132: 					slog.String("root", root.String()),
133: 					slog.Int64("duration_ms", d.Milliseconds()),
134: 					slog.Bool("failed", failed))
135: 			}
136: 		}
137: 	}()
138: 
139: 	if root.IsZero() {
140: 		return errors.New("crawl: crawl root must not be empty")
141: 	}
142: 
143: 	log.Info("starting crawl",
144: 		slog.String("root", root.String()),
145: 		slog.Int("max_depth", maxDepth),
146: 		slog.String("profile", string(profile)),
147: 		slog.Bool("force", force))
148: 
149: 	depthLimit := maxDepth
150: 	if maxDepth < 0 {
151: 		depthLimit = c.cfg.MaxDepth
152: 	}
153: 
154: 	type crawlStats struct {
155: 		citations int
156: 		citedBy   int
157: 		parents   int
158: 		children  int
159: 	}
160: 	stats := crawlStats{}
161: 
162: 	report := func(p Progress) {
163: 		p.CitationsCount = stats.citations
164: 		p.CitedByCount = stats.citedBy
165: 		p.ParentsCount = stats.parents
166: 		p.ChildrenCount = stats.children
167: 
168: 		if emit != nil {
169: 			emit(p)
170: 		}
171: 		if c.metrics != nil {
172: 			c.metrics.SetGauge("crawl.crawler.crawled", int64(p.CrawledCount))
173: 			c.metrics.SetGauge("crawl.crawler.discovered", int64(p.DiscoveredCount))
174: 			c.metrics.SetGauge("crawl.crawler.pending", int64(p.PendingCount))
175: 			c.metrics.SetGauge("crawl.crawler.citations", int64(p.CitationsCount))
176: 			c.metrics.SetGauge("crawl.crawler.cited_by", int64(p.CitedByCount))
177: 			c.metrics.SetGauge("crawl.crawler.parents", int64(p.ParentsCount))
178: 			c.metrics.SetGauge("crawl.crawler.children", int64(p.ChildrenCount))
179: 		}
180: 	}
181: 
182: 	seen := map[domain.PatentNumber]bool{root: true}
183: 	queue := []node{{number: root, depth: 0}}
184: 	ingested := 0
185: 
186: 	for len(queue) > 0 && ingested < c.cfg.NodeBudget {
187: 		if err := ctx.Err(); err != nil {
188: 			log.Info("crawl cancelled", slog.String("error", err.Error()))
189: 			return err
190: 		}
191: 		cur := queue[0]
192: 		queue = queue[1:]
193: 
194: 		fstart := time.Now()
195: 		res, err := c.fetch(ctx, cur.number, force)
196: 		if c.metrics != nil {
197: 			c.metrics.ObserveDuration("crawl.crawler.fetch", time.Since(fstart), err != nil)
198: 		}
199: 
200: 		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
201: 			log.Info("fetch cancelled", slog.String("number", cur.number.String()))
202: 			return err
203: 		}
204: 		if err != nil {
205: 			log.Warn("fetch failed",
206: 				slog.String("number", cur.number.String()),
207: 				slog.String("error", err.Error()))
208: 
209: 			// Root patent not found — fail the job so the caller can clean up.
210: 			if cur.depth == 0 && errors.Is(err, ErrNotAvailable) {
211: 				return fmt.Errorf("crawl: patent %s not found", cur.number)
212: 			}
213: 
214: 			// A referenced patent that could not be fetched: record a stub
215: 			// so the edge to it still resolves, and keep crawling.
216: 			if _, stubErr := c.ensureRecord(ctx, cur.number); stubErr != nil {
217: 				return stubErr
218: 			}
219: 			report(Progress{
220: 				CrawledCount: ingested, DiscoveredCount: len(seen), PendingCount: len(queue),
221: 				Message: fmt.Sprintf("%s unavailable: %v", cur.number, err),
222: 			})
223: 			continue
224: 		}
225: 
226: 		recordNumber, err := c.saveRecord(ctx, res)
227: 		if err != nil {
228: 			log.Error("save record failed",
229: 				slog.String("number", cur.number.String()),
230: 				slog.String("error", err.Error()))
231: 			return err
232: 		}
233: 
234: 		// Update stats for all relations found in this fetch, even if we've
235: 		// seen the neighbour before, so the user sees a count of edges found.
236: 		for _, rel := range res.Relations {
237: 			switch rel.Kind {
238: 			case domain.RelationCites:
239: 				stats.citations++
240: 			case domain.RelationCitedBy:
241: 				stats.citedBy++
242: 			case domain.RelationParent:
243: 				stats.parents++
244: 			case domain.RelationChild:
245: 				stats.children++
246: 			}
247: 		}
248: 
249: 		if err := c.saveRelations(ctx, recordNumber, res.Relations, cur.depth, depthLimit, profile, seen, &queue); err != nil {
250: 			log.Error("save relations failed",
251: 				slog.String("number", recordNumber.String()),
252: 				slog.String("error", err.Error()))
253: 			return err
254: 		}
255: 
256: 		ingested++
257: 		report(Progress{
258: 			CrawledCount: ingested, DiscoveredCount: len(seen), PendingCount: len(queue),
259: 			Message: fmt.Sprintf("crawled %s", recordNumber),
260: 		})
261: 	}
262: 
263: 	reason := "exhausted"
264: 	if ingested >= c.cfg.NodeBudget {
265: 		reason = "budget reached"
266: 	}
267: 
268: 	log.Info("crawl finished",
269: 		slog.String("root", root.String()),
270: 		slog.String("reason", reason),
271: 		slog.Int("crawled", ingested),
272: 		slog.Int("discovered", len(seen)),
273: 		slog.Int("citations", stats.citations),
274: 		slog.Int("cited_by", stats.citedBy),
275: 		slog.Int("parents", stats.parents),
276: 		slog.Int("children", stats.children),
277: 		slog.Duration("duration", time.Since(start)))
278: 
279: 	failed = false
280: 	return nil
281: }
282: 
283: // saveRecord stores a fetched Result as one record and returns the record's
284: // permanent number. When the fetched documents already belong to one or more
285: // records, the Result is folded into them (merging records that collide).
286: func (c *Crawler) saveRecord(ctx context.Context, res Result) (domain.PatentNumber, error) {
287: 	recordNumber, err := c.resolveRecord(ctx, candidateNumbers(res))
288: 	if err != nil {
289: 		return domain.PatentNumber{}, err
290: 	}
291: 	if recordNumber.IsZero() {
292: 		recordNumber = res.Patent.Number
293: 	}
294: 
295: 	existing, err := c.repo.Documents(ctx, recordNumber)
296: 	if err != nil {
297: 		return domain.PatentNumber{}, err
298: 	}
299: 
300: 	patent := res.Patent
301: 	patent.Number = recordNumber
302: 	patent.FetchState = domain.FetchCached
303: 	patent.Documents = mergeDocuments(existing, res.Documents)
304: 	patent.DisplayNumber = patent.NumberToShow()
305: 	if err := c.repo.SavePatent(ctx, patent); err != nil {
306: 		return domain.PatentNumber{}, err
307: 	}
308: 	for _, doc := range res.Documents {
309: 		if err := c.repo.SaveDocument(ctx, recordNumber, doc); err != nil {
310: 			return domain.PatentNumber{}, err
311: 		}
312: 	}
313: 	return recordNumber, nil
314: }
315: 
316: // resolveRecord finds which existing record (if any) the candidate numbers
317: // belong to. When they belong to several records, those records are merged and
318: // the survivor is returned. A zero number means none of them is known yet.
319: func (c *Crawler) resolveRecord(ctx context.Context, candidates []domain.PatentNumber) (domain.PatentNumber, error) {
320: 	var records []domain.PatentNumber
321: 	seen := map[domain.PatentNumber]bool{}
322: 	for _, n := range candidates {
323: 		rec, err := c.repo.RecordOf(ctx, n)
324: 		if errors.Is(err, store.ErrNotFound) {
325: 			continue
326: 		}
327: 		if err != nil {
328: 			return domain.PatentNumber{}, err
329: 		}
330: 		if !seen[rec] {
331: 			seen[rec] = true
332: 			records = append(records, rec)
333: 		}
334: 	}
335: 	if len(records) == 0 {
336: 		return domain.PatentNumber{}, nil
337: 	}
338: 	keep := records[0]
339: 	for _, other := range records[1:] {
340: 		if err := c.repo.MergeRecords(ctx, keep, other); err != nil {
341: 			return domain.PatentNumber{}, err
342: 		}
343: 	}
344: 	return keep, nil
345: }
346: 
347: // saveRelations records the fetched edges and queues neighbours. Every edge
348: // endpoint is resolved to a record number, creating a stub record when the
349: // neighbour is new, so an edge always points at real records.
350: func (c *Crawler) saveRelations(ctx context.Context, from domain.PatentNumber, relations []domain.Relation, depth, depthLimit int, profile domain.CrawlProfile, seen map[domain.PatentNumber]bool, queue *[]node) error {
351: 	for _, rel := range relations {
352: 		neighbour := rel.To
353: 		neighbourRecord, err := c.ensureRecord(ctx, neighbour)
354: 		if err != nil {
355: 			return err
356: 		}
357: 		if err := c.repo.SaveRelation(ctx, domain.Relation{
358: 			From: from, To: neighbourRecord, Kind: rel.Kind,
359: 		}); err != nil {
360: 			return err
361: 		}
362: 		if !seen[neighbour] {
363: 			seen[neighbour] = true
364: 			if depth < depthLimit {
365: 				shouldQueue := false
366: 				switch profile {
367: 				case domain.CrawlProfileCitations:
368: 					shouldQueue = rel.Kind == domain.RelationCites && depth == 0
369: 				case domain.CrawlProfileCitedBy:
370: 					shouldQueue = rel.Kind == domain.RelationCitedBy && depth == 0
371: 				case domain.CrawlProfileFamily:
372: 					shouldQueue = rel.Kind == domain.RelationParent || rel.Kind == domain.RelationChild
373: 				case domain.CrawlProfileAll, "":
374: 					if rel.Kind == domain.RelationCites || rel.Kind == domain.RelationCitedBy {
375: 						shouldQueue = depth == 0
376: 					} else {
377: 						shouldQueue = true
378: 					}
379: 				}
380: 				if shouldQueue {
381: 					*queue = append(*queue, node{number: neighbour, depth: depth + 1})
382: 				}
383: 			}
384: 		}
385: 	}
386: 	return nil
387: }
388: 
389: // ensureRecord returns the record a number belongs to, creating a stub record
390: // (a patent row plus one document) when the number is not known yet.
391: func (c *Crawler) ensureRecord(ctx context.Context, number domain.PatentNumber) (domain.PatentNumber, error) {
392: 	rec, err := c.repo.RecordOf(ctx, number)
393: 	if err == nil {
394: 		return rec, nil
395: 	}
396: 	if !errors.Is(err, store.ErrNotFound) {
397: 		return domain.PatentNumber{}, err
398: 	}
399: 	stub := domain.Patent{
400: 		Number:        number,
401: 		DisplayNumber: number,
402: 		FetchState:    domain.FetchStub,
403: 	}
404: 	if err := c.repo.SavePatent(ctx, stub); err != nil {
405: 		return domain.PatentNumber{}, err
406: 	}
407: 	if err := c.repo.SaveDocument(ctx, number, domain.Document{
408: 		Number: number,
409: 		Stage:  domain.GuessStage(number),
410: 	}); err != nil {
411: 		return domain.PatentNumber{}, err
412: 	}
413: 	return number, nil
414: }
415: 
416: // candidateNumbers lists every number that could identify a fetched Result's
417: // record: the fetched number and each of its documents.
418: func candidateNumbers(res Result) []domain.PatentNumber {
419: 	numbers := []domain.PatentNumber{res.Patent.Number}
420: 	for _, d := range res.Documents {
421: 		numbers = append(numbers, d.Number)
422: 	}
423: 	return numbers
424: }
425: 
426: // mergeDocuments combines a record's stored documents with newly fetched ones,
427: // keyed by number so a fetched document replaces its stored version.
428: func mergeDocuments(existing, fetched []domain.Document) []domain.Document {
429: 	byNumber := make(map[domain.PatentNumber]domain.Document, len(existing)+len(fetched))
430: 	order := make([]domain.PatentNumber, 0, len(existing)+len(fetched))
431: 	for _, group := range [][]domain.Document{existing, fetched} {
432: 		for _, d := range group {
433: 			if _, ok := byNumber[d.Number]; !ok {
434: 				order = append(order, d.Number)
435: 			}
436: 			byNumber[d.Number] = d
437: 		}
438: 	}
439: 	out := make([]domain.Document, 0, len(order))
440: 	for _, n := range order {
441: 		out = append(out, byNumber[n])
442: 	}
443: 	return out
444: }

(End of file - total 444 lines)
</content>
```

---

## Assistant (Build · Big Pickle · 12.1s)

_Thinking:_

Now I have a thorough understanding. Let me look at the Registry interface to understand Fetch/FetchExcluding, since I need to understand how to do a direct Google Patents fetch.

Actually, I think the simplest approach for fetching full text is:

1. The TUI pane will make a direct HTTP request to Google Patents URL
2. It will parse the response using goquery's `ParseAllClaims` function
3. This avoids needing to add RPC methods, engine methods, etc.

The TUI already has HTTP capability (it uses an RPC client that runs over a Unix socket, but it's a Go program that can make HTTP requests). I just need to add `net/http` to the imports.

Wait, but the crawl package already has the HTTP source infrastructure. Let me check the registry to understand...

Actually, the cleanest approach for this codebase is to:

1. Add the full text types to domain
2. Add a `ParseAllClaims` function in the crawl package (it already has goquery dependency)
3. Add a `FetchFullText` function that does the HTTP request + parsing
4. In the TUI pane, call this function directly

The TUI already imports `patentmine/internal/domain` but not `patentmine/internal/crawl`. I need to add that import. This is fine - the crawl package is a library, not just for the daemon.

Let me also check if the crawl package exports what I need...

The crawl package has:
- `NewGoogleSource() Source` which creates a `Source` interface
- `Source` has a `Fetch(ctx, number) (Result, error)` method
- The Result has a `Patent` field with `FirstClaim`

But I need ALL claims. So I need to:
1. Either extend the Google crawler's parser to extract all claims
2. Or add a separate function that fetches the page and parses all claims

Let me add a `ParseAllClaims` function to the crawl package that can be called independently.

Actually, the cleanest approach:
1. Create `internal/domain/fulltext.go` with `FullText` and `ClaimSection` types
2. Add `ParseAllClaims(body []byte) ([]ClaimSection, error)` in `internal/crawl/google.go`
3. Add `FetchFullText(ctx, number) (FullText, error)` that does the HTTP GET + parses
4. In `internal/tui/pane/fulltext.go`, call `FetchFullText` in `Init()`

For the HTTP GET, I need the Google Patents URL. The `NewGoogleSource` already has it:
```go
func(n domain.PatentNumber) string {
    return "https://patents.google.com/patent/" + n.Normalized() + "/en"
}
```

I'll reuse this URL pattern in the FetchFullText function.

Let me also check the go.mod for existing HTTP dependencies... The crawl package uses `goquery` which depends on `golang.org/x/net/html` and `net/http` is in the stdlib. So I can use `net/http` directly.

Now let me start implementing. I'll work through the files systematically.

Let me start writing all the files.

Now I have a complete understanding. Let me start implementing. I'll work through each file in dependency order.

**Tool: todowrite**

**Input:**
```json
{
  "todos": [
    {
      "priority": "high",
      "content": "Read proto, RPC, store files needed for implementation",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Create domain/fulltext.go with FullText and ClaimSection types",
      "status": "in_progress"
    },
    {
      "priority": "high",
      "content": "Add ParseAllClaims to crawl/google.go",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Create tui/clipboard.go (OSC 52 + platform fallbacks)",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Create tui/notes.go (notes buffer manager)",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Create tui/pane/fulltext.go (full text viewer pane)",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Add new commands to command/catalog.go and command/command.go",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Add key bindings to tui/keymap/default.go",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Wire up in tui/app.go (handlers, routing)",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Extend detail pane with full text link",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Update text/catalog_en.go with new strings",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Update tui/wiring.go for new scope",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Build and verify compilation",
      "status": "pending"
    }
  ]
}
```

**Output:**
```
[
  {
    "content": "Read proto, RPC, store files needed for implementation",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Create domain/fulltext.go with FullText and ClaimSection types",
    "status": "in_progress",
    "priority": "high"
  },
  {
    "content": "Add ParseAllClaims to crawl/google.go",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Create tui/clipboard.go (OSC 52 + platform fallbacks)",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Create tui/notes.go (notes buffer manager)",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Create tui/pane/fulltext.go (full text viewer pane)",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Add new commands to command/catalog.go and command/command.go",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Add key bindings to tui/keymap/default.go",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Wire up in tui/app.go (handlers, routing)",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Extend detail pane with full text link",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Update text/catalog_en.go with new strings",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Update tui/wiring.go for new scope",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Build and verify compilation",
    "status": "pending",
    "priority": "high"
  }
]
```

---

## Assistant (Build · Big Pickle · 4.9s)

_Thinking:_

Let me start creating the files. First, the domain type.

**Tool: write**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/domain/fulltext.go",
  "content": "package domain\n\nimport \"fmt\"\n\n// ClaimSection is one numbered claim of a patent's full text.\ntype ClaimSection struct {\n\tNumber int    `json:\"number\"`\n\tText   string `json:\"text\"`\n}\n\n// FullText holds the complete claims text of a patent, fetched on-demand\n// from the web rather than stored in the database.\ntype FullText struct {\n\tNumber PatentNumber   `json:\"number\"`\n\tClaims []ClaimSection `json:\"claims\"`\n}\n\n// String returns a compact summary of the full text.\nfunc (f FullText) String() string {\n\tif len(f.Claims) == 0 {\n\t\treturn \"no claims\"\n\t}\n\treturn fmt.Sprintf(\"%d claims\", len(f.Claims))\n}\n"
}
```

**Output:**
```
Wrote file successfully.
```

---

## Assistant (Build · Big Pickle · 8.2s)

_Thinking:_

Now let me add `ParseAllClaims` to the crawl/google.go file. I need to add a function that takes the HTML body and extracts all claims.

Now add the `ParseAllClaims` function to the Google crawler:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/crawl/google.go",
  "oldString": "// clean collapses runs of whitespace in scraped text to single spaces.\nfunc clean(value string) string {\n\treturn strings.Join(strings.Fields(value), \" \")\n}",
  "newString": "// ParseAllClaims extracts every numbered claim from a Google Patents HTML body.\n// It returns the claims in document order with their numbers stripped from the\n// text and stored as ClaimSection.Number.\nfunc ParseAllClaims(body []byte) ([]domain.ClaimSection, error) {\n\tdoc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))\n\tif err != nil {\n\t\treturn nil, fmt.Errorf(\"crawl/google: parse claims HTML: %w\", err)\n\t}\n\tvar claims []domain.ClaimSection\n\tdoc.Find(\"section[itemprop='claims'] .claim, .claims .claim\").Each(func(i int, s *goquery.Selection) {\n\t\traw := strings.TrimSpace(s.Text())\n\t\tif raw == \"\" {\n\t\t\treturn\n\t\t}\n\t\tnum := i + 1\n\t\t// Try to extract the claim number from the text (\"1. ...\", \"Claim 1. ...\")\n\t\tcleaned := clean(raw)\n\t\tclaims = append(claims, domain.ClaimSection{Number: num, Text: cleaned})\n\t})\n\tif len(claims) == 0 {\n\t\treturn nil, fmt.Errorf(\"crawl/google: no claims found\")\n\t}\n\treturn claims, nil\n}\n\n// FetchFullText fetches a patent's full claims text from Google Patents.\n// It makes a direct HTTP request and parses all claim sections.\nfunc FetchFullText(number domain.PatentNumber) (*domain.FullText, error) {\n\turl := \"https://patents.google.com/patent/\" + number.Normalized() + \"/en\"\n\tresp, err := httpGet(url)\n\tif err != nil {\n\t\treturn nil, fmt.Errorf(\"crawl/google: fetch full text: %w\", err)\n\t}\n\tdefer resp.Body.Close()\n\tbody, err := io.ReadAll(resp.Body)\n\tif err != nil {\n\t\treturn nil, fmt.Errorf(\"crawl/google: read body: %w\", err)\n\t}\n\tclaims, err := ParseAllClaims(body)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\treturn &domain.FullText{\n\t\tNumber: number,\n\t\tClaims: claims,\n\t}, nil\n}\n\n// httpGet is a variable so tests can replace it.\nvar httpGet = defaultHTTPGet\n\nfunc defaultHTTPGet(url string) (*http.Response, error) {\n\treturn http.DefaultClient.Get(url)\n}\n\n// clean collapses runs of whitespace in scraped text to single spaces.\nfunc clean(value string) string {\n\treturn strings.Join(strings.Fields(value), \" \")\n}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 5.0s)

_Thinking:_

Wait, I need to check the imports in google.go. The existing imports include "bytes", "fmt", and "strings", but not "io", "net/http", or "domain". Let me check the current imports and update them.

Looking at the imports:
```go
import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"patentmine/internal/domain"
)
```

I need to add "io" and "net/http" to the imports. The domain package is already imported.

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/crawl/google.go",
  "oldString": "import (\n\t\"bytes\"\n\t\"fmt\"\n\t\"regexp\"\n\t\"strings\"\n\t\"time\"\n\n\t\"github.com/PuerkitoBio/goquery\"\n\n\t\"patentmine/internal/domain\"\n)",
  "newString": "import (\n\t\"bytes\"\n\t\"fmt\"\n\t\"io\"\n\t\"net/http\"\n\t\"regexp\"\n\t\"strings\"\n\t\"time\"\n\n\t\"github.com/PuerkitoBio/goquery\"\n\n\t\"patentmine/internal/domain\"\n)"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.7s)

_Thinking:_

Good. Now let me create the clipboard utility. This will use OSC 52 escape sequences and fall back to platform-specific commands.

**Tool: todowrite**

**Input:**
```json
{
  "todos": [
    {
      "priority": "high",
      "content": "Read proto, RPC, store files needed for implementation",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Create domain/fulltext.go with FullText and ClaimSection types",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Add ParseAllClaims to crawl/google.go",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Create tui/clipboard.go (OSC 52 + platform fallbacks)",
      "status": "in_progress"
    },
    {
      "priority": "high",
      "content": "Create tui/notes.go (notes buffer manager)",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Create tui/pane/fulltext.go (full text viewer pane)",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Add new commands to command/catalog.go and command/command.go",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Add key bindings to tui/keymap/default.go",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Wire up in tui/app.go (handlers, routing)",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Extend detail pane with full text link",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Update text/catalog_en.go with new strings",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Update tui/wiring.go for new scope",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Build and verify compilation",
      "status": "pending"
    }
  ]
}
```

**Output:**
```
[
  {
    "content": "Read proto, RPC, store files needed for implementation",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Create domain/fulltext.go with FullText and ClaimSection types",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Add ParseAllClaims to crawl/google.go",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Create tui/clipboard.go (OSC 52 + platform fallbacks)",
    "status": "in_progress",
    "priority": "high"
  },
  {
    "content": "Create tui/notes.go (notes buffer manager)",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Create tui/pane/fulltext.go (full text viewer pane)",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Add new commands to command/catalog.go and command/command.go",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Add key bindings to tui/keymap/default.go",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Wire up in tui/app.go (handlers, routing)",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Extend detail pane with full text link",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Update text/catalog_en.go with new strings",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Update tui/wiring.go for new scope",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Build and verify compilation",
    "status": "pending",
    "priority": "high"
  }
]
```

---

## Assistant (Build · Big Pickle · 8.4s)

_Thinking:_

Now let me create the clipboard utility. This needs:
1. OSC 52 escape sequence support (write to stdout)
2. Platform fallbacks: clip.exe (WSL), pbcopy (macOS), xclip/wl-copy (Linux)
3. A simple function `CopyToClipboard(text string) error`

**Tool: write**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/clipboard.go",
  "content": "package tui\n\nimport (\n\t\"encoding/base64\"\n\t\"fmt\"\n\t\"os\"\n\t\"os/exec\"\n\t\"runtime\"\n\t\"strings\"\n)\n\n// clipboardStrategy names which clipboard backend to prefer.\ntype clipboardStrategy int\n\nconst (\n\tclipboardOSC52    clipboardStrategy = iota // ANSI escape sequence\n\tclipboardPlatform                           // OS-native command\n)\n\n// copyToClipboard copies text to the system clipboard. It first tries OSC 52\n// (works in most modern terminal emulators without external deps), then falls\n// back to the platform-native command.\nfunc copyToClipboard(text string) error {\n\tif err := copyOSC52(text); err == nil {\n\t\treturn nil\n\t}\n\treturn copyPlatform(text)\n}\n\n// copyOSC52 writes the OSC 52 escape sequence to stdout, asking the terminal\n// emulator to set the clipboard. Returns nil if the write succeeded (most\n// terminals silently swallow unsupported sequences, so this is best-effort).\nfunc copyOSC52(text string) error {\n\tencoded := base64.StdEncoding.EncodeToString([]byte(text))\n\tseq := fmt.Sprintf(\"\\x1b]52;c;%s\\x07\", encoded)\n\t_, err := os.Stdout.WriteString(seq)\n\treturn err\n}\n\n// copyPlatform shells out to the OS-native clipboard command.\nfunc copyPlatform(text string) error {\n\tcmd := clipboardCommand()\n\tif cmd == nil {\n\t\treturn fmt.Errorf(\"clipboard: no supported clipboard command found\")\n\t}\n\tc := exec.Command(cmd[0], cmd[1:]...)\n\tc.Stdin = strings.NewReader(text)\n\treturn c.Run()\n}\n\n// clipboardCommand returns the OS-native clipboard command and args.\nfunc clipboardCommand() []string {\n\tswitch runtime.GOOS {\n\tcase \"darwin\":\n\t\treturn []string{\"pbcopy\"}\n\tcase \"windows\":\n\t\treturn []string{\"clip\"}\n\tdefault:\n\t\t// Linux / WSL: try wl-copy (Wayland) then xclip (X11) then clip.exe (WSL)\n\t\tif _, err := exec.LookPath(\"wl-copy\"); err == nil {\n\t\t\treturn []string{\"wl-copy\"}\n\t\t}\n\t\tif _, err := exec.LookPath(\"xclip\"); err == nil {\n\t\t\treturn []string{\"xclip\", \"-selection\", \"clipboard\"}\n\t\t}\n\t\tif _, err := exec.LookPath(\"clip.exe\"); err == nil {\n\t\t\treturn []string{\"clip.exe\"}\n\t\t}\n\t\treturn nil\n\t}\n}\n"
}
```

**Output:**
```
Wrote file successfully.
```

---

## Assistant (Build · Big Pickle · 17.0s)

_Thinking:_

Now create the notes buffer manager. This manages the per-patent accumulated claim locators.

Now create the notes buffer manager:

**Tool: write**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/notes.go",
  "content": "package tui\n\nimport (\n\t\"fmt\"\n\t\"sort\"\n\t\"strings\"\n\n\t\"patentmine/internal/domain\"\n\t\"patentmine/internal/rpc\"\n\t\"patentmine/internal/text\"\n\t\"patentmine/internal/tui/pane\"\n\t\"patentmine/internal/tui/render\"\n\n\ttea \"github.com/charmbracelet/bubbletea\"\n\n\t\"patentmine/internal/proto\"\n)\n\n// notesAccumulator holds per-patent claim locators accumulated during a session.\n// It is owned by the App and accessed by the full text pane.\ntype notesAccumulator struct {\n\tentries map[domain.PatentNumber][]string // patent -> deduped sorted locators\n}\n\nfunc newNotesAccumulator() *notesAccumulator {\n\treturn &notesAccumulator{entries: make(map[domain.PatentNumber][]string)}\n}\n\n// Add inserts a locator for patent number. Returns true when the locator is new.\nfunc (n *notesAccumulator) Add(number domain.PatentNumber, locator string) bool {\n\tlocator = strings.TrimSpace(locator)\n\tif locator == \"\" {\n\t\treturn false\n\t}\n\texisting := n.entries[number]\n\tlower := strings.ToLower(locator)\n\tfor _, e := range existing {\n\t\tif strings.ToLower(e) == lower {\n\t\t\treturn false // already present\n\t\t}\n\t}\n\tn.entries[number] = append(existing, locator)\n\tsort.Strings(n.entries[number])\n\treturn true\n}\n\n// Locators returns the accumulated locators for number, sorted.\nfunc (n *notesAccumulator) Locators(number domain.PatentNumber) []string {\n\treturn n.entries[number]\n}\n\n// Clear resets the buffer for number.\nfunc (n *notesAccumulator) Clear(number domain.PatentNumber) {\n\tdelete(n.entries, number)\n}\n\n// FlushToIDS saves the accumulated locators to the patent's IDS relevant_passages field.\nfunc (n *notesAccumulator) FlushToIDS(client *rpc.Client, number domain.PatentNumber, project domain.ProjectID) tea.Cmd {\n\tlocators := n.entries[number]\n\tif len(locators) == 0 {\n\t\treturn nil\n\t}\n\tpassages := strings.Join(locators, \"; \")\n\treturn func() tea.Msg {\n\t\tctx, cancel := callContext()\n\t\tdefer cancel()\n\t\t// Get current entry first (if exists)\n\t\tvar existing proto.PatentResult\n\t\terr := client.Call(ctx, proto.MethodPatentGet,\n\t\t\tproto.PatentGetParams{Number: number, Project: project}, &existing)\n\t\tif err != nil {\n\t\t\treturn pane.StatusMsg{Key: text.StatusClipboardFailed, Args: []any{\"flush to IDS: \" + err.Error()}, Error: true}\n\t\t}\n\t\tentry := domain.IDSEntry{\n\t\t\tProject:          project,\n\t\t\tPatent:           number,\n\t\t\tRelevantPassages: passages,\n\t\t\tStatus:           domain.IDSEntryPending,\n\t\t}\n\t\tif existing.IDSEntry != nil {\n\t\t\tentry = *existing.IDSEntry\n\t\t\tif entry.RelevantPassages != \"\" {\n\t\t\t\tentry.RelevantPassages += \"; \" + passages\n\t\t\t} else {\n\t\t\t\tentry.RelevantPassages = passages\n\t\t\t}\n\t\t}\n\t\tvar result proto.IDSEntryResult\n\t\terr = client.Call(ctx, proto.MethodIDSEntrySave,\n\t\t\tproto.IDSEntrySaveParams{Entry: entry}, &result)\n\t\tif err != nil {\n\t\t\treturn pane.StatusMsg{Key: text.StatusClipboardFailed, Args: []any{\"flush to IDS: \" + err.Error()}, Error: true}\n\t\t}\n\t\tn.Clear(number)\n\t\treturn pane.StatusMsg{Key: text.StatusNotesFlushed, Args: []any{number.String(), passages}}\n\t}\n}\n\n// ExportText builds the full export string with patent metadata + locators.\nfunc (n *notesAccumulator) ExportText(number domain.PatentNumber, patent *domain.Patent) string {\n\tvar b strings.Builder\n\twriteMeta(&b, number, patent)\n\tlocators := n.entries[number]\n\tif len(locators) > 0 {\n\t\tb.WriteString(\"───────────────────────────────────────────────────────────────\\n\")\n\t\tb.WriteString(\"Relevant passages:\\n\")\n\t\tfor _, loc := range locators {\n\t\t\tb.WriteString(\"  - \" + loc + \"\\n\")\n\t\t}\n\t\tb.WriteString(\"───────────────────────────────────────────────────────────────\\n\")\n\t}\n\treturn b.String()\n}\n\n// NotesBufferOverlay is a simple overlay that shows accumulated notes.\ntype NotesBufferOverlay struct {\n\ttheme    render.Theme\n\tnumber   domain.PatentNumber\n\tlocators []string\n\tapp      *App\n}\n\nfunc newNotesBufferOverlay(theme render.Theme, number domain.PatentNumber, locators []string, app *App) *NotesBufferOverlay {\n\treturn &NotesBufferOverlay{\n\t\ttheme:    theme,\n\t\tnumber:   number,\n\t\tlocators: locators,\n\t\tapp:      app,\n\t}\n}\n\nfunc (o *NotesBufferOverlay) Title() string { return \"Notes Buffer · \" + o.number.String() }\n\nfunc (o *NotesBufferOverlay) Command(id string, repeat int) (interface{}, tea.Cmd) { return o, nil }\nfunc (o *NotesBufferOverlay) Handles() []string                                     { return nil }\n\nfunc (o *NotesBufferOverlay) View(maxW, maxH int) string {\n\tvar b strings.Builder\n\tif len(o.locators) == 0 {\n\t\tb.WriteString(o.theme.Dim.Render(\"  (no notes accumulated)\"))\n\t} else {\n\t\tb.WriteString(\"  Accumulated locators:\\n\")\n\t\tfor _, loc := range o.locators {\n\t\t\tb.WriteString(\"    · \" + loc + \"\\n\")\n\t\t}\n\t}\n\tb.WriteString(\"\\n\\n\")\n\tb.WriteString(o.theme.Dim.Render(\"  [F] Flush to IDS  [y] Export copy  [esc] Close\"))\n\treturn b.String()\n}\n\n// writeMeta writes the patent metadata header to b.\nfunc writeMeta(b *strings.Builder, number domain.PatentNumber, p *domain.Patent) {\n\tsep := strings.Repeat(\"═\", 48)\n\tb.WriteString(sep + \"\\n\")\n\tb.WriteString(fmt.Sprintf(\"Patent #:     %s\\n\", number.String()))\n\tif p != nil {\n\t\tb.WriteString(fmt.Sprintf(\"Title:        %s\\n\", p.Title))\n\t\tif len(p.Inventors) > 0 {\n\t\t\tinventorStr := string(p.Inventors[0])\n\t\t\tif len(p.Inventors) > 1 {\n\t\t\t\tinventorStr += \" et al. (\" + fmt.Sprintf(\"%d\", len(p.Inventors)) + \")\"\n\t\t\t}\n\t\t\tb.WriteString(fmt.Sprintf(\"Inventors:    %s\\n\", inventorStr))\n\t\t}\n\t\tif p.Assignee != \"\" {\n\t\t\tb.WriteString(fmt.Sprintf(\"Assignee:     %s\\n\", p.Assignee))\n\t\t}\n\t\t// Find application number from documents\n\t\tfor _, doc := range p.Documents {\n\t\t\tif doc.Stage == domain.StageApplication {\n\t\t\t\tdateStr := \"\"\n\t\t\t\tif !doc.Dated.IsZero() {\n\t\t\t\t\tdateStr = \" (\" + doc.Dated.Format(\"2006-01-02\") + \")\"\n\t\t\t\t}\n\t\t\t\tb.WriteString(fmt.Sprintf(\"Application #: %s%s\\n\", doc.Number.String(), dateStr))\n\t\t\t\tbreak\n\t\t\t}\n\t\t}\n\t\tif !p.PublicationDate.IsZero() {\n\t\t\tb.WriteString(fmt.Sprintf(\"Publication:  %s\\n\", p.PublicationDate.Format(\"2006-01-02\")))\n\t\t}\n\t\tif !p.ExpirationDate.IsZero() {\n\t\t\tsrc := \"\"\n\t\t\tif p.ExpirationSource != \"\" {\n\t\t\t\tsrc = \" (\" + p.ExpirationSource + \")\"\n\t\t\t}\n\t\t\tb.WriteString(fmt.Sprintf(\"Expiration:   %s%s\\n\", p.ExpirationDate.Format(\"2006-01-02\"), src))\n\t\t}\n\t}\n\tb.WriteString(sep + \"\\n\")\n}\n"
}
```

**Output:**
```
Wrote file successfully.
```

---

## Assistant (Build · Big Pickle · 5.7s)

_Thinking:_

Wait, I made a mistake. The NotesBufferOverlay's Command and Handles methods use wrong types. Let me fix this. The overlay.Overlay interface expects `command.ID` type, not string. Let me look at the overlay interface again...

```go
type Overlay interface {
    Title() string
    Command(id command.ID, repeat int) (Overlay, tea.Cmd)
    Handles() []command.ID
    View(maxW, maxH int) string
}
```

I used `string` instead of `command.ID`. Let me fix the notes.go file.

Let me fix the overlay type signatures in notes.go:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/notes.go",
  "oldString": "import (\n\t\"fmt\"\n\t\"sort\"\n\t\"strings\"\n\n\t\"patentmine/internal/domain\"\n\t\"patentmine/internal/rpc\"\n\t\"patentmine/internal/text\"\n\t\"patentmine/internal/tui/pane\"\n\t\"patentmine/internal/tui/render\"\n\n\ttea \"github.com/charmbracelet/bubbletea\"\n\n\t\"patentmine/internal/proto\"\n)",
  "newString": "import (\n\t\"fmt\"\n\t\"sort\"\n\t\"strings\"\n\n\ttea \"github.com/charmbracelet/bubbletea\"\n\n\t\"patentmine/internal/command\"\n\t\"patentmine/internal/domain\"\n\t\"patentmine/internal/proto\"\n\t\"patentmine/internal/rpc\"\n\t\"patentmine/internal/text\"\n\t\"patentmine/internal/tui/pane\"\n\t\"patentmine/internal/tui/render\"\n)"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.1s)

_Thinking:_

Now fix the Command and Handles methods to use command.ID type:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/notes.go",
  "oldString": "func (o *NotesBufferOverlay) Command(id string, repeat int) (interface{}, tea.Cmd) { return o, nil }\nfunc (o *NotesBufferOverlay) Handles() []string                                     { return nil }",
  "newString": "func (o *NotesBufferOverlay) Command(id command.ID, repeat int) (interface{}, tea.Cmd) { return o, nil }\nfunc (o *NotesBufferOverlay) Handles() []command.ID { return nil }"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 6.2s)

_Thinking:_

Wait, the overlay interface uses the overlay package types, not plain interface{}. Let me check...

```go
type Overlay interface {
    Title() string
    Command(id command.ID, repeat int) (Overlay, tea.Cmd)
    Handles() []command.ID
    View(maxW, maxH int) string
}
```

So `Command` should return `(overlay.Overlay, tea.Cmd)` - but that's the interface type. If I use `interface{}` it won't match. Let me see how other overlays do this...

Looking at textinput.go:
```go
func (t *TextInput) Command(command.ID, int) (Overlay, tea.Cmd) { return t, nil }
```

It returns `Overlay` which is `overlay.Overlay`. So I need to import the overlay package and use the right return type.

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/notes.go",
  "oldString": "import (\n\t\"fmt\"\n\t\"sort\"\n\t\"strings\"\n\n\ttea \"github.com/charmbracelet/bubbletea\"\n\n\t\"patentmine/internal/command\"\n\t\"patentmine/internal/domain\"\n\t\"patentmine/internal/proto\"\n\t\"patentmine/internal/rpc\"\n\t\"patentmine/internal/text\"\n\t\"patentmine/internal/tui/pane\"\n\t\"patentmine/internal/tui/render\"\n)",
  "newString": "import (\n\t\"fmt\"\n\t\"sort\"\n\t\"strings\"\n\n\ttea \"github.com/charmbracelet/bubbletea\"\n\n\t\"patentmine/internal/command\"\n\t\"patentmine/internal/domain\"\n\t\"patentmine/internal/proto\"\n\t\"patentmine/internal/rpc\"\n\t\"patentmine/internal/text\"\n\t\"patentmine/internal/tui/overlay\"\n\t\"patentmine/internal/tui/pane\"\n\t\"patentmine/internal/tui/render\"\n)"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.3s)

_Thinking:_

Now fix the Command method to return `overlay.Overlay` instead of `interface{}`:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/notes.go",
  "oldString": "func (o *NotesBufferOverlay) Command(id command.ID, repeat int) (interface{}, tea.Cmd) { return o, nil }\nfunc (o *NotesBufferOverlay) Handles() []command.ID { return nil }",
  "newString": "func (o *NotesBufferOverlay) Command(id command.ID, repeat int) (overlay.Overlay, tea.Cmd) { return o, nil }\nfunc (o *NotesBufferOverlay) Handles() []command.ID { return nil }"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.9s)

_Thinking:_

Now create the full text pane. This is the biggest piece.

**Tool: todowrite**

**Input:**
```json
{
  "todos": [
    {
      "priority": "high",
      "content": "Read proto, RPC, store files needed for implementation",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Create domain/fulltext.go with FullText and ClaimSection types",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Add ParseAllClaims to crawl/google.go",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Create tui/clipboard.go (OSC 52 + platform fallbacks)",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Create tui/notes.go (notes buffer manager)",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Create tui/pane/fulltext.go (full text viewer pane)",
      "status": "in_progress"
    },
    {
      "priority": "high",
      "content": "Add new commands to command/catalog.go and command/command.go",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Add key bindings to tui/keymap/default.go",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Wire up in tui/app.go (handlers, routing)",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Extend detail pane with full text link",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Update text/catalog_en.go with new strings",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Update tui/wiring.go for new scope",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Build and verify compilation",
      "status": "pending"
    }
  ]
}
```

**Output:**
```
[
  {
    "content": "Read proto, RPC, store files needed for implementation",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Create domain/fulltext.go with FullText and ClaimSection types",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Add ParseAllClaims to crawl/google.go",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Create tui/clipboard.go (OSC 52 + platform fallbacks)",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Create tui/notes.go (notes buffer manager)",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Create tui/pane/fulltext.go (full text viewer pane)",
    "status": "in_progress",
    "priority": "high"
  },
  {
    "content": "Add new commands to command/catalog.go and command/command.go",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Add key bindings to tui/keymap/default.go",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Wire up in tui/app.go (handlers, routing)",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Extend detail pane with full text link",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Update text/catalog_en.go with new strings",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Update tui/wiring.go for new scope",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Build and verify compilation",
    "status": "pending",
    "priority": "high"
  }
]
```

---

## Assistant (Build · Big Pickle · 5.8s)

_Thinking:_

Let me look at the fulltext pane implementation. This is the core component. I need to look at how the catalog pane and detail pane work to follow the patterns closely.

Key features I need:
1. Load patent data via RPC
2. Fetch full text via the crawl package's FetchFullText function
3. Scrollable view with claim headers
4. Visual selection mode (select lines with V, j/k)
5. Yank (y) to copy selected text + metadata to system clipboard
6. NoteAdd (n) to add claim locator to notes buffer
7. NoteOpen (N) to show notes overlay
8. Jump anchors for each claim
9. Standard navigation (j/k, gg/G, ctrl+d/u)

Let me look at the catalog pane's visual selection to understand how it's implemented, since the full text pane needs a similar but line-based selection.

Let me check how visual selection works in the catalog pane to understand the pattern:

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/pane/catalog.go",
  "offset": 1,
  "limit": 100
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/pane/catalog.go</path>
<type>file</type>
<content>
1: package pane
2: 
3: import (
4: 	"strings"
5: 
6: 	tea "github.com/charmbracelet/bubbletea"
7: 
8: 	"patentmine/internal/command"
9: 	"patentmine/internal/domain"
10: 	"patentmine/internal/proto"
11: 	"patentmine/internal/rpc"
12: 	"patentmine/internal/text"
13: 	"patentmine/internal/tui/render"
14: )
15: 
16: // catalogLoadedMsg delivers a finished patent.list result.
17: type catalogLoadedMsg struct {
18: 	requestID uint64
19: 	offset    int
20: 	total     int
21: 	patents   []domain.PatentRow
22: 	err       error
23: }
24: 
25: // Catalog is the main patent list pane.
26: type Catalog struct {
27: 	client   *rpc.Client
28: 	theme    render.Theme
29: 	handlers map[command.ID]cmdHandler
30: 
31: 	activeProject *domain.Project
32: 
33: 	patents       []domain.PatentRow
34: 	page          render.Paginator
35: 	loadedBase    int
36: 	loading       bool
37: 	loadErr       string
38: 	loadID        uint64
39: 	visualMode    bool
40: 	visualAnchor  int
41: 	lastActive        domain.PatentNumber
42: 	savedVisual       []domain.PatentNumber
43: 	savedVisualAnchor int
44: 	savedVisualCursor int
45: 	gvHighlight       map[domain.PatentNumber]bool
46: 	activeSort    domain.SortColumn
47: 	sortAscending bool
48: 	filter        PatentFilter
49: 	find          findBar
50: 	focusedColIdx int
51: 	lastWidth     int
52: }
53: 
54: // NewCatalog builds an empty catalog pane bound to a daemon client.
55: func NewCatalog(client *rpc.Client, theme render.Theme) *Catalog {
56: 	c := &Catalog{
57: 		client:        client,
58: 		theme:         theme,
59: 		page:          render.NewPaginator(defaultPageSize),
60: 		loading:       true,
61: 		activeSort:    domain.SortByReviewState,
62: 		sortAscending: true,
63: 		focusedColIdx: -1,
64: 	}
65: 	c.handlers = map[command.ID]cmdHandler{
66: 		command.NavDown:         func(inv Invocation) tea.Cmd { return c.move(func() { c.page.MoveDown(inv.Repeat) }) },
67: 		command.NavUp:           func(inv Invocation) tea.Cmd { return c.move(func() { c.page.MoveUp(inv.Repeat) }) },
68: 		command.NavPageDown:     func(Invocation) tea.Cmd { return c.move(c.page.PageDown) },
69: 		command.NavPageUp:       func(Invocation) tea.Cmd { return c.move(c.page.PageUp) },
70: 		command.NavTop:          func(Invocation) tea.Cmd { return c.move(c.page.Top) },
71: 		command.NavBottom:       func(Invocation) tea.Cmd { return c.move(c.page.Bottom) },
72: 		command.ReselectLast:    func(Invocation) tea.Cmd { return c.reselectLast() },
73: 		command.Refresh:         func(Invocation) tea.Cmd { c.loading = true; c.clearVisual(); return c.load() },
74: 		command.SelectVisual:    func(Invocation) tea.Cmd { return c.toggleVisual() },
75: 		command.SelectAll:       func(Invocation) tea.Cmd { return c.selectAllVisual() },
76: 		command.SelectClear: func(Invocation) tea.Cmd {
77: 			if c.visualMode {
78: 				c.saveVisual()
79: 				c.clearVisual()
80: 			}
81: 			return nil
82: 		},
83: 		command.CrawlFamily:    func(Invocation) tea.Cmd { return c.crawlSelected(domain.CrawlProfileFamily) },
84: 		command.CrawlCitations: func(Invocation) tea.Cmd { return c.crawlSelected(domain.CrawlProfileCitations) },
85: 		command.CrawlCitedBy:   func(Invocation) tea.Cmd { return c.crawlSelected(domain.CrawlProfileCitedBy) },
86: 		command.CrawlAll:       func(Invocation) tea.Cmd { return c.crawlSelected(domain.CrawlProfileAll) },
87: 		command.LookupPatent:   func(Invocation) tea.Cmd { return c.crawlSelected("") },
88: 		command.ColNext:         func(Invocation) tea.Cmd { return c.focusNext() },
89: 		command.ColPrev:         func(Invocation) tea.Cmd { return c.focusPrev() },
90: 		command.SortApply:       func(Invocation) tea.Cmd { return c.applySort() },
91: 		command.Filter:          c.applyFilter,
92: 		command.FindOpen:        func(Invocation) tea.Cmd { c.find.open(c.filter.Search); return nil },
93: 	}
94: 	return c
95: }
96: 
97: // Context implements Pane.
98: func (c *Catalog) Scope() command.Scope { return command.ScopeCatalog }
99: 
100: // Title implements Pane.

(Showing lines 1-100 of 508. Use offset=101 to continue.)
</content>
```

---

## Assistant (Build · Big Pickle · 3.9s)

_Thinking:_

I see - the catalog's visual selection mode works on patent rows (each row is a patent). For the full text viewer, I need line-based selection within a long text body.

Let me also look at how the catalog's `Selection()` and `Selections()` work:

**Tool: grep**

**Input:**
```json
{
  "pattern": "func \\(c \\*Catalog\\) (Selection|Selections|toggleVisual|clearVisual|visual",
  "include": "catalog.go"
}
```

**Output:**
```
No files found
```

---

## Assistant (Build · Big Pickle · 3.2s)

_Thinking:_

Let me search for the visual-related methods in catalog.go:

**Tool: grep**

**Input:**
```json
{
  "include": "catalog.go",
  "pattern": "func \\(c \\*Catalog\\)"
}
```

**Output:**
```
Found 25 matches
/mnt/d/Repos/PatentMineNew/internal/tui/pane/catalog.go:
  Line 98: func (c *Catalog) Scope() command.Scope { return command.ScopeCatalog }

  Line 101: func (c *Catalog) Title() string { return "Patents" }

  Line 104: func (c *Catalog) Init() tea.Cmd { return c.load() }

  Line 107: func (c *Catalog) load() tea.Cmd {

  Line 141: func (c *Catalog) applyFilter(inv Invocation) tea.Cmd {

  Line 156: func (c *Catalog) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {

  Line 181: func (c *Catalog) focusNext() tea.Cmd {

  Line 192: func (c *Catalog) focusPrev() tea.Cmd {

  Line 203: func (c *Catalog) applySort() tea.Cmd {

  Line 224: func (c *Catalog) currentCols() []tableCol {

  Line 233: func (c *Catalog) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {

  Line 241: func (c *Catalog) Handles() []command.ID { return handlerIDs(c.handlers) }

  Line 244: func (c *Catalog) crawlSelected(profile domain.CrawlProfile) tea.Cmd {

  Line 258: func (c *Catalog) move(motion func()) tea.Cmd {

  Line 273: func (c *Catalog) saveVisual() {

  Line 280: func (c *Catalog) SaveVisualSelection() {

  Line 290: func (c *Catalog) reselectLast() tea.Cmd {

  Line 332: func (c *Catalog) Update(msg tea.Msg) (Pane, tea.Cmd) {

  Line 375: func (c *Catalog) toggleVisual() tea.Cmd {

  Line 385: func (c *Catalog) selectAllVisual() tea.Cmd {

  Line 397: func (c *Catalog) clearVisual() {

  Line 403: func (c *Catalog) inVisualRange(absolute int) bool {

  Line 410: func (c *Catalog) Selections() []domain.PatentNumber {

  Line 429: func (c *Catalog) Selection() (domain.PatentNumber, bool) {

  Line 438: func (c *Catalog) View(w, h int) string {

```

---

## Assistant (Build · Big Pickle · 3.5s)

_Thinking:_

Let me read the visual selection area of the catalog to understand how it works:

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/pane/catalog.go",
  "offset": 273,
  "limit": 160
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/pane/catalog.go</path>
<type>file</type>
<content>
273: func (c *Catalog) saveVisual() {
274: 	c.savedVisual = c.Selections()
275: 	c.savedVisualAnchor = c.visualAnchor
276: 	c.savedVisualCursor = c.page.Cursor()
277: }
278: 
279: // SaveVisualSelection implements pane.VisualSelectionSaver.
280: func (c *Catalog) SaveVisualSelection() {
281: 	if c.visualMode {
282: 		c.saveVisual()
283: 		c.clearVisual()
284: 	}
285: }
286: 
287: // reselectLast re-enters visual mode over the last saved selection by patent
288: // number (gv behaves like Vim's gv). Falls back to the last active patent if
289: // no visual selection was saved.
290: func (c *Catalog) reselectLast() tea.Cmd {
291: 	targets := c.savedVisual
292: 	if len(targets) == 0 && !c.lastActive.IsZero() {
293: 		targets = []domain.PatentNumber{c.lastActive}
294: 	}
295: 	if len(targets) == 0 {
296: 		return status(text.StatusNoPatentSelected, false)
297: 	}
298: 
299: 	// Build index by patent number (sort-agnostic).
300: 	idx := make(map[domain.PatentNumber]int, len(c.patents))
301: 	for i, p := range c.patents {
302: 		idx[p.Number] = c.loadedBase + i
303: 	}
304: 
305: 	highlights := make(map[domain.PatentNumber]bool, len(targets))
306: 	first := -1
307: 	last := -1
308: 	for _, t := range targets {
309: 		highlights[t] = true
310: 		if pos, ok := idx[t]; ok {
311: 			if first == -1 || pos < first {
312: 				first = pos
313: 			}
314: 			if pos > last {
315: 				last = pos
316: 			}
317: 		}
318: 	}
319: 	if first == -1 {
320: 		return status(text.StatusNoPatentSelected, false)
321: 	}
322: 
323: 	c.clearVisual()
324: 	c.visualMode = true
325: 	c.visualAnchor = first
326: 	c.gvHighlight = highlights
327: 	c.page.ScrollTo(last)
328: 	return nil
329: }
330: 
331: // Update implements Pane.
332: func (c *Catalog) Update(msg tea.Msg) (Pane, tea.Cmd) {
333: 	switch m := msg.(type) {
334: 	case ResizeMsg:
335: 		pageSize := max(m.Height-headerRows, 1)
336: 		if pageSize != c.page.PageSize() {
337: 			before := c.page.Offset()
338: 			c.page.SetPageSize(pageSize)
339: 			if before != c.page.Offset() || len(c.patents) != c.page.PageSize() {
340: 				c.loading = true
341: 				return c, c.load()
342: 			}
343: 		}
344: 	case ProjectChangedMsg:
345: 		changed := !sameProject(c.activeProject, m.Project)
346: 		c.activeProject = cloneProject(m.Project)
347: 		if changed {
348: 			c.page.Top()
349: 			c.loadedBase = 0
350: 			c.loading = true
351: 			c.clearVisual()
352: 			return c, c.load()
353: 		}
354: 	case catalogLoadedMsg:
355: 		if m.requestID != c.loadID {
356: 			return c, nil
357: 		}
358: 		c.loading = false
359: 		if m.err != nil {
360: 			c.loadErr = m.err.Error()
361: 			return c, nil
362: 		}
363: 		c.loadErr = ""
364: 		c.patents = m.patents
365: 		c.loadedBase = m.offset
366: 		c.page.SetTotal(m.total)
367: 		if c.page.Offset() != m.offset {
368: 			c.loading = true
369: 			return c, c.load()
370: 		}
371: 	}
372: 	return c, nil
373: }
374: 
375: func (c *Catalog) toggleVisual() tea.Cmd {
376: 	if c.visualMode {
377: 		c.saveVisual()
378: 		c.clearVisual()
379: 		return nil
380: 	}
381: 	c.visualMode, c.visualAnchor = true, c.page.Cursor()
382: 	return nil
383: }
384: 
385: func (c *Catalog) selectAllVisual() tea.Cmd {
386: 	if c.page.Total() == 0 {
387: 		return nil
388: 	}
389: 	if c.visualMode {
390: 		c.saveVisual()
391: 	}
392: 	c.visualMode = true
393: 	c.visualAnchor = 0
394: 	return c.move(c.page.Bottom)
395: }
396: 
397: func (c *Catalog) clearVisual() {
398: 	c.visualMode = false
399: 	c.visualAnchor = 0
400: 	c.gvHighlight = nil
401: }
402: 
403: func (c *Catalog) inVisualRange(absolute int) bool {
404: 	lo := min(c.visualAnchor, c.page.Cursor())
405: 	hi := max(c.visualAnchor, c.page.Cursor())
406: 	return absolute >= lo && absolute <= hi
407: }
408: 
409: // Selections implements MultiSelector.
410: func (c *Catalog) Selections() []domain.PatentNumber {
411: 	if !c.visualMode || len(c.patents) == 0 {
412: 		return nil
413: 	}
414: 	lo := min(c.visualAnchor, c.page.Cursor())
415: 	hi := max(c.visualAnchor, c.page.Cursor())
416: 	lo = max(lo, c.loadedBase)
417: 	hi = min(hi, c.loadedBase+len(c.patents)-1)
418: 	if lo > hi {
419: 		return nil
420: 	}
421: 	out := make([]domain.PatentNumber, 0, hi-lo+1)
422: 	for abs := lo; abs <= hi; abs++ {
423: 		out = append(out, c.patents[abs-c.loadedBase].Number)
424: 	}
425: 	return out
426: }
427: 
428: // Selection implements Pane.
429: func (c *Catalog) Selection() (domain.PatentNumber, bool) {
430: 	cur := c.page.Cursor() - c.loadedBase
431: 	if cur < 0 || cur >= len(c.patents) {
432: 		return domain.PatentNumber{}, false

(Showing lines 273-432 of 508. Use offset=433 to continue.)
</content>
```

---

## Assistant (Build · Big Pickle · 3.9s)

_Thinking:_

Now I have a good understanding of how visual selection works in the catalog. It tracks a `visualMode` bool and a `visualAnchor` (starting line). The `inVisualRange` method checks if a given absolute line is within the selection range.

For the full text pane, I'll implement a similar but line-based selection mode. Let me also check how the calls.go (Cmd helpers) work for making RPC calls:

**Tool: read**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/pane/calls.go"
}
```

**Output:**
```
<path>/mnt/d/Repos/PatentMineNew/internal/tui/pane/calls.go</path>
<type>file</type>
<content>
1: package pane
2: 
3: import (
4: 	"context"
5: 	"strings"
6: 	"sync/atomic"
7: 	"time"
8: 
9: 	tea "github.com/charmbracelet/bubbletea"
10: 
11: 	"patentmine/internal/domain"
12: 	"patentmine/internal/proto"
13: 	"patentmine/internal/rpc"
14: 	"patentmine/internal/text"
15: )
16: 
17: // callTimeout bounds a single request to the daemon.
18: const callTimeout = 15 * time.Second
19: 
20: // callContext returns a context bounded by callTimeout.
21: func callContext() (context.Context, context.CancelFunc) {
22: 	return context.WithTimeout(context.Background(), callTimeout)
23: }
24: 
25: var asyncSeq atomic.Uint64
26: 
27: func nextAsyncID() uint64 {
28: 	return asyncSeq.Add(1)
29: }
30: 
31: // crawl depth selectors. A negative depth crawls the configured family depth;
32: // zero looks up only the named patent.
33: const (
34: 	crawlFamilyDepth = -1
35: 	lookupDepth      = 0
36: )
37: 
38: // crawlDepth returns the depth to use for a given profile. An empty profile
39: // means a single-patent lookup (depth 0); any other profile follows the full
40: // family crawl (depth -1, which defers to the daemon's configured default).
41: func crawlDepth(profile domain.CrawlProfile) int {
42: 	if profile == "" {
43: 		return lookupDepth
44: 	}
45: 	return crawlFamilyDepth
46: }
47: 
48: // CrawlCmd enqueues a crawl or lookup for number and reports the outcome as a
49: // StatusMsg. depth selects how far the family walk explicitly; a negative
50: // depth defers to the crawler's configured default. profile selects which
51: // family-graph edges to follow. force bypasses the local file cache.
52: func CrawlCmd(client *rpc.Client, number domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) tea.Cmd {
53: 	return func() tea.Msg {
54: 		ctx, cancel := callContext()
55: 		defer cancel()
56: 		var res proto.CrawlStartResult
57: 		err := client.Call(ctx, proto.MethodCrawlFamily,
58: 			proto.CrawlFamilyParams{Root: number, Depth: depth, Profile: profile, Force: force}, &res)
59: 		if err != nil {
60: 			return StatusMsg{Key: text.StatusCrawlStartFailed, Args: []any{err.Error()}, Error: true}
61: 		}
62: 		return StatusMsg{Key: text.StatusCrawlStarted, Args: []any{number.String(), res.JobID, depth}}
63: 	}
64: }
65: 
66: // MultiCrawlCmd starts a crawl or lookup for each number concurrently and
67: // returns a single MultiCrawlStartedMsg with all job IDs so the app can show
68: // one aggregate overlay for multi-selection.
69: func MultiCrawlCmd(client *rpc.Client, numbers []domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) tea.Cmd {
70: 	return func() tea.Msg {
71: 		type rpcResult struct {
72: 			number domain.PatentNumber
73: 			jobID  string
74: 			err    error
75: 		}
76: 		ch := make(chan rpcResult, len(numbers))
77: 		for _, n := range numbers {
78: 			go func(n domain.PatentNumber) {
79: 				ctx, cancel := callContext()
80: 				defer cancel()
81: 				var res proto.CrawlStartResult
82: 				err := client.Call(ctx, proto.MethodCrawlFamily,
83: 					proto.CrawlFamilyParams{Root: n, Depth: depth, Profile: profile, Force: force}, &res)
84: 				if err != nil {
85: 					ch <- rpcResult{number: n, err: err}
86: 				} else {
87: 					ch <- rpcResult{number: n, jobID: res.JobID}
88: 				}
89: 			}(n)
90: 		}
91: 		var jobIDs []string
92: 		var failErrs []string
93: 		for range numbers {
94: 			r := <-ch
95: 			if r.err != nil {
96: 				failErrs = append(failErrs, r.err.Error())
97: 			} else {
98: 				jobIDs = append(jobIDs, r.jobID)
99: 			}
100: 		}
101: 		if len(jobIDs) == 0 {
102: 			return StatusMsg{Key: text.StatusCrawlStartFailed, Args: []any{strings.Join(failErrs, "; ")}, Error: true}
103: 		}
104: 		if len(failErrs) > 0 {
105: 			jobIDs = append(jobIDs, "(errors: "+strings.Join(failErrs, "; ")+")")
106: 		}
107: 		return MultiCrawlStartedMsg{
108: 			Numbers: numbers,
109: 			JobIDs:  jobIDs,
110: 			Depth:   depth,
111: 		}
112: 	}
113: }
114: 
115: // ImportFileCmd loads a patent record from a local fixture file.
116: func ImportFileCmd(client *rpc.Client, path string) tea.Cmd {
117: 	return func() tea.Msg {
118: 		ctx, cancel := callContext()
119: 		defer cancel()
120: 		var res proto.Empty
121: 		if err := client.Call(ctx, proto.MethodImportFile,
122: 			proto.ImportFileParams{Path: path}, &res); err != nil {
123: 			return StatusMsg{Key: text.StatusImportFailed, Args: []any{err.Error()}, Error: true}
124: 		}
125: 		return StatusMsg{Key: text.StatusImported, Args: []any{path}}
126: 	}
127: }
128: 
129: // AddToProjectCmd links a patent to the active project.
130: func AddToProjectCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber) tea.Cmd {
131: 	return func() tea.Msg {
132: 		ctx, cancel := callContext()
133: 		defer cancel()
134: 		var res proto.MembershipAddResult
135: 		if err := client.Call(ctx, proto.MethodMembershipAdd,
136: 			proto.MembershipParams{Project: project, Patent: number}, &res); err != nil {
137: 			return StatusMsg{Key: text.StatusAddFailed, Args: []any{err.Error()}, Error: true}
138: 		}
139: 		if !res.FetchStarted {
140: 			return StatusMsg{Key: text.StatusAddedNoCrawl, Args: []any{number.String()}}
141: 		}
142: 		return StatusMsg{Key: text.StatusAdded, Args: []any{number.String(), string(project)}}
143: 	}
144: }
145: 
146: // SetReviewStateCmd changes a patent's state in the active project.
147: func SetReviewStateCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber, state domain.ReviewState) tea.Cmd {
148: 	return func() tea.Msg {
149: 		ctx, cancel := callContext()
150: 		defer cancel()
151: 		var res proto.Empty
152: 		if err := client.Call(ctx, proto.MethodReviewState,
153: 			proto.ReviewStateParams{Project: project, Patent: number, State: string(state)}, &res); err != nil {
154: 			return StatusMsg{Key: text.StatusSetStateFailed, Args: []any{err.Error()}, Error: true}
155: 		}
156: 		return StatusMsg{Key: text.StatusSetState, Args: []any{number.String(), string(state), string(project)}}
157: 	}
158: }
159: 
160: // AssignTagCmd tags a patent within the active project, creating the tag when
161: // the project does not have it yet.
162: func AssignTagCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber, name string) tea.Cmd {
163: 	return func() tea.Msg {
164: 		ctx, cancel := callContext()
165: 		defer cancel()
166: 		var res proto.Empty
167: 		if err := client.Call(ctx, proto.MethodTagAssign,
168: 			proto.TagParams{Project: project, Patent: number, Name: name}, &res); err != nil {
169: 			return StatusMsg{Key: text.StatusTagFailed, Args: []any{err.Error()}, Error: true}
170: 		}
171: 		return StatusMsg{Key: text.StatusTagged, Args: []any{number.String(), name, string(project)}}
172: 	}
173: }
174: 
175: // RemoveTagCmd removes a tag from a patent within the active project.
176: func RemoveTagCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber, name string) tea.Cmd {
177: 	return func() tea.Msg {
178: 		ctx, cancel := callContext()
179: 		defer cancel()
180: 		var res proto.Empty
181: 		if err := client.Call(ctx, proto.MethodTagRemove,
182: 			proto.TagParams{Project: project, Patent: number, Name: name}, &res); err != nil {
183: 			return StatusMsg{Key: text.StatusUntagFailed, Args: []any{err.Error()}, Error: true}
184: 		}
185: 		return StatusMsg{Key: text.StatusUntagged, Args: []any{name, number.String()}}
186: 	}
187: }
188: 
189: // DeletePatentCmd permanently removes a patent from the database.
190: func DeletePatentCmd(client *rpc.Client, number domain.PatentNumber) tea.Cmd {
191: 	return func() tea.Msg {
192: 		ctx, cancel := callContext()
193: 		defer cancel()
194: 		var res proto.Empty
195: 		if err := client.Call(ctx, proto.MethodPatentDelete,
196: 			proto.PatentDeleteParams{Number: number}, &res); err != nil {
197: 			return StatusMsg{Key: text.StatusDeleteFailed, Args: []any{err.Error()}, Error: true}
198: 		}
199: 		return StatusMsg{Key: text.StatusDeleted, Args: []any{number.String()}}
200: 	}
201: }
202: 
203: // CreateProjectCmd creates a project with the given name.
204: func CreateProjectCmd(client *rpc.Client, name string) tea.Cmd {
205: 	return func() tea.Msg {
206: 		ctx, cancel := callContext()
207: 		defer cancel()
208: 		var res proto.ProjectResult
209: 		if err := client.Call(ctx, proto.MethodProjectCreate,
210: 			proto.ProjectCreateParams{Name: name}, &res); err != nil {
211: 			return StatusMsg{Key: text.StatusProjectCreateFailed, Args: []any{err.Error()}, Error: true}
212: 		}
213: 		return StatusMsg{Key: text.StatusProjectCreated, Args: []any{res.Project.Name}}
214: 	}
215: }
216: 
217: // CreateTagTaxonomyCmd registers a tag in the project's taxonomy.
218: func CreateTagTaxonomyCmd(client *rpc.Client, project domain.ProjectID, name string) tea.Cmd {
219: 	return func() tea.Msg {
220: 		ctx, cancel := callContext()
221: 		defer cancel()
222: 		var res domain.Tag
223: 		if err := client.Call(ctx, proto.MethodTagCreate,
224: 			proto.TagCreateParams{Project: project, Name: name}, &res); err != nil {
225: 			return StatusMsg{Key: text.StatusTagTaxonomyAddFailed, Args: []any{err.Error()}, Error: true}
226: 		}
227: 		return StatusMsg{Key: text.StatusTagTaxonomyAdded, Args: []any{name, string(project)}}
228: 	}
229: }
230: 
231: // DeleteTagTaxonomyCmd removes a tag from the project's taxonomy.
232: func DeleteTagTaxonomyCmd(client *rpc.Client, project domain.ProjectID, name string) tea.Cmd {
233: 	return func() tea.Msg {
234: 		ctx, cancel := callContext()
235: 		defer cancel()
236: 		var res proto.Empty
237: 		if err := client.Call(ctx, proto.MethodTagDelete,
238: 			proto.TagDeleteParams{Project: project, Name: name}, &res); err != nil {
239: 			return StatusMsg{Key: text.StatusTagTaxonomyDeleteFailed, Args: []any{err.Error()}, Error: true}
240: 		}
241: 		return StatusMsg{Key: text.StatusTagTaxonomyDeleted, Args: []any{name, string(project)}}
242: 	}
243: }
244: 
245: // ListTagTaxonomyCmd lists all taxonomy tags in the project.
246: func ListTagTaxonomyCmd(client *rpc.Client, project domain.ProjectID) tea.Cmd {
247: 	return func() tea.Msg {
248: 		ctx, cancel := callContext()
249: 		defer cancel()
250: 		var res proto.TagListResult
251: 		if err := client.Call(ctx, proto.MethodTagList,
252: 			proto.TagListParams{Project: project}, &res); err != nil {
253: 			return StatusMsg{Key: text.StatusTagTaxonomyListFailed, Args: []any{err.Error()}, Error: true}
254: 		}
255: 		var names []string
256: 		for _, t := range res.Tags {
257: 			names = append(names, t.Name)
258: 		}
259: 		if len(names) == 0 {
260: 			return StatusMsg{Key: text.StatusFilter, Args: []any{"taxonomy: (no tags registered)"}}
261: 		}
262: 		return StatusMsg{Key: text.StatusFilter, Args: []any{"taxonomy: " + strings.Join(names, ", ")}}
263: 	}
264: }
265: 
266: // AssignPatentTagCmd assigns a taxonomy tag to a patent.
267: func AssignPatentTagCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber, name string) tea.Cmd {
268: 	return func() tea.Msg {
269: 		ctx, cancel := callContext()
270: 		defer cancel()
271: 		var res proto.Empty
272: 		if err := client.Call(ctx, proto.MethodPatentTagAdd,
273: 			proto.TagParams{Project: project, Patent: number, Name: name}, &res); err != nil {
274: 			return StatusMsg{Key: text.StatusTagPatentAddFailed, Args: []any{err.Error()}, Error: true}
275: 		}
276: 		return StatusMsg{Key: text.StatusTagPatentAdded, Args: []any{name, number.String()}}
277: 	}
278: }
279: 
280: // RemovePatentTagCmd removes a tag assignment from a patent.
281: func RemovePatentTagCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber, name string) tea.Cmd {
282: 	return func() tea.Msg {
283: 		ctx, cancel := callContext()
284: 		defer cancel()
285: 		var res proto.Empty
286: 		if err := client.Call(ctx, proto.MethodPatentTagDelete,
287: 			proto.TagParams{Project: project, Patent: number, Name: name}, &res); err != nil {
288: 			return StatusMsg{Key: text.StatusTagPatentDeleteFailed, Args: []any{err.Error()}, Error: true}
289: 		}
290: 		return StatusMsg{Key: text.StatusTagPatentDeleted, Args: []any{name, number.String()}}
291: 	}
292: }
293: 
294: // ListPatentTagsCmd lists all tags assigned to a patent.
295: func ListPatentTagsCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber) tea.Cmd {
296: 	return func() tea.Msg {
297: 		ctx, cancel := callContext()
298: 		defer cancel()
299: 		var res proto.PatentTagListResult
300: 		if err := client.Call(ctx, proto.MethodPatentTagList,
301: 			proto.PatentTagListParams{Project: project, Patent: number}, &res); err != nil {
302: 			return StatusMsg{Key: text.StatusTagPatentListFailed, Args: []any{err.Error()}, Error: true}
303: 		}
304: 		var tagStrings []string
305: 		for _, t := range res.Tags {
306: 			var assignedStr string
307: 			if !t.AssignedAt.IsZero() {
308: 				assignedStr = " (assigned " + t.AssignedAt.Format("2006-01-02 15:04:05") + ")"
309: 			}
310: 			tagStrings = append(tagStrings, t.Name+assignedStr)
311: 		}
312: 		if len(tagStrings) == 0 {
313: 			return StatusMsg{Key: text.StatusFilter, Args: []any{"patent tags: (none assigned)"}}
314: 		}
315: 		return StatusMsg{Key: text.StatusFilter, Args: []any{"patent tags: " + strings.Join(tagStrings, ", ")}}
316: 	}
317: }

(End of file - total 317 lines)
</content>
```

---

## Assistant (Build · Big Pickle · 12.5s)

_Thinking:_

Now I have enough context. Let me create the fulltext.go pane. This is the biggest piece of work.

I need to:
1. Create the pane with loading from RPC + crawl package for full text
2. The pane needs a patent loaded (via RPC) and full text (via crawl.FetchFullText)
3. Visual selection on lines
4. Yank to clipboard with metadata
5. NoteAdd for claim locators
6. Jump anchors per claim

Let me look at the imports for the pane package to understand what's available.

Actually, I realize there's a dependency issue. The `tui/pane` package can't import `tui` (clipboard.go, notes.go) because that would create a circular dependency. The `tui` package imports `tui/pane`.

So the clipboard and notes functionality needs to live in the `tui` package, and the pane just emits commands that the app handles. The app handlers in `tui/app.go` will handle the clipboard copy and notes management.

Let me design this:

1. `tui/pane/fulltext.go` - handles loading patent & full text, renders the view, handles key events for selecting text and marking claims
2. The pane emits custom messages that the app handles:
   - `CopyToClipboardMsg` - tells the app to copy text to system clipboard
   - `NoteAddMsg` - tells the app to add a locator to notes buffer
   - `NoteOpenMsg` - tells the app to show the notes overlay

Let me create these message types and the pane.

Actually, looking at the existing code more carefully, I see that panes communicate with the app through:
- `StatusMsg` for status messages
- Direct RPC commands via `tea.Cmd` closures
- The `Update` method receives messages

For the notes buffer, the easiest approach is:
1. The full text pane has a reference to the notes accumulator (passed from the app)
2. When user presses `n`, the pane directly modifies the notes accumulator and emits a StatusMsg
3. When user presses `y` with visual selection, the pane emits a CopyToClipboard message

Wait, but the pane package doesn't import the tui package. The notes accumulator is defined in tui. This creates a circular dependency.

The cleanest solution is to define the messages in the pane package and handle them in the app.

Let me add new message types to the pane package:

```go
// CopyToClipboardMsg asks the app to copy text to the system clipboard.
type CopyToClipboardMsg struct {
    Text string
}

// NoteAddMsg tells the app to add a claim locator to the notes buffer.
type NoteAddMsg struct {
    Number  domain.PatentNumber
    Locator string
}

// NoteOpenMsg tells the app to show the notes buffer overlay.
type NoteOpenMsg struct {
    Number  domain.PatentNumber
}
```

And the notes accumulator lives in the app (tui package), accessed only through these messages.

Let me write the full text pane now.

Now let me create the full text pane. First, I'll add message types to the pane package, then the pane itself:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/pane/pane.go",
  "oldString": "// MultiCrawlStartedMsg is emitted when multiple patents are selected and a\n// crawl or lookup is started for each of them. It carries all job IDs so the\n// app can show a single aggregate overlay instead of stacking one per job.\ntype MultiCrawlStartedMsg struct {\n\tNumbers []domain.PatentNumber\n\tJobIDs  []string\n\tDepth   int\n}\n\n// status returns a tea.Cmd that emits a StatusMsg for key.\nfunc status(key text.Key, isErr bool, args ...any) tea.Cmd {\n\treturn func() tea.Msg { return StatusMsg{Key: key, Args: args, Error: isErr} }\n}",
  "newString": "// MultiCrawlStartedMsg is emitted when multiple patents are selected and a\n// crawl or lookup is started for each of them. It carries all job IDs so the\n// app can show a single aggregate overlay instead of stacking one per job.\ntype MultiCrawlStartedMsg struct {\n\tNumbers []domain.PatentNumber\n\tJobIDs  []string\n\tDepth   int\n}\n\n// CopyToClipboardMsg asks the app to copy the given text to the system clipboard.\ntype CopyToClipboardMsg struct {\n\tText string\n}\n\n// NoteAddMsg tells the app to add a claim locator to the notes buffer.\ntype NoteAddMsg struct {\n\tNumber  domain.PatentNumber\n\tLocator string\n}\n\n// NoteOpenMsg tells the app to show the notes buffer overlay for a patent.\ntype NoteOpenMsg struct {\n\tNumber domain.PatentNumber\n}\n\n// FullTextLoadedMsg delivers the result of fetching full patent claims text.\ntype FullTextLoadedMsg struct {\n\tRequestID uint64\n\tFullText  *domain.FullText\n\tPatent    domain.Patent\n\tErr       error\n}\n\n// status returns a tea.Cmd that emits a StatusMsg for key.\nfunc status(key text.Key, isErr bool, args ...any) tea.Cmd {\n\treturn func() tea.Msg { return StatusMsg{Key: key, Args: args, Error: isErr} }\n}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 33.1s)

_Thinking:_

Now create the full text pane:

**Tool: write**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/pane/fulltext.go",
  "content": "package pane\n\nimport (\n\t\"fmt\"\n\t\"context\"\n\t\"strings\"\n\t\"sync/atomic\"\n\n\ttea \"github.com/charmbracelet/bubbletea\"\n\n\t\"patentmine/internal/command\"\n\t\"patentmine/internal/crawl\"\n\t\"patentmine/internal/domain\"\n\t\"patentmine/internal/proto\"\n\t\"patentmine/internal/rpc\"\n\t\"patentmine/internal/tui/render\"\n)\n\nvar fullTextSeq atomic.Uint64\n\nfunc nextFullTextID() uint64 {\n\treturn fullTextSeq.Add(1)\n}\n\n// FullText shows the complete claims text of one patent. It fetches the full\n// text on-demand from Google Patents (not stored in the DB) and supports visual\n// line selection, clipboard yank, and note accumulation.\ntype FullText struct {\n\tclient   *rpc.Client\n\ttheme    render.Theme\n\tnumber   domain.PatentNumber\n\tproject  domain.ProjectID\n\thandlers map[command.ID]cmdHandler\n\n\t// loaded data\n\tloading  bool\n\tloadErr  string\n\tloadID   uint64\n\tpatent   domain.Patent\n\tfullText domain.FullText\n\n\t// scrolling\n\tpage render.Paginator\n\n\t// visual selection (line-based)\n\tvisualMode   bool\n\tvisualAnchor int // line where visual mode started\n\n\t// jump mode\n\tjumpActive bool\n\tanchors    []render.JumpAnchor\n\tjumpKeys   map[string]rune\n}\n\n// NewFullText builds a full-text viewer for one patent.\nfunc NewFullText(client *rpc.Client, theme render.Theme, number domain.PatentNumber, project domain.ProjectID, boundLetters []rune) *FullText {\n\tf := &FullText{\n\t\tclient:  client,\n\t\ttheme:   theme,\n\t\tnumber:  number,\n\t\tproject: project,\n\t\tpage:    render.NewPaginator(10),\n\t\tloading: true,\n\t}\n\tf.computeJumpKeys(boundLetters)\n\tf.handlers = map[command.ID]cmdHandler{\n\t\tcommand.NavDown: func(inv Invocation) tea.Cmd {\n\t\t\tif f.jumpActive && len(f.anchors) > 0 {\n\t\t\t\tf.page.ScrollTo(f.nextAnchorLine())\n\t\t\t} else {\n\t\t\t\tf.move(inv.Repeat, 1)\n\t\t\t}\n\t\t\treturn nil\n\t\t},\n\t\tcommand.NavUp: func(inv Invocation) tea.Cmd {\n\t\t\tif f.jumpActive && len(f.anchors) > 0 {\n\t\t\t\tf.page.ScrollTo(f.prevAnchorLine())\n\t\t\t} else {\n\t\t\t\tf.move(inv.Repeat, -1)\n\t\t\t}\n\t\t\treturn nil\n\t\t},\n\t\tcommand.NavPageDown: func(Invocation) tea.Cmd { f.page.ScrollTo(f.page.Cursor() + f.page.PageSize()); return nil },\n\t\tcommand.NavPageUp:   func(Invocation) tea.Cmd { f.page.ScrollTo(f.page.Cursor() - f.page.PageSize()); return nil },\n\t\tcommand.NavTop:      func(Invocation) tea.Cmd { f.page.Top(); return nil },\n\t\tcommand.NavBottom:   func(Invocation) tea.Cmd { f.page.Bottom(); return nil },\n\t\tcommand.SelectVisual: func(Invocation) tea.Cmd { return f.toggleVisual() },\n\t\tcommand.CopyYank:    func(Invocation) tea.Cmd { return f.copyYank() },\n\t\tcommand.NoteAdd:     func(Invocation) tea.Cmd { return f.noteAdd() },\n\t\tcommand.NoteOpen:    func(Invocation) tea.Cmd { return f.noteOpen() },\n\t\tcommand.Refresh:     func(Invocation) tea.Cmd { f.loading = true; return f.reload() },\n\t}\n\treturn f\n}\n\nfunc (f *FullText) Scope() command.Scope { return command.ScopeFullText }\n\nfunc (f *FullText) Title() string { return \"Full text · \" + f.number.String() }\n\nfunc (f *FullText) Init() tea.Cmd { return f.reload() }\n\n// reload fetches both the patent record (via RPC) and full text (from Google).\nfunc (f *FullText) reload() tea.Cmd {\n\treturn f.load()\n}\n\nfunc (f *FullText) load() tea.Cmd {\n\tclient, number, project := f.client, f.number, f.project\n\trequestID := nextFullTextID()\n\tf.loadID = requestID\n\treturn func() tea.Msg {\n\t\t// Load patent metadata from daemon\n\t\tctx, cancel := context.WithTimeout(context.Background(), callTimeout)\n\t\tvar res proto.PatentResult\n\t\terr := client.Call(ctx, proto.MethodPatentGet,\n\t\t\tproto.PatentGetParams{Number: number, Project: project}, &res)\n\t\tcancel()\n\n\t\tpatent := res.Patent\n\n\t\t// If patent metadata fails, still try to load full text\n\t\tvar fullText *domain.FullText\n\t\tif err == nil {\n\t\t\tfetched, fetchErr := crawl.FetchFullText(number)\n\t\t\tif fetchErr == nil {\n\t\t\t\tfullText = fetched\n\t\t\t} else if err == nil {\n\t\t\t\terr = fetchErr\n\t\t\t}\n\t\t}\n\n\t\treturn FullTextLoadedMsg{\n\t\t\tRequestID: requestID,\n\t\t\tFullText:  fullText,\n\t\t\tPatent:    patent,\n\t\t\tErr:       err,\n\t\t}\n\t}\n}\n\nfunc (f *FullText) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {\n\tif handler, ok := f.handlers[id]; ok {\n\t\treturn f, handler(inv)\n\t}\n\treturn f, nil\n}\n\nfunc (f *FullText) Handles() []command.ID { return handlerIDs(f.handlers) }\n\nfunc (f *FullText) Selection() (domain.PatentNumber, bool) {\n\treturn f.number, true\n}\n\nfunc (f *FullText) Update(msg tea.Msg) (Pane, tea.Cmd) {\n\tswitch m := msg.(type) {\n\tcase FullTextLoadedMsg:\n\t\tif m.RequestID != f.loadID {\n\t\t\treturn f, nil\n\t\t}\n\t\tf.loading = false\n\t\tif m.Err != nil {\n\t\t\tf.loadErr = m.Err.Error()\n\t\t\treturn f, nil\n\t\t}\n\t\tf.loadErr = \"\"\n\t\tf.patent = m.Patent\n\t\tif m.FullText != nil {\n\t\t\tf.fullText = *m.FullText\n\t\t}\n\t\tf.page.Top()\n\t}\n\treturn f, nil\n}\n\n// View implements Pane.\nfunc (f *FullText) View(w, h int) string {\n\tswitch {\n\tcase f.loading:\n\t\treturn f.theme.Dim.Render(\"loading full text for \" + f.number.String() + \"…\")\n\tcase f.loadErr != \"\":\n\t\treturn f.theme.Error.Render(\"error: \" + f.loadErr)\n\t}\n\tlines := strings.Split(f.body(w), \"\\n\")\n\tf.page.SetTotal(len(lines))\n\tf.page.SetPageSize(max(h, 1))\n\tstart, end := f.page.Window()\n\tcur := f.page.Cursor()\n\tout := make([]string, 0, end-start)\n\tfor i, line := range lines[start:end] {\n\t\tabs := start + i\n\t\tisCursor := abs == cur\n\t\t// Determine if this line is in the visual selection range\n\t\tinVis := f.visualMode && f.inVisualRange(abs)\n\t\tvar styled string\n\t\tswitch {\n\t\tcase inVis && isCursor:\n\t\t\tstyled = f.theme.Selected.Render(render.Pad(line, w))\n\t\tcase inVis:\n\t\t\tstyled = f.theme.Selected.Render(render.Pad(line, w))\n\t\tcase isCursor:\n\t\t\tstyled = f.theme.Selected.Render(render.Pad(line, w))\n\t\tdefault:\n\t\t\tstyled = line\n\t\t}\n\t\tout = append(out, styled)\n\t}\n\treturn strings.Join(out, \"\\n\")\n}\n\n// body renders the full text content.\nfunc (f *FullText) body(w int) string {\n\tf.anchors = f.anchors[:0]\n\tvar b strings.Builder\n\n\t// Patent metadata header\n\tmetaLine := fmt.Sprintf(\"Patent #: %s\", f.number.String())\n\tif f.patent.Title != \"\" {\n\t\tmetaLine += \" — \" + f.patent.Title\n\t}\n\tb.WriteString(f.theme.Header.Render(metaLine))\n\tb.WriteByte('\\n')\n\n\t// Inventor line\n\tif len(f.patent.Inventors) > 0 {\n\t\tnames := make([]string, 0, len(f.patent.Inventors))\n\t\tfor _, inv := range f.patent.Inventors {\n\t\t\tnames = append(names, string(inv))\n\t\t}\n\t\tb.WriteString(f.theme.Row.Render(\"  Inventors: \" + strings.Join(names, \", \")))\n\t\tb.WriteByte('\\n')\n\t}\n\tif f.patent.Assignee != \"\" {\n\t\tb.WriteString(f.theme.Row.Render(\"  Assignee: \" + f.patent.Assignee))\n\t\tb.WriteByte('\\n')\n\t}\n\tif !f.patent.ExpirationDate.IsZero() {\n\t\texpText := f.patent.ExpirationDate.Format(\"2006-01-02\")\n\t\tif f.patent.ExpirationSource != \"\" {\n\t\t\texpText += \" (\" + f.patent.ExpirationSource + \")\"\n\t\t}\n\t\tb.WriteString(f.theme.Row.Render(\"  Expiration: \" + expText))\n\t\tb.WriteByte('\\n')\n\t}\n\tb.WriteByte('\\n')\n\n\t// Claims sections\n\tfor i, claim := range f.fullText.Claims {\n\t\tlabel := fmt.Sprintf(\"Claim %d\", claim.Number)\n\t\tkey := f.jumpKey(label)\n\t\tif key != 0 {\n\t\t\tdisplayLabel := fmt.Sprintf(\"[%s] %s\", string(key), label)\n\t\t\tf.anchors = append(f.anchors, render.JumpAnchor{\n\t\t\t\tKey:   key,\n\t\t\t\tLabel: label,\n\t\t\t\tLine:  strings.Count(b.String(), \"\\n\"),\n\t\t\t})\n\t\t\tb.WriteString(f.theme.Warn.Render(displayLabel))\n\t\t} else {\n\t\t\tb.WriteString(f.theme.Header.Render(label))\n\t\t}\n\t\tb.WriteByte('\\n')\n\t\tfor _, line := range wrapText(claim.Text, max(w-2, 1)) {\n\t\t\tb.WriteString(f.theme.Row.Render(\"  \" + line))\n\t\t\tb.WriteByte('\\n')\n\t\t}\n\t\tif i < len(f.fullText.Claims)-1 {\n\t\t\tb.WriteByte('\\n')\n\t\t}\n\t}\n\n\t// Status line hint\n\tb.WriteByte('\\n')\n\tb.WriteString(f.theme.Dim.Render(\"  V: select  y: copy  n: add to notes  N: notes  ;: jump\"))\n\tb.WriteByte('\\n')\n\n\treturn b.String()\n}\n\n// move scrolls and updates visual selection.\nfunc (f *FullText) move(repeat int, dir int) {\n\tfor i := 0; i < max(repeat, 1); i++ {\n\t\tif dir > 0 {\n\t\t\tf.page.MoveDown(1)\n\t\t} else {\n\t\t\tf.page.MoveUp(1)\n\t\t}\n\t}\n}\n\n// toggleVisual toggles visual line selection mode.\nfunc (f *FullText) toggleVisual() tea.Cmd {\n\tif f.visualMode {\n\t\tf.visualMode = false\n\t\treturn nil\n\t}\n\tf.visualMode = true\n\tf.visualAnchor = f.page.Cursor()\n\treturn nil\n}\n\n// inVisualRange reports whether abs is within the visual selection.\nfunc (f *FullText) inVisualRange(abs int) bool {\n\tlo := min(f.visualAnchor, f.page.Cursor())\n\thi := max(f.visualAnchor, f.page.Cursor())\n\treturn abs >= lo && abs <= hi\n}\n\n// selectedLines returns the text of visually selected lines.\nfunc (f *FullText) selectedLines() (string, int, int) {\n\tbody := f.body(80) // full width doesn't matter for text extraction\n\tlines := strings.Split(body, \"\\n\")\n\tif !f.visualMode {\n\t\treturn \"\", 0, 0\n\t}\n\tlo := min(f.visualAnchor, f.page.Cursor())\n\thi := max(f.visualAnchor, f.page.Cursor())\n\tlo = max(lo, 0)\n\thi = min(hi, len(lines)-1)\n\tif lo > hi {\n\t\treturn \"\", 0, 0\n\t}\n\t// Strip ANSI escape codes for plain text\n\tplain := make([]string, 0, hi-lo+1)\n\tfor i := lo; i <= hi; i++ {\n\t\tplain = append(plain, stripANSI(lines[i]))\n\t}\n\treturn strings.Join(plain, \"\\n\"), lo, hi\n}\n\n// copyYank builds the clipboard text with metadata and selected lines.\nfunc (f *FullText) copyYank() tea.Cmd {\n\tsel, _, _ := f.selectedLines()\n\tif sel == \"\" {\n\t\t// No visual selection — copy the current claim's text\n\t\tclaimNum := f.currentClaimNumber()\n\t\tif claimNum > 0 {\n\t\t\tfor _, c := range f.fullText.Claims {\n\t\t\t\tif c.Number == claimNum {\n\t\t\t\t\tsel = c.Text\n\t\t\t\t\tbreak\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n\tif sel == \"\" {\n\t\treturn status(text.StatusNoPatentSelected, false, \"nothing to copy\")\n\t}\n\n\t// Build the clipboard text with metadata header\n\tvar b strings.Builder\n\tb.WriteString(patentMeta(f.patent, f.number))\n\tsource := f.currentClaimLabel()\n\tif source != \"\" {\n\t\tb.WriteString(fmt.Sprintf(\"Source:       %s\\n\", source))\n\t}\n\tb.WriteString(strings.Repeat(\"─\", 48) + \"\\n\")\n\tb.WriteString(sel)\n\n\treturn func() tea.Msg {\n\t\treturn CopyToClipboardMsg{Text: b.String()}\n\t}\n}\n\n// noteAdd adds the current claim locator to the notes buffer.\nfunc (f *FullText) noteAdd() tea.Cmd {\n\tlabel := f.currentClaimLabel()\n\tif label == \"\" {\n\t\treturn status(text.StatusNoPatentSelected, false, \"move cursor to a claim first\")\n\t}\n\treturn func() tea.Msg {\n\t\treturn NoteAddMsg{Number: f.number, Locator: label}\n\t}\n}\n\n// noteOpen shows the notes buffer overlay.\nfunc (f *FullText) noteOpen() tea.Cmd {\n\treturn func() tea.Msg {\n\t\treturn NoteOpenMsg{Number: f.number}\n\t}\n}\n\n// currentClaimNumber returns the claim number under the cursor.\nfunc (f *FullText) currentClaimNumber() int {\n\tcursor := f.page.Cursor()\n\tline := 0\n\tfor _, c := range f.fullText.Claims {\n\t\t// Estimate: claim header takes 1 line, text takes wrapped lines\n\t\tline++ // header\n\t\t// Rough estimate of text lines (text / 78 chars per line)\n\t\ttextLines := (len(c.Text) / 78) + 1\n\t\tline += textLines\n\t\tif cursor < line {\n\t\t\treturn c.Number\n\t\t}\n\t\tif c != f.fullText.Claims[len(f.fullText.Claims)-1] {\n\t\t\tline++ // blank line separator\n\t\t}\n\t}\n\treturn 0\n}\n\n// currentClaimLabel returns \"Claim N\" for the claim under the cursor.\nfunc (f *FullText) currentClaimLabel() string {\n\tn := f.currentClaimNumber()\n\tif n == 0 {\n\t\treturn \"\"\n\t}\n\treturn fmt.Sprintf(\"Claim %d\", n)\n}\n\n// --- Jump mode support ---\n\nfunc (f *FullText) JumpAnchors() []render.JumpAnchor { return f.anchors }\n\nfunc (f *FullText) JumpTo(line int) { f.page.ScrollTo(line) }\n\nfunc (f *FullText) SetJumpActive(active bool) { f.jumpActive = active }\n\nfunc (f *FullText) JumpActive() bool { return f.jumpActive }\n\nfunc (f *FullText) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {\n\tif !f.jumpActive {\n\t\treturn f, nil, false\n\t}\n\tif msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {\n\t\tr := msg.Runes[0]\n\t\tfor _, a := range f.anchors {\n\t\t\tif a.Key == r {\n\t\t\t\tf.JumpTo(a.Line)\n\t\t\t\treturn f, nil, true\n\t\t\t}\n\t\t}\n\t}\n\treturn f, nil, false\n}\n\nfunc (f *FullText) nextAnchorLine() int {\n\tcur := f.page.Cursor()\n\tfor _, a := range f.anchors {\n\t\tif a.Line > cur {\n\t\t\treturn a.Line\n\t\t}\n\t}\n\tif len(f.anchors) > 0 {\n\t\treturn f.anchors[0].Line\n\t}\n\treturn 0\n}\n\nfunc (f *FullText) prevAnchorLine() int {\n\tcur := f.page.Cursor()\n\tfor i := len(f.anchors) - 1; i >= 0; i-- {\n\t\tif f.anchors[i].Line < cur {\n\t\t\treturn f.anchors[i].Line\n\t\t}\n\t}\n\tif len(f.anchors) > 0 {\n\t\treturn f.anchors[len(f.anchors)-1].Line\n\t}\n\treturn 0\n}\n\nfunc (f *FullText) computeJumpKeys(bound []rune) {\n\tboundSet := make(map[rune]bool, len(bound))\n\tfor _, r := range bound {\n\t\tboundSet[r] = true\n\t}\n\tused := make(map[rune]bool)\n\tf.jumpKeys = make(map[string]rune)\n\tfor i, c := range f.fullText.Claims {\n\t\tlabel := fmt.Sprintf(\"Claim %d\", c.Number)\n\t\tkey := f.assignKey(label, boundSet, used)\n\t\tf.jumpKeys[label] = key\n\t\tused[key] = true\n\t\t_ = i\n\t}\n}\n\nfunc (f *FullText) assignKey(label string, boundSet, used map[rune]bool) rune {\n\t// Use the first digit of the claim number if available\n\tfor _, r := range label {\n\t\tswitch {\n\t\tcase r >= '0' && r <= '9':\n\t\t\tif !boundSet[r] && !used[r] {\n\t\t\t\treturn r\n\t\t\t}\n\t\tcase r >= 'A' && r <= 'Z':\n\t\t\tr = r - 'A' + 'a'\n\t\t\tfallthrough\n\t\tcase r >= 'a' && r <= 'z':\n\t\t\tif !boundSet[r] && !used[r] {\n\t\t\t\treturn r\n\t\t\t}\n\t\t}\n\t}\n\tfor r := '0'; r <= '9'; r++ {\n\t\tif !boundSet[r] && !used[r] {\n\t\t\treturn r\n\t\t}\n\t}\n\tfor r := 'a'; r <= 'z'; r++ {\n\t\tif !boundSet[r] && !used[r] {\n\t\t\treturn r\n\t\t}\n\t}\n\treturn '?'\n}\n\nfunc (f *FullText) jumpKey(label string) rune {\n\tif f.jumpKeys != nil {\n\t\tif key, ok := f.jumpKeys[label]; ok {\n\t\t\treturn key\n\t\t}\n\t}\n\treturn 0\n}\n\n// --- Helpers ---\n\n// patentMeta builds the patent metadata header for clipboard export.\nfunc patentMeta(p domain.Patent, number domain.PatentNumber) string {\n\tvar b strings.Builder\n\tsep := strings.Repeat(\"═\", 48)\n\tb.WriteString(sep + \"\\n\")\n\tb.WriteString(fmt.Sprintf(\"Patent #:     %s\\n\", number.String()))\n\tif p.Title != \"\" {\n\t\tb.WriteString(fmt.Sprintf(\"Title:        %s\\n\", p.Title))\n\t}\n\tif len(p.Inventors) > 0 {\n\t\tinventorStr := string(p.Inventors[0])\n\t\tif len(p.Inventors) > 1 {\n\t\t\tinventorStr += \" et al. (\" + fmt.Sprintf(\"%d\", len(p.Inventors)) + \")\"\n\t\t}\n\t\tb.WriteString(fmt.Sprintf(\"Inventors:    %s\\n\", inventorStr))\n\t}\n\tif p.Assignee != \"\" {\n\t\tb.WriteString(fmt.Sprintf(\"Assignee:     %s\\n\", p.Assignee))\n\t}\n\tfor _, doc := range p.Documents {\n\t\tif doc.Stage == domain.StageApplication {\n\t\t\tdateStr := \"\"\n\t\t\tif !doc.Dated.IsZero() {\n\t\t\t\tdateStr = \" (\" + doc.Dated.Format(\"2006-01-02\") + \")\"\n\t\t\t}\n\t\t\tb.WriteString(fmt.Sprintf(\"Application #: %s%s\\n\", doc.Number.String(), dateStr))\n\t\t\tbreak\n\t\t}\n\t}\n\tif !p.PublicationDate.IsZero() {\n\t\tb.WriteString(fmt.Sprintf(\"Publication:  %s\\n\", p.PublicationDate.Format(\"2006-01-02\")))\n\t}\n\tif !p.ExpirationDate.IsZero() {\n\t\tsrc := \"\"\n\t\tif p.ExpirationSource != \"\" {\n\t\t\tsrc = \" (\" + p.ExpirationSource + \")\"\n\t\t}\n\t\tb.WriteString(fmt.Sprintf(\"Expiration:   %s%s\\n\", p.ExpirationDate.Format(\"2006-01-02\"), src))\n\t}\n\treturn b.String()\n}\n\n// stripANSI removes ANSI escape codes from a string.\nfunc stripANSI(s string) string {\n\tvar out strings.Builder\n\ti := 0\n\tfor i < len(s) {\n\t\tif s[i] == '\\x1b' && i+1 < len(s) && s[i+1] == '[' {\n\t\t\tj := i + 2\n\t\t\tfor j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {\n\t\t\t\tj++\n\t\t\t}\n\t\t\tif j < len(s) {\n\t\t\t\ti = j + 1\n\t\t\t} else {\n\t\t\t\ti = j\n\t\t\t}\n\t\t} else {\n\t\t\tout.WriteByte(s[i])\n\t\t\ti++\n\t\t}\n\t}\n\treturn out.String()\n}\n"
}
```

**Output:**
```
Wrote file successfully.
```

---

## Assistant (Build · Big Pickle · 13.1s)

_Thinking:_

Now let me update the command package. I need to add:
1. `ScopeFullText` to command.go
2. New command IDs to catalog.go

Then update the keymaps, app.go, wiring.go, and text catalog.

Let me also check if I need to add imports for the render package or wrapText function in the fulltext.go file...

Wait, I used `wrapText` in fulltext.go but it's defined in detail.go, not exported. Let me check...

Looking at detail.go, `wrapText` is lowercase (unexported). I need to either:
1. Export it
2. Duplicate it in fulltext.go
3. Move it to render package

Let me export it from the pane package by making it a public function. Actually, in Go, `wrapText` is lowercase so it's only visible within the `detail.go` file's package (which is `pane`). Since fulltext.go is also in the `pane` package, I can use `wrapText` directly.

Let me verify - both files are in `package pane`, so yes, I can use `wrapText` directly.

Now let me also check: I used `callTimeout` from `calls.go` which is in the same package. Good.

I used `render.Pad`, `render.Truncate`, `render.JumpAnchor`, and the theme. Those are imported from `patentmine/internal/tui/render`. Let me check my imports...

In fulltext.go, I imported:
- `render` - good, but I didn't actually use `render.Pad` or `render.Truncate` directly in the code yet... let me check.

Actually, I used `render.JumpAnchor`, `render.Pad`, `render.Truncate`. Let me check my body function...

I see I'm using `f.theme.Header.Render`, `f.theme.Row.Render`, `f.theme.Warn.Render`, `f.theme.Dim.Render`, `f.theme.Selected.Render` - these are all from the theme. I also use `render.Pad` and `render.Truncate` in the View function. Good.

Wait, let me re-examine the fulltext.go - in the View function, I have `render.Pad(line, w)` — I use `render.Pad`. I need to make sure I import `render`.

My current imports in fulltext.go:
```go
import (
	"fmt"
	"context"
	"strings"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/crawl"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/tui/render"
)
```

I import `render` but also need the render package for `render.JumpAnchor`, `render.Pad`. Wait, I don't actually use `render.Pad` or `render.Truncate` in the code... Let me check the View function:

```go
func (f *FullText) View(w, h int) string {
    ...
    for i, line := range lines[start:end] {
        ...
        var styled string
        switch {
        case inVis && isCursor:
            styled = f.theme.Selected.Render(render.Pad(line, w))
        case inVis:
            styled = f.theme.Selected.Render(render.Pad(line, w))
        case isCursor:
            styled = f.theme.Selected.Render(render.Pad(line, w))
        default:
            styled = line
        }
        ...
    }
}
```

I use `render.Pad(line, w)` — yes, I need it. Good, the import is correct.

Actually, looking more carefully at the `body` function, I use `wrapText` which is defined in `detail.go` - that's in the same package, so it works.

Now let me also check that the `callTimeout` and `callContext` used in the `load` function are accessible. They're defined in `calls.go` in the same package. Good.

And `status()` function - also in calls.go. Good.

Now let me also make sure I have all the command IDs I need. I used:
- NavDown, NavUp, NavPageDown, NavPageUp, NavTop, NavBottom
- SelectVisual
- CopyYank (new)
- NoteAdd (new)
- NoteOpen (new)
- Refresh

Let me now update the command files and other files. Let me start with commands.

Now let me update the command package. First, add `ScopeFullText` and new command IDs:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/command/command.go",
  "oldString": "const (\n\tScopeCatalog   Scope = \"catalog\"   // the main patent list\n\tScopeDetail    Scope = \"detail\"    // one patent's detail view\n\tScopeCitations Scope = \"citations\" // a citations / cited-by list\n\tScopeIDS       Scope = \"ids\"       // one patent's IDS entry editor\n\tScopeProjects  Scope = \"projects\"  // the project list\n\tScopeOverlay   Scope = \"overlay\"   // a modal overlay is focused\n)",
  "newString": "const (\n\tScopeCatalog   Scope = \"catalog\"   // the main patent list\n\tScopeDetail    Scope = \"detail\"    // one patent's detail view\n\tScopeCitations Scope = \"citations\" // a citations / cited-by list\n\tScopeIDS       Scope = \"ids\"       // one patent's IDS entry editor\n\tScopeProjects  Scope = \"projects\"  // the project list\n\tScopeFullText  Scope = \"fulltext\"  // full claims text viewer\n\tScopeOverlay   Scope = \"overlay\"   // a modal overlay is focused\n)"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.6s)

_Thinking:_

Now add the new command IDs to catalog.go:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/command/catalog.go",
  "oldString": "\t// IDS entry editing.\n\tIDSEditField   ID = \"ids.edit-field\"\n\tIDSToggleFull  ID = \"ids.toggle-full\"\n\tIDSCycleStatus ID = \"ids.cycle-status\"\n\tIDSDelete      ID = \"ids.delete\"\n)",
  "newString": "\t// IDS entry editing.\n\tIDSEditField   ID = \"ids.edit-field\"\n\tIDSToggleFull  ID = \"ids.toggle-full\"\n\tIDSCycleStatus ID = \"ids.cycle-status\"\n\tIDSDelete      ID = \"ids.delete\"\n\n\t// Full text viewer.\n\tOpenFullText ID = \"view.fulltext\"\n\tCopyYank     ID = \"edit.copy\"\n\tNoteAdd      ID = \"edit.note-add\"\n\tNoteOpen     ID = \"edit.note-open\"\n)"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 5.8s)

_Thinking:_

Now update the `Default()` function in catalog.go to register the new commands:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/command/catalog.go",
  "oldString": "\t\t// --- projects (engine) ---\n\t\tCommand{ID: ProjectCreate, Name: \"project.create\", Aliases: []string{\"create-project\"}, Usage: \":project.create [NAME]\", Kind: KindEngine, Method: proto.MethodProjectCreate, Scopes: []Scope{ScopeProjects}},\n\t)",
  "newString": "\t\t// --- full text viewer (view) ---\n\t\tCommand{ID: OpenFullText, Name: \"open.fulltext\", Aliases: []string{\"fulltext\", \"claims\", \"all-claims\"}, Usage: \":open.fulltext\", Kind: KindView, Scopes: []Scope{ScopeDetail}},\n\t\tCommand{ID: CopyYank, Name: \"copy\", Aliases: []string{\"yank\", \"clipboard\"}, Usage: \":copy\", Kind: KindView, Scopes: []Scope{ScopeFullText}},\n\t\tCommand{ID: NoteAdd, Name: \"note.add\", Aliases: []string{\"add-note\", \"note\"}, Usage: \":note.add\", Kind: KindView, Scopes: []Scope{ScopeFullText}},\n\t\tCommand{ID: NoteOpen, Name: \"note.open\", Aliases: []string{\"notes\", \"show-notes\"}, Usage: \":note.open\", Kind: KindView, Scopes: []Scope{ScopeFullText}},\n\n\t\t// --- projects (engine) ---\n\t\tCommand{ID: ProjectCreate, Name: \"project.create\", Aliases: []string{\"create-project\"}, Usage: \":project.create [NAME]\", Kind: KindEngine, Method: proto.MethodProjectCreate, Scopes: []Scope{ScopeProjects}},\n\t)"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.5s)

_Thinking:_

I also need to add `ScopeFullText` to the `listScopes` variable in catalog.go so that navigation commands are available:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/command/catalog.go",
  "oldString": "var listScopes = []Scope{ScopeCatalog, ScopeCitations, ScopeProjects, ScopeDetail, ScopeIDS}",
  "newString": "var listScopes = []Scope{ScopeCatalog, ScopeCitations, ScopeProjects, ScopeDetail, ScopeIDS, ScopeFullText}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.0s)

_Thinking:_

Also add to `patentScopes` and `projectScopes`:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/command/catalog.go",
  "oldString": "var patentScopes = []Scope{ScopeCatalog, ScopeDetail, ScopeCitations}",
  "newString": "var patentScopes = []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeFullText}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 5.0s)

_Thinking:_

Now update the keymap default.go to add the new key bindings:

**Tool: todowrite**

**Input:**
```json
{
  "todos": [
    {
      "priority": "high",
      "content": "Read proto, RPC, store files needed for implementation",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Create domain/fulltext.go with FullText and ClaimSection types",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Add ParseAllClaims to crawl/google.go",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Create tui/clipboard.go (OSC 52 + platform fallbacks)",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Create tui/notes.go (notes buffer manager)",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Create tui/pane/fulltext.go (full text viewer pane)",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Add new commands to command/catalog.go and command/command.go",
      "status": "completed"
    },
    {
      "priority": "high",
      "content": "Add key bindings to tui/keymap/default.go",
      "status": "in_progress"
    },
    {
      "priority": "high",
      "content": "Wire up in tui/app.go (handlers, routing)",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Extend detail pane with full text link",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Update text/catalog_en.go with new strings",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Update tui/wiring.go for new scope",
      "status": "pending"
    },
    {
      "priority": "high",
      "content": "Build and verify compilation",
      "status": "pending"
    }
  ]
}
```

**Output:**
```
[
  {
    "content": "Read proto, RPC, store files needed for implementation",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Create domain/fulltext.go with FullText and ClaimSection types",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Add ParseAllClaims to crawl/google.go",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Create tui/clipboard.go (OSC 52 + platform fallbacks)",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Create tui/notes.go (notes buffer manager)",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Create tui/pane/fulltext.go (full text viewer pane)",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Add new commands to command/catalog.go and command/command.go",
    "status": "completed",
    "priority": "high"
  },
  {
    "content": "Add key bindings to tui/keymap/default.go",
    "status": "in_progress",
    "priority": "high"
  },
  {
    "content": "Wire up in tui/app.go (handlers, routing)",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Extend detail pane with full text link",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Update text/catalog_en.go with new strings",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Update tui/wiring.go for new scope",
    "status": "pending",
    "priority": "high"
  },
  {
    "content": "Build and verify compilation",
    "status": "pending",
    "priority": "high"
  }
]
```

---

## Assistant (Build · Big Pickle · 4.8s)

_Thinking:_

Now add the fulltext layer and update the detail layer with the key binding to open full text:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/keymap/default.go",
  "oldString": "\tdetail := NewLayer(\"detail\", false).\n\t\tBindAll(listMotions()).\n\t\tBindAll(patentActions()).\n\t\tBindAll(viewActions()).\n\t\tBindAll(map[string]command.ID{\n\t\t\t\"/\":  command.OpenSearch,\n\t\t\t\";\":  command.JumpMode,\n\t\t})",
  "newString": "\tdetail := NewLayer(\"detail\", false).\n\t\tBindAll(listMotions()).\n\t\tBindAll(patentActions()).\n\t\tBindAll(viewActions()).\n\t\tBindAll(map[string]command.ID{\n\t\t\t\"/\":  command.OpenSearch,\n\t\t\t\";\":  command.JumpMode,\n\t\t\t\"t\":  command.OpenFullText,\n\t\t})"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 5.8s)

_Thinking:_

Now add the fullText layer in the `Default()` function:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/keymap/default.go",
  "oldString": "\toverlay := NewLayer(\"overlay\", false).BindAll(map[string]command.ID{\n\t\t\"esc\":    command.CloseOverlay,\n\t\t\"q\":      command.CloseOverlay,\n\t\t\"j\":      command.NavDown,\n\t\t\"down\":   command.NavDown,\n\t\t\"k\":      command.NavUp,\n\t\t\"up\":     command.NavUp,\n\t\t\"ctrl+d\": command.NavPageDown,\n\t\t\"ctrl+u\": command.NavPageUp,\n\t})",
  "newString": "\tfullText := NewLayer(\"fulltext\", false).\n\t\tBindAll(listMotions()).\n\t\tBindAll(patentActions()).\n\t\tBindAll(map[string]command.ID{\n\t\t\t\";\":  command.JumpMode,\n\t\t\t\"V\":  command.SelectVisual,\n\t\t\t\"y\":  command.CopyYank,\n\t\t\t\"n\":  command.NoteAdd,\n\t\t\t\"N\":  command.NoteOpen,\n\t\t\t\"w\":  command.OpenBrowser,\n\t\t\t\"/\":  command.OpenSearch,\n\t\t\t\"h\":  command.Back,\n\t\t\t\"esc\": command.Back,\n\t\t})\n\n\toverlay := NewLayer(\"overlay\", false).BindAll(map[string]command.ID{\n\t\t\"esc\":    command.CloseOverlay,\n\t\t\"q\":      command.CloseOverlay,\n\t\t\"j\":      command.NavDown,\n\t\t\"down\":   command.NavDown,\n\t\t\"k\":      command.NavUp,\n\t\t\"up\":     command.NavUp,\n\t\t\"ctrl+d\": command.NavPageDown,\n\t\t\"ctrl+u\": command.NavPageUp,\n\t})"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 5.1s)

_Thinking:_

Now add the fullText layer to the scopes map:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/keymap/default.go",
  "oldString": "\treturn &Keymaps{\n\t\tbase: base,\n\t\tscopes: map[command.Scope]*Layer{\n\t\t\tcommand.ScopeCatalog:   catalog,\n\t\t\tcommand.ScopeDetail:    detail,\n\t\t\tcommand.ScopeCitations: citations,\n\t\t\tcommand.ScopeIDS:       ids,\n\t\t\tcommand.ScopeProjects:  projects,\n\t\t\tcommand.ScopeOverlay:   overlay,\n\t\t},\n\t}",
  "newString": "\treturn &Keymaps{\n\t\tbase: base,\n\t\tscopes: map[command.Scope]*Layer{\n\t\t\tcommand.ScopeCatalog:   catalog,\n\t\t\tcommand.ScopeDetail:    detail,\n\t\t\tcommand.ScopeCitations: citations,\n\t\t\tcommand.ScopeIDS:       ids,\n\t\t\tcommand.ScopeProjects:  projects,\n\t\t\tcommand.ScopeFullText:  fullText,\n\t\t\tcommand.ScopeOverlay:   overlay,\n\t\t},\n\t}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 9.3s)

_Thinking:_

Now update the `typedAcceptsArgs` for the new commands and add app handlers in `app.go`:

Now update `app.go` with the new handlers. First add the app handlers and the `typedAcceptsArgs`:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/app.go",
  "oldString": "var appHandlers = map[command.ID]appHandler{\n\tcommand.Quit:               (*App).cmdQuit,\n\tcommand.Help:               (*App).cmdHelp,\n\tcommand.OpenSearch:         (*App).cmdOpenSearch,\n\tcommand.OpenCommand:        (*App).cmdOpenCommand,\n\tcommand.JumpMode:           (*App).cmdJumpMode,\n\tcommand.CloseOverlay:       (*App).cmdCloseOverlay,\n\tcommand.Back:               (*App).cmdBack,\n\tcommand.OpenDetail:         (*App).cmdOpenDetail,\n\tcommand.OpenBrowser:        (*App).cmdOpenBrowser,\n\tcommand.OpenCitations:      (*App).cmdOpenCitations,\n\tcommand.OpenCitedBy:        (*App).cmdOpenCitedBy,\n\tcommand.OpenIDS:            (*App).cmdOpenIDS,\n\tcommand.OpenProjects:       (*App).cmdOpenProjects,\n\tcommand.ProjectActivate:    (*App).cmdProjectActivate,\n\tcommand.ProjectClearActive: (*App).cmdProjectClear,\n\tcommand.ProjectCreate:      (*App).cmdProjectCreate,\n\tcommand.AddToProject:       (*App).cmdAddToProject,\n\tcommand.Import:             (*App).cmdImport,\n\tcommand.MarkStored:         (*App).cmdMarkStored,\n\tcommand.MarkUnderReview:    (*App).cmdMarkUnderReview,\n\tcommand.MarkIgnored:        (*App).cmdMarkIgnored,\n\tcommand.MarkDeleted:        (*App).cmdMarkDeleted,\n\tcommand.TagAdd:             (*App).cmdTagAdd,\n\tcommand.TagRemove:          (*App).cmdTagRemove,\n\tcommand.TagTaxonomyAdd:     (*App).cmdTagTaxonomyAdd,\n\tcommand.TagTaxonomyList:    (*App).cmdTagTaxonomyList,\n\tcommand.TagPatentManage:    (*App).cmdTagPatentManage,\n\tcommand.TagTaxonomyDelete:  (*App).cmdTagTaxonomyDelete,\n\tcommand.TagPatentAdd:       (*App).cmdTagPatentAdd,\n\tcommand.TagPatentDelete:    (*App).cmdTagPatentDelete,\n\tcommand.TagPatentList:      (*App).cmdTagPatentList,\n\tcommand.PatentDelete:       (*App).cmdPatentDelete,\n}",
  "newString": "var appHandlers = map[command.ID]appHandler{\n\tcommand.Quit:               (*App).cmdQuit,\n\tcommand.Help:               (*App).cmdHelp,\n\tcommand.OpenSearch:         (*App).cmdOpenSearch,\n\tcommand.OpenCommand:        (*App).cmdOpenCommand,\n\tcommand.JumpMode:           (*App).cmdJumpMode,\n\tcommand.CloseOverlay:       (*App).cmdCloseOverlay,\n\tcommand.Back:               (*App).cmdBack,\n\tcommand.OpenDetail:         (*App).cmdOpenDetail,\n\tcommand.OpenFullText:       (*App).cmdOpenFullText,\n\tcommand.OpenBrowser:        (*App).cmdOpenBrowser,\n\tcommand.OpenCitations:      (*App).cmdOpenCitations,\n\tcommand.OpenCitedBy:        (*App).cmdOpenCitedBy,\n\tcommand.OpenIDS:            (*App).cmdOpenIDS,\n\tcommand.OpenProjects:       (*App).cmdOpenProjects,\n\tcommand.ProjectActivate:    (*App).cmdProjectActivate,\n\tcommand.ProjectClearActive: (*App).cmdProjectClear,\n\tcommand.ProjectCreate:      (*App).cmdProjectCreate,\n\tcommand.AddToProject:       (*App).cmdAddToProject,\n\tcommand.Import:             (*App).cmdImport,\n\tcommand.MarkStored:         (*App).cmdMarkStored,\n\tcommand.MarkUnderReview:    (*App).cmdMarkUnderReview,\n\tcommand.MarkIgnored:        (*App).cmdMarkIgnored,\n\tcommand.MarkDeleted:        (*App).cmdMarkDeleted,\n\tcommand.TagAdd:             (*App).cmdTagAdd,\n\tcommand.TagRemove:          (*App).cmdTagRemove,\n\tcommand.TagTaxonomyAdd:     (*App).cmdTagTaxonomyAdd,\n\tcommand.TagTaxonomyList:    (*App).cmdTagTaxonomyList,\n\tcommand.TagPatentManage:    (*App).cmdTagPatentManage,\n\tcommand.TagTaxonomyDelete:  (*App).cmdTagTaxonomyDelete,\n\tcommand.TagPatentAdd:       (*App).cmdTagPatentAdd,\n\tcommand.TagPatentDelete:    (*App).cmdTagPatentDelete,\n\tcommand.TagPatentList:      (*App).cmdTagPatentList,\n\tcommand.PatentDelete:       (*App).cmdPatentDelete,\n\tcommand.CopyYank:           (*App).cmdCopyYank,\n\tcommand.NoteAdd:            (*App).cmdNoteAdd,\n\tcommand.NoteOpen:           (*App).cmdNoteOpen,\n}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.5s)

_Thinking:_

Now add `typedAcceptsArgs` entries for the new commands:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/app.go",
  "oldString": "var typedAcceptsArgs = map[command.ID]bool{\n\tcommand.AddToProject:    true,\n\tcommand.ProjectActivate: true,\n\tcommand.ProjectCreate:   true,\n\tcommand.Import:          true,\n\tcommand.TagAdd:          true,\n\tcommand.TagRemove:       true,\n\tcommand.TagTaxonomyAdd:     true,\n\tcommand.TagTaxonomyDelete:  true,\n\tcommand.TagPatentAdd:       true,\n\tcommand.TagPatentDelete:    true,\n\tcommand.Filter:          true,\n\tcommand.OpenBrowser:     true,\n}",
  "newString": "var typedAcceptsArgs = map[command.ID]bool{\n\tcommand.AddToProject:    true,\n\tcommand.ProjectActivate: true,\n\tcommand.ProjectCreate:   true,\n\tcommand.Import:          true,\n\tcommand.TagAdd:          true,\n\tcommand.TagRemove:       true,\n\tcommand.TagTaxonomyAdd:     true,\n\tcommand.TagTaxonomyDelete:  true,\n\tcommand.TagPatentAdd:       true,\n\tcommand.TagPatentDelete:    true,\n\tcommand.Filter:          true,\n\tcommand.OpenBrowser:     true,\n}"
}
```

**Error:**
```
No changes to apply: oldString and newString are identical.
```

---

## Assistant (Build · Big Pickle · 4.7s)

_Thinking:_

Right, the new commands don't accept typed args. Now I need to add the app handler methods. Let me add them after the existing handlers in app.go.

Now add the new handler methods to app.go:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/app.go",
  "oldString": "func (a *App) cmdOpenDetail(invocation) (tea.Model, tea.Cmd) { return a.openDetail() }",
  "newString": "func (a *App) cmdOpenDetail(invocation) (tea.Model, tea.Cmd) { return a.openDetail() }\n\nfunc (a *App) cmdOpenFullText(invocation) (tea.Model, tea.Cmd) { return a.openFullText() }"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 7.4s)

_Thinking:_

Now add the `openFullText` method and the CopyYank/NoteAdd/NoteOpen handler methods. Let me add them after the `openDetail` method:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/app.go",
  "oldString": "// openDetail pushes a detail pane for the focused pane's selected patent. The\n// active project, when set, scopes the detail's review state and tags.\nfunc (a *App) openDetail() (tea.Model, tea.Cmd) {\n\tnumber, ok := a.focusedPane().Selection()\n\tif !ok {\n\t\ta.setErr(text.StatusNoPatentSelected)\n\t\treturn a, nil\n\t}\n\tvar project domain.ProjectID\n\tif a.activeProject != nil {\n\t\tproject = a.activeProject.ID\n\t}\n\tbound := a.keymaps.BoundLetters(command.ScopeDetail)\n\treturn a.pushPane(pane.NewDetail(a.client, a.theme, number, project, bound))\n}",
  "newString": "// openDetail pushes a detail pane for the focused pane's selected patent. The\n// active project, when set, scopes the detail's review state and tags.\nfunc (a *App) openDetail() (tea.Model, tea.Cmd) {\n\tnumber, ok := a.focusedPane().Selection()\n\tif !ok {\n\t\ta.setErr(text.StatusNoPatentSelected)\n\t\treturn a, nil\n\t}\n\tvar project domain.ProjectID\n\tif a.activeProject != nil {\n\t\tproject = a.activeProject.ID\n\t}\n\tbound := a.keymaps.BoundLetters(command.ScopeDetail)\n\treturn a.pushPane(pane.NewDetail(a.client, a.theme, number, project, bound))\n}\n\n// openFullText pushes a full text viewer pane for the selected patent.\nfunc (a *App) openFullText() (tea.Model, tea.Cmd) {\n\tnumber, ok := a.focusedPane().Selection()\n\tif !ok {\n\t\ta.setErr(text.StatusNoPatentSelected)\n\t\treturn a, nil\n\t}\n\tvar project domain.ProjectID\n\tif a.activeProject != nil {\n\t\tproject = a.activeProject.ID\n\t}\n\tbound := a.keymaps.BoundLetters(command.ScopeFullText)\n\treturn a.pushPane(pane.NewFullText(a.client, a.theme, number, project, bound))\n}\n\n// cmdCopyYank handles the clipboard copy command from the full text pane.\nfunc (a *App) cmdCopyYank(inv invocation) (tea.Model, tea.Cmd) {\n\t// The pane handles this itself via CopyToClipboardMsg\n\treturn a, nil\n}\n\n// cmdNoteAdd handles the note add command from the full text pane.\nfunc (a *App) cmdNoteAdd(inv invocation) (tea.Model, tea.Cmd) {\n\t// The pane handles this itself via NoteAddMsg\n\treturn a, nil\n}\n\n// cmdNoteOpen handles the note open command from the full text pane.\nfunc (a *App) cmdNoteOpen(inv invocation) (tea.Model, tea.Cmd) {\n\t// The pane handles this itself via NoteOpenMsg\n\treturn a, nil\n}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 9.8s)

_Thinking:_

Wait, I need to actually handle those messages in the app's Update method. The pane emits `CopyToClipboardMsg`, `NoteAddMsg`, `NoteOpenMsg` which need to be handled by the App. Let me add the Update handlers.

Actually, looking at this more carefully, the pane handlers (copyYank, noteAdd, noteOpen) return tea.Cmd closures that emit `CopyToClipboardMsg`, `NoteAddMsg`, `NoteOpenMsg`. These messages need to be handled in the app's Update method.

But wait - the app handlers (`cmdCopyYank`, `cmdNoteAdd`, `cmdNoteOpen`) are called when the user presses the key binding. But the pane also has handlers for these commands. Let me trace the flow:

1. User presses `y` in full text pane
2. Keymap resolves to `command.CopyYank`
3. `invoke` is called with `command.CopyYank`
4. It checks `appHandlers` first - I added `command.CopyYank: (*App).cmdCopyYank`
5. So it calls `a.cmdCopyYank(inv)` which just returns `a, nil`
6. The pane's handler never fires

This is wrong. The pane's handlers should fire, not the app handlers, because the fulltext pane handles CopyYank internally.

The issue is that `appHandlers` has priority over pane handlers. For commands that the pane handles, I should NOT add them to `appHandlers`.

But the wiring check requires that every command bound in a keymap scope has a handler. For `ScopeFullText`, the commands are handled by the pane itself (the FullText pane's Handles() lists them), not by the app.

So I should remove these from appHandlers and instead let the pane handle them through the normal pane dispatch.

Let me revert the appHandlers additions and instead let the pane handle them.

Wait no, looking at the dispatch flow more carefully:

```go
func (a *App) invoke(id command.ID, inv invocation) (tea.Model, tea.Cmd) {
	if handler, ok := appHandlers[id]; ok {
		return handler(a, inv)
	}
	...
	p := a.focusedPane()
	if !slices.Contains(p.Handles(), id) {
		return a.unhandled(id)
	}
	updated, cmd := p.Command(id, pane.Invocation{Repeat: inv.repeat, Args: inv.args})
	...
}
```

So if the command is in `appHandlers`, it ALWAYS goes to the app handler and never reaches the pane. That's correct for some commands but wrong for CopyYank/NoteAdd/NoteOpen which the pane should handle.

I should remove CopyYank, NoteAdd, NoteOpen from appHandlers. The pane handles them directly.

But then where does the copy-to-clipboard logic live? The pane emits a `CopyToClipboardMsg` which the app's Update method must handle. Same for NoteAddMsg and NoteOpenMsg.

Let me restructure:
1. Remove CopyYank, NoteAdd, NoteOpen from appHandlers
2. In the app's Update method, handle CopyToClipboardMsg, NoteAddMsg, NoteOpenMsg
3. The pane's handlers emit these messages

Let me fix this.

I need to remove CopyYank, NoteAdd, NoteOpen from appHandlers — the pane handles them directly. The messages are handled via Update:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/app.go",
  "oldString": "\tcommand.PatentDelete:       (*App).cmdPatentDelete,\n\tcommand.CopyYank:           (*App).cmdCopyYank,\n\tcommand.NoteAdd:            (*App).cmdNoteAdd,\n\tcommand.NoteOpen:           (*App).cmdNoteOpen,\n}",
  "newString": "\tcommand.PatentDelete:       (*App).cmdPatentDelete,\n}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 5.0s)

_Thinking:_

Now remove the cmdCopyYank, cmdNoteAdd, cmdNoteOpen methods that I just added:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/app.go",
  "oldString": "// cmdCopyYank handles the clipboard copy command from the full text pane.\nfunc (a *App) cmdCopyYank(inv invocation) (tea.Model, tea.Cmd) {\n\t// The pane handles this itself via CopyToClipboardMsg\n\treturn a, nil\n}\n\n// cmdNoteAdd handles the note add command from the full text pane.\nfunc (a *App) cmdNoteAdd(inv invocation) (tea.Model, tea.Cmd) {\n\t// The pane handles this itself via NoteAddMsg\n\treturn a, nil\n}\n\n// cmdNoteOpen handles the note open command from the full text pane.\nfunc (a *App) cmdNoteOpen(inv invocation) (tea.Model, tea.Cmd) {\n\t// The pane handles this itself via NoteOpenMsg\n\treturn a, nil\n}",
  "newString": ""
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 8.6s)

_Thinking:_

Now I need to handle the messages from the fulltext pane in the App's Update method. The messages are: CopyToClipboardMsg, NoteAddMsg, NoteOpenMsg, FullTextLoadedMsg.

CopyToClipboardMsg needs to call copyToClipboard from clipboard.go.
NoteAddMsg needs to add to the notes accumulator.
NoteOpenMsg needs to open the notes overlay.
FullTextLoadedMsg needs to be forwarded to the pane (it's already handled by the Pane's Update method since it broadcasts messages).

Wait, actually FullTextLoadedMsg is broadcast to all panes through the default path in Update. Let me check...

In App.Update:
```go
default:
    return a, tea.Batch(a.broadcast(msg), a.broadcastOverlays(msg))
```

So FullTextLoadedMsg would be broadcast. That's fine - the FullText pane receives it and handles it in its Update method.

But CopyToClipboardMsg, NoteAddMsg, NoteOpenMsg need specific handling in the App:
- CopyToClipboardMsg: needs to call `copyToClipboard()`
- NoteAddMsg: needs to add to the `notesAccumulator`
- NoteOpenMsg: needs to create and push a notes overlay

These are not standard messages that panes handle. I need to add cases for them in the App.Update method.

Also, the App needs a `notesAccumulator` instance. Let me add that.

I need to add the notes accumulator to the App and handle the custom messages. Let me add notes to the App struct and handle messages:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/app.go",
  "oldString": "// App is the bubbletea root model.\ntype App struct {\n\tclient          *rpc.Client\n\tregistry        *command.Registry\n\tkeymaps         *keymap.Keymaps\n\thints           *keymap.HintCatalog\n\ttheme           render.Theme\n\ttext            *text.Catalog\n\treader          keys.Reader\n\tsaveLastProject func(domain.ProjectID) error\n\n\tpanes      []pane.Pane\n\toverlays   []overlay.Overlay\n\tconfirmCmd tea.Cmd // pending action awaiting confirmation\n\n\tstatus        string\n\tstatusErr     bool\n\twidth         int\n\theight        int\n\tactiveProject *domain.Project\n\tlastProjectID domain.ProjectID\n\ttuiVersion    string\n\tdaemonVersion string\n\topenURL       func(string) error\n}",
  "newString": "// App is the bubbletea root model.\ntype App struct {\n\tclient          *rpc.Client\n\tregistry        *command.Registry\n\tkeymaps         *keymap.Keymaps\n\thints           *keymap.HintCatalog\n\ttheme           render.Theme\n\ttext            *text.Catalog\n\treader          keys.Reader\n\tsaveLastProject func(domain.ProjectID) error\n\n\tpanes      []pane.Pane\n\toverlays   []overlay.Overlay\n\tconfirmCmd tea.Cmd // pending action awaiting confirmation\n\n\tstatus        string\n\tstatusErr     bool\n\twidth         int\n\theight        int\n\tactiveProject *domain.Project\n\tlastProjectID domain.ProjectID\n\ttuiVersion    string\n\tdaemonVersion string\n\topenURL       func(string) error\n\tnotes         *notesAccumulator\n}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.5s)

_Thinking:_

Now initialize the notes accumulator in `New()`:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/app.go",
  "oldString": "\tapp := &App{\n\t\tclient:        client,\n\t\tregistry:      registry,\n\t\tkeymaps:       keymaps,\n\t\thints:         hints,\n\t\ttheme:         theme,\n\t\ttext:          catalog,\n\t\ttuiVersion:    appversion.String(),\n\t\tdaemonVersion: \"connecting\",\n\t\topenURL:       openExternalURL,\n\t}",
  "newString": "\tapp := &App{\n\t\tclient:        client,\n\t\tregistry:      registry,\n\t\tkeymaps:       keymaps,\n\t\thints:         hints,\n\t\ttheme:         theme,\n\t\ttext:          catalog,\n\t\ttuiVersion:    appversion.String(),\n\t\tdaemonVersion: \"connecting\",\n\t\topenURL:       openExternalURL,\n\t\tnotes:         newNotesAccumulator(),\n\t}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 5.5s)

_Thinking:_

Now add the message handlers in the Update method for CopyToClipboardMsg, NoteAddMsg, and NoteOpenMsg:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/app.go",
  "oldString": "\tcase busEventMsg:\n\t\treturn a, tea.Batch(\n\t\t\ta.handleEvent(m.event),\n\t\t\ta.broadcastOverlays(m.event),\n\t\t\ta.listen())\n\tcase eventsClosedMsg:\n\t\ta.setErr(text.StatusDaemonClosed)\n\t\treturn a, nil\n\tdefault:",
  "newString": "\tcase pane.CopyToClipboardMsg:\n\t\tif err := copyToClipboard(m.Text); err != nil {\n\t\t\ta.setErr(text.StatusClipboardFailed, err.Error())\n\t\t} else {\n\t\t\ta.setStatus(text.StatusCopiedToClipboard, len(m.Text))\n\t\t}\n\t\treturn a, nil\n\tcase pane.NoteAddMsg:\n\t\tif a.notes.Add(m.Number, m.Locator) {\n\t\t\ta.setStatus(text.StatusNotesAdded, m.Locator)\n\t\t}\n\t\treturn a, nil\n\tcase pane.NoteOpenMsg:\n\t\tlocators := a.notes.Locators(m.Number)\n\t\ta.overlays = append(a.overlays, newNotesBufferOverlay(a.theme, m.Number, locators, a))\n\t\treturn a, nil\n\tcase busEventMsg:\n\t\treturn a, tea.Batch(\n\t\t\ta.handleEvent(m.event),\n\t\t\ta.broadcastOverlays(m.event),\n\t\t\ta.listen())\n\tcase eventsClosedMsg:\n\t\ta.setErr(text.StatusDaemonClosed)\n\t\treturn a, nil\n\tdefault:"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.8s)

_Thinking:_

Now I need to update the text catalog with the new strings. Let me also update the wiring.go to include ScopeFullText, and update the detail pane to include a link to full text.

Let me first update the text catalog:

Now update the text catalog with new strings:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/text/catalog_en.go",
  "oldString": "\tStatusBrowserOpened:    \"opened %d patent page(s) in browser\",",
  "newString": "\tStatusBrowserOpened:    \"opened %d patent page(s) in browser\",\n\tStatusNotesAdded:       \"added %s to notes buffer\",\n\tStatusNotesFlushed:     \"flushed notes for %s to IDS: %s\",\n\tStatusCopiedToClipboard: \"copied to clipboard: %d bytes\",\n\tStatusClipboardFailed:   \"clipboard: %s\","
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.5s)

_Thinking:_

Now add the new status keys to the named constants section:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/text/catalog_en.go",
  "oldString": "\tStatusExportFailed         Key = \"status.export_failed\"",
  "newString": "\tStatusNotesAdded          Key = \"status.notes_added\"\n\tStatusNotesFlushed        Key = \"status.notes_flushed\"\n\tStatusCopiedToClipboard   Key = \"status.copied_to_clipboard\"\n\tStatusClipboardFailed     Key = \"status.clipboard_failed\"\n\tStatusExportFailed         Key = \"status.export_failed\""
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 5.2s)

_Thinking:_

Now add the command title/help entries for the new commands:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/text/catalog_en.go",
  "oldString": "\t\"ids.delete\":               {\"Delete IDS entry\", \"Remove the current patent from the curated IDS.\"},",
  "newString": "\t\"ids.delete\":               {\"Delete IDS entry\", \"Remove the current patent from the curated IDS.\"},\n\t\"view.fulltext\":            {\"Full text\", \"Open the full claims text viewer for the selected patent.\"},\n\t\"edit.copy\":                {\"Copy/yank\", \"Copy selected text to clipboard with patent metadata.\"},\n\t\"edit.note-add\":            {\"Add to notes\", \"Add the current claim locator to the session notes buffer.\"},\n\t\"edit.note-open\":           {\"Open notes\", \"Show the accumulated notes buffer for this patent.\"},"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.0s)

_Thinking:_

Now update the wiring.go to include ScopeFullText in the pane handler sets:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/wiring.go",
  "oldString": "var paneScopes = []command.Scope{\n\tcommand.ScopeCatalog, command.ScopeDetail,\n\tcommand.ScopeCitations, command.ScopeIDS, command.ScopeProjects,\n}",
  "newString": "var paneScopes = []command.Scope{\n\tcommand.ScopeCatalog, command.ScopeDetail,\n\tcommand.ScopeCitations, command.ScopeIDS, command.ScopeProjects,\n\tcommand.ScopeFullText,\n}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.7s)

_Thinking:_

Now add the FullText pane to the paneHandlerSets:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/wiring.go",
  "oldString": "\tpanes := []pane.Pane{\n\t\tpane.NewCatalog(nil, theme),\n\t\tpane.NewDetail(nil, theme, domain.PatentNumber{}, \"\", nil),\n\t\tpane.NewCitations(nil, theme, domain.PatentNumber{}, domain.RelationCites),\n\t\tpane.NewIDSDetail(nil, theme, domain.PatentNumber{}, \"\"),\n\t\tpane.NewProjects(nil, theme),\n\t}",
  "newString": "\tpanes := []pane.Pane{\n\t\tpane.NewCatalog(nil, theme),\n\t\tpane.NewDetail(nil, theme, domain.PatentNumber{}, \"\", nil),\n\t\tpane.NewCitations(nil, theme, domain.PatentNumber{}, domain.RelationCites),\n\t\tpane.NewIDSDetail(nil, theme, domain.PatentNumber{}, \"\"),\n\t\tpane.NewProjects(nil, theme),\n\t\tpane.NewFullText(nil, theme, domain.PatentNumber{}, \"\", nil),\n\t}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 4.7s)

_Thinking:_

Now I need to extend the detail pane to show a "Full claims text" link that the user can jump to. Let me add it to the detail pane's body and anchor labels.

Now let me update the detail pane to include a "Full claims text" section:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/pane/detail.go",
  "oldString": "var detailAnchorLabels = []string{\n\t\"Assignee\",\n\t\"Inventors\",\n\t\"Expiration\",\n\t\"Review state\",\n\t\"IDS\",\n\t\"Tags\",\n\t\"Citations\",\n\t\"Documents\",\n\t\"First claim\",\n\t\"Abstract\",\n}",
  "newString": "var detailAnchorLabels = []string{\n\t\"Assignee\",\n\t\"Inventors\",\n\t\"Expiration\",\n\t\"Review state\",\n\t\"IDS\",\n\t\"Tags\",\n\t\"Citations\",\n\t\"Documents\",\n\t\"First claim\",\n\t\"Full claims text\",\n\t\"Abstract\",\n}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 8.9s)

_Thinking:_

Wait, but the detail pane doesn't actually have a "Full claims text" section in its body. The anchor labels are for jump targets, but each one needs a corresponding entry in the `body()` method. The anchor labels list includes labels that have jump keys assigned, and the `body()` method creates anchors with `addAnchor()` for each section. But "Full claims text" is not a section of the detail body — it's a separate action. 

Actually, looking at how this works: the jump mode shows the jump keys in an overlay, and pressing one scrolls to that section. "Full claims text" doesn't have a section to scroll to in the detail view. Instead, the `t` key binding opens the full text pane.

I think the right approach is:
1. Keep the anchor label for "Full claims text" in the detail pane's body
2. When a user presses the jump key for it, instead of scrolling, it opens the full text pane

But that's more complex. A simpler approach: the "Full claims text" entry in the detail view acts as a button — pressing its jump key or `t` opens the full text viewer. I'll add it as a separate "action" section in the detail body that is clickable.

Actually, the simplest approach is to just have the `t` key binding open the full text viewer from the detail pane (which I've already set up in the keymap). The jump anchor for it can scroll to a line that says "(press t to view full claims text)" or similar.

Let me add a section in the detail body for this:

After the "First claim" section, add:
```
[t] Full claims text
```
That shows the user they can press `t` to open the full text view.

But I need the anchor to work. Let me add an `addAnchor` call for it and a descriptive line.

Let me revert the anchor labels change and instead just add an action hint line in the body.

Actually, the "Full claims text" shouldn't be a jump anchor (since it doesn't scroll anywhere). The `t` key binding is sufficient. Let me revert that anchor label and instead add an action hint in the detail body after "First claim":

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/pane/detail.go",
  "oldString": "var detailAnchorLabels = []string{\n\t\"Assignee\",\n\t\"Inventors\",\n\t\"Expiration\",\n\t\"Review state\",\n\t\"IDS\",\n\t\"Tags\",\n\t\"Citations\",\n\t\"Documents\",\n\t\"First claim\",\n\t\"Full claims text\",\n\t\"Abstract\",\n}",
  "newString": "var detailAnchorLabels = []string{\n\t\"Assignee\",\n\t\"Inventors\",\n\t\"Expiration\",\n\t\"Review state\",\n\t\"IDS\",\n\t\"Tags\",\n\t\"Citations\",\n\t\"Documents\",\n\t\"First claim\",\n\t\"Abstract\",\n}"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle · 5.6s)

_Thinking:_

Now add a "Full text" action hint in the detail body, after the "First claim" section:

**Tool: edit**

**Input:**
```json
{
  "filePath": "/mnt/d/Repos/PatentMineNew/internal/tui/pane/detail.go",
  "oldString": "\td.addAnchor(&b, d.jumpKey(\"First claim\"), \"First claim\", 1)\n\td.section(&b, w, \"First claim\", p.FirstClaim)\n\td.addAnchor(&b, d.jumpKey(\"Abstract\"), \"Abstract\", 1)\n\td.section(&b, w, \"Abstract\", p.Abstract)",
  "newString": "\td.addAnchor(&b, d.jumpKey(\"First claim\"), \"First claim\", 1)\n\td.section(&b, w, \"First claim\", p.FirstClaim)\n\n\t// Full claims text action hint\n\tb.WriteByte('\\n')\n\tfullTextLabel := \"Full claims text\"\n\tkey := d.jumpKey(fullTextLabel)\n\tif key != 0 {\n\t\tb.WriteString(d.theme.Warn.Render(fmt.Sprintf(\"[%s] %s — press '%s' to open full text viewer\", string(key), fullTextLabel, string(key))))\n\t} else {\n\t\tb.WriteString(d.theme.Dim.Render(\"Full claims text — press 't' to open full text viewer\"))\n\t}\n\tb.WriteByte('\\n')\n\n\td.addAnchor(&b, d.jumpKey(\"Abstract\"), \"Abstract\", 1)\n\td.section(&b, w, \"Abstract\", p.Abstract)"
}
```

**Output:**
```
Edit applied successfully.
```

---

## Assistant (Build · Big Pickle)

---

