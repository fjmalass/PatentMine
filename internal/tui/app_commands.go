package tui

import (
	"context"
	"fmt"
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
	if a.activityDir == "" {
		a.setErr(text.StatusUsage, "activity logging is not configured")
		return a, nil
	}
	records, err := observability.ReadActivityRecords(a.activityDir, observability.ActivityQuery{Limit: 200})
	if err != nil {
		a.setErr(text.StatusUsage, err.Error())
		return a, nil
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

func (a *App) cmdHistoryBack(invocation) (tea.Model, tea.Cmd) {
	if len(a.history) == 0 {
		a.setErr(text.StatusHistoryEmpty)
		return a, nil
	}
	targetIndex := a.historyCursor - 1
	if targetIndex < 0 {
		a.setErr(text.StatusHistoryAtEnd)
		return a, nil
	}
	targetNumber := a.history[targetIndex]
	return a, a.checkPatentExists(targetIndex, targetNumber)
}

func (a *App) cmdHistoryForward(invocation) (tea.Model, tea.Cmd) {
	if len(a.history) == 0 {
		a.setErr(text.StatusHistoryEmpty)
		return a, nil
	}
	targetIndex := a.historyCursor + 1
	if targetIndex >= len(a.history) {
		a.setErr(text.StatusHistoryAtEnd)
		return a, nil
	}
	targetNumber := a.history[targetIndex]
	return a, a.checkPatentExists(targetIndex, targetNumber)
}

func (a *App) cmdOpenHistory(invocation) (tea.Model, tea.Cmd) {
	if a.activityDir == "" {
		a.setErr(text.StatusUsage, "activity logging is not configured")
		return a, nil
	}
	records, err := observability.ReadActivityRecords(a.activityDir, observability.ActivityQuery{Limit: 500})
	if err != nil {
		a.setErr(text.StatusUsage, err.Error())
		return a, nil
	}
	var filtered []observability.Record
	var lastKey string
	for _, r := range records {
		key := r.Action + ":" + r.Entity + ":" + r.EntityID
		if r.Action == "filter.apply" || r.Action == "project.switch" || r.Action == "membership.set_state" || r.Action == "patent.tag_assign" || r.Action == "patent.tag_remove" {
			if key != lastKey {
				filtered = append(filtered, r)
				lastKey = key
			}
		} else if r.Action == "ui.focus" && r.Entity == "patent" {
			// Only include actual detail/navigation page views (detail, citations, family, ids, fulltext),
			// ignoring background catalog hover cursor dwell events (catalog).
			if scope, ok := r.Metadata["scope"].(string); ok && (scope == "detail" || scope == "citations" || scope == "family" || scope == "ids" || scope == "fulltext") {
				if key != lastKey {
					filtered = append(filtered, r)
					lastKey = key
				}
			}
		}
	}
	if len(filtered) == 0 {
		a.setErr(text.StatusHistoryEmpty)
		return a, nil
	}
	projectNames := make(map[string]string)
	if a.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()
		var res proto.ProjectListResult
		if err := a.client.Call(ctx, proto.MethodProjectList, nil, &res); err == nil {
			for _, p := range res.Projects {
				projectNames[string(p.ID)] = p.Name
			}
		}
	}
	o := overlay.NewHistoryOverlay(a.theme, filtered, projectNames)
	a.overlays = append(a.overlays, o)
	return a, nil
}

func (a *App) handleHistoryReplay(rec observability.Record, confirmed bool) (tea.Model, tea.Cmd) {
	switch rec.Action {
	case "ui.focus", "membership.set_state", "patent.tag_assign", "patent.tag_remove":
		// Immediate navigation to patent details/views (no confirmation overlay required)
		a.overlays = nil
		if len(a.panes) > 1 {
			a.panes = a.panes[:1]
		}
		var numStr string
		if rec.Action == "ui.focus" {
			numStr = rec.EntityID
		} else if reqNum, ok := rec.Metadata["requested_number"].(string); ok && reqNum != "" {
			numStr = reqNum
		} else {
			// fallback: parse from entity ID (e.g. "p-123/US12345" or "p-123/US12345/tag")
			parts := strings.Split(rec.EntityID, "/")
			if len(parts) >= 2 {
				numStr = parts[1]
			}
		}
		if number, err := domain.ParsePatentNumber(numStr); err == nil {
			entityParts := strings.Split(rec.EntityID, "/")
			isMembershipOrTag := rec.Action == "membership.set_state" || rec.Action == "patent.tag_assign" || rec.Action == "patent.tag_remove"

			var project domain.ProjectID
			if pVal, ok := rec.Metadata["project"].(string); ok && pVal != "" {
				project = domain.ProjectID(pVal)
			} else if isMembershipOrTag && len(entityParts) >= 1 && entityParts[0] != "" {
				project = domain.ProjectID(entityParts[0])
			} else if a.activeProject != nil {
				project = a.activeProject.ID
			}

			var switchCmd tea.Cmd
			if project != "" && (a.activeProject == nil || a.activeProject.ID != project) {
				if proj, ok := a.resolveProjectArg(string(project)); ok {
					a.activeProject = &proj
					a.lastProjectID = proj.ID
					if a.saveLastProject != nil {
						_ = a.saveLastProject(proj.ID)
					}
					a.setStatus(text.StatusActiveProject, proj.Name)
					switchCmd = a.broadcast(pane.ProjectChangedMsg{Project: &proj})
				}
			}

			scope, _ := rec.Metadata["scope"].(string)
			var replayModel tea.Model
			var replayCmd tea.Cmd
			switch scope {
			case "citations":
				kind := domain.RelationCites
				if kStr, ok := rec.Metadata["relation"].(string); ok {
					kind = domain.RelationKind(kStr)
				}
				replayModel, replayCmd = a.pushPane(pane.NewCitations(a.client, a.theme, number, kind).WithLogger(a.log()))
			case "family":
				depth := 1
				if dVal, ok := rec.Metadata["depth"].(float64); ok {
					depth = int(dVal)
				}
				var countries []string
				if cList, ok := rec.Metadata["countries"].([]any); ok {
					for _, c := range cList {
						if cStr, ok := c.(string); ok {
							countries = append(countries, cStr)
						}
					}
				}
				replayModel, replayCmd = a.pushPane(pane.NewFamilyGraph(a.client, a.theme, number, depth, countries).WithLogger(a.log()))
			case "fulltext":
				bound := a.keymaps.BoundLetters(command.ScopeFullText)
				replayModel, replayCmd = a.pushPane(pane.NewFullText(a.client, a.theme, number, project, bound).WithLogger(a.log()))
			case "ids":
				replayModel, replayCmd = a.pushPane(pane.NewIDSDetail(a.client, a.theme, number, project).WithLogger(a.log()))
			default:
				bound := a.keymaps.BoundLetters(command.ScopeDetail)
				replayModel, replayCmd = a.pushPane(pane.NewDetail(a.client, a.theme, number, project, bound).WithLogger(a.log()))
			}
			return replayModel, tea.Batch(switchCmd, replayCmd)
		}
		a.setErr(text.StatusHistoryPatentUnavailable, rec.EntityID)
		return a, nil

	case "filter.apply":
		if !confirmed {
			a.confirmCmd = func() tea.Msg { return overlay.ConfirmHistoryReplayMsg{Record: rec} }
			prompt := fmt.Sprintf("Apply filter '%s'?", rec.EntityID)
			o := overlay.NewConfirm(a.theme, prompt)
			a.overlays = append(a.overlays, o)
			return a, nil
		}
		a.overlays = nil
		if len(a.panes) > 1 {
			a.panes = a.panes[:1]
		}
		return a.executeTypedCommand("filter " + rec.EntityID)

	case "project.switch":
		if !confirmed {
			a.confirmCmd = func() tea.Msg { return overlay.ConfirmHistoryReplayMsg{Record: rec} }
			projectName := rec.EntityID
			if name, ok := rec.Metadata["project_name"].(string); ok && name != "" {
				projectName = name
			}
			prompt := fmt.Sprintf("Switch to project '%s'?", projectName)
			o := overlay.NewConfirm(a.theme, prompt)
			a.overlays = append(a.overlays, o)
			return a, nil
		}
		a.overlays = nil
		if len(a.panes) > 1 {
			a.panes = a.panes[:1]
		}
		return a.activateProjectByArg(rec.EntityID)
	}
	return a, nil
}

func (a *App) cmdOpenAllNotes(invocation) (tea.Model, tea.Cmd) {
	return a.pushPane(pane.NewAllNotes(a.client, a.theme, a.activeProject).
		WithExportDir(a.notesExportDir).
		WithMetrics(a.metrics))
}
