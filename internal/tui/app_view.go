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
	header := a.headerBlock(focused)
	headerLines := 0
	if header != "" {
		headerLines = strings.Count(header, "\n") + 1
	}
	bodyHeight := max(a.height-headerLines-statusRows, 1)

	body := fitBody(focused.View(a.width, bodyHeight), bodyHeight)

	statusStyle := a.theme.Status
	if a.statusErr {
		statusStyle = a.theme.Error
	}
	status := statusStyle.Render(render.Pad(" "+a.statusText(), a.width))

	screen := body + "\n" + status
	if header != "" {
		screen = header + "\n" + screen
	}
	if len(a.overlays) > 0 {
		screen = a.compositeOverlay(screen)
	}
	return screen
}

func (a *App) headerBlock(focused pane.Pane) string {
	if splash, ok := focused.(interface{ IsSplash() bool }); ok && splash.IsSplash() {
		return ""
	}
	line1 := a.renderScreenHeader(focused)
	line2 := a.theme.Dim.Render(render.Pad(" "+a.helperLine(focused.Scope()), a.width))
	line3 := a.theme.Header.Render(strings.Repeat("─", a.width))
	return line1 + "\n" + line2 + "\n" + line3
}

func (a *App) renderScreenHeader(focused pane.Pane) string {
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
	return render.Pad(" "+b.String(), a.width)
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

// compositeOverlay draws the focused overlay centred over the dimmed screen.
func (a *App) compositeOverlay(screen string) string {
	boxWidth := min(a.width-overlayMargin, overlayMaxWidth)
	boxHeight := min(a.height-overlayMargin, overlayMaxHeight)
	if boxWidth < 16 || boxHeight < 6 {
		return screen // terminal too small to frame an overlay
	}
	ov := a.focusedOverlay()
	innerWidth := boxWidth - overlayChrome
	content := a.theme.Title.Render(ov.Title()) + "\n\n" +
		ov.View(innerWidth, boxHeight-overlayChrome)
	box := a.theme.Box.Width(innerWidth).Height(boxHeight - 2).Render(content)

	dimmed := render.Dim(screen)
	x, y := render.CenterOffset(a.width, a.height, lipgloss.Width(box), lipgloss.Height(box))
	return render.Composite(dimmed, box, x, y)
}

// fitBody pads or trims rendered pane output to exactly height lines so the
// status line always sits on the bottom row.
func fitBody(body string, height int) string {
	lines := strings.Split(body, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:height], "\n")
}
