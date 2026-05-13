package domain

import (
	"fmt"
	"strings"
	"time"
)

const PatentCountryUnknown = ""

// ISO 3166-1 alpha-2 codes for major patent offices.
const (
	PatentCountryUS = "US" // United States
	PatentCountryEP = "EP" // European Patent Office
	PatentCountryWO = "WO" // WIPO / PCT
	PatentCountryJP = "JP" // Japan
	PatentCountryCN = "CN" // China
	PatentCountryKR = "KR" // South Korea
	PatentCountryDE = "DE" // Germany
	PatentCountryFR = "FR" // France
	PatentCountryGB = "GB" // United Kingdom
	PatentCountryCA = "CA" // Canada
	PatentCountryAU = "AU" // Australia
	PatentCountryIN = "IN" // India
	PatentCountryTW = "TW" // Taiwan
	PatentCountryIL = "IL" // Israel
	PatentCountrySG = "SG" // Singapore
	PatentCountryBR = "BR" // Brazil
	PatentCountryMX = "MX" // Mexico
	PatentCountryRU = "RU" // Russia
	PatentCountryMY = "MY" // Malaysia
)

var PatentCountryCodes = []string{
	PatentCountryUS, PatentCountryEP, PatentCountryWO,
	PatentCountryJP, PatentCountryCN, PatentCountryKR,
	PatentCountryDE, PatentCountryFR, PatentCountryGB,
	PatentCountryCA, PatentCountryAU, PatentCountryIN,
	PatentCountryTW, PatentCountryIL, PatentCountrySG,
	PatentCountryBR, PatentCountryMX, PatentCountryRU,
	PatentCountryMY,
}

const (
	RelationCites   = "cites"
	RelationCitedBy = "cited_by"
)

const (
	LifecycleTypeApp   = "app"
	LifecycleTypePub   = "pub"
	LifecycleTypeGrant = "grant"
	LifecycleTypeExp   = "exp"
)

const (
	FilterOpAnd = "and"
	FilterOpOr  = "or"
)

const (
	FamilyRelationContinuation = "continuation"
	FamilyRelationDivisional   = "divisional"
	FamilyRelationCIP          = "cip"
	FamilyRelationPCT          = "pct"
)

type FamilyEdge struct {
	ProjectID    string
	ParentNumber string
	ChildNumber  string
	RelationType string
	CreatedAt    time.Time
}

const (
	CitationStatusUnderReview = "under_review"
	CitationStatusStored      = "stored"
	CitationStatusIgnored     = "ignored"
	CitationStatusCached      = "cached"
)

const (
	SortColumnNumber     = "number"
	SortColumnTitle      = "title"
	SortColumnDate       = "date"
	SortColumnStatus     = "status"
	SortColumnAssignee   = "assignee"
	SortColumnInventor   = "inventor"
	SortColumnClass      = "class"
	SortColumnCPC        = "cpc"
	SortColumnExpiration = "expiration"
	SortColumnUpdated    = "updated"
	SortColumnNotes      = "notes"
	SortColumnIDS        = "ids"

	SortOrderAsc  = "asc"
	SortOrderDesc = "desc"
)

var PatentSortColumns = []string{
	SortColumnNumber,
	SortColumnTitle,
	SortColumnDate,
	SortColumnStatus,
	SortColumnAssignee,
	SortColumnInventor,
	SortColumnClass,
	SortColumnCPC,
	SortColumnExpiration,
	SortColumnUpdated,
	SortColumnNotes,
	SortColumnIDS,
}

const (
	AnalysisTypeSummary    = "summary"
	AnalysisTypeComparison = "comparison"
)

const (
	ProjectStatusActive   = "active"
	ProjectStatusArchived = "archived"
)

const (
	ProjectSummaryStatusWorkInProgress   = "work-in-progress"
	ProjectSummaryStatusProvisionalFiled = "provisional-filed"
	ProjectSummaryStatusApplicationFiled = "application-filed"
	ProjectSummaryStatusPublished        = "published"
	ProjectSummaryStatusGranted          = "granted"
)

const (
	InvoiceDirectionToFirm   = "to-firm"
	InvoiceDirectionFromFirm = "from-firm"
)

const (
	InvoiceStatusOutstanding = "outstanding"
	InvoiceStatusPaid        = "paid"
	InvoiceStatusOverdue     = "overdue"
	InvoiceStatusDisputed    = "disputed"
)

type IDSStatus string

const (
	IDSStatusPending   IDSStatus = "pending"
	IDSStatusSubmitted IDSStatus = "submitted"
	IDSStatusAccepted  IDSStatus = "accepted"
)

// USPTO document kind codes.
const (
	IDSKindCodeA1 = "A1" // published application (first publication)
	IDSKindCodeA2 = "A2" // published application (second publication)
	IDSKindCodeB1 = "B1" // patent, no pre-grant publication
	IDSKindCodeB2 = "B2" // patent, with prior pre-grant publication
	IDSKindCodeE1 = "E1" // reissue patent
	IDSKindCodeH  = "H"  // statutory invention registration
	IDSKindCodeP  = "P"  // plant patent application
	IDSKindCodeP2 = "P2" // plant patent
	IDSKindCodeS  = "S"  // design patent
)

// IDSKindCodes lists the recognized USPTO kind codes for TUI hints and validation.
var IDSKindCodes = []string{
	IDSKindCodeA1, IDSKindCodeA2,
	IDSKindCodeB1, IDSKindCodeB2,
	IDSKindCodeE1, IDSKindCodeH,
	IDSKindCodeP, IDSKindCodeP2,
	IDSKindCodeS,
}

const (
	IDSCountryUS = PatentCountryUS
	IDSCountryEP = PatentCountryEP
	IDSCountryWO = PatentCountryWO
	IDSCountryJP = PatentCountryJP
	IDSCountryCN = PatentCountryCN
	IDSCountryKR = PatentCountryKR
	IDSCountryDE = PatentCountryDE
	IDSCountryFR = PatentCountryFR
	IDSCountryGB = PatentCountryGB
	IDSCountryCA = PatentCountryCA
	IDSCountryAU = PatentCountryAU
	IDSCountryIN = PatentCountryIN
	IDSCountryTW = PatentCountryTW
	IDSCountryIL = PatentCountryIL
	IDSCountrySG = PatentCountrySG
	IDSCountryBR = PatentCountryBR
	IDSCountryMX = PatentCountryMX
	IDSCountryRU = PatentCountryRU
)

// IDSCountryCodes lists recognized country codes for TUI hints and validation.
var IDSCountryCodes = PatentCountryCodes

func PatentCountryFromNumber(number string) string {
	n := strings.ToUpper(strings.TrimSpace(number))
	if len(n) < 2 {
		return PatentCountryUnknown
	}
	code := n[:2]
	for _, valid := range PatentCountryCodes {
		if code == valid {
			return code
		}
	}
	return PatentCountryUnknown
}

type ProjectInvoice struct {
	ID            int64
	ProjectID     string
	Direction     string
	FirmName      string
	InvoiceNumber string
	Amount        string
	Currency      string
	InvoiceDate   string
	DueDate       string
	PaidDate      string
	Status        string
	Description   string
	Notes         string
	CreatedAt     time.Time
}

const (
	EventTypeProvisionalFiled  = "provisional-filed"
	EventTypeApplicationFiled  = "application-filed"
	EventTypePublication       = "publication"
	EventTypeOANonFinal        = "office-action-non-final"
	EventTypeOAFinal           = "office-action-final"
	EventTypeResponseFiled     = "response-filed"
	EventTypeRCEFiled          = "rce-filed"
	EventTypeNoticeOfAllowance = "notice-of-allowance"
	EventTypeIssueFee          = "issue-fee-paid"
	EventTypeGranted           = "patent-granted"
	EventTypeMaintenance3      = "maintenance-due-3y"
	EventTypeMaintenance7      = "maintenance-due-7y"
	EventTypeMaintenance11     = "maintenance-due-11y"
	EventTypeContinuationFiled = "continuation-filed"
	EventTypeDivisionalFiled   = "divisional-filed"
	EventTypeCIPFiled          = "cip-filed"
	EventTypeAppealFiled       = "appeal-filed"
	EventTypePTABDecision      = "ptab-decision"
	EventTypeIPRFiled          = "ipr-filed"
	EventTypeReexamRequested   = "reexam-requested"
	EventTypeExtensionFiled    = "extension-filed"
	EventTypeAbandonment       = "abandonment"
	EventTypeRevivalFiled      = "revival-filed"
)

type ProjectEvent struct {
	ID        int64
	ProjectID string
	EventType string
	EventDate string
	DueDate   string
	Reference string
	Notes     string
	CreatedAt time.Time
}

const (
	ExpirationSourceManual    = "manual"
	ExpirationSourceImported  = "imported"
	ExpirationSourceEstimated = "estimated"
)

type Patent struct {
	Number              string
	CountryCode         string
	Title               string
	Abstract            string
	Assignee            string
	Inventors           []string
	PublicationDate     string
	GrantDate           string
	ExpirationDate      string
	ExpirationSource    string
	SourceGoogleURL     string
	ImportSource        string
	StoredAt            time.Time
	UpdatedAt           time.Time
	StatusChangedAt     time.Time
	Status              string
	ClassificationLabel string
	LatestAssignment    string
	ExpectedCitations   int // Total backward count reported by source (-1 if unknown)
	ExpectedCitedBy     int // Total forward count reported by source (-1 if unknown)
	NotesCount          int
	ApplicationNumber   string
	ApplicationDate     string
	PublicationNumber   string
	GrantNumber         string
	FirstClaim          string
}

type PatentTextSection struct {
	PatentNumber string
	SectionType  string
	Ordinal      int
	Text         string
}

type CitationEdge struct {
	SourcePatent         string
	TargetPatent         string
	RelationType         string
	Status               string
	CreatedAt            time.Time
	RefreshedAt          time.Time
	LabeledAt            time.Time
	TargetTitle          string
	TargetInventors      []string
	TargetExpirationDate string
	TargetImportSource   string
}

type Classification struct {
	PatentNumber string
	System       string
	Code         string
	Section      string
	Class        string
	Subclass     string
	MainGroup    string
	Subgroup     string
	Description  string
}

type ResearchNote struct {
	ID           int64
	PatentNumber string
	Body         string
	CreatedAt    time.Time
}

type ReferenceEntry struct {
	ID            int64
	PatentNumber  string
	CitationLabel string
	CreatedAt     time.Time
}

type Project struct {
	ID            string
	Name          string
	Status        string
	SummaryStatus string
	Summary       string
	Comments      string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type AIAnalysis struct {
	ID                   int64
	PatentNumber         string
	AnalysisType         string
	ComparedPatentNumber string
	Provider             string
	Body                 string
	CreatedAt            time.Time
}

// IDSIconInFull is the icon shown when an IDS entry is cited in its entirety.
const IDSIconInFull = "⊛"

type IDSEntry struct {
	ID               int64
	ProjectID        string
	PatentNumber     string
	KindCode         string // see IDSKindCode* constants
	CountryCode      string // see IDSCountry* constants
	IsNPL            bool   // non-patent literature
	NPLAuthor        string
	NPLTitle         string
	NPLDate          string
	NPLPublisher     string
	InFull           bool   // entire document is cited; overrides RelevantPassages
	RelevantPassages string // pages/columns/lines with relevant content
	Notes            string
	Status           IDSStatus // see IDSStatus* constants
	AddedAt          time.Time
}

// IDSPassagesText returns the display text for the relevant passages column.
// When InFull is set it returns the icon + "In full", ignoring RelevantPassages.
func IDSPassagesText(e IDSEntry) string {
	if e.InFull {
		return IDSIconInFull + " In full"
	}
	return e.RelevantPassages
}

// IDSPassagesFormatGuide is the one-line hint shown when editing passages.
const IDSPassagesFormatGuide = "p. 5 · pp. 12-15 · col. 2 · cols. 3-4 · l. 10 · ll. 50-62 · para. [0014] · paras. [0014]-[0020] · FIG. 2 · FIGS. 4A-4C · Sec. 3.1"

// knownPassagePrefixes are the recognised token prefixes in order of specificity.
var knownPassagePrefixes = []string{
	"paras. ", "para. ", "pp. ", "p. ",
	"cols. ", "col. ",
	"ll. ", "l. ",
	"FIGS. ", "FIG. ",
	"Sec. ", "§ ", "¶ ",
}

// ValidateIDSPassages checks that every comma-separated token in s starts with
// a recognised prefix. Returns a non-nil error naming the first bad token.
func ValidateIDSPassages(s string) error {
	if s == "" {
		return nil
	}
	tokens := strings.Split(s, ",")
	for _, raw := range tokens {
		tok := strings.TrimSpace(raw)
		if tok == "" {
			continue
		}
		matched := false
		for _, pfx := range knownPassagePrefixes {
			if strings.HasPrefix(tok, pfx) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("unknown passage format %q — use: p., pp., col., cols., l., ll., para., paras., FIG., FIGS., Sec., §, ¶", tok)
		}
	}
	return nil
}

type IDSMetadata struct {
	ProjectID      string
	AppNumber      string
	FilingDate     string
	FirstInventor  string
	ExaminerName   string
	ArtUnit        string
	AttorneyDocket string
	UpdatedAt      time.Time
}

type PatentBundle struct {
	Patent            Patent
	Sections          []PatentTextSection
	Citations         []CitationEdge
	FamilyEdges       []FamilyEdge
	Classifications   []Classification
	References        []ReferenceEntry
	ExpectedCitations int // Total backward count reported by source (-1 if unknown)
	ExpectedCitedBy   int // Total forward count reported by source (-1 if unknown)
}

func (p Patent) IsExpired(now time.Time) bool {
	if p.ExpirationDate == "" {
		return false
	}
	expiration, err := time.Parse("2006-01-02", p.ExpirationDate)
	if err != nil {
		return false
	}
	return expiration.Before(dateOnly(now)) || expiration.Equal(dateOnly(now))
}

func dateOnly(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}
