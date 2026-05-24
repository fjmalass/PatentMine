package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/observability"
	"patentmine/internal/tui/render"
)

// HistoryOverlay displays the persistent timeline of recent searches, project switches, and patent details views.
type HistoryOverlay struct {
	theme   render.Theme
	records []observability.Record
	cursor  int // local selection cursor, 0 maps to the newest item
}

// NewHistoryOverlay builds a HistoryOverlay.
func NewHistoryOverlay(theme render.Theme, records []observability.Record) *HistoryOverlay {
	return &HistoryOverlay{
		theme:   theme,
		records: records,
		cursor:  0,
	}
}

// Title implements Overlay.
func (h *HistoryOverlay) Title() string { return "Activity History" }

// Command implements Overlay.
func (h *HistoryOverlay) Command(command.ID, int) (Overlay, tea.Cmd) { return h, nil }

// Handles implements Overlay.
func (h *HistoryOverlay) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler.
func (h *HistoryOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	n := len(h.records)
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
		rec := h.records[h.cursor]
		return h, func() tea.Msg {
			return HistoryReplayMsg{Record: rec}
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
	n := len(h.records)
	if n == 0 {
		return h.theme.Dim.Render("No history entries found.")
	}

	var b strings.Builder
	// Visible limit. Overlay height is boxHeight-chrome.
	visibleLimit := maxH - 4
	if visibleLimit < 2 {
		visibleLimit = 2
	}

	start := 0
	end := n
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

	// Header
	b.WriteString(h.theme.Dim.Render(render.Pad("  Time     Project       Act  Details", maxW)))
	b.WriteString("\n")

	for idx := start; idx < end; idx++ {
		rec := h.records[idx]

		// Prefix
		prefix := "  "
		style := h.theme.Row
		if idx == h.cursor {
			prefix = " >"
			style = h.theme.Selected
		}

		// Project ID
		projID := "-"
		if pid, ok := rec.Metadata["project"].(string); ok && pid != "" {
			projID = pid
		} else if rec.Action == "project.switch" {
			projID = rec.EntityID
		}
		// Capped project ID for alignment
		if len(projID) > 12 {
			projID = projID[:11] + "…"
		}

		// Icon and Details
		icon := "❓"
		details := ""

		switch rec.Action {
		case "filter.apply":
			icon = "🔍"
			details = fmt.Sprintf("Search: %q", rec.EntityID)
		case "project.switch":
			icon = "📁"
			projName := rec.EntityID
			if name, ok := rec.Metadata["project_name"].(string); ok && name != "" {
				projName = name
			}
			details = fmt.Sprintf("Switch Project to %q", projName)
		case "ui.focus":
			icon = "📄"
			title := ""
			if t, ok := rec.Metadata["title"].(string); ok && t != "" {
				title = " - " + t
			}
			details = fmt.Sprintf("Viewed Patent %s%s", rec.EntityID, title)
		}

		// Row content: time, project, icon, details
		timeStr := localClock(rec.Timestamp)
		line := fmt.Sprintf("%s %s %-12s %s  %s", prefix, timeStr, projID, icon, details)
		b.WriteString(style.Render(render.Pad(line, maxW)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(h.theme.Dim.Render(render.Pad("  [j/k] Move  [Enter] Replay/Confirm  [q/Esc] Close", maxW)))
	return b.String()
}
