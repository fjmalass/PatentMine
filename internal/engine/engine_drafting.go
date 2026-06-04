package engine

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"patentmine/internal/ai"
	"patentmine/internal/domain"
	"patentmine/internal/export/docx"
	"patentmine/internal/observability"
	"patentmine/internal/pdf"
)

// CreateDraft starts a new draft for a project, seeded with the conventional
// empty section skeleton for its kind (so the author opens onto a structured
// outline rather than a blank page). The title is optional for a response.
func (e *Engine) CreateDraft(ctx context.Context, project domain.ProjectID, kind domain.DraftKind, title string) (d domain.Draft, err error) {
	defer e.observeDuration("engine.create_draft", time.Now(), &err)
	if project == "" {
		return domain.Draft{}, errors.New("engine: draft requires a project")
	}
	if !kind.Valid() {
		return domain.Draft{}, fmt.Errorf("engine: invalid draft kind %q", kind)
	}
	if _, err := e.repo.Project(ctx, project); err != nil {
		return domain.Draft{}, fmt.Errorf("engine: create draft: %w", err)
	}
	now := time.Now().UTC()
	d = domain.Draft{
		ID:        domain.DraftID(fmt.Sprintf("d-%d", now.UnixNano())),
		Project:   project,
		Kind:      kind,
		Title:     strings.TrimSpace(title),
		Status:    domain.DraftStatusDraft,
		CreatedAt: now,
		UpdatedAt: now,
		Sections:  domain.DefaultSections(kind),
	}
	if err := e.repo.SaveDraft(ctx, d); err != nil {
		return domain.Draft{}, err
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionDraftCreate,
		Entity:   "draft",
		EntityID: string(d.ID),
		Status:   "committed",
		Attributes: map[string]any{
			"project": string(project),
			"kind":    string(kind),
		},
	})
	e.announceChange()
	return d, nil
}

// Draft returns one draft with its sections and claims.
func (e *Engine) Draft(ctx context.Context, id domain.DraftID) (domain.Draft, error) {
	return e.repo.Draft(ctx, id)
}

// ListDrafts returns a project's drafts as summaries.
func (e *Engine) ListDrafts(ctx context.Context, project domain.ProjectID) ([]domain.Draft, error) {
	return e.repo.ListDrafts(ctx, project)
}

// SaveDraft persists edits to a draft (its header, sections, and claims),
// stamping UpdatedAt. CreatedAt is taken from the stored row so a save cannot
// rewrite it.
func (e *Engine) SaveDraft(ctx context.Context, d domain.Draft) (saved domain.Draft, err error) {
	defer e.observeDuration("engine.save_draft", time.Now(), &err)
	if d.ID == "" {
		return domain.Draft{}, errors.New("engine: draft id required")
	}
	existing, err := e.repo.Draft(ctx, d.ID)
	if err != nil {
		return domain.Draft{}, err
	}
	d.Project = existing.Project
	d.CreatedAt = existing.CreatedAt
	d.UpdatedAt = time.Now().UTC()
	if d.Kind == "" {
		d.Kind = existing.Kind
	}
	if d.Status == "" {
		d.Status = existing.Status
	}
	if err := e.repo.SaveDraft(ctx, d); err != nil {
		return domain.Draft{}, err
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionDraftSave,
		Entity:   "draft",
		EntityID: string(d.ID),
		Status:   "committed",
	})
	e.announceChange()
	return d, nil
}

// DeleteDraft removes a draft and its sections and claims.
func (e *Engine) DeleteDraft(ctx context.Context, id domain.DraftID) (err error) {
	defer e.observeDuration("engine.delete_draft", time.Now(), &err)
	if id == "" {
		return errors.New("engine: draft id required")
	}
	if err := e.repo.DeleteDraft(ctx, id); err != nil {
		return err
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionDraftDelete,
		Entity:   "draft",
		EntityID: string(id),
		Status:   "committed",
	})
	e.announceChange()
	return nil
}

// ExportDraftDocx renders a draft to a .docx file under the configured docs
// export base and returns the absolute path. A response draft pulls its caption
// from the linked office action when
// present, otherwise from the project's prosecution header.
func (e *Engine) ExportDraftDocx(ctx context.Context, id domain.DraftID) (path string, err error) {
	defer e.observeDuration("engine.export_draft_docx", time.Now(), &err)
	base := e.docsExportDir
	if base == "" {
		return "", errors.New("engine: docs export dir not configured")
	}
	d, err := e.repo.Draft(ctx, id)
	if err != nil {
		return "", err
	}
	project, err := e.repo.Project(ctx, d.Project)
	if err != nil {
		return "", err
	}
	var oa *domain.OfficeAction
	if d.OfficeActionID != "" {
		loaded, err := e.repo.OfficeAction(ctx, d.OfficeActionID)
		if err == nil {
			oa = &loaded
		}
	}

	data, err := docx.RenderDraft(d, project, oa)
	if err != nil {
		return "", fmt.Errorf("engine: render draft: %w", err)
	}

	dir := filepath.Join(base, sanitizeProjectID(string(d.Project)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("engine: create draft dir: %w", err)
	}
	path = filepath.Join(dir, docx.DraftFilename(d, time.Now().UTC()))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("engine: write draft docx: %w", err)
	}

	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionDraftExportDocx,
		Entity:   "draft",
		EntityID: string(id),
		Status:   "ok",
		Attributes: map[string]any{
			"path":  path,
			"kind":  string(d.Kind),
			"bytes": len(data),
		},
	})
	e.incCounter("engine.draft.export_docx_total", 1)
	return path, nil
}

// DraftAIInput selects the section to draft and the grounding the model may use.
type DraftAIInput struct {
	Draft          domain.DraftID
	SectionOrdinal int
	Instruction    string
	Notes          string
	Pinned         []string
	PriorArt       []ai.PriorArtRef
}

// DraftAIResult is a generated section the caller reviews before accepting.
// Problems lists the mechanical guardrail failures (pinned text not reproduced,
// or needs-attorney-input markers): an empty Problems means the guardrails
// passed, not that the text is correct.
type DraftAIResult struct {
	Text     string
	Problems []string
	Provider string
	Model    string
}

// DraftSection generates one section of a draft with the configured AI drafter,
// grounded only in the draft's own material (title, claims, the linked office
// action's text, and caller-supplied references and notes). It returns the
// generated text plus any guardrail problems; it deliberately does NOT save the
// result — the author reviews, edits, and then accepts it via SaveDraft, which
// matches legal review practice and keeps machine text out of the document until
// a human signs off.
func (e *Engine) DraftSection(ctx context.Context, in DraftAIInput) (res DraftAIResult, err error) {
	defer e.observeDuration("engine.draft_section_ai", time.Now(), &err)
	if e.drafter == nil {
		return DraftAIResult{}, errors.New("engine: no AI drafter configured; set up Ollama (cargo make ollama-setup) or a Gemini API key and restart the daemon")
	}
	d, err := e.repo.Draft(ctx, in.Draft)
	if err != nil {
		return DraftAIResult{}, err
	}
	if in.SectionOrdinal < 0 || in.SectionOrdinal >= len(d.Sections) {
		return DraftAIResult{}, fmt.Errorf("engine: section ordinal %d out of range (draft has %d sections)", in.SectionOrdinal, len(d.Sections))
	}
	section := d.Sections[in.SectionOrdinal]

	project, err := e.repo.Project(ctx, d.Project)
	if err != nil {
		return DraftAIResult{}, err
	}

	var oaText string
	if d.OfficeActionID != "" {
		if oa, err := e.repo.OfficeAction(ctx, d.OfficeActionID); err == nil {
			oaText = oa.ExtractedText
		}
	}

	pinned := in.Pinned
	if len(pinned) == 0 {
		pinned = section.Pinned
	}

	g := ai.GroundingInput{
		Kind:             d.Kind,
		Section:          section.Kind,
		SectionHeading:   section.Heading,
		Title:            d.Title,
		Project:          project,
		Claims:           d.Claims,
		PriorArt:         in.PriorArt,
		OfficeActionText: oaText,
		Notes:            in.Notes,
		Pinned:           pinned,
		Instruction:      in.Instruction,
	}
	prompt := ai.BuildDraftPrompt(g)
	aiStart := time.Now()
	text, err := e.drafter.Complete(ctx, prompt)
	if err != nil {
		return DraftAIResult{}, fmt.Errorf("engine: draft section: %w", err)
	}

	res = DraftAIResult{
		Text:     text,
		Problems: ai.VerifyDraft(text, pinned),
		Provider: string(e.drafter.Provider()),
		Model:    e.drafter.Model(),
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionDraftSectionAI,
		Entity:   "draft",
		EntityID: string(in.Draft),
		Status:   "ok",
		Attributes: map[string]any{
			"section":  string(section.Kind),
			"provider": res.Provider,
			"model":    res.Model,
			"problems": len(res.Problems),
		},
	})
	e.recordAICall(ctx, d.Project, d.OfficeActionID, string(in.Draft), res.Provider, res.Model, time.Since(aiStart))
	e.incCounter("engine.draft.section_ai_total", 1)
	return res, nil
}

// ImportOfficeActionInput describes an office action to record against a project.
// SourcePath, when set, is a local PDF (or text) file to copy into the project's
// office-action store; ExtractedText may carry pre-extracted plain text for
// grounding (a .txt SourcePath is read in automatically).
type ImportOfficeActionInput struct {
	Project           domain.ProjectID
	ApplicationNumber string
	MailDate          time.Time
	Type              domain.OAType
	Examiner          string
	ArtUnit           string
	SourcePath        string
	ExtractedText     string
	Source            string
}

// ImportOfficeAction records an office action for a project. The examiner
// document is copied onto disk (hash recorded for integrity) under the project's
// office-action directory — the source_snapshot convention of pointer-in-row,
// bytes-on-disk — rather than stored in SQLite. This is the manual-import path
// that always works; ODP auto-fetch can populate the same record later.
func (e *Engine) ImportOfficeAction(ctx context.Context, in ImportOfficeActionInput) (oa domain.OfficeAction, err error) {
	defer e.observeDuration("engine.import_office_action", time.Now(), &err)
	if in.Project == "" {
		return domain.OfficeAction{}, errors.New("engine: office action requires a project")
	}
	if in.Type != "" && !in.Type.Valid() {
		return domain.OfficeAction{}, fmt.Errorf("engine: invalid office action type %q", in.Type)
	}
	if _, err := e.repo.Project(ctx, in.Project); err != nil {
		return domain.OfficeAction{}, fmt.Errorf("engine: import office action: %w", err)
	}

	now := time.Now().UTC()
	source := in.Source
	if source == "" {
		source = domain.OASourceManual
	}
	oa = domain.OfficeAction{
		ID:                fmt.Sprintf("oa-%d", now.UnixNano()),
		Project:           in.Project,
		ApplicationNumber: in.ApplicationNumber,
		MailDate:          in.MailDate,
		Type:              in.Type,
		Examiner:          in.Examiner,
		ArtUnit:           in.ArtUnit,
		ExtractedText:     in.ExtractedText,
		// Seed the response deadline from the mail date and type; the user can
		// edit it later. An action awaiting a response opens as such.
		ResponseDue: domain.ResponseDeadline(in.MailDate, in.Type),
		Status:      domain.OAStatusOpen,
		Source:      source,
		ImportedAt:  now,
	}

	var docName string
	if path := strings.TrimSpace(in.SourcePath); path != "" {
		base := e.docsExportDir
		if base == "" {
			return domain.OfficeAction{}, errors.New("engine: docs export dir not configured")
		}
		dir := filepath.Join(base, sanitizeProjectID(string(in.Project)), "office-actions")
		stored, hash, err := copyWithHash(path, dir, oa.ID)
		if err != nil {
			return domain.OfficeAction{}, fmt.Errorf("engine: store office action document: %w", err)
		}
		oa.BlobPath = stored
		oa.BlobHash = hash
		docName = filepath.Base(path)
		// Extract the searchable/groundable text when none was supplied (.txt
		// inline, .pdf via the extractor).
		if oa.ExtractedText == "" {
			oa.ExtractedText = extractDocText(path)
		}
		if oa.ExtractedText != "" {
			saveExtractedTextFile(oa.BlobPath, oa.ExtractedText)
		}
	}

	if err := e.repo.SaveOfficeAction(ctx, oa); err != nil {
		return domain.OfficeAction{}, err
	}

	// Surface the examiner's letter in the matter's document list. The blob is
	// already stored on disk, so the document row just points at it (id derived
	// from the OA id to match the v7→v8 backfill and stay idempotent).
	if oa.BlobPath != "" {
		if docName == "" {
			docName = domain.MatterDocOA.Label()
		}
		doc := domain.MatterDocument{
			ID:             "mdoc-" + oa.ID,
			Project:        oa.Project,
			OfficeActionID: oa.ID,
			Kind:           domain.MatterDocOA,
			DisplayName:    docName,
			BlobPath:       oa.BlobPath,
			BlobHash:       oa.BlobHash,
			ExtractedText:  oa.ExtractedText,
			AddedAt:        now,
		}
		if err := e.repo.SaveMatterDocument(ctx, doc); err != nil {
			return domain.OfficeAction{}, fmt.Errorf("engine: record office action document: %w", err)
		}
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionOfficeActionImport,
		Entity:   "office_action",
		EntityID: oa.ID,
		Status:   "committed",
		Attributes: map[string]any{
			"project": string(in.Project),
			"source":  source,
			"stored":  oa.BlobPath != "",
		},
	})
	e.announceChange()
	return oa, nil
}

// OfficeAction returns one stored office action.
func (e *Engine) OfficeAction(ctx context.Context, id string) (domain.OfficeAction, error) {
	return e.repo.OfficeAction(ctx, id)
}

// SaveOfficeActionNotes replaces the attorney notes on one office action and
// returns the updated record. The rest of the office action is left untouched.
func (e *Engine) SaveOfficeActionNotes(ctx context.Context, id, notes string) (oa domain.OfficeAction, err error) {
	defer e.observeDuration("engine.save_office_action_notes", time.Now(), &err)
	if id == "" {
		return domain.OfficeAction{}, errors.New("engine: office action id required")
	}
	oa, err = e.repo.OfficeAction(ctx, id)
	if err != nil {
		return domain.OfficeAction{}, err
	}
	oa.Notes = notes
	if err := e.repo.SaveOfficeAction(ctx, oa); err != nil {
		return domain.OfficeAction{}, err
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionOfficeActionUpdate,
		Entity:   "office_action",
		EntityID: oa.ID,
		Status:   "committed",
		Attributes: map[string]any{
			"project": string(oa.Project),
			"notes":   true,
		},
	})
	e.announceChange()
	return oa, nil
}

// UpdateOfficeActionMeta updates an office action's metadata and recomputes its response due date.
func (e *Engine) UpdateOfficeActionMeta(ctx context.Context, id string, examiner, mailDateStr string, oaType domain.OAType, artUnit, appNumber string) (oa domain.OfficeAction, err error) {
	defer e.observeDuration("engine.update_office_action_meta", time.Now(), &err)
	if id == "" {
		return domain.OfficeAction{}, errors.New("engine: office action id required")
	}
	oa, err = e.repo.OfficeAction(ctx, id)
	if err != nil {
		return domain.OfficeAction{}, err
	}

	mailDate, err := time.Parse(domain.DateLayout, mailDateStr)
	if err != nil {
		return domain.OfficeAction{}, fmt.Errorf("engine: invalid mail date format %q: %w", mailDateStr, err)
	}

	if !oaType.Valid() {
		return domain.OfficeAction{}, fmt.Errorf("engine: invalid office action type %q", oaType)
	}

	oa.Examiner = examiner
	oa.MailDate = mailDate
	oa.Type = oaType
	oa.ArtUnit = artUnit
	oa.ApplicationNumber = appNumber
	oa.ResponseDue = domain.ResponseDeadline(mailDate, oaType)

	if err := e.repo.SaveOfficeAction(ctx, oa); err != nil {
		return domain.OfficeAction{}, err
	}

	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionOfficeActionUpdate,
		Entity:   "office_action",
		EntityID: oa.ID,
		Status:   "committed",
		Attributes: map[string]any{
			"project":   string(oa.Project),
			"type":      string(oa.Type),
			"mail_date": oa.MailDate.Format(domain.DateLayout),
		},
	})
	e.announceChange()
	return oa, nil
}

// ListOfficeActions returns a project's office actions, newest first.
func (e *Engine) ListOfficeActions(ctx context.Context, project domain.ProjectID) ([]domain.OfficeAction, error) {
	return e.repo.ListOfficeActions(ctx, project)
}

// ImportMatterDocumentInput describes a file to file under a matter (a project).
// OfficeActionID is optional — empty for a matter-wide document (a specification,
// a shared reference), set to relate the document to one office action.
type ImportMatterDocumentInput struct {
	Project        domain.ProjectID
	OfficeActionID string
	SourcePath     string
	Kind           domain.MatterDocKind
	DisplayName    string
}

// ImportMatterDocument copies a local file into the matter's document store
// (bytes on disk, pointer + extracted text in the row) and records it. This is
// the multi-document path: a matter accumulates the office action, the response,
// references, and so on, and a response is drafted by copying from them.
func (e *Engine) ImportMatterDocument(ctx context.Context, in ImportMatterDocumentInput) (doc domain.MatterDocument, err error) {
	defer e.observeDuration("engine.import_matter_document", time.Now(), &err)
	if in.Project == "" {
		return domain.MatterDocument{}, errors.New("engine: matter document requires a project")
	}
	if in.Kind != "" && !in.Kind.Valid() {
		return domain.MatterDocument{}, fmt.Errorf("engine: invalid matter document kind %q", in.Kind)
	}
	if _, err := e.repo.Project(ctx, in.Project); err != nil {
		return domain.MatterDocument{}, fmt.Errorf("engine: import matter document: %w", err)
	}
	path := strings.TrimSpace(in.SourcePath)
	if path == "" {
		return domain.MatterDocument{}, errors.New("engine: matter document requires a source path")
	}
	base := e.docsExportDir
	if base == "" {
		return domain.MatterDocument{}, errors.New("engine: docs export dir not configured")
	}

	now := time.Now().UTC()
	kind := in.Kind
	if kind == "" {
		kind = domain.MatterDocOther
	}
	name := strings.TrimSpace(in.DisplayName)
	if name == "" {
		name = filepath.Base(path)
	}
	doc = domain.MatterDocument{
		ID:             fmt.Sprintf("mdoc-%d", now.UnixNano()),
		Project:        in.Project,
		OfficeActionID: in.OfficeActionID,
		Kind:           kind,
		DisplayName:    name,
		AddedAt:        now,
	}
	dir := filepath.Join(base, sanitizeProjectID(string(in.Project)), "documents")
	stored, hash, err := copyWithHash(path, dir, doc.ID)
	if err != nil {
		return domain.MatterDocument{}, fmt.Errorf("engine: store matter document: %w", err)
	}
	doc.BlobPath = stored
	doc.BlobHash = hash
	doc.ExtractedText = extractDocText(path)
	if doc.ExtractedText != "" {
		saveExtractedTextFile(doc.BlobPath, doc.ExtractedText)
	}

	if err := e.repo.SaveMatterDocument(ctx, doc); err != nil {
		return domain.MatterDocument{}, err
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionMatterDocumentImport,
		Entity:   "matter_document",
		EntityID: doc.ID,
		Status:   "committed",
		Attributes: map[string]any{
			"project":       string(in.Project),
			"kind":          string(kind),
			"office_action": in.OfficeActionID,
			"name":          doc.DisplayName,
		},
	})
	e.announceChange()
	return doc, nil
}

// ListMatterDocuments returns a project's documents, newest first.
func (e *Engine) ListMatterDocuments(ctx context.Context, project domain.ProjectID) ([]domain.MatterDocument, error) {
	return e.repo.ListMatterDocuments(ctx, project)
}

// MatterDocument returns one stored matter document.
func (e *Engine) MatterDocument(ctx context.Context, id string) (domain.MatterDocument, error) {
	return e.repo.MatterDocument(ctx, id)
}

// RenameMatterDocument changes only the display label of a document and returns
// the updated record.
func (e *Engine) RenameMatterDocument(ctx context.Context, id, name string) (doc domain.MatterDocument, err error) {
	defer e.observeDuration("engine.rename_matter_document", time.Now(), &err)
	if id == "" {
		return domain.MatterDocument{}, errors.New("engine: matter document id required")
	}
	if strings.TrimSpace(name) == "" {
		return domain.MatterDocument{}, errors.New("engine: matter document name required")
	}
	if err := e.repo.RenameMatterDocument(ctx, id, name); err != nil {
		return domain.MatterDocument{}, err
	}
	doc, err = e.repo.MatterDocument(ctx, id)
	if err != nil {
		return domain.MatterDocument{}, err
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionMatterDocumentRename,
		Entity:   "matter_document",
		EntityID: id,
		Status:   "committed",
		Attributes: map[string]any{
			"project": string(doc.Project),
			"name":    doc.DisplayName,
		},
	})
	e.announceChange()
	return doc, nil
}

// DeleteMatterDocument removes a document. The blob on disk is unlinked only when
// the document owns it (a standalone file under the matter's documents dir); an
// office action's own document shares the office-action blob and is left intact
// so deleting it never destroys the examiner's letter.
func (e *Engine) DeleteMatterDocument(ctx context.Context, id string) (err error) {
	defer e.observeDuration("engine.delete_matter_document", time.Now(), &err)
	if id == "" {
		return errors.New("engine: matter document id required")
	}
	doc, err := e.repo.MatterDocument(ctx, id)
	if err != nil {
		return err
	}
	if err := e.repo.DeleteMatterDocument(ctx, id); err != nil {
		return err
	}
	if doc.BlobPath != "" && e.docsExportDir != "" {
		docsDir := filepath.Join(e.docsExportDir, sanitizeProjectID(string(doc.Project)), "documents")
		if strings.HasPrefix(doc.BlobPath, docsDir+string(os.PathSeparator)) {
			_ = os.Remove(doc.BlobPath)
		}
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionMatterDocumentDelete,
		Entity:   "matter_document",
		EntityID: id,
		Status:   "committed",
		Attributes: map[string]any{
			"project": string(doc.Project),
			"name":    doc.DisplayName,
		},
	})
	e.announceChange()
	return nil
}

func (e *Engine) DeleteOfficeAction(ctx context.Context, id string, deleteFiles bool) (oa domain.OfficeAction, err error) {
	defer e.observeDuration("engine.delete_office_action", time.Now(), &err)
	if id == "" {
		return domain.OfficeAction{}, errors.New("engine: office action id required")
	}
	oa, err = e.repo.OfficeAction(ctx, id)
	if err != nil {
		return domain.OfficeAction{}, err
	}

	if err := e.repo.DeleteOfficeAction(ctx, id); err != nil {
		return domain.OfficeAction{}, err
	}

	_ = e.repo.DeleteMatterDocument(ctx, "mdoc-"+oa.ID)

	if deleteFiles && oa.BlobPath != "" && e.docsExportDir != "" {
		oaDir := filepath.Join(e.docsExportDir, sanitizeProjectID(string(oa.Project)), "office-actions")
		if strings.HasPrefix(oa.BlobPath, oaDir+string(os.PathSeparator)) {
			_ = os.Remove(oa.BlobPath)
			if extPath := extractedTextPath(oa.BlobPath); extPath != "" {
				_ = os.Remove(extPath)
			}
		}
	}

	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionOfficeActionDelete,
		Entity:   "office_action",
		EntityID: oa.ID,
		Status:   "committed",
		Attributes: map[string]any{
			"project": string(oa.Project),
		},
	})
	e.announceChange()
	return oa, nil
}

// ExtractDocumentText (re-)derives a document's plain text with the configured
// multimodal AI provider — OCR for a scanned, image-only PDF the pure-Go
// extractor could not read — and saves it back onto the document. It is a
// deliberately separate, on-demand step (not part of import) because OCR of a
// multi-page scan is slow and bills AI usage, so the user triggers it per
// document. Requires an AI provider that satisfies ai.Extractor (cloud Gemini);
// a text-only/local provider returns an actionable error.
func (e *Engine) ExtractDocumentText(ctx context.Context, id string) (doc domain.MatterDocument, err error) {
	defer e.observeDuration("engine.extract_document_text", time.Now(), &err)
	if id == "" {
		return domain.MatterDocument{}, errors.New("engine: matter document id required")
	}
	doc, err = e.repo.MatterDocument(ctx, id)
	if err != nil {
		return domain.MatterDocument{}, err
	}
	if doc.BlobPath == "" {
		return domain.MatterDocument{}, errors.New("engine: document has no stored file to extract")
	}

	var text string
	extPath := extractedTextPath(doc.BlobPath)
	if extPath != "" {
		if diskData, err := os.ReadFile(extPath); err == nil && len(diskData) > 0 {
			text = string(diskData)
		}
	}

	if text == "" {
		if stdTxt := extractDocText(doc.BlobPath); stdTxt != "" {
			text = stdTxt
			doc.ExtractedText = text
			if err := e.repo.SaveMatterDocument(ctx, doc); err != nil {
				return domain.MatterDocument{}, err
			}
			if doc.Kind == domain.MatterDocOA && doc.OfficeActionID != "" {
				if oa, err := e.repo.OfficeAction(ctx, doc.OfficeActionID); err == nil {
					oa.ExtractedText = text
					_ = e.repo.SaveOfficeAction(ctx, oa)
				}
			}
			saveExtractedTextFile(doc.BlobPath, text)

			e.recordActivity(ctx, observability.Record{
				Action:   observability.ActionMatterDocumentExtract,
				Entity:   "matter_document",
				EntityID: doc.ID,
				Status:   "committed",
				Attributes: map[string]any{
					"project": string(doc.Project),
					"source":  "standard_parser",
					"chars":   len(text),
					"name":    doc.DisplayName,
				},
			})
			e.announceChange()
			return doc, nil
		}

		extractor, ok := e.drafter.(ai.Extractor)
		if !ok {
			return domain.MatterDocument{}, errors.New("engine: standard extraction found no text, and AI text extraction is not available with the configured provider (set a Gemini API key)")
		}

		data, err := os.ReadFile(doc.BlobPath)
		if err != nil {
			return domain.MatterDocument{}, fmt.Errorf("engine: read document file: %w", err)
		}
		aiStart := time.Now()
		text, err = extractor.ExtractText(ctx, data, mimeForPath(doc.BlobPath))
		if err != nil {
			return domain.MatterDocument{}, fmt.Errorf("engine: extract document text: %w", err)
		}
		aiElapsed := time.Since(aiStart)
		if strings.TrimSpace(text) == "" {
			return domain.MatterDocument{}, errors.New("engine: no text could be extracted from the document")
		}
		doc.ExtractedText = text
		if err := e.repo.SaveMatterDocument(ctx, doc); err != nil {
			return domain.MatterDocument{}, err
		}
		saveExtractedTextFile(doc.BlobPath, text)

		provider, model := string(extractor.Provider()), extractor.Model()
		e.recordActivity(ctx, observability.Record{
			Action:   observability.ActionMatterDocumentExtract,
			Entity:   "matter_document",
			EntityID: doc.ID,
			Status:   "committed",
			Attributes: map[string]any{
				"project":  string(doc.Project),
				"provider": provider,
				"model":    model,
				"chars":    len(text),
				"name":     doc.DisplayName,
			},
		})
		e.log(ctx, slog.LevelInfo, "extracted document text with AI",
			slog.String("document_id", doc.ID),
			slog.String("provider", provider),
			slog.String("model", model),
			slog.Int("chars", len(text)))
		e.recordAICall(ctx, doc.Project, doc.OfficeActionID, "", provider, model, aiElapsed)
	} else {
		doc.ExtractedText = text
		if err := e.repo.SaveMatterDocument(ctx, doc); err != nil {
			return domain.MatterDocument{}, err
		}
		e.recordActivity(ctx, observability.Record{
			Action:   observability.ActionMatterDocumentExtract,
			Entity:   "matter_document",
			EntityID: doc.ID,
			Status:   "committed",
			Attributes: map[string]any{
				"project": string(doc.Project),
				"source":  "filesystem_cache",
				"chars":   len(text),
				"name":    doc.DisplayName,
			},
		})
		e.log(ctx, slog.LevelInfo, "loaded extracted document text from filesystem cache",
			slog.String("document_id", doc.ID),
			slog.Int("chars", len(text)))
	}

	e.announceChange()
	return doc, nil
}

// mimeForPath maps a stored document's extension to the MIME type the multimodal
// extractor expects, defaulting to PDF.
func mimeForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".tif", ".tiff":
		return "image/tiff"
	default:
		return "application/pdf"
	}
}

// SetMatterType records the prosecution stage of a project (provisional,
// non-provisional, in-prosecution, issued) and returns the updated project. An
// empty value clears the classification.
func (e *Engine) SetMatterType(ctx context.Context, project domain.ProjectID, mt domain.MatterType) (p domain.Project, err error) {
	defer e.observeDuration("engine.set_matter_type", time.Now(), &err)
	if project == "" {
		return domain.Project{}, errors.New("engine: project id required")
	}
	if mt != "" && !mt.Valid() {
		return domain.Project{}, fmt.Errorf("engine: invalid matter type %q", mt)
	}
	p, err = e.repo.Project(ctx, project)
	if err != nil {
		return domain.Project{}, err
	}
	p.MatterType = mt
	if err := e.repo.SaveProject(ctx, p); err != nil {
		return domain.Project{}, err
	}
	e.recordActivity(ctx, observability.Record{
		Action: observability.ActionProjectSetMatterType, Entity: "project",
		EntityID: string(project), Status: "committed",
		Attributes: map[string]any{"matter_type": string(mt)},
	})
	e.announceChange()
	return p, nil
}

// AddMatterEventInput describes one communications-log entry to record.
type AddMatterEventInput struct {
	Project        domain.ProjectID
	OfficeActionID string
	Kind           domain.MatterEventKind
	Party          string
	OccurredAt     time.Time
	DueAt          time.Time
	Summary        string
	Comment        string
}

// AddMatterEvent records one entry in a matter's communications log (a call, an
// email, an examiner interview, a filing, a deadline). OccurredAt defaults to now
// when unset.
func (e *Engine) AddMatterEvent(ctx context.Context, in AddMatterEventInput) (ev domain.MatterEvent, err error) {
	defer e.observeDuration("engine.add_matter_event", time.Now(), &err)
	if in.Project == "" {
		return domain.MatterEvent{}, errors.New("engine: matter event requires a project")
	}
	if in.Kind != "" && !in.Kind.Valid() {
		return domain.MatterEvent{}, fmt.Errorf("engine: invalid matter event kind %q", in.Kind)
	}
	if _, err := e.repo.Project(ctx, in.Project); err != nil {
		return domain.MatterEvent{}, fmt.Errorf("engine: add matter event: %w", err)
	}
	now := time.Now().UTC()
	kind := in.Kind
	if kind == "" {
		kind = domain.MatterEventNote
	}
	occurred := in.OccurredAt
	if occurred.IsZero() {
		occurred = now
	}
	ev = domain.MatterEvent{
		ID:             fmt.Sprintf("mev-%d", now.UnixNano()),
		Project:        in.Project,
		OfficeActionID: in.OfficeActionID,
		Kind:           kind,
		Party:          in.Party,
		OccurredAt:     occurred,
		DueAt:          in.DueAt,
		Summary:        in.Summary,
		Comment:        in.Comment,
		CreatedAt:      now,
	}
	if err := e.repo.SaveMatterEvent(ctx, ev); err != nil {
		return domain.MatterEvent{}, err
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionMatterEventAdd,
		Entity:   "matter_event",
		EntityID: ev.ID,
		Status:   "committed",
		Attributes: map[string]any{
			"project": string(in.Project),
			"kind":    string(kind),
		},
	})
	e.announceChange()
	return ev, nil
}

// ListMatterEvents returns a project's communications-log entries, newest first.
func (e *Engine) ListMatterEvents(ctx context.Context, project domain.ProjectID) ([]domain.MatterEvent, error) {
	return e.repo.ListMatterEvents(ctx, project)
}

// DeleteMatterEvent removes one communications-log entry.
func (e *Engine) DeleteMatterEvent(ctx context.Context, id string) (err error) {
	defer e.observeDuration("engine.delete_matter_event", time.Now(), &err)
	if id == "" {
		return errors.New("engine: matter event id required")
	}
	if err := e.repo.DeleteMatterEvent(ctx, id); err != nil {
		return err
	}
	e.recordActivity(ctx, observability.Record{
		Action: observability.ActionMatterEventDelete, Entity: "matter_event",
		EntityID: id, Status: "committed",
	})
	e.announceChange()
	return nil
}

// LogTimeInput describes one unit of work to record against a matter.
type LogTimeInput struct {
	Project        domain.ProjectID
	OfficeActionID string
	Activity       domain.TimeActivity
	Seconds        int
	Source         domain.TimeSource
	Validated      bool
	Note           string
	StartedAt      time.Time
	EndedAt        time.Time
}

// LogTime records one time entry. Manual entries are validated on entry; auto
// entries (editor focus, AI calls) start unvalidated for the attorney to review.
func (e *Engine) LogTime(ctx context.Context, in LogTimeInput) (entry domain.TimeEntry, err error) {
	defer e.observeDuration("engine.log_time", time.Now(), &err)
	if in.Project == "" {
		return domain.TimeEntry{}, errors.New("engine: time entry requires a project")
	}
	if in.Activity != "" && !in.Activity.Valid() {
		return domain.TimeEntry{}, fmt.Errorf("engine: invalid time activity %q", in.Activity)
	}
	if in.Seconds <= 0 {
		return domain.TimeEntry{}, errors.New("engine: time entry needs a positive duration")
	}
	now := time.Now().UTC()
	source := in.Source
	if source == "" {
		source = domain.TimeSourceManual
	}
	entry = domain.TimeEntry{
		ID:             fmt.Sprintf("te-%d", now.UnixNano()),
		Project:        in.Project,
		OfficeActionID: in.OfficeActionID,
		Activity:       in.Activity,
		Source:         source,
		StartedAt:      in.StartedAt,
		EndedAt:        in.EndedAt,
		Seconds:        in.Seconds,
		Validated:      in.Validated,
		Note:           in.Note,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if in.Validated {
		entry.ValidatedAt = now
	}
	if err := e.repo.SaveTimeEntry(ctx, entry); err != nil {
		return domain.TimeEntry{}, err
	}
	e.recordActivity(ctx, observability.Record{
		Action: observability.ActionTimeLog, Entity: "time_entry", EntityID: entry.ID, Status: "committed",
		Attributes: map[string]any{
			"project": string(in.Project), "activity": string(in.Activity),
			"seconds": in.Seconds, "source": string(source),
		},
	})
	e.announceChange()
	return entry, nil
}

// ListTime returns a project's time entries, newest first.
func (e *Engine) ListTime(ctx context.Context, project domain.ProjectID) ([]domain.TimeEntry, error) {
	return e.repo.ListTimeEntries(ctx, project)
}

// ListUnvalidatedTime returns a project's unvalidated time entries (review queue).
func (e *Engine) ListUnvalidatedTime(ctx context.Context, project domain.ProjectID) ([]domain.TimeEntry, error) {
	return e.repo.ListUnvalidatedTime(ctx, project)
}

// UnvalidatedTimeCount returns how many of a project's time entries await
// validation — the trigger for the validate-on-reopen prompt.
func (e *Engine) UnvalidatedTimeCount(ctx context.Context, project domain.ProjectID) (int, error) {
	return e.repo.CountUnvalidatedTime(ctx, project)
}

// TimeSummary aggregates a project's recorded time by activity and review state.
func (e *Engine) TimeSummary(ctx context.Context, project domain.ProjectID) (domain.TimeSummary, error) {
	return e.repo.SummarizeTime(ctx, project)
}

// AIUsageSummary aggregates a project's AI usage for billing.
func (e *Engine) AIUsageSummary(ctx context.Context, project domain.ProjectID) (domain.AIUsageSummary, error) {
	return e.repo.SummarizeAIUsage(ctx, project)
}

// UpdateTimeEntry applies an attorney's corrections to an auto-captured entry and
// optionally marks it validated, the heart of the review-before-billing flow.
func (e *Engine) UpdateTimeEntry(ctx context.Context, id string, seconds int, activity domain.TimeActivity, note string, validated bool) (entry domain.TimeEntry, err error) {
	defer e.observeDuration("engine.update_time_entry", time.Now(), &err)
	if id == "" {
		return domain.TimeEntry{}, errors.New("engine: time entry id required")
	}
	if activity != "" && !activity.Valid() {
		return domain.TimeEntry{}, fmt.Errorf("engine: invalid time activity %q", activity)
	}
	entry, err = e.repo.TimeEntry(ctx, id)
	if err != nil {
		return domain.TimeEntry{}, err
	}
	now := time.Now().UTC()
	if seconds > 0 {
		entry.Seconds = seconds
	}
	if activity != "" {
		entry.Activity = activity
	}
	entry.Note = note
	if validated && !entry.Validated {
		entry.ValidatedAt = now
	}
	entry.Validated = validated
	entry.UpdatedAt = now
	if err := e.repo.SaveTimeEntry(ctx, entry); err != nil {
		return domain.TimeEntry{}, err
	}
	if validated {
		e.recordActivity(ctx, observability.Record{
			Action: observability.ActionTimeValidate, Entity: "time_entry", EntityID: id, Status: "committed",
		})
	}
	e.announceChange()
	return entry, nil
}

// DeleteTimeEntry removes one time entry (a mistaken auto-capture the attorney
// rejects during review).
func (e *Engine) DeleteTimeEntry(ctx context.Context, id string) (err error) {
	defer e.observeDuration("engine.delete_time_entry", time.Now(), &err)
	if id == "" {
		return errors.New("engine: time entry id required")
	}
	if err := e.repo.DeleteTimeEntry(ctx, id); err != nil {
		return err
	}
	e.recordActivity(ctx, observability.Record{
		Action: observability.ActionTimeDelete, Entity: "time_entry", EntityID: id, Status: "committed",
	})
	e.announceChange()
	return nil
}

// recordAICall persists an AI call against a matter (for billing) and an auto,
// unvalidated time entry for the time it took, so AI work shows up in both the
// AI-usage and time summaries. Best-effort: a recording failure is logged but
// does not fail the AI operation that triggered it.
func (e *Engine) recordAICall(ctx context.Context, project domain.ProjectID, oaID, draftID, provider, model string, elapsed time.Duration) {
	if project == "" {
		return
	}
	now := time.Now().UTC()
	usage := domain.AIUsage{
		ID:             fmt.Sprintf("aiu-%d", now.UnixNano()),
		Project:        project,
		OfficeActionID: oaID,
		DraftID:        draftID,
		Provider:       provider,
		Model:          model,
		Prompts:        1,
		CreatedAt:      now,
	}
	if err := e.repo.SaveAIUsage(ctx, usage); err != nil {
		e.log(ctx, slog.LevelWarn, "record ai usage failed", slog.String("error", err.Error()))
	}
	e.recordActivity(ctx, observability.Record{
		Action: observability.ActionAIUsage, Entity: "ai_usage", EntityID: usage.ID, Status: "committed",
		Attributes: map[string]any{"project": string(project), "provider": provider, "model": model},
	})
	secs := int(elapsed.Round(time.Second) / time.Second)
	if secs <= 0 {
		secs = 1
	}
	if _, err := e.LogTime(ctx, LogTimeInput{
		Project: project, OfficeActionID: oaID, Activity: domain.TimeAI,
		Source: domain.TimeSourceAuto, Seconds: secs, Note: provider + " " + model,
	}); err != nil {
		e.log(ctx, slog.LevelWarn, "record ai time failed", slog.String("error", err.Error()))
	}
}

func extractedTextPath(blobPath string) string {
	if blobPath == "" {
		return ""
	}
	return strings.TrimSuffix(blobPath, filepath.Ext(blobPath)) + "_extracted.txt"
}

func saveExtractedTextFile(blobPath, text string) {
	if blobPath == "" || text == "" {
		return
	}
	_ = os.WriteFile(extractedTextPath(blobPath), []byte(text), 0644)
}

// extractDocText returns the plain text of a local document for reading and
// grounding: a text source is read inline, a PDF is run through the extractor,
// and anything else (or an unreadable file) yields an empty string.
func extractDocText(path string) string {
	if extPath := extractedTextPath(path); extPath != "" {
		if data, err := os.ReadFile(extPath); err == nil {
			return string(data)
		}
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md", ".text":
		if data, err := os.ReadFile(path); err == nil {
			return string(data)
		}
	case ".pdf":
		if txt, err := pdf.ExtractText(path); err == nil {
			return txt
		}
	case ".docx":
		if txt, err := extractDocxText(path); err == nil {
			return txt
		}
	}
	return ""
}

func extractDocxText(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = zr.Close() }()

	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer func() { _ = rc.Close() }()
			return parseDocxXML(rc)
		}
	}
	return "", fmt.Errorf("word/document.xml not found in docx")
}

func parseDocxXML(r io.Reader) (string, error) {
	dec := xml.NewDecoder(r)
	var b strings.Builder
	var inText bool
	for {
		t, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		switch se := t.(type) {
		case xml.StartElement:
			if se.Name.Local == "t" {
				inText = true
			} else if se.Name.Local == "p" || se.Name.Local == "br" || se.Name.Local == "cr" {
				b.WriteByte('\n')
			}
		case xml.EndElement:
			if se.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				b.Write(se)
			}
		}
	}
	return b.String(), nil
}

// copyWithHash copies src into dir, naming the file "<id><ext>", and returns the
// destination path and the source's SHA-256 hex digest.
func copyWithHash(src, dir, id string) (string, string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	dest := filepath.Join(dir, id+filepath.Ext(src))
	out, err := os.Create(dest)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = out.Close() }()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), in); err != nil {
		return "", "", err
	}
	return dest, hex.EncodeToString(h.Sum(nil)), nil
}
