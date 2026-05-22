package overlay

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/text"
	"patentmine/internal/tui/keymap"
	"patentmine/internal/tui/render"
)

// keyColumnWidth is the width of the key-sequence column in the help list.
const keyColumnWidth = 18

// Help is a scrollable overlay listing every command and its key bindings. It
// is generated from the command registry and the keymap, so it can never drift
// out of sync with the bindings that are actually active.
type Help struct {
	catalog  *text.Catalog
	theme    render.Theme
	handlers map[command.ID]cmdHandler
	lines    []string
	page     render.Paginator
}

// NewHelp builds the help overlay from the registry and keymap.
func NewHelp(reg *command.Registry, km *keymap.Keymaps, theme render.Theme, catalog *text.Catalog) *Help {
	lines := buildHelpLines(reg, km, theme, catalog)
	page := render.NewPaginator(12)
	page.SetTotal(len(lines))
	h := &Help{catalog: catalog, theme: theme, lines: lines, page: page}
	h.handlers = map[command.ID]cmdHandler{
		command.NavDown:     func(r int) tea.Cmd { h.page.MoveDown(r); return nil },
		command.NavUp:       func(r int) tea.Cmd { h.page.MoveUp(r); return nil },
		command.NavPageDown: func(int) tea.Cmd { h.page.PageDown(); return nil },
		command.NavPageUp:   func(int) tea.Cmd { h.page.PageUp(); return nil },
	}
	return h
}

// Title implements Overlay.
func (h *Help) Title() string { return h.catalog.T(text.OverlayHelpTitle) }

// Command implements Overlay: navigation scrolls the list.
func (h *Help) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if handler, ok := h.handlers[id]; ok {
		return h, handler(repeat)
	}
	return h, nil
}

// Handles implements Overlay.
func (h *Help) Handles() []command.ID { return handlerIDs(h.handlers) }

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

// section adds a heading line and binding lines for a keymap layer.
func section(reg *command.Registry, lines *[]string, theme render.Theme, catalog *text.Catalog, name text.Key, layer *keymap.Layer) {
	if layer == nil {
		return
	}
	if len(*lines) > 0 {
		*lines = append(*lines, "")
	}
	*lines = append(*lines, theme.Title.Render(catalog.T(name)))

	byCommand := make(map[command.ID][]string)
	for seq, id := range layer.Bindings() {
		byCommand[id] = append(byCommand[id], seq)
	}
	for _, c := range reg.All() {
		seqs, bound := byCommand[c.ID]
		if !bound {
			continue
		}
		sort.Strings(seqs)
		keyCol := render.Pad(strings.Join(seqs, " / "), keyColumnWidth)
		label := catalog.T(text.CmdTitle(string(c.ID)))
		if c.Name != "" {
			usage := c.Usage
			if usage == "" {
				usage = ":" + c.Name
			}
			label += "  " + theme.Dim.Render(usage)
		}
		*lines = append(*lines,
			"  "+theme.HelpKey.Render(keyCol)+" "+theme.Row.Render(label))
	}
}

// availableLine renders a "Available keys: h, l, m, …" line for a keymap layer.
func availableLine(lines *[]string, theme render.Theme, catalog *text.Catalog, layer *keymap.Layer) {
	if layer == nil {
		return
	}
	bound := layer.BoundLetters()
	free := freeLetters(bound)
	if len(free) == 0 {
		return
	}
	var b strings.Builder
	for i, r := range free {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(string(r))
	}
	*lines = append(*lines, "  "+theme.Dim.Render(catalog.T(text.HelpSectionAvailable)+":")+" "+theme.HelpKey.Render(b.String()))
}

// freeLetters returns a-z0-9 runes not in bound, sorted.
func freeLetters(bound []rune) []rune {
	boundSet := make(map[rune]bool, len(bound))
	for _, r := range bound {
		boundSet[r] = true
	}
	var out []rune
	for r := 'a'; r <= 'z'; r++ {
		if !boundSet[r] {
			out = append(out, r)
		}
	}
	for r := '0'; r <= '9'; r++ {
		if !boundSet[r] {
			out = append(out, r)
		}
	}
	return out
}

// scopeSection renders one scope's bindings grouped by pane-specific then
// available keys. The Detail scope additionally shows a jump-mode caption.
func scopeSection(reg *command.Registry, lines *[]string, theme render.Theme, catalog *text.Catalog, name text.Key, km *keymap.Keymaps, scope command.Scope) {
	layer := km.Scope(scope)
	if layer == nil && scope == command.ScopeDetail {
		// Detail scope always exists; show jump mode hint even if layer empty.
	}
	if layer == nil {
		return
	}
	if len(*lines) > 0 {
		*lines = append(*lines, "")
	}
	*lines = append(*lines, theme.Title.Render(catalog.T(name)))

	byCommand := make(map[command.ID][]string)
	for seq, id := range layer.Bindings() {
		byCommand[id] = append(byCommand[id], seq)
	}
	for _, c := range reg.All() {
		seqs, bound := byCommand[c.ID]
		if !bound {
			continue
		}
		sort.Strings(seqs)
		keyCol := render.Pad(strings.Join(seqs, " / "), keyColumnWidth)
		label := catalog.T(text.CmdTitle(string(c.ID)))
		if c.Name != "" {
			usage := c.Usage
			if usage == "" {
				usage = ":" + c.Name
			}
			label += "  " + theme.Dim.Render(usage)
		}
		*lines = append(*lines,
			"  "+theme.HelpKey.Render(keyCol)+" "+theme.Row.Render(label))
	}

	if scope == command.ScopeDetail {
		*lines = append(*lines, "")
		*lines = append(*lines, "  "+theme.Dim.Render("; jump — shows inline anchor labels, j/k cycle sections"))
	}

	avail := freeLetters(km.BoundLetters(scope))
	if len(avail) > 0 {
		var b strings.Builder
		for i, r := range avail {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(string(r))
		}
		*lines = append(*lines, "  "+theme.Dim.Render(catalog.T(text.HelpSectionAvailable)+":")+" "+theme.HelpKey.Render(b.String()))
	}
}

// buildHelpLines renders the registry+keymap into styled help lines.
func buildHelpLines(reg *command.Registry, km *keymap.Keymaps, theme render.Theme, catalog *text.Catalog) []string {
	var lines []string

	// General section — base layer (always active).
	section(reg, &lines, theme, catalog, text.HelpSectionGlobal, km.Base())

	// Per-scope sections with available keys.
	scopes := []struct {
		name  text.Key
		scope command.Scope
	}{
		{text.HelpSectionCatalog, command.ScopeCatalog},
		{text.HelpSectionDetail, command.ScopeDetail},
		{text.HelpSectionCitations, command.ScopeCitations},
		{text.HelpSectionFamily, command.ScopeFamily},
		{text.HelpSectionIDS, command.ScopeIDS},
		{text.HelpSectionProjects, command.ScopeProjects},
		{text.HelpSectionOverlay, command.ScopeOverlay},
	}
	for _, s := range scopes {
		scopeSection(reg, &lines, theme, catalog, s.name, km, s.scope)
	}
	return lines
}
