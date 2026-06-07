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

// RevisionKind records why a draft revision was captured, so the history reads
// as a prosecution audit trail: a machine generation, a human checkpoint, an
// export, or the version actually filed.
type RevisionKind string

const (
	RevisionAI     RevisionKind = "ai"     // a fresh AI generation of claims/remarks
	RevisionManual RevisionKind = "manual" // a human-saved checkpoint
	RevisionExport RevisionKind = "export" // captured at an export/render
	RevisionFiled  RevisionKind = "filed"  // the version filed with the USPTO
)

// Valid reports whether the RevisionKind is a known value.
func (k RevisionKind) Valid() bool {
	switch k {
	case RevisionAI, RevisionManual, RevisionExport, RevisionFiled:
		return true
	default:
		return false
	}
}

// Label returns the human-readable name of a revision kind for the TUI.
func (k RevisionKind) Label() string {
	switch k {
	case RevisionAI:
		return "AI draft"
	case RevisionManual:
		return "Checkpoint"
	case RevisionExport:
		return "Export"
	case RevisionFiled:
		return "Filed"
	default:
		return string(k)
	}
}

// DraftRevision is one immutable snapshot of a draft, captured each time its
// new_claims.md / response.md are (re)generated, checkpointed, exported, or
// filed. The live Draft is the editable head; revisions are the append-only
// history behind it — the audit trail of what the AI produced versus what the
// attorney changed versus what was filed. The structured Sections/Claims are
// stored so a revision can be re-rendered or restored exactly, and the rendered
// ClaimsMarkdown/ResponseMarkdown are stored so the exact human-readable text is
// reproducible even without re-rendering. GitCommit, when set, points at the
// commit in the per-matter git mirror that holds the same rendered files.
type DraftRevision struct {
	ID               string         `json:"id"`
	Draft            DraftID        `json:"draft"`
	Revno            int            `json:"revno"`
	Label            string         `json:"label,omitempty"`
	Kind             RevisionKind   `json:"kind"`
	Sections         []DraftSection `json:"sections,omitempty"`
	Claims           []DraftClaim   `json:"claims,omitempty"`
	ClaimsMarkdown   string         `json:"claims_markdown,omitempty"`
	ResponseMarkdown string         `json:"response_markdown,omitempty"`
	Provider         string         `json:"provider,omitempty"`
	Model            string         `json:"model,omitempty"`
	GitCommit        string         `json:"git_commit,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
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
	Status OAStatus `json:"status,omitempty"`
	// StatusChangedAt is when Status last changed. It is seeded at import (when
	// the action enters the open state) and bumped on every later transition, so
	// the UI can show how long an action has sat in its current status.
	StatusChangedAt time.Time `json:"status_changed_at,omitempty"`
	Source          string    `json:"source,omitempty"`
	ImportedAt      time.Time `json:"imported_at"`
	Name            string    `json:"name,omitempty"`
	LastOpenedAt    time.Time `json:"last_opened_at,omitempty"`
	// Tags are the project-taxonomy labels assigned to this office action. Like a
	// matter document, an action is tagged independently within its project; the
	// tags are loaded when an action is read and never written through SaveOfficeAction.
	Tags []Tag `json:"tags,omitempty"`
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
	// MatterDocCoversheet is a filed cover/transmittal sheet — e.g. the provisional
	// application cover sheet (PTO/SB/16) rendered from a ProvisionalCoverSheet record.
	MatterDocCoversheet MatterDocKind = "coversheet"
	// MatterDocCorrespondence is an email/letter/text artifact exchanged during
	// prosecution. The communication event itself is a MatterEvent; an attached
	// file or saved text is filed as a correspondence document.
	MatterDocCorrespondence MatterDocKind = "correspondence"
	MatterDocOther          MatterDocKind = "other" // anything else in the matter
)

// Valid reports whether the MatterDocKind is a known value.
func (k MatterDocKind) Valid() bool {
	switch k {
	case MatterDocOA, MatterDocResponse, MatterDocReference, MatterDocAmendment,
		MatterDocSpec, MatterDocDrawings, MatterDocIDS, MatterDocCoversheet, MatterDocCorrespondence, MatterDocOther:
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
	case MatterDocCoversheet:
		return "Cover Sheet"
	case MatterDocCorrespondence:
		return "Correspondence"
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

// DocOrigin records who produced a matter document, or where it came from. It is
// orthogonal to MatterDocKind (what the document is) and DocStage (where it is in
// its lifecycle). It is deliberately named "origin" — not "provenance" — to stay
// distinct from the patent Provenance display, and applies only to matter
// documents, never to the patents loaded into a project.
type DocOrigin string

const (
	OriginOffice     DocOrigin = "office"      // the patent office (USPTO): examiner letters, official notices
	OriginFirm       DocOrigin = "firm"        // our side: attorney/agent drafts, responses, amendments
	OriginClient     DocOrigin = "client"      // supplied by the client
	OriginThirdParty DocOrigin = "third_party" // a third party: cited prior art, opposing submissions
)

// DocOrigins lists every origin in canonical cycle order, for iteration and for
// the TUI's cycle-through-values control.
var DocOrigins = []DocOrigin{OriginOffice, OriginFirm, OriginClient, OriginThirdParty}

// Valid reports whether the DocOrigin is a known value.
func (o DocOrigin) Valid() bool {
	switch o {
	case OriginOffice, OriginFirm, OriginClient, OriginThirdParty:
		return true
	default:
		return false
	}
}

// Cycle returns the next origin after o (wrapping), advancing by delta steps. An
// unset/unknown origin starts the cycle at the first value.
func (o DocOrigin) Cycle(delta int) DocOrigin {
	return DocOrigin(cycleEnum(string(o), docOriginStrings(), delta))
}

// Label returns the human-readable origin for the TUI.
func (o DocOrigin) Label() string {
	switch o {
	case OriginOffice:
		return "Patent Office"
	case OriginFirm:
		return "Our Firm"
	case OriginClient:
		return "Client"
	case OriginThirdParty:
		return "Third Party"
	default:
		return string(o)
	}
}

// ParseDocOrigin converts a string into a DocOrigin. The empty string is
// accepted and returns an empty origin (callers infer a default via
// InferOriginStage), so an unspecified import is still recorded.
func ParseDocOrigin(s string) (DocOrigin, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "", nil
	}
	o := DocOrigin(s)
	if !o.Valid() {
		return "", fmt.Errorf("domain: unknown document origin %q", s)
	}
	return o, nil
}

// DocStage records where a matter document sits in its lifecycle, from arrival to
// filing. It is deliberately named "stage" — not "review_state" — to stay
// distinct from the per-patent ReviewState, which means something else entirely
// (patent triage within a project).
type DocStage string

const (
	StageReceived    DocStage = "received"     // arrived as-is from outside (office, third party); not ours to draft
	StageDraft       DocStage = "draft"        // being worked on
	StageUnderReview DocStage = "under_review" // awaiting human review/approval
	StageApproved    DocStage = "approved"     // reviewed and ready to file
	StageFiled       DocStage = "filed"        // submitted to the patent office
)

// DocStages lists every stage in canonical lifecycle order, for iteration and
// for the TUI's cycle-through-values control.
var DocStages = []DocStage{StageReceived, StageDraft, StageUnderReview, StageApproved, StageFiled}

// Valid reports whether the DocStage is a known value.
func (s DocStage) Valid() bool {
	switch s {
	case StageReceived, StageDraft, StageUnderReview, StageApproved, StageFiled:
		return true
	default:
		return false
	}
}

// Cycle returns the next stage after s (wrapping), advancing by delta steps. An
// unset/unknown stage starts the cycle at the first value.
func (s DocStage) Cycle(delta int) DocStage {
	return DocStage(cycleEnum(string(s), docStageStrings(), delta))
}

// Label returns the human-readable stage for the TUI.
func (s DocStage) Label() string {
	switch s {
	case StageReceived:
		return "Received"
	case StageDraft:
		return "Draft"
	case StageUnderReview:
		return "Under Review"
	case StageApproved:
		return "Approved"
	case StageFiled:
		return "Filed"
	default:
		return string(s)
	}
}

// ParseDocStage converts a string into a DocStage. The empty string is accepted
// and returns an empty stage (callers infer a default via InferOriginStage).
func ParseDocStage(s string) (DocStage, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "", nil
	}
	st := DocStage(s)
	if !st.Valid() {
		return "", fmt.Errorf("domain: unknown document stage %q", s)
	}
	return st, nil
}

func docOriginStrings() []string {
	out := make([]string, len(DocOrigins))
	for i, o := range DocOrigins {
		out[i] = string(o)
	}
	return out
}

func docStageStrings() []string {
	out := make([]string, len(DocStages))
	for i, s := range DocStages {
		out[i] = string(s)
	}
	return out
}

// cycleEnum returns the value delta steps after current in the ordered values
// slice, wrapping around. A current value not found in values (empty/unknown)
// is treated as position -1, so Cycle(1) lands on the first value.
func cycleEnum(current string, values []string, delta int) string {
	if len(values) == 0 {
		return current
	}
	idx := -1
	for i, v := range values {
		if v == current {
			idx = i
			break
		}
	}
	n := len(values)
	next := ((idx+delta)%n + n) % n
	return values[next]
}

// InferOriginStage derives sensible default Origin and Stage values from a
// document's Kind, for imports that leave them unset and for backfilling rows
// created before these axes existed. The mapping reflects who normally produces
// each kind and the lifecycle point at which it typically enters the matter.
func InferOriginStage(kind MatterDocKind) (DocOrigin, DocStage) {
	switch kind {
	case MatterDocOA:
		return OriginOffice, StageReceived
	case MatterDocResponse, MatterDocAmendment, MatterDocSpec, MatterDocDrawings:
		return OriginFirm, StageDraft
	case MatterDocIDS:
		return OriginFirm, StageFiled
	case MatterDocReference:
		return OriginThirdParty, StageReceived
	case MatterDocCorrespondence:
		return OriginFirm, StageReceived
	default:
		return OriginFirm, StageReceived
	}
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
	// Origin records who produced the document / where it came from (the patent
	// office, our firm, the client, a third party) — orthogonal to Kind. Stage
	// records where it sits in its lifecycle (received → draft → under_review →
	// approved → filed). Both are deliberately distinct from the patent
	// ReviewState/Provenance so the two never get confused (see [DocOrigin]).
	Origin        DocOrigin `json:"origin,omitempty"`
	Stage         DocStage  `json:"stage,omitempty"`
	DisplayName   string    `json:"display_name"`
	BlobPath      string    `json:"blob_path,omitempty"`
	BlobHash      string    `json:"blob_hash,omitempty"`
	ExtractedText string    `json:"extracted_text,omitempty"`
	Tags          []Tag     `json:"tags,omitempty"`
	AddedAt       time.Time `json:"added_at"`
	LastOpenedAt  time.Time `json:"last_opened_at,omitempty"`
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
