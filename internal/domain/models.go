package domain

import "time"

const (
	RelationCites   = "cites"
	RelationCitedBy = "cited_by"
)

const (
	CitationStatusUnderReview = "under_review"
	CitationStatusStored      = "stored"
	CitationStatusIgnored     = "ignored"
	CitationStatusCached      = "cached"
)

const (
	SortColumnNumber   = "number"
	SortColumnTitle    = "title"
	SortColumnDate     = "date"
	SortColumnStatus   = "status"
	SortColumnAssignee = "assignee"
	SortColumnInventor = "inventor"
	SortColumnClass    = "class"
	SortColumnCPC      = "cpc"

	SortOrderAsc  = "asc"
	SortOrderDesc = "desc"
)

const (
	AnalysisTypeSummary    = "summary"
	AnalysisTypeComparison = "comparison"
)

const (
	ProjectStatusActive   = "active"
	ProjectStatusArchived = "archived"
)

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
	SourceURL           string
	StoredAt            time.Time
	Status              string
	ClassificationLabel string
	LatestAssignment    string
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
	ID        string
	Name      string
	Status    string
	Comments  string
	CreatedAt time.Time
	UpdatedAt time.Time
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

type PatentBundle struct {
	Patent          Patent
	Sections        []PatentTextSection
	Citations       []CitationEdge
	Classifications []Classification
	References      []ReferenceEntry
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
