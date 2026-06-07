package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/export/coversheet"
	"patentmine/internal/observability"
	"patentmine/internal/store"
)

// provisionalCoverSheetDocID derives the deterministic matter-document id for a project's
// rendered cover sheet, so re-approving replaces the same row in place.
func provisionalCoverSheetDocID(project domain.ProjectID) string {
	return matterDocProvisionalCoverSheetPrefix + string(project)
}

// provisionalCoverSheetTemplateDir returns the directory under the configured templates
// root where a user-supplied SB/16 blank may live to override the embedded form.
func (e *Engine) provisionalCoverSheetTemplateDir() string {
	return filepath.Join(e.templatesDir, domain.TemplatesOfficeActionsSubdir, string(domain.DocSetProvisional))
}

const matterDocProvisionalCoverSheetPrefix = "mdoc-coversheet-"

// ProvisionalCoverSheet returns a project's PTO/SB/16 cover sheet, or store.ErrNotFound
// when none has been started.
func (e *Engine) ProvisionalCoverSheet(ctx context.Context, project domain.ProjectID) (domain.ProvisionalCoverSheet, error) {
	if project == "" {
		return domain.ProvisionalCoverSheet{}, errors.New("engine: project id required")
	}
	return e.repo.ProvisionalCoverSheet(ctx, project)
}

// SaveProvisionalCoverSheet inserts or updates a project's cover sheet, stamping
// timestamps. CreatedAt is preserved from the stored record so a save cannot
// rewrite it; editing always clears Approved so a changed sheet must be
// re-approved before it is rendered to the filing PDF.
func (e *Engine) SaveProvisionalCoverSheet(ctx context.Context, cs domain.ProvisionalCoverSheet) (saved domain.ProvisionalCoverSheet, err error) {
	defer e.observeDuration("engine.save_provisional_cover_sheet", time.Now(), &err)
	if cs.Project == "" {
		return domain.ProvisionalCoverSheet{}, errors.New("engine: cover sheet requires a project")
	}
	if !cs.GovInterest.Valid() {
		return domain.ProvisionalCoverSheet{}, fmt.Errorf("engine: invalid government-interest value %q", cs.GovInterest)
	}
	if _, err := e.repo.Project(ctx, cs.Project); err != nil {
		return domain.ProvisionalCoverSheet{}, fmt.Errorf("engine: save cover sheet: %w", err)
	}
	now := time.Now().UTC()
	existing, err := e.repo.ProvisionalCoverSheet(ctx, cs.Project)
	switch {
	case err == nil:
		cs.ID = existing.ID
		cs.CreatedAt = existing.CreatedAt
	case errors.Is(err, store.ErrNotFound):
		if cs.ID == "" {
			cs.ID = fmt.Sprintf("cs-%d", now.UnixNano())
		}
		cs.CreatedAt = now
	default:
		return domain.ProvisionalCoverSheet{}, err
	}
	cs.Approved = false // an edit invalidates a prior approval
	cs.UpdatedAt = now
	if err := e.repo.SaveProvisionalCoverSheet(ctx, cs); err != nil {
		return domain.ProvisionalCoverSheet{}, err
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionProvisionalCoverSheetSave,
		Entity:   "provisional_cover_sheet",
		EntityID: cs.ID,
		Status:   observability.StatusCommitted,
		Attributes: map[string]any{
			"project": string(cs.Project),
			"title":   cs.Title,
		},
	})
	e.announceChange()
	return cs, nil
}

// PreviewProvisionalCoverSheetPDF renders the project's current cover-sheet data into the
// PTO/SB/16 form and returns the filled PDF bytes without persisting anything —
// the "view how the template is filled" step. It renders even an incomplete
// sheet so the author can see work in progress.
func (e *Engine) PreviewProvisionalCoverSheetPDF(ctx context.Context, project domain.ProjectID) (pdf []byte, err error) {
	defer e.observeDuration("engine.preview_provisional_cover_sheet", time.Now(), &err)
	cs, err := e.repo.ProvisionalCoverSheet(ctx, project)
	if err != nil {
		return nil, err
	}
	return coversheet.RenderProvisional(cs, e.provisionalCoverSheetTemplate())
}

// ApproveProvisionalCoverSheetResult reports the outcome of approving a cover sheet.
// Missing is non-empty only when approval was refused for an incomplete sheet —
// the basis for the TUI's "cannot submit" popup. When Missing is empty the PDF
// was rendered and filed.
type ApproveProvisionalCoverSheetResult struct {
	ProvisionalCoverSheet domain.ProvisionalCoverSheet
	Document              domain.MatterDocument
	Path                  string
	Missing               []string
}

// ApproveProvisionalCoverSheetPDF marks a project's cover sheet approved and renders it to
// the official filing PDF, registered as a tracked matter document (kind
// coversheet, stage approved). It refuses an incomplete sheet, returning the
// missing required fields rather than an error so the caller can show them in a
// popup. The rendered PDF uses a stable name and a deterministic document id, so
// re-approving replaces the prior PDF in place.
func (e *Engine) ApproveProvisionalCoverSheetPDF(ctx context.Context, project domain.ProjectID) (res ApproveProvisionalCoverSheetResult, err error) {
	defer e.observeDuration("engine.approve_provisional_cover_sheet", time.Now(), &err)
	base := e.docsExportDir
	if base == "" {
		return ApproveProvisionalCoverSheetResult{}, errors.New("engine: docs export dir not configured")
	}
	cs, err := e.repo.ProvisionalCoverSheet(ctx, project)
	if err != nil {
		return ApproveProvisionalCoverSheetResult{}, err
	}
	if miss := cs.Missing(); len(miss) > 0 {
		return ApproveProvisionalCoverSheetResult{ProvisionalCoverSheet: cs, Missing: miss}, nil
	}

	pdf, err := coversheet.RenderProvisional(cs, e.provisionalCoverSheetTemplate())
	if err != nil {
		return ApproveProvisionalCoverSheetResult{}, err
	}

	dir := filepath.Join(base, sanitizeProjectID(string(project)), string(domain.DocSetProvisional))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ApproveProvisionalCoverSheetResult{}, fmt.Errorf("engine: create provisional dir: %w", err)
	}
	path := filepath.Join(dir, coversheet.ProvisionalRenderedFilename)
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		return ApproveProvisionalCoverSheetResult{}, fmt.Errorf("engine: write cover sheet pdf: %w", err)
	}

	now := time.Now().UTC()
	docID := provisionalCoverSheetDocID(project)
	added := now
	if existing, err := e.repo.MatterDocument(ctx, docID); err == nil {
		added = existing.AddedAt
	} else if !errors.Is(err, store.ErrNotFound) {
		return ApproveProvisionalCoverSheetResult{}, err
	}
	sum := sha256.Sum256(pdf)
	doc := domain.MatterDocument{
		ID:          docID,
		Project:     project,
		Kind:        domain.MatterDocCoversheet,
		Origin:      domain.OriginFirm,
		Stage:       domain.StageApproved,
		DisplayName: coversheet.ProvisionalDocumentName,
		BlobPath:    path,
		BlobHash:    hex.EncodeToString(sum[:]),
		AddedAt:     added,
	}
	if err := e.repo.SaveMatterDocument(ctx, doc); err != nil {
		return ApproveProvisionalCoverSheetResult{}, err
	}

	cs.Approved = true
	cs.UpdatedAt = now
	if err := e.repo.SaveProvisionalCoverSheet(ctx, cs); err != nil {
		return ApproveProvisionalCoverSheetResult{}, err
	}

	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionProvisionalCoverSheetApprove,
		Entity:   "provisional_cover_sheet",
		EntityID: cs.ID,
		Status:   observability.StatusCommitted,
		Attributes: map[string]any{
			"project": string(project),
			"path":    path,
			"bytes":   len(pdf),
		},
	})
	e.announceChange()
	return ApproveProvisionalCoverSheetResult{ProvisionalCoverSheet: cs, Document: doc, Path: path}, nil
}

// provisionalCoverSheetTemplate returns a user-supplied SB/16 blank from the templates dir
// when one is present under the canonical name, else nil so the embedded form is
// used. A swapped form is honored as long as it keeps the official AcroForm
// field names.
func (e *Engine) provisionalCoverSheetTemplate() []byte {
	if e.templatesDir == "" {
		return nil
	}
	path := filepath.Join(e.provisionalCoverSheetTemplateDir(), coversheet.ProvisionalBlankFilename)
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		return data
	}
	return nil
}
