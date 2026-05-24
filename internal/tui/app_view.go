package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/command"
	"patentmine/internal/text"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
)

// View implements tea.Model.
func (a *App) View() string {
	if a.width == 0 || a.height == 0 {
		return "starting…"
	}
	focused := a.focusedPane()
	viewWidth := safeViewWidth(a.width)
	header := a.headerBlock(focused)
	headerLines := 0
	if header != "" {
		headerLines = strings.Count(header, "\n") + 1
	}
	bodyHeight := max(a.height-headerLines-statusRows, 1)

	body := fitBody(focused.View(viewWidth, bodyHeight), bodyHeight, viewWidth)

	statusStyle := a.theme.Status
	if a.statusErr {
		statusStyle = a.theme.Error
	}
	status := statusStyle.Render(render.Pad(" "+a.statusText(), viewWidth))

	screen := body + "\n" + status
	if header != "" {
		screen = header + "\n" + screen
	}
	if len(a.overlays) > 0 {
		screen = a.compositeOverlay(screen)
	}
	return clearLineEnds(screen)
}

func (a *App) headerBlock(focused pane.Pane) string {
	if splash, ok := focused.(interface{ IsSplash() bool }); ok && splash.IsSplash() {
		return ""
	}
	viewWidth := safeViewWidth(a.width)
	line1 := a.renderScreenHeader(focused, viewWidth)
	line2 := a.theme.Dim.Render(render.Pad(" "+a.helperLine(focused.Scope()), viewWidth))
	line3 := a.theme.Header.Render(strings.Repeat("─", viewWidth))
	return line1 + "\n" + line2 + "\n" + line3
}

func (a *App) renderScreenHeader(focused pane.Pane, width int) string {
	var b strings.Builder
	b.WriteString(a.theme.Title.Render("PatentMine"))
	if a.activeProject != nil {
		b.WriteString(" ")
		project := a.activeProject.Name + " [" + string(a.activeProject.ID) + "]"
		b.WriteString(a.theme.Row.Render(project))
	}
	b.WriteString(" ")
	b.WriteString(a.theme.Header.Render("· "))
	b.WriteString(a.theme.Row.Bold(true).Render(focused.Title()))
	return render.Pad(" "+b.String(), width)
}

func (a *App) helperLine(scope command.Scope) string {
	var parts []string
	for _, h := range a.hints.For(scope) {
		if len(h.Commands) == 1 {
			parts = append(parts, a.shortcutHint(scope, h.Commands[0], h.Label))
		} else {
			parts = append(parts, a.multiShortcutHint(scope, h.Commands, h.Label))
		}
	}
	return a.joinHints(parts...)
}

func (a *App) splashFooterHint() string {
	scope := command.ScopeProjects
	return a.joinHints(
		a.navigationHint(scope),
		a.shortcutHint(scope, command.ProjectActivate, text.HintSelect),
		a.text.T(text.HintSlashCommands),
		a.shortcutHint(scope, command.OpenCommand, text.HintCommand),
		a.shortcutHint(scope, command.ProjectCreate, text.HintNewProject),
		a.shortcutHint(scope, command.Quit, text.HintQuit),
	)
}

func (a *App) splashEmptyHint() string {
	scope := command.ScopeProjects
	createUsage := ":project.create"
	if cmd, ok := a.registry.Lookup(command.ProjectCreate); ok && cmd.Usage != "" {
		createUsage = cmd.Usage
	}
	shortcut := a.shortcutKeys(scope, command.ProjectCreate)
	if shortcut == "" {
		return a.text.Tf(text.SplashCreateHint, createUsage)
	}
	return a.text.Tf(text.SplashCreateKeyHint, createUsage, shortcut)
}

func (a *App) navigationHint(scope command.Scope) string {
	down := a.shortcutKeys(scope, command.NavDown)
	up := a.shortcutKeys(scope, command.NavUp)
	move := a.text.T(text.HintMove)
	if down == "" && up == "" {
		return move
	}
	if down == "" {
		return up + " " + move
	}
	if up == "" {
		return down + " " + move
	}
	return down + "/" + up + " " + move
}

func (a *App) shortcutHint(scope command.Scope, id command.ID, labelKey text.Key) string {
	label := a.text.T(labelKey)
	keys := a.shortcutKeys(scope, id)
	if keys == "" {
		return label
	}
	return keys + " " + label
}

func (a *App) multiShortcutHint(scope command.Scope, ids []command.ID, labelKey text.Key) string {
	label := a.text.T(labelKey)
	var keys []string
	for _, id := range ids {
		k := a.shortcutKeys(scope, id)
		if k != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return label
	}
	return strings.Join(keys, "/") + " " + label
}

func (a *App) shortcutKeys(scope command.Scope, id command.ID) string {
	shortcuts := a.keymaps.Shortcuts(scope, id)
	if len(shortcuts) == 0 && scope != command.ScopeOverlay {
		shortcuts = a.keymaps.Shortcuts(command.ScopeOverlay, id)
	}
	if len(shortcuts) == 0 {
		return ""
	}
	return strings.Join(shortcuts, "/")
}

func (a *App) joinHints(parts ...string) string {
	filtered := parts[:0]
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, " · ")
}

// statusText appends the chord reader's pending input, Vim-style.
func (a *App) statusText() string {
	versionText := "   [tui " + a.tuiVersion + " | daemon " + a.daemonVersion + "]"
	if a.activeProject != nil {
		versionText += "   [project " + a.activeProject.Name + "]"
	}
	visual := ""
	if ms, ok := a.focusedPane().(pane.MultiSelector); ok {
		if sels := ms.Selections(); len(sels) > 0 {
			visual = fmt.Sprintf("   [VISUAL %d]", len(sels))
		}
	}
	if pending := a.reader.Pending(); pending != "" {
		return a.status + versionText + visual + "   [" + pending + "]"
	}
	return a.status + versionText + visual
}

// DynamicSize is implemented by overlays that wish to customize their own size
// rather than use the default bounding box.
type DynamicSize interface {
	OverlaySize(termW, termH int) (w, h int)
}

var emojiPlaceholders = []struct {
	Emoji       string
	Placeholder string
}{
	// History icons (with variation selectors)
	{"👁️", "\uE001"},
	{"🏷️", "\uE002"},
	{"⚡️", "\uE003"},
	// History icons (plain)
	{"👁", "\uE001"},
	{"🏷", "\uE002"},
	{"⚡", "\uE003"},
	// Other emojis (measured as 1-cell)
	{"❓", "\uE004"},
	{"🔻", "\uE005"},
	{"📂", "\uE006"},
	{"🔗", "\uE007"},
	{"🌳", "\uE008"},
	{"📄", "\uE009"},
	{"📋", "\uE00A"},
	{"✅", "\uE00B"},
	{"➖", "\uE00C"},
	{"📥", "\uE00D"},
	{"🩺", "\uE00E"},
	{"⚙️", "\uE00F"},
	{"⚙", "\uE00F"},
	{"🔍", "\uE010"},
}

// compositeOverlay draws the focused overlay centred over the dimmed screen.
func (a *App) compositeOverlay(screen string) string {
	boxWidth := min(a.width-overlayMargin, overlayMaxWidth)
	boxHeight := min(a.height-overlayMargin, overlayMaxHeight)
	ov := a.focusedOverlay()
	if ds, ok := ov.(DynamicSize); ok {
		boxWidth, boxHeight = ds.OverlaySize(a.width, a.height)
	}
	if boxWidth < 16 || boxHeight < 6 {
		return screen // terminal too small to frame an overlay
	}
	innerWidth := boxWidth - overlayChrome
	content := a.theme.Title.Render(ov.Title()) + "\n\n" +
		ov.View(innerWidth, boxHeight-overlayChrome)

	// Replace emojis with 2-cell private-use placeholders so Lipgloss/ansi measures width perfectly
	for _, mapping := range emojiPlaceholders {
		content = strings.ReplaceAll(content, mapping.Emoji, mapping.Placeholder)
	}

	box := a.theme.Box.Width(innerWidth).Height(boxHeight - 2).Render(content)

	// Restore original emojis with full variability
	for _, mapping := range emojiPlaceholders {
		box = strings.ReplaceAll(box, mapping.Placeholder, mapping.Emoji)
	}

	dimmed := render.Dim(screen)
	x, y := render.CenterOffset(a.width, a.height, lipgloss.Width(box), lipgloss.Height(box))
	return render.Composite(dimmed, box, x, y)
}

func safeViewWidth(width int) int {
	return max(width-1, 1)
}

func clearLineEnds(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = line + "\x1b[K"
	}
	return strings.Join(lines, "\n")
}

// fitBody pads or trims rendered pane output to exactly height lines so the
// status line always sits on the bottom row.
func fitBody(body string, height, width int) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = render.Pad(render.Truncate(line, width), width)
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines[:height], "\n")
}
