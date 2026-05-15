package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

func (m *Model) reloadProjects() *Model {
	m.projects, _ = m.repo.ListProjects(m.ctx)
	m.unpaidCounts, _ = m.repo.CountUnpaidInvoicesByProject(m.ctx)
	return m
}

// importPatent fetches a patent bundle using the configured import source.
func (m *Model) saveLastProject() {
	_ = m.repo.SetSetting(m.ctx, SettingLastProjectID, m.ProjectID)
}

// logActivity writes one structured line to the monthly activity log.
// action uses dot notation: "patent.add", "citation.store", "note", etc.
// subject is the primary identifier (patent number, invoice id, etc.) or "".
// note is optional free-text annotation.
func (m *Model) logActivity(action, subject, note string) {
	if m.activityLog == nil {
		return
	}
	args := []any{"project", m.ProjectID, "action", action}
	if subject != "" {
		args = append(args, "subject", subject)
	}
	if note != "" {
		args = append(args, "note", note)
	}
	m.activityLog.Info("activity", args...)
}

func (m *Model) idsEntryForPatent(number string) *domain.IDSEntry {
	// Try cache first
	if m.detailCache.Number != "" {
		for i, e := range m.detailCache.IDSEntries {
			if e.PatentNumber == number {
				return &m.detailCache.IDSEntries[i]
			}
		}
	}

	// Fallback for safety (though detailFields should have populated cache)
	entries, err := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
	if err != nil {
		return nil
	}
	for i, e := range entries {
		if e.PatentNumber == number {
			return &entries[i]
		}
	}
	return nil
}

func (m *Model) openCurrentPatentIDSEdit() *Model {
	entry := m.idsEntryForPatent(m.current.Number)
	if entry == nil {
		if _, err := m.repo.AddIDSEntry(m.ctx, domain.IDSEntry{
			ProjectID:    m.ProjectID,
			PatentNumber: m.current.Number,
			Status:       domain.IDSStatusPending,
		}); err != nil {
			m.err = err.Error()
			return m
		}
		m.populateDetailCache()
	}
	return m.navigateTo(viewIDSEdit)
}

// markdownHeadingSummary extracts # and ## heading lines from body as a compact
// summary. Falls back to the first non-empty line when no headings are present.
func markdownHeadingSummary(body string) string {
	var headings []string
	for _, line := range strings.SplitAfter(body, "\n") {
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "## ") {
			headings = append(headings, strings.TrimPrefix(line, "## "))
		} else if strings.HasPrefix(line, "# ") {
			headings = append(headings, strings.TrimPrefix(line, "# "))
		}
	}
	if len(headings) > 0 {
		return strings.Join(headings, " · ")
	}
	for _, line := range strings.SplitAfter(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func (m *Model) recentNotesSnippet(number string) string {
	notes, err := m.repo.ListNotes(m.ctx, m.ProjectID, number)
	if err != nil || len(notes) == 0 {
		return ""
	}
	subtleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle))
	var b strings.Builder
	limit := noteDetailSnippetCount
	if len(notes) < limit {
		limit = len(notes)
	}
	for i := 0; i < limit; i++ {
		note := notes[i]
		b.WriteString(subtleStyle.Render(note.CreatedAt.Format("2006-01-02 15:04")))
		b.WriteString("  ")
		body := note.Body
		if idx := strings.Index(body, "\n"); idx >= 0 {
			body = body[:idx]
		}
		b.WriteString(body)
		b.WriteString("\n")
	}
	return b.String()
}

func nextIDSStatus(current domain.IDSStatus) domain.IDSStatus {
	switch current {
	case "":
		return domain.IDSStatusPending
	case domain.IDSStatusPending:
		return domain.IDSStatusSubmitted
	case domain.IDSStatusSubmitted:
		return domain.IDSStatusAccepted
	case domain.IDSStatusAccepted:
		return ""
	default:
		return domain.IDSStatusPending
	}
}

func idsStatusColor(status domain.IDSStatus) string {
	switch status {
	case domain.IDSStatusSubmitted:
		return ColorTheme
	case domain.IDSStatusAccepted:
		return ColorSuccess
	default:
		return ColorSubtle
	}
}

func (m *Model) cycleCurrentPatentIDSStatus() (tea.Model, tea.Cmd) {
	if m.current.Number == "" {
		return m, nil
	}

	entry := m.idsEntryForPatent(m.current.Number)
	if entry == nil {
		created, err := m.repo.AddIDSEntry(m.ctx, domain.IDSEntry{
			ProjectID:    m.ProjectID,
			PatentNumber: m.current.Number,
			Status:       domain.IDSStatusPending,
		})
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.populateDetailCache()
		m.logActivity(ActivityIDSAdd, m.current.Number, string(created.Status))
		m.message = "IDS status: " + string(created.Status)
		return m, nil
	}

	next := nextIDSStatus(entry.Status)
	if next == "" {
		if err := m.repo.DeleteIDSEntry(m.ctx, entry.ID); err != nil {
			m.err = err.Error()
			return m, nil
		}
		filtered := m.detailCache.IDSEntries[:0]
		for _, idsEntry := range m.detailCache.IDSEntries {
			if idsEntry.ID != entry.ID {
				filtered = append(filtered, idsEntry)
			}
		}
		m.detailCache.IDSEntries = filtered
		m.logActivity(ActivityIDSRemove, m.current.Number, "")
		m.message = "IDS entry removed"
		return m, nil
	}
	if err := m.repo.UpdateIDSEntryStatus(m.ctx, entry.ID, next); err != nil {
		m.err = err.Error()
		return m, nil
	}
	for i := range m.detailCache.IDSEntries {
		if m.detailCache.IDSEntries[i].ID == entry.ID {
			m.detailCache.IDSEntries[i].Status = next
			break
		}
	}
	m.logActivity(ActivityIDSStatus, m.current.Number, string(next))
	m.message = "IDS status: " + string(next)
	return m, nil
}

func (m *Model) idsEditCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) < 1 {
		m.err = "usage: :ids <note|kind|country|passages> <value>"
		return m, nil
	}
	entry := m.idsEntryForPatent(m.current.Number)
	if entry == nil {
		m.err = "no IDS entry for this patent; add one first"
		return m, nil
	}
	value := strings.Join(args[1:], " ")
	switch args[0] {
	case idsEditSubNote:
		entry.Notes = value
	case idsEditSubKind:
		entry.KindCode = value
	case idsEditSubCountry:
		entry.CountryCode = value
	case idsEditSubPassages:
		if err := domain.ValidateIDSPassages(value); err != nil {
			m.err = err.Error()
			return m, nil
		}
		entry.RelevantPassages = value
		entry.InFull = false
	case idsEditSubFull:
		entry.InFull = !entry.InFull
	default:
		m.err = "unknown ids field: " + args[0] + ". Use: note, kind, country, passages, full"
		return m, nil
	}
	if err := m.repo.UpdateIDSEntry(m.ctx, *entry); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.populateDetailCache()
	m.message = "IDS " + args[0] + " updated"
	return m, nil
}

// refreshSelectedCitationDetail re-fetches Google Patents for the single
// selected citation row and updates its title, inventors, and expiration.
func (m *Model) projectCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.setMode(viewSplash)
		m = m.reloadProjects()
		// Set selection to current project
		for i, p := range m.projects {
			if p.ID == m.ProjectID {
				m.projectSelected = i
				break
			}
		}
		return m, nil
	}

	sub := args[0]
	switch sub {
	case projectSubList:
		m.setMode(viewSplash)
		m = m.reloadProjects()
		for i, p := range m.projects {
			if p.ID == m.ProjectID {
				m.projectSelected = i
				break
			}
		}
		return m, nil
	case projectSubCreate:
		if len(args) < 2 {
			m.err = "usage: :project create <id> [name]"
			return m, nil
		}
		id := args[1]
		name := id
		if len(args) > 2 {
			name = strings.Join(args[2:], " ")
		}
		p := domain.Project{ID: id, Name: name}
		if err := m.repo.CreateProject(m.ctx, p); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "project created: " + id
		m = m.reloadProjects()
		m.ProjectID = id
		m.saveLastProject()
		for i, proj := range m.projects {
			if proj.ID == id {
				m.projectSelected = i
				break
			}
		}
		m.setMode(viewList)
		return m.refreshList()
	case projectSubAdd:
		if len(args) != 2 {
			m.err = "usage: :project add <project_id>"
			return m, nil
		}
		if m.current.Number == EmptyFilter {
			m.err = "open a patent first"
			return m, nil
		}
		targetID := args[1]
		if err := m.repo.AddPatentToProject(m.ctx, targetID, m.current.Number); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = fmt.Sprintf("added %s to project %s", m.current.Number, targetID)
		return m, nil
	case projectSubSwitch:
		if len(args) != 2 {
			m.err = "usage: :project switch <id>"
			return m, nil
		}
		id := args[1]
		p, err := m.repo.GetProject(m.ctx, id)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.ProjectID = p.ID
		m.saveLastProject()
		m.setMode(viewList)
		m.message = "switched to project: " + p.Name
		return m.refreshList()
	case projectSubStatus:
		if len(args) < 2 {
			m.err = "usage: :project status <status>"
			return m, nil
		}
		status := strings.Join(args[1:], " ")
		p, err := m.repo.GetProject(m.ctx, m.ProjectID)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		p.Status = status
		if err := m.repo.UpdateProject(m.ctx, p); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "project status updated"
		m = m.reloadProjects()
		return m, nil
	case projectSubSummaryStatus:
		if len(args) < 2 {
			m.err = "usage: :project summary-status <work-in-progress|provisional-filed|application-filed|published|granted>"
			return m, nil
		}
		summaryStatus := strings.Join(args[1:], " ")
		validSummaryStatuses := map[string]bool{
			domain.ProjectSummaryStatusWorkInProgress:   true,
			domain.ProjectSummaryStatusProvisionalFiled: true,
			domain.ProjectSummaryStatusApplicationFiled: true,
			domain.ProjectSummaryStatusPublished:        true,
			domain.ProjectSummaryStatusGranted:          true,
		}
		if !validSummaryStatuses[summaryStatus] {
			m.err = "invalid summary status: " + summaryStatus
			return m, nil
		}
		p, err := m.repo.GetProject(m.ctx, m.ProjectID)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		p.SummaryStatus = summaryStatus
		if err := m.repo.UpdateProject(m.ctx, p); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "project summary status updated: " + summaryStatus
		m = m.reloadProjects()
		return m, nil
	case projectSubEvent:
		if len(args) < 2 {
			m.err = "usage: :project event <type> [date YYYY-MM-DD] [due YYYY-MM-DD] [ref <ref>] [note <text>]"
			return m, nil
		}
		eventType := args[1]
		e := domain.ProjectEvent{ProjectID: m.ProjectID, EventType: eventType}
		rest := args[2:]
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case argDate:
				if i+1 < len(rest) {
					i++
					e.EventDate = rest[i]
				}
			case argDue:
				if i+1 < len(rest) {
					i++
					e.DueDate = rest[i]
				}
			case argRef:
				if i+1 < len(rest) {
					i++
					e.Reference = rest[i]
				}
			case argNote:
				if i+1 < len(rest) {
					e.Notes = strings.Join(rest[i+1:], " ")
					i = len(rest)
				}
			}
		}
		if _, err := m.repo.AddProjectEvent(m.ctx, e); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "event added: " + eventType
		return m, nil
	case projectSubEvents:
		if len(m.projects) > 0 {
			m.ProjectID = m.projects[m.projectSelected].ID
		}
		m.projectEventsSelected = 0
		m.setMode(viewProjectEvents)
		return m, nil
	case projectSubInvoices:
		if len(m.projects) > 0 {
			m.ProjectID = m.projects[m.projectSelected].ID
		}
		m.projectInvoicesSelected = 0
		m.setMode(viewProjectInvoices)
		return m, nil
	case projectSubInvoice:
		if len(args) < 2 {
			m.err = "usage: :project invoice <amount> [currency USD] [direction to-firm|from-firm] [date YYYY-MM-DD] [due YYYY-MM-DD] [firm <name>] [ref <number>] [note <text>]"
			return m, nil
		}
		inv := domain.ProjectInvoice{
			ProjectID: m.ProjectID,
			Amount:    args[1],
			Currency:  "USD",
			Direction: domain.InvoiceDirectionToFirm,
			Status:    domain.InvoiceStatusOutstanding,
		}
		rest := args[2:]
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case argCurrency:
				if i+1 < len(rest) {
					i++
					inv.Currency = rest[i]
				}
			case argDirection:
				if i+1 < len(rest) {
					i++
					inv.Direction = rest[i]
				}
			case argDate:
				if i+1 < len(rest) {
					i++
					inv.InvoiceDate = rest[i]
				}
			case argDue:
				if i+1 < len(rest) {
					i++
					inv.DueDate = rest[i]
				}
			case argFirm:
				if i+1 < len(rest) {
					i++
					inv.FirmName = rest[i]
				}
			case argRef:
				if i+1 < len(rest) {
					i++
					inv.InvoiceNumber = rest[i]
				}
			case argStatus:
				if i+1 < len(rest) {
					i++
					inv.Status = rest[i]
				}
			case argNote:
				if i+1 < len(rest) {
					inv.Notes = strings.Join(rest[i+1:], " ")
					i = len(rest)
				}
			}
		}
		if _, err := m.repo.AddProjectInvoice(m.ctx, inv); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "invoice added: " + inv.Amount + " " + inv.Currency
		m = m.reloadProjects()
		return m, nil
	case projectSubID:
		m.message = fmt.Sprintf("project: %s (%s)", m.projectNameForExport(), m.ProjectID)
		return m, nil
	case projectSubIDS:
		return m.idsCommand(args[1:])
	case projectSubExport:
		return m.exportCommand(args[1:])
	case projectSubSummary:
		if len(args) < 2 {
			m.err = "usage: :project summary <text>"
			return m, nil
		}
		summary := strings.Join(args[1:], " ")
		p, err := m.repo.GetProject(m.ctx, m.ProjectID)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		p.Summary = summary
		if err := m.repo.UpdateProject(m.ctx, p); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "project summary updated"
		m = m.reloadProjects()
		return m, nil
	case projectSubComment:
		if len(args) < 2 {
			m.err = "usage: :project comment <text>"
			return m, nil
		}
		comment := strings.Join(args[1:], " ")
		p, err := m.repo.GetProject(m.ctx, m.ProjectID)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		p.Comments = comment
		if err := m.repo.UpdateProject(m.ctx, p); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "project comment updated"
		m = m.reloadProjects()
		return m, nil
	default:
		m.err = "unknown project subcommand: " + sub
	}
	return m, nil
}

func (m *Model) deleteSelectedPatent() (tea.Model, tea.Cmd) {
	if m.patentSelected < 0 || m.patentSelected >= len(m.patents) {
		m.setMode(viewList)
		return m, nil
	}
	p := m.patents[m.patentSelected]
	if err := m.repo.DeletePatent(m.ctx, m.ProjectID, p.Number); err != nil {
		m.err = err.Error()
		m.setMode(viewList)
		return m, nil
	}

	// Delete PDF if it exists
	pdfPath := filepath.Join(defaultPDFDir, p.Number+".pdf")
	if _, err := os.Stat(pdfPath); err == nil {
		if err := os.Remove(pdfPath); err != nil {
			m.logger.Error("failed to delete pdf", "path", pdfPath, "error", err)
		} else {
			m.logger.Info("deleted pdf", "path", pdfPath)
		}
	}

	m.logActivity(ActivityPatentDelete, p.Number, "")
	m.message = fmt.Sprintf(m.text.T(TextMessageDeletedPatent), p.Number)
	m.setMode(viewList)
	return m.refreshList()
}

func (m *Model) idsCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.projectIDSSelected = 0
		m.setMode(viewProjectIDS)
		return m, nil
	}
	switch args[0] {
	case idsSubAdd:
		if len(args) < 2 {
			m.err = "usage: :project ids add <patent-number> [note <text>]"
			return m, nil
		}
		entry := domain.IDSEntry{
			ProjectID:    m.ProjectID,
			PatentNumber: strings.ToUpper(strings.TrimSpace(args[1])),
		}
		rest := args[2:]
		for i := 0; i < len(rest); i++ {
			if rest[i] == argNote && i+1 < len(rest) {
				entry.Notes = strings.Join(rest[i+1:], " ")
				break
			}
		}
		if _, err := m.repo.AddIDSEntry(m.ctx, entry); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "IDS entry added: " + entry.PatentNumber
	case idsSubMeta:
		// :project ids meta <field> <value>
		// fields: app, filed, inventor, art, examiner, docket
		if len(args) < 3 {
			m.err = "usage: :project ids meta <field> <value>"
			return m, nil
		}
		meta, err := m.repo.GetIDSMetadata(m.ctx, m.ProjectID)
		if err != nil {
			m.err = "error loading IDS metadata: " + err.Error()
			return m, nil
		}
		value := strings.Join(args[2:], " ")
		switch strings.ToLower(args[1]) {
		case "appnumber", "app":
			meta.AppNumber = value
		case "filingdate", "filed":
			meta.FilingDate = value
		case "inventor":
			meta.FirstInventor = value
		case "artunit", "art":
			meta.ArtUnit = value
		case "examiner":
			meta.ExaminerName = value
		case "docket":
			meta.AttorneyDocket = value
		default:
			m.err = "unknown IDS meta field: " + args[1] + ". Use: app, filed, inventor, art, examiner, docket"
			return m, nil
		}
		if err := m.repo.SaveIDSMetadata(m.ctx, meta); err != nil {
			m.err = "error saving IDS metadata: " + err.Error()
			return m, nil
		}
		m.message = "IDS metadata updated"
	default:
		m.err = "unknown ids subcommand: " + args[0]
	}
	return m, nil
}

func (m *Model) exportCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.err = "usage: :project export ids|review_state|state [<state>] [filename]"
		return m, nil
	}
	switch args[0] {
	case exportSubIDS:
		return m.idsExportCommand(args[1:])
	case exportSubReviewState:
		return m.reviewStateExportCommand(args[1:])
	case exportSubState:
		return m.stateExportCommand(args[1:])
	default:
		m.err = "unknown export type: " + args[0] + " — use ids, status, or state"
	}
	return m, nil
}

func (m *Model) projectNameForExport() string {
	for _, p := range m.projects {
		if p.ID == m.ProjectID {
			return p.Name
		}
	}
	return m.ProjectID
}

func (m *Model) reviewStateExportCommand(args []string) (tea.Model, tea.Cmd) {
	projectName := m.projectNameForExport()
	filename := fmt.Sprintf("%s_status_%s.md", strings.ReplaceAll(projectName, " ", "_"), time.Now().Format("2006-01-02"))
	if len(args) > 0 {
		filename = args[0]
	}

	var proj domain.Project
	for _, p := range m.projects {
		if p.ID == m.ProjectID {
			proj = p
			break
		}
	}

	events, _ := m.repo.ListProjectEvents(m.ctx, m.ProjectID)
	invoices, _ := m.repo.ListProjectInvoices(m.ctx, m.ProjectID)
	ids, _ := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
	allPatents, _ := m.repo.ListPatents(m.ctx, m.ProjectID, storage.ListPatentsOptions{ReviewStateFilter: reviewStateFilterNone})

	statusCounts := map[string]int{}
	for _, p := range allPatents {
		statusCounts[p.ReviewState]++
	}

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("# Project Status: %s\n\n", projectName))
	buf.WriteString(fmt.Sprintf("**ID:** %s  \n", proj.ID))
	buf.WriteString(fmt.Sprintf("**Status:** %s  \n", proj.Status))
	if proj.SummaryStatus != "" {
		label := proj.SummaryStatus
		if l, ok := SummaryStatusLabels[proj.SummaryStatus]; ok {
			label = l
		}
		buf.WriteString(fmt.Sprintf("**Stage:** %s  \n", label))
	}
	buf.WriteString(fmt.Sprintf("**Updated:** %s  \n", proj.UpdatedAt.Format("2006-01-02")))
	if proj.Summary != "" {
		buf.WriteString(fmt.Sprintf("\n> %s\n", proj.Summary))
	}
	if proj.Comments != "" {
		buf.WriteString(fmt.Sprintf("\n*%s*\n", proj.Comments))
	}

	buf.WriteString("\n## Patents\n\n")
	buf.WriteString(fmt.Sprintf("| Status | Count |\n|--------|-------|\n"))
	for _, s := range []string{domain.ReviewStateStored, domain.ReviewStateUnderReview, domain.ReviewStateIgnored, domain.ReviewStateCached} {
		if n := statusCounts[s]; n > 0 {
			buf.WriteString(fmt.Sprintf("| %s | %d |\n", s, n))
		}
	}
	buf.WriteString(fmt.Sprintf("\n**IDS entries:** %d\n", len(ids)))

	if len(events) > 0 {
		buf.WriteString("\n## Prosecution History\n\n")
		buf.WriteString("| Event | Date | Due | Reference | Notes |\n")
		buf.WriteString("|-------|------|-----|-----------|-------|\n")
		for _, e := range events {
			label := e.EventType
			if l, ok := EventTypeLabels[e.EventType]; ok {
				label = l
			}
			due, date, ref, notes := e.DueDate, e.EventDate, e.Reference, e.Notes
			if due == "" {
				due = "—"
			}
			if date == "" {
				date = "—"
			}
			if ref == "" {
				ref = "—"
			}
			if notes == "" {
				notes = "—"
			}
			buf.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n", label, date, due, ref, notes))
		}
	}

	if len(invoices) > 0 {
		buf.WriteString("\n## Invoices\n\n")
		buf.WriteString("| Amount | Currency | Direction | Status | Firm | Date | Notes |\n")
		buf.WriteString("|--------|----------|-----------|--------|------|------|-------|\n")
		for _, inv := range invoices {
			dir := inv.Direction
			if l, ok := InvoiceDirectionLabels[inv.Direction]; ok {
				dir = l
			}
			firm, date, notes := inv.FirmName, inv.InvoiceDate, inv.Notes
			if firm == "" {
				firm = "—"
			}
			if date == "" {
				date = "—"
			}
			if notes == "" {
				notes = "—"
			}
			buf.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |\n",
				inv.Amount, inv.Currency, dir, inv.Status, firm, date, notes))
		}
	}

	if err := os.WriteFile(filename, []byte(buf.String()), 0644); err != nil {
		m.err = "export failed: " + err.Error()
		return m, nil
	}
	m.message = fmt.Sprintf("status exported to %s", filename)
	return m, nil
}

var exportStateAliases = map[string]string{
	exportStateStored:      domain.ReviewStateStored,
	exportStateIgnored:     domain.ReviewStateIgnored,
	exportStateUnderReview: domain.ReviewStateUnderReview,
	"under_review":         domain.ReviewStateUnderReview,
	exportStateAll:         reviewStateFilterNone,
	exportStateNone:        reviewStateFilterNone,
}

func (m *Model) stateExportCommand(args []string) (tea.Model, tea.Cmd) {
	reviewStateFilter := m.listFilter.ReviewState
	filenameArgs := args

	if len(args) > 0 {
		if canonical, ok := exportStateAliases[strings.ToLower(args[0])]; ok {
			reviewStateFilter = canonical
			filenameArgs = args[1:]
		}
	}

	if reviewStateFilter == "" {
		reviewStateFilter = reviewStateFilterNone
	}

	projectName := m.projectNameForExport()
	stateLabel := reviewStateFilter
	filename := fmt.Sprintf("%s_state_%s_%s.md", strings.ReplaceAll(projectName, " ", "_"), stateLabel, time.Now().Format("2006-01-02"))
	if len(filenameArgs) > 0 {
		filename = filenameArgs[0]
	}

	patents, err := m.repo.ListPatents(m.ctx, m.ProjectID, storage.ListPatentsOptions{ReviewStateFilter: reviewStateFilter})
	if err != nil {
		m.err = "export failed: " + err.Error()
		return m, nil
	}

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("# Patent List: %s\n\n", projectName))
	buf.WriteString(fmt.Sprintf("**State filter:** %s  \n", stateLabel))
	buf.WriteString(fmt.Sprintf("**Date:** %s  \n", time.Now().Format("2006-01-02")))
	buf.WriteString(fmt.Sprintf("**Count:** %d  \n\n", len(patents)))

	if len(patents) == 0 {
		buf.WriteString("*No patents found.*\n")
	} else {
		buf.WriteString("| # | Number | Title | Assignee | Publication | Expiration | Status |\n")
		buf.WriteString("|---|--------|-------|----------|-------------|------------|--------|\n")
		for i, p := range patents {
			title := p.Title
			if len(title) > 50 {
				title = title[:47] + "..."
			}
			assignee := p.Assignee
			if assignee == "" {
				assignee = "—"
			}
			pub := p.PublicationDate
			if pub == "" {
				pub = "—"
			}
			exp := p.ExpirationDate
			if exp == "" {
				exp = "—"
			}
			buf.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %s | %s |\n",
				i+1, p.Number, title, assignee, pub, exp, p.ReviewState))
		}
	}

	if err := os.WriteFile(filename, []byte(buf.String()), 0644); err != nil {
		m.err = "export failed: " + err.Error()
		return m, nil
	}
	m.message = fmt.Sprintf("state exported to %s (%d patents)", filename, len(patents))
	return m, nil
}
