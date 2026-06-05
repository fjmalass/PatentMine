package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/text"
	"patentmine/internal/tui/overlay"
	"patentmine/internal/tui/pane"
)

// --- App-level command handlers ---------------------------------------------

func (a *App) cmdQuit(invocation) (tea.Model, tea.Cmd) { return a, tea.Quit }

func (a *App) cmdHelp(invocation) (tea.Model, tea.Cmd) {
	if len(a.overlays) == 0 {
		a.overlays = append(a.overlays, overlay.NewHelp(a.registry, a.keymaps, a.theme, a.text))
	}
	return a, nil
}

func (a *App) cmdOpenSearch(invocation) (tea.Model, tea.Cmd) {
	return a.openPrompt(overlay.PromptPalette)
}

func (a *App) cmdOpenCommand(invocation) (tea.Model, tea.Cmd) {
	return a.openPrompt(overlay.PromptDirect)
}

type sourceModeChangedMsg struct {
	mode string
	err  error
}

func (a *App) cmdSourceMode(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) > 1 {
		return a.usageError(command.SourceMode)
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	mode := ""
	if len(inv.args) == 1 {
		mode = strings.TrimSpace(inv.args[0])
	}
	return a, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()
		var res proto.SourceModeResult
		method := proto.MethodSourceModeGet
		var params any
		if mode != "" {
			method = proto.MethodSourceModeSet
			params = proto.SourceModeParams{Mode: mode}
		}
		if err := a.client.Call(ctx, method, params, &res); err != nil {
			return sourceModeChangedMsg{err: err}
		}
		return sourceModeChangedMsg{mode: res.Mode}
	}
}

// cmdSourceCompare opens the source comparison overlay for the current patent
// (especially useful after USPTO enrichment when Google provided the primary data
// in compare mode). Default choice is USPTO. Loads real diffs via RPC (Option A).
func (a *App) cmdSourceCompare(inv invocation) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	number, ok := a.focusedPane().Selection()
	if !ok || number.IsZero() {
		a.setErr(text.StatusGeneric, "no patent selected (focus a detail pane first)")
		return a, nil
	}
	return a.openSourceCompare(number)
}

// openSourceCompare loads diffs for number and opens the comparison overlay.
// Shared between the typed :source-compare command and the "press C" affordance
// in the Loading overlay's done view.
func (a *App) openSourceCompare(number domain.PatentNumber) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var listRes proto.SourceDiffsListResult
	listErr := a.client.Call(ctx, proto.MethodSourceDiffsList, proto.SourceDiffsListParams{Number: number}, &listRes)

	a.metrics.IncCounter("tui.source.compare.initiated_total", 1)
	a.recordActivity(observability.Record{
		Action:   observability.ActionSourceCompareOpen,
		Entity:   observability.EntityPatent,
		EntityID: number.String(),
		Status:   map[bool]string{true: observability.StatusError, false: observability.StatusOpened}[listErr != nil],
		Attributes: map[string]any{
			observability.AttrDiffCount: len(listRes.Diffs),
		},
	})

	if listErr != nil {
		a.metrics.IncCounter("tui.source.compare.error_total", 1)
		a.setErr(text.StatusGeneric, "load diffs failed: "+listErr.Error())
		return a, nil
	}

	a.metrics.IncCounter("tui.source.diffs.loaded_total", int64(len(listRes.Diffs)))

	o := overlay.NewSourceComparisonOverlay(a.theme, number, listRes.Diffs)
	a.overlays = append(a.overlays, o)

	a.setStatus(text.StatusGeneric, fmt.Sprintf("Source comparison — %s (default = USPTO)", number))

	a.log().Info("source comparison opened",
		slog.String("patent", number.String()),
		slog.Int("diff_count", len(listRes.Diffs)))

	return a, nil
}

// cmdSourceBibs opens the read-only all-fields side-by-side source view for the
// focused patent.
func (a *App) cmdSourceBibs(inv invocation) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	number, ok := a.focusedPane().Selection()
	if !ok || number.IsZero() {
		a.setErr(text.StatusGeneric, "no patent selected (focus a detail pane first)")
		return a, nil
	}
	return a.openSourceBibs(number)
}

// openSourceBibs loads every source's bibliographic snapshot for number and
// opens the all-fields comparison overlay.
func (a *App) openSourceBibs(number domain.PatentNumber) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var res proto.SourceBibsListResult
	if err := a.client.Call(ctx, proto.MethodSourceBibsList, proto.SourceBibsListParams{Number: number}, &res); err != nil {
		a.setErr(text.StatusGeneric, "load source bibs failed: "+err.Error())
		return a, nil
	}

	o := overlay.NewSourceBibsOverlay(a.theme, number, res.Bibs)
	a.overlays = append(a.overlays, o)
	a.setStatus(text.StatusGeneric, fmt.Sprintf("Source bibliography — %s (%d sources)", number, len(res.Bibs)))
	a.log().Info("source bibliography opened",
		slog.String("patent", number.String()),
		slog.Int("source_count", len(res.Bibs)))
	return a, nil
}

func (a *App) cmdOpenMetrics(invocation) (tea.Model, tea.Cmd) {
	o, cmd := overlay.NewMetricsOverlay(a.client, a.theme, a.text, a.metrics)
	a.overlays = append(a.overlays, o)
	return a, cmd
}

func (a *App) cmdOpenActivity(invocation) (tea.Model, tea.Cmd) {
	var records []observability.Record
	if a.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()
		var res proto.ActivityRawResult
		if err := a.client.Call(ctx, proto.MethodActivityRaw, proto.ActivityRawParams{Limit: 200}, &res); err == nil {
			records = res.Records
		}
	}
	if len(records) == 0 && a.activityDir == "" {
		a.setErr(text.StatusUsage, "activity logging is not configured")
		return a, nil
	}
	if len(records) == 0 {
		var err error
		records, err = observability.ReadActivityRecords(a.activityDir, observability.ActivityQuery{Limit: 200})
		if err != nil {
			a.setErr(text.StatusUsage, err.Error())
			return a, nil
		}
	}
	a.overlays = append(a.overlays, overlay.NewActivity(a.theme, records))
	return a, nil
}

func (a *App) cmdOpenPatentNote(inv invocation) (tea.Model, tea.Cmd) {
	return a.openPatentNote(inv)
}

// cmdJumpMode toggles jump mode on the focused pane. When jump mode is active
// the pane renders inline shortcut labels (e.g. "[a] Assignee") and navigation
// keys (j/k) cycle between labelled sections instead of scrolling line-by-line.
// It is a no-op when an overlay is already open.
func (a *App) cmdJumpMode(invocation) (tea.Model, tea.Cmd) {
	if len(a.overlays) > 0 {
		return a, nil
	}
	if toggler, ok := a.focusedPane().(interface {
		SetJumpActive(bool)
		JumpActive() bool
	}); ok {
		toggler.SetJumpActive(!toggler.JumpActive())
	}
	return a, nil
}

func (a *App) cmdCloseOverlay(invocation) (tea.Model, tea.Cmd) {
	a.popOverlay()
	return a, nil
}

func (a *App) cmdBack(invocation) (tea.Model, tea.Cmd) {
	if len(a.overlays) > 0 {
		a.popOverlay()
	} else if len(a.panes) > 1 {
		a.panes = a.panes[:len(a.panes)-1]
		a.syncHistoryCursor()
	}
	return a, nil
}

func (a *App) cmdOpenDetail(invocation) (tea.Model, tea.Cmd) { return a.openDetail() }

func (a *App) cmdOpenFullText(invocation) (tea.Model, tea.Cmd) { return a.openFullText() }

func (a *App) cmdOpenBrowser(inv invocation) (tea.Model, tea.Cmd) {
	return a.cmdOpenBrowserTarget(inv, browseTargetDefault)
}

func (a *App) cmdOpenBrowserUSPTO(inv invocation) (tea.Model, tea.Cmd) {
	return a.cmdOpenBrowserTarget(inv, browseTargetUSPTOSmart)
}

func (a *App) cmdOpenBrowserUSPTOGrant(inv invocation) (tea.Model, tea.Cmd) {
	return a.cmdOpenBrowserTarget(inv, browseTargetUSPTOGrant)
}

func (a *App) cmdOpenBrowserUSPTOPGPub(inv invocation) (tea.Model, tea.Cmd) {
	return a.cmdOpenBrowserTarget(inv, browseTargetUSPTOPGPub)
}

func (a *App) cmdOpenBrowserGoogle(inv invocation) (tea.Model, tea.Cmd) {
	return a.cmdOpenBrowserTarget(inv, browseTargetGoogle)
}

func (a *App) cmdOpenBrowserTarget(inv invocation, target browseTarget) (tea.Model, tea.Cmd) {
	var numbers []domain.PatentNumber
	if len(inv.args) > 0 {
		numbers = make([]domain.PatentNumber, 0, len(inv.args))
		for _, arg := range inv.args {
			number, err := domain.ParsePatentNumber(arg)
			if err != nil {
				a.setErr(text.StatusInvalidPatentNumber, err.Error())
				return a, nil
			}
			numbers = append(numbers, number)
		}
	} else {
		numbers = a.focusedSelections()
		if len(numbers) == 0 {
			a.setErr(text.StatusNoPatentSelected)
			return a, nil
		}
	}
	return a, a.openPatentsInBrowser(numbers, target)
}
func (a *App) cmdOpenCitations(invocation) (tea.Model, tea.Cmd) {
	return a.openCitations(domain.RelationCites)
}
func (a *App) cmdOpenCitedBy(invocation) (tea.Model, tea.Cmd) {
	return a.openCitations(domain.RelationCitedBy)
}
func (a *App) cmdOpenParents(invocation) (tea.Model, tea.Cmd) {
	return a.openCitations(domain.RelationParent)
}
func (a *App) cmdOpenChildren(invocation) (tea.Model, tea.Cmd) {
	return a.openCitations(domain.RelationChild)
}
func (a *App) cmdOpenFamilyGraph(inv invocation) (tea.Model, tea.Cmd) {
	depth := 0
	args := inv.args
	if len(args) > 0 {
		if parsed, err := strconv.Atoi(args[0]); err == nil {
			depth = parsed
			args = args[1:]
		}
	}
	return a.openFamilyGraph(depth, args)
}

func (a *App) cmdFamilyExportMermaid(inv invocation) (tea.Model, tea.Cmd) {
	// If the focused pane is the FamilyGraph pane, delegate to it so it uses the current view filters
	if g, ok := a.focusedPane().(*pane.FamilyGraph); ok {
		updated, cmd := g.Command(command.FamilyExportMermaid, pane.Invocation{Repeat: inv.repeat, Args: inv.args})
		a.panes[len(a.panes)-1] = updated
		return a, cmd
	}

	// Otherwise, resolve the active selection
	numbers := a.focusedSelections()
	if len(numbers) == 0 {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	number := numbers[0]

	// Output path argument (if any)
	var path string
	if len(inv.args) > 0 {
		path = inv.args[0]
	}

	projectID := domain.ProjectID("")
	if a.activeProject != nil {
		projectID = a.activeProject.ID
	}

	return a, pane.ExportFamilyMermaid(a.client, number, projectID, path)
}

func (a *App) cmdOpenIDS(invocation) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	numbers := a.focusedSelections()
	switch len(numbers) {
	case 0:
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	case 1:
		return a.openIDS()
	default:
		a.overlays = append(a.overlays, overlay.NewIDSStatusPicker(a.theme, numbers))
		return a, nil
	}
}

func (a *App) cmdOpenInventors(invocation) (tea.Model, tea.Cmd) {
	detail, ok := a.focusedPane().(*pane.Detail)
	if !ok {
		return a, nil
	}

	// 1. If cursor is on a relation count line, open the corresponding list pane
	if kind, ok := detail.ResolveCursorRelation(); ok {
		return a.openCitations(kind)
	}

	if detail.IsCursorOnAssignee() {
		p := detail.Patent()
		if num, ok := detail.Selection(); ok {
			if p.Number.Serial == "" {
				p.Number = num
			}
			if p.DisplayNumber.Serial == "" {
				p.DisplayNumber = num
			}
		}
		var project domain.ProjectID
		if a.activeProject != nil {
			project = a.activeProject.ID
		}
		o, cmd := overlay.NewAllAssigneesHistoryOverlay(a.client, a.theme, a.text, p, project, true)
		a.overlays = append(a.overlays, o)
		return a, cmd
	}

	if detail.IsCursorOnClassifications() {
		p := detail.Patent()
		if num, ok := detail.Selection(); ok {
			if p.Number.Serial == "" {
				p.Number = num
			}
			if p.DisplayNumber.Serial == "" {
				p.DisplayNumber = num
			}
		}
		return a.openClassificationStats(p)
	}

	// 2. Cursor on a PGPub/Grant XML URL row: download (or load cached) + open.
	if kind, ok := detail.ResolveCursorUSPTOXML(); ok {
		return a.fetchAndOpenUSPTOXML([]domain.PatentNumber{detail.PatentNumber()}, kind)
	}

	// 3. Only activate on Enter if the cursor is actually on the Inventors field
	if !detail.IsCursorOnInventors() {
		return a, nil
	}

	p := detail.Patent()
	if num, ok := detail.Selection(); ok {
		if p.Number.Serial == "" {
			p.Number = num
		}
		if p.DisplayNumber.Serial == "" {
			p.DisplayNumber = num
		}
	}

	return a.openInventors(p, false)
}

type openAssigneesPatentLoadedMsg struct {
	patent domain.Patent
	err    error
}

func (a *App) cmdOpenAssignees(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) != 0 {
		return a.usageError(command.OpenAssignees)
	}
	if detail, ok := a.focusedPane().(*pane.Detail); ok {
		p := detail.Patent()
		if num, ok := detail.Selection(); ok {
			if p.Number.Serial == "" {
				p.Number = num
			}
			if p.DisplayNumber.Serial == "" {
				p.DisplayNumber = num
			}
		}
		return a.openAssigneeTimeline(p)
	}

	number, ok := a.focusedPane().Selection()
	if !ok {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	var project domain.ProjectID
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	a.setStatus(text.StatusLoadingPatent, number.String())
	return a, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.PatentResult
		err := a.client.Call(ctx, proto.MethodPatentGet, proto.PatentGetParams{Number: number, Project: project}, &res)
		if err != nil {
			return openAssigneesPatentLoadedMsg{err: err}
		}
		return openAssigneesPatentLoadedMsg{patent: res.Patent}
	}
}

func (a *App) cmdOpenAssigneesProject(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) != 0 {
		return a.usageError(command.OpenAssigneesProject)
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	o, cmd := overlay.NewProjectAssigneesOverlay(a.client, a.theme, a.activeProject.ID)
	a.overlays = append(a.overlays, o)
	return a, cmd
}

type openAllAssigneesHistoryPatentLoadedMsg struct {
	patent domain.Patent
	err    error
}

func (a *App) cmdOpenAllAssigneesHistory(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) != 0 {
		return a.usageError(command.OpenAllAssigneesHistory)
	}
	if detail, ok := a.focusedPane().(*pane.Detail); ok {
		p := detail.Patent()
		if num, ok := detail.Selection(); ok {
			if p.Number.Serial == "" {
				p.Number = num
			}
			if p.DisplayNumber.Serial == "" {
				p.DisplayNumber = num
			}
		}
		return a.openAssigneesProject(p)
	}

	number, ok := a.focusedPane().Selection()
	if !ok {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	var project domain.ProjectID
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	a.setStatus(text.StatusLoadingPatent, number.String())
	return a, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.PatentResult
		err := a.client.Call(ctx, proto.MethodPatentGet, proto.PatentGetParams{Number: number, Project: project}, &res)
		if err != nil {
			return openAllAssigneesHistoryPatentLoadedMsg{err: err}
		}
		return openAllAssigneesHistoryPatentLoadedMsg{patent: res.Patent}
	}
}

func (a *App) cmdPatentExpirationDate(inv invocation) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	var number domain.PatentNumber
	refresh := false
	for _, arg := range inv.args {
		switch arg {
		case "refresh", "--refresh":
			refresh = true
		default:
			if number.IsZero() {
				var err error
				number, err = domain.ParsePatentNumber(arg)
				if err != nil {
					a.setErr(text.StatusInvalidPatentNumber, err.Error())
					return a, nil
				}
			}
		}
	}
	if number.IsZero() {
		var ok bool
		number, ok = a.focusedPane().Selection()
		if !ok || number.IsZero() {
			a.setErr(text.StatusGeneric, "no patent selected (focus a detail pane first)")
			return a, nil
		}
	}
	return a.openExpirationOverlay(number, refresh)
}

func (a *App) openExpirationOverlay(number domain.PatentNumber, refresh bool) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var result proto.USPTOExpirationCalculateResult
	err := a.client.Call(ctx, proto.MethodUSPTOExpirationCalculate, proto.USPTOExpirationCalculateParams{Number: number, Refresh: refresh}, &result)
	if err != nil {
		a.setErr(text.StatusGeneric, "expiration calculation failed: "+err.Error())
		return a, nil
	}

	a.overlays = append(a.overlays, overlay.NewExpirationOverlay(a.theme, result))
	a.setStatus(text.StatusGeneric, fmt.Sprintf("Expiration analysis — %s", number))

	a.log().Info("expiration overlay opened",
		slog.String("patent", number.String()),
		slog.String("computed", result.ComputedExpirationDate),
		slog.String("google", result.GoogleExpirationDate))

	return a, nil
}

func (a *App) cmdOpenClassificationStats(invocation) (tea.Model, tea.Cmd) {
	detail, ok := a.focusedPane().(*pane.Detail)
	if !ok {
		return a, nil
	}
	p := detail.Patent()
	if num, ok := detail.Selection(); ok {
		if p.Number.Serial == "" {
			p.Number = num
		}
		if p.DisplayNumber.Serial == "" {
			p.DisplayNumber = num
		}
	}
	return a.openClassificationStats(p)
}

func (a *App) cmdOpenInventorsDirect(invocation) (tea.Model, tea.Cmd) {
	detail, ok := a.focusedPane().(*pane.Detail)
	if !ok {
		return a, nil
	}
	p := detail.Patent()
	if num, ok := detail.Selection(); ok {
		if p.Number.Serial == "" {
			p.Number = num
		}
		if p.DisplayNumber.Serial == "" {
			p.DisplayNumber = num
		}
	}
	// Direct shortcut v lands directly in the popup
	return a.openInventors(p, true)
}

func (a *App) openInventors(p domain.Patent, focusPatents bool) (tea.Model, tea.Cmd) {
	var project domain.ProjectID
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	o, cmd := overlay.NewInventorStatsOverlay(a.client, a.theme, a.text, p, project, focusPatents)
	a.overlays = append(a.overlays, o)
	return a, cmd
}

func (a *App) openAssigneesProject(p domain.Patent) (tea.Model, tea.Cmd) {
	var project domain.ProjectID
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	o, cmd := overlay.NewAllAssigneesHistoryOverlay(a.client, a.theme, a.text, p, project, false)
	a.overlays = append(a.overlays, o)
	return a, cmd
}

func (a *App) openAssigneeTimeline(p domain.Patent) (tea.Model, tea.Cmd) {
	o, cmd := overlay.NewAssigneeTimelineOverlay(a.client, a.theme, p)
	a.overlays = append(a.overlays, o)
	return a, cmd
}

// cmdFetchAssignmentsProject batch-fetches assignments for the active project's
// curated members. An optional "include-expired" arg also refreshes expired
// patents (skipped by default).
func (a *App) cmdFetchAssignmentsProject(inv invocation) (tea.Model, tea.Cmd) {
	includeExpired := false
	for _, arg := range inv.args {
		switch arg {
		case "include-expired", "--include-expired", "expired":
			includeExpired = true
		default:
			return a.usageError(command.FetchAssignmentsProject)
		}
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	return a, pane.FetchAssignmentsProjectCmd(a.client, a.activeProject.ID, includeExpired)
}

func (a *App) openClassificationStats(p domain.Patent) (tea.Model, tea.Cmd) {
	var project domain.ProjectID
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	o, cmd := overlay.NewClassificationStatsOverlay(a.client, a.theme, a.text, p, project)
	a.overlays = append(a.overlays, o)
	return a, cmd
}

func (a *App) cmdOpenProjects(invocation) (tea.Model, tea.Cmd) {
	return a.pushPane(pane.NewProjects(a.client, a.theme, a.lastProjectID, a.activeAIString(), a.activeSearchString(), a.activeBackupString(), a.activeDaemonString()).WithLogger(a.log()))
}

func (a *App) cmdCrawlDepthMax(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) > 0 {
		return a.usageError(command.CrawlDepthMax)
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var res proto.CrawlConfigResult
		if err := a.client.Call(ctx, proto.MethodCrawlConfig, nil, &res); err != nil {
			return pane.StatusMsg{Key: text.StatusCrawlStartFailed, Args: []any{err.Error()}, Error: true}
		}
		return pane.StatusMsg{Key: text.StatusCrawlDepthMax, Args: []any{res.MaxDepth}}
	}
}

func (a *App) cmdProjectClear(invocation) (tea.Model, tea.Cmd) {
	a.activeProject = nil
	a.setStatus(text.StatusClearedProject)
	return a, a.broadcast(pane.ProjectChangedMsg{})
}

func (a *App) cmdMarkActive(invocation) (tea.Model, tea.Cmd) {
	return a.runReviewState(command.MarkActive, domain.ReviewStateActive)
}
func (a *App) cmdMarkUnderReview(invocation) (tea.Model, tea.Cmd) {
	return a.runReviewState(command.MarkUnderReview, domain.ReviewStateUnderReview)
}
func (a *App) cmdMarkIgnored(invocation) (tea.Model, tea.Cmd) {
	return a.runReviewState(command.MarkIgnored, domain.ReviewStateIgnored)
}
func (a *App) cmdMarkDeleted(invocation) (tea.Model, tea.Cmd) {
	return a.runReviewState(command.MarkDeleted, domain.ReviewStateDeleted)
}

func (a *App) cmdIDSCycleStatus(inv invocation) (tea.Model, tea.Cmd) {
	if len(a.overlays) == 0 && a.focusedPane().Scope() == command.ScopeIDS {
		p := a.focusedPane()
		updated, cmd := p.Command(command.IDSCycleStatus, pane.Invocation{Repeat: inv.repeat, Args: inv.args})
		a.panes[len(a.panes)-1] = updated
		return a, cmd
	}
	return a.runBulkAction(command.IDSCycleStatus, func(project domain.ProjectID, patents []domain.PatentNumber) tea.Cmd {
		return pane.CycleIDSEntryStatusesCmd(a.client, project, patents)
	})
}

func (a *App) cmdProjectActivate(inv invocation) (tea.Model, tea.Cmd) {
	switch len(inv.args) {
	case 0:
		return a.activateProject()
	case 1:
		return a.activateProjectByArg(inv.args[0])
	default:
		return a.usageError(command.ProjectActivate)
	}
}

func (a *App) cmdAddToProject(inv invocation) (tea.Model, tea.Cmd) {
	return a.cmdAddToProjectFromSource(inv, "", command.AddToProject)
}

func (a *App) cmdAddUSPTO(inv invocation) (tea.Model, tea.Cmd) {
	return a.cmdAddToProjectFromSource(inv, domain.SourceUSPTO, command.AddUSPTO)
}

func (a *App) cmdAddGoogle(inv invocation) (tea.Model, tea.Cmd) {
	return a.cmdAddToProjectFromSource(inv, domain.SourceGoogle, command.AddGoogle)
}

// cmdAddRelated walks every relation edge of the current selection(s) and
// grants membership in the active project to any neighbor that lacks one. The
// engine resolves the relations and writes memberships in one RPC per patent.
func (a *App) cmdAddRelated(inv invocation) (tea.Model, tea.Cmd) {
	return a.runAction(command.AddRelated, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
		return pane.AddRelatedCmd(a.client, project, patent)
	})
}

// cmdFetchUSPTO fetches the best available USPTO XML for the selection: an
// empty kind tells the engine to prefer the grant and fall back to the
// pre-grant publication when no grant body is on record.
func (a *App) cmdFetchUSPTO(inv invocation) (tea.Model, tea.Cmd) {
	return a.cmdFetchUSPTOXML(inv, "", command.FetchUSPTO)
}

func (a *App) cmdFetchUSPTOPGPub(inv invocation) (tea.Model, tea.Cmd) {
	return a.cmdFetchUSPTOXML(inv, proto.USPTOXMLKindPGPub, command.FetchUSPTOPGPub)
}

// cmdFetchUSPTOAssignments triggers the engine's Patent Assignment Search
// fetch for the focused patent. A single-patent dispatch displays a modal
// spinner overlay for active feedback; multi-patent dispatches run silently
// in the background.
func (a *App) cmdFetchUSPTOAssignments(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) != 0 {
		return a.usageError(command.FetchUSPTOAssignments)
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	numbers := a.focusedSelections()
	if len(numbers) == 0 {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	if len(numbers) == 1 {
		spinner := overlay.NewUSPTOAssignmentsFetchingOverlay(a.theme, numbers[0])
		a.overlays = append(a.overlays, spinner)
		a.metrics.IncCounter("tui.uspto.fetch_assignments.interactive.started", 1)
		return a, tea.Batch(spinner.Init(), pane.FetchUSPTOAssignmentsInteractiveCmd(a.client, numbers[0]))
	}

	a.metrics.IncCounter("tui.uspto.fetch_assignments.batch.started", 1)
	a.metrics.IncCounter("tui.uspto.fetch_assignments.batch.total", int64(len(numbers)))
	cmds := make([]tea.Cmd, 0, len(numbers))
	for _, n := range numbers {
		cmds = append(cmds, pane.FetchUSPTOAssignmentsCmd(a.client, n))
	}
	return a, tea.Batch(cmds...)
}

func (a *App) cmdFetchUSPTOGrant(inv invocation) (tea.Model, tea.Cmd) {
	return a.cmdFetchUSPTOXML(inv, proto.USPTOXMLKindGrant, command.FetchUSPTOGrant)
}

func (a *App) cmdFetchUSPTOXML(inv invocation, kind proto.USPTOXMLKind, usage command.ID) (tea.Model, tea.Cmd) {
	if len(inv.args) != 0 {
		return a.usageError(usage)
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	numbers := a.focusedSelections()
	if len(numbers) == 0 {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	return a.fetchAndOpenUSPTOXML(numbers, kind)
}

// fetchAndOpenUSPTOXML dispatches one or more USPTO XML fetches concurrently.
// Each fetch is an independent RPC; the daemon serves each on its own
// goroutine, and USPTO bulk-dataset XML downloads carry no minimum-interval
// rate limit, so concurrency is bounded only by the local HTTP client and
// the upstream's own throttling.
//
// A single-patent dispatch keeps the spinner overlay (the most common
// detail-pane case); a multi-patent dispatch swaps the spinner for an
// inline batch status line so the user can still navigate other panes
// while downloads run.
func (a *App) fetchAndOpenUSPTOXML(numbers []domain.PatentNumber, kind proto.USPTOXMLKind) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	if len(numbers) == 1 {
		spinner := overlay.NewUSPTOXMLFetchingOverlay(a.theme, numbers[0], kind)
		a.overlays = append(a.overlays, spinner)
		a.xmlBatch = nil
		return a, tea.Batch(spinner.Init(), pane.FetchUSPTOXMLInteractiveCmd(a.client, numbers[0], kind))
	}
	a.xmlBatch = &xmlBatchState{
		total:     len(numbers),
		kind:      kind,
		startedAt: time.Now(),
	}
	a.status = a.text.Tf(text.StatusXMLBatchStarted, len(numbers), string(kind))
	a.statusErr = false
	a.log().Info("uspto xml batch fetch started",
		slog.Int("count", len(numbers)),
		slog.String("kind", string(kind)))
	a.metrics.IncCounter("tui.uspto.fetch_xml.batch.started", 1)
	a.metrics.IncCounter("tui.uspto.fetch_xml.batch.total", int64(len(numbers)))
	cmds := make([]tea.Cmd, 0, len(numbers))
	for _, n := range numbers {
		cmds = append(cmds, pane.FetchUSPTOXMLInteractiveCmd(a.client, n, kind))
	}
	return a, tea.Batch(cmds...)
}

// handleUSPTOAssignmentsFetched dismisses the interactive assignments fetch overlay
// and reports the status (assignments and parties counts).
func (a *App) handleUSPTOAssignmentsFetched(m pane.USPTOAssignmentsFetchedMsg) (tea.Model, tea.Cmd) {
	if len(a.overlays) > 0 {
		if _, ok := a.overlays[len(a.overlays)-1].(*overlay.USPTOAssignmentsFetchingOverlay); ok {
			a.overlays = a.overlays[:len(a.overlays)-1]
		}
	}
	if m.Err != "" {
		a.metrics.IncCounter("tui.uspto.fetch_assignments.interactive.failed", 1)
		a.setErr(text.StatusAssignmentsFetchFailed, m.Err)
		if strings.Contains(m.Err, "Missing Authentication Token") || strings.Contains(m.Err, "Forbidden") || strings.Contains(m.Err, "403") {
			msg := "USPTO Assignment Search failed due to Missing Authentication Token.\n\nYour API key is not subscribed to the product \"Patent Assignment Search\".\n\nPlease log in to developer.uspto.gov and add this product subscription to your API key."
			a.overlays = append(a.overlays, overlay.NewErrorOverlay(a.theme, msg))
		}
		return a, nil
	}
	a.metrics.IncCounter("tui.uspto.fetch_assignments.interactive.success", 1)
	a.status = a.text.Tf(text.StatusAssignmentsFetched, m.Number.String(), m.Assignments, m.Parties)
	a.statusErr = false
	return a, a.refreshPanes()
}

// handleUSPTOXMLFetched dismisses the fetch spinner and reports the saved
// path. We do not hand the file off to the OS opener: xdg-open on a .xml is
// the system browser on most Linux desktops, which is not what the user
// wants. They wanted the file on disk; the status line shows where it is.
//
// In batch mode (multi-patent fetch in flight) per-result statuses are
// aggregated into a single running tally; the final message reports
// successes / failures and clears the batch state.
func (a *App) handleUSPTOXMLFetched(m pane.USPTOXMLFetchedMsg) (tea.Model, tea.Cmd) {
	if a.xmlBatch != nil {
		return a.handleUSPTOXMLBatchTick(m)
	}
	if len(a.overlays) > 0 {
		if _, ok := a.overlays[len(a.overlays)-1].(*overlay.USPTOXMLFetchingOverlay); ok {
			a.overlays = a.overlays[:len(a.overlays)-1]
		}
	}
	if m.Err != "" {
		a.setErr(text.StatusXMLFetchFailed, m.Err)
		return a, nil
	}
	key := text.StatusXMLFetched
	if m.Cached {
		key = text.StatusXMLCached
	}
	a.status = a.text.Tf(key, string(m.Kind), m.LocalPath, m.Bytes, m.DownloadCount)
	a.statusErr = false
	return a, nil
}

func (a *App) handleUSPTOXMLBatchTick(m pane.USPTOXMLFetchedMsg) (tea.Model, tea.Cmd) {
	b := a.xmlBatch
	b.done++
	switch {
	case m.Err != "":
		b.failed++
		a.log().Warn("uspto xml batch item failed",
			slog.String("patent", m.Number.String()),
			slog.String("kind", string(m.Kind)),
			slog.String("error", m.Err))
		a.metrics.IncCounter("tui.uspto.fetch_xml.batch.failed", 1)
	case m.Cached:
		b.cached++
		a.metrics.IncCounter("tui.uspto.fetch_xml.batch.cached", 1)
	default:
		b.downloaded++
		a.metrics.IncCounter("tui.uspto.fetch_xml.batch.downloaded", 1)
	}

	if b.done < b.total {
		a.status = a.text.Tf(text.StatusXMLBatchProgress, b.done, b.total, b.cached, b.downloaded, b.failed)
		a.statusErr = false
		return a, nil
	}

	elapsed := time.Since(b.startedAt)
	a.log().Info("uspto xml batch fetch complete",
		slog.Int("total", b.total),
		slog.Int("cached", b.cached),
		slog.Int("downloaded", b.downloaded),
		slog.Int("failed", b.failed),
		slog.Duration("elapsed", elapsed),
		slog.String("kind", string(b.kind)))
	a.metrics.ObserveDuration("tui.uspto.fetch_xml.batch.duration", elapsed, b.failed > 0)
	a.status = a.text.Tf(text.StatusXMLBatchDone, b.total, b.downloaded, b.cached, b.failed)
	a.statusErr = b.failed > 0
	a.xmlBatch = nil
	return a, nil
}

func (a *App) cmdAddToProjectFromSource(inv invocation, source domain.Source, usage command.ID) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.runProjectAction(func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
			return pane.AddToProjectFromSourceCmd(a.client, project, patent, source)
		})
	}
	// Batch typed form: `:add.uspto N1 N2 N3`. Each number is parsed up-front
	// so a typo aborts the batch before any RPC is dispatched.
	numbers := make([]domain.PatentNumber, 0, len(inv.args))
	for _, arg := range inv.args {
		number, err := domain.ParsePatentNumber(arg)
		if err != nil {
			a.setErr(text.StatusInvalidPatentNumber, err.Error())
			return a, nil
		}
		numbers = append(numbers, number)
	}
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	if len(numbers) > 0 {
		for _, p := range a.panes {
			if catalog, ok := p.(*pane.Catalog); ok {
				catalog.SetPendingScrollAnchor(numbers[0])
			}
		}
	}
	cmds := make([]tea.Cmd, 0, len(numbers))
	for _, n := range numbers {
		cmds = append(cmds, pane.AddToProjectFromSourceCmd(a.client, a.activeProject.ID, n, source))
	}
	if len(numbers) > 1 {
		a.status = a.text.Tf(text.StatusAddBatchStarted, len(numbers), string(source))
		a.statusErr = false
		a.log().Info("add batch dispatched",
			slog.Int("count", len(numbers)),
			slog.String("source", string(source)))
		a.metrics.IncCounter("tui.add.batch.started", 1)
		a.metrics.IncCounter("tui.add.batch.total", int64(len(numbers)))
	}
	_ = usage
	return a, tea.Batch(cmds...)
}

// cmdTag tags the selected patent(s) within the active project. The tag name
// is the typed argument; it may contain spaces.
func (a *App) cmdTag(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.Tag)
	}
	name := strings.Join(inv.args, " ")
	return a.runBulkAction(command.Tag, func(project domain.ProjectID, patents []domain.PatentNumber) tea.Cmd {
		return pane.TagPatentCmd(a.client, project, patents, name)
	})
}

// cmdUntag removes a tag from the selected patent(s) within the active project.
func (a *App) cmdUntag(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.Untag)
	}
	name := strings.Join(inv.args, " ")
	return a.runBulkAction(command.Untag, func(project domain.ProjectID, patents []domain.PatentNumber) tea.Cmd {
		return pane.UntagPatentCmd(a.client, project, patents, name)
	})
}

// cmdTagTaxonomyAdd registers a tag in the project's taxonomy.
func (a *App) cmdTagTaxonomyAdd(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.TagTaxonomyAdd)
	}
	name := strings.Join(inv.args, " ")
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a, pane.CreateTagTaxonomyCmd(a.client, a.activeProject.ID, name)
}

// cmdTagTaxonomyList lists all taxonomy tags in the active project.
func (a *App) cmdTagTaxonomyList(inv invocation) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	o, cmd := overlay.NewTagTaxonomyOverlay(a.client, a.theme, a.text, a.activeProject.ID)
	a.overlays = append(a.overlays, o)
	return a, cmd
}

// cmdClassificationTaxonomyList lists all cached patent classifications.
func (a *App) cmdClassificationTaxonomyList(inv invocation) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	o, cmd := overlay.NewClassificationListOverlay(a.client, a.theme, a.text)
	a.overlays = append(a.overlays, o)
	return a, cmd
}

// cmdOpenPatentClassifications opens a per-patent classification overlay for
// the focused selection. Shows cached descriptions; press L inside to bulk-fetch.
func (a *App) cmdOpenPatentClassifications(inv invocation) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	numbers := a.focusedSelections()
	if len(numbers) == 0 {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	var project domain.ProjectID
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	o, cmd := overlay.NewPatentClassificationOverlay(a.client, a.theme, a.text, a.metrics, project, numbers[0])
	a.overlays = append(a.overlays, o)
	return a, cmd
}

// cmdClassificationLookup looks up details for a classification code.
func (a *App) cmdClassificationLookup(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.ClassificationLookup)
	}
	code := strings.Join(inv.args, " ")
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a, pane.LookupClassificationCmd(a.client, code)
}

// cmdTagPatentManage opens the interactive tag manager popup for the selected patent(s).
func (a *App) cmdTagPatentManage(inv invocation) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	numbers := a.focusedSelections()
	if len(numbers) == 0 {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	o, cmd := overlay.NewTagPatentOverlay(a.client, a.theme, a.text, a.activeProject.ID, numbers)
	a.overlays = append(a.overlays, o)
	return a, cmd
}

// cmdTagTaxonomyDelete removes a tag from the project's taxonomy.
func (a *App) cmdTagTaxonomyDelete(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.TagTaxonomyDelete)
	}
	name := strings.Join(inv.args, " ")
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a, pane.DeleteTagTaxonomyCmd(a.client, a.activeProject.ID, name)
}

// cmdTagStrict assigns a tag to the selected patent(s) within the active project.
func (a *App) cmdTagStrict(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.TagStrict)
	}
	name := strings.Join(inv.args, " ")
	return a.runBulkAction(command.TagStrict, func(project domain.ProjectID, patents []domain.PatentNumber) tea.Cmd {
		return pane.TagPatentStrictCmd(a.client, project, patents, name)
	})
}

// cmdUntagStrict removes a tag assignment from the selected patent(s) within the active project.
func (a *App) cmdUntagStrict(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.UntagStrict)
	}
	name := strings.Join(inv.args, " ")
	return a.runBulkAction(command.UntagStrict, func(project domain.ProjectID, patents []domain.PatentNumber) tea.Cmd {
		return pane.UntagPatentStrictCmd(a.client, project, patents, name)
	})
}

// cmdTagPatentList lists all tags assigned to the selected patent.
func (a *App) cmdTagPatentList(inv invocation) (tea.Model, tea.Cmd) {
	return a.runAction(command.TagPatentList, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
		return pane.ListPatentTagsCmd(a.client, project, patent)
	})
}

func (a *App) cmdPatentDelete(invocation) (tea.Model, tea.Cmd) {
	return a.runBulkAction(command.PatentDelete, func(_ domain.ProjectID, patents []domain.PatentNumber) tea.Cmd {
		return pane.DeletePatentsCmd(a.client, patents)
	})
}

func (a *App) cmdClearPatentCache(inv invocation) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}

	// 1. Explicit arguments provided (e.g., ":patent.clear_cache US11611785B2 ...")
	if len(inv.args) > 0 {
		// Special keyword "all" to clear everything
		if len(inv.args) == 1 && strings.ToLower(inv.args[0]) == "all" {
			recCmd := a.recordActivity(observability.Record{
				Action:     observability.ActionUIClearCache,
				Entity:     observability.EntityPatents,
				Status:     observability.StatusRequested,
				Attributes: map[string]any{observability.AttrScope: "all"},
			})
			return a, tea.Batch(pane.ClearPatentCacheCmd(a.client, nil), recCmd)
		}

		var patents []domain.PatentNumber
		for _, arg := range inv.args {
			num, err := domain.ParsePatentNumber(arg)
			if err != nil {
				a.status = fmt.Sprintf("invalid patent number %q: %v", arg, err)
				a.statusErr = true
				return a, nil
			}
			patents = append(patents, num)
		}

		recCmd := a.recordActivity(observability.Record{
			Action:     observability.ActionUIClearCache,
			Entity:     observability.EntityPatents,
			Status:     observability.StatusRequested,
			Attributes: map[string]any{observability.AttrPatents: inv.args},
		})
		return a, tea.Batch(pane.ClearPatentCacheCmd(a.client, patents), recCmd)
	}

	// 2. No arguments, check if we have focused selections in list scopes
	numbers := a.focusedSelections()
	if len(numbers) > 0 {
		recCmd := a.recordActivity(observability.Record{
			Action:     observability.ActionUIClearCache,
			Entity:     observability.EntityPatents,
			Status:     observability.StatusRequested,
			Attributes: map[string]any{observability.AttrPatentsCount: len(numbers)},
		})
		return a, tea.Batch(pane.ClearPatentCacheCmd(a.client, numbers), recCmd)
	}

	// 3. No selection and no arguments -> clear cache for all patents in the database (with confirmation)
	a.confirmCmd = pane.ClearPatentCacheCmd(a.client, nil)
	recCmd := a.recordActivity(observability.Record{
		Action:     observability.ActionUIClearCache,
		Entity:     observability.EntityPatents,
		Status:     observability.StatusRequested,
		Attributes: map[string]any{observability.AttrScope: "all_confirm"},
	})

	confirmMsg := "Are you sure you want to clear the parsed body cache for ALL patents in the database?"
	a.overlays = append(a.overlays, overlay.NewConfirm(a.theme, confirmMsg))
	return a, recCmd
}

// cmdProjectCreate opens a name-entry overlay, or — given a typed name — creates
// the project directly.
func (a *App) cmdProjectCreate(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) > 0 {
		return a.createProject(strings.Join(inv.args, " "))
	}
	a.overlays = append(a.overlays, overlay.NewTextInput(
		a.theme, a.text, overlay.PurposeCreateProject, text.NewProjectTitle, text.NewProjectCaption))
	return a, nil
}

// cmdImport fetches a patent by number — optionally forcing past the file
// cache — or loads a fixture file when the argument is a path.
func (a *App) cmdImport(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.Import)
	}
	force := false
	for _, arg := range inv.args[1:] {
		if !strings.EqualFold(arg, "force") {
			return a.usageError(command.Import)
		}
		force = true
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	target := inv.args[0]
	if isFixturePath(target) {
		return a, pane.ImportFileCmd(a.client, target)
	}
	number, err := domain.ParsePatentNumber(target)
	if err != nil {
		a.setErr(text.StatusInvalidPatentNumber, err.Error())
		return a, nil
	}
	var projectID domain.ProjectID
	if a.activeProject != nil {
		projectID = a.activeProject.ID
	}
	return a, pane.CrawlCmd(a.client, projectID, number, 0, "", force)
}

// isFixturePath reports whether arg names a fixture file rather than a patent.
func isFixturePath(arg string) bool {
	return strings.ContainsAny(arg, `/\`) || strings.HasSuffix(strings.ToLower(arg), ".json")
}

// cmdAddFile bulk-adds every patent number listed in a plain-text file into the
// active project, exactly as repeated :add would. The daemon reads, validates,
// and parses the file; each number is added with manual provenance.
func (a *App) cmdAddFile(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) != 1 {
		return a.usageError(command.AddFile)
	}
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a, pane.AddFileCmd(a.client, a.activeProject.ID, inv.args[0])
}

// cmdAddOfficeAction imports an Office Action document into the active project.
// Rather than opening a file picker first, it directly opens the metadata form overlay,
// where the File Path is an editable field. If a path argument is provided, it is
// resolved to an absolute path and pre-filled.
func (a *App) cmdAddOfficeAction(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) > 1 {
		return a.usageError(command.AddOfficeAction)
	}
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	path := ""
	if len(inv.args) == 1 {
		path = absPath(inv.args[0])
	}
	return a.openOfficeActionMetaForm(path)
}

// openOfficeActionMetaForm pushes the office-action metadata form, pre-filled
// from the active project.
func (a *App) openOfficeActionMetaForm(path string) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	o := overlay.NewOfficeActionMetaForm(a.theme, *a.activeProject, path)
	a.overlays = append(a.overlays, o)
	return a, nil
}

// cmdAddDocument files a supporting document (reference, response, …) under the
// active matter. With a path it imports directly; with none it opens the file
// picker rooted at the user's home directory.
func (a *App) cmdAddDocument(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) > 1 {
		return a.usageError(command.AddDocument)
	}
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	if len(inv.args) == 1 {
		loading := overlay.NewConverting(a.theme, "Adding Document", "Converting and adding document…")
		a.overlays = append(a.overlays, loading)
		return a, tea.Batch(loading.Init(), pane.AddMatterDocumentCmd(a.client, a.activeProject.ID, absPath(inv.args[0]), domain.MatterDocReference))
	}
	start := a.importFromDir
	if start == "" {
		var err error
		start, err = os.UserHomeDir()
		if err != nil || start == "" {
			start = "."
		}
	}
	if a.activeProject != nil {
		key := string(a.activeProject.ID) + ":" + string(overlay.PurposeAddMatterDocument)
		if last, ok := a.lastPickerDirs[key]; ok && last != "" {
			start = last
		}
	}
	o := overlay.NewFilePicker(a.theme, "Add Document", overlay.PurposeAddMatterDocument, start, []string{".pdf", ".docx", ".xlsx", ".xlsm", ".xltx", ".xltm", ".xls", ".txt", ".csv", ".tsv", ".md"})
	a.overlays = append(a.overlays, o)
	return a, o.Init()
}

// cmdDraftResponse creates an office-action response draft for the active matter
// (linked to the latest office action) and opens the split response editor to
// copy from the matter's documents into it.
func (a *App) cmdDraftResponse(invocation) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a, pane.CreateResponseCmd(a.client, a.activeProject.ID)
}

// cmdLogComm opens the form to record one communications-log entry.
func (a *App) cmdLogComm(invocation) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	o := overlay.NewCommEventForm(a.theme, a.activeProject.ID)
	a.overlays = append(a.overlays, o)
	return a, nil
}

// cmdShowDeadlines opens the cross-matter deadlines docket (office-action
// responses + patent maintenance fees).
func (a *App) cmdShowDeadlines(invocation) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	o := overlay.NewDeadlines(a.client, a.theme)
	a.overlays = append(a.overlays, o)
	return a, o.Init()
}

// cmdTrackRenewals tracks a granted patent's U.S. maintenance-fee deadlines.
func (a *App) cmdTrackRenewals(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) < 1 || len(inv.args) > 2 {
		return a.usageError(command.TrackRenewals)
	}
	var entitySize string
	if len(inv.args) == 2 {
		entitySize = inv.args[1]
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a, pane.TrackRenewalsCmd(a.client, inv.args[0], entitySize)
}

// cmdUntrackRenewals stops tracking a patent's U.S. maintenance-fee/annuity deadlines.
func (a *App) cmdUntrackRenewals(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) != 1 {
		return a.usageError(command.UntrackRenewals)
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a, pane.UntrackRenewalsCmd(a.client, inv.args[0])
}

// cmdLogTime records a manual time entry: :log.time <activity> <duration> [note].
func (a *App) cmdLogTime(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) < 2 {
		return a.usageError(command.LogTime)
	}
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	activity, err := domain.ParseTimeActivity(inv.args[0])
	if err != nil {
		return a.usageError(command.LogTime)
	}
	seconds, err := parseTimeDuration(inv.args[1])
	if err != nil {
		return a.usageError(command.LogTime)
	}
	note := strings.Join(inv.args[2:], " ")
	return a, pane.LogTimeCmd(a.client, a.activeProject.ID, activity, seconds, note)
}

// cmdValidateTime opens the review queue of unvalidated time entries.
func (a *App) cmdValidateTime(invocation) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	o := overlay.NewTimeReview(a.client, a.theme, a.activeProject.ID)
	a.overlays = append(a.overlays, o)
	return a, o.Init()
}

// cmdShowTime opens the matter's time + AI-usage summary.
func (a *App) cmdShowTime(invocation) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	o := overlay.NewTimeSummary(a.client, a.theme, a.activeProject.ID)
	a.overlays = append(a.overlays, o)
	return a, o.Init()
}

// parseTimeDuration accepts a Go duration ("30m", "1h15m"), an "H:MM" clock
// value, or a plain integer read as minutes, and returns whole seconds.
func parseTimeDuration(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if d, err := time.ParseDuration(s); err == nil {
		secs := int(d.Round(time.Second) / time.Second)
		if secs <= 0 {
			return 0, fmt.Errorf("non-positive duration")
		}
		return secs, nil
	}
	if h, m, ok := strings.Cut(s, ":"); ok {
		hi, err1 := strconv.Atoi(strings.TrimSpace(h))
		mi, err2 := strconv.Atoi(strings.TrimSpace(m))
		if err1 == nil && err2 == nil && hi >= 0 && mi >= 0 {
			secs := hi*3600 + mi*60
			if secs > 0 {
				return secs, nil
			}
		}
		return 0, fmt.Errorf("invalid H:MM duration %q", s)
	}
	if mins, err := strconv.Atoi(s); err == nil && mins > 0 {
		return mins * 60, nil
	}
	return 0, fmt.Errorf("unrecognized duration %q", s)
}

// cmdOpenComms opens the active matter's communications log.
func (a *App) cmdOpenComms(invocation) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	o := overlay.NewMatterComms(a.client, a.theme, a.activeProject.ID)
	a.overlays = append(a.overlays, o)
	return a, o.Init()
}

// cmdOpenDocuments opens the active matter's document list.
func (a *App) cmdOpenDocuments(invocation) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	var oa *domain.OfficeAction
	if detail, ok := a.focusedPane().(*pane.OfficeActionDetail); ok {
		currentOA := detail.OfficeAction()
		oa = &currentOA
	}
	o := overlay.NewMatterDocuments(a.client, a.theme, a.activeProject.ID, oa)
	a.overlays = append(a.overlays, o)
	return a, o.Init()
}

// cmdSetMatterType records the active project's prosecution stage.
func (a *App) cmdSetMatterType(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) != 1 {
		return a.usageError(command.SetMatterType)
	}
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	mt, err := domain.ParseMatterType(inv.args[0])
	if err != nil {
		return a.usageError(command.SetMatterType)
	}
	return a, pane.SetMatterTypeCmd(a.client, a.activeProject.ID, mt)
}

// cmdListOfficeActions opens the active matter's office-action table as a pane.
// Each row drills into the detail pane (documents · timing · communications ·
// response); `a` imports a new office action.
func (a *App) cmdListOfficeActions(invocation) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a.pushPane(pane.NewOfficeActions(a.client, a.theme, a.activeProject.ID).WithLogger(a.log()))
}

// handleFilePicked routes a file chosen in a FilePicker overlay to its action.
func (a *App) handleFilePicked(m overlay.FilePickedMsg) (tea.Model, tea.Cmd) {
	switch m.Purpose {
	case overlay.PurposeAddOfficeAction:
		// Collect the examiner and dates before importing the office action.
		return a.openOfficeActionMetaForm(m.Path)
	case overlay.PurposeAddMatterDocument:
		if a.activeProject == nil {
			a.setErr(text.StatusNoActiveProject)
			return a, nil
		}
		if a.client == nil {
			a.setErr(text.StatusDaemonUnavailable)
			return a, nil
		}
		loading := overlay.NewConverting(a.theme, "Adding Document", "Converting and adding document…")
		a.overlays = append(a.overlays, loading)
		return a, tea.Batch(loading.Init(), pane.AddMatterDocumentCmd(a.client, a.activeProject.ID, m.Path, domain.MatterDocReference))
	}
	return a, nil
}

// cmdExportAdded writes the active project's manually-added patents to a
// plain-text list file that :add.file can later reload.
// cmdExportAdded writes the active project's manually-added patents to a
// plain-text list file. With an explicit path argument it writes there
// directly; with no argument it proposes a default path in the export
// directory and opens a confirmation popup so the user can see where the file
// will be saved before it is written. Either way, a result modal afterwards
// reports how many patents were exported and where.
func (a *App) cmdExportAdded(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) > 1 {
		return a.usageError(command.ExportAdded)
	}
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}

	// Explicit path: the user chose the destination, so skip the location
	// picker — but still warn before clobbering an existing file. The path is
	// resolved to absolute so the popup always shows the full destination.
	if len(inv.args) == 1 {
		path := absPath(inv.args[0])
		writeCmd := pane.ExportAddedCmd(a.client, a.activeProject.ID, path)
		if _, err := os.Stat(path); err == nil {
			a.confirmCmd = writeCmd
			a.overlays = append(a.overlays, overlay.NewConfirm(a.theme,
				fmt.Sprintf("%s already exists. Overwrite?", path)))
			return a, nil
		}
		return a, writeCmd
	}

	// No path: propose a default in the export directory and confirm it.
	dir := a.notesExportDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dir = home
	}
	safeName := strings.NewReplacer(" ", "-", "/", "-", "\\", "-").Replace(a.activeProject.Name)
	filename := fmt.Sprintf("patentmine-added-%s-%s.txt", safeName, time.Now().Format(domain.DateLayout))
	path := absPath(filepath.Join(dir, filename))
	a.confirmCmd = pane.ExportAddedCmd(a.client, a.activeProject.ID, path)
	a.overlays = append(a.overlays, overlay.NewExportConfirm(a.theme, "Export Added Patents", path, scanExistingAddedExports(dir)))
	return a, nil
}

// absPath resolves p to an absolute path for display and writing, falling back
// to the original string if resolution fails.
func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// scanExistingAddedExports returns the patentmine-added-*.txt filenames already
// in dir, shown in the export confirmation so the user can spot prior exports.
func scanExistingAddedExports(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "patentmine-added-") && strings.HasSuffix(name, ".txt") {
			out = append(out, name)
		}
	}
	return out
}

type aiPatentLoadedMsg struct {
	patent domain.Patent
	err    error
}

func (a *App) cmdAIAnalyze(invocation) (tea.Model, tea.Cmd) {
	number, ok := a.focusedPane().Selection()
	if !ok {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	var project domain.ProjectID
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	a.setStatus(text.StatusAIAnalysisStarted, number.String(), string(a.aiProvider))
	return a, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.PatentResult
		err := a.client.Call(ctx, proto.MethodPatentGet, proto.PatentGetParams{Number: number, Project: project}, &res)
		if err != nil {
			return aiPatentLoadedMsg{err: err}
		}
		return aiPatentLoadedMsg{patent: res.Patent}
	}
}

func (a *App) cmdSettingsAI(invocation) (tea.Model, tea.Cmd) {
	o := overlay.NewSettingsOverlay(a.theme, a.aiProvider, a.geminiAPIKey, a.ollamaHost, a.ollamaModel, a.openaiAPIKey, a.openaiModel, a.usptoConfigured)
	a.overlays = append(a.overlays, o)
	return a, nil
}

func (a *App) cmdOpenAllNotes(invocation) (tea.Model, tea.Cmd) {
	return a.pushPane(pane.NewAllNotes(a.client, a.theme, a.activeProject).
		WithExportDir(a.notesExportDir).
		WithMetrics(a.metrics))
}

func (a *App) cmdOpenOrphans(invocation) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a.pushPane(pane.NewOrphans(a.client, a.theme).WithLogger(a.log()))
}
