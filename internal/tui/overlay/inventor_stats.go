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
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

// loadedInventorStatsMsg carries the asynchronously fetched stats.
type loadedInventorStatsMsg struct {
	stats []domain.InventorStats
	err   error
}

// InventorStatsOverlay presents interactive statistics for the patent's inventors.
type InventorStatsOverlay struct {
	client   *rpc.Client
	theme    render.Theme
	catalog  *text.Catalog
	patent   domain.Patent
	project  domain.ProjectID
	stats    []domain.InventorStats
	selected int
	loading  bool
	err      error
}

// NewInventorStatsOverlay initializes and triggers an async query for inventor stats.
func NewInventorStatsOverlay(client *rpc.Client, theme render.Theme, catalog *text.Catalog, patent domain.Patent, project domain.ProjectID) (*InventorStatsOverlay, tea.Cmd) {
	o := &InventorStatsOverlay{
		client:  client,
		theme:   theme,
		catalog: catalog,
		patent:  patent,
		project: project,
		loading: true,
	}
	return o, o.loadStatsCmd()
}

// Title implements Overlay.
func (o *InventorStatsOverlay) Title() string {
	return "Inventor Analytics"
}

// Handles implements Overlay.
func (o *InventorStatsOverlay) Handles() []command.ID {
	return []command.ID{
		command.CloseOverlay,
	}
}

// Command implements Overlay.
func (o *InventorStatsOverlay) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return o, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return o, nil
}

// Update processes background load responses.
func (o *InventorStatsOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case loadedInventorStatsMsg:
		o.loading = false
		if m.err != nil {
			o.err = m.err
		} else {
			o.stats = m.stats
			if o.selected >= len(o.stats) {
				o.selected = max(0, len(o.stats)-1)
			}
		}
		return o, nil
	}
	return o, nil
}

// HandleKey implements KeyHandler for scroll and dismiss.
func (o *InventorStatsOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	o.err = nil

	switch msg.String() {
	case "q", "esc":
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "j", "down":
		if len(o.stats) > 0 {
			o.selected = (o.selected + 1) % len(o.stats)
		}
		return o, nil, true
	case "k", "up":
		if len(o.stats) > 0 {
			o.selected = (o.selected - 1 + len(o.stats)) % len(o.stats)
		}
		return o, nil, true
	}

	return o, nil, true
}

// OverlaySize implements DynamicSize, giving us extra width for stats formatting.
func (o *InventorStatsOverlay) OverlaySize(termW, termH int) (w, h int) {
	w = min(termW-4, 90)
	h = min(termH-4, 22)
	return w, h
}

// View renders the list of inventors and their respective metrics.
func (o *InventorStatsOverlay) View(maxW, maxH int) string {
	var b strings.Builder

	if o.err != nil {
		b.WriteString(o.theme.Error.Render(render.Truncate("Error: "+o.err.Error(), maxW)))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[q/Esc] Close"))
		return b.String()
	}

	if o.loading {
		b.WriteString(o.theme.MutedItalic.Render("Loading inventor statistics..."))
		return b.String()
	}

	if len(o.stats) == 0 {
		b.WriteString(o.theme.MutedItalic.Render("No inventors found for this patent."))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[q/Esc] Close"))
		return b.String()
	}

	titleText := fmt.Sprintf("Patent: %s (Total Inventors: %d)", o.patent.DisplayNumber.String(), len(o.stats))
	b.WriteString(o.theme.Dim.Render(titleText))
	b.WriteString("\n\n")

	// Find the maximum name length to perfectly align stats horizontally
	maxNameLen := 0
	for _, s := range o.stats {
		if len(s.Inventor) > maxNameLen {
			maxNameLen = len(s.Inventor)
		}
	}

	pageSize := maxH - 6
	if pageSize < 1 {
		pageSize = 1
	}

	start := max(0, o.selected-pageSize/2)
	end := min(len(o.stats), start+pageSize)
	if end-start < pageSize && start > 0 {
		start = max(0, end-pageSize)
	}

	for i := start; i < end; i++ {
		s := o.stats[i]
		prefix := "  "
		if i == o.selected {
			prefix = "→ "
		}

		paddedName := render.Pad(s.Inventor+":", maxNameLen+2)
		statsStr := render.FormatEntityStats(s.Total, s.States, s.Tags)
		line := fmt.Sprintf("%s%s%s", prefix, paddedName, statsStr)

		if i == o.selected {
			b.WriteString(o.theme.Selected.Render(render.Truncate(line, maxW)))
		} else {
			if i%2 == 1 {
				b.WriteString(o.theme.RowAlt.Render(render.Truncate(line, maxW)))
			} else {
				b.WriteString(o.theme.Row.Render(render.Truncate(line, maxW)))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(o.theme.Dim.Render("[j/k/↑/↓] Scroll  [q/Esc] Close"))

	return b.String()
}

func (o *InventorStatsOverlay) loadStatsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var res proto.PatentInventorStatsResult
		params := proto.PatentGetParams{
			Number:  o.patent.Number,
			Project: o.project,
		}
		if err := o.client.Call(ctx, proto.MethodPatentInventorStats, params, &res); err != nil {
			return loadedInventorStatsMsg{err: err}
		}
		return loadedInventorStatsMsg{stats: res.Stats}
	}
}
