package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
)

func TestMatterDocuments_HandleKey_Import(t *testing.T) {
	theme := render.NewTheme()
	o := NewMatterDocuments(nil, theme, "p-1", nil)
	o.loading = false // simulate loaded state

	// Press 'l' to trigger document load
	updated, cmd, consumed := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if !consumed {
		t.Error("expected 'l' key to be consumed")
	}

	oUpdated, ok := updated.(*MatterDocuments)
	if !ok {
		t.Fatalf("expected *MatterDocuments, got %T", updated)
	}

	if cmd == nil {
		t.Fatal("expected a command to trigger import message")
	}

	msg := cmd()
	if _, ok := msg.(StartDocumentImportMsg); !ok {
		t.Errorf("expected StartDocumentImportMsg, got %T", msg)
	}

	// Verify reload when MatterDocumentImportedMsg is received
	oUpdated.msg = ""
	updated2, cmd2 := oUpdated.Update(pane.MatterDocumentImportedMsg{Name: "test_doc.pdf"})
	oUpdated2, ok := updated2.(*MatterDocuments)
	if !ok {
		t.Fatalf("expected *MatterDocuments, got %T", updated2)
	}

	if oUpdated2.msg != "loaded document: test_doc.pdf" {
		t.Errorf("expected status msg to be set, got %q", oUpdated2.msg)
	}

	if cmd2 == nil {
		t.Fatal("expected load command to be returned to refresh the documents list")
	}
}

// TestMatterDocumentsDualNavigation guards the port to the shared dualTable
// controller: the two panes split associated vs all, tab switches panes, and the
// selection + assign/remove act on the correct pane.
func TestMatterDocumentsDualNavigation(t *testing.T) {
	oa := domain.OfficeAction{ID: "oa1", Project: "p1"}
	o := NewMatterDocuments(nil, render.NewTheme(), "p1", &oa)

	assoc := domain.MatterDocument{ID: "d1", Project: "p1", DisplayName: "Assigned Ref", OfficeActionID: "oa1"}
	other := domain.MatterDocument{ID: "d2", Project: "p1", DisplayName: "Other Doc"}
	ov, _ := o.Update(matterDocsLoadedMsg{
		projectItems: []domain.MatterDocument{assoc, other},
		allItems:     []domain.MatterDocument{assoc, other},
	})
	o = ov.(*MatterDocuments)

	if len(o.items0) != 1 || o.items0[0].ID != "d1" {
		t.Fatalf("associated pane = %+v, want [d1]", o.items0)
	}
	if len(o.items1) != 1 || o.items1[0].ID != "d2" {
		t.Fatalf("all pane = %+v, want [d2]", o.items1)
	}

	// Starts on the associated pane; selection is d1.
	if sel, ok := o.selected(); !ok || sel.ID != "d1" {
		t.Fatalf("initial selection = %+v ok=%v, want d1", sel, ok)
	}

	// tab switches to the all pane; selection is d2.
	o.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	if o.dt.Active() != 1 {
		t.Fatalf("after tab active pane = %d, want 1", o.dt.Active())
	}
	if sel, ok := o.selected(); !ok || sel.ID != "d2" {
		t.Fatalf("after tab selection = %+v, want d2", sel)
	}

	// 'a' on the all pane associates the selected doc to the OA (emits a command).
	_, cmd, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !handled || cmd == nil {
		t.Fatal("'a' on the all pane should emit an associate command")
	}
}
