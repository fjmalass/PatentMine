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
