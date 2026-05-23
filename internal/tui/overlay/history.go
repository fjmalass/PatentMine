package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// HistorySelectMsg signals that a patent has been selected from the history list.
type HistorySelectMsg struct {
	Index  int
	Number domain.PatentNumber
}

// HistoryOverlay displays the list of recently visited patents and lets the user choose one.
type HistoryOverlay struct {
	theme         render.Theme
	history       []domain.PatentNumber
	historyCursor int
	cursor        int // local selection cursor, 0 maps to the newest item in history
}

// NewHistoryOverlay builds a HistoryOverlay.
func NewHistoryOverlay(theme render.Theme, history []domain.PatentNumber, historyCursor int) *HistoryOverlay {
	// Initialize local cursor so that it highlights the currently active historical patent
	localCursor := 0
	if len(history) > 0 {
		localCursor = len(history) - 1 - historyCursor
		if localCursor < 0 {
			localCursor = 0
		}
		if localCursor >= len(history) {
			localCursor = len(history) - 1
		}
	}
	return &HistoryOverlay{
		theme:         theme,
		history:       history,
		historyCursor: historyCursor,
		cursor:        localCursor,
	}
}

// Title implements Overlay.
func (h *HistoryOverlay) Title() string { return "Patent History" }

// Command implements Overlay: HistoryOverlay handles key navigation directly.
func (h *HistoryOverlay) Command(command.ID, int) (Overlay, tea.Cmd) { return h, nil }

// Handles implements Overlay.
func (h *HistoryOverlay) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler.
func (h *HistoryOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	n := len(h.history)
	if n == 0 {
		return h, func() tea.Msg { return CloseOverlayMsg{} }, true
	}

	switch msg.String() {
	case "j", "down":
		h.cursor++
		if h.cursor >= n {
			h.cursor = 0 // wrap around
		}
		return h, nil, true
	case "k", "up":
		h.cursor--
		if h.cursor < 0 {
			h.cursor = n - 1 // wrap around
		}
		return h, nil, true
	case "enter":
		targetIndex := n - 1 - h.cursor
		targetNumber := h.history[targetIndex]
		return h, func() tea.Msg {
			return HistorySelectMsg{Index: targetIndex, Number: targetNumber}
		}, true
	case "q", "esc":
		return h, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	if msg.Type == tea.KeyEscape {
		return h, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	return h, nil, true
}

// View implements Overlay.
func (h *HistoryOverlay) View(maxW, maxH int) string {
	n := len(h.history)
	if n == 0 {
		return h.theme.Dim.Render("No history entries.")
	}

	var b strings.Builder
	// We want to fit within the maxH budget. Overlay height is boxHeight-chrome.
	// We list items in newest-first order.
	start := 0
	end := n
	visibleLimit := maxH - 2 // leave room for headers/footers or instructions
	if visibleLimit < 2 {
		visibleLimit = 2
	}

	// Sliding window for long history lists
	if n > visibleLimit {
		half := visibleLimit / 2
		start = h.cursor - half
		if start < 0 {
			start = 0
		}
		end = start + visibleLimit
		if end > n {
			end = n
			start = end - visibleLimit
		}
	}

	b.WriteString(h.theme.Dim.Render(render.Pad("  Recent Visited Patents", maxW)))
	b.WriteString("\n")

	for idx := start; idx < end; idx++ {
		// history index is n - 1 - idx
		histIdx := n - 1 - idx
		number := h.history[histIdx]

		prefix := "  "
		style := h.theme.Row
		if idx == h.cursor {
			prefix = " >"
			style = h.theme.Title // bold/active style
		}

		activeMarker := "  "
		if histIdx == h.historyCursor {
			activeMarker = " •" // active historical item
		}

		line := fmt.Sprintf("%s%s %s", prefix, activeMarker, number.String())
		b.WriteString(style.Render(render.Pad(line, maxW)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(h.theme.Dim.Render(render.Pad("  [j/k] Move  [Enter] Select  [q/Esc] Close", maxW)))
	return b.String()
}
