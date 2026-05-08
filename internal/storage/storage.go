package storage

import (
	"context"

	"patentmine/internal/domain"
)

type ListPatentsOptions struct {
	Filter      string
	ClassFilter string
	SortColumn  string
	SortOrder   string
}

type ListCitationsOptions struct {
	SortColumn string
	SortOrder  string
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
	ListPatents(ctx context.Context, projectID string, opts ListPatentsOptions) ([]domain.Patent, error)
	ListCitations(ctx context.Context, projectID string, number, relationType string, opts ListCitationsOptions) ([]domain.CitationEdge, error)
	ListCitationsByStatus(ctx context.Context, projectID string, status string, opts ListCitationsOptions) ([]domain.CitationEdge, error)
	UpdateCitationStatus(ctx context.Context, projectID string, edge domain.CitationEdge, status string) error
	UpdatePatentStatus(ctx context.Context, projectID string, number string, status string) error
	UpdateClassificationDescription(ctx context.Context, projectID string, system, code, description string) error
	DeletePatent(ctx context.Context, projectID string, number string) error
	ListClassifications(ctx context.Context, projectID string, number string) ([]domain.Classification, error)
	ListTextSections(ctx context.Context, projectID string, number string) ([]domain.PatentTextSection, error)
	AddNote(ctx context.Context, projectID string, number, body string) (domain.ResearchNote, error)
	ListNotes(ctx context.Context, projectID string, number string) ([]domain.ResearchNote, error)
	AddReference(ctx context.Context, projectID string, number, label string) (domain.ReferenceEntry, error)
	ListReferences(ctx context.Context, projectID string) ([]domain.ReferenceEntry, error)
	AddAIAnalysis(ctx context.Context, projectID string, analysis domain.AIAnalysis) (domain.AIAnalysis, error)
	ListAIAnalyses(ctx context.Context, projectID string, number string) ([]domain.AIAnalysis, error)
}
