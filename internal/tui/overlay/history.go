package overlay

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/observability"
	"patentmine/internal/tui/render"
)

// HistoryOverlay displays the persistent timeline of recent searches, project switches, and patent details views.
type HistoryOverlay struct {
	theme        render.Theme
	allRecords   []observability.Record
	records      []observability.Record
	projectNames map[string]string
	page         render.Paginator
	vimCount     int

	filterMode    bool
	filterInput   string
	filterQuery   string
	sortAscending bool
}

// NewHistoryOverlay builds a HistoryOverlay.
func NewHistoryOverlay(theme render.Theme, records []observability.Record, projectNames map[string]string) *HistoryOverlay {
	h := &HistoryOverlay{
		theme:        theme,
		allRecords:   append([]observability.Record(nil), records...),
		projectNames: projectNames,
		page:         render.NewPaginator(1),
	}
	h.applyHistoryView()
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
	if h.filterMode {
		return h.handleFilterKey(msg)
	}
	s := msg.String()
	switch s {
	case "/":
		h.filterMode = true
		h.filterInput = h.filterQuery
		return h, nil, true
	case "c", "C":
		if h.filterQuery == "" && !h.sortAscending {
			return h, nil, true
		}
		h.filterQuery = ""
		h.filterInput = ""
		h.sortAscending = false
		h.applyHistoryView()
		return h, h.historyFilterMsg(), true
	case ".":
		h.sortAscending = !h.sortAscending
		h.applyHistoryView()
		return h, h.historyFilterMsg(), true
	}
	if len(h.records) == 0 {
		switch s {
		case "q", "Q", "esc":
			return h, func() tea.Msg { return CloseOverlayMsg{} }, true
		}
		return h, nil, true
	}
	if h.page.HandleKey(msg) {
		return h, nil, true
	}
	if handleSubtableMotionKey(&h.page, msg, &h.vimCount) {
		return h, nil, true
	}

	s = msg.String()
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
		lines := []string{h.theme.Dim.Render("No history entries found.")}
		if h.filterQuery != "" {
			lines = append(lines, h.theme.Dim.Render(fmt.Sprintf("Filter: %q  [/] Edit  [c] Clear  [q/Esc] Close", h.filterQuery)))
		} else {
			lines = append(lines, h.theme.Dim.Render("[/] Filter  [q/Esc] Close"))
		}
		return strings.Join(lines, "\n")
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
		{Key: "details", Label: " Details", Width: detailsW},
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
			return " " + details
		}
		return ""
	}

	var b strings.Builder
	if h.filterMode {
		b.WriteString(h.theme.Dim.Render(render.Pad("  Filter history: "+h.filterInput+"█", maxW)))
		b.WriteString("\n")
	} else if h.filterQuery != "" || h.sortAscending {
		sortLabel := "newest first"
		if h.sortAscending {
			sortLabel = "oldest first"
		}
		filterLabel := "all"
		if h.filterQuery != "" {
			filterLabel = h.filterQuery
		}
		b.WriteString(h.theme.Dim.Render(render.Pad(fmt.Sprintf("  Filter: %q  Sort: %s", filterLabel, sortLabel), maxW)))
		b.WriteString("\n")
	}
	b.WriteString(renderSubtable(subtableParams{
		Theme:        h.theme,
		Columns:      cols,
		Page:         &h.page,
		Total:        n,
		PageSize:     pageSize,
		FocusActive:  true,
		VisualMode:   h.page.VisualMode(),
		IsRowSelected: func(absIdx int) bool {
			return h.page.IsRowSelected(absIdx)
		},
	}, maxW, getCell))
	b.WriteString("\n")
	help := fmt.Sprintf("  %s  [/] Filter  [.] Sort  [c] Clear  [j/k] Move  [ctrl+u/d] Page  [Enter] Replay  [q/Esc] Close", subtableStatus(h.page))
	b.WriteString(h.theme.Dim.Render(render.Pad(help, maxW)))
	return b.String()
}

func (h *HistoryOverlay) handleFilterKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyEsc:
		h.filterMode = false
		h.filterInput = h.filterQuery
		return h, nil, true
	case tea.KeyEnter:
		h.filterMode = false
		h.filterQuery = strings.TrimSpace(h.filterInput)
		h.applyHistoryView()
		return h, h.historyFilterMsg(), true
	case tea.KeyBackspace:
		if h.filterInput != "" {
			runes := []rune(h.filterInput)
			h.filterInput = string(runes[:len(runes)-1])
		}
		return h, nil, true
	case tea.KeyRunes, tea.KeySpace:
		h.filterInput += msg.String()
		return h, nil, true
	}
	return h, nil, true
}

func (h *HistoryOverlay) applyHistoryView() {
	query := strings.TrimSpace(h.filterQuery)
	h.records = h.records[:0]
	for _, rec := range h.allRecords {
		if query == "" || historyRecordMatches(rec, h.projectNames, query) {
			h.records = append(h.records, rec)
		}
	}
	sort.SliceStable(h.records, func(i, j int) bool {
		if h.sortAscending {
			return h.records[i].Timestamp.Before(h.records[j].Timestamp)
		}
		return h.records[j].Timestamp.Before(h.records[i].Timestamp)
	})
	h.page.SetTotal(len(h.records))
	h.page.Top()
	h.page.ClearVisual()
}

func (h *HistoryOverlay) historyFilterMsg() tea.Cmd {
	query := h.filterQuery
	sortAscending := h.sortAscending
	resultCount := len(h.records)
	totalCount := len(h.allRecords)
	return func() tea.Msg {
		return HistoryFilterAppliedMsg{Query: query, SortAscending: sortAscending, ResultCount: resultCount, TotalCount: totalCount}
	}
}

func historyRecordMatches(rec observability.Record, projectNames map[string]string, query string) bool {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return true
	}
	_, details := historyIconAndDetails(render.NewTheme(), rec)
	parts := []string{
		rec.Action,
		rec.Entity,
		rec.EntityID,
		rec.Status,
		historyProjectName(rec, projectNames),
		details,
		rec.Timestamp.Format("2006-01-02 15:04:05"),
	}
	for _, raw := range []any{rec.Attributes, rec.Before, rec.After} {
		if raw == nil {
			continue
		}
		if b, err := json.Marshal(raw); err == nil {
			parts = append(parts, string(b))
		}
	}
	return strings.Contains(strings.ToLower(strings.Join(parts, " ")), needle)
}

// historyProjectName resolves the project ID embedded in the record into a display name.
func historyProjectName(rec observability.Record, projectNames map[string]string) string {
	entityParts := strings.Split(rec.EntityID, "/")
	isProjectPatentRec := IsProjectPatentAction(rec.Action)

	projID := ""
	if pid, ok := rec.Attributes["project"].(string); ok && pid != "" {
		projID = pid
	} else if isProjectPatentRec && len(entityParts) >= 1 && entityParts[0] != "" {
		projID = entityParts[0]
	} else if rec.Action == observability.ActionProjectSwitch {
		projID = rec.EntityID
	}

	if projID == "" {
		return "-"
	}
	if name, ok := projectNames[projID]; ok && name != "" {
		return name
	}
	if name, ok := rec.Attributes["project_name"].(string); ok && name != "" {
		return name
	}
	return projID
}

// historyIconAndDetails returns the action icon and details column text for a record.
func historyIconAndDetails(theme render.Theme, rec observability.Record) (string, string) {
	entityParts := strings.Split(rec.EntityID, "/")
	isProjectPatentRec := IsProjectPatentAction(rec.Action)

	numStr := rec.EntityID
	if reqNum, ok := rec.Attributes["requested_number"].(string); ok && reqNum != "" {
		numStr = reqNum
	} else if dn, ok := rec.Attributes["display_number"].(string); ok && dn != "" {
		numStr = dn
	} else if isProjectPatentRec && len(entityParts) >= 2 {
		numStr = entityParts[1]
	}

	invs := "-"
	if is, ok := rec.Attributes["inventors_short"].(string); ok && is != "" {
		invs = is
	}
	pubDate := "-"
	if pd, ok := rec.Attributes["publication_date"].(string); ok && pd != "" {
		pubDate = pd
	}
	title := ""
	if t, ok := rec.Attributes["title"].(string); ok && t != "" {
		title = t
	}
	pat := patentSummary(numStr, invs, pubDate, title)

	switch rec.Action {
	case observability.ActionFilterApply:
		if _, ok := rec.Attributes["search"].(string); ok {
			if _, hasFilter := rec.Attributes["filter"]; !hasFilter {
				return theme.Glyphs.HistSearch, fmt.Sprintf("Search: %q", rec.EntityID)
			}
		}
		return theme.Glyphs.HistSearch, fmt.Sprintf("Filter: %q", rec.EntityID)
	case observability.ActionProjectSwitch:
		pName := rec.EntityID
		if name, ok := rec.Attributes["project_name"].(string); ok && name != "" {
			pName = name
		}
		return theme.Glyphs.HistProject, fmt.Sprintf("Switch Project to %q", pName)
	case observability.ActionUIFocus:
		scope, _ := rec.Attributes["scope"].(string)
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
	case observability.ActionNotesExport:
		projectName := rec.EntityID
		if name, ok := rec.Attributes["project_name"].(string); ok && name != "" {
			projectName = name
		}
		count, _ := rec.Attributes["count"].(float64)
		path, _ := rec.Attributes["path"].(string)
		return theme.Glyphs.HistNotesExport, fmt.Sprintf("Export notes %q: %d note(s) → %s", projectName, int(count), path)
	case observability.ActionNotesSave:
		return theme.Glyphs.HistNotesExport, "Save Note: " + pat
	case observability.ActionNotesDelete:
		return theme.Glyphs.HistNotesExport, "Delete Note: " + pat
	case observability.ActionMembershipAdd:
		method := "manual"
		if isManualVal, ok := rec.Attributes["manual"].(bool); ok {
			if isManualVal {
				method = "manual"
			} else {
				method = "system"
			}
		} else if isSystemVal, ok := rec.Attributes["system"].(bool); ok {
			if isSystemVal {
				method = "system"
			} else {
				method = "manual"
			}
		} else if src, ok := rec.Attributes["source"].(string); ok {
			if strings.HasPrefix(src, "auto") || strings.HasPrefix(src, "system") {
				method = "system"
			} else if src == "add.related" || src == "related" {
				method = "related"
			} else if src == "direct" {
				method = "manual"
			} else {
				method = src
			}
		}
		return theme.Glyphs.HistPatent, fmt.Sprintf("Add Patent (%s): %s", method, pat)
	case observability.ActionMembershipSetState:
		rawState := ""
		if s, ok := rec.Attributes["state"].(string); ok {
			rawState = s
		} else if afterMap, ok := rec.After.(map[string]any); ok {
			if s, ok := afterMap["review_state"].(string); ok {
				rawState = s
			}
		}
		stateIcon := theme.ReviewStateGlyph(rawState)
		return theme.Glyphs.HistState, "State: " + stateIcon + "  " + pat
	case observability.ActionPatentTagAssign:
		tagName := ""
		if len(entityParts) >= 3 {
			tagName = entityParts[2]
		} else if afterMap, ok := rec.After.(map[string]any); ok {
			if t, ok := afterMap["tag_name"].(string); ok {
				tagName = t
			}
		}
		return theme.Glyphs.HistTagAdd, fmt.Sprintf("Tag %q: ", tagName) + pat
	case observability.ActionPatentTagRemove:
		tagName := ""
		if len(entityParts) >= 3 {
			tagName = entityParts[2]
		} else if beforeMap, ok := rec.Before.(map[string]any); ok {
			if t, ok := beforeMap["tag_name"].(string); ok {
				tagName = t
			}
		}
		return theme.Glyphs.HistTagRemove, fmt.Sprintf("Untag %q: ", tagName) + pat
	case observability.ActionIDSEntrySave:
		status := ""
		if s, ok := rec.Attributes["status"].(string); ok {
			status = s
		}
		prior := ""
		if s, ok := rec.Attributes["prior_status"].(string); ok {
			prior = s
		}
		label := "IDS save"
		if prior != "" && prior != status {
			label = fmt.Sprintf("IDS %s → %s", prior, status)
		} else if status != "" {
			label = "IDS " + status
		}
		return theme.Glyphs.HistIDS, label + ": " + pat
	case observability.ActionIDSEntryDelete:
		return theme.Glyphs.HistIDS, "IDS delete: " + pat
	}
	return theme.Glyphs.HistUnknown, rec.EntityID
}

func patentSummary(numStr, invs, pubDate, title string) string {
	return fmt.Sprintf("%s [%s] [%s] %s", numStr, invs, pubDate, title)
}
