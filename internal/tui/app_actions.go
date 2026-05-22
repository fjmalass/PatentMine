package tui

import (
	"context"
	"encoding/json"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/text"
	"patentmine/internal/tui/overlay"
	"patentmine/internal/tui/pane"
)

// handleTextSubmit routes a value entered in a TextInput overlay to its action.
func (a *App) handleTextSubmit(m overlay.TextSubmitMsg) (tea.Model, tea.Cmd) {
	switch m.Purpose {
	case overlay.PurposeCreateProject:
		return a.createProject(m.Value)
	case overlay.PurposeAIAnalyzeCustom:
		a.setStatus(text.StatusAIAnalysisStarted, a.aiTargetPatent.Number.String(), string(a.aiProvider))
		return a, a.runAIAnalysis(a.aiTargetPatent, "custom", m.Value)
	case overlay.PurposeEditIDSKind:
		return a.applyIDSFieldEdit("kind", m.Value)
	case overlay.PurposeEditIDSCountry:
		return a.applyIDSFieldEdit("country", m.Value)
	case overlay.PurposeEditIDSPassages:
		return a.applyIDSFieldEdit("passages", m.Value)
	case overlay.PurposeEditIDSNotes:
		return a.applyIDSFieldEdit("notes", m.Value)
	default:
		return a, nil
	}
}

func (a *App) openIDSEditInput(field string) (tea.Model, tea.Cmd) {
	var (
		purpose overlay.Purpose
		title   text.Key
		caption text.Key
	)
	switch field {
	case "kind":
		purpose, title, caption = overlay.PurposeEditIDSKind, text.EditIDSKindTitle, text.EditIDSKindCaption
	case "country":
		purpose, title, caption = overlay.PurposeEditIDSCountry, text.EditIDSCountryTitle, text.EditIDSCountryCaption
	case "passages":
		purpose, title, caption = overlay.PurposeEditIDSPassages, text.EditIDSPassagesTitle, text.EditIDSPassagesCaption
	case "notes":
		purpose, title, caption = overlay.PurposeEditIDSNotes, text.EditIDSNotesTitle, text.EditIDSNotesCaption
	default:
		return a, nil
	}
	a.overlays = append(a.overlays, overlay.NewTextInput(a.theme, a.text, purpose, title, caption))
	return a, nil
}

func (a *App) applyIDSFieldEdit(field, value string) (tea.Model, tea.Cmd) {
	idsPane, ok := a.focusedPane().(interface{ ApplyTextValue(string, string) tea.Cmd })
	if !ok {
		return a, nil
	}
	return a, idsPane.ApplyTextValue(field, value)
}

// createProject sends a project.create request for name.
func (a *App) createProject(name string) (tea.Model, tea.Cmd) {
	name = strings.TrimSpace(name)
	if name == "" {
		a.setErr(text.StatusProjectNameEmpty)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a, pane.CreateProjectCmd(a.client, name)
}

// usageError reports the correct invocation of a command.
func (a *App) usageError(id command.ID) (tea.Model, tea.Cmd) {
	cmd, _ := a.registry.Lookup(id)
	usage := cmd.Usage
	if usage == "" {
		usage = ":" + cmd.Name
	}
	a.setErr(text.StatusUsage, usage)
	return a, nil
}

// --- pane stack --------------------------------------------------------------

// preparePane syncs a newly mounted pane with the current app state before its
// first render/load, so list panes size themselves from the live window rather
// than their constructor defaults.
func (a *App) preparePane(p pane.Pane) (pane.Pane, []tea.Cmd) {
	var cmds []tea.Cmd
	if a.width > 0 && a.height > 0 {
		updated, cmd := p.Update(pane.ResizeMsg{Width: a.width, Height: max(a.height-statusRows, 1)})
		p = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	updated, cmd := p.Update(pane.ProjectChangedMsg{Project: a.activeProject})
	p = updated
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	return p, cmds
}

// pushPane adds a pane to the stack and returns its init command.
func (a *App) pushPane(p pane.Pane) (tea.Model, tea.Cmd) {
	p, cmds := a.preparePane(p)
	a.panes = append(a.panes, p)
	if len(cmds) == 0 {
		cmds = append(cmds, p.Init())
	}
	return a, tea.Batch(cmds...)
}

// openDetail pushes a detail pane for the focused pane's selected patent. The
// active project, when set, scopes the detail's review state and tags.
func (a *App) openDetail() (tea.Model, tea.Cmd) {
	number, ok := a.focusedPane().Selection()
	if !ok {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	var project domain.ProjectID
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	bound := a.keymaps.BoundLetters(command.ScopeDetail)
	return a.pushPane(pane.NewDetail(a.client, a.theme, number, project, bound))
}

// openFullText pushes a full text viewer pane for the selected patent.
func (a *App) openFullText() (tea.Model, tea.Cmd) {
	number, ok := a.focusedPane().Selection()
	if !ok {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	var project domain.ProjectID
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	bound := a.keymaps.BoundLetters(command.ScopeFullText)
	return a.pushPane(pane.NewFullText(a.client, a.theme, number, project, bound))
}

func (a *App) openIDS() (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	number, ok := a.focusedPane().Selection()
	if !ok {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	return a.pushPane(pane.NewIDSDetail(a.client, a.theme, number, a.activeProject.ID))
}

// openCitations pushes a family-edge pane for the focused pane's selected
// patent, showing edges of the given kind.
func (a *App) openCitations(kind domain.RelationKind) (tea.Model, tea.Cmd) {
	number, ok := a.focusedPane().Selection()
	if !ok {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	return a.pushPane(pane.NewCitations(a.client, a.theme, number, kind))
}

func (a *App) activateProject() (tea.Model, tea.Cmd) {
	selector, ok := a.focusedPane().(interface{ SelectedProject() (domain.Project, bool) })
	if !ok {
		a.setErr(text.StatusNoProjectSelection)
		return a, nil
	}
	project, ok := selector.SelectedProject()
	if !ok {
		a.setErr(text.StatusNoProjectSelected)
		return a, nil
	}
	return a.useProject(project)
}

func (a *App) activateProjectByArg(arg string) (tea.Model, tea.Cmd) {
	project, ok := a.resolveProjectArg(arg)
	if !ok {
		a.setErr(text.StatusProjectNotFound, arg)
		return a, nil
	}
	return a.useProject(project)
}

// useProject makes project the active project and updates every pane.
func (a *App) useProject(project domain.Project) (tea.Model, tea.Cmd) {
	a.activeProject = &project
	a.lastProjectID = project.ID
	if a.saveLastProject != nil {
		if err := a.saveLastProject(project.ID); err != nil {
			a.setErr(text.StatusActiveProjectSaveErr, project.Name, err.Error())
		} else {
			a.setStatus(text.StatusActiveProject, project.Name)
		}
	} else {
		a.setStatus(text.StatusActiveProject, project.Name)
	}
	if splash, ok := a.focusedPane().(interface{ IsSplash() bool }); ok && splash.IsSplash() && len(a.panes) == 1 {
		var mounted pane.Pane = pane.NewCatalog(a.client, a.theme)
		mounted, cmds := a.preparePane(mounted)
		a.panes[0] = mounted
		if len(cmds) == 0 {
			cmds = append(cmds, mounted.Init())
		}
		return a, tea.Batch(cmds...)
	}
	if len(a.panes) > 1 {
		a.panes = a.panes[:len(a.panes)-1]
	}
	return a, a.broadcast(pane.ProjectChangedMsg{Project: &project})
}

func (a *App) resolveProjectArg(arg string) (domain.Project, bool) {
	if selector, ok := a.focusedPane().(*pane.Projects); ok {
		if project, found := selector.ProjectByArg(arg); found {
			return project, true
		}
	}
	if a.client == nil {
		return domain.Project{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	var res proto.ProjectListResult
	if err := a.client.Call(ctx, proto.MethodProjectList, nil, &res); err != nil {
		return domain.Project{}, false
	}
	needle := strings.TrimSpace(strings.ToLower(arg))
	for _, project := range res.Projects {
		if strings.ToLower(string(project.ID)) == needle || strings.ToLower(project.Name) == needle {
			return project, true
		}
	}
	return domain.Project{}, false
}

func (a *App) runReviewState(id command.ID, target domain.ReviewState) (tea.Model, tea.Cmd) {
	return a.runAction(id, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
		return pane.SetReviewStateCmd(a.client, project, patent, target)
	})
}

// runAction is the canonical dispatcher for single-patent and multi-patent
// project actions. It validates preconditions, loops over all focused selections,
// and applies any confirmation registered for id in commandPolicies.
func (a *App) runAction(
	id command.ID,
	build func(domain.ProjectID, domain.PatentNumber) tea.Cmd,
) (tea.Model, tea.Cmd) {
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
	if len(numbers) >= 2 {
		if vs, ok := a.focusedPane().(pane.VisualSelectionSaver); ok {
			vs.SaveVisualSelection()
		}
	}
	cmds := make([]tea.Cmd, 0, len(numbers))
	for _, n := range numbers {
		cmds = append(cmds, build(a.activeProject.ID, n))
	}
	cmd := tea.Batch(cmds...)

	if policy := commandPolicies[id]; policy.Confirm != nil {
		if msg, needs := policy.Confirm(numbers); needs {
			a.confirmCmd = cmd
			a.overlays = append(a.overlays, overlay.NewConfirm(a.theme, msg))
			return a, nil
		}
	}
	return a, cmd
}

func (a *App) focusedSelections() []domain.PatentNumber {
	if ms, ok := a.focusedPane().(pane.MultiSelector); ok {
		if sels := ms.Selections(); len(sels) >= 2 {
			return sels
		}
	}
	if number, ok := a.focusedPane().Selection(); ok {
		return []domain.PatentNumber{number}
	}
	return nil
}

func (a *App) runProjectAction(action func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd) (tea.Model, tea.Cmd) {
	return a.runAction("", action)
}

func (a *App) openPrompt(mode overlay.PromptMode) (tea.Model, tea.Cmd) {
	a.overlays = append(a.overlays, overlay.NewPrompt(a.registry, a.keymaps, a.theme, a.text, a.scope(), mode))
	return a, nil
}

func (a *App) scope() command.Scope {
	if len(a.overlays) > 0 {
		if source, ok := a.focusedOverlay().(overlay.ScopeSource); ok {
			return source.SourceScope()
		}
		return command.ScopeOverlay
	}
	return a.focusedPane().Scope()
}

// executeTypedCommand parses a typed command and routes it through invoke, the
// same path a key chord takes — so a command can never work one way and not the
// other.
func (a *App) executeTypedCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(strings.TrimSpace(input))
	if len(parts) == 0 {
		return a, nil
	}
	cmd, ok := a.registry.LookupName(parts[0])
	if !ok {
		a.setErr(text.StatusUnknownCommand, parts[0])
		return a, nil
	}
	if !cmd.AvailableIn(a.scope()) {
		a.setErr(text.StatusCommandNotHere, cmd.Name)
		return a, nil
	}
	args := parts[1:]
	if len(args) > 0 && !typedAcceptsArgs[cmd.ID] {
		return a.usageError(cmd.ID)
	}
	return a.invoke(cmd.ID, invocation{repeat: 1, args: args})
}

// handleEvent reflects a daemon event into the status line and refreshes data.
func (a *App) handleEvent(ev proto.Event) tea.Cmd {
	switch ev.Method {
	case proto.EventCrawlProgress:
		var p proto.CrawlProgress
		if json.Unmarshal(ev.Params, &p) == nil {
			a.setStatus(text.StatusCrawlProgress, p.JobID, p.CrawledCount, p.DiscoveredCount, p.Message)
		}
		return nil
	case proto.EventCrawlDone:
		var d proto.CrawlDone
		_ = json.Unmarshal(ev.Params, &d)
		if d.Error != "" {
			a.setErr(text.StatusCrawlFailed, d.JobID, d.Error)
		} else {
			a.setStatus(text.StatusCrawlComplete, d.JobID)
		}
		return a.refreshPanes()
	case proto.EventDBChanged:
		return a.refreshPanes()
	}
	return nil
}

// refreshPanes asks every pane to reload from the daemon. Panes cache their own
// view state, so a store mutation must invalidate the whole stack rather than
// only the currently visible pane.
func (a *App) refreshPanes() tea.Cmd {
	var cmds []tea.Cmd
	for i, p := range a.panes {
		updated, cmd := p.Command(command.Refresh, pane.Invocation{Repeat: 1})
		a.panes[i] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

func (a *App) focusedPane() pane.Pane          { return a.panes[len(a.panes)-1] }
func (a *App) focusedOverlay() overlay.Overlay { return a.overlays[len(a.overlays)-1] }

func (a *App) popOverlay() {
	if n := len(a.overlays); n > 0 {
		if _, ok := a.overlays[n-1].(*overlay.Jump); ok {
			if setter, ok := a.focusedPane().(interface{ SetJumpActive(bool) }); ok {
				setter.SetJumpActive(false)
			}
		}
		a.overlays = a.overlays[:n-1]
	}
}

// setStatus and setErr resolve a catalog key into the status line.
func (a *App) setStatus(key text.Key, args ...any) {
	a.status, a.statusErr = a.text.Tf(key, args...), false
}

func (a *App) setErr(key text.Key, args ...any) {
	a.status, a.statusErr = a.text.Tf(key, args...), true
}
