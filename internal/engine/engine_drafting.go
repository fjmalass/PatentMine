package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"patentmine/internal/ai"
	"patentmine/internal/domain"
	"patentmine/internal/export/docx"
	"patentmine/internal/observability"
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
		Source:            source,
		ImportedAt:        now,
	}

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
		// A plain-text source doubles as the extracted text when none was given.
		if oa.ExtractedText == "" && strings.EqualFold(filepath.Ext(path), ".txt") {
			if data, rErr := os.ReadFile(path); rErr == nil {
				oa.ExtractedText = string(data)
			}
		}
	}

	if err := e.repo.SaveOfficeAction(ctx, oa); err != nil {
		return domain.OfficeAction{}, err
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
	e.announceChange()
	return oa, nil
}

// ListOfficeActions returns a project's office actions, newest first.
func (e *Engine) ListOfficeActions(ctx context.Context, project domain.ProjectID) ([]domain.OfficeAction, error) {
	return e.repo.ListOfficeActions(ctx, project)
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
