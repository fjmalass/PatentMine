package tui

import (
	"context"
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
	return a, a.openPatentsInBrowser(numbers)
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

func (a *App) cmdOpenIDS(invocation) (tea.Model, tea.Cmd) { return a.openIDS() }

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
		return a.openAssignees(p)
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

	// 2. Only activate on Enter if the cursor is actually on the Inventors field
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

func (a *App) cmdOpenAssignees(invocation) (tea.Model, tea.Cmd) {
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
	return a.openAssignees(p)
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

func (a *App) openAssignees(p domain.Patent) (tea.Model, tea.Cmd) {
	var project domain.ProjectID
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	o, cmd := overlay.NewAssigneeStatsOverlay(a.client, a.theme, a.text, p, project)
	a.overlays = append(a.overlays, o)
	return a, cmd
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
	return a.pushPane(pane.NewProjects(a.client, a.theme, a.activeAIString(), a.activeSearchString()).WithLogger(a.log()))
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
	switch len(inv.args) {
	case 0:
		return a.runProjectAction(func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
			return pane.AddToProjectCmd(a.client, project, patent)
		})
	case 1:
		number, err := domain.ParsePatentNumber(inv.args[0])
		if err != nil {
			a.setErr(text.StatusInvalidPatentNumber, err.Error())
			return a, nil
		}
		if a.activeProject == nil {
			a.setErr(text.StatusNoActiveProject)
			return a, nil
		}
		if a.client == nil {
			a.setErr(text.StatusDaemonUnavailable)
			return a, nil
		}
		return a, pane.AddToProjectCmd(a.client, a.activeProject.ID, number)
	default:
		return a.usageError(command.AddToProject)
	}
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
	return a, pane.CrawlCmd(a.client, number, 0, "", force)
}

// isFixturePath reports whether arg names a fixture file rather than a patent.
func isFixturePath(arg string) bool {
	return strings.ContainsAny(arg, `/\`) || strings.HasSuffix(strings.ToLower(arg), ".json")
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
	o := overlay.NewSettingsOverlay(a.theme, a.aiProvider, a.geminiAPIKey, a.ollamaHost, a.ollamaModel, a.usptoConfigured)
	a.overlays = append(a.overlays, o)
	return a, nil
}

func (a *App) cmdOpenAllNotes(invocation) (tea.Model, tea.Cmd) {
	return a.pushPane(pane.NewAllNotes(a.client, a.theme, a.activeProject).
		WithExportDir(a.notesExportDir).
		WithMetrics(a.metrics))
}
