package domain

import "time"

const (
	RelationCites   = "cites"
	RelationCitedBy = "cited_by"
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

// ISO 3166-1 alpha-2 codes for major patent offices.
const (
	IDSCountryUS = "US" // United States
	IDSCountryEP = "EP" // European Patent Office
	IDSCountryWO = "WO" // WIPO / PCT
	IDSCountryJP = "JP" // Japan
	IDSCountryCN = "CN" // China
	IDSCountryKR = "KR" // South Korea
	IDSCountryDE = "DE" // Germany
	IDSCountryFR = "FR" // France
	IDSCountryGB = "GB" // United Kingdom
	IDSCountryCA = "CA" // Canada
	IDSCountryAU = "AU" // Australia
	IDSCountryIN = "IN" // India
	IDSCountryTW = "TW" // Taiwan
	IDSCountryIL = "IL" // Israel
	IDSCountrySG = "SG" // Singapore
	IDSCountryBR = "BR" // Brazil
	IDSCountryMX = "MX" // Mexico
	IDSCountryRU = "RU" // Russia
)

// IDSCountryCodes lists recognized country codes for TUI hints and validation.
var IDSCountryCodes = []string{
	IDSCountryUS, IDSCountryEP, IDSCountryWO,
	IDSCountryJP, IDSCountryCN, IDSCountryKR,
	IDSCountryDE, IDSCountryFR, IDSCountryGB,
	IDSCountryCA, IDSCountryAU, IDSCountryIN,
	IDSCountryTW, IDSCountryIL, IDSCountrySG,
	IDSCountryBR, IDSCountryMX, IDSCountryRU,
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

type Patent struct {
	Number              string
	Title               string
	Abstract            string
	Assignee            string
	Inventors           []string
	PublicationDate     string
	GrantDate           string
	ExpirationDate      string
	ExpirationEstimated bool
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

type IDSEntry struct {
	ID               int64
	ProjectID        string
	PatentNumber     string
	KindCode         string    // see IDSKindCode* constants
	CountryCode      string    // see IDSCountry* constants
	IsNPL            bool      // non-patent literature
	NPLAuthor        string
	NPLTitle         string
	NPLDate          string
	NPLPublisher     string
	RelevantPassages string    // pages/columns/lines with relevant content
	Notes            string
	Status           IDSStatus // see IDSStatus* constants
	AddedAt          time.Time
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
