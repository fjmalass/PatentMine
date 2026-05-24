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

// OverlaySize implements DynamicSize so the history overlay takes 80% of the screen.
func (h *HistoryOverlay) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, 80, 80, 40, 10)
}

// HandleKey implements KeyHandler.
func (h *HistoryOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	n := len(h.records)
	if n == 0 {
		return h, func() tea.Msg { return CloseOverlayMsg{} }, true
	}

	switch msg.String() {
	case "j", "down":
		h.cursor = min(h.cursor+1, n-1) // no wrap around (clamp at bottom)
		return h, nil, true
	case "k", "up":
		h.cursor = max(h.cursor-1, 0) // no wrap around (clamp at top)
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
	// Account for Box horizontal Padding(0, 1) to prevent any Lipgloss word-wrapping
	maxW = max(maxW-2, 10)

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

	// Header: "  "<2> + "Time    "<8> + " "<1> + "Project     "<12> + " "<1> + icon<2> + " "<1> + "Details"
	// Columns align exactly with row content below.
	hdr := "  Time     Project      " + h.theme.Glyphs.HistColType + "  Details"
	b.WriteString(h.theme.Dim.Render(render.Pad(hdr, maxW)))
	b.WriteString("\n")

	for idx := start; idx < end; idx++ {
		rec := h.records[idx]

		// Prefix
		prefix := "  "
		style := h.theme.Row
		if idx == h.cursor {
			prefix = "→ "
			style = h.theme.Selected
		}

		// For membership/tag actions the entity ID encodes "project/num[/tag]".
		// Split once and reuse for both projName and numStr fallbacks.
		entityParts := strings.Split(rec.EntityID, "/")
		isMembershipOrTag := rec.Action == "membership.set_state" ||
			rec.Action == "patent.tag_assign" ||
			rec.Action == "patent.tag_remove"

		// Project Name
		projName := "-"
		if name, ok := rec.Metadata["project_name"].(string); ok && name != "" {
			projName = name
		} else if pid, ok := rec.Metadata["project"].(string); ok && pid != "" {
			projName = pid
		} else if rec.Action == "project.switch" {
			projName = rec.EntityID
			if name, ok := rec.Metadata["project_name"].(string); ok && name != "" {
				projName = name
			}
		} else if isMembershipOrTag && len(entityParts) >= 1 && entityParts[0] != "" {
			projName = entityParts[0]
		}
		// Capped project name for alignment
		if len(projName) > 12 {
			projName = projName[:11] + "…"
		}

		// Icon and Details
		icon := h.theme.Glyphs.HistUnknown
		details := ""

		// Common fields for patent actions
		numStr := rec.EntityID
		if reqNum, ok := rec.Metadata["requested_number"].(string); ok && reqNum != "" {
			numStr = reqNum
		} else if dn, ok := rec.Metadata["display_number"].(string); ok && dn != "" {
			numStr = dn
		} else if isMembershipOrTag && len(entityParts) >= 2 {
			numStr = entityParts[1]
		}

		invs := "-"
		if is, ok := rec.Metadata["inventors_short"].(string); ok && is != "" {
			invs = is
		}
		pubDate := "-"
		if pd, ok := rec.Metadata["publication_date"].(string); ok && pd != "" {
			pubDate = pd
		}
		title := ""
		if t, ok := rec.Metadata["title"].(string); ok && t != "" {
			title = t
		}

		pat := patentSummary(numStr, invs, pubDate, title)

		switch rec.Action {
		case "filter.apply":
			icon = h.theme.SortActive.Render(h.theme.Glyphs.HistSearch)
			details = fmt.Sprintf("Search: %q", rec.EntityID)
		case "project.switch":
			icon = h.theme.SortActive.Render(h.theme.Glyphs.HistProject)
			pName := rec.EntityID
			if name, ok := rec.Metadata["project_name"].(string); ok && name != "" {
				pName = name
			}
			details = fmt.Sprintf("Switch Project to %q", pName)
		case "ui.focus":
			scope, _ := rec.Metadata["scope"].(string)
			switch scope {
			case "citations":
				icon = h.theme.Warn.Render(h.theme.Glyphs.HistCitations)
				details = "Citations: " + pat
			case "family":
				icon = h.theme.OK.Render(h.theme.Glyphs.HistFamily)
				details = "Family Tree: " + pat
			case "fulltext":
				icon = h.theme.SortActive.Render(h.theme.Glyphs.HistFulltext)
				details = "Full Text: " + pat
			case "ids":
				icon = h.theme.Warn.Render(h.theme.Glyphs.HistIDS)
				details = "IDS Entry: " + pat
			default:
				icon = h.theme.OK.Render(h.theme.Glyphs.HistPatent)
				details = pat
			}
		case "membership.set_state":
			icon = h.theme.OK.Render(h.theme.Glyphs.HistState)
			rawState := ""
			if afterMap, ok := rec.After.(map[string]any); ok {
				if s, ok := afterMap["review_state"].(string); ok {
					rawState = s
				}
			}
			stateIcon := h.theme.ReviewStateGlyph(rawState)
			details = "State " + stateIcon + "  " + pat
		case "patent.tag_assign":
			icon = h.theme.Warn.Render(h.theme.Glyphs.HistTagAdd)
			tagName := ""
			if len(entityParts) >= 3 {
				tagName = entityParts[2]
			} else if afterMap, ok := rec.After.(map[string]any); ok {
				if t, ok := afterMap["tag_name"].(string); ok {
					tagName = t
				}
			}
			details = fmt.Sprintf("Tag %q: ", tagName) + pat
		case "patent.tag_remove":
			icon = h.theme.Warn.Render(h.theme.Glyphs.HistTagRemove)
			tagName := ""
			if len(entityParts) >= 3 {
				tagName = entityParts[2]
			} else if beforeMap, ok := rec.Before.(map[string]any); ok {
				if t, ok := beforeMap["tag_name"].(string); ok {
					tagName = t
				}
			}
			details = fmt.Sprintf("Untag %q: ", tagName) + pat
		}

		// Row content: time, project, icon, details. Aligns perfectly with header labels.
		// Icon is a 2-wide emoji; concatenate directly so terminal width is preserved.
		timeStr := localClock(rec.Timestamp)
		line := fmt.Sprintf("%s%s %-12s ", prefix, timeStr, projName) + icon + "  " + details
		// Truncate to maxW to ensure it does not wrap around
		truncated := render.Truncate(line, maxW)
		b.WriteString(style.Render(render.Pad(truncated, maxW)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(h.theme.Dim.Render(render.Pad("  [j/k] Move  [Enter] Replay/Confirm  [q/Esc] Close", maxW)))
	return b.String()
}

func patentSummary(numStr, invs, pubDate, title string) string {
	return fmt.Sprintf("%s [%s] [%s] %s", numStr, invs, pubDate, title)
}
