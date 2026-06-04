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

func TestFilePickerSearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "apple.txt"), []byte("apple"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "banana.txt"), []byte("banana"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := NewFilePicker(render.NewTheme(), "Pick", PurposeAddOfficeAction, dir, []string{".txt"})

	// Start search
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !o.showSearch {
		t.Fatal("expected showSearch to be true after '/'")
	}

	// Type query
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if o.searchQuery != "ba" {
		t.Fatalf("expected searchQuery 'ba', got %q", o.searchQuery)
	}

	// Verify filtered entries (should only be ".." and "banana.txt")
	if len(o.entries) != 2 {
		t.Fatalf("expected 2 filtered entries, got %d: %+v", len(o.entries), o.entries)
	}
	if o.entries[1].name != "banana.txt" {
		t.Errorf("expected banana.txt, got %q", o.entries[1].name)
	}

	// Esc exits search
	o.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if o.showSearch {
		t.Error("expected showSearch to be false after Esc")
	}
	if len(o.entries) != 3 {
		t.Fatalf("expected 3 entries after search cancel, got %d", len(o.entries))
	}
}

func TestFilePickerDirectPathEntry(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "target")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(subDir, "doc.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	o := NewFilePicker(render.NewTheme(), "Pick", PurposeAddOfficeAction, dir, []string{".txt"})

	// Enter search/path mode
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	
	// Input absolute path of directory
	o.searchQuery = subDir
	_, cmd, _ := o.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("expected nil command for directory navigation, got %T", cmd)
	}
	if o.dir != subDir {
		t.Errorf("expected directory to be changed to %q, got %q", subDir, o.dir)
	}

	// Enter search/path mode again
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	// Input absolute path of file
	o.searchQuery = filePath
	_, cmd, _ = o.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected file picked command")
	}
	msg := cmd()
	picked, ok := msg.(FilePickedMsg)
	if !ok {
		t.Fatalf("expected FilePickedMsg, got %T", msg)
	}
	if picked.Path != filePath {
		t.Errorf("expected picked path %q, got %q", filePath, picked.Path)
	}
}

func TestFilePickerTabCompletion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "apple.txt"), []byte("apple"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "apricot.txt"), []byte("apricot"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "banana.txt"), []byte("banana"), 0o644); err != nil {
		t.Fatal(err)
	}

	o := NewFilePicker(render.NewTheme(), "Pick", PurposeAddOfficeAction, dir, []string{".txt"})

	// Start search and type 'b'
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	// Press Tab -> unique match 'banana.txt'
	o.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	if o.searchQuery != "banana.txt" {
		t.Errorf("expected complete to 'banana.txt', got %q", o.searchQuery)
	}

	// Exit search, start again and type 'ap'
	o.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	// Press Tab -> matches 'apple.txt' and 'apricot.txt', LCP is 'ap'
	o.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	if o.searchQuery != "ap" {
		t.Errorf("expected LCP 'ap', got %q", o.searchQuery)
	}
}

func TestFilePickerTabCompletionDirectoryExpansion(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "photos")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "pic.txt"), []byte("pic"), 0o644); err != nil {
		t.Fatal(err)
	}

	o := NewFilePicker(render.NewTheme(), "Pick", PurposeAddOfficeAction, dir, []string{".txt"})

	// Type 'ph' and press Tab -> completes and descends into 'photos'
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyTab})

	if o.dir != subDir {
		t.Errorf("expected directory to expand and change to %q, got %q", subDir, o.dir)
	}
	if o.searchQuery != "" {
		t.Errorf("expected searchQuery to be reset to empty, got %q", o.searchQuery)
	}

	// Now in 'photos', type 'pi' and press Tab -> completes to 'pic.txt'
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	if o.searchQuery != "pic.txt" {
		t.Errorf("expected complete to 'pic.txt', got %q", o.searchQuery)
	}
}
