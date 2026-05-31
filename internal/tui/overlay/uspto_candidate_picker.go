package overlay

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// USPTOCandidateSelectMsg is dispatched when the user confirms one or more
// candidates. Candidates is ordered by their position in the picker list.
type USPTOCandidateSelectMsg struct {
	Project        domain.ProjectID
	OriginalPatent domain.PatentNumber
	Candidates     []domain.USPTOCandidate
}

// USPTOCandidatePicker is the Choice Menu Popup overlay shown when multiple
// matching wrappers are found. Supports multi-select via space; Enter confirms
// all chosen rows (or the cursor row if none are chosen).
type USPTOCandidatePicker struct {
	theme          render.Theme
	project        domain.ProjectID
	candidates     []domain.USPTOCandidate
	originalPatent domain.PatentNumber
	page           render.Paginator
	vimCount       int
	chosen         map[int]bool
}

// NewUSPTOCandidatePicker builds the candidate picker.
func NewUSPTOCandidatePicker(theme render.Theme, project domain.ProjectID, candidates []domain.USPTOCandidate, originalPatent domain.PatentNumber) *USPTOCandidatePicker {
	page := render.NewPaginator(5)
	page.SetTotal(len(candidates))
	return &USPTOCandidatePicker{
		theme:          theme,
		project:        project,
		candidates:     candidates,
		originalPatent: originalPatent,
		page:           page,
		chosen:         make(map[int]bool),
	}
}

// Title implements Overlay.
func (p *USPTOCandidatePicker) Title() string {
	return fmt.Sprintf("Multiple USPTO wrappers found (%d)", len(p.candidates))
}

// Command implements Overlay.
func (p *USPTOCandidatePicker) Command(command.ID, int) (Overlay, tea.Cmd) { return p, nil }

// Handles implements Overlay.
func (p *USPTOCandidatePicker) Handles() []command.ID { return nil }

// OverlaySize implements DynamicSize so the picker takes 80% of the screen,
// preventing wraps on long titles.
func (p *USPTOCandidatePicker) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, 80, 80, 40, 10)
}

// HandleKey implements KeyHandler.
func (p *USPTOCandidatePicker) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if handleSubtableMotionKey(&p.page, msg, &p.vimCount) {
		return p, nil, true
	}
	key := msg.String()
	switch key {
	case "q", "esc":
		return p, func() tea.Msg { return CloseOverlayMsg{} }, true
	case " ", "space", "x":
		if len(p.candidates) > 0 {
			idx := p.page.Cursor()
			if p.chosen[idx] {
				delete(p.chosen, idx)
			} else {
				p.chosen[idx] = true
			}
		}
		return p, nil, true
	case "enter":
		if len(p.candidates) == 0 {
			return p, nil, true
		}
		picked := p.pickedCandidates()
		project := p.project
		orig := p.originalPatent
		return p, func() tea.Msg {
			return USPTOCandidateSelectMsg{Project: project, OriginalPatent: orig, Candidates: picked}
		}, true
	}
	return p, nil, true
}

// pickedCandidates returns chosen candidates in list order, or the cursor row
// alone when nothing has been explicitly chosen.
func (p *USPTOCandidatePicker) pickedCandidates() []domain.USPTOCandidate {
	if len(p.chosen) == 0 {
		return []domain.USPTOCandidate{p.candidates[p.page.Cursor()]}
	}
	idxs := make([]int, 0, len(p.chosen))
	for i := range p.chosen {
		if i >= 0 && i < len(p.candidates) {
			idxs = append(idxs, i)
		}
	}
	sort.Ints(idxs)
	out := make([]domain.USPTOCandidate, 0, len(idxs))
	for _, i := range idxs {
		out = append(out, p.candidates[i])
	}
	return out
}

// View implements Overlay.
func (p *USPTOCandidatePicker) View(maxW, maxH int) string {
	maxW = max(maxW-2, 10)
	pageSize := max(maxH-5, 1)

	// Columns: App Number, Filing Date, Patent #, First Inventor, Title
	const appW = 12
	const filingW = 11
	const patentW = 13
	const inventorW = 25
	prefixW := p.theme.TablePrefixWidth()
	fixed := prefixW + appW + filingW + patentW + inventorW + 4 // prefix + 4 gaps
	titleW := max(maxW-fixed, 15)

	cols := []render.TableColumn{
		{Key: "app_num", Label: "App Number", Width: appW},
		{Key: "filing", Label: "Filing Date", Width: filingW},
		{Key: "patent", Label: "Patent #", Width: patentW},
		{Key: "inventor", Label: "First Inventor", Width: inventorW},
		{Key: "title", Label: "Title", Width: titleW},
	}

	getCell := func(absIdx, _ int, colIdx int) string {
		if absIdx < 0 || absIdx >= len(p.candidates) {
			return ""
		}
		c := p.candidates[absIdx]
		switch cols[colIdx].Key {
		case "app_num":
			return c.ApplicationNumber
		case "filing":
			return firstNonEmpty(c.FilingDate, "N/A")
		case "patent":
			return firstNonEmpty(c.GrantNumber, c.PublicationNumber, "—")
		case "inventor":
			return firstNonEmpty(c.FirstInventorName, "Unknown")
		case "title":
			title := c.Title
			if title == "" {
				title = "(No Title)"
			}
			return title
		}
		return ""
	}

	var b strings.Builder
	b.WriteString(p.theme.Dim.Render(render.Truncate(
		"Select one or more application wrappers to load from USPTO:",
		maxW)))
	b.WriteString("\n\n")

	chosen := p.chosen
	b.WriteString(renderSubtable(subtableParams{
		Theme:       p.theme,
		Columns:     cols,
		Page:        &p.page,
		Total:       len(p.candidates),
		PageSize:    pageSize,
		FocusActive: true,
		MarkGlyph:   p.theme.Glyphs.RowChosen,
		VisualMode:  false,
		IsRowMarked: func(absIdx int) bool {
			return chosen[absIdx]
		},
	}, maxW, getCell))

	b.WriteByte('\n')
	help := fmt.Sprintf("  %s  %s  [space] Toggle  [enter] Add %s  [q/esc] Cancel",
		subtableStatus(p.page), p.chosenSummary(), p.enterTarget())
	b.WriteString(p.theme.Dim.Render(render.Pad(help, maxW)))
	return b.String()
}

// chosenSummary renders the running count of chosen rows.
func (p *USPTOCandidatePicker) chosenSummary() string {
	return fmt.Sprintf("[%d chosen]", len(p.chosen))
}

// enterTarget describes what Enter will dispatch given the current state.
func (p *USPTOCandidatePicker) enterTarget() string {
	if len(p.chosen) == 0 {
		return "cursor"
	}
	if len(p.chosen) == 1 {
		return "1 chosen"
	}
	return fmt.Sprintf("%d chosen", len(p.chosen))
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
