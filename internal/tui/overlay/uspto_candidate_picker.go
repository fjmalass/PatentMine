package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// USPTOCandidateSelectMsg is dispatched when the user selects a candidate.
type USPTOCandidateSelectMsg struct {
	Project        domain.ProjectID
	OriginalPatent domain.PatentNumber
	Candidate      domain.USPTOCandidate
}

// USPTOCandidatePicker is the Choice Menu Popup overlay shown when multiple
// matching wrappers are found.
type USPTOCandidatePicker struct {
	theme          render.Theme
	project        domain.ProjectID
	candidates     []domain.USPTOCandidate
	originalPatent domain.PatentNumber
	page           render.Paginator
	vimCount       int
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
	case "enter":
		if len(p.candidates) > 0 {
			c := p.candidates[p.page.Cursor()]
			project := p.project
			orig := p.originalPatent
			return p, func() tea.Msg {
				return USPTOCandidateSelectMsg{Project: project, OriginalPatent: orig, Candidate: c}
			}, true
		}
	}
	return p, nil, true
}

// View implements Overlay.
func (p *USPTOCandidatePicker) View(maxW, maxH int) string {
	maxW = max(maxW-2, 10)
	pageSize := max(maxH-5, 1)

	// Columns: App Number, Filing Date, First Inventor, Title
	const appW = 12
	const filingW = 11
	const inventorW = 25
	fixed := 2 + appW + filingW + inventorW + 3 // prefix + 3 gaps
	titleW := max(maxW-fixed, 15)

	cols := []render.TableColumn{
		{Key: "app_num", Label: "App Number", Width: appW},
		{Key: "filing", Label: "Filing Date", Width: filingW},
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
		"Select the correct application wrapper to load from USPTO:",
		maxW)))
	b.WriteString("\n\n")

	b.WriteString(renderSubtable(subtableParams{
		Theme:        p.theme,
		Columns:      cols,
		Page:         &p.page,
		Total:        len(p.candidates),
		PageSize:     pageSize,
		FocusActive:  true,
		PrefixCursor: "→ ",
		PrefixNormal: "  ",
		VisualMode:   false,
	}, maxW, getCell))

	b.WriteByte('\n')
	help := fmt.Sprintf("  %s  [enter] Choose  [q/esc] Cancel", subtableStatus(p.page))
	b.WriteString(p.theme.Dim.Render(render.Pad(help, maxW)))
	return b.String()
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
