package overlay

import (
	"os"
	"path/filepath"
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

// TestFilePickerSurvivesBrokenSymlink reproduces the crash that bubbles/filepicker
// hit: a directory entry whose os.FileInfo cannot be stat-ed. The hand-rolled
// browser must list and render it without panicking, because it never stats.
func TestFilePickerSurvivesBrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.pdf"), []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/nonexistent/target", filepath.Join(dir, "broken.pdf")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	o := NewFilePicker(render.NewTheme(), "Pick", PurposeAddOfficeAction, dir, []string{".pdf"})
	// Rendering a dir containing a broken symlink must not panic.
	_ = o.View(80, 20)

	// Move to the real file and select it.
	var picked string
	for i := 0; i < len(o.entries); i++ {
		if o.entries[o.cursor].name == "real.pdf" {
			if cmd := o.activate(); cmd != nil {
				if m, ok := cmd().(FilePickedMsg); ok {
					picked = m.Path
				}
			}
			break
		}
		o.move(1)
	}
	if picked != filepath.Join(dir, "real.pdf") {
		t.Fatalf("selecting real.pdf emitted %q", picked)
	}
}

func TestFilePickerBrowseIntoSubdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "oa.txt"), []byte("rejected"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := NewFilePicker(render.NewTheme(), "Pick", PurposeAddOfficeAction, dir, []string{".txt"})

	// First non-".." entry is the "sub" directory; entering it should re-root.
	o.cursor = 1
	if o.entries[o.cursor].name != "sub" {
		t.Fatalf("expected sub dir at index 1, got %q", o.entries[o.cursor].name)
	}
	o.activate()
	if filepath.Base(o.dir) != "sub" {
		t.Fatalf("activate on a dir should descend, dir=%q", o.dir)
	}
	// ".." ascends back.
	o.cursor = 0
	o.activate()
	if o.dir != dir {
		t.Fatalf("'..' should ascend to %q, got %q", dir, o.dir)
	}
}
