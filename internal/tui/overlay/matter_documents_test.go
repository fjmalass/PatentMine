package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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
