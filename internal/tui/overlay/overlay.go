// Package overlay holds the TUI's modal layers. Like a Pane, each Overlay owns
// only its own state; the App owns the overlay stack and draws each overlay
// over a dimmed copy of the pane behind it.
package overlay

import (
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/observability"
)

// Overlay is one modal layer.
type Overlay interface {
	// Title is shown at the top of the overlay box.
	Title() string
	// Command applies a resolved command intent (e.g. scrolling).
	Command(id command.ID, repeat int) (Overlay, tea.Cmd)
	// Handles reports every command ID the overlay services, so the App's
	// wiring check can confirm overlay key bindings reach a handler.
	Handles() []command.ID
	// View renders the overlay body within maxW columns by maxH rows.
	View(maxW, maxH int) string
}

// KeyHandler overlays consume raw key events directly, which prompt-like
// overlays need for text entry.
type KeyHandler interface {
	HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool)
}

// ScopeSource reports the underlying pane scope an overlay was opened from.
// Prompt overlays use this so their filtered command list reflects the screen
// behind them rather than the generic overlay scope.
type ScopeSource interface {
	SourceScope() command.Scope
}

// cmdHandler carries out one command for an overlay.
type cmdHandler func(repeat int) tea.Cmd

// handlerIDs returns the sorted command IDs of a handler table.
func handlerIDs(handlers map[command.ID]cmdHandler) []command.ID {
	ids := make([]command.ID, 0, len(handlers))
	for id := range handlers {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// PromptMode distinguishes palette search from direct command entry.
type PromptMode string

const (
	PromptPalette PromptMode = "palette"
	PromptDirect  PromptMode = "direct"
)

// PromptSubmitMsg asks the app to execute one typed command string.
type PromptSubmitMsg struct {
	Input string
}

// PromptCloseMsg asks the app to close the focused prompt overlay.
type PromptCloseMsg struct{}

// Purpose names what a TextInput overlay is collecting, so the App routes the
// submitted value to the right action.
type Purpose string

const (
	// PurposeCreateProject collects a name for a new project.
	PurposeCreateProject   Purpose = "create-project"
	PurposeEditIDSKind     Purpose = "edit-ids-kind"
	PurposeEditIDSCountry  Purpose = "edit-ids-country"
	PurposeEditIDSPassages Purpose = "edit-ids-passages"
	PurposeEditIDSNotes    Purpose = "edit-ids-notes"
)

// TextSubmitMsg carries a value entered in a TextInput overlay.
type TextSubmitMsg struct {
	Purpose Purpose
	Value   string
}

// CloseOverlayMsg asks the app to close the focused overlay.
type CloseOverlayMsg struct{}

// OpenPatentDetailMsg asks the app to open the detail view of a patent.
type OpenPatentDetailMsg struct {
	Number domain.PatentNumber
}

// ReplayActivityMsg asks the app to open a replayed activity target and record
// that the activity was played from the replay UI.
type ReplayActivityMsg struct {
	Number domain.PatentNumber
	Record observability.Record
}

// ApplyClassificationFilterMsg asks the app to apply a classification code
// filter to the pane behind the overlay. Codes are the marked-row set in the
// overlay's current sort order; they are joined as a space-separated string
// and routed through the pane's existing classification filter pipeline.
type ApplyClassificationFilterMsg struct {
	Codes []string
}

// OpenTagPatentOverlayMsg asks the app to open the patent tag manager.
type OpenTagPatentOverlayMsg struct {
	Patents []domain.PatentNumber
}

// PctSize computes an overlay size as a percentage of the terminal, clamped
// between min and term-2. Every overlay that implements DynamicSize should
// delegate to this rather than hand-rolling the arithmetic.
func PctSize(termW, termH, pctW, pctH, minW, minH int) (w, h int) {
	w = max(minW, termW*pctW/100)
	h = max(minH, termH*pctH/100)
	if termW > 2 {
		w = min(w, termW-2)
	}
	if termH > 2 {
		h = min(h, termH-2)
	}
	return w, h
}
