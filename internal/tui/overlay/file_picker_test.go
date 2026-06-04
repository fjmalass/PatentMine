package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/tui/render"
)

// Compile-time proof the picker is a usable, key-consuming overlay.
var (
	_ Overlay    = (*FilePicker)(nil)
	_ KeyHandler = (*FilePicker)(nil)
)

func TestFilePickerEscCancels(t *testing.T) {
	o := NewFilePicker(render.NewTheme(), "Add Office Action", PurposeAddOfficeAction, ".", []string{".pdf", ".txt"})
	if o.Title() != "Add Office Action" {
		t.Errorf("title = %q", o.Title())
	}
	// A pure KeyHandler overlay binds no catalog commands.
	if len(o.Handles()) != 0 {
		t.Errorf("Handles() should be empty, got %v", o.Handles())
	}

	_, cmd, consumed := o.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !consumed {
		t.Fatal("Esc must be consumed")
	}
	if cmd == nil {
		t.Fatal("Esc must emit a command")
	}
	if _, ok := cmd().(CloseOverlayMsg); !ok {
		t.Fatalf("Esc must emit CloseOverlayMsg, got %T", cmd())
	}
}
