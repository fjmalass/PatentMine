// Drafting subsystem payloads: provisional / non-provisional applications and
// office-action responses, plus office-action import.
package proto

import "patentmine/internal/domain"

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
// copying a local PDF/text file into the project's office-action store.
type OfficeActionImportParams struct {
	Project           domain.ProjectID `json:"project"`
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

// OfficeActionListResult carries a project's office actions.
type OfficeActionListResult struct {
	OfficeActions []domain.OfficeAction `json:"office_actions"`
}

// OfficeActionListParams selects a project's office actions.
type OfficeActionListParams struct {
	Project domain.ProjectID `json:"project"`
}

// OfficeActionIDParams identifies one office action.
type OfficeActionIDParams struct {
	ID string `json:"id"`
}

// OfficeActionSaveNotesParams replaces the attorney notes on one office action.
type OfficeActionSaveNotesParams struct {
	ID    string `json:"id"`
	Notes string `json:"notes"`
}

// MatterDocumentImportParams files a local document under a matter, optionally
// linking it to one office action. Only the path travels over RPC (client and
// daemon share a filesystem); the daemon copies the bytes itself.
type MatterDocumentImportParams struct {
	Project        domain.ProjectID     `json:"project"`
	OfficeActionID string               `json:"office_action_id,omitempty"`
	Kind           domain.MatterDocKind `json:"kind,omitempty"`
	DisplayName    string               `json:"display_name,omitempty"`
	SourcePath     string               `json:"source_path"`
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

// MatterDocumentIDParams identifies one matter document.
type MatterDocumentIDParams struct {
	ID string `json:"id"`
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
