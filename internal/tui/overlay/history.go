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
	theme        render.Theme
	records      []observability.Record
	projectNames map[string]string
	page         render.Paginator
	vimCount     int
}

// NewHistoryOverlay builds a HistoryOverlay.
func NewHistoryOverlay(theme render.Theme, records []observability.Record, projectNames map[string]string) *HistoryOverlay {
	h := &HistoryOverlay{
		theme:        theme,
		records:      records,
		projectNames: projectNames,
		page:         render.NewPaginator(1),
	}
	h.page.SetTotal(len(records))
	return h
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
	if len(h.records) == 0 {
		return h, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	if h.page.HandleKey(msg) {
		return h, nil, true
	}
	if handleSubtableMotionKey(&h.page, msg, &h.vimCount) {
		return h, nil, true
	}

	s := msg.String()
	switch s {
	case "enter":
		rec := h.records[h.page.Cursor()]
		return h, func() tea.Msg { return HistoryReplayMsg{Record: rec} }, true
	case "q", "Q", "esc":
		if h.page.VisualMode() {
			h.page.SaveVisual()
			h.page.ClearVisual()
			return h, nil, true
		}
		return h, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	if msg.Type == tea.KeyEscape {
		if h.page.VisualMode() {
			h.page.SaveVisual()
			h.page.ClearVisual()
			return h, nil, true
		}
		return h, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	return h, nil, true
}

// View implements Overlay.
func (h *HistoryOverlay) View(maxW, maxH int) string {
	maxW = max(maxW-2, 10)
	n := len(h.records)
	if n == 0 {
		return h.theme.Dim.Render("No history entries found.")
	}

	pageSize := max(maxH-3, 1)

	// Columns: line number, time, project, action icon, details.
	gutterW := max(render.GutterWidth(n)-1, 1)
	const timeW = 8
	const projW = 12
	const iconW = 2
	fixed := 2 + gutterW + timeW + projW + iconW + 4 // prefix + cols + 4 single-space gaps
	detailsW := max(maxW-fixed, 10)

	cols := []render.TableColumn{
		{Key: "ln", Label: strings.Repeat(" ", gutterW), Width: gutterW},
		{Key: "time", Label: "Time", Width: timeW},
		{Key: "proj", Label: "Project", Width: projW},
		{Key: "icon", Label: h.theme.Glyphs.HistColType, Width: iconW},
		{Key: "details", Label: "Details", Width: detailsW},
	}

	getCell := func(absIdx, _ int, colIdx int) string {
		if absIdx < 0 || absIdx >= n {
			return ""
		}
		rec := h.records[absIdx]
		switch cols[colIdx].Key {
		case "ln":
			return fmt.Sprintf("%*d", gutterW, absIdx+1)
		case "time":
			return localClock(rec.Timestamp)
		case "proj":
			return historyProjectName(rec, h.projectNames)
		case "icon":
			icon, _ := historyIconAndDetails(h.theme, rec)
			return icon
		case "details":
			_, details := historyIconAndDetails(h.theme, rec)
			return details
		}
		return ""
	}

	var b strings.Builder
	b.WriteString(renderSubtable(subtableParams{
		Theme:        h.theme,
		Columns:      cols,
		Page:         &h.page,
		Total:        n,
		PageSize:     pageSize,
		FocusActive:  true,
		PrefixCursor: "→ ",
		PrefixNormal: "  ",
		VisualMode:   h.page.VisualMode(),
		IsRowSelected: func(absIdx int) bool {
			return h.page.IsRowSelected(absIdx)
		},
	}, maxW, getCell))
	b.WriteString("\n")
	help := fmt.Sprintf("  %s  [j/k] Move  [ctrl+u/d] Page  [gg/G] Top/Bot  [v] Visual  [ga] All  [Enter] Replay  [q/Esc] Close", subtableStatus(h.page))
	b.WriteString(h.theme.Dim.Render(render.Pad(help, maxW)))
	return b.String()
}

// historyProjectName resolves the project ID embedded in the record into a display name.
func historyProjectName(rec observability.Record, projectNames map[string]string) string {
	entityParts := strings.Split(rec.EntityID, "/")
	isMembershipOrTag := rec.Action == "membership.set_state" ||
		rec.Action == "patent.tag_assign" ||
		rec.Action == "patent.tag_remove"

	projID := ""
	if pid, ok := rec.Metadata["project"].(string); ok && pid != "" {
		projID = pid
	} else if isMembershipOrTag && len(entityParts) >= 1 && entityParts[0] != "" {
		projID = entityParts[0]
	} else if rec.Action == "project.switch" {
		projID = rec.EntityID
	}

	if projID == "" {
		return "-"
	}
	if name, ok := projectNames[projID]; ok && name != "" {
		return name
	}
	if name, ok := rec.Metadata["project_name"].(string); ok && name != "" {
		return name
	}
	return projID
}

// historyIconAndDetails returns the action icon and details column text for a record.
func historyIconAndDetails(theme render.Theme, rec observability.Record) (string, string) {
	entityParts := strings.Split(rec.EntityID, "/")
	isMembershipOrTag := rec.Action == "membership.set_state" ||
		rec.Action == "patent.tag_assign" ||
		rec.Action == "patent.tag_remove"

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
		return theme.Glyphs.HistSearch, fmt.Sprintf("Search: %q", rec.EntityID)
	case "project.switch":
		pName := rec.EntityID
		if name, ok := rec.Metadata["project_name"].(string); ok && name != "" {
			pName = name
		}
		return theme.Glyphs.HistProject, fmt.Sprintf("Switch Project to %q", pName)
	case "ui.focus":
		scope, _ := rec.Metadata["scope"].(string)
		switch scope {
		case "citations":
			return theme.Glyphs.HistCitations, "Citations: " + pat
		case "family":
			return theme.Glyphs.HistFamily, "Family Tree: " + pat
		case "fulltext":
			return theme.Glyphs.HistFulltext, "Full Text: " + pat
		case "ids":
			return theme.Glyphs.HistIDS, "IDS Entry: " + pat
		default:
			return theme.Glyphs.HistPatent, pat
		}
	case "membership.set_state":
		rawState := ""
		if afterMap, ok := rec.After.(map[string]any); ok {
			if s, ok := afterMap["review_state"].(string); ok {
				rawState = s
			}
		}
		stateIcon := theme.ReviewStateGlyph(rawState)
		return theme.Glyphs.HistState, "State: " + stateIcon + "  " + pat
	case "patent.tag_assign":
		tagName := ""
		if len(entityParts) >= 3 {
			tagName = entityParts[2]
		} else if afterMap, ok := rec.After.(map[string]any); ok {
			if t, ok := afterMap["tag_name"].(string); ok {
				tagName = t
			}
		}
		return theme.Glyphs.HistTagAdd, fmt.Sprintf("Tag %q: ", tagName) + pat
	case "patent.tag_remove":
		tagName := ""
		if len(entityParts) >= 3 {
			tagName = entityParts[2]
		} else if beforeMap, ok := rec.Before.(map[string]any); ok {
			if t, ok := beforeMap["tag_name"].(string); ok {
				tagName = t
			}
		}
		return theme.Glyphs.HistTagRemove, fmt.Sprintf("Untag %q: ", tagName) + pat
	}
	return theme.Glyphs.HistUnknown, rec.EntityID
}

func patentSummary(numStr, invs, pubDate, title string) string {
	return fmt.Sprintf("%s [%s] [%s] %s", numStr, invs, pubDate, title)
}
