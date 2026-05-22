package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
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

func (a *App) cmdOpenIDS(invocation) (tea.Model, tea.Cmd) { return a.openIDS() }

func (a *App) cmdOpenProjects(invocation) (tea.Model, tea.Cmd) {
	return a.pushPane(pane.NewProjects(a.client, a.theme, a.activeAIString(), a.activeSearchString()))
}

func (a *App) cmdProjectClear(invocation) (tea.Model, tea.Cmd) {
	a.activeProject = nil
	a.setStatus(text.StatusClearedProject)
	return a, a.broadcast(pane.ProjectChangedMsg{})
}

func (a *App) cmdMarkStored(invocation) (tea.Model, tea.Cmd) {
	return a.runReviewState(command.MarkStored, domain.ReviewStateStored)
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

// cmdTagAdd tags the selected patent within the active project. The tag name
// is the typed argument; it may contain spaces.
func (a *App) cmdTagAdd(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.TagAdd)
	}
	name := strings.Join(inv.args, " ")
	return a.runAction(command.TagAdd, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
		return pane.AssignTagCmd(a.client, project, patent, name)
	})
}

// cmdTagRemove removes a tag from the selected patent within the active project.
func (a *App) cmdTagRemove(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.TagRemove)
	}
	name := strings.Join(inv.args, " ")
	return a.runAction(command.TagRemove, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
		return pane.RemoveTagCmd(a.client, project, patent, name)
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

// cmdTagPatentAdd assigns a tag to the selected patent within the active project.
func (a *App) cmdTagPatentAdd(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.TagPatentAdd)
	}
	name := strings.Join(inv.args, " ")
	return a.runAction(command.TagPatentAdd, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
		return pane.AssignPatentTagCmd(a.client, project, patent, name)
	})
}

// cmdTagPatentDelete removes a tag assignment from the selected patent within the active project.
func (a *App) cmdTagPatentDelete(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.TagPatentDelete)
	}
	name := strings.Join(inv.args, " ")
	return a.runAction(command.TagPatentDelete, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
		return pane.RemovePatentTagCmd(a.client, project, patent, name)
	})
}

// cmdTagPatentList lists all tags assigned to the selected patent.
func (a *App) cmdTagPatentList(inv invocation) (tea.Model, tea.Cmd) {
	return a.runAction(command.TagPatentList, func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
		return pane.ListPatentTagsCmd(a.client, project, patent)
	})
}

func (a *App) cmdPatentDelete(invocation) (tea.Model, tea.Cmd) {
	return a.runAction(command.PatentDelete, func(_ domain.ProjectID, n domain.PatentNumber) tea.Cmd {
		return pane.DeletePatentCmd(a.client, n)
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
