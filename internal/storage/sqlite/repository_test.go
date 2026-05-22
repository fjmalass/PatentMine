package sqlite

import (
	"context"
	"fmt"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/importer"
	"patentmine/internal/storage"
)

func TestDeletePatentCompletelyRemovesPatentAndChildren(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	bundle, err := importer.LoadFixture("../../../fixtures/US11611785B2.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertPatentBundle(ctx, "default", bundle); err != nil {
		t.Fatal(err)
	}
	num := bundle.Patent.Number

	if err := repo.DeletePatentCompletely(ctx, num); err != nil {
		t.Fatal(err)
	}

	checks := []struct{ table, col string }{
		{"patents", "number"},
		{"project_patents", "patent_number"},
		{"patent_classifications", "patent_number"},
		{"patent_text_sections", "patent_number"},
		{"research_notes", "patent_number"},
		{"reference_entries", "patent_number"},
		{"ai_analyses", "patent_number"},
		{"project_ids", "patent_number"},
		{"patent_tags", "patent_number"},
	}
	for _, c := range checks {
		var n int
		q := fmt.Sprintf("select count(*) from %s where %s = ?", c.table, c.col)
		if err := repo.db.QueryRowContext(ctx, q, num).Scan(&n); err != nil {
			t.Fatalf("%s: %v", c.table, err)
		}
		if n != 0 {
			t.Fatalf("%s still has %d row(s) for %s after complete delete", c.table, n, num)
		}
	}
	var edges int
	if err := repo.db.QueryRowContext(ctx,
		`select count(*) from citation_edges where source_patent = ? or target_patent = ?`,
		num, num).Scan(&edges); err != nil {
		t.Fatal(err)
	}
	if edges != 0 {
		t.Fatalf("citation_edges still has %d row(s) for %s after complete delete", edges, num)
	}
}

func TestRepositorySetupAndSeedIdempotency(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	bundle, err := importer.LoadFixture("../../../fixtures/US11611785B2.json")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := repo.UpsertPatentBundle(ctx, "default", bundle); err != nil {
			t.Fatal(err)
		}
	}
	patents, err := repo.ListPatents(ctx, "default", storage.ListPatentsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(patents) != 1 {
		t.Fatalf("expected one patent after repeated seed, got %d", len(patents))
	}
	classifications, _ := repo.ListClassifications(ctx, "default", "US11611785B2")
	if len(classifications) != len(bundle.Classifications) {
		t.Fatalf("expected %d classifications, got %d", len(bundle.Classifications), len(classifications))
	}
	patent, err := repo.GetPatent(ctx, "default", "US11611785B2")
	if err != nil {
		t.Fatal(err)
	}
	if patent.ExpirationDate != "2043-03-21" {
		t.Fatalf("expected expiration date to persist, got %q", patent.ExpirationDate)
	}
	if patent.ExpirationSource != domain.ExpirationSourceEstimated {
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
	if err := repo.UpsertPatentBundle(ctx, "default", bundle); err != nil {
		t.Fatal(err)
	}
	classifications, err := repo.ListClassifications(ctx, "default", "US-DUP-CPC")
	if err != nil {
		t.Fatal(err)
	}
	if len(classifications) != 1 {
		t.Fatalf("expected one deduplicated classification, got %d", len(classifications))
	}
}

func TestRepositoryRefreshPrunesStaleCitationsAndPreservesStatus(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	initial := domain.PatentBundle{
		Patent: domain.Patent{Number: "US-REFRESH", Title: "Refresh citations"},
		Citations: []domain.CitationEdge{
			{TargetPatent: "US1", RelationType: domain.RelationCites},
			{TargetPatent: "US2", RelationType: domain.RelationCitedBy},
			{TargetPatent: "US3", RelationType: domain.RelationCitedBy},
		},
	}
	if err := repo.UpsertPatentBundle(ctx, "default", initial); err != nil {
		t.Fatal(err)
	}
	citedBy, err := repo.ListCitations(ctx, "default", "US-REFRESH", domain.RelationCitedBy, storage.ListCitationsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(citedBy) != 2 {
		t.Fatalf("expected initial cited-by count 2, got %d", len(citedBy))
	}
	if err := repo.UpdateCitationReviewState(ctx, "default", citedBy[0], domain.ReviewStateIgnored); err != nil {
		t.Fatal(err)
	}

	refreshed := initial
	refreshed.Citations = []domain.CitationEdge{
		{TargetPatent: "US1", RelationType: domain.RelationCites},
		{TargetPatent: citedBy[0].TargetPatent, RelationType: domain.RelationCitedBy},
	}
	if err := repo.UpsertPatentBundle(ctx, "default", refreshed); err != nil {
		t.Fatal(err)
	}
	citedBy, err = repo.ListCitations(ctx, "default", "US-REFRESH", domain.RelationCitedBy, storage.ListCitationsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(citedBy) != 1 {
		t.Fatalf("expected stale cited-by row to be pruned, got %+v", citedBy)
	}
	if citedBy[0].ReviewState != domain.ReviewStateIgnored {
		t.Fatalf("expected review state to be preserved, got %q", citedBy[0].ReviewState)
	}
}

func TestRepositoryCreatesCitationPatentStubsWithoutOverwritingMetadata(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	target := domain.PatentBundle{
		Patent: domain.Patent{
			Number:   "US-TARGET",
			Title:    "Known target metadata",
			Assignee: "Example Assignee",
		},
	}
	if err := repo.UpsertPatentBundle(ctx, "default", target); err != nil {
		t.Fatal(err)
	}

	source := domain.PatentBundle{
		Patent: domain.Patent{Number: "US-SOURCE", Title: "Source patent"},
		Citations: []domain.CitationEdge{
			{SourcePatent: "US-SOURCEB2", TargetPatent: "US-TARGET", RelationType: domain.RelationCites},
			{TargetPatent: "US-UNKNOWN", RelationType: domain.RelationCitedBy},
		},
		References: []domain.ReferenceEntry{{
			PatentNumber:  "US-SOURCEB2",
			CitationLabel: "requested kind-code reference",
		}},
	}
	if err := repo.UpsertPatentBundle(ctx, "default", source); err != nil {
		t.Fatal(err)
	}

	known, err := repo.GetPatent(ctx, "default", "US-TARGET")
	if err != nil {
		t.Fatal(err)
	}
	if known.Title != "Known target metadata" || known.Assignee != "Example Assignee" {
		t.Fatalf("expected citation stub insert to preserve existing target metadata, got %+v", known)
	}
	alias, err := repo.GetPatent(ctx, "default", "US-SOURCEB2")
	if err != nil {
		t.Fatal(err)
	}
	if alias.Title != "US-SOURCEB2" {
		t.Fatalf("expected source alias stub title, got %+v", alias)
	}
	unknown, err := repo.GetPatent(ctx, "default", "US-UNKNOWN")
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Title != "US-UNKNOWN" {
		t.Fatalf("expected target stub title, got %+v", unknown)
	}
	cited, err := repo.ListCitations(ctx, "default", "US-SOURCEB2", domain.RelationCites, storage.ListCitationsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cited) != 1 || cited[0].TargetPatent != "US-TARGET" || cited[0].TargetTitle != "Known target metadata" {
		t.Fatalf("expected citation edge with preserved target metadata, got %+v", cited)
	}
	refs, err := repo.ListReferences(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	foundRef := false
	for _, ref := range refs {
		if ref.PatentNumber == "US-SOURCEB2" && ref.CitationLabel == "requested kind-code reference" {
			foundRef = true
		}
	}
	if !foundRef {
		t.Fatalf("expected kind-code reference row, got %+v", refs)
	}
}

func TestRepositoryOperations(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	bundle := domain.PatentBundle{
		Patent: domain.Patent{
			Number:           "US1",
			Title:            "Widget analyzer",
			Abstract:         "Analyzes widgets.",
			Assignee:         "Example",
			Inventors:        []string{"Inventor One"},
			PublicationDate:  "2024-01-01",
			GrantDate:        "2024-02-01",
			ExpirationDate:   "2044-01-01",
			ExpirationSource: domain.ExpirationSourceEstimated,
			SourceGoogleURL:  "https://example.test",
		},
		Sections:        []domain.PatentTextSection{{SectionType: "claims", Ordinal: 1, Text: "A widget analyzer."}},
		Citations:       []domain.CitationEdge{{TargetPatent: "US2", RelationType: domain.RelationCites}},
		Classifications: []domain.Classification{{Code: "G06F", Description: "Computing"}},
	}
	if err := repo.UpsertPatentBundle(ctx, "default", bundle); err != nil {
		t.Fatal(err)
	}
	patent, err := repo.GetPatent(ctx, "default", "US1")
	if err != nil {
		t.Fatal(err)
	}
	if patent.ExpirationDate != "2044-01-01" {
		t.Fatalf("expected expiration date, got %q", patent.ExpirationDate)
	}
	if patent.ExpirationSource != domain.ExpirationSourceEstimated {
		t.Fatal("expected expiration estimated flag")
	}
	if patent.StoredAt.IsZero() {
		t.Fatal("expected stored-local timestamp")
	}
	if got, _ := repo.ListPatents(ctx, "default", storage.ListPatentsOptions{Filter: "widget"}); len(got) != 1 {
		t.Fatalf("expected filtered patent, got %d", len(got))
	}
	if got, _ := repo.ListPatents(ctx, "default", storage.ListPatentsOptions{Filter: "Inventor One"}); len(got) != 1 {
		t.Fatalf("expected inventor-filtered patent, got %d", len(got))
	}
	citations, err := repo.ListCitations(ctx, "default", "US1", domain.RelationCites, storage.ListCitationsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(citations) != 1 {
		t.Fatalf("expected citation, got %d", len(citations))
	}
	if citations[0].ReviewState != domain.ReviewStateCached {
		t.Fatalf("expected cached citation review state (new default), got %q", citations[0].ReviewState)
	}
	if citations[0].CreatedAt.IsZero() || citations[0].RefreshedAt.IsZero() || citations[0].LabeledAt.IsZero() {
		t.Fatalf("expected citation timestamps, got created=%v refreshed=%v labeled=%v", citations[0].CreatedAt, citations[0].RefreshedAt, citations[0].LabeledAt)
	}
	if err := repo.UpdateCitationReviewState(ctx, "default", citations[0], domain.ReviewStateIgnored); err != nil {
		t.Fatal(err)
	}
	citations, err = repo.ListCitations(ctx, "default", "US1", domain.RelationCites, storage.ListCitationsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if citations[0].ReviewState != domain.ReviewStateIgnored {
		t.Fatalf("expected ignored citation review state, got %q", citations[0].ReviewState)
	}
	if citations[0].LabeledAt.IsZero() {
		t.Fatal("expected citation labeled timestamp")
	}
	ignored, err := repo.ListCitationsByReviewState(ctx, "default", domain.ReviewStateIgnored, storage.ListCitationsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ignored) != 1 || ignored[0].TargetPatent != "US2" {
		t.Fatalf("expected ignored citation queue, got %+v", ignored)
	}
	if err := repo.UpsertPatentBundle(ctx, "default", bundle); err != nil {
		t.Fatal(err)
	}
	citations, err = repo.ListCitations(ctx, "default", "US1", domain.RelationCites, storage.ListCitationsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if citations[0].ReviewState != domain.ReviewStateIgnored {
		t.Fatalf("expected refresh to preserve citation review state, got %q", citations[0].ReviewState)
	}
	if got, _ := repo.ListTextSections(ctx, "default", "US1"); len(got) != 1 {
		t.Fatalf("expected text section, got %d", len(got))
	}
	if note, err := repo.AddNote(ctx, "default", "US1", "important"); err != nil || note.Body != "important" {
		t.Fatalf("unexpected note: %+v %v", note, err)
	}
	if got, _ := repo.ListNotes(ctx, "default", "US1"); len(got) != 1 {
		t.Fatalf("expected note, got %d", len(got))
	}
	if _, err := repo.AddReference(ctx, "default", "US1", "US1, Widget analyzer"); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.ListReferences(ctx, "default"); len(got) != 1 {
		t.Fatalf("expected reference, got %d", len(got))
	}
	if _, err := repo.AddAIAnalysis(ctx, "default", domain.AIAnalysis{PatentNumber: "US1", AnalysisType: "summary", Provider: "local-stub", Body: "summary"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.ListAIAnalyses(ctx, "default", "US1"); len(got) != 1 {
		t.Fatalf("expected AI artifact, got %d", len(got))
	}

	// Test Patent Status and Deletion
	if err := repo.DeletePatent(ctx, "default", "US1"); err != nil {
		t.Fatal(err)
	}
	patent, err = repo.GetPatent(ctx, "default", "US1")
	if err != nil {
		t.Fatal(err)
	}
	if patent.ReviewState != domain.ReviewStateIgnored {
		t.Fatalf("expected patent status to be ignored, got %q", patent.ReviewState)
	}
	// Default (stored) should not show deleted patent
	storedPatents, err := repo.ListPatents(ctx, "default", storage.ListPatentsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range storedPatents {
		if p.Number == "US1" {
			t.Fatal("deleted patent should not appear in default (stored) list")
		}
	}
	// ReviewStateFilter ignored should surface deleted project patent
	ignoredPatents, err := repo.ListPatents(ctx, "default", storage.ListPatentsOptions{ReviewStateFilter: domain.ReviewStateIgnored})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range ignoredPatents {
		if p.Number == "US1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected deleted patent to appear under statusfilter ignored")
	}
}

func TestRepositoryListFamilyPatentsReturnsLightweightMetadata(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	bundles := []domain.PatentBundle{
		{Patent: domain.Patent{Number: "US-FAM-1", Title: "Family One", Assignee: "Alpha", PublicationDate: "2018-01-01", GrantDate: "2020-02-02", ExpirationDate: "2038-03-03", ExpirationSource: domain.ExpirationSourceEstimated, ImportSource: "google", CountryCode: "US"}},
		{Patent: domain.Patent{Number: "US-FAM-2", Title: "Family Two", Assignee: "Beta", PublicationDate: "2019-01-01", GrantDate: "2021-02-02", ExpirationDate: "2039-03-03", ExpirationSource: domain.ExpirationSourceManual, ImportSource: "uspto", CountryCode: "EP"}},
	}
	for _, bundle := range bundles {
		if err := repo.UpsertPatentBundle(ctx, "default", bundle); err != nil {
			t.Fatal(err)
		}
	}

	got, err := repo.ListFamilyPatents(ctx, "default", []domain.PatentNumber{"US-FAM-1", "US-FAM-2", "US-MISSING"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 family patents, got %d", len(got))
	}
	if got["US-FAM-1"].Title != "Family One" || got["US-FAM-1"].GrantDate != "2020-02-02" {
		t.Fatalf("unexpected family patent metadata: %+v", got["US-FAM-1"])
	}
	if got["US-FAM-2"].Assignee != "Beta" || got["US-FAM-2"].ImportSource != "uspto" {
		t.Fatalf("unexpected family patent metadata: %+v", got["US-FAM-2"])
	}
	if _, ok := got["US-MISSING"]; ok {
		t.Fatal("did not expect missing family patent metadata to be returned")
	}
}

func newTestRepo(t *testing.T) *Repository {

	t.Helper()
	repo, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	return repo
}
