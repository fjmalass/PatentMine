// Drafting subsystem RPC handlers: provisional / non-provisional applications,
// office-action responses, and office-action import.
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"patentmine/internal/ai"
	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/proto"
)

func (s *Server) draftCreate(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftCreateParams](raw)
	if err != nil {
		return nil, err
	}
	d, err := s.engine.CreateDraft(ctx, p.Project, p.Kind, p.Title)
	if err != nil {
		return nil, err
	}
	return proto.DraftResult{Draft: d}, nil
}

func (s *Server) draftGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftIDParams](raw)
	if err != nil {
		return nil, err
	}
	d, err := s.engine.Draft(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return proto.DraftResult{Draft: d}, nil
}

func (s *Server) draftList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftListParams](raw)
	if err != nil {
		return nil, err
	}
	drafts, err := s.engine.ListDrafts(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.DraftListResult{Drafts: drafts}, nil
}

func (s *Server) draftSave(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftSaveParams](raw)
	if err != nil {
		return nil, err
	}
	d, err := s.engine.SaveDraft(ctx, p.Draft)
	if err != nil {
		return nil, err
	}
	return proto.DraftResult{Draft: d}, nil
}

func (s *Server) draftDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftIDParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteDraft(ctx, p.ID); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) draftExportDocx(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftIDParams](raw)
	if err != nil {
		return nil, err
	}
	path, err := s.engine.ExportDraftDocx(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return proto.DraftExportResult{Path: path}, nil
}

func (s *Server) draftSectionAI(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftSectionAIParams](raw)
	if err != nil {
		return nil, err
	}
	refs := make([]ai.PriorArtRef, 0, len(p.PriorArt))
	for _, r := range p.PriorArt {
		refs = append(refs, ai.PriorArtRef{
			Number: r.Number, Title: r.Title, Assignee: r.Assignee,
			Abstract: r.Abstract, Relevant: r.Relevant,
		})
	}
	res, err := s.engine.DraftSection(ctx, engine.DraftAIInput{
		Draft:          p.Draft,
		SectionOrdinal: p.SectionOrdinal,
		Instruction:    p.Instruction,
		Notes:          p.Notes,
		Pinned:         p.Pinned,
		PriorArt:       refs,
	})
	if err != nil {
		return nil, err
	}
	return proto.DraftSectionAIResult{
		Text:     res.Text,
		Problems: res.Problems,
		Provider: res.Provider,
		Model:    res.Model,
	}, nil
}

func (s *Server) draftSnapshot(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftSnapshotParams](raw)
	if err != nil {
		return nil, err
	}
	res, err := s.engine.SnapshotDraftResponse(ctx, p.Draft, p.Kind, p.Label)
	if err != nil {
		return nil, err
	}
	return proto.DraftSnapshotResult{
		Revision:     res.Revision,
		ClaimsPath:   res.ClaimsDoc.BlobPath,
		ResponsePath: res.ResponseDoc.BlobPath,
	}, nil
}

func (s *Server) draftRevisionList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftRevisionListParams](raw)
	if err != nil {
		return nil, err
	}
	revs, err := s.engine.ListDraftRevisions(ctx, p.Draft)
	if err != nil {
		return nil, err
	}
	return proto.DraftRevisionListResult{Revisions: revs}, nil
}

func (s *Server) draftRevisionGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftRevisionIDParams](raw)
	if err != nil {
		return nil, err
	}
	rev, err := s.engine.DraftRevision(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return proto.DraftRevisionResult{Revision: rev}, nil
}

func (s *Server) draftRestore(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftRestoreParams](raw)
	if err != nil {
		return nil, err
	}
	d, err := s.engine.RestoreDraftRevision(ctx, p.Draft, p.Revision)
	if err != nil {
		return nil, err
	}
	return proto.DraftResult{Draft: d}, nil
}

// provisionalCoverSheetPreviewFilePattern names the temp file a cover-sheet preview is
// written to (one printf verb for the project token).
const provisionalCoverSheetPreviewFilePattern = "patentmine-sb16-%s.pdf"

func (s *Server) provisionalCoverSheetGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ProvisionalCoverSheetGetParams](raw)
	if err != nil {
		return nil, err
	}
	cs, err := s.engine.ProvisionalCoverSheet(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.ProvisionalCoverSheetResult{ProvisionalCoverSheet: cs}, nil
}

func (s *Server) provisionalCoverSheetSave(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ProvisionalCoverSheetSaveParams](raw)
	if err != nil {
		return nil, err
	}
	cs, err := s.engine.SaveProvisionalCoverSheet(ctx, p.ProvisionalCoverSheet)
	if err != nil {
		return nil, err
	}
	return proto.ProvisionalCoverSheetResult{ProvisionalCoverSheet: cs}, nil
}

func (s *Server) provisionalCoverSheetPreview(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ProvisionalCoverSheetGetParams](raw)
	if err != nil {
		return nil, err
	}
	pdf, err := s.engine.PreviewProvisionalCoverSheetPDF(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	// Client and daemon share a filesystem; write the preview to a temp path the
	// client can open rather than shipping the bytes over the wire.
	path := filepath.Join(os.TempDir(), fmt.Sprintf(provisionalCoverSheetPreviewFilePattern, sanitizeForFile(string(p.Project))))
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		return nil, fmt.Errorf("rpc: write cover sheet preview: %w", err)
	}
	return proto.ProvisionalCoverSheetPreviewResult{Path: path}, nil
}

func (s *Server) provisionalCoverSheetApprove(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ProvisionalCoverSheetGetParams](raw)
	if err != nil {
		return nil, err
	}
	res, err := s.engine.ApproveProvisionalCoverSheetPDF(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.ProvisionalCoverSheetApproveResult{
		ProvisionalCoverSheet: res.ProvisionalCoverSheet,
		Path:                  res.Path,
		Missing:               res.Missing,
	}, nil
}

// sanitizeForFile reduces an id to a filesystem-safe token for temp file names.
func sanitizeForFile(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "project"
	}
	return b.String()
}

func (s *Server) officeActionImport(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionImportParams](raw)
	if err != nil {
		return nil, err
	}
	mailDate, err := parseFlexDate(p.MailDate)
	if err != nil {
		return nil, fmt.Errorf("%w: mail_date: %v", ErrBadParams, err)
	}
	oa, err := s.engine.ImportOfficeAction(ctx, engine.ImportOfficeActionInput{
		Project:           p.Project,
		Name:              p.Name,
		ApplicationNumber: p.ApplicationNumber,
		MailDate:          mailDate,
		Type:              p.Type,
		Examiner:          p.Examiner,
		ArtUnit:           p.ArtUnit,
		SourcePath:        p.SourcePath,
		ExtractedText:     p.ExtractedText,
		Source:            p.Source,
	})
	if err != nil {
		return nil, err
	}
	return proto.OfficeActionResult{OfficeAction: oa}, nil
}

func (s *Server) officeActionList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionListParams](raw)
	if err != nil {
		return nil, err
	}
	list, err := s.engine.ListOfficeActions(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	billable, err := s.engine.BillableTimeByOfficeAction(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.OfficeActionListResult{OfficeActions: list, BillableSecs: billable}, nil
}

func (s *Server) officeActionGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionIDParams](raw)
	if err != nil {
		return nil, err
	}
	oa, err := s.engine.OfficeAction(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return proto.OfficeActionResult{OfficeAction: oa}, nil
}

func (s *Server) officeActionOpen(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionIDParams](raw)
	if err != nil {
		return nil, err
	}
	oa, err := s.engine.OpenOfficeAction(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return proto.OfficeActionResult{OfficeAction: oa}, nil
}

func (s *Server) officeActionSaveNotes(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionSaveNotesParams](raw)
	if err != nil {
		return nil, err
	}
	oa, err := s.engine.SaveOfficeActionNotes(ctx, p.ID, p.Notes)
	if err != nil {
		return nil, err
	}
	return proto.OfficeActionResult{OfficeAction: oa}, nil
}

func (s *Server) officeActionUpdate(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionUpdateParams](raw)
	if err != nil {
		return nil, err
	}
	mailDate, err := parseFlexDate(p.MailDate)
	if err != nil {
		return nil, fmt.Errorf("%w: mail_date: %v", ErrBadParams, err)
	}
	mailDateStr := mailDate.Format(domain.DateLayout)

	oa, err := s.engine.UpdateOfficeActionMeta(ctx, p.ID, p.Name, p.Examiner, mailDateStr, p.Type, p.ArtUnit, p.ApplicationNumber, p.Status)
	if err != nil {
		return nil, err
	}
	return proto.OfficeActionResult{OfficeAction: oa}, nil
}

func (s *Server) officeActionDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionDeleteParams](raw)
	if err != nil {
		return nil, err
	}
	oa, err := s.engine.DeleteOfficeAction(ctx, p.ID, p.DeleteFiles)
	if err != nil {
		return nil, err
	}
	return proto.OfficeActionResult{OfficeAction: oa}, nil
}

func (s *Server) matterDocumentImport(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterDocumentImportParams](raw)
	if err != nil {
		return nil, err
	}
	doc, err := s.engine.ImportMatterDocument(ctx, engine.ImportMatterDocumentInput{
		Project:         p.Project,
		OfficeActionID:  p.OfficeActionID,
		OfficeActionIDs: p.OfficeActionIDs,
		SourcePath:      p.SourcePath,
		Kind:            p.Kind,
		Origin:          p.Origin,
		Stage:           p.Stage,
		DisplayName:     p.DisplayName,
		Tags:            p.Tags,
	})
	if err != nil {
		return nil, err
	}
	return proto.MatterDocumentResult{Document: doc}, nil
}

func (s *Server) matterDocumentSetMeta(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterDocumentMetaParams](raw)
	if err != nil {
		return nil, err
	}
	doc, err := s.engine.SetMatterDocumentMeta(ctx, p.ID, p.Origin, p.Stage)
	if err != nil {
		return nil, err
	}
	return proto.MatterDocumentResult{Document: doc}, nil
}

func (s *Server) matterDocumentList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterDocumentListParams](raw)
	if err != nil {
		return nil, err
	}
	docs, err := s.engine.ListMatterDocuments(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.MatterDocumentListResult{Documents: docs}, nil
}

func (s *Server) matterDocumentRename(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterDocumentRenameParams](raw)
	if err != nil {
		return nil, err
	}
	doc, err := s.engine.RenameMatterDocument(ctx, p.ID, p.Name)
	if err != nil {
		return nil, err
	}
	return proto.MatterDocumentResult{Document: doc}, nil
}

func (s *Server) matterDocumentDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterDocumentIDParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteMatterDocument(ctx, p.ID); err != nil {
		return nil, err
	}
	return proto.MatterDocumentResult{}, nil
}

func (s *Server) matterDocumentExtract(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterDocumentIDParams](raw)
	if err != nil {
		return nil, err
	}
	doc, err := s.engine.ExtractDocumentText(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return proto.MatterDocumentResult{Document: doc}, nil
}

func (s *Server) matterDocumentTag(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterDocumentTagParams](raw)
	if err != nil {
		return nil, err
	}
	doc, err := s.engine.TagMatterDocument(ctx, p.ID, p.Tag)
	if err != nil {
		return nil, err
	}
	return proto.MatterDocumentResult{Document: doc}, nil
}

func (s *Server) matterDocumentUntag(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterDocumentTagParams](raw)
	if err != nil {
		return nil, err
	}
	doc, err := s.engine.UntagMatterDocument(ctx, p.ID, p.Tag)
	if err != nil {
		return nil, err
	}
	return proto.MatterDocumentResult{Document: doc}, nil
}

func (s *Server) officeActionTag(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionTagParams](raw)
	if err != nil {
		return nil, err
	}
	oa, err := s.engine.TagOfficeAction(ctx, p.ID, p.Tag)
	if err != nil {
		return nil, err
	}
	return proto.OfficeActionResult{OfficeAction: oa}, nil
}

func (s *Server) officeActionUntag(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionTagParams](raw)
	if err != nil {
		return nil, err
	}
	oa, err := s.engine.UntagOfficeAction(ctx, p.ID, p.Tag)
	if err != nil {
		return nil, err
	}
	return proto.OfficeActionResult{OfficeAction: oa}, nil
}

func (s *Server) officeActionAssignPatents(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionAssignPatentsParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.AssignPatentsToOfficeAction(ctx, p.ID, p.Patents)
}

func (s *Server) officeActionReleasePatents(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionReleasePatentsParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.ReleasePatentsFromOfficeAction(ctx, p.ID, p.Patents)
}

func (s *Server) officeActionReviewStatus(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionReviewStatusParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.SetOfficeActionReviewStatus(ctx, p.ID, p.Patent, p.Status)
}

func (s *Server) officeActionPatents(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionPatentsParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.OfficeActionPatents(ctx, p.ID)
}

func (s *Server) officeActionCopyPatents(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionPatentsParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.CopyOfficeActionPatentsFromPrevious(ctx, p.ID)
}

func (s *Server) patentOfficeActions(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentOfficeActionsParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.PatentOfficeActions(ctx, p.Patents)
}

func (s *Server) matterDocumentOpen(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterDocumentIDParams](raw)
	if err != nil {
		return nil, err
	}
	doc, err := s.engine.OpenMatterDocument(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return proto.MatterDocumentResult{Document: doc}, nil
}

func (s *Server) matterDocumentAssign(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterDocumentOfficeActionParams](raw)
	if err != nil {
		return nil, err
	}
	doc, err := s.engine.AssignMatterDocumentOfficeAction(ctx, p.ID, p.OfficeActionID)
	if err != nil {
		return nil, err
	}
	return proto.MatterDocumentResult{Document: doc}, nil
}

func (s *Server) matterDocumentUnassign(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterDocumentOfficeActionParams](raw)
	if err != nil {
		return nil, err
	}
	doc, err := s.engine.UnassignMatterDocumentOfficeAction(ctx, p.ID, p.OfficeActionID)
	if err != nil {
		return nil, err
	}
	return proto.MatterDocumentResult{Document: doc}, nil
}

func (s *Server) matterEventAdd(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterEventAddParams](raw)
	if err != nil {
		return nil, err
	}
	occurred, err := parseFlexDate(p.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("%w: occurred_at: %v", ErrBadParams, err)
	}
	due, err := parseFlexDate(p.DueAt)
	if err != nil {
		return nil, fmt.Errorf("%w: due_at: %v", ErrBadParams, err)
	}
	ev, err := s.engine.AddMatterEvent(ctx, engine.AddMatterEventInput{
		Project:        p.Project,
		OfficeActionID: p.OfficeActionID,
		Kind:           p.Kind,
		Party:          p.Party,
		OccurredAt:     occurred,
		DueAt:          due,
		Summary:        p.Summary,
		Comment:        p.Comment,
	})
	if err != nil {
		return nil, err
	}
	return proto.MatterEventResult{Event: ev}, nil
}

func (s *Server) matterEventList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterEventListParams](raw)
	if err != nil {
		return nil, err
	}
	events, err := s.engine.ListMatterEvents(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.MatterEventListResult{Events: events}, nil
}

func (s *Server) matterEventDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MatterEventIDParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteMatterEvent(ctx, p.ID); err != nil {
		return nil, err
	}
	return proto.MatterEventResult{}, nil
}

func (s *Server) timeLog(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TimeLogParams](raw)
	if err != nil {
		return nil, err
	}
	// A typed-in entry is validated on entry; an editor-captured (auto) entry is
	// recorded unvalidated for the attorney to review.
	source, validated := domain.TimeSourceManual, true
	if p.Auto {
		source, validated = domain.TimeSourceAuto, false
	}
	entry, err := s.engine.LogTime(ctx, engine.LogTimeInput{
		Project:        p.Project,
		OfficeActionID: p.OfficeActionID,
		Activity:       p.Activity,
		Seconds:        p.Seconds,
		Source:         source,
		Validated:      validated,
		Note:           p.Note,
	})
	if err != nil {
		return nil, err
	}
	return proto.TimeEntryResult{Entry: entry}, nil
}

func (s *Server) timeList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TimeListParams](raw)
	if err != nil {
		return nil, err
	}
	entries, err := s.engine.ListTime(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.TimeListResult{Entries: entries}, nil
}

func (s *Server) timeUnvalidated(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TimeListParams](raw)
	if err != nil {
		return nil, err
	}
	entries, err := s.engine.ListUnvalidatedTime(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.TimeListResult{Entries: entries}, nil
}

func (s *Server) timeUpdate(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TimeUpdateParams](raw)
	if err != nil {
		return nil, err
	}
	entry, err := s.engine.UpdateTimeEntry(ctx, p.ID, p.Seconds, p.Activity, p.Note, p.Validated)
	if err != nil {
		return nil, err
	}
	return proto.TimeEntryResult{Entry: entry}, nil
}

func (s *Server) timeDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TimeIDParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteTimeEntry(ctx, p.ID); err != nil {
		return nil, err
	}
	return proto.TimeEntryResult{}, nil
}

func (s *Server) timeSummary(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TimeListParams](raw)
	if err != nil {
		return nil, err
	}
	timeSum, err := s.engine.TimeSummary(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	aiSum, err := s.engine.AIUsageSummary(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.TimeSummaryResult{Time: timeSum, AI: aiSum}, nil
}

func (s *Server) deadlineList(ctx context.Context, _ json.RawMessage) (any, error) {
	deadlines, err := s.engine.ListDeadlines(ctx)
	if err != nil {
		return nil, err
	}
	return proto.DeadlineListResult{Deadlines: deadlines}, nil
}

func (s *Server) trackRenewals(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TrackRenewalsParams](raw)
	if err != nil {
		return nil, err
	}
	deadlines, err := s.engine.TrackRenewals(ctx, p.PatentNumber, p.EntitySize)
	if err != nil {
		return nil, err
	}
	return proto.DeadlineListResult{Deadlines: deadlines}, nil
}

func (s *Server) untrackRenewals(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.UntrackRenewalsParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.UntrackRenewals(ctx, p.PatentNumber); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func (s *Server) deadlineSetStatus(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DeadlineStatusParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.SetDeadlineStatus(ctx, p.ID, p.Status); err != nil {
		return nil, err
	}
	return proto.DeadlineListResult{}, nil
}

func (s *Server) deadlineRemind(ctx context.Context, _ json.RawMessage) (any, error) {
	sent, err := s.engine.SendDueReminders(ctx)
	if err != nil {
		return nil, err
	}
	return proto.DeadlineRemindResult{Sent: sent}, nil
}

func (s *Server) validationList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentValidationParams](raw)
	if err != nil {
		return nil, err
	}
	validations, err := s.engine.PatentValidations(ctx, p.PatentNumber)
	if err != nil {
		return nil, err
	}
	return proto.PatentValidationResult{Validations: validations}, nil
}

func (s *Server) validationSet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentValidationSetParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.SetPatentValidation(ctx, p.PatentNumber, p.Country, p.Status); err != nil {
		return nil, err
	}
	validations, err := s.engine.PatentValidations(ctx, p.PatentNumber)
	if err != nil {
		return nil, err
	}
	return proto.PatentValidationResult{Validations: validations}, nil
}

func (s *Server) validationFetchEPO(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentValidationParams](raw)
	if err != nil {
		return nil, err
	}
	validations, events, err := s.engine.FetchEPOPatentValidations(ctx, p.PatentNumber)
	if err != nil {
		return nil, err
	}
	return proto.PatentValidationResult{Validations: validations, Events: events}, nil
}

func (s *Server) projectSetMatterType(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ProjectSetMatterTypeParams](raw)
	if err != nil {
		return nil, err
	}
	proj, err := s.engine.SetMatterType(ctx, p.Project, p.MatterType)
	if err != nil {
		return nil, err
	}
	return proto.ProjectResult{Project: proj}, nil
}

// parseFlexDate accepts either an RFC3339 timestamp or a YYYY-MM-DD date, and
// returns the zero time for an empty string.
func parseFlexDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(domain.DateLayout, s)
}

func (s *Server) conflictFlag(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ConflictFlagParams](raw)
	if err != nil {
		return nil, err
	}
	c, err := s.engine.FlagConflict(ctx, engine.FlagConflictInput{
		Project:    p.Project,
		Record:     p.Record,
		Against:    p.Against,
		DocumentID: p.DocumentID,
		Reason:     p.Reason,
		FlaggedBy:  p.FlaggedBy,
	})
	if err != nil {
		return nil, err
	}
	return proto.ConflictResult{Conflict: c}, nil
}

func (s *Server) conflictResolve(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ConflictResolveParams](raw)
	if err != nil {
		return nil, err
	}
	c, err := s.engine.ResolveConflict(ctx, p.ID, p.Status)
	if err != nil {
		return nil, err
	}
	return proto.ConflictResult{Conflict: c}, nil
}

func (s *Server) conflictList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ConflictListParams](raw)
	if err != nil {
		return nil, err
	}
	conflicts, err := s.engine.ListConflicts(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.ConflictListResult{Conflicts: conflicts}, nil
}

func (s *Server) conflictDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ConflictIDParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteConflict(ctx, p.ID); err != nil {
		return nil, err
	}
	return proto.ConflictResult{}, nil
}
