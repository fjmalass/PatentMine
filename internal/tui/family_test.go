package tui

import (
	"context"
	"patentmine/internal/domain"
	"patentmine/internal/storage"
	"testing"
)

type familyStubRepo struct {
	stubRepo
	edges []domain.FamilyEdge
}

func (r familyStubRepo) ListAllFamilyEdges(ctx context.Context, projectID domain.ProjectID) ([]domain.FamilyEdge, error) {
	return r.edges, nil
}

func (r familyStubRepo) ListCitationsByReviewState(context.Context, domain.ProjectID, string, storage.ListCitationsOptions) ([]domain.CitationEdge, error) {
	return nil, nil
}

func (r familyStubRepo) GetPatent(ctx context.Context, projectID domain.ProjectID, number domain.PatentNumber) (domain.Patent, error) {
	return domain.Patent{Number: number}, nil
}

func (r familyStubRepo) ListFamilyPatents(ctx context.Context, projectID domain.ProjectID, numbers []domain.PatentNumber) (map[domain.PatentNumber]domain.Patent, error) {
	out := make(map[domain.PatentNumber]domain.Patent, len(numbers))
	for _, number := range numbers {
		out[number] = domain.Patent{Number: number}
	}
	return out, nil
}

func TestBuildFamilyTreeWithCycle(t *testing.T) {
	repo := familyStubRepo{
		edges: []domain.FamilyEdge{
			{ParentNumber: "A", ChildNumber: "B"},
			{ParentNumber: "B", ChildNumber: "A"},
		},
	}
	m := &Model{
		repo:      repo,
		ProjectID: "p1",
		current:   domain.Patent{Number: "A"},
	}

	nodes := m.buildFamilyTreeFresh()
	if len(nodes) == 0 {
		t.Errorf("expected some nodes even with cycles, got 0")
	}
}

func TestBuildFamilyTreeWithDiamond(t *testing.T) {
	repo := familyStubRepo{
		edges: []domain.FamilyEdge{
			{ParentNumber: "A", ChildNumber: "B"},
			{ParentNumber: "A", ChildNumber: "C"},
			{ParentNumber: "B", ChildNumber: "D"},
			{ParentNumber: "C", ChildNumber: "D"},
		},
	}
	m := &Model{
		repo:      repo,
		ProjectID: "p1",
		current:   domain.Patent{Number: "A"},
	}

	nodes := m.buildFamilyTreeFresh()
	// A
	// ├─ B
	// │  └─ D
	// └─ C
	// D appears exactly once in the new single-path logic.
	dCount := 0
	for _, n := range nodes {
		if n.number == "D" {
			dCount++
		}
	}
	if dCount != 1 {
		t.Errorf("expected D to appear exactly once in diamond with single-path logic, got %d", dCount)
	}
}
