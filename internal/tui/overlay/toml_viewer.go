package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
	"patentmine/internal/uspto"
)

// TOMLViewer is a scrollable modal overlay that displays a structured document
// formatted as TOML with standard styling.
//
// Why this looks quite different from FullText (the other main line-based viewer):
//
//   - FullText is a first-class Pane. It can implement JumpProvider, get
//     special treatment from cmdJumpMode, use the shared *JumpController +
//     render.JumpAnchor machinery, etc.
//   - TOMLViewer is an Overlay. cmdJumpMode explicitly does nothing when any
//     overlay is open, so we have to handle ";" (and everything else) locally.
//   - FullText renders from rich structured data (claims + paragraphs) and can
//     assign meaningful labels during its render pass. TOMLViewer receives a
//     flat []string of already-rendered text, so its jump implementation is
//     necessarily much simpler (just letters for the current window).
//   - The viewer started life as a minimal "pretty printer". Search, visual
//     selection, yank, and jump were added incrementally, each time with the
//     smallest local implementation that made the feature work.
//
// As a result you see duplicated logic for visual selection (visualMode,
// visualAnchor, toggleVisual, inVisualRange, yank) and a hand-rolled jump
// system instead of the shared JumpController.
type TOMLViewer struct {
	theme    render.Theme
	title    string
	lines    []string
	page     render.Paginator
	handlers map[command.ID]cmdHandler

	// Search state
	searchInput  string
	searchActive bool   // true while the user is typing the query
	searchMatches []int // absolute line indices matching the current query
	searchCur     int   // index into searchMatches for current match

	// Jump mode (activated with ';') – reuses the official JumpController
	// from the pane package for consistency with FullText / Detail.
	jump *pane.JumpController

	// Visual selection reuses the shared VisualState from the pane package
	// (the same one used by catalog, citations, etc.).
	visual pane.VisualState
}

// NewTOMLViewer builds a TOML viewer overlay.
func NewTOMLViewer(theme render.Theme, title string, tomlText string) *TOMLViewer {
	lines := strings.Split(tomlText, "\n")
	page := render.NewPaginator(20)
	page.SetTotal(len(lines))

	v := &TOMLViewer{
		theme: theme,
		title: title,
		lines: lines,
		page:  page,
		jump:  pane.NewJumpController(),
	}

	v.handlers = map[command.ID]cmdHandler{
		command.NavDown:      func(r int) tea.Cmd { v.page.MoveDown(r); return nil },
		command.NavUp:        func(r int) tea.Cmd { v.page.MoveUp(r); return nil },
		command.NavPageDown:  func(int) tea.Cmd { v.page.PageDown(); return nil },
		command.NavPageUp:    func(int) tea.Cmd { v.page.PageUp(); return nil },
		command.NavTop:       func(int) tea.Cmd { v.page.ScrollTo(0); return nil },
		command.NavBottom:    func(int) tea.Cmd { v.page.ScrollTo(len(lines) - 1); return nil },
		command.SelectVisual: func(int) tea.Cmd { v.visual.Toggle(v.page.Cursor()); return nil },
		command.CopyYank:     func(int) tea.Cmd { return v.yankSelection() },
		command.CopyYankMeta: func(int) tea.Cmd { return v.yankSelection() }, // treat the same for this viewer
	}

	return v
}

// Title implements Overlay.
func (v *TOMLViewer) Title() string {
	return v.title
}

// Init implements Overlay.
func (v *TOMLViewer) Init() tea.Cmd {
	return nil
}

// Command implements Overlay.
func (v *TOMLViewer) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if handler, ok := v.handlers[id]; ok {
		return v, handler(repeat)
	}
	return v, nil
}

// Handles implements Overlay.
func (v *TOMLViewer) Handles() []command.ID {
	return handlerIDs(v.handlers)
}

// HandleKey implements KeyHandler to close on esc/q/enter.
func (v *TOMLViewer) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	// While actively typing a search query, consume most keys for input.
	if v.searchActive {
		switch msg.Type {
		case tea.KeyEnter:
			v.searchActive = false
			v.updateSearchMatches()
			if len(v.searchMatches) > 0 {
				v.searchCur = 0
				v.page.ScrollTo(v.searchMatches[0])
			}
			return v, nil, true
		case tea.KeyEsc:
			v.searchActive = false
			v.searchInput = ""
			v.searchMatches = nil
			return v, nil, true
		case tea.KeyBackspace, tea.KeyDelete:
			if len(v.searchInput) > 0 {
				runes := []rune(v.searchInput)
				v.searchInput = string(runes[:len(runes)-1])
			}
			return v, nil, true
		case tea.KeyCtrlU:
			v.searchInput = ""
			return v, nil, true
		case tea.KeyCtrlW:
			// delete last word
			trimmed := strings.TrimRight(v.searchInput, " ")
			if idx := strings.LastIndex(trimmed, " "); idx >= 0 {
				v.searchInput = v.searchInput[:idx+1]
			} else {
				v.searchInput = ""
			}
			return v, nil, true
		case tea.KeyRunes:
			v.searchInput += string(msg.Runes)
			return v, nil, true
		}
		return v, nil, true // consume everything else while in search input
	}

	s := msg.String()
	switch s {
	case "esc", "q":
		if v.visual.Mode {
			v.visual.Clear()
			return v, nil, true
		}
		return v, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "enter":
		if v.visual.Mode {
			// Enter while visual: yank and exit visual (convenience)
			cmd := v.yankSelection()
			return v, cmd, true
		}
		return v, func() tea.Msg { return CloseOverlayMsg{} }, true

	// Search
	case "/":
		v.searchActive = true
		v.searchInput = ""
		return v, nil, true
	case "n":
		v.nextSearchMatch(+1)
		return v, nil, true
	case "N":
		v.nextSearchMatch(-1)
		return v, nil, true

	// Basic navigation (also available via commands)
	case "j", "down":
		v.page.MoveDown(1)
		return v, nil, true
	case "k", "up":
		v.page.MoveUp(1)
		return v, nil, true
	case "ctrl+d", "pgdown":
		v.page.PageDown()
		return v, nil, true
	case "ctrl+u", "pgup":
		v.page.PageUp()
		return v, nil, true
	case "g", "home":
		v.page.ScrollTo(0)
		return v, nil, true
	case "G", "end":
		v.page.ScrollTo(len(v.lines) - 1)
		return v, nil, true

	// Visual selection
	case "v", "V":
		v.visual.Toggle(v.page.Cursor())
		return v, nil, true

	// Yank (works with or without visual)
	case "y", "Y":
		cmd := v.yankSelection()
		return v, cmd, true

	// Jump / highlight mode (one-key line cursor movement) — delegate to controller
	case ";":
		active := !v.jump.JumpActive()
		v.jump.SetJumpActive(active)
		if active {
			v.populateJumpAnchorsForCurrentView()
		} else {
			v.jump.ClearAnchors()
		}
		return v, nil, true
	}

	// If the JumpController is active, let it handle the key (letter jump or cancel)
	if v.jump.JumpActive() {
		consumed := v.jump.HandleKey(msg, func(line int) {
			v.page.ScrollTo(line)
		})
		if consumed {
			return v, nil, true
		}
	}

	return v, nil, false
}

// populateJumpAnchorsForCurrentView creates jump targets for the visible
// window, giving priority to TOML section headers (matching the spirit of
// how FullText prioritizes "Claim N" and disclosure sections).
func (v *TOMLViewer) populateJumpAnchorsForCurrentView() {
	v.jump.ClearAnchors()

	start, end := v.page.Window()
	if start >= end {
		return
	}

	labels := []string{}
	lineForLabel := map[string]int{}

	for i := start; i < end; i++ {
		line := v.lines[i]
		trimmed := strings.TrimSpace(line)

		var label string
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			label = trimmed
		} else if strings.Contains(line, " = ") {
			// For key lines, use a short prefix as label
			if parts := strings.SplitN(trimmed, " = ", 2); len(parts) > 0 {
				label = strings.TrimSpace(parts[0])
				if len(label) > 30 {
					label = label[:30] + "…"
				}
			}
		}

		if label != "" {
			if _, exists := lineForLabel[label]; !exists {
				labels = append(labels, label)
				lineForLabel[label] = i
			}
		}
	}

	// Use the controller's Compute to assign non-conflicting keys
	v.jump.Compute(labels, nil, nil, false)

	// Register anchors using the assigned keys
	for _, label := range labels {
		key := v.jump.JumpKey(label)
		if key != 0 {
			line := lineForLabel[label]
			v.jump.AddAnchor(label, "", line, false, key)
		}
	}
}

// Update keeps the overlay ticking.
func (v *TOMLViewer) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	return v, nil
}

// OverlaySize sets the dimensions to show maximum contents.
func (v *TOMLViewer) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, 85, 85, 80, 24)
}

// View renders the TOML lines with dedicated theme syntax highlighting.
// It also highlights the current visual selection and search matches.
func (v *TOMLViewer) View(maxW, maxH int) string {
	contentH := maxH
	searchBarH := 0
	if v.searchActive {
		searchBarH = 1
		contentH = max(1, maxH-searchBarH)
	}

	v.page.SetPageSize(max(contentH, 1))
	start, end := v.page.Window()

	var b strings.Builder
	for i := start; i < end; i++ {
		if i > start {
			b.WriteByte('\n')
		}
		line := v.lines[i]

		// Determine highlight state (visual takes precedence over search)
		inVisual := v.visual.InRange(v.page.Cursor(), i)
		isCurMatch := len(v.searchMatches) > 0 && v.searchCur < len(v.searchMatches) && i == v.searchMatches[v.searchCur]
		isAnyMatch := len(v.searchMatches) > 0 && v.lineMatchesSearch(i)

		trimmed := strings.TrimSpace(line)
		var rendered string
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			rendered = v.theme.Header.Render(render.Truncate(line, maxW))
		} else if strings.Contains(line, " = ") {
			parts := strings.SplitN(line, " = ", 2)
			rendered = v.theme.HelpKey.Render(parts[0]) +
				v.theme.Dim.Render(" = ") +
				v.theme.Row.Render(render.Truncate(parts[1], maxW-len(parts[0])-3))
		} else {
			rendered = v.theme.Row.Render(render.Truncate(line, maxW))
		}

		switch {
		case inVisual:
			rendered = v.theme.Selected.Render(render.Truncate(stripForHighlight(line), maxW))
		case isCurMatch:
			// Current search match gets strong highlight
			rendered = v.theme.Selected.Render(render.Truncate(line, maxW))
		case isAnyMatch:
			// Other matches get a lighter treatment
			rendered = v.theme.Header.Render(render.Truncate(line, maxW))
		}

		// Jump mode: prefix visible lines with their assigned key (e.g. "[a] ")
		// using the official JumpController anchors (recycled from FullText pattern).
		if v.jump.JumpActive() {
			for _, a := range v.jump.JumpAnchors() {
				if a.Line == i && a.Key != 0 {
					keyStr := v.theme.Warn.Copy().Bold(true).Render(fmt.Sprintf("[%c]", a.Key))
					rendered = keyStr + " " + rendered
					break
				}
			}
		}

		b.WriteString(rendered)
	}

	if v.searchActive {
		bar := "/ " + v.searchInput + "▋"
		if len(v.searchMatches) > 0 {
			bar += "  (" + matchLabel(len(v.searchMatches)) + ")"
		}
		b.WriteString("\n" + v.theme.Dim.Render(render.Pad(bar, maxW)))
	}

	if v.jump.JumpActive() {
		hint := v.theme.Dim.Render("  jump mode · press a highlighted letter (or any other key to cancel)")
		b.WriteString("\n" + hint)
	}

	return b.String()
}

// --- Search helpers ---------------------------------------------------------

func (v *TOMLViewer) updateSearchMatches() {
	v.searchMatches = nil
	q := strings.ToLower(strings.TrimSpace(v.searchInput))
	if q == "" {
		return
	}
	for i, line := range v.lines {
		if strings.Contains(strings.ToLower(line), q) {
			v.searchMatches = append(v.searchMatches, i)
		}
	}
}

func (v *TOMLViewer) nextSearchMatch(delta int) {
	if len(v.searchMatches) == 0 {
		return
	}
	v.searchCur = (v.searchCur + delta + len(v.searchMatches)) % len(v.searchMatches)
	v.page.ScrollTo(v.searchMatches[v.searchCur])
}

func (v *TOMLViewer) lineMatchesSearch(abs int) bool {
	for _, m := range v.searchMatches {
		if m == abs {
			return true
		}
	}
	return false
}

func matchLabel(n int) string {
	if n == 1 {
		return "1 match"
	}
	return fmt.Sprintf("%d matches", n)
}

// yankSelection is implemented above using the visual helper.

func (v *TOMLViewer) yankSelection() tea.Cmd {
	if len(v.lines) == 0 {
		return nil
	}

	var text string
	if v.visual.Mode {
		lo, hi := v.visual.Anchor, v.page.Cursor()
		if lo > hi {
			lo, hi = hi, lo
		}
		lo = max(lo, 0)
		hi = min(hi, len(v.lines)-1)
		if lo <= hi {
			text = strings.Join(v.lines[lo:hi+1], "\n")
		}
		v.visual.Clear()
	} else {
		cur := v.page.Cursor()
		if cur >= 0 && cur < len(v.lines) {
			text = v.lines[cur]
		}
	}

	if text == "" {
		return nil
	}

	return func() tea.Msg {
		return pane.CopyToClipboardMsg{Text: text}
	}
}

// stripForHighlight is a tiny helper so we don't double-render ANSI when
// wrapping an already-styled line for visual selection.
func stripForHighlight(s string) string {
	// Very naive strip of common ANSI sequences we emit ourselves.
	// Good enough for this viewer.
	return strings.NewReplacer(
		"\x1b[0m", "",
		"\x1b[1m", "",
		"\x1b[2m", "",
	).Replace(s)
}

// StructToTOML is a thin forwarding helper so existing call sites inside the
// overlay package continue to work after the pure formatting logic moved to
// the uspto package (where the daemon can also use it for RPC-backed viewing).
func StructToTOML(val any) (string, error) {
	return uspto.StructToTOML(val)
}


