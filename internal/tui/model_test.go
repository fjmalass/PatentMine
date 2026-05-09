package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/storage"
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

func TestScreenHeaderUsesActiveModeTitle(t *testing.T) {
	model := Model{mode: viewCites}
	got := model.renderScreenHeader()
	if !strings.Contains(got, "Citations") {
		t.Fatalf("expected citations title, got %q", got)
	}
}

func TestHelpKeyOpensContextPopup(t *testing.T) {
	model := Model{
		mode:    viewCites,
		text:    EnglishText(),
		repo:    citationRepo{edges: sampleCitationEdges(2)},
		current: domain.Patent{Number: "US10218760B2"},
		width:   100,
		height:  20,
	}
	updated, _ := model.Update(teaKey(keyHelp))
	got := updated.(Model)
	if got.mode != viewHelpPopup {
		t.Fatalf("expected mode %q, got %q", viewHelpPopup, got.mode)
	}
	if !strings.Contains(got.View(), "Help · Citations") {
		t.Fatalf("expected contextual help title, got %q", got.View())
	}
	if !strings.Contains(got.View(), "This Screen") {
		t.Fatalf("expected contextual help popup, got %q", got.View())
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
	if got.detailSelected != 2 {
		t.Fatalf("expected detail selection 2, got %d", got.detailSelected)
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
		"L",
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
	got := model.viewClassifications()
	if !strings.Contains(got, "No CPC/USPC") {
		t.Fatalf("expected empty classification message, got %q", got)
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

func TestEnterOnClassificationListOpensDetail(t *testing.T) {
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
	if got.mode != viewClassificationDetail {
		t.Fatalf("expected mode %q, got %q", viewClassificationDetail, got.mode)
	}
}

func TestClassificationListViewRendersPopup(t *testing.T) {
	model := Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    classificationRepo{classifications: sampleClassifications(3)},
		mode:    viewClassifications,
		current: domain.Patent{Number: "US10218760B2"},
		width:   120,
		height:  40,
	}
	view := model.View()
	if !strings.Contains(view, "┌") {
		t.Fatalf("expected popup border in View(), got:\n%s", view)
	}
	if !strings.Contains(view, "H04N21/430") {
		t.Fatalf("expected classification code in View(), got:\n%s", view)
	}
}

func TestClassificationDetailViewRendersPopup(t *testing.T) {
	model := Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    classificationRepo{classifications: sampleClassifications(3)},
		mode:    viewClassificationDetail,
		current: domain.Patent{Number: "US10218760B2"},
		width:   120,
		height:  40,
	}
	view := model.View()
	if !strings.Contains(view, "┌") {
		t.Fatalf("expected popup border in View(), got:\n%s", view)
	}
	if !strings.Contains(view, "H04N21/430") {
		t.Fatalf("expected classification code in View(), got:\n%s", view)
	}
}

func TestPressLFromListOpensClassificationPopup(t *testing.T) {
	patents := []domain.Patent{{Number: "US10218760B2", Title: "Test Patent"}}
	model := Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    classificationRepo{classifications: sampleClassifications(3)},
		mode:    viewList,
		patents: patents,
		current: domain.Patent{},
		width:   120,
		height:  40,
	}
	updated, _ := model.Update(teaKey(keyClassification))
	got := updated.(Model)
	if got.mode != viewClassifications {
		t.Fatalf("expected mode %q after pressing %q, got %q", viewClassifications, keyClassification, got.mode)
	}
	view := got.View()
	if !strings.Contains(view, "┌") {
		t.Fatalf("expected popup border in View() after pressing %q, got:\n%s", keyClassification, view)
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
	updated, _ = updated.Update(teaKey(keyVimDown))
	got := updated.(Model)
	if got.selected != 3 {
		t.Fatalf("expected 3j to move to row 4, got %d", got.selected)
	}
	updated, _ = got.Update(teaKey("2"))
	updated, _ = updated.Update(teaKey(keyVimUp))
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

func TestOpenStoredCitationShowsPreviewOverlay(t *testing.T) {
	model := Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    storedCitationRepo{citationRepo: citationRepo{edges: sampleCitationEdges(1)}},
		mode:    viewCites,
		current: domain.Patent{Number: "US10218760B2"},
		width:   100,
		height:  20,
	}
	updated, _ := model.Update(teaKey(keyEnter))
	got := updated.(Model)
	if got.mode != viewPreview {
		t.Fatalf("expected mode %q, got %q", viewPreview, got.mode)
	}
	if got.pendingBundle.Patent.Number != "US1000001B2" {
		t.Fatalf("expected pending preview patent, got %+v", got.pendingBundle.Patent)
	}
	if view := got.View(); !strings.Contains(view, "Reference preview") || !strings.Contains(view, "US1000001B2") {
		t.Fatalf("expected preview overlay content, got %q", view)
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

func TestVisibleCitationEdgesReturnsCurrentPage(t *testing.T) {
	model := Model{
		ctx:           t.Context(),
		text:          EnglishText(),
		repo:          citationRepo{edges: sampleCitationEdges(12)},
		mode:          viewCites,
		current:       domain.Patent{Number: "US10218760B2"},
		citesSelected: 7,
		height:        12,
	}
	edges, err := model.visibleCitationEdges()
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 5 {
		t.Fatalf("expected 5 visible edges, got %d", len(edges))
	}
	if edges[0].TargetPatent != "US1000006B2" {
		t.Fatalf("expected current page to start at row 6, got %+v", edges[0])
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
	// Index 2 is now the grouped inventors field
	if fields[2].label != TextDetailInventors || fields[2].value != "Inventor One, Inventor Two" || fields[2].displayValue != "(2) Inventor One, Inventor Two" {
		t.Fatalf("unexpected grouped inventors field: %+v", fields[2])
	}
	if fields[2].jumpLabel != jumpLabelInventors {
		t.Fatalf("unexpected inventor jump label: %q", fields[2].jumpLabel)
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

func (emptyClassificationRepo) ListClassifications(context.Context, string, string) ([]domain.Classification, error) {
	return nil, nil
}

type classificationRepo struct {
	stubRepo
	classifications []domain.Classification
}

func (r classificationRepo) ListClassifications(context.Context, string, string) ([]domain.Classification, error) {
	return r.classifications, nil
}

type citationRepo struct {
	stubRepo
	edges []domain.CitationEdge
}

func (r citationRepo) ListCitations(context.Context, string, string, string, storage.ListCitationsOptions) ([]domain.CitationEdge, error) {
	return r.edges, nil
}

func (r citationRepo) ListCitationsByStatus(context.Context, string, string, storage.ListCitationsOptions) ([]domain.CitationEdge, error) {
	return r.edges, nil
}

type storedCitationRepo struct {
	citationRepo
}

func (r storedCitationRepo) GetPatent(_ context.Context, _ string, number string) (domain.Patent, error) {
	return domain.Patent{
		Number:              number,
		Title:               "Stored citation patent",
		Inventors:           []string{"Inventor One"},
		PublicationDate:     "2019-01-01",
		GrantDate:           "2020-01-01",
		ExpirationDate:      "2040-01-01",
		ExpirationEstimated: true,
		SourceURL:           "https://patents.google.com/patent/" + number + "/en",
	}, nil
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

func (stubRepo) Close() error                                                            { return nil }
func (stubRepo) Setup(context.Context) error                                             { return nil }
func (stubRepo) CreateProject(context.Context, domain.Project) error                     { return nil }
func (stubRepo) GetProject(context.Context, string) (domain.Project, error)              { return domain.Project{}, nil }
func (stubRepo) ListProjects(context.Context) ([]domain.Project, error)                  { return nil, nil }
func (stubRepo) UpdateProject(context.Context, domain.Project) error                     { return nil }
func (stubRepo) DeleteProject(context.Context, string) error                             { return nil }
func (stubRepo) AddPatentToProject(context.Context, string, string) error                { return nil }
func (stubRepo) RemovePatentFromProject(context.Context, string, string) error           { return nil }
func (stubRepo) UpsertPatentBundle(context.Context, string, domain.PatentBundle) error   { return nil }
func (stubRepo) GetPatent(context.Context, string, string) (domain.Patent, error)         { return domain.Patent{}, nil }
func (stubRepo) ListPatents(context.Context, string, storage.ListPatentsOptions) ([]domain.Patent, error) {
	return nil, nil
}

func (stubRepo) ListCitations(context.Context, string, string, string, storage.ListCitationsOptions) ([]domain.CitationEdge, error) {
	return nil, nil
}
func (stubRepo) ListCitationsByStatus(context.Context, string, string, storage.ListCitationsOptions) ([]domain.CitationEdge, error) {
	return nil, nil
}
func (stubRepo) UpdateCitationStatus(context.Context, string, domain.CitationEdge, string) error {
	return nil
}
func (stubRepo) UpdatePatentStatus(context.Context, string, string, string) error { return nil }
func (stubRepo) UpdateClassificationDescription(context.Context, string, string, string, string) error {
	return nil
}
func (stubRepo) DeletePatent(context.Context, string, string) error { return nil }
func (stubRepo) ListClassifications(context.Context, string, string) ([]domain.Classification, error) {
	return nil, nil
}
func (stubRepo) ListTextSections(context.Context, string, string) ([]domain.PatentTextSection, error) {
	return nil, nil
}
func (stubRepo) AddNote(context.Context, string, string, string) (domain.ResearchNote, error) {
	return domain.ResearchNote{}, nil
}
func (stubRepo) ListNotes(context.Context, string, string) ([]domain.ResearchNote, error) { return nil, nil }
func (stubRepo) AddReference(context.Context, string, string, string) (domain.ReferenceEntry, error) {
	return domain.ReferenceEntry{}, nil
}
func (stubRepo) ListReferences(context.Context, string) ([]domain.ReferenceEntry, error) { return nil, nil }
func (stubRepo) AddAIAnalysis(context.Context, string, domain.AIAnalysis) (domain.AIAnalysis, error) {
	return domain.AIAnalysis{}, nil
}
func (stubRepo) ListAIAnalyses(context.Context, string, string) ([]domain.AIAnalysis, error) {
	return nil, nil
}
func (stubRepo) AddProjectEvent(context.Context, domain.ProjectEvent) (domain.ProjectEvent, error) {
	return domain.ProjectEvent{}, nil
}
func (stubRepo) ListProjectEvents(context.Context, string) ([]domain.ProjectEvent, error) {
	return nil, nil
}
func (stubRepo) DeleteProjectEvent(context.Context, int64) error { return nil }
func (stubRepo) AddProjectInvoice(context.Context, domain.ProjectInvoice) (domain.ProjectInvoice, error) {
	return domain.ProjectInvoice{}, nil
}
func (stubRepo) ListProjectInvoices(context.Context, string) ([]domain.ProjectInvoice, error) {
	return nil, nil
}
func (stubRepo) UpdateProjectInvoice(context.Context, domain.ProjectInvoice) error { return nil }
func (stubRepo) DeleteProjectInvoice(context.Context, int64) error                { return nil }
func (stubRepo) CountUnpaidInvoicesByProject(context.Context) (map[string]int, error) {
	return nil, nil
}
func (stubRepo) GetSetting(context.Context, string) (string, error)        { return "", nil }
func (stubRepo) SetSetting(context.Context, string, string) error          { return nil }
