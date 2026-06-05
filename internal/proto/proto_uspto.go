// USPTO, assignment/assignee, source-reconciliation and lookup payloads.
package proto

import (
	"fmt"
	"patentmine/internal/domain"
	"time"
)

// USPTOGrantBodyParams identifies the (patent, kind) whose parsed body should
// be returned. Kind defaults to grant; the daemon falls back to pgpub when no
// grant body has been ingested yet.
type USPTOGrantBodyParams struct {
	Number domain.PatentNumber `json:"number"`
	Kind   USPTOXMLKind        `json:"kind,omitempty"`
}

// USPTOGrantBodyResult carries the body parsed from XML, when one has been
// ingested. Present is false when nothing has been parsed for this patent.
// SourcePath is the local XML file the body was generated from (incl. directory)
// and Kind reports which document (grant/pgpub) it came from.
type USPTOGrantBodyResult struct {
	Present    bool                  `json:"present"`
	Body       domain.USPTOGrantBody `json:"body,omitempty"`
	SourcePath string                `json:"source_path,omitempty"`
	Kind       USPTOXMLKind          `json:"kind,omitempty"`
}

// USPTOXMLKind selects which XML document to fetch for a USPTO application.
type USPTOXMLKind string

const (
	USPTOXMLKindPGPub USPTOXMLKind = "pgpub"
	USPTOXMLKindGrant USPTOXMLKind = "grant"
)

// USPTOFetchXMLParams identifies which XML to download for a patent.
type USPTOFetchXMLParams struct {
	Number domain.PatentNumber `json:"number"`
	Kind   USPTOXMLKind        `json:"kind"`
}

// USPTOFetchXMLResult reports the local file written and how many bytes the
// upstream returned (after unzip, when applicable). Cached is true when the
// XML was served from the local patents dir without re-hitting the API.
// DownloadCount is the total number of times this (application, kind) has
// been requested, including the call that produced this result.
type USPTOFetchXMLResult struct {
	LocalPath     string `json:"local_path"`
	Bytes         int64  `json:"bytes"`
	Cached        bool   `json:"cached"`
	DownloadCount int64  `json:"download_count"`
}

// USPTOFetchAssignmentsParams names the patent whose recorded assignment
// chain should be fetched from the USPTO Patent Assignment Search API.
type USPTOFetchAssignmentsParams struct {
	Number domain.PatentNumber `json:"number"`
}

// USPTOFetchAssignmentsResult reports the resolved application number and
// counts saved. Existing assignments for the application are replaced.
type USPTOFetchAssignmentsResult struct {
	ApplicationNumber string `json:"application_number"`
	Assignments       int    `json:"assignments"`
	Parties           int    `json:"parties"`
}

// USPTOAssignmentListParams names the patent whose stored assignment chain
// should be returned.
type USPTOAssignmentListParams struct {
	Number domain.PatentNumber `json:"number"`
}

// USPTOAssignmentListResult carries the persisted chain in document order.
type USPTOAssignmentListResult struct {
	ApplicationNumber string                   `json:"application_number"`
	Assignments       []domain.USPTOAssignment `json:"assignments"`
}

// USPTOFetchAssignmentsBatchParams drives a batch assignment fetch over an
// explicit list of patents and/or a project's curated members. Expired patents
// are skipped unless IncludeExpired is set (ownership is frozen post-expiry).
type USPTOFetchAssignmentsBatchParams struct {
	Numbers        []domain.PatentNumber `json:"numbers,omitempty"`
	Project        domain.ProjectID      `json:"project,omitempty"`
	IncludeExpired bool                  `json:"include_expired,omitempty"`
}

// USPTOFetchAssignmentsBatchResult aggregates a batch run. Skipped counts
// expired (or non-USPTO) patents that were not fetched; Failures carries one
// "number: reason" line per patent that errored.
type USPTOFetchAssignmentsBatchResult struct {
	Total       int      `json:"total"`
	Fetched     int      `json:"fetched"`
	Skipped     int      `json:"skipped"`
	Failed      int      `json:"failed"`
	Assignments int      `json:"assignments"`
	Parties     int      `json:"parties"`
	Failures    []string `json:"failures,omitempty"`
}

// AssigneeHistoryParams names the patent whose ownership timeline to return.
type AssigneeHistoryParams struct {
	Number domain.PatentNumber `json:"number"`
}

// AssigneeHistoryResult carries one record's ownership timeline (oldest first;
// the current owner(s) are flagged IsLatest).
type AssigneeHistoryResult struct {
	Number  domain.PatentNumber           `json:"number"`
	Entries []domain.AssigneeHistoryEntry `json:"entries"`
}

// ProjectAssigneesParams names the project to roll up.
type ProjectAssigneesParams struct {
	Project domain.ProjectID `json:"project"`
}

// USPTOExpirationCalculateParams names the patent whose statutory U.S.
// expiration date should be computed (and persisted). Refresh forces a
// re-fetch of application metadata from the USPTO ODP API.
type USPTOExpirationCalculateParams struct {
	Number    domain.PatentNumber `json:"number"`
	ProjectID string              `json:"project_id"`
	Refresh   bool                `json:"refresh,omitempty"`
}

// USPTOExpirationCalculateResult carries the computed expiration alongside the
// intermediate USPTO inputs and the Google Patents estimate for comparison.
type USPTOExpirationCalculateResult struct {
	ApplicationNumber      string `json:"application_number"`
	PatentNumber           string `json:"patent_number"`
	Title                  string `json:"title,omitempty"`
	Inventors              string `json:"inventors,omitempty"`
	FilingDate             string `json:"filing_date"`
	GrantDate              string `json:"grant_date"`
	EarliestTermFilingDate string `json:"earliest_term_filing_date"`
	// EarliestTermAppNum is the application number that owns the earliest term filing date.
	// Empty when it equals ApplicationNumber (no continuity walk needed).
	EarliestTermAppNum string `json:"earliest_term_app_num,omitempty"`
	// EarliestTermPatentNumber, EarliestTermGrantDate, EarliestTermTitle describe the
	// parent patent that contributed the earliest filing date, when known from the store.
	EarliestTermPatentNumber string `json:"earliest_term_patent_number,omitempty"`
	EarliestTermGrantDate    string `json:"earliest_term_grant_date,omitempty"`
	EarliestTermTitle        string `json:"earliest_term_title,omitempty"`
	EarliestTermInventors    string `json:"earliest_term_inventors,omitempty"`
	PatentTermAdjustmentDays int    `json:"patent_term_adjustment_days"`
	PatentTermExtensionDays  int    `json:"patent_term_extension_days"`
	TerminalDisclaimerDate   string `json:"terminal_disclaimer_date"`
	ComputedExpirationDate   string `json:"computed_expiration_date"`
	GoogleExpirationDate     string `json:"google_expiration_date"`
	ComputedAt               string `json:"computed_at,omitempty"`
}

// ComparisonLine returns a single-line match/diff string comparing the
// computed USPTO expiration date with the Google Patents expiration date.
// Returns "" when either date is empty or unparseable.
func (r USPTOExpirationCalculateResult) ComparisonLine() string {
	if r.ComputedExpirationDate == "" || r.GoogleExpirationDate == "" {
		return ""
	}
	tUSPTO, errU := time.Parse(domain.DateLayout, r.ComputedExpirationDate)
	tGoogle, errG := time.Parse(domain.DateLayout, r.GoogleExpirationDate)
	if errU != nil || errG != nil {
		return ""
	}
	diff := int(tUSPTO.Sub(tGoogle).Hours() / 24)
	if diff == 0 {
		return "✅ Computed USPTO date matches Google date."
	}
	return fmt.Sprintf("❌ Difference (USPTO - Google): %d days", diff)
}

// SourceResolveDiffsParams carries the patent and the (updated) diffs from the
// comparison overlay. The engine will apply ChosenValue to patent fields and
// stamp reconciled metadata on the diffs (Option A).
type SourceResolveDiffsParams struct {
	Number domain.PatentNumber `json:"number"`
	Diffs  []domain.SourceDiff `json:"diffs"`
}

// SourceResolveDiffsResult reports the outcome of reconciliation.
type SourceResolveDiffsResult struct {
	Resolved int    `json:"resolved"`
	Message  string `json:"message,omitempty"`
}

// SourceDiffsListParams / Result for loading diffs for the comparison overlay.
type SourceDiffsListParams struct {
	Number domain.PatentNumber `json:"number"`
}

type SourceDiffsListResult struct {
	Diffs []domain.SourceDiff `json:"diffs"`
}

// SourceBibsListParams / Result load each source's full bibliographic snapshot
// for the all-fields side-by-side comparison view.
type SourceBibsListParams struct {
	Number domain.PatentNumber `json:"number"`
}

type SourceBibsListResult struct {
	Bibs []domain.SourceBib `json:"bibs"`
}

type USPTOLookupParams struct {
	Number domain.PatentNumber `json:"number"`
}

type USPTOLookupResult struct {
	RawJSON string `json:"raw_json"`
}
