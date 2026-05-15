package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) viewNoteEdit() string {
	var b strings.Builder
	p := m.current
	year := ""
	switch {
	case len(p.GrantDate) >= 4:
		year = p.GrantDate[:4]
	case len(p.PublicationDate) >= 4:
		year = p.PublicationDate[:4]
	}

	title := "Note · " + p.Number
	if inv := formatInventorsShort(p.Inventors); inv != "-" {
		title += " · " + inv
	}
	if year != "" {
		title += " · " + year
	}
	b.WriteString(m.renderPopupHeader(title))
	b.WriteString(m.noteTA.View())
	return b.String()
}

func (m *Model) viewNotes() string {
	notes, err := m.repo.ListNotes(m.ctx, m.ProjectID, m.current.Number)
	if err != nil {
		return m.renderPopup("Notes", err.Error()+"\n")
	}
	base := overlayBase()
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)
	subtleStyle := base.Foreground(lipgloss.Color(ColorSubtle))

	var body strings.Builder
	if len(notes) == 0 {
		body.WriteString(dimStyle.Render("No notes. Press N to add one."))
		body.WriteString("\n")
	} else {
		for _, note := range notes {
			year := note.CreatedAt.Format("2006")
			name := markdownHeadingSummary(note.Body)
			if len(name) > 48 {
				name = name[:48] + "…"
			}
			header := year
			if name != "" {
				header += " · " + name
			}
			body.WriteString(subtleStyle.Render(header))
			body.WriteString("\n")
			body.WriteString(note.Body)
			body.WriteString("\n\n")
		}
	}
	return m.renderPopup("Notes · "+m.current.Number, body.String())
}
