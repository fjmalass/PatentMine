package pane

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/rpc"
	"patentmine/internal/tui/render"
)

// USPTORawXML is a full-screen pane (analogous to FullText) for viewing
// the raw fetched USPTO grant/pgpub XML as nicely formatted TOML.
// It lives in the main pane stack (full screen below the header) rather
// than as a popup overlay.
type USPTORawXML struct {
	client *rpc.Client
	theme  render.Theme
	number domain.PatentNumber
	kind   string // "grant" or "pgpub"
	title  string

	lines []string
	page  render.Paginator

	// Viewer state (recycled from shared pane helpers)
	visual VisualState
	jump   *JumpController

	// Search (inline, matching the previous overlay behavior)
	searchInput   string
	searchActive  bool
	searchMatches []int
	searchCur     int

	// Jump "highlight mode" (separate from the controller's Active for
	// the inline [a][b] style the user likes)
	jumpMode      bool
	jumpKeys      map[rune]int
	jumpLineToKey map[int]rune

	handlers map[command.ID]cmdHandler
}

func NewUSPTORawXML(
	client *rpc.Client,
	theme render.Theme,
	number domain.PatentNumber,
	kind string,
	tomlText string,
	displayTitle string,
) *USPTORawXML {
	lines := strings.Split(tomlText, "\n")
	p := render.NewPaginator(20)
	p.SetTotal(len(lines))

	v := &USPTORawXML{
		client: client,
		theme:  theme,
		number: number,
		kind:   kind,
		title:  displayTitle,
		lines:  lines,
		page:   p,
		jump:   NewJumpController(),
	}

	v.handlers = map[command.ID]cmdHandler{
		command.NavDown:      func(inv Invocation) tea.Cmd { v.page.MoveDown(inv.Repeat); return nil },
		command.NavUp:        func(inv Invocation) tea.Cmd { v.page.MoveUp(inv.Repeat); return nil },
		command.NavPageDown:  func(Invocation) tea.Cmd { v.page.PageDown(); return nil },
		command.NavPageUp:    func(Invocation) tea.Cmd { v.page.PageUp(); return nil },
		command.NavTop:       func(inv Invocation) tea.Cmd { v.page.NavTop(inv.Repeat); return nil },
		command.NavBottom:    func(inv Invocation) tea.Cmd { v.page.NavBottom(inv.Repeat); return nil },
		command.SelectVisual: func(Invocation) tea.Cmd { v.visual.Toggle(v.page.Cursor()); return nil },
		command.CopyYank:     func(Invocation) tea.Cmd { return v.yankSelection() },
		command.CopyYankMeta: func(Invocation) tea.Cmd { return v.yankSelection() },
	}

	return v
}

func (v *USPTORawXML) Scope() command.Scope { return command.ScopeUSPTOXML }
func (v *USPTORawXML) Title() string        { return v.title }
func (v *USPTORawXML) Init() tea.Cmd        { return nil }

func (v *USPTORawXML) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
	if h, ok := v.handlers[id]; ok {
		return v, h(inv)
	}
	return v, nil
}

func (v *USPTORawXML) Handles() []command.ID {
	return handlerIDs(v.handlers)
}

func (v *USPTORawXML) Update(msg tea.Msg) (Pane, tea.Cmd) {
	return v, nil
}

func (v *USPTORawXML) View(w, h int) string {
	total := len(v.lines)
	if total == 0 {
		return v.theme.Dim.Render("(empty)")
	}

	// Always reserve one line for the pagination header
	contentH := max(1, h-1)
	extraLines := 1
	if v.searchActive {
		extraLines++
	}
	if !v.searchActive && !v.jumpMode {
		extraLines++ // bottom position indicator
	}
	if extraLines > 1 {
		contentH = max(1, h-extraLines)
	}

	v.page.SetPageSize(max(contentH, 1))
	start, end := v.page.Window()

	cursor := v.page.Cursor()

	// Calculate gutter width for line numbers (1-based)
	gutterDigits := len(fmt.Sprintf("%d", total))
	gutterWidth := gutterDigits + 2 // number + " │ "
	gutterSep := "│ "

	// Pagination header (top)
	pageSize := v.page.PageSize()
	currentPage := 0
	totalPages := 0
	if pageSize > 0 {
		currentPage = (v.page.Offset() / pageSize) + 1
		totalPages = ((total + pageSize - 1) / pageSize)
	}
	if totalPages < 1 {
		totalPages = 1
	}
	header := fmt.Sprintf("Line %d/%d   Page %d/%d", cursor+1, total, currentPage, totalPages)
	var b strings.Builder
	b.WriteString(v.theme.Dim.Render(render.Pad(header, w)))
	b.WriteString("\n")

	for i := start; i < end; i++ {
		if i > start {
			b.WriteByte('\n')
		}

		line := v.lines[i]
		absLine := i + 1 // 1-based for display

		inVisual := v.visual.InRange(cursor, i)
		isCurMatch := len(v.searchMatches) > 0 && v.searchCur < len(v.searchMatches) && i == v.searchMatches[v.searchCur]
		isAnyMatch := len(v.searchMatches) > 0 && v.lineMatchesSearch(i)
		isCursor := (i == cursor) && !inVisual

		rowHighlighted := inVisual || isCursor || isCurMatch

		// Build gutter — highlighted together with the line when appropriate
		lineNum := fmt.Sprintf("%*d", gutterDigits, absLine)
		gutterStyle := v.theme.Dim
		if rowHighlighted {
			gutterStyle = v.theme.Selected
		}
		gutter := gutterStyle.Render(lineNum + gutterSep)

		// Render the TOML content
		trimmed := strings.TrimSpace(line)
		var content string
		avail := w - gutterWidth
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			content = v.theme.Header.Render(render.Truncate(line, avail))
		} else if strings.Contains(line, " = ") {
			parts := strings.SplitN(line, " = ", 2)
			content = v.theme.HelpKey.Render(parts[0]) +
				v.theme.Dim.Render(" = ") +
				v.theme.Row.Render(render.Truncate(parts[1], avail-len(parts[0])-4))
		} else {
			content = v.theme.Row.Render(render.Truncate(line, avail))
		}

		// Apply row-level highlighting to the content part
		if rowHighlighted {
			content = v.theme.Selected.Render(content)
		} else if isAnyMatch {
			content = v.theme.Header.Render(content)
		}

		// Jump mode keys go after gutter but inside the row highlight
		if v.jumpMode {
			if key, has := v.jumpLineToKey[i]; has {
				keyStr := v.theme.Warn.Copy().Bold(true).Render(fmt.Sprintf("[%c]", key))
				content = keyStr + " " + content
				if rowHighlighted {
					// re-apply so the key is also on selected background
					content = v.theme.Selected.Render(keyStr + " " + content)
				}
			}
		}

		fullLine := gutter + content

		// For empty highlighted lines, ensure the background extends
		if rowHighlighted && strings.TrimSpace(line) == "" {
			// Force a full-width highlighted row
			fullLine = v.theme.Selected.Render(strings.Repeat(" ", max(0, w)))
			// Re-apply gutter on top so line number is still visible
			fullLine = gutter + v.theme.Selected.Render(strings.Repeat(" ", max(0, w-gutterWidth)))
		}

		b.WriteString(render.Truncate(fullLine, w))
	}

	// Search bar (when active)
	if v.searchActive {
		bar := "/ " + v.searchInput + "▋"
		if len(v.searchMatches) > 0 {
			bar += "  (" + usptoMatchLabel(len(v.searchMatches)) + ")"
		}
		if len(v.searchMatches) > 0 {
			bar += fmt.Sprintf("  [%d/%d]", v.searchCur+1, len(v.searchMatches))
		}
		b.WriteString("\n" + v.theme.Dim.Render(render.Pad(bar, w)))
	}

	// Jump mode hint
	if v.jumpMode {
		hint := v.theme.Dim.Render("  jump mode · press letter (other key cancels)")
		b.WriteString("\n" + hint)
	}

	// Bottom position (only when nothing else is active)
	if !v.searchActive && !v.jumpMode {
		pos := fmt.Sprintf("  %d/%d", cursor+1, total)
		b.WriteString("\n" + v.theme.Dim.Render(render.Pad(pos, w)))
	}

	return b.String()
}

func (v *USPTORawXML) Selection() (domain.PatentNumber, bool) {
	return v.number, !v.number.IsZero()
}

// --- Search & Jump helpers (recycled logic from previous overlay work) ---

func (v *USPTORawXML) updateSearchMatches() {
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

func (v *USPTORawXML) nextSearchMatch(delta int) {
	if len(v.searchMatches) == 0 {
		return
	}
	v.searchCur = (v.searchCur + delta + len(v.searchMatches)) % len(v.searchMatches)
	v.page.ScrollTo(v.searchMatches[v.searchCur])
}

func (v *USPTORawXML) lineMatchesSearch(abs int) bool {
	for _, m := range v.searchMatches {
		if m == abs {
			return true
		}
	}
	return false
}

func (v *USPTORawXML) toggleJump() {
	if v.jumpMode {
		v.jumpMode = false
		v.jumpKeys = nil
		v.jumpLineToKey = nil
		return
	}
	v.jumpMode = true
	v.jumpKeys = make(map[rune]int)
	v.jumpLineToKey = make(map[int]rune)

	start, end := v.page.Window()
	used := make(map[rune]bool)

	for i := start; i < end && len(v.jumpKeys) < 26; i++ {
		line := v.lines[i]
		trimmed := strings.TrimSpace(line)
		label := trimmed
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			label = "section " + trimmed
		}
		key := render.AssignKey(label, used, false)
		if key != 0 {
			v.jumpKeys[key] = i
			v.jumpLineToKey[i] = key
			used[key] = true
		}
	}
}

func (v *USPTORawXML) yankSelection() tea.Cmd {
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
		text = strings.Join(v.lines[lo:hi+1], "\n")
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
		return CopyToClipboardMsg{Text: text}
	}
}

// Small utilities (duplicated locally for self-containment in this pane for now)
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func usptoMatchLabel(n int) string {
	if n == 1 {
		return "1 match"
	}
	return fmt.Sprintf("%d matches", n)
}

func stripForHighlight(s string) string {
	return strings.NewReplacer("\x1b[0m", "", "\x1b[1m", "", "\x1b[2m", "").Replace(s)
}

// HandleKey implements pane.KeyHandler so the App forwards raw keys to us
// (required for our custom jump highlight mode and future interactive features).
func (v *USPTORawXML) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {
	// === Search input mode (highest priority when active) ===
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
		return v, nil, true // consume everything else while typing search
	}

	// === Start search with / ===
	if msg.String() == "/" {
		v.searchActive = true
		v.searchInput = ""
		return v, nil, true
	}

	// === Search navigation (n/N) - works even when not actively typing ===
	if msg.String() == "n" {
		v.nextSearchMatch(+1)
		return v, nil, true
	}
	if msg.String() == "N" {
		v.nextSearchMatch(-1)
		return v, nil, true
	}

	// === Custom per-window jump highlight mode ===
	if v.jumpMode {
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			r := msg.Runes[0]
			if line, ok := v.jumpKeys[r]; ok {
				v.page.ScrollTo(line)
				v.jumpMode = false
				v.jumpKeys = nil
				v.jumpLineToKey = nil
				return v, nil, true
			}
		}
		// Cancel on any other key
		v.jumpMode = false
		v.jumpKeys = nil
		v.jumpLineToKey = nil
		return v, nil, true
	}

	// Enter custom jump highlight mode on ';'
	if msg.String() == ";" {
		v.enterJumpMode()
		return v, nil, true
	}

	return v, nil, false
}

// enterJumpMode populates one-key highlights for the current visible window.
func (v *USPTORawXML) enterJumpMode() {
	v.jumpMode = true
	v.jumpKeys = make(map[rune]int)
	v.jumpLineToKey = make(map[int]rune)

	start, end := v.page.Window()
	used := make(map[rune]bool)

	for i := start; i < end && len(v.jumpKeys) < 26; i++ {
		line := v.lines[i]
		trimmed := strings.TrimSpace(line)
		label := trimmed
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			label = "section " + trimmed
		}
		key := render.AssignKey(label, used, false)
		if key != 0 {
			v.jumpKeys[key] = i
			v.jumpLineToKey[i] = key
			used[key] = true
		}
	}
}
