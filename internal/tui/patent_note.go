package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/text"
	"patentmine/internal/tui/overlay"
	"patentmine/internal/tui/pane"
)

func (a *App) openPatentNote(_ invocation) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	numbers := a.focusedSelections()
	if len(numbers) == 0 {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	if len(numbers) != 1 {
		a.setErr(text.StatusFilter, "patent notes require a single selected patent")
		return a, nil
	}
	o := overlay.NewPatentNoteEditor(a.client, a.theme, a.activeProject.ID, numbers[0])
	a.overlays = append(a.overlays, o)
	return a, o.Init()
}

func openPatentNoteMsg(number domain.PatentNumber) tea.Msg {
	return pane.PatentNoteOpenMsg{Number: number}
}
