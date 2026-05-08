package storage

import (
	"context"

	"patentmine/internal/domain"
)

type Repository interface {
	Close() error
	Setup(ctx context.Context) error
	UpsertPatentBundle(ctx context.Context, bundle domain.PatentBundle) error
	GetPatent(ctx context.Context, number string) (domain.Patent, error)
	ListPatents(ctx context.Context, filter string) ([]domain.Patent, error)
	ListCitations(ctx context.Context, number, relationType string) ([]domain.CitationEdge, error)
	ListCitationsByStatus(ctx context.Context, status string) ([]domain.CitationEdge, error)
	UpdateCitationStatus(ctx context.Context, edge domain.CitationEdge, status string) error
	UpdatePatentStatus(ctx context.Context, number string, status string) error
	UpdateClassificationDescription(ctx context.Context, system, code, description string) error
	DeletePatent(ctx context.Context, number string) error
	ListClassifications(ctx context.Context, number string) ([]domain.Classification, error)
	ListTextSections(ctx context.Context, number string) ([]domain.PatentTextSection, error)
	AddNote(ctx context.Context, number, body string) (domain.ResearchNote, error)
	ListNotes(ctx context.Context, number string) ([]domain.ResearchNote, error)
	AddReference(ctx context.Context, number, label string) (domain.ReferenceEntry, error)
	ListReferences(ctx context.Context) ([]domain.ReferenceEntry, error)
	AddAIArtifact(ctx context.Context, artifact domain.AIArtifact) (domain.AIArtifact, error)
	ListAIArtifacts(ctx context.Context, number string) ([]domain.AIArtifact, error)
}
