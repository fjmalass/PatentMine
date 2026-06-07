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
	"patentmine/internal/export/markdown"
	"patentmine/internal/observability"
	"patentmine/internal/store"
)

// Filesystem and document-id layout for rendered response artifacts, pinned as
// named constants rather than scattered string literals.
const (
	responsesSubdir         = "responses"      // <docs>/<project>/responses/
	matterDocClaimsPrefix   = "mdoc-claims-"   // new_claims.md document id prefix
	matterDocResponsePrefix = "mdoc-response-" // response.md document id prefix
)

// SnapshotResult reports the outcome of generating a draft's new_claims.md /
// response.md: the immutable revision recorded behind the editable head, the two
// rendered files on disk, and the tracked MatterDocument rows that point at them.
type SnapshotResult struct {
	Revision    domain.DraftRevision
	ClaimsDoc   domain.MatterDocument
	ResponseDoc domain.MatterDocument
}

// SnapshotDraftResponse renders a draft's current state to new_claims.md and
// response.md, registers (or refreshes) the two files as tracked MatterDocuments
// under the matter, records an immutable draft_revision behind the editable
// head, and commits the rendered files to the per-matter git mirror. The working
// copy on disk always holds only the latest version (stable filenames); every
// prior version lives in the draft_revision history and the git log.
//
// kind classifies why the snapshot was taken (ai / manual / export / filed);
// label is an optional human note. Rendering is deterministic — it does not call
// the model — so this is safe to run on every save or export.
func (e *Engine) SnapshotDraftResponse(ctx context.Context, id domain.DraftID, kind domain.RevisionKind, label string) (res SnapshotResult, err error) {
	defer e.observeDuration("engine.snapshot_draft_response", time.Now(), &err)
	if id == "" {
		return SnapshotResult{}, errors.New("engine: draft id required")
	}
	if kind == "" {
		kind = domain.RevisionManual
	}
	if !kind.Valid() {
		return SnapshotResult{}, fmt.Errorf("engine: invalid revision kind %q", kind)
	}
	base := e.docsExportDir
	if base == "" {
		return SnapshotResult{}, errors.New("engine: docs export dir not configured")
	}

	d, err := e.repo.Draft(ctx, id)
	if err != nil {
		return SnapshotResult{}, err
	}
	project, err := e.repo.Project(ctx, d.Project)
	if err != nil {
		return SnapshotResult{}, err
	}
	var oa *domain.OfficeAction
	if d.OfficeActionID != "" {
		if loaded, err := e.repo.OfficeAction(ctx, d.OfficeActionID); err == nil {
			oa = &loaded
		}
	}

	claimsMD := markdown.RenderClaims(d, project, oa)
	responseMD := markdown.RenderResponse(d, project, oa)

	dir := filepath.Join(base, sanitizeProjectID(string(d.Project)), responsesSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return SnapshotResult{}, fmt.Errorf("engine: create responses dir: %w", err)
	}
	claimsPath := filepath.Join(dir, markdown.ClaimsFilename)
	responsePath := filepath.Join(dir, markdown.ResponseFilename)
	if err := os.WriteFile(claimsPath, []byte(claimsMD), 0o644); err != nil {
		return SnapshotResult{}, fmt.Errorf("engine: write %s: %w", markdown.ClaimsFilename, err)
	}
	if err := os.WriteFile(responsePath, []byte(responseMD), 0o644); err != nil {
		return SnapshotResult{}, fmt.Errorf("engine: write %s: %w", markdown.ResponseFilename, err)
	}

	now := time.Now().UTC()
	revno, err := e.repo.NextDraftRevno(ctx, id)
	if err != nil {
		return SnapshotResult{}, err
	}

	// Commit the rendered files to the matter's git mirror (best-effort backup).
	commit := gitCommitDir(ctx, dir, fmt.Sprintf("%s r%d: %s", kind.Label(), revno, snapshotMessage(d, label)))

	// Register/refresh the two files as tracked MatterDocuments. Deterministic ids
	// derived from the draft keep re-snapshots updating the same rows in place, so
	// the document list shows only the latest of each.
	res.ClaimsDoc, err = e.saveResponseDocument(ctx, d, matterDocClaimsPrefix+string(d.ID),
		domain.MatterDocAmendment, markdown.ClaimsFilename, claimsPath, claimsMD, now)
	if err != nil {
		return SnapshotResult{}, err
	}
	res.ResponseDoc, err = e.saveResponseDocument(ctx, d, matterDocResponsePrefix+string(d.ID),
		domain.MatterDocResponse, markdown.ResponseFilename, responsePath, responseMD, now)
	if err != nil {
		return SnapshotResult{}, err
	}

	rev := domain.DraftRevision{
		ID:               fmt.Sprintf("drev-%d", now.UnixNano()),
		Draft:            d.ID,
		Revno:            revno,
		Label:            label,
		Kind:             kind,
		Sections:         d.Sections,
		Claims:           d.Claims,
		ClaimsMarkdown:   claimsMD,
		ResponseMarkdown: responseMD,
		GitCommit:        commit,
		CreatedAt:        now,
	}
	if kind == domain.RevisionAI && e.drafter != nil {
		rev.Provider = string(e.drafter.Provider())
		rev.Model = e.drafter.Model()
	}
	if err := e.repo.SaveDraftRevision(ctx, rev); err != nil {
		return SnapshotResult{}, err
	}
	res.Revision = rev

	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionDraftSnapshot,
		Entity:   "draft",
		EntityID: string(d.ID),
		Status:   observability.StatusCommitted,
		Attributes: map[string]any{
			"project":       string(d.Project),
			"office_action": d.OfficeActionID,
			"revno":         revno,
			"kind":          string(kind),
			"git_commit":    commit,
		},
	})
	e.incCounter("engine.draft.snapshot_total", 1)
	e.announceChange()
	return res, nil
}

// snapshotMessage builds a human-readable git commit subject for a snapshot.
func snapshotMessage(d domain.Draft, label string) string {
	if label != "" {
		return label
	}
	if d.Title != "" {
		return d.Title
	}
	return string(d.ID)
}

// saveResponseDocument upserts one rendered .md as a tracked MatterDocument,
// preserving the original AddedAt across re-snapshots so the document keeps its
// place in the matter while its bytes and extracted text are refreshed in place.
func (e *Engine) saveResponseDocument(ctx context.Context, d domain.Draft, docID string, kind domain.MatterDocKind, name, path, content string, now time.Time) (domain.MatterDocument, error) {
	added := now
	if existing, err := e.repo.MatterDocument(ctx, docID); err == nil {
		added = existing.AddedAt
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.MatterDocument{}, err
	}
	sum := sha256.Sum256([]byte(content))
	origin, stage := domain.InferOriginStage(kind)
	doc := domain.MatterDocument{
		ID:             docID,
		Project:        d.Project,
		OfficeActionID: d.OfficeActionID,
		Kind:           kind,
		Origin:         origin,
		Stage:          stage,
		DisplayName:    name,
		BlobPath:       path,
		BlobHash:       hex.EncodeToString(sum[:]),
		ExtractedText:  content,
		AddedAt:        added,
	}
	if err := e.repo.SaveMatterDocument(ctx, doc); err != nil {
		return domain.MatterDocument{}, err
	}
	return doc, nil
}

// ListDraftRevisions returns a draft's revision history, newest first.
func (e *Engine) ListDraftRevisions(ctx context.Context, id domain.DraftID) ([]domain.DraftRevision, error) {
	if id == "" {
		return nil, errors.New("engine: draft id required")
	}
	return e.repo.ListDraftRevisions(ctx, id)
}

// DraftRevision returns one revision with its full structured and rendered content.
func (e *Engine) DraftRevision(ctx context.Context, id string) (domain.DraftRevision, error) {
	if id == "" {
		return domain.DraftRevision{}, errors.New("engine: revision id required")
	}
	return e.repo.DraftRevision(ctx, id)
}

// RestoreDraftRevision overwrites a draft's editable head with the structured
// sections and claims captured in one of its past revisions, then re-snapshots
// so the restore itself is recorded in the history (nothing is lost). It returns
// the restored draft.
func (e *Engine) RestoreDraftRevision(ctx context.Context, draftID domain.DraftID, revisionID string) (d domain.Draft, err error) {
	defer e.observeDuration("engine.restore_draft_revision", time.Now(), &err)
	if draftID == "" || revisionID == "" {
		return domain.Draft{}, errors.New("engine: draft id and revision id required")
	}
	rev, err := e.repo.DraftRevision(ctx, revisionID)
	if err != nil {
		return domain.Draft{}, err
	}
	if rev.Draft != draftID {
		return domain.Draft{}, errors.New("engine: revision belongs to a different draft")
	}
	d, err = e.repo.Draft(ctx, draftID)
	if err != nil {
		return domain.Draft{}, err
	}
	d.Sections = rev.Sections
	d.Claims = rev.Claims
	d.UpdatedAt = time.Now().UTC()
	if err := e.repo.SaveDraft(ctx, d); err != nil {
		return domain.Draft{}, err
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionDraftRestore,
		Entity:   "draft",
		EntityID: string(draftID),
		Status:   observability.StatusCommitted,
		Attributes: map[string]any{
			"project":     string(d.Project),
			"from_revno":  rev.Revno,
			"revision_id": revisionID,
		},
	})
	e.announceChange()
	// Record the restored state as a new revision so the head and history agree.
	if _, err := e.SnapshotDraftResponse(ctx, draftID, domain.RevisionManual,
		fmt.Sprintf("restored from r%d", rev.Revno)); err != nil {
		return domain.Draft{}, err
	}
	return e.repo.Draft(ctx, draftID)
}
