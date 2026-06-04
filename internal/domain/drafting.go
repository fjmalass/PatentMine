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
	Notes      string    `json:"notes,omitempty"`
	Source     string    `json:"source,omitempty"`
	ImportedAt time.Time `json:"imported_at"`
}
