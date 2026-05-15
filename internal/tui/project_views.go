package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"patentmine/internal/domain"
)

func (m *Model) viewSplash() string {
	logo := `
  ██████╗  █████╗ ████████╗███████╗███╗   ██╗████████╗    ███╗   ███╗██╗███╗   ██╗███████╗
  ██╔══██╗██╔══██╗╚══██╔══╝██╔════╝████╗  ██║╚══██╔══╝    ████╗ ████║██║████╗  ██║██╔════╝
  ██████╔╝███████║   ██║   █████╗  ██╔██╗ ██║   ██║       ██╔████╔██║██║██╔██╗ ██║█████╗
  ██╔═══╝ ██╔══██║   ██║   ██╔══╝  ██║╚██╗██║   ██║       ██║╚██╔╝██║██║██║╚██╗██║██╔══╝
  ██║     ██║  ██║   ██║   ███████╗██║ ╚████║   ██║       ██║ ╚═╝ ██║██║██║ ╚████║███████╗
  ╚═╝     ╚═╝  ╚═╝   ╚═╝   ╚══════╝╚═╝  ╚═══╝   ╚═╝       ╚═╝     ╚═╝╚═╝╚═╝  ╚═══╝╚══════╝
	`
	logoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.screenColor())).Bold(true)
	subStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Italic(true)

	var b strings.Builder
	b.WriteString("\n\n")
	b.WriteString(m.center(logoStyle.Render(logo)))
	b.WriteString("\n")
	b.WriteString(m.center(subStyle.Render("Local Patent Research & Intelligence")))
	b.WriteString("\n")
	b.WriteString(m.center(subStyle.Render("Version " + m.displayVersion())))
	separator := lipgloss.NewStyle().Width(m.width).Foreground(lipgloss.Color(ColorSubtle)).Render(strings.Repeat("─", m.width))

	b.WriteString("\n\n" + separator + "\n\n")

	if m.input.Focused() {
		promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.screenColor())).Bold(true)
		b.WriteString(m.center(promptStyle.Render("COMMAND: ") + m.input.View()))
		b.WriteString("\n\n")
	}

	b.WriteString(m.center(lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color(m.screenColor())).Render("SELECT PROJECT")))
	b.WriteString("\n\n")

	if len(m.projects) == 0 {
		b.WriteString(m.center("No projects found. Create one with 'n'"))
	} else {
		pHeader := fmt.Sprintf("   %-20s %-12s %-10s %-13s %-10s %s", "Name", "ID", "Status", "Summary", "Unpaid", "Updated")
		b.WriteString(m.center(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Underline(true).Render(pHeader)))
		b.WriteString("\n")

		for i, p := range m.projects {
			prefix := "  "
			if i == m.projectSelected {
				prefix = "→ "
			}

			updated := p.UpdatedAt.Format("2006-01-02")

			summaryLabel := p.SummaryStatus
			if label, ok := SummaryStatusLabels[p.SummaryStatus]; ok {
				summaryLabel = label
			}
			summaryColor := ColorSubtle
			if c, ok := SummaryStatusColors[p.SummaryStatus]; ok {
				summaryColor = c
			}

			unpaidCount := m.unpaidCounts[p.ID]
			unpaidLabel := "—"
			unpaidColor := ColorSubtle
			if unpaidCount > 0 {
				unpaidLabel = fmt.Sprintf("%d unpaid", unpaidCount)
				unpaidColor = ColorWarning
			}

			rowBase := fmt.Sprintf("%-20s %-12s %-10s ", p.Name, "["+p.ID+"]", p.Status)
			summaryPart := fmt.Sprintf("%-13s", summaryLabel)
			unpaidPart := fmt.Sprintf("%-10s", unpaidLabel)
			datePart := updated

			rowStyle := lipgloss.NewStyle()
			if i == m.projectSelected {
				rowStyle = rowStyle.Foreground(lipgloss.Color(ColorTheme)).Bold(true)
			} else {
				rowStyle = rowStyle.Foreground(lipgloss.Color(ColorSubtle))
			}
			summaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(summaryColor))
			unpaidStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(unpaidColor))
			if i == m.projectSelected {
				summaryStyle = summaryStyle.Bold(true)
				unpaidStyle = unpaidStyle.Bold(true)
			}

			b.WriteString(m.center(prefix + rowStyle.Render(rowBase) + summaryStyle.Render(summaryPart) + unpaidStyle.Render(unpaidPart) + rowStyle.Render(datePart)))
			b.WriteString("\n")
			if p.Summary != "" {
				summaryTextStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim)).Italic(true)
				b.WriteString(m.center("    " + summaryTextStyle.Render(p.Summary)))
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n\n" + separator + "\n")
	b.WriteString(m.center(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Render("[j/k/↓↑]: move · [enter]: select · [e]: events · [i]: invoices · [d]: IDS · [n]: new · [Q]: quit")))

	// Center vertically
	content := b.String()
	lines := strings.Count(content, "\n")
	padding := (m.height - lines) / 2
	if padding > 0 {
		content = strings.Repeat("\n", padding) + content
	}

	return content
}

func (m *Model) viewProjectEvents() string {
	events, err := m.repo.ListProjectEvents(m.ctx, m.ProjectID)
	if err != nil {
		return "error loading events: " + err.Error()
	}

	base := overlayBase()
	titleStyle := base.Bold(true).Underline(true)
	subtleStyle := base.Foreground(lipgloss.Color(ColorSubtle))
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Prosecution History · %s", m.ProjectID)))
	b.WriteString("\n\n")

	if len(events) == 0 {
		b.WriteString(dimStyle.Render("No events. Add with :project event <type> [date YYYY-MM-DD] [due YYYY-MM-DD] [note <text>]"))
		b.WriteString("\n\n")
		b.WriteString(subtleStyle.Render("Event types: provisional-filed · application-filed · publication · office-action-non-final\n"))
		b.WriteString(subtleStyle.Render("             office-action-final · response-filed · rce-filed · notice-of-allowance\n"))
		b.WriteString(subtleStyle.Render("             issue-fee-paid · patent-granted · maintenance-due-3y/7y/11y\n"))
		b.WriteString(subtleStyle.Render("             continuation-filed · divisional-filed · cip-filed · appeal-filed\n"))
		b.WriteString(subtleStyle.Render("             ptab-decision · ipr-filed · reexam-requested · extension-filed\n"))
		b.WriteString(subtleStyle.Render("             abandonment · revival-filed"))
	} else {
		headerStyle := base.Foreground(lipgloss.Color(ColorSubtle)).Underline(true)
		b.WriteString(headerStyle.Render(fmt.Sprintf("  %-26s %-12s %-12s %-14s %s", "Event", "Date", "Due", "Reference", "Notes")))
		b.WriteString("\n")
		for i, e := range events {
			label := e.EventType
			if l, ok := EventTypeLabels[e.EventType]; ok {
				label = l
			}
			color := ColorSubtle
			if c, ok := EventTypeColors[e.EventType]; ok {
				color = c
			}
			eventStyle := base.Foreground(lipgloss.Color(color))
			rowStyle := base.Foreground(lipgloss.Color(ColorSubtle))
			if i == m.projectEventsSelected {
				eventStyle = eventStyle.Bold(true).Reverse(true)
				rowStyle = rowStyle.Bold(true)
			}

			prefix := "  "
			if i == m.projectEventsSelected {
				prefix = "→ "
			}

			due := e.DueDate
			if due == "" {
				due = "—"
			}
			date := e.EventDate
			if date == "" {
				date = "—"
			}
			ref := e.Reference
			if ref == "" {
				ref = "—"
			}
			const evNotesW = 30
			notes := e.Notes
			if notes == "" {
				notes = "—"
			} else {
				notes = m.truncate(notes, evNotesW)
			}

			b.WriteString(prefix + eventStyle.Render(fmt.Sprintf("%-26s", label)) + rowStyle.Render(fmt.Sprintf("%-12s %-12s %-14s %s", date, due, ref, notes)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("j/k: move · D: delete · esc: back"))
	return b.String()
}

func (m *Model) viewProjectInvoices() string {
	invoices, err := m.repo.ListProjectInvoices(m.ctx, m.ProjectID)
	if err != nil {
		return "error loading invoices: " + err.Error()
	}

	base := overlayBase()
	titleStyle := base.Bold(true).Underline(true)
	subtleStyle := base.Foreground(lipgloss.Color(ColorSubtle))
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Invoices · %s", m.ProjectID)))
	b.WriteString("\n\n")

	if len(invoices) == 0 {
		b.WriteString(dimStyle.Render("No invoices. Add with :project invoice <amount> [currency USD] [direction to-firm|from-firm] [date YYYY-MM-DD] [due YYYY-MM-DD] [firm <name>] [ref <number>] [note <text>]"))
	} else {
		headerStyle := base.Foreground(lipgloss.Color(ColorSubtle)).Underline(true)
		b.WriteString(headerStyle.Render(fmt.Sprintf("  %-12s %-10s %-10s %-14s %-16s %-12s %s", "Amount", "Currency", "Direction", "Status", "Firm", "Date", "Notes")))
		b.WriteString("\n")
		for i, inv := range invoices {
			statusColor := ColorSubtle
			if c, ok := InvoiceStatusColors[inv.Status]; ok {
				statusColor = c
			}
			statusLabel := inv.Status
			if l, ok := InvoiceStatusLabels[inv.Status]; ok {
				statusLabel = l
			}
			dirLabel := inv.Direction
			if l, ok := InvoiceDirectionLabels[inv.Direction]; ok {
				dirLabel = l
			}

			amtStyle := base.Foreground(lipgloss.Color(statusColor))
			rowStyle := base.Foreground(lipgloss.Color(ColorSubtle))
			if i == m.projectInvoicesSelected {
				amtStyle = amtStyle.Bold(true).Reverse(true)
				rowStyle = rowStyle.Bold(true)
			}

			prefix := "  "
			if i == m.projectInvoicesSelected {
				prefix = "→ "
			}

			const invFirmW, invNotesW = 16, 20
			notes := inv.Notes
			if notes == "" {
				notes = "—"
			} else {
				notes = m.truncate(notes, invNotesW-3) + "..."
			}
			date := inv.InvoiceDate
			if date == "" {
				date = "—"
			}
			firm := inv.FirmName
			if firm == "" {
				firm = "—"
			} else {
				firm = m.truncate(firm, invFirmW-2) + ".."
			}

			b.WriteString(prefix + amtStyle.Render(fmt.Sprintf("%-12s", inv.Amount+" "+inv.Currency)) + rowStyle.Render(fmt.Sprintf("%-10s %-14s %-16s %-12s %s", dirLabel, statusLabel, firm, date, notes)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("j/k: move · p: mark paid · D: delete · esc: back"))
	return b.String()
}

func (m *Model) viewProjectIDS() string {
	ids, err := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
	if err != nil {
		return "error loading IDS: " + err.Error()
	}

	base := overlayBase()
	subtleStyle := base.Foreground(lipgloss.Color(ColorSubtle))
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)

	var b strings.Builder
	b.WriteString(m.renderPopupTitle(fmt.Sprintf("IDS · %s", m.ProjectID)))
	b.WriteString("\n\n")

	if len(ids) == 0 {
		b.WriteString(dimStyle.Render("No IDS entries. Add with :project ids add <patent-number> [note <text>]"))
		b.WriteString("\n\n")
		b.WriteString(subtleStyle.Render("IDS entries are prior art references to disclose to the patent office."))
	} else {
		sel := clamp(m.projectIDSSelected, 0, len(ids)-1)
		window := pageWindow(sel, len(ids), m.overlayPageSize())

		b.WriteString(subtleStyle.Render(pageStatus(m.text.T(TextValuePageStatus), window)))
		b.WriteString("\n\n")

		// fixed cols: prefix(2) + #(4) + patent(17) + status(12) + date(11) = 46
		// remaining split: notes ~20, passages ~rest
		rowWidth := max(60, m.overlayWidth()-4)
		noteW := 20
		passW := max(10, rowWidth-46-noteW-1)

		headerStyle := base.Foreground(lipgloss.Color(ColorSubtle)).Underline(true)
		b.WriteString(headerStyle.Render(fmt.Sprintf("  %-3s %-16s %-11s %-10s %-*s %s", "#", "Patent", "Status", "Added", noteW, "Notes", "Passages")))
		b.WriteString("\n")
		for i := window.Start; i < window.End; i++ {
			e := ids[i]
			rowStyle := base.Foreground(lipgloss.Color(ColorSubtle))
			numStyle := base.Foreground(lipgloss.Color(ColorTheme))
			statusColor := idsStatusColor(e.Status)
			if i == sel {
				rowStyle = rowStyle.Bold(true).Reverse(true)
				numStyle = numStyle.Bold(true)
			}
			prefix := "  "
			if i == sel {
				prefix = "→ "
			}
			notes := e.Notes
			if len(notes) > noteW {
				notes = notes[:noteW-3] + "..."
			}
			if notes == "" {
				notes = "—"
			}
			passages := IDSPassagesText(e)
			if len(passages) > passW {
				passages = passages[:passW-3] + "..."
			}
			if passages == "" {
				passages = "—"
			}
			statusStr := string(e.Status)
			if statusStr == "" {
				statusStr = string(domain.IDSStatusPending)
			}
			statusRendered := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(fmt.Sprintf("%-11s", statusStr))
			b.WriteString(prefix + numStyle.Render(fmt.Sprintf("%-3d", i+1)) + " " + rowStyle.Render(fmt.Sprintf("%-16s", e.PatentNumber)) + " " + statusRendered + " " + rowStyle.Render(fmt.Sprintf("%-10s %-*s %s", e.AddedAt.Format("2006-01-02"), noteW, notes, passages)))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m *Model) viewProjectInfo() string {
	var proj domain.Project
	for _, p := range m.projects {
		if p.ID == m.ProjectID {
			proj = p
			break
		}
	}

	events, _ := m.repo.ListProjectEvents(m.ctx, m.ProjectID)
	invoices, _ := m.repo.ListProjectInvoices(m.ctx, m.ProjectID)

	base := overlayBase()
	labelStyle := base.Foreground(lipgloss.Color(ColorSubtle))
	valueStyle := base.Foreground(lipgloss.Color(ColorTheme))
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)

	var body strings.Builder
	body.WriteString(labelStyle.Render(fmt.Sprintf("%-16s", "Name:")) + valueStyle.Render(proj.Name) + "\n")
	body.WriteString(labelStyle.Render(fmt.Sprintf("%-16s", "ID:")) + valueStyle.Render(proj.ID) + "\n")
	body.WriteString(labelStyle.Render(fmt.Sprintf("%-16s", "Status:")) + valueStyle.Render(proj.Status) + "\n")

	if proj.SummaryStatus != "" {
		label := proj.SummaryStatus
		if l, ok := SummaryStatusLabels[proj.SummaryStatus]; ok {
			label = l
		}
		color := ColorSubtle
		if c, ok := SummaryStatusColors[proj.SummaryStatus]; ok {
			color = c
		}
		body.WriteString(labelStyle.Render(fmt.Sprintf("%-16s", "App Status:")) +
			base.Foreground(lipgloss.Color(color)).Bold(true).Render(label) + "\n")
	}

	body.WriteString(labelStyle.Render(fmt.Sprintf("%-16s", "Updated:")) + valueStyle.Render(proj.UpdatedAt.Format("2006-01-02")) + "\n")

	if proj.Summary != "" {
		body.WriteString("\n" + dimStyle.Render(proj.Summary) + "\n")
	}
	if proj.Comments != "" {
		body.WriteString(dimStyle.Render(proj.Comments) + "\n")
	}

	// Invoice summary
	body.WriteString("\n" + base.Bold(true).Render("Invoices") + "\n\n")
	if len(invoices) == 0 {
		body.WriteString(dimStyle.Render("No invoices.") + "\n")
	} else {
		counts := map[string]int{}
		for _, inv := range invoices {
			counts[inv.Status]++
		}
		for _, status := range []string{
			domain.InvoiceStatusOutstanding,
			domain.InvoiceStatusOverdue,
			domain.InvoiceStatusDisputed,
			domain.InvoiceStatusPaid,
		} {
			if n := counts[status]; n > 0 {
				label := status
				if l, ok := InvoiceStatusLabels[status]; ok {
					label = l
				}
				color := ColorSubtle
				if c, ok := InvoiceStatusColors[status]; ok {
					color = c
				}
				body.WriteString(labelStyle.Render(fmt.Sprintf("  %-14s", label+":")) +
					base.Foreground(lipgloss.Color(color)).Render(fmt.Sprintf("%d", n)) + "\n")
			}
		}
	}

	// Recent events
	body.WriteString("\n" + base.Bold(true).Render("Recent Events") + "\n\n")
	if len(events) == 0 {
		body.WriteString(dimStyle.Render("No events.") + "\n")
	} else {
		limit := 5
		if len(events) < limit {
			limit = len(events)
		}
		for _, e := range events[:limit] {
			label := e.EventType
			if l, ok := EventTypeLabels[e.EventType]; ok {
				label = l
			}
			color := ColorSubtle
			if c, ok := EventTypeColors[e.EventType]; ok {
				color = c
			}
			date := e.EventDate
			if date == "" {
				date = "—"
			}
			body.WriteString(base.Foreground(lipgloss.Color(color)).Render(fmt.Sprintf("  %-26s", label)) +
				labelStyle.Render(date) + "\n")
		}
		if len(events) > 5 {
			body.WriteString(dimStyle.Render(fmt.Sprintf("  … and %d more", len(events)-5)) + "\n")
		}
	}

	body.WriteString("\n")
	if m.input.Focused() {
		body.WriteString(base.Foreground(lipgloss.Color(ColorTheme)).Bold(true).Render("COMMAND: ") + m.input.View() + "\n")
	}
	return m.renderPopup("Project Info", body.String())
}

func (m *Model) viewIDSEdit() string {
	entry := m.idsEntryForPatent(m.current.Number)

	base := overlayBase()
	labelStyle := base.Foreground(lipgloss.Color(ColorSubtle))
	valueStyle := base.Foreground(lipgloss.Color(ColorWhite))
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)
	hintStyle := base.Foreground(lipgloss.Color(ColorSubtle))

	var body strings.Builder
	if entry == nil {
		body.WriteString(dimStyle.Render("No IDS entry.") + "\n")
		return m.renderPopup("IDS Entry · "+m.current.Number, body.String())
	}

	row := func(label string, value string) {
		body.WriteString(labelStyle.Render(fmt.Sprintf("%-18s", label+":")) + " " + value + "\n")
	}

	statusColor := idsStatusColor(entry.Status)
	statusStr := string(entry.Status)
	if statusStr == "" {
		statusStr = string(domain.IDSStatusPending)
	}
	row("Status", base.Foreground(lipgloss.Color(statusColor)).Bold(true).Render(statusStr))

	kindVal := entry.KindCode
	if kindVal == "" {
		kindVal = dimStyle.Render("—")
	} else {
		kindVal = valueStyle.Render(kindVal)
	}
	row("Kind Code", kindVal)

	ccVal := entry.CountryCode
	if ccVal == "" {
		ccVal = dimStyle.Render("—")
	} else {
		ccVal = valueStyle.Render(ccVal)
	}
	row("Country Code", ccVal)

	notesVal := entry.Notes
	if notesVal == "" {
		notesVal = dimStyle.Render("—")
	} else {
		notesVal = valueStyle.Render(notesVal)
	}
	row("Notes", notesVal)

	inFullVal := dimStyle.Render("—")
	if entry.InFull {
		inFullVal = base.Foreground(lipgloss.Color(ColorLime)).Bold(true).Render(IDSIconInFull + " Yes")
	}
	row("In Full", inFullVal)

	passagesVal := IDSPassagesText(*entry)
	if passagesVal == "" {
		passagesVal = dimStyle.Render("—")
	} else {
		passagesVal = valueStyle.Render(passagesVal)
	}
	row("Relevant Passages", passagesVal)

	if m.input.Focused() {
		body.WriteString("\n")
		body.WriteString(base.Foreground(lipgloss.Color(ColorTheme)).Bold(true).Render("COMMAND: ") + base.Render(m.input.View()) + "\n")
		if strings.HasPrefix(m.input.Value(), ":ids passages") {
			body.WriteString(hintStyle.Render("fmt: "+domain.IDSPassagesFormatGuide) + "\n")
		}
	}
	return m.renderPopup("IDS Entry · "+m.current.Number, body.String())
}

func (m *Model) center(s string) string {
	if m.width <= 0 {
		return s
	}
	if m.isSplashContext() {
		return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, s)
	}
	return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, s,
		lipgloss.WithWhitespaceBackground(lipgloss.Color(ColorSurface)))
}

func (m *Model) handleViewSplashKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyVimDown, keyArrowDown:
		m.projectSelected = clamp(m.projectSelected+m.consumeCount(1), 0, len(m.projects)-1)
	case keyVimUp, keyArrowUp:
		m.projectSelected = max(0, m.projectSelected-m.consumeCount(1))
	case keyEnter:
		if len(m.projects) > 0 {
			m.ProjectID = m.projects[m.projectSelected].ID
			m.saveLastProject()
			m.setMode(viewList)
			return m.refreshList()
		}
	case keyEvents:
		if len(m.projects) > 0 {
			m.ProjectID = m.projects[m.projectSelected].ID
		}
		m.projectEventsSelected = 0
		m.setMode(viewProjectEvents)
		return m, nil
	case keyInvoices:
		if len(m.projects) > 0 {
			m.ProjectID = m.projects[m.projectSelected].ID
		}
		m.projectInvoicesSelected = 0
		m.setMode(viewProjectInvoices)
		return m, nil
	case keyIDS:
		if len(m.projects) > 0 {
			m.ProjectID = m.projects[m.projectSelected].ID
		}
		m.projectIDSSelected = 0
		m.setMode(viewProjectIDS)
		return m, nil
	case keyNew:
		m.input.Focus()
		m.input.SetValue(":project create ")
		return m, nil
	case keyQuit:
		return m, tea.Quit
	case keyEsc:
		return m.goBack()
	default:
		m.tryAccumulateCount(msg.String())
	}
	return m, nil
}

func (m *Model) handleViewProjectInfoKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEsc, keyBack, keyProjectInfo:
		return m.goBack()
	case keyEditAppStatus:
		m.input.Focus()
		m.input.SetValue(":project summary-status ")
		return m, nil
	case keyEditSummary:
		m.input.Focus()
		m.input.SetValue(":project summary ")
		return m, nil
	case keyEditComment:
		m.input.Focus()
		m.input.SetValue(":project comment ")
		return m, nil
	case keyEditProjectStatus:
		m.input.Focus()
		m.input.SetValue(":project status ")
		return m, nil
	}
	return m, nil
}

func (m *Model) handleViewIDSEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEsc, keyBack:
		return m.goBack()
	case "s":
		return m.cycleCurrentPatentIDSStatus()
	case "n":
		entry := m.idsEntryForPatent(m.current.Number)
		if entry != nil {
			m.input.Focus()
			m.input.SetValue(":ids note " + entry.Notes)
			m.input.CursorEnd()
		}
	case "k":
		entry := m.idsEntryForPatent(m.current.Number)
		if entry != nil {
			m.input.Focus()
			m.input.SetValue(":ids kind " + entry.KindCode)
			m.input.CursorEnd()
		}
	case "c":
		entry := m.idsEntryForPatent(m.current.Number)
		if entry != nil {
			m.input.Focus()
			m.input.SetValue(":ids country " + entry.CountryCode)
			m.input.CursorEnd()
		}
	case "p":
		entry := m.idsEntryForPatent(m.current.Number)
		if entry != nil {
			m.input.Focus()
			m.input.SetValue(":ids passages " + entry.RelevantPassages)
			m.input.CursorEnd()
		}
	case "f":
		return m.idsEditCommand([]string{idsEditSubFull})
	case keyDelete:
		entry := m.idsEntryForPatent(m.current.Number)
		if entry != nil {
			if err := m.repo.DeleteIDSEntry(m.ctx, entry.ID); err != nil {
				m.err = err.Error()
			} else {
				m.populateDetailCache()
				m.message = "IDS entry removed"
				return m.goBack()
			}
		}
	}
	return m, nil
}

func (m *Model) handleViewProjectEventsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyVimDown, keyArrowDown:
		events, _ := m.repo.ListProjectEvents(m.ctx, m.ProjectID)
		m.projectEventsSelected = clamp(m.projectEventsSelected+m.consumeCount(1), 0, len(events)-1)
	case keyVimUp, keyArrowUp:
		m.projectEventsSelected = max(0, m.projectEventsSelected-m.consumeCount(1))
	case keyDelete:
		events, _ := m.repo.ListProjectEvents(m.ctx, m.ProjectID)
		if m.projectEventsSelected >= 0 && m.projectEventsSelected < len(events) {
			_ = m.repo.DeleteProjectEvent(m.ctx, events[m.projectEventsSelected].ID)
			m.projectEventsSelected = 0
			m.message = "event deleted"
		}
	case keyEsc, keyBack:
		return m.goBack()
	default:
		m.tryAccumulateCount(msg.String())
	}
	return m, nil
}

func (m *Model) handleViewProjectInvoicesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyVimDown, keyArrowDown:
		invoices, _ := m.repo.ListProjectInvoices(m.ctx, m.ProjectID)
		m.projectInvoicesSelected = clamp(m.projectInvoicesSelected+m.consumeCount(1), 0, len(invoices)-1)
	case keyVimUp, keyArrowUp:
		m.projectInvoicesSelected = max(0, m.projectInvoicesSelected-m.consumeCount(1))
	case keyDelete:
		invoices, _ := m.repo.ListProjectInvoices(m.ctx, m.ProjectID)
		if m.projectInvoicesSelected >= 0 && m.projectInvoicesSelected < len(invoices) {
			_ = m.repo.DeleteProjectInvoice(m.ctx, invoices[m.projectInvoicesSelected].ID)
			m.projectInvoicesSelected = 0
			m.message = "invoice deleted"
			m = m.reloadProjects()
		}
	case keyMarkPaid:
		invoices, _ := m.repo.ListProjectInvoices(m.ctx, m.ProjectID)
		if m.projectInvoicesSelected >= 0 && m.projectInvoicesSelected < len(invoices) {
			inv := invoices[m.projectInvoicesSelected]
			inv.Status = domain.InvoiceStatusPaid
			_ = m.repo.UpdateProjectInvoice(m.ctx, inv)
			m.message = "marked as paid"
			m = m.reloadProjects()
		}
	case keyEsc, keyBack:
		return m.goBack()
	default:
		m.tryAccumulateCount(msg.String())
	}
	return m, nil
}

func (m *Model) handleViewProjectIDSKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyVimDown, keyArrowDown:
		ids, _ := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
		m.projectIDSSelected = clamp(m.projectIDSSelected+m.consumeCount(1), 0, len(ids)-1)
	case keyVimUp, keyArrowUp:
		m.projectIDSSelected = max(0, m.projectIDSSelected-m.consumeCount(1))
	case "s":
		ids, _ := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
		if m.projectIDSSelected >= 0 && m.projectIDSSelected < len(ids) {
			entry := ids[m.projectIDSSelected]
			next := nextIDSStatus(entry.Status)
			if next == "" {
				if err := m.repo.DeleteIDSEntry(m.ctx, entry.ID); err != nil {
					m.err = err.Error()
				} else {
					m.projectIDSSelected = clamp(m.projectIDSSelected, 0, max(0, len(ids)-2))
					m.logActivity(ActivityIDSRemove, entry.PatentNumber, "")
					m.message = "IDS entry removed"
				}
			} else if err := m.repo.UpdateIDSEntryStatus(m.ctx, entry.ID, next); err != nil {
				m.err = err.Error()
			} else {
				m.logActivity(ActivityIDSStatus, entry.PatentNumber, string(next))
				m.message = "IDS status: " + string(next)
			}
		}
	case keyDelete:
		ids, _ := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
		if m.projectIDSSelected >= 0 && m.projectIDSSelected < len(ids) {
			_ = m.repo.DeleteIDSEntry(m.ctx, ids[m.projectIDSSelected].ID)
			m.projectIDSSelected = 0
			m.message = "IDS entry removed"
		}
	case keyEsc, keyBack:
		return m.goBack()
	default:
		m.tryAccumulateCount(msg.String())
	}
	return m, nil
}
