package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"patentmine/internal/changes"
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

	title := "Note" + sepBullet + string(p.Number)
	if inv := formatInventorsShort(p.Inventors); inv != "-" {
		title += sepBullet + inv
	}
	if year != "" {
		title += sepBullet + year
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
			year := note.CreatedAt.Format(dateFmtYear)
			name := markdownHeadingSummary(note.Body)
			if len(name) > 48 {
				name = name[:48] + "…"
			}
			header := year
			if name != "" {
				header += sepBullet + name
			}
			body.WriteString(subtleStyle.Render(header))
			body.WriteString("\n")
			body.WriteString(note.Body)
			body.WriteString("\n\n")
		}
	}
	return m.renderPopup("Notes"+sepBullet+string(m.current.Number), body.String())
}

func (m *Model) handleViewNoteEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyCtrlS:
		body := strings.TrimSpace(m.noteTA.Value())
		if body != "" {
			stamp := time.Now().Format(dateFmtDateTime)
			body = fmt.Sprintf("[%s]\n%s", stamp, body)
			if m.applyChange(changes.AddNote(m.ProjectID, m.current.Number, body)) {
				m.logActivity(ActivityNoteAdd, string(m.current.Number), "")
				m.message = "note saved"
			}
		}
		m.noteTA.Reset()
		m.noteTA.Blur()
		return m.goBack()
	case keyEsc:
		m.noteTA.Reset()
		m.noteTA.Blur()
		return m.goBack()
	default:
		var cmd tea.Cmd
		m.noteTA, cmd = m.noteTA.Update(msg)
		return m, cmd
	}
}
