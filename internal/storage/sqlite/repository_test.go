package sqlite

import (
	"context"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/importer"
)

func TestRepositorySetupAndSeedIdempotency(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	bundle, err := importer.LoadFixture("../../../fixtures/US11611785B2.json")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := repo.UpsertPatentBundle(ctx, bundle); err != nil {
			t.Fatal(err)
		}
	}
	patents, err := repo.ListPatents(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(patents) != 1 {
		t.Fatalf("expected one patent after repeated seed, got %d", len(patents))
	}
	classifications, _ := repo.ListClassifications(ctx, "US11611785B2")
	if len(classifications) != len(bundle.Classifications) {
		t.Fatalf("expected %d classifications, got %d", len(bundle.Classifications), len(classifications))
	}
	patent, err := repo.GetPatent(ctx, "US11611785B2")
	if err != nil {
		t.Fatal(err)
	}
	if patent.ExpirationDate != "2043-03-21" {
		t.Fatalf("expected expiration date to persist, got %q", patent.ExpirationDate)
	}
	if !patent.ExpirationEstimated {
		t.Fatal("expected estimated expiration flag to persist")
	}
	if patent.StoredAt.IsZero() {
		t.Fatal("expected stored-local timestamp")
	}
}

func TestRepositoryIgnoresDuplicateClassificationsInImportBundle(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	bundle := domain.PatentBundle{
		Patent: domain.Patent{
			Number: "US-DUP-CPC",
			Title:  "Duplicate classification import",
		},
		Classifications: []domain.Classification{
			{Code: "G06F16/33", Description: "Information retrieval"},
			{Code: "G06F16/33", Description: "Information retrieval duplicate"},
			{Code: "", Description: "empty code should be skipped"},
		},
	}
	if err := repo.UpsertPatentBundle(ctx, bundle); err != nil {
		t.Fatal(err)
	}
	classifications, err := repo.ListClassifications(ctx, "US-DUP-CPC")
	if err != nil {
		t.Fatal(err)
	}
	if len(classifications) != 1 {
		t.Fatalf("expected one deduplicated classification, got %d", len(classifications))
	}
}

func TestRepositoryOperations(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	bundle := domain.PatentBundle{
		Patent: domain.Patent{
			Number:              "US1",
			Title:               "Widget analyzer",
			Abstract:            "Analyzes widgets.",
			Assignee:            "Example",
			Inventors:           []string{"Inventor One"},
			PublicationDate:     "2024-01-01",
			GrantDate:           "2024-02-01",
			ExpirationDate:      "2044-01-01",
			ExpirationEstimated: true,
			SourceURL:           "https://example.test",
		},
		Sections:        []domain.PatentTextSection{{SectionType: "claims", Ordinal: 1, Text: "A widget analyzer."}},
		Citations:       []domain.CitationEdge{{TargetPatent: "US2", RelationType: domain.RelationCites}},
		Classifications: []domain.Classification{{Code: "G06F", Description: "Computing"}},
	}
	if err := repo.UpsertPatentBundle(ctx, bundle); err != nil {
		t.Fatal(err)
	}
	patent, err := repo.GetPatent(ctx, "US1")
	if err != nil {
		t.Fatal(err)
	}
	if patent.ExpirationDate != "2044-01-01" {
		t.Fatalf("expected expiration date, got %q", patent.ExpirationDate)
	}
	if !patent.ExpirationEstimated {
		t.Fatal("expected expiration estimated flag")
	}
	if patent.StoredAt.IsZero() {
		t.Fatal("expected stored-local timestamp")
	}
	if got, _ := repo.ListPatents(ctx, "widget"); len(got) != 1 {
		t.Fatalf("expected filtered patent, got %d", len(got))
	}
	if got, _ := repo.ListPatents(ctx, "Inventor One"); len(got) != 1 {
		t.Fatalf("expected inventor-filtered patent, got %d", len(got))
	}
	citations, err := repo.ListCitations(ctx, "US1", domain.RelationCites)
	if err != nil {
		t.Fatal(err)
	}
	if len(citations) != 1 {
		t.Fatalf("expected citation, got %d", len(citations))
	}
	if citations[0].Status != domain.CitationStatusUnderReview {
		t.Fatalf("expected unreviewed citation status, got %q", citations[0].Status)
	}
	if citations[0].CreatedAt.IsZero() || citations[0].RefreshedAt.IsZero() || citations[0].LabeledAt.IsZero() {
		t.Fatalf("expected citation timestamps, got created=%v refreshed=%v labeled=%v", citations[0].CreatedAt, citations[0].RefreshedAt, citations[0].LabeledAt)
	}
	if err := repo.UpdateCitationStatus(ctx, citations[0], domain.CitationStatusIgnored); err != nil {
		t.Fatal(err)
	}
	citations, err = repo.ListCitations(ctx, "US1", domain.RelationCites)
	if err != nil {
		t.Fatal(err)
	}
	if citations[0].Status != domain.CitationStatusIgnored {
		t.Fatalf("expected ignored citation status, got %q", citations[0].Status)
	}
	if citations[0].LabeledAt.IsZero() {
		t.Fatal("expected citation labeled timestamp")
	}
	ignored, err := repo.ListCitationsByStatus(ctx, domain.CitationStatusIgnored)
	if err != nil {
		t.Fatal(err)
	}
	if len(ignored) != 1 || ignored[0].TargetPatent != "US2" {
		t.Fatalf("expected ignored citation queue, got %+v", ignored)
	}
	if err := repo.UpsertPatentBundle(ctx, bundle); err != nil {
		t.Fatal(err)
	}
	citations, err = repo.ListCitations(ctx, "US1", domain.RelationCites)
	if err != nil {
		t.Fatal(err)
	}
	if citations[0].Status != domain.CitationStatusIgnored {
		t.Fatalf("expected refresh to preserve citation status, got %q", citations[0].Status)
	}
	if got, _ := repo.ListTextSections(ctx, "US1"); len(got) != 1 {
		t.Fatalf("expected text section, got %d", len(got))
	}
	if note, err := repo.AddNote(ctx, "US1", "important"); err != nil || note.Body != "important" {
		t.Fatalf("unexpected note: %+v %v", note, err)
	}
	if got, _ := repo.ListNotes(ctx, "US1"); len(got) != 1 {
		t.Fatalf("expected note, got %d", len(got))
	}
	if _, err := repo.AddReference(ctx, "US1", "US1, Widget analyzer"); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.ListReferences(ctx); len(got) != 1 {
		t.Fatalf("expected reference, got %d", len(got))
	}
	if _, err := repo.AddAIArtifact(ctx, domain.AIArtifact{PatentNumber: "US1", ArtifactType: "summary", Provider: "local-stub", Body: "summary"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.ListAIArtifacts(ctx, "US1"); len(got) != 1 {
		t.Fatalf("expected AI artifact, got %d", len(got))
	}

	// Test Patent Status and Deletion
	if err := repo.DeletePatent(ctx, "US1"); err != nil {
		t.Fatal(err)
	}
	patent, err = repo.GetPatent(ctx, "US1")
	if err != nil {
		t.Fatal(err)
	}
	if patent.Status != domain.CitationStatusIgnored {
		t.Fatalf("expected patent status to be ignored, got %q", patent.Status)
	}
	patents, err := repo.ListPatents(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range patents {
		if p.Number == "US1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected ignored patent to be present in ListPatents")
	}
}

func TestRepositoryMigratesExpirationDateColumn(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if _, err := repo.db.ExecContext(ctx, `create table patents (
		number text primary key,
		title text not null,
		abstract text not null,
		assignee text not null,
		inventors_json text not null,
		publication_date text not null,
		grant_date text not null,
		source_url text not null
	)`); err != nil {
		t.Fatal(err)
	}
	if err := repo.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	hasColumn, err := repo.hasColumn(ctx, "patents", "expiration_date")
	if err != nil {
		t.Fatal(err)
	}
	if !hasColumn {
		t.Fatal("expected expiration_date column to be added")
	}
	hasColumn, err = repo.hasColumn(ctx, "patents", "expiration_estimated")
	if err != nil {
		t.Fatal(err)
	}
	if !hasColumn {
		t.Fatal("expected expiration_estimated column to be added")
	}
}

func newTestRepo(t *testing.T) *Repository {
	t.Helper()
	repo, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	return repo
}
