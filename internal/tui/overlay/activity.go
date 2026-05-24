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
	theme   render.Theme
	records []observability.Record
	cursor  int
}

// NewActivity builds an activity journal overlay.
func NewActivity(theme render.Theme, records []observability.Record) *Activity {
	return &Activity{theme: theme, records: records}
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
	switch msg.String() {
	case "j", "down":
		a.cursor = min(a.cursor+1, len(a.records)-1)
		return a, nil, true
	case "k", "up":
		a.cursor = max(a.cursor-1, 0)
		return a, nil, true
	case "enter":
		rec := a.records[a.cursor]
		if number, ok := activityPatent(a.records[a.cursor]); ok {
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
	if len(a.records) == 0 {
		return a.theme.Dim.Render("No activity records yet.")
	}
	visible := max(maxH-4, 1)
	start := 0
	if a.cursor >= visible {
		start = a.cursor - visible + 1
	}
	end := min(start+visible, len(a.records))

	var b strings.Builder
	b.WriteString(a.theme.Dim.Render(render.Pad("  Time     Component Action                 Entity", maxW)))
	b.WriteByte('\n')
	for i := start; i < end; i++ {
		rec := a.records[i]
		prefix := "  "
		style := a.theme.Row
		if i == a.cursor {
			prefix = " >"
			style = a.theme.Selected
		}
		line := fmt.Sprintf("%s %s %-9s %-22s %s", prefix, localClock(rec.Timestamp), rec.Component, rec.Action, activityEntity(rec))
		b.WriteString(style.Render(render.Pad(line, maxW)))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	selected := a.records[a.cursor]
	detail := fmt.Sprintf("  %s %s", selected.Status, activitySummary(selected))
	b.WriteString(a.theme.Dim.Render(render.Pad(detail, maxW)))
	b.WriteByte('\n')
	b.WriteString(a.theme.Dim.Render(render.Pad("  [j/k] Move  [Enter] Open patent  [q/Esc] Close", maxW)))
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
	if rec.Metadata != nil {
		for _, key := range []string{"title", "filter", "review_state", "duration_ms"} {
			if v, ok := rec.Metadata[key]; ok && fmt.Sprint(v) != "" {
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
	if rec.Metadata != nil {
		if v, ok := rec.Metadata["requested_number"]; ok {
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
