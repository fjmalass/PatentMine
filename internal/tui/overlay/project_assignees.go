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

type loadedProjectAssigneesMsg struct {
	rollup domain.ProjectAssigneeRollup
	err    error
}

// ProjectAssigneesOverlay is a read-only rollup of a project's assignees:
// current owners (deduped, live patents only) vs. every assignee ever, plus
// live/expired/not-fetched coverage. It answers "who owns this portfolio". Data
// comes from the project.assignees RPC; the body scrolls with j/k.
type ProjectAssigneesOverlay struct {
	client  *rpc.Client
	theme   render.Theme
	project domain.ProjectID
	rollup  domain.ProjectAssigneeRollup
	loading bool
	err     error
	offset  int
}

// NewProjectAssigneesOverlay builds the overlay and its initial load command.
func NewProjectAssigneesOverlay(client *rpc.Client, theme render.Theme, project domain.ProjectID) (*ProjectAssigneesOverlay, tea.Cmd) {
	o := &ProjectAssigneesOverlay{client: client, theme: theme, project: project, loading: true}
	return o, o.loadCmd()
}

func (o *ProjectAssigneesOverlay) loadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res domain.ProjectAssigneeRollup
		if err := o.client.Call(ctx, proto.MethodProjectAssignees,
			proto.ProjectAssigneesParams{Project: o.project}, &res); err != nil {
			return loadedProjectAssigneesMsg{err: err}
		}
		return loadedProjectAssigneesMsg{rollup: res}
	}
}

func (o *ProjectAssigneesOverlay) Title() string { return "Project Assignees" }

func (o *ProjectAssigneesOverlay) Handles() []command.ID { return []command.ID{command.CloseOverlay} }

func (o *ProjectAssigneesOverlay) Command(id command.ID, _ int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return o, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return o, nil
}

func (o *ProjectAssigneesOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case loadedProjectAssigneesMsg:
		o.loading = false
		o.err = m.err
		o.rollup = m.rollup
		return o, nil
	case proto.Event:
		if m.Method == proto.EventDBChanged {
			o.loading = true
			return o, o.loadCmd()
		}
	}
	return o, nil
}

func (o *ProjectAssigneesOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
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

func (o *ProjectAssigneesOverlay) OverlaySize(termW, termH int) (w, h int) {
	return PctSize(termW, termH, 80, 85, 80, 20)
}

// lines builds the full rendered body as a slice so View can window it for
// scrolling. Each assignee group is one line; the display label is the
// most-frequent raw spelling resolved server-side.
func (o *ProjectAssigneesOverlay) lines(width int) []string {
	var out []string
	r := o.rollup

	out = append(out, o.theme.Header.Render(render.Truncate(
		fmt.Sprintf("CURRENT OWNERS  (live patents only · %d)", len(r.CurrentOwners)), width)))
	if len(r.CurrentOwners) == 0 {
		out = append(out, o.theme.MutedItalic.Render("  none — run :fetch.uspto.assignments.project"))
	}
	for _, g := range r.CurrentOwners {
		out = append(out, o.theme.Row.Render(render.Truncate(
			fmt.Sprintf("  %-40s %3d", g.Display, g.Patents), width)))
	}

	out = append(out, "")
	out = append(out, o.theme.Header.Render(render.Truncate(
		fmt.Sprintf("ALL ASSIGNEES EVER  (unique · %d)", len(r.AllAssignees)), width)))
	for _, g := range r.AllAssignees {
		status := "transferred away"
		if g.Current {
			status = "current ✓"
		}
		span := strings.TrimSpace(g.FirstDate + " – " + g.LastDate)
		out = append(out, o.theme.Row.Render(render.Truncate(
			fmt.Sprintf("  %-32s %-21s %s", g.Display, span, status), width)))
	}

	out = append(out, "")
	out = append(out, o.theme.Dim.Render(render.Truncate(fmt.Sprintf(
		"Coverage:  live %d · expired (frozen) %d · not fetched %d",
		r.LivePatents, r.ExpiredPatents, r.NotFetched), width)))
	return out
}

func (o *ProjectAssigneesOverlay) View(maxW, maxH int) string {
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
		b.WriteString(o.theme.MutedItalic.Render("Loading project assignees..."))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[q/Esc] Close"))
		return b.String()
	}

	lines := o.lines(width)
	bodyH := maxH - 2 // reserve the footer
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
