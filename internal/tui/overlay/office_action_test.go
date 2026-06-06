package overlay

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// Compile-time proof both overlays are usable, key-consuming overlays.
var (
	_ Overlay    = (*OfficeActionList)(nil)
	_ KeyHandler = (*OfficeActionList)(nil)
	_ Overlay    = (*OfficeActionEditor)(nil)
	_ KeyHandler = (*OfficeActionEditor)(nil)
	_ Overlay    = (*OfficeActionMetaForm)(nil)
	_ KeyHandler = (*OfficeActionMetaForm)(nil)
)

func mustParseDate(s string) time.Time {
	t, err := time.Parse(domain.DateLayout, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestOfficeActionEditorCtrlWSwitchesFocus(t *testing.T) {
	oa := domain.OfficeAction{ID: "oa-1", ExtractedText: "Claims 1-3 rejected.", Notes: "traverse"}
	o := NewOfficeActionEditor(nil, render.NewTheme(), oa)
	if !o.focusNotes {
		t.Fatal("editor should start focused on the notes pane")
	}
	// ctrl-w h → examiner (left)
	o.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlW})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if o.focusNotes {
		t.Fatal("ctrl-w h should focus the examiner (left) pane")
	}
	// ctrl-w l → notes (right)
	o.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlW})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if !o.focusNotes {
		t.Fatal("ctrl-w l should focus the notes (right) pane")
	}
}

func TestOfficeActionEditorExaminerReadOnly(t *testing.T) {
	oa := domain.OfficeAction{ID: "oa-1", ExtractedText: "original", Notes: ""}
	o := NewOfficeActionEditor(nil, render.NewTheme(), oa)
	// Focus the examiner pane and try to edit it.
	o.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlW})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")}) // try insert
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Z")})
	if o.examiner.Value() != "original" {
		t.Fatalf("examiner pane must be read-only, got %q", o.examiner.Value())
	}
}

func TestOfficeActionEditorEscCloses(t *testing.T) {
	o := NewOfficeActionEditor(nil, render.NewTheme(), domain.OfficeAction{ID: "oa-1"})
	_, cmd, consumed := o.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !consumed || cmd == nil {
		t.Fatal("esc should be consumed and emit a command")
	}
	if _, ok := cmd().(CloseOverlayMsg); !ok {
		t.Fatalf("esc in normal mode should close, got %T", cmd())
	}
}

func TestOfficeActionListEnterOpens(t *testing.T) {
	o := NewOfficeActionList(nil, render.NewTheme(), "p1")
	o.loading = false
	o.items = []domain.OfficeAction{{ID: "oa-A"}, {ID: "oa-B"}}
	o.cursor = 1

	_, cmd, consumed := o.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !consumed || cmd == nil {
		t.Fatal("enter should be consumed and emit a command")
	}
	msg, ok := cmd().(OpenOfficeActionMsg)
	if !ok {
		t.Fatalf("enter should emit OpenOfficeActionMsg, got %T", cmd())
	}
	if msg.OA.ID != "oa-B" {
		t.Fatalf("enter should open the cursor row, got %q", msg.OA.ID)
	}
}

func TestOfficeActionMetaFormLoad(t *testing.T) {
	proj := domain.Project{
		ID:                "p1",
		ApplicationNumber: "12/345,678",
		ArtUnit:           "1600",
		Examiners:         []domain.ProjectExaminer{{Name: "Smith"}},
	}
	// Initializing with no path argument
	o := NewOfficeActionMetaForm(render.NewTheme(), proj, "")
	if o.isEdit {
		t.Fatal("should not be edit mode")
	}
	if len(o.form.values) != 4 {
		t.Fatalf("expected 4 values, got %d", len(o.form.values))
	}
	if o.form.Value(oaFieldName) != "" {
		t.Fatalf("expected empty action name, got %q", o.form.Value(oaFieldName))
	}
	if o.form.Value(oaFieldType) != string(domain.OANonFinal) {
		t.Fatalf("expected non_final, got %q", o.form.Value(oaFieldType))
	}
	if o.form.Value(oaFieldMailDate) != "" {
		t.Fatalf("expected empty date, got %q", o.form.Value(oaFieldMailDate))
	}
	if o.form.Value(oaFieldAppNumber) != "12/345,678" {
		t.Fatalf("expected application number '12/345,678', got %q", o.form.Value(oaFieldAppNumber))
	}

	// Safety: a form arrives in *view* mode, so stray keystrokes on the Type
	// field must NOT cycle it — the user has to enter edit mode deliberately.
	o.form.focus = oaFieldType
	o.HandleKey(tea.KeyMsg{Type: tea.KeySpace})
	if o.form.Value(oaFieldType) != string(domain.OANonFinal) {
		t.Fatalf("view-mode space must not cycle type, got %q", o.form.Value(oaFieldType))
	}

	// Enter toggles edit mode on the focused field.
	o.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !o.form.Editing() {
		t.Fatal("enter should toggle edit mode on the focused field")
	}

	// Backspace / ctrl-u must not change a choice field.
	o.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if o.form.Value(oaFieldType) != string(domain.OANonFinal) {
		t.Fatalf("expected non_final after backspace, got %q", o.form.Value(oaFieldType))
	}

	// In edit mode, space cycles to final.
	o.HandleKey(tea.KeyMsg{Type: tea.KeySpace})
	if o.form.Value(oaFieldType) != string(domain.OAFinal) {
		t.Fatalf("expected final after cycling, got %q", o.form.Value(oaFieldType))
	}

	// 'r' sets restriction.
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if o.form.Value(oaFieldType) != string(domain.OARestriction) {
		t.Fatalf("expected restriction after pressing 'r', got %q", o.form.Value(oaFieldType))
	}

	// 'n' when current is 'restriction' sets 'non_final'.
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if o.form.Value(oaFieldType) != string(domain.OANonFinal) {
		t.Fatalf("expected non_final, got %q", o.form.Value(oaFieldType))
	}

	// 'n' when current is 'non_final' toggles to 'notice_of_allowance'.
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if o.form.Value(oaFieldType) != string(domain.OANoticeOfAllowance) {
		t.Fatalf("expected notice_of_allowance, got %q", o.form.Value(oaFieldType))
	}

	// Enter commits and returns to view mode without changing the value.
	o.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if o.form.Editing() {
		t.Fatal("enter should commit and leave edit mode")
	}
	if o.form.Value(oaFieldType) != string(domain.OANoticeOfAllowance) {
		t.Fatalf("commit must keep the value, got %q", o.form.Value(oaFieldType))
	}

	// Test Jump Mode navigation (view mode):
	// Start at focus 1 (Type)
	o.form.focus = oaFieldType

	// 1. Jump to line 3 using ';3g'
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(";")})
	if !o.form.jump.Active {
		t.Fatal("expected jump.Active to be true after ';'")
	}
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if o.form.jump.Active {
		t.Fatal("expected jump.Active to be false after movement")
	}
	if o.form.focus != oaFieldMailDate { // line 3 → field index 2
		t.Fatalf("expected focus to jump to the Mail Date field, got %d", o.form.focus)
	}

	// 2. Move down 2 fields using ';2j' — wraps from Mail Date back to Action Name.
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(";")})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if o.form.focus != oaFieldName { // (2 + 2) % 4 = 0
		t.Fatalf("expected focus to wrap to the Action Name field, got %d", o.form.focus)
	}

	// 3. Move to the last field using ';G'.
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(";")})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if o.form.focus != oaFieldAppNumber {
		t.Fatalf("expected focus to move to the last (Application Number) field, got %d", o.form.focus)
	}

	// 4. Move to the first field using ';gg'.
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(";")})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if !o.form.jump.PendingG {
		t.Fatal("expected jump.PendingG to be true after first 'g'")
	}
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if o.form.focus != oaFieldName {
		t.Fatalf("expected focus to move to the Action Name field, got %d", o.form.focus)
	}
}

func TestOfficeActionMetaFormEdit(t *testing.T) {
	oa := domain.OfficeAction{
		ID:                "oa-edit",
		Name:              "My Edit",
		Examiner:          "Jones",
		MailDate:          mustParseDate("2026-01-02"),
		Type:              domain.OAFinal,
		ArtUnit:           "1700",
		ApplicationNumber: "11/222,333",
		Status:            domain.OAStatusResponded,
	}
	o := NewOfficeActionEditForm(render.NewTheme(), "p1", oa)
	if !o.isEdit {
		t.Fatal("should be edit mode")
	}
	// The edit form carries the four import fields plus an edit-only Status field.
	if len(o.form.values) != 5 {
		t.Fatalf("expected 5 values for edit form, got %d", len(o.form.values))
	}
	if o.form.Value(oaFieldName) != "My Edit" {
		t.Fatalf("expected My Edit, got %q", o.form.Value(oaFieldName))
	}
	if o.form.Value(oaFieldType) != string(domain.OAFinal) {
		t.Fatalf("expected final, got %q", o.form.Value(oaFieldType))
	}
	if o.form.Value(oaFieldMailDate) != "2026-01-02" {
		t.Fatalf("expected 2026-01-02, got %q", o.form.Value(oaFieldMailDate))
	}
	if o.form.Value(oaFieldAppNumber) != "11/222,333" {
		t.Fatalf("expected 11/222,333, got %q", o.form.Value(oaFieldAppNumber))
	}
	if o.form.Value(oaFieldStatus) != string(domain.OAStatusResponded) {
		t.Fatalf("expected responded, got %q", o.form.Value(oaFieldStatus))
	}
}

