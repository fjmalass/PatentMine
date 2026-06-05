package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/text"
	"patentmine/internal/tui/overlay"
	"patentmine/internal/tui/pane"
)

const historyRawLimit = 500

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
	return a, a.checkPatentExists(targetIndex, a.history[targetIndex])
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
	return a, a.checkPatentExists(targetIndex, a.history[targetIndex])
}

func (a *App) cmdOpenHistory(invocation) (tea.Model, tea.Cmd) {
	feed, err := a.loadHistoryFeed()
	if err != nil {
		a.setErr(text.StatusUsage, err.Error())
		return a, nil
	}
	if len(feed.Records) == 0 {
		a.setErr(text.StatusHistoryEmpty)
		return a, nil
	}
	a.overlays = append(a.overlays, overlay.NewHistoryOverlay(a.theme, feed.Records, a.historyProjectNames()))
	return a, nil
}

func (a *App) loadHistoryFeed() (observability.HistoryFeed, error) {
	if a.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()
		var feed observability.HistoryFeed
		err := a.client.Call(ctx, proto.MethodHistoryFeed, proto.HistoryFeedParams{RawLimit: historyRawLimit}, &feed)
		if err == nil {
			return feed, nil
		}
	}
	if a.activityDir == "" {
		return observability.HistoryFeed{}, fmt.Errorf("activity logging is not configured")
	}
	return observability.ReadHistoryFeed(a.activityDir, observability.HistoryQuery{RawLimit: historyRawLimit})
}

func (a *App) historyProjectNames() map[string]string {
	projectNames := make(map[string]string)
	if a.client == nil {
		return projectNames
	}
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	var res proto.ProjectListResult
	if err := a.client.Call(ctx, proto.MethodProjectList, nil, &res); err != nil {
		return projectNames
	}
	for _, project := range res.Projects {
		projectNames[string(project.ID)] = project.Name
	}
	return projectNames
}

func (a *App) handleHistoryReplay(rec observability.Record, confirmed bool) (tea.Model, tea.Cmd) {
	switch rec.Action {
	case observability.ActionUIFocus,
		observability.ActionMembershipSetState,
		observability.ActionPatentTagAssign, observability.ActionPatentTagRemove,
		observability.ActionIDSEntrySave, observability.ActionIDSEntryDelete:
		return a.replayHistoryPatentTarget(rec)
	case observability.ActionFilterApply:
		return a.replayHistoryFilter(rec, confirmed)
	case observability.ActionProjectSwitch:
		return a.replayHistoryProjectSwitch(rec, confirmed)
	case observability.ActionOfficeActionImport,
		observability.ActionOfficeActionSaveNotes,
		observability.ActionOfficeActionUpdate,
		observability.ActionOfficeActionDelete:
		return a.replayHistoryOfficeAction(rec)
	case observability.ActionMatterDocumentImport,
		observability.ActionMatterDocumentRename,
		observability.ActionMatterDocumentDelete,
		observability.ActionMatterDocumentExtract,
		observability.ActionMatterDocumentTag,
		observability.ActionMatterDocumentUntag,
		observability.ActionMatterDocumentAssignOA,
		observability.ActionMatterDocumentUnassignOA:
		return a.replayHistoryMatterDocument(rec)
	default:
		return a, nil
	}
}

func (a *App) replayHistoryPatentTarget(rec observability.Record) (tea.Model, tea.Cmd) {
	number, ok := historyPatentNumber(rec)
	if !ok {
		a.setErr(text.StatusHistoryPatentUnavailable, rec.EntityID)
		return a, nil
	}
	project := a.historyProjectID(rec)
	switchCmd := a.switchHistoryProject(project)
	a.closeHistoryReplayOverlays()
	scope, _ := rec.Attributes[observability.AttrScope].(string)
	if replayScope := overlay.ReplayScope(rec.Action); replayScope != "" {
		scope = replayScope
	}
	replayModel, replayCmd := a.pushHistoryReplayPane(scope, number, project, rec.Attributes)
	return replayModel, tea.Batch(switchCmd, replayCmd)
}

func historyPatentNumber(rec observability.Record) (domain.PatentNumber, bool) {
	if rec.Action == observability.ActionUIFocus {
		if number, err := domain.ParsePatentNumber(rec.EntityID); err == nil {
			return number, true
		}
	}
	if requested, ok := rec.Attributes["requested_number"].(string); ok && requested != "" {
		if number, err := domain.ParsePatentNumber(requested); err == nil {
			return number, true
		}
	}
	parts := strings.Split(rec.EntityID, "/")
	if len(parts) >= 2 {
		if number, err := domain.ParsePatentNumber(parts[1]); err == nil {
			return number, true
		}
	}
	return domain.PatentNumber{}, false
}

func (a *App) historyProjectID(rec observability.Record) domain.ProjectID {
	if project, ok := rec.Attributes[observability.AttrProject].(string); ok && project != "" {
		return domain.ProjectID(project)
	}
	if overlay.IsProjectPatentAction(rec.Action) {
		parts := strings.Split(rec.EntityID, "/")
		if len(parts) >= 1 && parts[0] != "" {
			return domain.ProjectID(parts[0])
		}
	}
	if a.activeProject != nil {
		return a.activeProject.ID
	}
	return ""
}

func (a *App) switchHistoryProject(project domain.ProjectID) tea.Cmd {
	if project == "" || (a.activeProject != nil && a.activeProject.ID == project) {
		return nil
	}
	proj, ok := a.resolveProjectArg(string(project))
	if !ok {
		return nil
	}
	a.activeProject = &proj
	a.lastProjectID = proj.ID
	if a.saveLastProject != nil {
		_ = a.saveLastProject(proj.ID)
	}
	a.setStatus(text.StatusActiveProject, proj.Name)
	return a.broadcast(pane.ProjectChangedMsg{Project: &proj})
}

func (a *App) closeHistoryReplayOverlays() {
	a.overlays = nil
	if len(a.panes) > 1 {
		a.panes = a.panes[:1]
	}
}

func (a *App) pushHistoryReplayPane(scope string, number domain.PatentNumber, project domain.ProjectID, attrs map[string]any) (tea.Model, tea.Cmd) {
	switch scope {
	case "citations":
		kind := domain.RelationCites
		if relation, ok := attrs[observability.AttrRelation].(string); ok {
			kind = domain.RelationKind(relation)
		}
		return a.pushPane(pane.NewCitations(a.client, a.theme, number, kind).WithLogger(a.log()))
	case "family":
		depth := 1
		if value, ok := attrs["depth"].(float64); ok {
			depth = int(value)
		}
		var countries []string
		if values, ok := attrs["countries"].([]any); ok {
			for _, value := range values {
				if country, ok := value.(string); ok {
					countries = append(countries, country)
				}
			}
		}
		return a.pushPane(pane.NewFamilyGraph(a.client, a.theme, number, depth, countries).WithLogger(a.log()))
	case "fulltext":
		bound := a.keymaps.BoundLetters(command.ScopeFullText)
		return a.pushPane(pane.NewFullText(a.client, a.theme, number, project, bound).WithLogger(a.log()))
	case "ids":
		return a.pushPane(pane.NewIDSDetail(a.client, a.theme, number, project).WithLogger(a.log()))
	default:
		bound := a.keymaps.BoundLetters(command.ScopeDetail)
		return a.pushPane(pane.NewDetail(a.client, a.theme, number, project, bound).WithLogger(a.log()))
	}
}

func (a *App) replayHistoryFilter(rec observability.Record, confirmed bool) (tea.Model, tea.Cmd) {
	if !confirmed {
		a.confirmCmd = func() tea.Msg { return overlay.ConfirmHistoryReplayMsg{Record: rec} }
		a.overlays = append(a.overlays, overlay.NewConfirm(a.theme, fmt.Sprintf("Apply filter '%s'?", rec.EntityID)))
		return a, nil
	}
	a.closeHistoryReplayOverlays()
	return a.executeTypedCommand("filter " + rec.EntityID)
}

func (a *App) replayHistoryProjectSwitch(rec observability.Record, confirmed bool) (tea.Model, tea.Cmd) {
	if !confirmed {
		a.confirmCmd = func() tea.Msg { return overlay.ConfirmHistoryReplayMsg{Record: rec} }
		projectName := rec.EntityID
		if name, ok := rec.Attributes[observability.AttrProjectName].(string); ok && name != "" {
			projectName = name
		}
		a.overlays = append(a.overlays, overlay.NewConfirm(a.theme, fmt.Sprintf("Switch to project '%s'?", projectName)))
		return a, nil
	}
	a.closeHistoryReplayOverlays()
	return a.activateProjectByArg(rec.EntityID)
}

func (a *App) replayHistoryOfficeAction(rec observability.Record) (tea.Model, tea.Cmd) {
	project := a.historyProjectID(rec)
	switchCmd := a.switchHistoryProject(project)
	a.closeHistoryReplayOverlays()
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, switchCmd
	}
	replayModel, replayCmd := a.pushPane(pane.NewOfficeActions(a.client, a.theme, project).WithLogger(a.log()))
	return replayModel, tea.Batch(switchCmd, replayCmd)
}

func (a *App) replayHistoryMatterDocument(rec observability.Record) (tea.Model, tea.Cmd) {
	project := a.historyProjectID(rec)
	switchCmd := a.switchHistoryProject(project)
	a.closeHistoryReplayOverlays()
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, switchCmd
	}
	o := overlay.NewMatterDocuments(a.client, a.theme, project, nil)
	a.overlays = append(a.overlays, o)
	return a, tea.Batch(switchCmd, o.Init())
}
