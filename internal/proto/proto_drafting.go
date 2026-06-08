// Drafting subsystem payloads: provisional / non-provisional applications and
// office-action responses, plus office-action import.
package proto

import (
	"time"

	"patentmine/internal/domain"
)

// DraftCreateParams starts a new draft for a project.
type DraftCreateParams struct {
	Project domain.ProjectID `json:"project"`
	Kind    domain.DraftKind `json:"kind"`
	Title   string           `json:"title,omitempty"`
}

// DraftIDParams identifies one draft.
type DraftIDParams struct {
	ID domain.DraftID `json:"id"`
}

// DraftListParams selects a project's drafts.
type DraftListParams struct {
	Project domain.ProjectID `json:"project"`
}

// DraftSaveParams carries the draft to persist (header, sections, claims).
type DraftSaveParams struct {
	Draft domain.Draft `json:"draft"`
}

// DraftResult carries one draft.
type DraftResult struct {
	Draft domain.Draft `json:"draft"`
}

// DraftListResult carries draft summaries for a project.
type DraftListResult struct {
	Drafts []domain.Draft `json:"drafts"`
}

// DraftExportResult reports where a rendered .docx was written.
type DraftExportResult struct {
	Path string `json:"path"`
}

// DraftSnapshotParams generates a draft's new_claims.md / response.md and records
// an immutable revision behind the editable head. Kind classifies why the
// snapshot was taken (ai / manual / export / filed); Label is an optional note.
type DraftSnapshotParams struct {
	Draft domain.DraftID      `json:"draft"`
	Kind  domain.RevisionKind `json:"kind,omitempty"`
	Label string              `json:"label,omitempty"`
}

// DraftSnapshotResult reports the recorded revision and where the two rendered
// files were written (the BlobPath of each tracked MatterDocument).
type DraftSnapshotResult struct {
	Revision     domain.DraftRevision `json:"revision"`
	ClaimsPath   string               `json:"claims_path"`
	ResponsePath string               `json:"response_path"`
}

// DraftRevisionListParams selects a draft's revision history.
type DraftRevisionListParams struct {
	Draft domain.DraftID `json:"draft"`
}

// DraftRevisionListResult carries a draft's revisions, newest first, as
// summaries (no structured/rendered bodies loaded).
type DraftRevisionListResult struct {
	Revisions []domain.DraftRevision `json:"revisions"`
}

// DraftRevisionIDParams identifies one revision.
type DraftRevisionIDParams struct {
	ID string `json:"id"`
}

// DraftRevisionResult carries one revision with its full content.
type DraftRevisionResult struct {
	Revision domain.DraftRevision `json:"revision"`
}

// DraftRestoreParams restores a draft's editable head from one of its revisions.
type DraftRestoreParams struct {
	Draft    domain.DraftID `json:"draft"`
	Revision string         `json:"revision"`
}

// PriorArtRef is a cited reference offered as grounding for AI drafting. It
// mirrors ai.PriorArtRef on the wire so the daemon can rebuild it without the
// client importing the ai package.
type PriorArtRef struct {
	Number   string `json:"number"`
	Title    string `json:"title,omitempty"`
	Assignee string `json:"assignee,omitempty"`
	Abstract string `json:"abstract,omitempty"`
	Relevant string `json:"relevant,omitempty"`
}

// DraftSectionAIParams asks the daemon to draft one section with the configured
// AI drafter, grounded in the draft's material plus the supplied references.
type DraftSectionAIParams struct {
	Draft          domain.DraftID `json:"draft"`
	SectionOrdinal int            `json:"section_ordinal"`
	Instruction    string         `json:"instruction,omitempty"`
	Notes          string         `json:"notes,omitempty"`
	Pinned         []string       `json:"pinned,omitempty"`
	PriorArt       []PriorArtRef  `json:"prior_art,omitempty"`
}

// DraftSectionAIResult is the generated section plus the mechanical guardrail
// findings the reviewer must clear before accepting it.
type DraftSectionAIResult struct {
	Text     string   `json:"text"`
	Problems []string `json:"problems,omitempty"`
	Provider string   `json:"provider"`
	Model    string   `json:"model"`
}

// OfficeActionImportParams records an office action against a project, optionally
// copying a local PDF/DOCX/spreadsheet/text file into the project's office-action store.
type OfficeActionImportParams struct {
	Project           domain.ProjectID `json:"project"`
	Name              string           `json:"name,omitempty"`
	ApplicationNumber string           `json:"application_number,omitempty"`
	MailDate          string           `json:"mail_date,omitempty"` // RFC3339 or YYYY-MM-DD
	Type              domain.OAType    `json:"type,omitempty"`
	Examiner          string           `json:"examiner,omitempty"`
	ArtUnit           string           `json:"art_unit,omitempty"`
	SourcePath        string           `json:"source_path,omitempty"`
	ExtractedText     string           `json:"extracted_text,omitempty"`
	Source            string           `json:"source,omitempty"`
}

// OfficeActionResult carries one office action.
type OfficeActionResult struct {
	OfficeAction domain.OfficeAction `json:"office_action"`
}

// OfficeActionTagParams assigns/removes one project taxonomy tag on an office
// action. It mirrors MatterDocumentTagParams.
type OfficeActionTagParams struct {
	ID  string `json:"id"`
	Tag string `json:"tag"`
}

// OfficeActionAssignPatentsParams assigns reference patents to an office action
// for review. Patents are canonical record numbers (PatentNumber, the catalog's
// row identity). Re-assigning an already-linked patent preserves its progress.
type OfficeActionAssignPatentsParams struct {
	ID      string                `json:"id"`
	Patents []domain.PatentNumber `json:"patents"`
}

// OfficeActionReleasePatentsParams removes the review assignment of the given
// patents from an office action.
type OfficeActionReleasePatentsParams struct {
	ID      string                `json:"id"`
	Patents []domain.PatentNumber `json:"patents"`
}

// OfficeActionReviewStatusParams sets the review status of one patent assigned to
// an office action.
type OfficeActionReviewStatusParams struct {
	ID     string                `json:"id"`
	Patent domain.PatentNumber   `json:"patent"`
	Status domain.OAReviewStatus `json:"status"`
}

// OfficeActionPatentsParams selects the reference patents assigned to one office
// action.
type OfficeActionPatentsParams struct {
	ID string `json:"id"`
}

// OfficeActionAssignedPatent is one reference patent assigned to an office action,
// enriched for the review list: the canonical Number (load key) plus the public
// Display number and Title, alongside the review state and dates.
type OfficeActionAssignedPatent struct {
	Number     domain.PatentNumber   `json:"number"`
	Display    domain.PatentNumber   `json:"display,omitempty"`
	Title      string                `json:"title,omitempty"`
	Status     domain.OAReviewStatus `json:"status"`
	AssignedAt time.Time             `json:"assigned_at"`
	ReviewedAt time.Time             `json:"reviewed_at,omitempty"`
}

// OfficeActionPatentsResult carries the reference patents assigned to an office
// action, newest assignment first.
type OfficeActionPatentsResult struct {
	Patents []OfficeActionAssignedPatent `json:"patents,omitempty"`
}

// PatentOfficeActionsParams asks which office actions the given patents (canonical
// record numbers) are assigned to — used to annotate the catalog / patent detail.
type PatentOfficeActionsParams struct {
	Patents []domain.PatentNumber `json:"patents"`
}

// PatentOfficeActionRef names one office action a patent is assigned to, with the
// review state, enough to label the patent list/detail without a second lookup.
type PatentOfficeActionRef struct {
	OfficeActionID string                `json:"office_action_id"`
	Name           string                `json:"name,omitempty"`
	Status         domain.OAReviewStatus `json:"status"`
	AssignedAt     time.Time             `json:"assigned_at"`
}

// PatentOfficeActionsResult maps each patent's canonical record number
// (PatentNumber.Normalized()) to the office actions it is assigned to. Patents
// with no assignment are absent.
type PatentOfficeActionsResult struct {
	ByPatent map[string][]PatentOfficeActionRef `json:"by_patent,omitempty"`
}

// OfficeActionListResult carries a project's office actions. BillableSecs is the
// per-action billable total — validated time entries summed and keyed by office
// action id — so the list can show time spent without polluting the stored
// OfficeAction with a computed field.
type OfficeActionListResult struct {
	OfficeActions []domain.OfficeAction `json:"office_actions"`
	BillableSecs  map[string]int        `json:"billable_secs,omitempty"`
}

// OfficeActionListParams selects a project's office actions.
type OfficeActionListParams struct {
	Project domain.ProjectID `json:"project"`
}

// OfficeActionIDParams identifies one office action.
type OfficeActionIDParams struct {
	ID string `json:"id"`
}

// OfficeActionDeleteParams contains parameters for deleting an office action.
type OfficeActionDeleteParams struct {
	ID          string `json:"id"`
	DeleteFiles bool   `json:"delete_files"`
}

// OfficeActionSaveNotesParams replaces the attorney notes on one office action.
type OfficeActionSaveNotesParams struct {
	ID    string `json:"id"`
	Notes string `json:"notes"`
}

// OfficeActionUpdateParams updates an existing office action's metadata. An empty
// Status leaves the current status unchanged.
type OfficeActionUpdateParams struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Examiner          string          `json:"examiner"`
	MailDate          string          `json:"mail_date"`
	Type              domain.OAType   `json:"type"`
	ArtUnit           string          `json:"art_unit"`
	ApplicationNumber string          `json:"app_number"`
	Status            domain.OAStatus `json:"status,omitempty"`
}

// MatterDocumentImportParams files a local document under a matter, optionally
// linking it to one office action. Only the path travels over RPC (client and
// daemon share a filesystem); the daemon copies the bytes itself.
type MatterDocumentImportParams struct {
	Project         domain.ProjectID     `json:"project"`
	OfficeActionID  string               `json:"office_action_id,omitempty"`
	OfficeActionIDs []string             `json:"office_action_ids,omitempty"`
	Kind            domain.MatterDocKind `json:"kind,omitempty"`
	Origin          domain.DocOrigin     `json:"origin,omitempty"`
	Stage           domain.DocStage      `json:"stage,omitempty"`
	DisplayName     string               `json:"display_name,omitempty"`
	SourcePath      string               `json:"source_path"`
	Tags            []string             `json:"tags,omitempty"`
}

// MatterDocumentResult carries one matter document.
type MatterDocumentResult struct {
	Document domain.MatterDocument `json:"document"`
}

// MatterDocumentListParams selects a matter's documents.
type MatterDocumentListParams struct {
	Project domain.ProjectID `json:"project"`
}

// MatterDocumentListResult carries a matter's documents.
type MatterDocumentListResult struct {
	Documents []domain.MatterDocument `json:"documents"`
}

// MatterDocumentRenameParams changes the display label of one document.
type MatterDocumentRenameParams struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// MatterDocumentMetaParams updates a document's tracking axes — origin (who
// produced it) and stage (lifecycle point). An empty field leaves that axis
// unchanged.
type MatterDocumentMetaParams struct {
	ID     string           `json:"id"`
	Origin domain.DocOrigin `json:"origin,omitempty"`
	Stage  domain.DocStage  `json:"stage,omitempty"`
}

// MatterDocumentIDParams identifies one matter document.
type MatterDocumentIDParams struct {
	ID string `json:"id"`
}

// MatterDocumentTagParams assigns/removes one project taxonomy tag on a
// preparation document.
type MatterDocumentTagParams struct {
	ID  string `json:"id"`
	Tag string `json:"tag"`
}

// MatterDocumentOfficeActionParams assigns/removes one office-action link on a
// preparation document.
type MatterDocumentOfficeActionParams struct {
	ID             string `json:"id"`
	OfficeActionID string `json:"office_action_id"`
}

// ProjectSetMatterTypeParams records the prosecution stage of a project. The
// updated project comes back in a ProjectResult (see proto_project.go).
type ProjectSetMatterTypeParams struct {
	Project    domain.ProjectID  `json:"project"`
	MatterType domain.MatterType `json:"matter_type"`
}

// MatterEventAddParams records one communications-log entry. OccurredAt/DueAt are
// RFC3339 or YYYY-MM-DD; an empty OccurredAt defaults to now.
type MatterEventAddParams struct {
	Project        domain.ProjectID       `json:"project"`
	OfficeActionID string                 `json:"office_action_id,omitempty"`
	Kind           domain.MatterEventKind `json:"kind,omitempty"`
	Party          string                 `json:"party,omitempty"`
	OccurredAt     string                 `json:"occurred_at,omitempty"`
	DueAt          string                 `json:"due_at,omitempty"`
	Summary        string                 `json:"summary,omitempty"`
	Comment        string                 `json:"comment,omitempty"`
}

// MatterEventResult carries one communications-log entry.
type MatterEventResult struct {
	Event domain.MatterEvent `json:"event"`
}

// MatterEventListParams selects a project's communications-log entries.
type MatterEventListParams struct {
	Project domain.ProjectID `json:"project"`
}

// MatterEventListResult carries a project's communications-log entries.
type MatterEventListResult struct {
	Events []domain.MatterEvent `json:"events"`
}

// MatterEventIDParams identifies one communications-log entry.
type MatterEventIDParams struct {
	ID string `json:"id"`
}

// TimeLogParams records one time entry. A manual entry (Auto false) is validated
// on entry — the user typed it; an Auto entry (the editor's reading/writing focus
// capture) is recorded unvalidated for the attorney to review. Seconds is the
// duration.
type TimeLogParams struct {
	Project        domain.ProjectID    `json:"project"`
	OfficeActionID string              `json:"office_action_id,omitempty"`
	Activity       domain.TimeActivity `json:"activity"`
	Seconds        int                 `json:"seconds"`
	Note           string              `json:"note,omitempty"`
	Auto           bool                `json:"auto,omitempty"`
}

// TimeEntryResult carries one time entry.
type TimeEntryResult struct {
	Entry domain.TimeEntry `json:"entry"`
}

// TimeListParams selects a project's time entries.
type TimeListParams struct {
	Project domain.ProjectID `json:"project"`
}

// TimeListResult carries a list of time entries.
type TimeListResult struct {
	Entries []domain.TimeEntry `json:"entries"`
}

// TimeUpdateParams applies corrections to one time entry and optionally validates
// it (the review-before-billing step).
type TimeUpdateParams struct {
	ID        string              `json:"id"`
	Seconds   int                 `json:"seconds,omitempty"`
	Activity  domain.TimeActivity `json:"activity,omitempty"`
	Note      string              `json:"note,omitempty"`
	Validated bool                `json:"validated"`
}

// TimeIDParams identifies one time entry.
type TimeIDParams struct {
	ID string `json:"id"`
}

// TimeSummaryResult is a matter's billing readout: recorded time by activity and
// review state, plus AI usage.
type TimeSummaryResult struct {
	Time domain.TimeSummary    `json:"time"`
	AI   domain.AIUsageSummary `json:"ai"`
}

// DeadlineListResult carries a set of deadlines (pending across the database, or
// a patent's schedule).
type DeadlineListResult struct {
	Deadlines []domain.Deadline `json:"deadlines"`
}

// TrackRenewalsParams asks the daemon to derive a granted patent's U.S.
// maintenance-fee deadlines from its grant date.
type TrackRenewalsParams struct {
	PatentNumber string `json:"patent_number"`
	EntitySize   string `json:"entity_size,omitempty"`
}

// UntrackRenewalsParams asks the daemon to stop tracking a patent's renewals.
type UntrackRenewalsParams struct {
	PatentNumber string `json:"patent_number"`
}

// DeadlineStatusParams marks one deadline done/dismissed/pending.
type DeadlineStatusParams struct {
	ID     string                `json:"id"`
	Status domain.DeadlineStatus `json:"status"`
}

// DeadlineRemindResult reports how many reminders were delivered.
type DeadlineRemindResult struct {
	Sent int `json:"sent"`
}

// ConflictFlagParams flags one record as conflicting within a matter. Record is
// the conflicting patent/application/reference number; Against and DocumentID are
// optional second endpoints; Reason is free text.
type ConflictFlagParams struct {
	Project    domain.ProjectID `json:"project"`
	Record     string           `json:"record"`
	Against    string           `json:"against,omitempty"`
	DocumentID string           `json:"document_id,omitempty"`
	Reason     string           `json:"reason,omitempty"`
	FlaggedBy  string           `json:"flagged_by,omitempty"`
}

// ConflictResolveParams changes a conflict's status (open/resolved/waived).
type ConflictResolveParams struct {
	ID     string                `json:"id"`
	Status domain.ConflictStatus `json:"status"`
}

// ConflictIDParams identifies one conflict edge.
type ConflictIDParams struct {
	ID string `json:"id"`
}

// ConflictResult carries one conflict edge.
type ConflictResult struct {
	Conflict domain.Conflict `json:"conflict"`
}

// ConflictListParams selects a project's conflicts.
type ConflictListParams struct {
	Project domain.ProjectID `json:"project"`
}

// ConflictListResult carries a project's conflicts.
type ConflictListResult struct {
	Conflicts []domain.Conflict `json:"conflicts"`
}
