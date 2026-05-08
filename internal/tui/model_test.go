package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
)

func TestFormatExpirationShowsEstimatedSuffix(t *testing.T) {
	got := (Model{}).formatExpiration(domain.Patent{
		ExpirationDate:      "2043-03-21",
		ExpirationEstimated: true,
	})
	if got != "2043-03-21 (est.)" {
		t.Fatalf("expected estimated expiration label, got %q", got)
	}
}

func TestFormatExpirationKeepsExpiredLabelText(t *testing.T) {
	got := (Model{}).formatExpiration(domain.Patent{
		ExpirationDate:      "2001-01-01",
		ExpirationEstimated: true,
	})
	if got != "2001-01-01 (est.)" {
		t.Fatalf("expected expired expiration label text, got %q", got)
	}
}

func TestDetailRowAlignsValues(t *testing.T) {
	model := Model{text: EnglishText()}
	got := model.detailRow(TextDetailGrant, "2023-03-21") + model.detailRow(TextDetailExpiration, "2043-03-21 (est.)")
	want := "Grant:       2023-03-21\nExpiration:  2043-03-21 (est.)\n"
	if got != want {
		t.Fatalf("expected aligned detail rows:\n%q\ngot:\n%q", want, got)
	}
}

func TestDetailRowUsesTextCatalog(t *testing.T) {
	model := Model{text: TextCatalog{
		TextDetailAssignee: "Titulaire",
		TextValueUnknown:   "inconnu",
	}}
	got := model.detailRow(TextDetailAssignee, "")
	want := "Titulaire:   inconnu\n"
	if got != want {
		t.Fatalf("expected localized detail row %q, got %q", want, got)
	}
}

func TestListNumberWidthAlignsPatentNumbers(t *testing.T) {
	model := Model{patents: []domain.Patent{{Number: "US1"}, {Number: "US12345B2"}}}
	if got := model.listNumberWidth(); got != len("US12345B2") {
		t.Fatalf("expected list number width %d, got %d", len("US12345B2"), got)
	}
}

func TestPreviewOverlayUsesBorderAndSmallerWidth(t *testing.T) {
	model := Model{width: 120}
	got := model.previewOverlay("US1\nPreview")
	if !strings.Contains(got, "┌") || !strings.Contains(got, "┘") {
		t.Fatalf("expected bordered overlay, got %q", got)
	}
	if width := model.overlayWidth(); width >= model.width {
		t.Fatalf("expected overlay width below terminal width, got %d for terminal %d", width, model.width)
	}
}

func TestNavigationStackGoesBackToPreviousView(t *testing.T) {
	model := Model{mode: viewList, selected: 3}
	model = model.navigateTo(viewDetail)
	model.selected = 0

	updated, _ := model.goBack()
	got := updated.(Model)
	if got.mode != viewList {
		t.Fatalf("expected view %q, got %q", viewList, got.mode)
	}
	if got.selected != 3 {
		t.Fatalf("expected selection to be restored, got %d", got.selected)
	}
}

func TestApplyJumpSelectsVisibleListTarget(t *testing.T) {
	model := Model{
		mode:     viewList,
		jumpMode: true,
		patents:  []domain.Patent{{Number: "US1"}, {Number: "US2"}, {Number: "US3"}},
	}
	got := model.applyJump("d")
	if got.selected != 2 {
		t.Fatalf("expected list selection 2, got %d", got.selected)
	}
	if got.jumpMode {
		t.Fatal("expected jump mode to exit after a valid jump")
	}
}

func TestApplyJumpSelectsDetailField(t *testing.T) {
	model := Model{
		mode:     viewDetail,
		jumpMode: true,
		text:     EnglishText(),
		current:  domain.Patent{Assignee: "Divx LLC", Inventors: []string{"Kourosh Soroushian"}},
	}
	got := model.applyJump(jumpLabelInventors)
	if got.detailSelected != 1 {
		t.Fatalf("expected detail selection 1, got %d", got.detailSelected)
	}
}

func TestDetailJumpLabelsMatchFields(t *testing.T) {
	model := Model{
		mode:    viewDetail,
		text:    EnglishText(),
		current: domain.Patent{Assignee: "Divx LLC", Inventors: []string{"Kourosh Soroushian"}},
	}
	got := model.jumpLabels()
	want := []string{
		jumpLabelAssignee,
		jumpLabelInventors,
		jumpLabelPublication,
		jumpLabelGrant,
		jumpLabelClassification,
		jumpLabelExpiration,
		jumpLabelStoredLocal,
		jumpLabelCitationCount,
		jumpLabelCitedByCount,
		jumpLabelSource,
	}
	if len(got) != len(want) {
		t.Fatalf("expected labels %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected labels %v, got %v", want, got)
		}
	}
}

func TestDetailFieldsAlwaysIncludeClassification(t *testing.T) {
	model := Model{
		text:    EnglishText(),
		current: domain.Patent{Assignee: "Divx LLC"},
	}
	fields := model.detailFields()
	found := false
	for _, field := range fields {
		if field.label == TextDetailClassification {
			found = true
			if field.displayValue != "Empty" {
				t.Fatalf("expected empty classification value, got %+v", field)
			}
			if field.action != detailActionClassification {
				t.Fatalf("expected classification action, got %+v", field)
			}
		}
	}
	if !found {
		t.Fatal("expected classification field")
	}
}

func TestViewClassificationsShowsEmptyState(t *testing.T) {
	model := Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    emptyClassificationRepo{},
		current: domain.Patent{Number: "US10218760B2"},
	}
	if got := model.viewClassifications(); got != "Empty\n" {
		t.Fatalf("expected empty classification view, got %q", got)
	}
}

func TestViewClassificationsShowsPagedIndexedRows(t *testing.T) {
	model := Model{
		ctx:                    t.Context(),
		text:                   EnglishText(),
		repo:                   classificationRepo{classifications: sampleClassifications(8)},
		current:                domain.Patent{Number: "US10218760B2"},
		classificationSelected: 6,
		height:                 12,
		width:                  80,
	}
	got := model.viewClassifications()
	if !strings.Contains(got, "Page 2/2 - items 6-8 of 8") {
		t.Fatalf("expected page status, got %q", got)
	}
	if strings.Contains(got, "[CPC]") {
		t.Fatalf("expected no CPC prefix, got %q", got)
	}
	if !strings.Contains(got, ">   7") {
		t.Fatalf("expected selected absolute row index, got %q", got)
	}
	if !strings.Contains(got, "H04N21/436") {
		t.Fatalf("expected selected classification code, got %q", got)
	}
}

func TestEnterOnDetailClassificationOpensClassificationList(t *testing.T) {
	model := Model{
		ctx:            t.Context(),
		text:           EnglishText(),
		repo:           classificationRepo{classifications: sampleClassifications(3)},
		mode:           viewDetail,
		current:        domain.Patent{Number: "US10218760B2", ClassificationLabel: "H04N21/430, H04N21/431, H04N21/432"},
		detailSelected: detailFieldIndex(EnglishText(), TextDetailClassification),
		width:          100,
		height:         20,
	}
	updated, _ := model.Update(teaKey(keyEnter))
	got := updated.(Model)
	if got.mode != viewClassifications {
		t.Fatalf("expected mode %q, got %q", viewClassifications, got.mode)
	}
	view := got.viewClassifications()
	if !strings.Contains(view, "H04N21/430") || !strings.Contains(view, "Page 1/1") {
		t.Fatalf("expected classification list, got %q", view)
	}
}

func TestOpenKeyOnDetailClassificationOpensClassificationList(t *testing.T) {
	model := Model{
		ctx:            t.Context(),
		text:           EnglishText(),
		repo:           classificationRepo{classifications: sampleClassifications(3)},
		mode:           viewDetail,
		current:        domain.Patent{Number: "US10218760B2", ClassificationLabel: "H04N21/430"},
		detailSelected: detailFieldIndex(EnglishText(), TextDetailClassification),
		width:          100,
		height:         20,
	}
	updated, _ := model.Update(teaKey(keyOpen))
	got := updated.(Model)
	if got.mode != viewClassifications {
		t.Fatalf("expected mode %q, got %q", viewClassifications, got.mode)
	}
}

func TestEnterOnClassificationListKeepsListOpen(t *testing.T) {
	model := Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    classificationRepo{classifications: sampleClassifications(3)},
		mode:    viewClassifications,
		current: domain.Patent{Number: "US10218760B2"},
		width:   100,
		height:  20,
	}
	updated, _ := model.Update(teaKey(keyEnter))
	got := updated.(Model)
	if got.mode != viewClassifications {
		t.Fatalf("expected mode %q, got %q", viewClassifications, got.mode)
	}
	if view := got.viewClassifications(); !strings.Contains(view, "Page 1/1") || !strings.Contains(view, "H04N21/430") {
		t.Fatalf("expected classification list to remain visible, got %q", view)
	}
}

func TestClassificationPageKeysMoveByPage(t *testing.T) {
	model := Model{
		ctx:                    t.Context(),
		text:                   EnglishText(),
		repo:                   classificationRepo{classifications: sampleClassifications(8)},
		mode:                   viewClassifications,
		current:                domain.Patent{Number: "US10218760B2"},
		classificationSelected: 0,
		height:                 12,
	}
	updated, _ := model.Update(teaKey(keyCtrlF))
	got := updated.(Model)
	if got.classificationSelected != 5 {
		t.Fatalf("expected page-down classification selection 5, got %d", got.classificationSelected)
	}
	updated, _ = got.Update(teaKey(keyCtrlD))
	got = updated.(Model)
	if got.classificationSelected != 0 {
		t.Fatalf("expected page-up classification selection 0, got %d", got.classificationSelected)
	}
}

func TestNumericPrefixMovesSelections(t *testing.T) {
	model := Model{
		mode:     viewList,
		patents:  []domain.Patent{{Number: "US1"}, {Number: "US2"}, {Number: "US3"}, {Number: "US4"}},
		selected: 0,
	}
	updated, _ := model.Update(teaKey("3"))
	updated, _ = updated.Update(teaKey(keyDown))
	got := updated.(Model)
	if got.selected != 3 {
		t.Fatalf("expected 3j to move to row 4, got %d", got.selected)
	}
	updated, _ = got.Update(teaKey("2"))
	updated, _ = updated.Update(teaKey(keyUp))
	got = updated.(Model)
	if got.selected != 1 {
		t.Fatalf("expected 2k to move to row 2, got %d", got.selected)
	}
}

func TestNumericPrefixGoesToAbsoluteRow(t *testing.T) {
	model := Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    classificationRepo{classifications: sampleClassifications(12)},
		mode:    viewClassifications,
		current: domain.Patent{Number: "US10218760B2"},
	}
	updated, _ := model.Update(teaKey("1"))
	updated, _ = updated.Update(teaKey("0"))
	updated, _ = updated.Update(teaKey(keyGoto))
	got := updated.(Model)
	if got.classificationSelected != 9 {
		t.Fatalf("expected 10g to jump to row 10, got %d", got.classificationSelected)
	}
}

func TestViewCitationsShowsIndexedRows(t *testing.T) {
	model := Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    citationRepo{edges: sampleCitationEdges(3)},
		mode:    viewCites,
		current: domain.Patent{Number: "US10218760B2"},
		width:   100,
		height:  20,
	}
	got := model.viewCitations(domain.RelationCites)
	if !strings.Contains(got, "#") {
		t.Fatalf("expected index header, got %q", got)
	}
	if !strings.Contains(got, ">") || !strings.Contains(got, "  1 US1000001B2") {
		t.Fatalf("expected selected indexed citation row, got %q", got)
	}
}

func TestViewReviewQueueShowsIndexedRows(t *testing.T) {
	model := Model{
		ctx:          t.Context(),
		text:         EnglishText(),
		repo:         citationRepo{edges: sampleCitationEdges(3)},
		mode:         viewReview,
		reviewStatus: domain.CitationStatusUnderReview,
		width:        100,
		height:       20,
	}
	got := model.viewReviewQueue()
	if !strings.Contains(got, "#") {
		t.Fatalf("expected index header, got %q", got)
	}
	if !strings.Contains(got, ">") || !strings.Contains(got, "  1 US1000001B2") {
		t.Fatalf("expected selected indexed review row, got %q", got)
	}
}

func TestRowIndexLabelUsesThreeSpacePaddedCharacters(t *testing.T) {
	tests := map[int]string{
		0:   "  1",
		11:  " 12",
		122: "123",
	}
	for input, want := range tests {
		if got := rowIndexLabel(input); got != want {
			t.Fatalf("expected rowIndexLabel(%d) = %q, got %q", input, want, got)
		}
	}
}

func TestDetailFieldsGroupInventors(t *testing.T) {
	model := Model{
		text: EnglishText(),
		current: domain.Patent{
			Assignee:  "Divx LLC",
			Inventors: []string{"Inventor One", "Inventor Two"},
		},
	}
	fields := model.detailFields()
	// Index 1 is now the grouped inventors field
	if fields[1].label != TextDetailInventors || fields[1].value != "Inventor One, Inventor Two" || fields[1].displayValue != "(2) Inventor One, Inventor Two" {
		t.Fatalf("unexpected grouped inventors field: %+v", fields[1])
	}
	if fields[1].jumpLabel != jumpLabelInventors {
		t.Fatalf("unexpected inventor jump label: %q", fields[1].jumpLabel)
	}
}

func TestDeleteShortcutEntersConfirmationMode(t *testing.T) {
	model := Model{
		mode:    viewList,
		patents: []domain.Patent{{Number: "US1"}},
	}
	updated, _ := model.Update(teaKey(keyDelete))
	got := updated.(Model)
	if got.mode != viewConfirmDelete {
		t.Fatalf("expected mode %q, got %q", viewConfirmDelete, got.mode)
	}
}

func TestConfirmationNoReturnsToList(t *testing.T) {
	model := Model{
		mode: viewConfirmDelete,
	}
	updated, _ := model.Update(teaKey(keyNo))
	got := updated.(Model)
	if got.mode != viewList {
		t.Fatalf("expected mode %q, got %q", viewList, got.mode)
	}
}

func teaKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

type emptyClassificationRepo struct {
	stubRepo
}

func (emptyClassificationRepo) ListClassifications(context.Context, string) ([]domain.Classification, error) {
	return nil, nil
}

type classificationRepo struct {
	stubRepo
	classifications []domain.Classification
}

func (r classificationRepo) ListClassifications(context.Context, string) ([]domain.Classification, error) {
	return r.classifications, nil
}

type citationRepo struct {
	stubRepo
	edges []domain.CitationEdge
}

func (r citationRepo) ListCitations(context.Context, string, string) ([]domain.CitationEdge, error) {
	return r.edges, nil
}

func (r citationRepo) ListCitationsByStatus(context.Context, string) ([]domain.CitationEdge, error) {
	return r.edges, nil
}

func sampleClassifications(count int) []domain.Classification {
	out := make([]domain.Classification, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, domain.Classification{
			System:      "CPC",
			Code:        fmt.Sprintf("H04N21/43%d", i),
			Description: fmt.Sprintf("Classification description %d", i+1),
		})
	}
	return out
}

func sampleCitationEdges(count int) []domain.CitationEdge {
	out := make([]domain.CitationEdge, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, domain.CitationEdge{
			SourcePatent:         "US10218760B2",
			TargetPatent:         fmt.Sprintf("US100000%dB2", i+1),
			RelationType:         domain.RelationCites,
			Status:               domain.CitationStatusUnderReview,
			TargetTitle:          fmt.Sprintf("Citation %d", i+1),
			TargetInventors:      []string{fmt.Sprintf("Inventor %d", i+1)},
			TargetExpirationDate: "2030-01-01",
		})
	}
	return out
}

func detailFieldIndex(text TextCatalog, label TextKey) int {
	model := Model{text: text}
	for i, field := range model.detailFields() {
		if field.label == label {
			return i
		}
	}
	return 0
}

type stubRepo struct{}

func (stubRepo) Close() error                                                  { return nil }
func (stubRepo) Setup(context.Context) error                                   { return nil }
func (stubRepo) UpsertPatentBundle(context.Context, domain.PatentBundle) error { return nil }
func (stubRepo) GetPatent(context.Context, string) (domain.Patent, error) {
	return domain.Patent{}, nil
}
func (stubRepo) ListPatents(context.Context, string) ([]domain.Patent, error) { return nil, nil }
func (stubRepo) ListCitations(context.Context, string, string) ([]domain.CitationEdge, error) {
	return nil, nil
}
func (stubRepo) ListCitationsByStatus(context.Context, string) ([]domain.CitationEdge, error) {
	return nil, nil
}
func (stubRepo) UpdateCitationStatus(context.Context, domain.CitationEdge, string) error { return nil }
func (stubRepo) UpdatePatentStatus(context.Context, string, string) error                { return nil }
func (stubRepo) UpdateClassificationDescription(context.Context, string, string, string) error {
	return nil
}
func (stubRepo) DeletePatent(context.Context, string) error { return nil }
func (stubRepo) ListClassifications(context.Context, string) ([]domain.Classification, error) {
	return nil, nil
}
func (stubRepo) ListTextSections(context.Context, string) ([]domain.PatentTextSection, error) {
	return nil, nil
}
func (stubRepo) AddNote(context.Context, string, string) (domain.ResearchNote, error) {
	return domain.ResearchNote{}, nil
}
func (stubRepo) ListNotes(context.Context, string) ([]domain.ResearchNote, error) { return nil, nil }
func (stubRepo) AddReference(context.Context, string, string) (domain.ReferenceEntry, error) {
	return domain.ReferenceEntry{}, nil
}
func (stubRepo) ListReferences(context.Context) ([]domain.ReferenceEntry, error) { return nil, nil }
func (stubRepo) AddAIArtifact(context.Context, domain.AIArtifact) (domain.AIArtifact, error) {
	return domain.AIArtifact{}, nil
}
func (stubRepo) ListAIArtifacts(context.Context, string) ([]domain.AIArtifact, error) {
	return nil, nil
}
