package tui

import (
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
