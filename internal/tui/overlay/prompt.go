package overlay

import (
	"cmp"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"patentmine/internal/command"
	"patentmine/internal/text"
	"patentmine/internal/tui/keymap"
	"patentmine/internal/tui/render"
)

const (
	promptShortcutW = 18
	promptNameW     = 22
)

type promptEntry struct {
	command   command.Command
	shortcuts []string
}

// Prompt is a rich command popup for both / palette search and : command entry.
type Prompt struct {
	mode     PromptMode
	ctx      command.Context
	theme    render.Theme
	catalog  *text.Catalog
	handlers map[command.ID]cmdHandler
	all      []promptEntry
	shown    []promptEntry
	page     render.Paginator
	query    string
	cursor   int // rune index into query
	error    string
}

// NewPrompt builds a prompt overlay for commands valid in ctx.
func NewPrompt(reg *command.Registry, km *keymap.Keymaps, theme render.Theme, catalog *text.Catalog, ctx command.Context, mode PromptMode) *Prompt {
	cmds := reg.TypedInContext(ctx)
	all := make([]promptEntry, 0, len(cmds))
	for _, c := range cmds {
		all = append(all, promptEntry{command: c, shortcuts: km.Shortcuts(ctx, c.ID)})
	}
	p := &Prompt{mode: mode, ctx: ctx, theme: theme, catalog: catalog, all: all, page: render.NewPaginator(8)}
	p.handlers = map[command.ID]cmdHandler{
		command.NavDown:     func(r int) tea.Cmd { p.page.MoveDown(r); return nil },
		command.NavUp:       func(r int) tea.Cmd { p.page.MoveUp(r); return nil },
		command.NavPageDown: func(int) tea.Cmd { p.page.PageDown(); return nil },
		command.NavPageUp:   func(int) tea.Cmd { p.page.PageUp(); return nil },
	}
	p.filter()
	return p
}

func (p *Prompt) Title() string {
	if p.mode == PromptDirect {
		return p.catalog.T(text.OverlayCommandTitle)
	}
	return p.catalog.T(text.OverlayCommandsTitle)
}

func (p *Prompt) SourceContext() command.Context { return p.ctx }

func (p *Prompt) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if handler, ok := p.handlers[id]; ok {
		return p, handler(repeat)
	}
	return p, nil
}

// Handles implements Overlay.
func (p *Prompt) Handles() []command.ID { return handlerIDs(p.handlers) }

func (p *Prompt) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyEsc:
		return p, func() tea.Msg { return PromptCloseMsg{} }, true
	case tea.KeyEnter:
		input := p.submitInput()
		if input == "" {
			return p, nil, true
		}
		return p, func() tea.Msg { return PromptSubmitMsg{Input: input} }, true
	case tea.KeyBackspace:
		if p.cursor == 0 {
			return p, nil, true
		}
		runes := []rune(p.query)
		p.query = string(append(runes[:p.cursor-1], runes[p.cursor:]...))
		p.cursor--
		p.error = ""
		p.filter()
		return p, nil, true
	case tea.KeyLeft:
		if p.cursor > 0 {
			p.cursor--
		}
		return p, nil, true
	case tea.KeyRight:
		if p.cursor < len([]rune(p.query)) {
			p.cursor++
		}
		return p, nil, true
	case tea.KeyUp:
		p.page.MoveUp(1)
		return p, nil, true
	case tea.KeyDown:
		p.page.MoveDown(1)
		return p, nil, true
	case tea.KeyRunes, tea.KeySpace:
		runes := []rune(p.query)
		ins := []rune(msg.String())
		merged := make([]rune, 0, len(runes)+len(ins))
		merged = append(merged, runes[:p.cursor]...)
		merged = append(merged, ins...)
		merged = append(merged, runes[p.cursor:]...)
		p.query = string(merged)
		p.cursor += len(ins)
		p.error = ""
		p.filter()
		return p, nil, true
	}
	return p, nil, false
}

func (p *Prompt) View(maxW, maxH int) string {
	if maxH < 6 {
		return ""
	}
	p.page.SetPageSize(max(maxH-5, 1))
	var b strings.Builder
	b.WriteString(p.inputLine(maxW))
	b.WriteString("\n\n")
	b.WriteString(p.listView(maxW))
	b.WriteString("\n\n")
	b.WriteString(p.footerLine(maxW))
	return b.String()
}

func (p *Prompt) submitInput() string {
	if p.mode == PromptPalette {
		if selected, ok := p.selected(); ok {
			return selected.command.Name
		}
		return ""
	}
	return strings.TrimSpace(p.query)
}

func (p *Prompt) inputLine(maxW int) string {
	prefix := "/"
	hint := p.catalog.T(text.PromptFilterHint)
	if p.mode == PromptDirect {
		prefix = ":"
		hint = p.catalog.T(text.PromptDirectHint)
	}
	runes := []rune(p.query)
	before := string(runes[:p.cursor])
	after := string(runes[p.cursor:])
	var input string
	if p.query == "" {
		input = prefix + " " + p.theme.Title.Render("█") + p.theme.Dim.Render(" "+hint)
	} else {
		input = prefix + " " + before + p.theme.Title.Render("█") + after
	}
	return render.Truncate(input, maxW)
}

func (p *Prompt) listView(maxW int) string {
	if len(p.shown) == 0 {
		return p.theme.Dim.Render(p.catalog.T(text.PromptNoMatch))
	}
	var b strings.Builder
	head := render.Pad("SHORTCUT", promptShortcutW) + " " + render.Pad("COMMAND", promptNameW) + " TITLE"
	b.WriteString(p.theme.Header.Render(render.Truncate(head, maxW)))
	start, end := p.page.Window()
	for i := start; i < end; i++ {
		entry := p.shown[i]
		line := render.Pad(strings.Join(entry.shortcuts, " / "), promptShortcutW) + " " +
			render.Pad(entry.command.Name, promptNameW) + " " +
			p.catalog.T(text.CmdTitle(string(entry.command.ID)))
		b.WriteByte('\n')
		if i == p.page.Cursor() {
			b.WriteString(p.theme.Selected.Render(render.Pad(render.Truncate(line, maxW), maxW)))
		} else {
			b.WriteString(p.theme.Row.Render(render.Truncate(line, maxW)))
		}
	}
	return b.String()
}

func (p *Prompt) footerLine(maxW int) string {
	if p.error != "" {
		return render.Truncate(p.theme.Error.Render(p.error), maxW)
	}
	selected, ok := p.selected()
	if !ok {
		return render.Truncate(p.theme.Dim.Render(p.catalog.T(text.PromptRunHint)), maxW)
	}
	usage := selected.command.Usage
	if usage == "" {
		usage = ":" + selected.command.Name
	}
	summary := p.catalog.T(text.CmdHelp(string(selected.command.ID)))
	if p.mode == PromptDirect {
		summary = p.catalog.T(text.CmdTitle(string(selected.command.ID)))
	}
	usage = render.Truncate(usage, maxW)
	if ansi.StringWidth(usage) >= maxW {
		return p.theme.HelpKey.Render(usage)
	}
	remaining := maxW - ansi.StringWidth(usage) - 2
	if remaining <= 0 {
		return p.theme.HelpKey.Render(usage)
	}
	return p.theme.HelpKey.Render(usage) + "  " + p.theme.Row.Render(render.Truncate(summary, remaining))
}

func (p *Prompt) selected() (promptEntry, bool) {
	if len(p.shown) == 0 {
		return promptEntry{}, false
	}
	idx := p.page.Cursor()
	if idx < 0 || idx >= len(p.shown) {
		return promptEntry{}, false
	}
	return p.shown[idx], true
}

func (p *Prompt) filter() {
	needle := strings.ToLower(strings.TrimSpace(p.query))
	type scoredEntry struct {
		entry promptEntry
		score int
	}
	var scored []scoredEntry
	for _, entry := range p.all {
		if score, ok := promptScore(entry, needle, p.catalog); ok {
			scored = append(scored, scoredEntry{entry: entry, score: score})
		}
	}
	slices.SortFunc(scored, func(a, b scoredEntry) int {
		if a.score != b.score {
			return cmp.Compare(a.score, b.score)
		}
		return cmp.Compare(a.entry.command.Name, b.entry.command.Name)
	})
	p.shown = p.shown[:0]
	for _, entry := range scored {
		p.shown = append(p.shown, entry.entry)
	}
	p.page.SetTotal(len(p.shown))
	if len(p.shown) == 0 {
		p.page.Top()
	}
}

func promptScore(entry promptEntry, needle string, catalog *text.Catalog) (int, bool) {
	if needle == "" {
		return 1000, true
	}
	fields := []string{
		entry.command.Name,
		catalog.T(text.CmdTitle(string(entry.command.ID))),
		catalog.T(text.CmdHelp(string(entry.command.ID))),
	}
	fields = append(fields, entry.command.Aliases...)
	fields = append(fields, entry.shortcuts...)
	best := 0
	found := false
	for _, field := range fields {
		score, ok := fieldScore(strings.ToLower(field), needle)
		if !ok {
			continue
		}
		if !found || score < best {
			best = score
			found = true
		}
	}
	return best, found
}

func fieldScore(field, needle string) (int, bool) {
	if field == needle {
		return 0, true
	}
	if strings.HasPrefix(field, needle) {
		return 10 + len(field) - len(needle), true
	}
	if idx := strings.Index(field, needle); idx >= 0 {
		return 100 + idx, true
	}
	if gap, ok := subsequenceGap(field, needle); ok {
		return 200 + gap, true
	}
	return 0, false
}

func subsequenceGap(field, needle string) (int, bool) {
	if needle == "" {
		return 0, true
	}
	idx := 0
	start := -1
	for _, r := range needle {
		pos := strings.IndexRune(field[idx:], r)
		if pos < 0 {
			return 0, false
		}
		if start < 0 {
			start = idx + pos
		}
		idx += pos + 1
	}
	end := idx - 1
	span := end - start + 1
	return span - len([]rune(needle)), true
}
