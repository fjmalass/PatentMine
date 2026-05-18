package overlay

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/tui/keymap"
	"patentmine/internal/tui/render"
)

// keyColumnWidth is the width of the key-sequence column in the help list.
const keyColumnWidth = 18

// Help is a scrollable overlay listing every command and its key bindings. It
// is generated from the command registry and the keymap, so it can never drift
// out of sync with the bindings that are actually active.
type Help struct {
	theme render.Theme
	lines []string
	page  render.Paginator
}

// NewHelp builds the help overlay from the registry and keymap.
func NewHelp(reg *command.Registry, km *keymap.Keymaps, theme render.Theme) *Help {
	lines := buildHelpLines(reg, km, theme)
	page := render.NewPaginator(12)
	page.SetTotal(len(lines))
	return &Help{theme: theme, lines: lines, page: page}
}

// Title implements Overlay.
func (h *Help) Title() string { return "Help — key bindings" }

// Command implements Overlay: navigation scrolls the list.
func (h *Help) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	switch id {
	case command.NavDown:
		h.page.MoveDown(repeat)
	case command.NavUp:
		h.page.MoveUp(repeat)
	case command.NavPageDown:
		h.page.PageDown()
	case command.NavPageUp:
		h.page.PageUp()
	}
	return h, nil
}

// View implements Overlay.
func (h *Help) View(maxW, maxH int) string {
	h.page.SetPageSize(max(maxH, 1))
	start, end := h.page.Window()
	var b strings.Builder
	for i := start; i < end; i++ {
		if i > start {
			b.WriteByte('\n')
		}
		b.WriteString(render.Truncate(h.lines[i], maxW))
	}
	return b.String()
}

// buildHelpLines renders the registry+keymap into styled help lines.
func buildHelpLines(reg *command.Registry, km *keymap.Keymaps, theme render.Theme) []string {
	var lines []string

	section := func(name string, layer *keymap.Layer) {
		if layer == nil {
			return
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, theme.Title.Render(name))

		byCommand := make(map[command.ID][]string)
		for seq, id := range layer.Bindings() {
			byCommand[id] = append(byCommand[id], seq)
		}
		// Iterate the registry for a stable, meaningful order.
		for _, c := range reg.All() {
			seqs, bound := byCommand[c.ID]
			if !bound {
				continue
			}
			sort.Strings(seqs)
			keyCol := render.Pad(strings.Join(seqs, " / "), keyColumnWidth)
			label := c.Title
			if c.Name != "" {
				usage := c.Usage
				if usage == "" {
					usage = ":" + c.Name
				}
				label += "  " + theme.Dim.Render(usage)
			}
			lines = append(lines,
				"  "+theme.HelpKey.Render(keyCol)+" "+theme.Row.Render(label))
		}
	}

	section("Global", km.Base())
	section("Catalog", km.Context(command.ContextCatalog))
	section("Detail", km.Context(command.ContextDetail))
	section("Citations", km.Context(command.ContextCitations))
	section("Projects", km.Context(command.ContextProjects))
	section("Overlay", km.Context(command.ContextOverlay))
	return lines
}
