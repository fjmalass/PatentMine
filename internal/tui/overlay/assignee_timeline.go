package overlay

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/tui/render"
)

type loadedAssigneeTimelineMsg struct {
	entries []domain.AssigneeHistoryEntry
	err     error
}

// AssigneeTimelineOverlay presents the unified ownership timeline for a single patent,
// displaying both when the assignments were made (effective date) and when they were
// found/pulled from the USPTO database.
type AssigneeTimelineOverlay struct {
	client  *rpc.Client
	theme   render.Theme
	patent  domain.Patent
	entries []domain.AssigneeHistoryEntry
	loading bool
	err     error
	offset  int
}

// NewAssigneeTimelineOverlay builds the overlay and starts loading the assignee history.
func NewAssigneeTimelineOverlay(client *rpc.Client, theme render.Theme, patent domain.Patent) (*AssigneeTimelineOverlay, tea.Cmd) {
	o := &AssigneeTimelineOverlay{
		client:  client,
		theme:   theme,
		patent:  patent,
		loading: true,
	}
	return o, o.loadCmd()
}

func (o *AssigneeTimelineOverlay) loadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.AssigneeHistoryResult
		if err := o.client.Call(ctx, proto.MethodAssigneeHistory,
			proto.AssigneeHistoryParams{Number: o.patent.Number}, &res); err != nil {
			return loadedAssigneeTimelineMsg{err: err}
		}
		return loadedAssigneeTimelineMsg{entries: res.Entries}
	}
}

func (o *AssigneeTimelineOverlay) Title() string { return "Assignee Ownership History" }

func (o *AssigneeTimelineOverlay) Handles() []command.ID { return []command.ID{command.CloseOverlay} }

func (o *AssigneeTimelineOverlay) Command(id command.ID, _ int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return o, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return o, nil
}

func (o *AssigneeTimelineOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case loadedAssigneeTimelineMsg:
		o.loading = false
		o.err = m.err
		o.entries = m.entries
		return o, nil
	case proto.Event:
		if m.Method == proto.EventDBChanged {
			o.loading = true
			return o, o.loadCmd()
		}
	}
	return o, nil
}

func (o *AssigneeTimelineOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.String() {
	case "q", "Q", "esc":
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "j", "down":
		o.offset++
		return o, nil, true
	case "k", "up":
		if o.offset > 0 {
			o.offset--
		}
		return o, nil, true
	}
	return o, nil, true
}

func (o *AssigneeTimelineOverlay) OverlaySize(termW, termH int) (w, h int) {
	return PctSize(termW, termH, 85, 80, 85, 20)
}

func (o *AssigneeTimelineOverlay) lines(width int) []string {
	var out []string

	numStr := o.patent.DisplayNumber.String()
	if numStr == "" {
		numStr = o.patent.Number.String()
	}
	out = append(out, o.theme.Header.Render(render.Truncate(
		fmt.Sprintf("OWNERSHIP TIMELINE FOR PATENT %s", numStr), width)))
	if o.patent.Title != "" {
		out = append(out, o.theme.Dim.Render(render.Truncate("  "+o.patent.Title, width)))
	}
	out = append(out, "")

	if len(o.entries) == 0 {
		out = append(out, o.theme.MutedItalic.Render("  No assignee history recorded."))
		out = append(out, o.theme.MutedItalic.Render("  Run :fetch.uspto.assignments to pull recorded assignments."))
		return out
	}

	headerLine := fmt.Sprintf("  %-10s   %-10s   %-10s   %-7s   %-30s", "DATE MADE", "DATE FOUND", "ORIGIN", "CURRENT", "ASSIGNEE & DETAILS")
	out = append(out, o.theme.FocusHeader.Render(render.Truncate(headerLine, width)))

	for i, e := range o.entries {
		effDate := e.EffectiveDate
		if effDate == "" {
			effDate = "at-grant"
		}
		foundAt := e.PulledAt
		if len(foundAt) > 10 {
			foundAt = foundAt[:10]
		}
		if foundAt == "" {
			foundAt = "-"
		}

		origin := e.PullType
		if origin == "at_grant" {
			origin = "grant"
		} else if origin == "assignment" {
			origin = "assign"
		}

		currentStr := ""
		if e.IsLatest {
			currentStr = " ✓ Yes"
		}

		namePart := e.AssigneeName
		if e.IsLatest {
			namePart = namePart + " (current)"
		}

		var extra []string
		if e.ConveyanceText != "" {
			extra = append(extra, e.ConveyanceText)
		}
		if e.ReelFrame != "" {
			extra = append(extra, "reel/frame: "+e.ReelFrame)
		}

		details := namePart
		if len(extra) > 0 {
			details = fmt.Sprintf("%s (%s)", namePart, strings.Join(extra, " · "))
		}

		rowStr := fmt.Sprintf("  %-10s   %-10s   %-10s   %-7s   %-30s", effDate, foundAt, origin, currentStr, details)
		if e.IsLatest {
			out = append(out, o.theme.Selected.Render(render.Truncate(rowStr, width)))
		} else if i%2 == 1 {
			out = append(out, o.theme.RowAlt.Render(render.Truncate(rowStr, width)))
		} else {
			out = append(out, o.theme.Row.Render(render.Truncate(rowStr, width)))
		}
	}

	return out
}

func (o *AssigneeTimelineOverlay) View(maxW, maxH int) string {
	width := maxW - 2
	if width < 20 {
		width = 20
	}
	var b strings.Builder
	if o.err != nil {
		b.WriteString(o.theme.Error.Render(render.Truncate("Error: "+o.err.Error(), width)))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[q/Esc] Close"))
		return b.String()
	}
	if o.loading {
		b.WriteString(o.theme.MutedItalic.Render("Loading assignee history timeline..."))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[q/Esc] Close"))
		return b.String()
	}

	lines := o.lines(width)
	bodyH := maxH - 2
	if bodyH < 1 {
		bodyH = 1
	}
	if o.offset > len(lines)-bodyH {
		o.offset = max(0, len(lines)-bodyH)
	}
	end := min(len(lines), o.offset+bodyH)
	for i := o.offset; i < end; i++ {
		b.WriteString(lines[i])
		b.WriteByte('\n')
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate("[j/k] Scroll  [q/Q/Esc] Close", width)))
	return b.String()
}
