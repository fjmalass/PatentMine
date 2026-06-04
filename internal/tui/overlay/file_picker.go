package overlay

import (
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/tui/render"
)

// PurposeAddOfficeAction routes a picked file to the :add.officeaction import.
const PurposeAddOfficeAction Purpose = "add-officeaction"

// FilePickedMsg reports a file chosen in a FilePicker overlay. Purpose tells the
// App which action to route the path to.
type FilePickedMsg struct {
	Purpose Purpose
	Path    string
}

// FilePicker is a modal file browser over the local filesystem. It is a
// KeyHandler overlay — it consumes navigation keys itself (arrows / j,k / enter
// / backspace, the filepicker's own bindings) — and emits a FilePickedMsg when a
// file is chosen, or CloseOverlayMsg on Esc. The path is local to this machine,
// which is exactly what the daemon needs: it shares the filesystem (unix socket)
// and copies the file itself, so only the path travels over RPC.
type FilePicker struct {
	theme   render.Theme
	title   string
	purpose Purpose
	picker  filepicker.Model
	note    string
}

// NewFilePicker builds a file picker rooted at startDir. allowedTypes restricts
// which extensions are selectable (others are shown but disabled); empty allows
// any file.
func NewFilePicker(theme render.Theme, title string, purpose Purpose, startDir string, allowedTypes []string) *FilePicker {
	fp := filepicker.New()
	if startDir != "" {
		fp.CurrentDirectory = startDir
	}
	fp.AllowedTypes = allowedTypes
	fp.ShowHidden = false
	fp.ShowSize = true
	fp.DirAllowed = false
	fp.FileAllowed = true
	fp.AutoHeight = false
	return &FilePicker{theme: theme, title: title, purpose: purpose, picker: fp}
}

func (o *FilePicker) Title() string { return o.title }

// Handles returns nil: the picker consumes its keys via HandleKey and binds no
// catalog commands, so it needs no entry in the wiring validator.
func (o *FilePicker) Handles() []command.ID { return nil }

func (o *FilePicker) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

// Init kicks off the first directory read.
func (o *FilePicker) Init() tea.Cmd { return o.picker.Init() }

// Update routes async messages — chiefly the filepicker's own directory-read
// results — back into the picker so the listing populates and refreshes.
func (o *FilePicker) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	var cmd tea.Cmd
	o.picker, cmd = o.picker.Update(msg)
	return o, cmd
}

func (o *FilePicker) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if msg.Type == tea.KeyEsc {
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	var cmd tea.Cmd
	o.picker, cmd = o.picker.Update(msg)
	if did, path := o.picker.DidSelectFile(msg); did {
		purpose := o.purpose
		return o, func() tea.Msg { return FilePickedMsg{Purpose: purpose, Path: path} }, true
	}
	if did, path := o.picker.DidSelectDisabledFile(msg); did {
		o.note = path + " is not a selectable type (pick a .pdf or .txt)"
		return o, cmd, true
	}
	o.note = ""
	return o, cmd, true
}

func (o *FilePicker) View(maxW, maxH int) string {
	// Reserve rows for the directory line, the hint, and any note.
	body := max(maxH-4, 3)
	o.picker.SetHeight(body)

	var b strings.Builder
	b.WriteString(o.theme.Dim.Render(render.Truncate("dir: "+o.picker.CurrentDirectory, maxW)))
	b.WriteByte('\n')
	if o.note != "" {
		b.WriteString(o.theme.Warn.Render(render.Truncate(o.note, maxW)))
		b.WriteByte('\n')
	}
	b.WriteString(o.picker.View())
	b.WriteByte('\n')
	b.WriteString(o.theme.Dim.Render(render.Truncate(
		"↑/↓ or j/k move · enter open/select · ←/backspace up · esc cancel", maxW)))
	return b.String()
}
