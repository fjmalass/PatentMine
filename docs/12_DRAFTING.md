# PatentMine: Drafting Subsystem

This document covers the drafting subsystem: authoring **provisional** and
**non-provisional first applications** and **responses to office actions**, then
rendering them to **.docx**, with optional grounded AI drafting.

Related docs:

- [01_README.md](./01_README.md) — architecture & data model.
- [17_AI.md](./17_AI.md) — multi-provider AI runtime (Gemini / Ollama).
- [03_USPTO_CONFIG_LOADING.md](./03_USPTO_CONFIG_LOADING.md) — API keys & provider modes.

---

## 1. One model for three documents

A provisional application, a first non-provisional application, and an
office-action response are all the same kind of thing: a **project-scoped,
section-structured legal document rendered to .docx**. So they share one model,
`domain.Draft`, distinguished by `Kind`:

| Kind | Meaning | Claims | Default sections |
|---|---|---|---|
| `provisional` | Provisional application (35 U.S.C. 111(b)) | none | Field, Background, Summary, Brief Description of Drawings, Detailed Description, Abstract |
| `nonprovisional` | First utility application | required | same spec sections + a claim listing |
| `oa_response` | Response/amendment to an Office Action | amended | Remarks (+ an "Amendments to the Claims" listing) |

`domain.DefaultSections(kind)` seeds the conventional skeleton at creation, so
the author opens onto a structured outline rather than a blank page — a
deliberate constraint that bounds where free-text (and AI drafting) can go.

A `Draft` is a child of a `project` (the same prosecution matter the IDS flow
uses: application number, art unit, examiner history, attorney docket). That is
the "table like IDS, specific to the project" — drafts hang off the project and
read its header for the document caption.

### Schema (v7)

```
project ──1:N─→ draft ──1:N─→ draft_section
   │              │  └─1:N─→ draft_claim
   │              └─(nullable FK)─→ office_action
   └──1:N─→ office_action   (the examiner document a response answers)
```

- `draft` — `id`, `project_id`, `kind`, `title`, `status` (draft/final/filed),
  `office_action_id` (NULL for a first application), timestamps.
- `draft_section` — ordered prose sections with `pinned_json` (verbatim spans)
  and AI provenance (`ai_provider`, `ai_model`, `generated_at`, `human_edited`).
- `draft_claim` — `number`, `claim_type`, `depends_on`, `status` (MPEP 714
  identifier), `base_text` (prior version), `text` (current).
- `office_action` — the examiner document: bytes on disk (`blob_path`,
  `blob_hash`), `extracted_text` for grounding, plus `mail_date`, `oa_type`,
  `examiner`, `art_unit`, `source` (manual/odp).

All tables are project-scoped with `ON DELETE CASCADE`, so deleting a project
removes its drafts and office actions. The migration `migrateV6ToV7` is purely
additive.

---

## 2. The .docx renderer

`internal/export/docx` is a **dependency-free WordprocessingML writer** built on
the standard library `archive/zip` — a .docx is just a ZIP of XML parts. This
keeps the single-static-binary build (the same reason the IDS exporter fills a
PDF form rather than embedding a renderer).

`docx.RenderDraft(draft, project, officeAction)` produces:

- **Applications** — title page (title, inventors, docket), the specification
  sections, and, for a non-provisional, a claim listing under "What is claimed
  is:".
- **Office-action responses** — the USPTO caption (application no., examiner,
  art unit, docket), an **"Amendments to the Claims"** listing, then the
  **Remarks**.

### Claim-amendment markup is computed, not generated

A currently-amended claim shows additions underlined and deletions struck
through (MPEP 714). The renderer computes this **deterministically in code** via
a word-level diff of `base_text` against `text` (`amendmentRuns`), then emits the
underline/strikethrough runs. The AI never writes claim markup — this guarantees
the legal markup is always correct and removes the highest-risk hallucination
surface (a model silently altering claim scope). Status identifiers
(`(Currently Amended)`, `(Cancelled)`, …) come from `AmendmentStatus.Label()`.

The DB stays the source of truth and you **edit in-app until final**; the .docx
is a one-way handoff/redline artifact (no docx round-trip).

---

## 3. Grounded AI drafting (and how it avoids hallucination)

AI drafting is **opt-in, per section, and never auto-saved** — the author
reviews, edits, and accepts. `engine.DraftSection` grounds the model only in the
draft's own material: title, claims, the linked office action's extracted text,
plus caller-supplied prior-art references and notes.

`ai.BuildDraftPrompt` enforces the anti-hallucination strategy in the prompt:

1. **Use only the supplied source material** — no invented prior art, citations,
   statutes, dates, names, or numbers.
2. **Mark anything unsupported** with the literal token `‹NEEDS ATTORNEY INPUT›`
   rather than guessing.
3. **Reproduce PINNED spans verbatim** — the "select elements to not
   hallucinate" capability: you mark exact claim language or an exact reference
   quote as pinned, and the model must reproduce it character-for-character.
4. Draft prose only; claims are handled separately (see §2).

Then `ai.VerifyDraft` **mechanically verifies** the output: it string-matches
each pinned span against the draft (whitespace-normalized) and counts any
`‹NEEDS ATTORNEY INPUT›` markers, returning a list of problems. An empty list
means the guardrails passed — not that the text is correct, only that the
mechanical checks held. This turns "tell it not to hallucinate" from a prompt
wish into an enforced, auditable invariant.

### Local vs cloud (confidentiality)

A draft is attorney work product and may quote an **unpublished** application, so
the provider choice matters. Both providers satisfy `ai.Drafter`
(`Complete` + `Provider` + `Model`); `ai.NewDrafter` selects from config
(`PATENTMINE_AI_PROVIDER`, `GEMINI_API_KEY`, `OLLAMA_HOST`/`OLLAMA_MODEL`).
Run **local Ollama** for privileged/pre-publication drafting and opt into
**cloud Gemini** for published matters. Install the local path with
`cargo make ollama-setup` (or `cargo make draft-setup`).

> The tool drafts; the practitioner reviews, signs, and is responsible. AI text
> carries provenance (`ai_provider`/`ai_model`) and is never finalized for you.

---

## 4. Using it from the CLI

Drafting runs in the engine daemon, so start it first: `patentmine serve`
(or `cargo make start-daemon`). Create the project in the TUI or web API, then:

```bash
# Draft a provisional first application
patentmine draft new --project p-123 --kind provisional --title "Self-Cleaning Widget"
patentmine draft list --project p-123
patentmine draft export --id d-...            # → writes a .docx, prints the path

# Non-provisional (carries a claim listing)
patentmine draft new --project p-123 --kind nonprovisional --title "Self-Cleaning Widget"

# AI-draft one section, pinning a phrase that must appear verbatim
patentmine draft ai --id d-... --section 1 \
  --instruction "Draft the summary." \
  --pin "a self-cleaning surface coating"

# Office-action response: import the examiner document, then respond
patentmine draft oa-import --project p-123 --file ./office-action.txt \
  --type non_final --examiner "Jane Doe" --art-unit 2151 --mail-date 2026-01-15
patentmine draft new --project p-123 --kind oa-response --oa oa-...
patentmine draft export --id d-...
```

Via cargo-make: `cargo make draft new --project p-123 --kind provisional --title "X"`.

Rendered .docx files land under the configured docs export base
(`PATENTMINE_DOCS_EXPORT_DIR`, default `$HOME/exports`; the legacy
`PATENTMINE_IDS_EXPORT_DIR` is still honored) in `<base>/<project>/`; imported
office-action documents under `<base>/<project>/office-actions/`.

---

## 5. RPC surface

All operations are daemon methods (`internal/proto/proto_drafting.go`,
`internal/rpc/server_drafting.go`), so the web API and any RPC client can drive
them too:

`draft.create`, `draft.get`, `draft.list`, `draft.save`, `draft.delete`,
`draft.export.docx`, `draft.section.ai`, `office_action.import`,
`office_action.list`, `office_action.get`.

---

## 6. Status & follow-ups

Implemented: data model, schema/migration, pure-Go .docx rendering with
deterministic claim markup, grounded AI drafting with verbatim verification,
office-action import (manual), engine + RPC + CLI, Makefile tasks.

Deferred (documented in the plan):

- **TUI overlays** for drafting (today it is RPC/CLI/web-driven).
- **PDF text extraction / OCR** for scanned office actions — currently a `.txt`
  source doubles as extracted text, or text is supplied explicitly; PDF/OCR
  extraction would add an external dependency (kept out of the pure-Go build).
- **ODP auto-fetch** of the office action when the API key is entitled (manual
  import is the always-works baseline).
