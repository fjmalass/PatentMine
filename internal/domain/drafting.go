package domain

import (
	"fmt"
	"strings"
	"time"
)

// DraftID is the stable identifier of a drafted document. A distinct type keeps
// it from being mixed up with project ids or patent numbers.
type DraftID string

// DraftKind enumerates the kinds of document the drafting subsystem produces.
// Provisional and non-provisional are first filings authored from scratch; an
// office-action response is authored against an existing application's rejections.
// All three are project-scoped, section-structured documents rendered to .docx,
// which is why they share one model.
type DraftKind string

const (
	// DraftProvisional is a provisional patent application (35 U.S.C. 111(b)):
	// a specification and drawings, no claims required.
	DraftProvisional DraftKind = "provisional"
	// DraftNonprovisional is a first non-provisional (utility) application: a
	// full specification plus at least one claim.
	DraftNonprovisional DraftKind = "nonprovisional"
	// DraftOAResponse is a response/amendment to an Office Action.
	DraftOAResponse DraftKind = "oa_response"
)

// Valid reports whether the DraftKind is a known value.
func (k DraftKind) Valid() bool {
	switch k {
	case DraftProvisional, DraftNonprovisional, DraftOAResponse:
		return true
	default:
		return false
	}
}

// NeedsClaims reports whether a draft of this kind is expected to carry claims.
// A provisional needs none; the other kinds do.
func (k DraftKind) NeedsClaims() bool {
	return k == DraftNonprovisional || k == DraftOAResponse
}

// ParseDraftKind converts a string into a DraftKind.
func ParseDraftKind(s string) (DraftKind, error) {
	k := DraftKind(strings.TrimSpace(strings.ToLower(s)))
	if !k.Valid() {
		return "", fmt.Errorf("domain: unknown draft kind %q", s)
	}
	return k, nil
}

// DraftStatus is the workflow state of a draft. A draft is editable until it is
// finalized; filing is recorded for provenance.
type DraftStatus string

const (
	DraftStatusDraft DraftStatus = "draft"
	DraftStatusFinal DraftStatus = "final"
	DraftStatusFiled DraftStatus = "filed"
)

// Valid reports whether the DraftStatus is a known value.
func (s DraftStatus) Valid() bool {
	switch s {
	case DraftStatusDraft, DraftStatusFinal, DraftStatusFiled:
		return true
	default:
		return false
	}
}

// SectionKind names the role of one prose section of a draft. Claims are not a
// SectionKind: they are structured DraftClaim rows so the renderer can compute
// amendment markup deterministically.
type SectionKind string

const (
	SectionField               SectionKind = "field"
	SectionBackground          SectionKind = "background"
	SectionSummary             SectionKind = "summary"
	SectionBriefDrawings       SectionKind = "brief_description_of_drawings"
	SectionDetailedDescription SectionKind = "detailed_description"
	SectionAbstract            SectionKind = "abstract"
	// SectionRemarks holds the arguments traversing an Office Action's
	// rejections in a response.
	SectionRemarks SectionKind = "remarks"
)

// Heading returns the conventional document heading for a section kind.
func (s SectionKind) Heading() string {
	switch s {
	case SectionField:
		return "FIELD OF THE INVENTION"
	case SectionBackground:
		return "BACKGROUND"
	case SectionSummary:
		return "SUMMARY"
	case SectionBriefDrawings:
		return "BRIEF DESCRIPTION OF THE DRAWINGS"
	case SectionDetailedDescription:
		return "DETAILED DESCRIPTION"
	case SectionAbstract:
		return "ABSTRACT"
	case SectionRemarks:
		return "REMARKS"
	default:
		return strings.ToUpper(strings.ReplaceAll(string(s), "_", " "))
	}
}

// DraftSection is one prose section of a draft. AI provenance is carried so a
// reviewer can tell machine-drafted text from human-authored text, and Pinned
// records verbatim source spans the section must reproduce exactly — the
// anti-hallucination guardrail (see ai.VerifyVerbatim).
type DraftSection struct {
	Ordinal     int         `json:"ordinal"`
	Kind        SectionKind `json:"kind"`
	Heading     string      `json:"heading"`
	Body        string      `json:"body"`
	Pinned      []string    `json:"pinned,omitempty"`
	AIProvider  string      `json:"ai_provider,omitempty"`
	AIModel     string      `json:"ai_model,omitempty"`
	GeneratedAt time.Time   `json:"generated_at"`
	HumanEdited bool        `json:"human_edited"`
}

// ClaimType distinguishes an independent claim from a dependent one.
type ClaimType string

const (
	ClaimIndependent ClaimType = "independent"
	ClaimDependent   ClaimType = "dependent"
)

// AmendmentStatus is the MPEP 714 status identifier carried by a claim in an
// amendment. For a first application every claim is Original (or New); the other
// values appear in office-action responses.
type AmendmentStatus string

const (
	AmendOriginal         AmendmentStatus = "original"
	AmendCurrentlyAmended AmendmentStatus = "currently_amended"
	AmendNew              AmendmentStatus = "new"
	AmendCancelled        AmendmentStatus = "cancelled"
	AmendWithdrawn        AmendmentStatus = "withdrawn"
)

// Valid reports whether the AmendmentStatus is a known value.
func (s AmendmentStatus) Valid() bool {
	switch s {
	case AmendOriginal, AmendCurrentlyAmended, AmendNew, AmendCancelled, AmendWithdrawn:
		return true
	default:
		return false
	}
}

// Label returns the parenthetical status identifier written next to a claim
// number in a claim listing, e.g. "(Currently Amended)".
func (s AmendmentStatus) Label() string {
	switch s {
	case AmendOriginal:
		return "(Original)"
	case AmendCurrentlyAmended:
		return "(Currently Amended)"
	case AmendNew:
		return "(New)"
	case AmendCancelled:
		return "(Cancelled)"
	case AmendWithdrawn:
		return "(Withdrawn)"
	default:
		return ""
	}
}

// ShowsMarkup reports whether a claim of this status renders insertion/deletion
// markup (underline/strikethrough). Only a currently-amended claim does.
func (s AmendmentStatus) ShowsMarkup() bool {
	return s == AmendCurrentlyAmended
}

// DraftClaim is one claim of a draft. BaseText holds the prior version when the
// claim is being amended; the renderer diffs BaseText against Text to produce
// the underline/strikethrough markup. For an original or new claim BaseText is
// empty and Text stands alone.
type DraftClaim struct {
	Ordinal   int             `json:"ordinal"`
	Number    int             `json:"number"`
	Type      ClaimType       `json:"type"`
	DependsOn int             `json:"depends_on,omitempty"`
	Status    AmendmentStatus `json:"status"`
	BaseText  string          `json:"base_text,omitempty"`
	Text      string          `json:"text"`
}

// Draft is a project-scoped, section-structured legal document rendered to
// .docx. It unifies first-application drafting (provisional / non-provisional)
// and office-action responses behind one model: the Kind selects the section
// skeleton and whether claims apply, and OfficeActionID links a response back
// to the examiner document it answers.
type Draft struct {
	ID             DraftID        `json:"id"`
	Project        ProjectID      `json:"project"`
	Kind           DraftKind      `json:"kind"`
	Title          string         `json:"title"`
	Status         DraftStatus    `json:"status"`
	OfficeActionID string         `json:"office_action_id,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	Sections       []DraftSection `json:"sections,omitempty"`
	Claims         []DraftClaim   `json:"claims,omitempty"`
}

// DefaultSections returns the conventional empty section skeleton for a draft of
// the given kind. Seeding the skeleton at creation gives the author a structured
// starting point and bounds where free-text (and AI drafting) can go — a
// deliberate constraint that keeps a machine draft on the rails.
func DefaultSections(kind DraftKind) []DraftSection {
	var kinds []SectionKind
	switch kind {
	case DraftProvisional:
		// A provisional needs a complete specification but no claims.
		kinds = []SectionKind{
			SectionField, SectionBackground, SectionSummary,
			SectionBriefDrawings, SectionDetailedDescription, SectionAbstract,
		}
	case DraftNonprovisional:
		kinds = []SectionKind{
			SectionField, SectionBackground, SectionSummary,
			SectionBriefDrawings, SectionDetailedDescription, SectionAbstract,
		}
	case DraftOAResponse:
		kinds = []SectionKind{SectionRemarks}
	default:
		return nil
	}
	out := make([]DraftSection, len(kinds))
	for i, k := range kinds {
		out[i] = DraftSection{Ordinal: i, Kind: k, Heading: k.Heading()}
	}
	return out
}

// OAType classifies an Office Action.
type OAType string

const (
	OANonFinal          OAType = "non_final"
	OAFinal             OAType = "final"
	OARestriction       OAType = "restriction"
	OAAdvisory          OAType = "advisory"
	OANoticeOfAllowance OAType = "notice_of_allowance"
)

// Valid reports whether the OAType is a known value.
func (t OAType) Valid() bool {
	switch t {
	case OANonFinal, OAFinal, OARestriction, OAAdvisory, OANoticeOfAllowance:
		return true
	default:
		return false
	}
}

// OASource records how an Office Action document arrived.
const (
	OASourceManual = "manual" // user imported the PDF from Patent Center
	OASourceODP    = "odp"    // fetched from the USPTO Open Data Portal
)

// OAStatus is the prosecution state of an office action. It governs deadline
// reminders: an open action with a future response date is reminded; a responded
// or closed one is not.
type OAStatus string

const (
	OAStatusOpen      OAStatus = "open"      // awaiting a response
	OAStatusResponded OAStatus = "responded" // a response has been filed
	OAStatusClosed    OAStatus = "closed"    // no response needed/abandoned
)

// Valid reports whether the OAStatus is a known value.
func (s OAStatus) Valid() bool {
	switch s {
	case OAStatusOpen, OAStatusResponded, OAStatusClosed:
		return true
	default:
		return false
	}
}

// ResponseDeadline returns the statutory deadline to respond to an office action
// of the given type mailed on mailDate. Most actions run a 3-month shortened
// statutory period (extendable to 6); a restriction requirement and an advisory
// action also run shorter periods in practice, but 3 months is the safe default
// the user can override. A type with no response (notice of allowance) returns
// the zero time. The returned date is a starting point — the user can edit it.
func ResponseDeadline(mailDate time.Time, t OAType) time.Time {
	if mailDate.IsZero() {
		return time.Time{}
	}
	switch t {
	case OANoticeOfAllowance:
		return time.Time{}
	case OAAdvisory:
		// An advisory action does not reset the period; default to no new date.
		return time.Time{}
	default:
		return mailDate.AddDate(0, 3, 0)
	}
}

// OfficeAction is the examiner's document that a response answers. The PDF bytes
// live on disk (BlobPath, with BlobHash for integrity); ExtractedText is the
// searchable/groundable plain text pulled from it. This mirrors the
// source_snapshot convention (pointer-to-file, metadata in the row) rather than
// storing the blob in SQLite.
type OfficeAction struct {
	ID                string    `json:"id"`
	Project           ProjectID `json:"project"`
	ApplicationNumber string    `json:"application_number,omitempty"`
	MailDate          time.Time `json:"mail_date"`
	Type              OAType    `json:"type"`
	Examiner          string    `json:"examiner,omitempty"`
	ArtUnit           string    `json:"art_unit,omitempty"`
	BlobPath          string    `json:"blob_path,omitempty"`
	BlobHash          string    `json:"blob_hash,omitempty"`
	ExtractedText     string    `json:"extracted_text,omitempty"`
	// Notes is the attorney's free-text annotation of this office action
	// (markdown), edited in the TUI's split view while reading the examiner text.
	Notes string `json:"notes,omitempty"`
	// ResponseDue is the deadline to file a response. It is seeded from the mail
	// date and type at import (see ResponseDeadline) and may be edited; the zero
	// time means no deadline applies (e.g. a notice of allowance).
	ResponseDue time.Time `json:"response_due"`
	// Status governs deadline reminders — only an open action is reminded.
	Status     OAStatus  `json:"status,omitempty"`
	Source     string    `json:"source,omitempty"`
	ImportedAt time.Time `json:"imported_at"`
}

// MatterDocKind classifies one document filed under a matter (a project). A
// prosecution thread carries more than the examiner's letter — the response that
// answers it, cited prior-art references, claim amendments — and a new
// application carries its specification, drawings, and IDS. The kind drives how a
// document is labelled and which ones a response draws from.
type MatterDocKind string

// PreparationDocKind is the user-facing name for project preparation documents.
// It aliases MatterDocKind to avoid conflicting with the patent life-stage
// Document type and to keep existing storage compatible.
type PreparationDocKind = MatterDocKind

const (
	MatterDocOA        MatterDocKind = "oa"        // an examiner's office action letter
	MatterDocResponse  MatterDocKind = "response"  // a response/amendment answering an OA
	MatterDocReference MatterDocKind = "reference" // a cited prior-art document
	MatterDocAmendment MatterDocKind = "amendment" // a standalone claim amendment paper
	MatterDocSpec      MatterDocKind = "spec"      // a specification draft/filing
	MatterDocDrawings  MatterDocKind = "drawings"  // figures/drawings
	MatterDocIDS       MatterDocKind = "ids"       // an information disclosure statement
	MatterDocOther     MatterDocKind = "other"     // anything else in the matter
)

// Valid reports whether the MatterDocKind is a known value.
func (k MatterDocKind) Valid() bool {
	switch k {
	case MatterDocOA, MatterDocResponse, MatterDocReference, MatterDocAmendment,
		MatterDocSpec, MatterDocDrawings, MatterDocIDS, MatterDocOther:
		return true
	default:
		return false
	}
}

// Label returns the human-readable name of a document kind for the TUI.
func (k MatterDocKind) Label() string {
	switch k {
	case MatterDocOA:
		return "Office Action"
	case MatterDocResponse:
		return "Response"
	case MatterDocReference:
		return "Reference"
	case MatterDocAmendment:
		return "Amendment"
	case MatterDocSpec:
		return "Specification"
	case MatterDocDrawings:
		return "Drawings"
	case MatterDocIDS:
		return "IDS"
	case MatterDocOther:
		return "Other"
	default:
		return string(k)
	}
}

// ParseMatterDocKind converts a string into a MatterDocKind, defaulting an empty
// string to MatterDocOther so an unspecified import is still recorded.
func ParseMatterDocKind(s string) (MatterDocKind, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return MatterDocOther, nil
	}
	k := MatterDocKind(s)
	if !k.Valid() {
		return "", fmt.Errorf("domain: unknown matter document kind %q", s)
	}
	return k, nil
}

// MatterDocument is one file filed under a matter (a project), optionally linked
// to a specific office action (OfficeActionID empty = matter-wide, e.g. a
// specification or a shared reference). Like the office action it follows the
// source_snapshot convention — the bytes live on disk (BlobPath, BlobHash for
// integrity) while the row holds the metadata and the extracted plain text used
// for reading and copy-into-response. DisplayName is the user-renameable label
// shown in the document list (it defaults to the imported file's base name).
type MatterDocument struct {
	ID              string        `json:"id"`
	Project         ProjectID     `json:"project"`
	OfficeActionID  string        `json:"office_action_id,omitempty"`
	OfficeActionIDs []string      `json:"office_action_ids,omitempty"`
	Kind            MatterDocKind `json:"kind"`
	DisplayName     string        `json:"display_name"`
	BlobPath        string        `json:"blob_path,omitempty"`
	BlobHash        string        `json:"blob_hash,omitempty"`
	ExtractedText   string        `json:"extracted_text,omitempty"`
	Tags            []Tag         `json:"tags,omitempty"`
	AddedAt         time.Time     `json:"added_at"`
}

// PreparationDocument is the clearer name for files used while preparing a
// response or application, including additional references. It aliases the
// existing type so older RPC/storage names keep working while user-facing code
// can use the simpler term.
type PreparationDocument = MatterDocument

// MatterEventKind classifies one entry in a matter's communications log — the
// running record of what happened during prosecution (examiner interviews,
// client emails, filings, deadlines).
type MatterEventKind string

const (
	MatterEventEmail     MatterEventKind = "email"
	MatterEventPhone     MatterEventKind = "phone"
	MatterEventInterview MatterEventKind = "interview"
	MatterEventFiled     MatterEventKind = "filed"
	MatterEventDeadline  MatterEventKind = "deadline"
	MatterEventNote      MatterEventKind = "note"
)

// Valid reports whether the MatterEventKind is a known value.
func (k MatterEventKind) Valid() bool {
	switch k {
	case MatterEventEmail, MatterEventPhone, MatterEventInterview,
		MatterEventFiled, MatterEventDeadline, MatterEventNote:
		return true
	default:
		return false
	}
}

// Label returns the human-readable name of an event kind for the TUI.
func (k MatterEventKind) Label() string {
	switch k {
	case MatterEventEmail:
		return "Email"
	case MatterEventPhone:
		return "Phone call"
	case MatterEventInterview:
		return "Interview"
	case MatterEventFiled:
		return "Filed"
	case MatterEventDeadline:
		return "Deadline"
	case MatterEventNote:
		return "Note"
	default:
		return string(k)
	}
}

// ParseMatterEventKind converts a string into a MatterEventKind, defaulting an
// empty string to MatterEventNote.
func ParseMatterEventKind(s string) (MatterEventKind, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return MatterEventNote, nil
	}
	k := MatterEventKind(s)
	if !k.Valid() {
		return "", fmt.Errorf("domain: unknown matter event kind %q", s)
	}
	return k, nil
}

// MatterEvent is one entry in a matter's communications log: a phone call, an
// email, an examiner interview, a filing, or a tracked deadline. It is
// project-scoped and optionally tied to one office action. Party records who the
// communication was with (the examiner, the client). DueAt is set for a deadline
// entry; Comment is the free-text "what happened".
type MatterEvent struct {
	ID             string          `json:"id"`
	Project        ProjectID       `json:"project"`
	OfficeActionID string          `json:"office_action_id,omitempty"`
	Kind           MatterEventKind `json:"kind"`
	Party          string          `json:"party,omitempty"`
	OccurredAt     time.Time       `json:"occurred_at"`
	DueAt          time.Time       `json:"due_at,omitempty"`
	Summary        string          `json:"summary,omitempty"`
	Comment        string          `json:"comment,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// TimeActivity classifies a unit of billable work. Reading and writing are
// captured automatically from the editor (time the source pane vs the response
// pane held focus); ai is captured from AI calls; call and admin are entered
// manually.
type TimeActivity string

const (
	TimeReading TimeActivity = "reading"
	TimeWriting TimeActivity = "writing"
	TimeAI      TimeActivity = "ai"
	TimeCall    TimeActivity = "call"
	TimeAdmin   TimeActivity = "admin"
)

// Valid reports whether the TimeActivity is a known value.
func (a TimeActivity) Valid() bool {
	switch a {
	case TimeReading, TimeWriting, TimeAI, TimeCall, TimeAdmin:
		return true
	default:
		return false
	}
}

// Label returns the human-readable name of a time activity for the TUI.
func (a TimeActivity) Label() string {
	switch a {
	case TimeReading:
		return "Reading"
	case TimeWriting:
		return "Writing"
	case TimeAI:
		return "AI"
	case TimeCall:
		return "Call"
	case TimeAdmin:
		return "Admin"
	default:
		return string(a)
	}
}

// ParseTimeActivity converts a string into a TimeActivity.
func ParseTimeActivity(s string) (TimeActivity, error) {
	a := TimeActivity(strings.TrimSpace(strings.ToLower(s)))
	if !a.Valid() {
		return "", fmt.Errorf("domain: unknown time activity %q", s)
	}
	return a, nil
}

// TimeSource records how a time entry was captured. Auto entries (editor focus,
// AI calls) need attorney validation before they are billed; manual entries are
// validated on entry.
type TimeSource string

const (
	TimeSourceAuto   TimeSource = "auto"
	TimeSourceManual TimeSource = "manual"
)

// TimeEntry is one unit of recorded work on a matter, the basis for billing.
// Auto-captured entries start unvalidated; the attorney reviews and validates
// (or corrects/deletes) them — captured time is a draft until a human confirms
// it. Rates/amounts are intentionally absent for now; only durations are tracked.
type TimeEntry struct {
	ID             string       `json:"id"`
	Project        ProjectID    `json:"project"`
	OfficeActionID string       `json:"office_action_id,omitempty"`
	Activity       TimeActivity `json:"activity"`
	Source         TimeSource   `json:"source"`
	StartedAt      time.Time    `json:"started_at,omitempty"`
	EndedAt        time.Time    `json:"ended_at,omitempty"`
	Seconds        int          `json:"seconds"`
	Validated      bool         `json:"validated"`
	ValidatedAt    time.Time    `json:"validated_at,omitempty"`
	Note           string       `json:"note,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// AIUsage records one AI call against a matter so AI work can be charged.
// Token counts are populated when the provider returns usage (see ai.Drafter /
// ai.Extractor); until then Prompts, Provider, and Model are captured.
type AIUsage struct {
	ID               string    `json:"id"`
	Project          ProjectID `json:"project"`
	OfficeActionID   string    `json:"office_action_id,omitempty"`
	DraftID          string    `json:"draft_id,omitempty"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	Prompts          int       `json:"prompts"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	CreatedAt        time.Time `json:"created_at"`
}

// TimeSummary aggregates a matter's recorded time by activity, for a billing
// readout. Seconds maps each activity to its total; Validated/Unvalidated split
// the grand total by review state.
type TimeSummary struct {
	Seconds          map[TimeActivity]int `json:"seconds"`
	ValidatedSecs    int                  `json:"validated_secs"`
	UnvalidatedSecs  int                  `json:"unvalidated_secs"`
	UnvalidatedCount int                  `json:"unvalidated_count"`
}

// AIUsageSummary aggregates a matter's AI usage for a billing readout.
type AIUsageSummary struct {
	Calls            int `json:"calls"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
