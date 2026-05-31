// SourceBibsOverlay is a read-only, all-fields side-by-side view of every
// source's bibliographic snapshot of a record (USPTO vs Google vs …). Unlike
// SourceComparisonOverlay, which surfaces only the fields that differ and lets
// the user choose, this shows the complete picture — every comparable field,
// whether the sources agree or not — so "see both versions" is literal.
package overlay

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// bibField names one comparable row and extracts its value from a SourceBib.
type bibField struct {
	label string
	get   func(domain.SourceBib) string
}

var bibFields = []bibField{
	{"Title", func(b domain.SourceBib) string { return b.Title }},
	{"Assignee", func(b domain.SourceBib) string { return b.Assignee }},
	{"Inventors", func(b domain.SourceBib) string { return joinInventors(b.Inventors) }},
	{"Filing", func(b domain.SourceBib) string { return bibDate(b.ApplicationDate) }},
	{"Published", func(b domain.SourceBib) string { return bibDate(b.PublicationDate) }},
	{"Granted", func(b domain.SourceBib) string { return bibDate(b.GrantDate) }},
	{"Expires", func(b domain.SourceBib) string { return bibDate(b.ExpirationDate) }},
	{"Classes", func(b domain.SourceBib) string { return strings.Join(b.Classifications, ", ") }},
	{"Abstract", func(b domain.SourceBib) string { return b.Abstract }},
	{"Claim 1", func(b domain.SourceBib) string { return b.FirstClaim }},
}

func joinInventors(in []domain.Inventor) string {
	parts := make([]string, len(in))
	for i, n := range in {
		parts[i] = string(n)
	}
	return strings.Join(parts, "; ")
}

func bibDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(domain.DateLayout)
}

// SourceBibsOverlay renders the per-source bibliographic table.
type SourceBibsOverlay struct {
	theme  render.Theme
	patent domain.PatentNumber
	bibs   []domain.SourceBib
	cursor int // focused field row
}

func NewSourceBibsOverlay(theme render.Theme, patent domain.PatentNumber, bibs []domain.SourceBib) *SourceBibsOverlay {
	return &SourceBibsOverlay{theme: theme, patent: patent, bibs: bibs}
}

func (o *SourceBibsOverlay) Title() string {
	return fmt.Sprintf("Source Bibliography — %s", o.patent)
}

func (o *SourceBibsOverlay) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *SourceBibsOverlay) Handles() []command.ID { return nil }

func (o *SourceBibsOverlay) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, 90, 70, 60, 20)
}

func (o *SourceBibsOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.String() {
	case "q", "esc":
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "up", "k":
		if o.cursor > 0 {
			o.cursor--
		}
	case "down", "j":
		if o.cursor < len(bibFields)-1 {
			o.cursor++
		}
	}
	return o, nil, true
}

func (o *SourceBibsOverlay) View(maxW, _ int) string {
	var b strings.Builder
	b.WriteString(o.theme.Dim.Render(fmt.Sprintf("Source Bibliography — %s  (read-only; differing values highlighted)", o.patent)))
	b.WriteString("\n")

	if len(o.bibs) == 0 {
		b.WriteString("\n")
		b.WriteString(o.theme.Dim.Render("No per-source bibliographic data recorded for this record yet."))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("press q or esc to close"))
		return b.String()
	}

	const fieldW = 12
	// Split the remaining width evenly across the source columns.
	valW := (maxW - fieldW - 4) / len(o.bibs)
	if valW < 12 {
		valW = 12
	}

	// Header row of source names.
	header := render.Pad("", fieldW)
	for _, bib := range o.bibs {
		header += "  " + o.theme.HelpKey.Render(render.Pad(strings.ToUpper(string(bib.Source)), valW))
	}
	b.WriteString(header)
	b.WriteString("\n")

	for i, f := range bibFields {
		prefix := "  "
		labelStyle := o.theme.Row
		if i == o.cursor {
			prefix = "> "
			labelStyle = o.theme.Selected
		}

		// Gather each source's value to detect disagreement.
		vals := make([]string, len(o.bibs))
		distinct := map[string]struct{}{}
		for j, bib := range o.bibs {
			vals[j] = strings.TrimSpace(f.get(bib))
			distinct[vals[j]] = struct{}{}
		}
		differs := len(distinct) > 1

		line := prefix + labelStyle.Render(render.Pad(f.label, fieldW-len(prefix)))
		for _, v := range vals {
			cell := render.Truncate(v, valW)
			if differs {
				cell = o.theme.Warn.Render(cell)
			}
			line += "  " + render.Pad(cell, valW)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}

	b.WriteString("\n")
	b.WriteString(o.theme.Dim.Render("[↑/↓] move  •  [q/esc] close"))
	return b.String()
}
