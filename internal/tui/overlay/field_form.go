package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/tui/render"
)

// fieldKind classifies how a fieldForm field accepts input.
type fieldKind int

const (
	// fieldText is a free-text field edited with a visible cursor.
	fieldText fieldKind = iota
	// fieldChoice cycles through a fixed set of values; it has no free-text
	// entry and no cursor — keys delegate to the field's Cycle function.
	fieldChoice
)

// fieldFormField describes one labeled field in a fieldForm.
type fieldFormField struct {
	// Label is the left-column text shown for the field, already stripped of
	// any parenthetical help (the host shows that help in its own hint).
	Label string
	Kind  fieldKind
	// Cycle advances a fieldChoice value. key is the pressed key (a space or
	// empty string means the default "next" action); it returns the new value.
	Cycle func(current, key string) string
}

// fieldFormAction is what the host overlay should do after a key is handled.
type fieldFormAction int

const (
	fieldFormNone fieldFormAction = iota
	fieldFormSubmit
	fieldFormCancel
)

// fieldForm is the shared view/edit field editor behind the app's detail forms.
// A form starts in *view* mode: ↑/↓/tab/j/k navigate, ';' jumps, and stray keys
// are ignored — so a misplaced keystroke can no longer corrupt a value. Enter
// enters *edit* mode on the focused field, showing a block cursor at the
// insertion point; Enter commits and returns to view, Esc discards the edit.
// Ctrl+S asks the host to submit the whole form, Esc (in view mode) to cancel.
//
// The component owns navigation, the cursor, and jump state; the host owns what
// submit and cancel mean (each form emits its own message) via the returned
// fieldFormAction.
type fieldForm struct {
	fields  []fieldFormField
	values  []string
	focus   int
	editing bool
	cursor  int    // rune index into values[focus] while editing a text field
	backup  string // pre-edit snapshot of values[focus], restored on Esc-discard
	jump    JumpNavigator
}

// newFieldForm builds a form over the given fields, with empty values.
func newFieldForm(fields []fieldFormField) fieldForm {
	return fieldForm{fields: fields, values: make([]string, len(fields))}
}

// Value returns the current value of field i.
func (f *fieldForm) Value(i int) string {
	if i < 0 || i >= len(f.values) {
		return ""
	}
	return f.values[i]
}

// SetValue seeds field i (used by constructors to pre-fill).
func (f *fieldForm) SetValue(i int, v string) {
	if i < 0 || i >= len(f.values) {
		return
	}
	f.values[i] = v
}

// Focus reports the focused field index.
func (f *fieldForm) Focus() int { return f.focus }

// Editing reports whether the focused field is currently in edit mode.
func (f *fieldForm) Editing() bool { return f.editing }

// HandleKey processes one key press. It returns the action the host should take
// and always reports the key as consumed (the form is modal).
func (f *fieldForm) HandleKey(msg tea.KeyMsg) (fieldFormAction, bool) {
	if f.editing {
		return f.handleEditKey(msg), true
	}

	// View mode: jump navigation gets first crack at the key.
	if newFocus, handled := f.jump.HandleKey(msg, f.focus, len(f.values)); handled {
		f.focus = newFocus
		return fieldFormNone, true
	}

	n := len(f.values)
	switch msg.Type {
	case tea.KeyEsc:
		return fieldFormCancel, true
	case tea.KeyCtrlS:
		return fieldFormSubmit, true
	case tea.KeyEnter:
		if n > 0 {
			f.beginEdit()
		}
		return fieldFormNone, true
	}
	if n == 0 {
		return fieldFormNone, true
	}
	switch msg.Type {
	case tea.KeyTab, tea.KeyDown:
		f.focus = (f.focus + 1) % n
	case tea.KeyShiftTab, tea.KeyUp:
		f.focus = (f.focus - 1 + n) % n
	case tea.KeyRunes:
		switch msg.String() {
		case "j":
			f.focus = (f.focus + 1) % n
		case "k":
			f.focus = (f.focus - 1 + n) % n
		}
	}
	return fieldFormNone, true
}

// beginEdit enters edit mode on the focused field, placing the cursor at the
// end of the value and snapshotting it for a possible Esc-discard.
func (f *fieldForm) beginEdit() {
	f.editing = true
	f.backup = f.values[f.focus]
	f.cursor = len([]rune(f.values[f.focus]))
}

func (f *fieldForm) handleEditKey(msg tea.KeyMsg) fieldFormAction {
	switch msg.Type {
	case tea.KeyEnter:
		// Commit: the value is already mutated in place.
		f.editing = false
		f.cursor = 0
		return fieldFormNone
	case tea.KeyEsc:
		// Discard the in-progress edit.
		f.values[f.focus] = f.backup
		f.editing = false
		f.cursor = 0
		return fieldFormNone
	}

	if f.fields[f.focus].Kind == fieldChoice {
		switch msg.Type {
		case tea.KeySpace, tea.KeyRunes:
			if cycle := f.fields[f.focus].Cycle; cycle != nil {
				f.values[f.focus] = cycle(f.values[f.focus], msg.String())
			}
		}
		return fieldFormNone
	}

	// Free-text field: edit at the cursor.
	r := []rune(f.values[f.focus])
	switch msg.Type {
	case tea.KeyLeft:
		if f.cursor > 0 {
			f.cursor--
		}
	case tea.KeyRight:
		if f.cursor < len(r) {
			f.cursor++
		}
	case tea.KeyHome, tea.KeyCtrlA:
		f.cursor = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		f.cursor = len(r)
	case tea.KeyBackspace:
		if f.cursor > 0 {
			f.values[f.focus] = string(r[:f.cursor-1]) + string(r[f.cursor:])
			f.cursor--
		}
	case tea.KeyCtrlU:
		f.values[f.focus] = ""
		f.cursor = 0
	case tea.KeyRunes, tea.KeySpace:
		s := msg.String()
		f.values[f.focus] = string(r[:f.cursor]) + s + string(r[f.cursor:])
		f.cursor += len([]rune(s))
	}
	return fieldFormNone
}

// RenderFields draws the labeled field lines. labelW is the width of the left
// label column (its padding aligns the values).
func (f *fieldForm) RenderFields(theme render.Theme, maxW, labelW int) string {
	var b strings.Builder
	for i, field := range f.fields {
		lineNum := i + 1
		gutter := f.jump.GutterPrefix(lineNum)
		marker := "  "
		if i == f.focus {
			marker = "▸ "
		}
		prefix := marker + gutter + render.Pad(field.Label+":", labelW) + " "

		focused := i == f.focus
		var line string
		switch {
		case focused && f.editing && field.Kind == fieldText:
			// Compose styled segments so the block cursor (Selected) is not
			// nested inside the line-level Title styling.
			before, under, after := splitAtCursor(f.values[i], f.cursor)
			styled := theme.Title.Render(prefix+before) + theme.Selected.Render(under) + theme.Title.Render(after)
			line = render.Truncate(styled, maxW)
		case focused && f.editing && field.Kind == fieldChoice:
			line = theme.Title.Render(render.Truncate(prefix+"‹ "+f.values[i]+" ›", maxW))
		case focused:
			line = theme.Title.Render(render.Truncate(prefix+f.values[i], maxW))
		default:
			line = theme.Row.Render(render.Truncate(prefix+f.values[i], maxW))
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// Hint returns the mode-aware help line for the form.
func (f *fieldForm) Hint() string {
	if f.jump.Active {
		return f.jump.HintSuffix(f.focus, -1, false)
	}
	if f.editing {
		if f.fields[f.focus].Kind == fieldChoice {
			return "[space] cycle · [enter] ok · [esc] discard"
		}
		return "[type] edit · [←/→] move · [enter] ok · [esc] discard · [ctrl+u] clear"
	}
	return "[↑/↓ or tab] field · [enter] edit · [ctrl+s] save · [esc] cancel · [;] jump"
}

// splitAtCursor divides s at the cursor rune index into the text before the
// cursor, the single rune under it (a space when the cursor sits at the end),
// and the text after.
func splitAtCursor(s string, cursor int) (before, under, after string) {
	r := []rune(s)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(r) {
		cursor = len(r)
	}
	before = string(r[:cursor])
	if cursor < len(r) {
		under = string(r[cursor])
		after = string(r[cursor+1:])
	} else {
		under = " "
	}
	return before, under, after
}
