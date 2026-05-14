package storage

import (
	"context"

	"patentmine/internal/domain"
)

const (
	ReviewStateFilterNone = "none"
)

type ListPatentsOptions struct {
	Filter           string
	CountryFilter    string
	ReviewStateFilter string // "stored" (default), "ignored", "under_review", "all"
	ClassFilters     []string // CPC prefix filters
	ClassFilterOp    string   // "and" (default) or "or"
	TagFilter        string
	SortColumn       string
	SortOrder        string
	SortColumn2      string
	SortOrder2       string
}

type ListCitationsOptions struct {
	ReviewStateFilter string // "stored", "ignored", "under_review", or "" for all
	SortColumn        string
	SortOrder         string
}

type Repository interface {
	Close() error
	Setup(ctx context.Context) error

	// Project Management
	CreateProject(ctx context.Context, p domain.Project) error
	GetProject(ctx context.Context, id string) (domain.Project, error)
	ListProjects(ctx context.Context) ([]domain.Project, error)
	UpdateProject(ctx context.Context, p domain.Project) error
	DeleteProject(ctx context.Context, id string) error
	AddPatentToProject(ctx context.Context, projectID, patentNumber string) error
	RemovePatentFromProject(ctx context.Context, projectID, patentNumber string) error

	UpsertPatentBundle(ctx context.Context, projectID string, bundle domain.PatentBundle) error
	GetPatent(ctx context.Context, projectID string, number string) (domain.Patent, error)
	ListFamilyPatents(ctx context.Context, projectID string, numbers []string) (map[string]domain.Patent, error)
	ListPatents(ctx context.Context, projectID string, opts ListPatentsOptions) ([]domain.Patent, error)
	ListCitations(ctx context.Context, projectID string, number, relationType string, opts ListCitationsOptions) ([]domain.CitationEdge, error)
	ListCitationsByReviewState(ctx context.Context, projectID string, reviewState string, opts ListCitationsOptions) ([]domain.CitationEdge, error)
	UpdateCitationReviewState(ctx context.Context, projectID string, edge domain.CitationEdge, reviewState string) error
	UpdatePatentReviewState(ctx context.Context, projectID string, number string, reviewState string) error
	UpdatePatentDate(ctx context.Context, number string, dateType string, value string) error
	UpdatePatentNumber(ctx context.Context, number string, numType string, value string) error
	UpdateClassificationDescription(ctx context.Context, projectID string, system, code, description string) error
	DeletePatent(ctx context.Context, projectID string, number string) error
	ListClassifications(ctx context.Context, projectID string, number string) ([]domain.Classification, error)
	GetClassificationStats(ctx context.Context, projectID string, code string) (domain.ClassificationStats, error)
	ListTextSections(ctx context.Context, projectID string, number string) ([]domain.PatentTextSection, error)
	AddNote(ctx context.Context, projectID string, number, body string) (domain.ResearchNote, error)
	ListNotes(ctx context.Context, projectID string, number string) ([]domain.ResearchNote, error)
	AddReference(ctx context.Context, projectID string, number, label string) (domain.ReferenceEntry, error)
	ListReferences(ctx context.Context, projectID string) ([]domain.ReferenceEntry, error)
	AddAIAnalysis(ctx context.Context, projectID string, analysis domain.AIAnalysis) (domain.AIAnalysis, error)
	ListAIAnalyses(ctx context.Context, projectID string, number string) ([]domain.AIAnalysis, error)

	// Project lifecycle events
	AddProjectEvent(ctx context.Context, e domain.ProjectEvent) (domain.ProjectEvent, error)
	ListProjectEvents(ctx context.Context, projectID string) ([]domain.ProjectEvent, error)
	DeleteProjectEvent(ctx context.Context, id int64) error

	// Patent family relationships
	AddFamilyEdge(ctx context.Context, edge domain.FamilyEdge) error
	ListFamilyEdges(ctx context.Context, projectID, number string) (parents []domain.FamilyEdge, children []domain.FamilyEdge, err error)
	ListAllFamilyEdges(ctx context.Context, projectID string) ([]domain.FamilyEdge, error)
	RemoveFamilyEdge(ctx context.Context, projectID, parentNumber, childNumber string) error

	// Settings
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error

	// Project invoices
	AddProjectInvoice(ctx context.Context, inv domain.ProjectInvoice) (domain.ProjectInvoice, error)
	ListProjectInvoices(ctx context.Context, projectID string) ([]domain.ProjectInvoice, error)
	UpdateProjectInvoice(ctx context.Context, inv domain.ProjectInvoice) error
	DeleteProjectInvoice(ctx context.Context, id int64) error
	CountUnpaidInvoicesByProject(ctx context.Context) (map[string]int, error)

	// IDS (Information Disclosure Statement)
	AddIDSEntry(ctx context.Context, entry domain.IDSEntry) (domain.IDSEntry, error)
	ListIDSEntries(ctx context.Context, projectID string) ([]domain.IDSEntry, error)
	DeleteIDSEntry(ctx context.Context, id int64) error
	UpdateIDSEntryStatus(ctx context.Context, id int64, status domain.IDSStatus) error
	UpdateIDSEntry(ctx context.Context, entry domain.IDSEntry) error
	GetIDSMetadata(ctx context.Context, projectID string) (domain.IDSMetadata, error)
	SaveIDSMetadata(ctx context.Context, meta domain.IDSMetadata) error
	AddIDSNPLEntry(ctx context.Context, entry domain.IDSEntry) (domain.IDSEntry, error)
	ListIDSNPLEntries(ctx context.Context, projectID string) ([]domain.IDSEntry, error)
	DeleteIDSNPLEntry(ctx context.Context, id int64) error

	// Tagging
	CreateTag(ctx context.Context, projectID, name, color string) (int64, error)
	ListTagsWithCounts(ctx context.Context, projectID string) ([]domain.TagWithCount, error)
	DeleteTag(ctx context.Context, tagID int64) error
	RenameTag(ctx context.Context, tagID int64, newName string) error
	UpdateTagColor(ctx context.Context, tagID int64, color string) error
	GetTagByName(ctx context.Context, projectID, name string) (domain.Tag, error)
	ApplyTagToPatent(ctx context.Context, patentNumber string, tagID int64) error
	RemoveTagFromPatent(ctx context.Context, patentNumber string, tagID int64) error
	GetPatentTags(ctx context.Context, patentNumber string) ([]domain.Tag, error)
	ListPatentTagsForProject(ctx context.Context, projectID string) (map[string][]domain.Tag, error)

	// Maintenance
	PurgeIgnored(ctx context.Context, projectID string) (int, error)
	Compact(ctx context.Context) error
}
