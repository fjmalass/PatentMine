package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
	"patentmine/internal/uspto"
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
			return pane.StatusMsg{Key: text.StatusUsage, Args: []any{err.Error()}, Error: true}
		}
		return pane.StatusMsg{Key: text.StatusUsage, Args: []any{"source mode: " + res.Mode}}
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
	var number domain.PatentNumber
	if len(inv.args) > 0 {
		var err error
		number, err = domain.ParsePatentNumber(inv.args[0])
		if err != nil {
			a.setErr(text.StatusInvalidPatentNumber, err.Error())
			return a, nil
		}
	} else {
		var ok bool
		number, ok = a.focusedPane().Selection()
		if !ok || number.IsZero() {
			a.setErr(text.StatusGeneric, "no patent selected (focus a detail pane first)")
			return a, nil
		}
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
		Entity:   "patent",
		EntityID: number.String(),
		Status:   map[bool]string{true: "error", false: "opened"}[listErr != nil],
		Attributes: map[string]any{
			"diff_count": len(listRes.Diffs),
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

	// 2. Cursor on a PGPub/Grant XML URL row: ask the daemon for a ready-to-view
	// TOML rendering of the raw XML. This path no longer assumes the XML file
	// returned by the server is readable on the client's local disk.
	if kind, ok := detail.ResolveCursorUSPTOXML(); ok {
		return a.viewUSPTOXML(detail.PatentNumber(), kind)
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
	return a.pushPane(pane.NewProjects(a.client, a.theme, a.activeAIString(), a.activeSearchString(), a.activeBackupString(), a.activeDaemonString()).WithLogger(a.log()))
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

func (a *App) cmdFetchUSPTOPGPub(inv invocation) (tea.Model, tea.Cmd) {
	return a.cmdFetchUSPTOXML(inv, proto.USPTOXMLKindPGPub, command.FetchUSPTOPGPub)
}

// cmdFetchUSPTOAssignments triggers the engine's Patent Assignment Search
// fetch for the focused patent. One RPC per selected patent — the chain is
// usually small and the search endpoint replies fast, so no spinner overlay.
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
	cmds := make([]tea.Cmd, 0, len(numbers))
	for _, n := range numbers {
		cmds = append(cmds, pane.FetchUSPTOAssignmentsCmd(a.client, n))
	}
	return a, tea.Batch(cmds...)
}

func (a *App) cmdViewUSPTOXML(inv invocation) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}

	var number domain.PatentNumber
	if len(inv.args) > 0 {
		var err error
		number, err = domain.ParsePatentNumber(inv.args[0])
		if err != nil {
			a.setErr(text.StatusInvalidPatentNumber, err.Error())
			return a, nil
		}
	} else {
		var ok bool
		number, ok = a.focusedPane().Selection()
		if !ok || number.IsZero() {
			a.setErr(text.StatusNoPatentSelected)
			return a, nil
		}
	}

	// Default to grant for the direct command. Users who want the pgpub version
	// can position the cursor on the PGPub XML line in the detail pane and press Enter.
	return a.viewUSPTOXML(number, proto.USPTOXMLKindGrant)
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
	// Single-patent explicit fetch commands now use the dedicated view RPC
	// (daemon does parse + TOML) so we never assume the returned LocalPath is
	// readable on the client machine.
	if len(numbers) == 1 {
		return a.viewUSPTOXML(numbers[0], kind)
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

// viewUSPTOXML asks the daemon (via uspto.xml.view) to ensure the requested
// USPTO XML is present on the server and return a pre-rendered TOML view of
// it. The TUI never opens a server-side file path locally.
func (a *App) viewUSPTOXML(number domain.PatentNumber, kind proto.USPTOXMLKind) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	// Use the full-screen pane (like FullText) instead of a popup overlay.
	return a, tea.Batch(
		pane.FetchUSPTOXMLViewCmd(a.client, number, kind), // still uses the RPC for content
	)
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

	// Open the downloaded USPTO XML file and show it in TOML format!
	f, err := os.Open(m.LocalPath)
	if err == nil {
		defer f.Close()

		// Time the raw XML parse (separate from the subsequent struct→TOML step).
		parseStart := time.Now()
		doc, parseErr := uspto.ParseUSPTOGrantXML(f)
		parseDur := time.Since(parseStart)
		a.metrics.ObserveDuration("tui.uspto.xml.parse.duration", parseDur, parseErr != nil)

		if parseErr != nil {
			a.metrics.IncCounter("tui.uspto.xml.parse.error", 1)
			a.log().Error("uspto xml parse failed (for viewer)",
				slog.String("patent", m.Number.String()),
				slog.String("kind", string(m.Kind)),
				slog.Duration("parse_duration", parseDur),
				slog.String("error", parseErr.Error()))
		} else {
			// Measure the XML struct → TOML conversion (the work done by the
			// new toml_viewer helpers: json roundtrip + MapToTOML recursion).
			convStart := time.Now()
			tomlStr, tomlErr := overlay.StructToTOML(doc)
			convDur := time.Since(convStart)
			a.metrics.ObserveDuration("tui.uspto.xml.to_toml.duration", convDur, tomlErr != nil)

			if tomlErr != nil {
				a.metrics.IncCounter("tui.uspto.xml.to_toml.error", 1)
				a.log().Error("uspto xml to toml conversion failed",
					slog.String("patent", m.Number.String()),
					slog.String("kind", string(m.Kind)),
					slog.Duration("duration", convDur),
					slog.String("error", tomlErr.Error()))
			} else {
				a.metrics.IncCounter("tui.uspto.xml.to_toml.count", 1)

				title := fmt.Sprintf("%s XML · %s", strings.ToUpper(string(m.Kind)), m.Number.String())
				a.overlays = append(a.overlays, overlay.NewTOMLViewer(a.theme, title, tomlStr))

				bib := doc.Bibliographic
				if bib.ApplicationRef.DocumentID.DocNumber == "" && doc.BibliographicApp.ApplicationRef.DocumentID.DocNumber != "" {
					bib = doc.BibliographicApp
				}
				inventorsShort := "-"
				if len(bib.Inventors) > 0 {
					first := bib.Inventors[0].Addressbook.LastName
					if first == "" {
						first = bib.Inventors[0].Addressbook.OrgName
					}
					if len(bib.Inventors) > 1 {
						inventorsShort = first + " et al."
					} else {
						inventorsShort = first
					}
				}
				pubDate := bib.PublicationRef.Date
				if pubDate == "" {
					pubDate = "-"
				}
				invTitle := strings.TrimSpace(bib.InventionTitle)

				attrs := map[string]any{
					"kind":             string(m.Kind),
					"local_path":       m.LocalPath,
					"bytes":            m.Bytes,
					"cached":           m.Cached,
					"title":            invTitle,
					"publication_date": pubDate,
					"inventors_short":  inventorsShort,
					"parse_duration_ms": parseDur.Milliseconds(),
					"to_toml_duration_ms": convDur.Milliseconds(),
				}
				if a.activeProject != nil {
					attrs["project"] = string(a.activeProject.ID)
				}

				// Telemetry and activity tracking
				a.metrics.IncCounter("tui.uspto.xml.viewed_total", 1)
				a.recordActivity(observability.Record{
					Action:     observability.ActionUSPTOXMLView,
					Entity:     "patent",
					EntityID:   m.Number.String(),
					Status:     "opened",
					Attributes: attrs,
				})

				a.log().Info("uspto xml parsed and viewed in toml format",
					slog.String("patent", m.Number.String()),
					slog.String("kind", string(m.Kind)),
					slog.String("local_path", m.LocalPath),
					slog.Bool("cached", m.Cached),
					slog.Duration("to_toml_duration", convDur))
			}
		}
	}

	return a, nil
}

// handleUSPTOXMLViewReady is the clean daemon-backed path for the raw XML
// TOML viewer popup. The daemon performed the fetch (if needed), parse, and
// StructToTOML conversion; we receive the finished TOML string and simply
// display it. No server-side file paths are ever opened on the client.
func (a *App) handleUSPTOXMLViewReady(m pane.USPTOXMLViewReadyMsg) (tea.Model, tea.Cmd) {
	// Dismiss any fetching spinner
	if len(a.overlays) > 0 {
		if _, ok := a.overlays[len(a.overlays)-1].(*overlay.USPTOXMLFetchingOverlay); ok {
			a.overlays = a.overlays[:len(a.overlays)-1]
		}
	}

	if m.Err != "" {
		a.setErr(text.StatusXMLFetchFailed, m.Err)
		return a, nil
	}

	if m.TOML == "" {
		a.setErr(text.StatusGeneric, "daemon returned empty TOML for XML view")
		return a, nil
	}

	title := m.Title
	if title == "" {
		title = fmt.Sprintf("%s XML · %s", strings.ToUpper(string(m.Kind)), m.Number.String())
	}
	if m.ConvertDurationMillis > 0 {
		title = fmt.Sprintf("%s  ·  %dms", title, m.ConvertDurationMillis)
	}

	attrs := map[string]any{
		"kind":                string(m.Kind),
		"bytes":               m.Bytes,
		"cached":              m.Cached,
		"download_count":      m.DownloadCount,
		"via":                 "uspto.xml.view",
		"convert_duration_ms": m.ConvertDurationMillis,
	}
	if a.activeProject != nil {
		attrs["project"] = string(a.activeProject.ID)
	}

	a.metrics.IncCounter("tui.uspto.xml.viewed_total", 1)
	a.recordActivity(observability.Record{
		Action:     observability.ActionUSPTOXMLView,
		Entity:     "patent",
		EntityID:   m.Number.String(),
		Status:     "opened",
		Attributes: attrs,
	})

	a.log().Info("uspto xml viewed via daemon",
		slog.String("patent", m.Number.String()),
		slog.String("kind", string(m.Kind)),
		slog.Bool("cached", m.Cached),
		slog.Int64("bytes", m.Bytes),
		slog.Int64("convert_duration_ms", m.ConvertDurationMillis))

	// Full-screen pane (like FullText), not a popup overlay.
	return a.pushPane(pane.NewUSPTORawXML(a.client, a.theme, m.Number, string(m.Kind), m.TOML, title))
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
				Entity:     "patents",
				Status:     "requested",
				Attributes: map[string]any{"scope": "all"},
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
			Entity:     "patents",
			Status:     "requested",
			Attributes: map[string]any{"patents": inv.args},
		})
		return a, tea.Batch(pane.ClearPatentCacheCmd(a.client, patents), recCmd)
	}

	// 2. No arguments, check if we have focused selections in list scopes
	numbers := a.focusedSelections()
	if len(numbers) > 0 {
		recCmd := a.recordActivity(observability.Record{
			Action:     observability.ActionUIClearCache,
			Entity:     "patents",
			Status:     "requested",
			Attributes: map[string]any{"patents_count": len(numbers)},
		})
		return a, tea.Batch(pane.ClearPatentCacheCmd(a.client, numbers), recCmd)
	}

	// 3. No selection and no arguments -> clear cache for all patents in the database (with confirmation)
	a.confirmCmd = pane.ClearPatentCacheCmd(a.client, nil)
	recCmd := a.recordActivity(observability.Record{
		Action:     observability.ActionUIClearCache,
		Entity:     "patents",
		Status:     "requested",
		Attributes: map[string]any{"scope": "all_confirm"},
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

func (a *App) cmdOpenOrphans(invocation) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a.pushPane(pane.NewOrphans(a.client, a.theme).WithLogger(a.log()))
}
