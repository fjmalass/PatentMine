package ai

import (
	"strings"
	"testing"

	"patentmine/internal/domain"
)

func TestSummarizeAndCompare(t *testing.T) {
	p := domain.Patent{Number: "US1", Title: "Patent search system", Abstract: "A system searches patent documents.", Assignee: "Example"}
	summary := Summarize(p, []domain.Classification{{Code: "G06F"}}, []domain.CitationEdge{{TargetPatent: "US2"}})
	if summary.Provider != Provider || !strings.Contains(summary.Body, "US1") {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	comparison := Compare(p, domain.Patent{Number: "US2", Title: "Patent search method", Abstract: "A method searches documents.", Assignee: "Example"})
	if comparison.ArtifactType != "comparison" || !strings.Contains(comparison.Body, "US1 vs US2") {
		t.Fatalf("unexpected comparison: %+v", comparison)
	}
}
