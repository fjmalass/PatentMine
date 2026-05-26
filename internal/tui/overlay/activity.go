package overlay

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/tui/render"
)

// Activity shows the replay/review activity journal.
type Activity struct {
	theme    render.Theme
	records  []observability.Record
	page     render.Paginator
	vimCount int
}

// NewActivity builds an activity journal overlay.
func NewActivity(theme render.Theme, records []observability.Record) *Activity {
	a := &Activity{theme: theme, records: records, page: render.NewPaginator(1)}
	a.page.SetTotal(len(records))
	return a
}

// Title implements Overlay.
func (a *Activity) Title() string { return "Activity Replay" }

// Command implements Overlay.
func (a *Activity) Command(command.ID, int) (Overlay, tea.Cmd) { return a, nil }

// Handles implements Overlay.
func (a *Activity) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler.
func (a *Activity) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if len(a.records) == 0 {
		return a, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	if handleSubtableMotionKey(&a.page, msg, &a.vimCount) {
		return a, nil, true
	}

	s := msg.String()
	switch s {
	case "enter":
		rec := a.records[a.page.Cursor()]
		if number, ok := activityPatent(rec); ok {
			return a, func() tea.Msg { return ReplayActivityMsg{Number: number, Record: rec} }, true
		}
		return a, nil, true
	case "q", "esc":
		return a, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	if msg.Type == tea.KeyEscape {
		return a, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	return a, nil, true
}

// View implements Overlay.
func (a *Activity) View(maxW, maxH int) string {
	n := len(a.records)
	if n == 0 {
		return a.theme.Dim.Render("No activity records yet.")
	}

	pageSize := max(maxH-4, 1)

	gutterW := max(render.GutterWidth(n)-1, 1)
	const timeW = 8
	const compW = 9
	const actW = 22
	fixed := 2 + gutterW + timeW + compW + actW + 4 // prefix + cols + 4 single-space gaps
	entityW := max(maxW-fixed, 10)

	cols := []render.TableColumn{
		{Key: "ln", Label: strings.Repeat(" ", gutterW), Width: gutterW},
		{Key: "time", Label: "Time", Width: timeW},
		{Key: "component", Label: "Component", Width: compW},
		{Key: "action", Label: "Action", Width: actW},
		{Key: "entity", Label: "Entity", Width: entityW},
	}

	getCell := func(absIdx, _ int, colIdx int) string {
		if absIdx < 0 || absIdx >= n {
			return ""
		}
		rec := a.records[absIdx]
		switch cols[colIdx].Key {
		case "ln":
			return fmt.Sprintf("%*d", gutterW, absIdx+1)
		case "time":
			return localClock(rec.Timestamp)
		case "component":
			return rec.Component
		case "action":
			return rec.Action
		case "entity":
			return activityEntity(rec)
		}
		return ""
	}

	var b strings.Builder
	b.WriteString(renderSubtable(subtableParams{
		Theme:        a.theme,
		Columns:      cols,
		Page:         &a.page,
		Total:        n,
		PageSize:     pageSize,
		FocusActive:  true,
		PrefixCursor: " >",
		PrefixNormal: "  ",
	}, maxW, getCell))
	b.WriteByte('\n')
	selected := a.records[a.page.Cursor()]
	detail := fmt.Sprintf("  %s %s", selected.Status, activitySummary(selected))
	b.WriteString(a.theme.Dim.Render(render.Pad(detail, maxW)))
	b.WriteByte('\n')
	help := fmt.Sprintf("  %s  [j/k] Move  [ctrl+u/d] Page  [g/G] Top/Bot  [Enter] Open patent  [q/Esc] Close", subtableStatus(a.page))
	b.WriteString(a.theme.Dim.Render(render.Pad(help, maxW)))
	return b.String()
}

func localClock(t time.Time) string {
	if t.IsZero() {
		return "--:--:--"
	}
	return t.Local().Format("15:04:05")
}

func activityEntity(rec observability.Record) string {
	if rec.EntityID == "" {
		return rec.Entity
	}
	return rec.Entity + ":" + rec.EntityID
}

func activitySummary(rec observability.Record) string {
	parts := []string{activityEntity(rec)}
	if rec.Attributes != nil {
		for _, key := range []string{"title", "filter", "review_state", "prior_state", "state", "prior_status", "status", "duration_ms"} {
			if v, ok := rec.Attributes[key]; ok && fmt.Sprint(v) != "" {
				parts = append(parts, fmt.Sprintf("%s=%v", key, v))
			}
		}
	}
	return strings.Join(parts, "  ")
}

func activityPatent(rec observability.Record) (domain.PatentNumber, bool) {
	candidates := []string{rec.EntityID}
	if idx := strings.LastIndex(rec.EntityID, "/"); idx >= 0 {
		candidates = append(candidates, rec.EntityID[idx+1:])
	}
	if rec.Attributes != nil {
		if v, ok := rec.Attributes["requested_number"]; ok {
			candidates = append(candidates, fmt.Sprint(v))
		}
	}
	for _, candidate := range candidates {
		if number, err := domain.ParsePatentNumber(candidate); err == nil {
			return number, true
		}
	}
	return domain.PatentNumber{}, false
}
