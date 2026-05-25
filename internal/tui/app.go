// Package tui is the terminal client. The App is the bubbletea root: it owns
// the pane and overlay stacks and routes input, but holds no business state —
// every screen's state lives in its own Pane or Overlay. That decomposition is
// the deliberate structural defence against a god-object UI model.
//
// Input flows through one path. A key chord and a typed command both resolve
// to a command.ID and run through invoke, so the two can never diverge. Every
// command.ID is serviced by exactly one handler — an entry in appHandlers or a
// pane/overlay that lists the ID in Handles — and validateWiring fails the boot
// if any bound key or typed command would resolve to an unhandled ID.
package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/ai"
	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/keys"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/keymap"
	"patentmine/internal/tui/overlay"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
	appversion "patentmine/internal/version"
)

// statusRows is the bottom status line the App always draws.
const statusRows = 1

// pingTimeout bounds the startup version probe.
const pingTimeout = 5 * time.Second

// overlay box sizing.
const (
	overlayMaxWidth  = 76
	overlayMaxHeight = 22
	overlayMargin    = 4 // free space kept around the box
	overlayChrome    = 4 // border (2) + horizontal padding (2)
)

// busEventMsg carries a daemon event into the bubbletea update loop.
type busEventMsg struct{ event proto.Event }

// eventsClosedMsg signals that the daemon event stream ended.
type eventsClosedMsg struct{}

// pingLoadedMsg carries the daemon's reported build version.
type pingLoadedMsg struct {
	version         string
	usptoConfigured bool
	err             error
}

type historyCheckResultMsg struct {
	targetIndex int
	number      domain.PatentNumber
	exists      bool
	err         error
}

// invocation carries the arguments of one command request: empty for a key
// chord, populated for a typed command.
type invocation struct {
	repeat int
	args   []string
}

// appHandler services one command at the App level. appHandlers is the single
// table of App-handled commands; validateWiring reads it to prove every bound
// key reaches a handler.
type appHandler func(*App, invocation) (tea.Model, tea.Cmd)

var appHandlers = map[command.ID]appHandler{
	command.Quit:                       (*App).cmdQuit,
	command.Help:                       (*App).cmdHelp,
	command.OpenSearch:                 (*App).cmdOpenSearch,
	command.OpenCommand:                (*App).cmdOpenCommand,
	command.OpenMetrics:                (*App).cmdOpenMetrics,
	command.OpenPatentNote:             (*App).cmdOpenPatentNote,
	command.JumpMode:                   (*App).cmdJumpMode,
	command.CloseOverlay:               (*App).cmdCloseOverlay,
	command.Back:                       (*App).cmdBack,
	command.OpenDetail:                 (*App).cmdOpenDetail,
	command.OpenFullText:               (*App).cmdOpenFullText,
	command.OpenBrowser:                (*App).cmdOpenBrowser,
	command.OpenCitations:              (*App).cmdOpenCitations,
	command.OpenCitedBy:                (*App).cmdOpenCitedBy,
	command.OpenParents:                (*App).cmdOpenParents,
	command.OpenChildren:               (*App).cmdOpenChildren,
	command.OpenFamilyGraph:            (*App).cmdOpenFamilyGraph,
	command.OpenIDS:                    (*App).cmdOpenIDS,
	command.OpenProjects:               (*App).cmdOpenProjects,
	command.ProjectActivate:            (*App).cmdProjectActivate,
	command.ProjectClearActive:         (*App).cmdProjectClear,
	command.ProjectCreate:              (*App).cmdProjectCreate,
	command.ProjectIDSHeader:           (*App).cmdEditIDSHeader,
	command.IDSExportPDF:               (*App).cmdIDSExportPDF,
	command.AddToProject:               (*App).cmdAddToProject,
	command.Import:                     (*App).cmdImport,
	command.CrawlDepthMax:              (*App).cmdCrawlDepthMax,
	command.MarkActive:                 (*App).cmdMarkActive,
	command.MarkUnderReview:            (*App).cmdMarkUnderReview,
	command.MarkIgnored:                (*App).cmdMarkIgnored,
	command.MarkDeleted:                (*App).cmdMarkDeleted,
	command.Tag:                        (*App).cmdTag,
	command.Untag:                      (*App).cmdUntag,
	command.TagTaxonomyAdd:             (*App).cmdTagTaxonomyAdd,
	command.TagTaxonomyList:            (*App).cmdTagTaxonomyList,
	command.TagPatentManage:            (*App).cmdTagPatentManage,
	command.TagTaxonomyDelete:          (*App).cmdTagTaxonomyDelete,
	command.TagStrict:                  (*App).cmdTagStrict,
	command.UntagStrict:                (*App).cmdUntagStrict,
	command.TagPatentList:              (*App).cmdTagPatentList,
	command.ClassificationTaxonomyList: (*App).cmdClassificationTaxonomyList,
	command.ClassificationLookup:       (*App).cmdClassificationLookup,
	command.OpenPatentClassifications:  (*App).cmdOpenPatentClassifications,
	command.PatentDelete:               (*App).cmdPatentDelete,
	command.IDSCycleStatus:             (*App).cmdIDSCycleStatus,
	command.AIAnalyze:                  (*App).cmdAIAnalyze,
	command.SettingsAI:                 (*App).cmdSettingsAI,
	command.OpenAssignees:              (*App).cmdOpenAssignees,
	command.OpenClassificationStats:    (*App).cmdOpenClassificationStats,
	command.OpenInventors:              (*App).cmdOpenInventors,
	command.OpenInventorsDirect:        (*App).cmdOpenInventorsDirect,
	command.HistoryBack:                (*App).cmdHistoryBack,
	command.HistoryForward:             (*App).cmdHistoryForward,
	command.OpenHistory:                (*App).cmdOpenHistory,
	command.OpenActivity:               (*App).cmdOpenActivity,
	command.OpenAllNotes:               (*App).cmdOpenAllNotes,
}

// typedAcceptsArgs lists the commands whose typed form takes arguments. Every
// other typed command is rejected with a usage error when given any.
var typedAcceptsArgs = map[command.ID]bool{
	command.AddToProject:         true,
	command.ProjectActivate:      true,
	command.ProjectCreate:        true,
	command.Import:               true,
	command.Tag:                  true,
	command.Untag:                true,
	command.TagTaxonomyAdd:       true,
	command.TagTaxonomyDelete:    true,
	command.TagStrict:            true,
	command.UntagStrict:          true,
	command.ClassificationLookup: true,
	command.Filter:               true,
	command.OpenBrowser:          true,
}

// App is the bubbletea root model.
type App struct {
	client          *rpc.Client
	registry        *command.Registry
	keymaps         *keymap.Keymaps
	hints           *keymap.HintCatalog
	theme           render.Theme
	text            *text.Catalog
	reader          keys.Reader
	saveLastProject func(domain.ProjectID) error

	panes      []pane.Pane
	overlays   []overlay.Overlay
	confirmCmd tea.Cmd // pending action awaiting confirmation

	history       []domain.PatentNumber
	historyCursor int

	status        string
	statusErr     bool
	width         int
	height        int
	activeProject *domain.Project
	lastProjectID domain.ProjectID
	tuiVersion    string
	daemonVersion string
	openURL       func(string) error
	notes         *notesAccumulator
	logger        *slog.Logger
	activity      *observability.Recorder
	activityDir   string
	activityMin   time.Duration
	metrics       *observability.Metrics
	focusKey      string
	focusStarted  time.Time
	focusRecorded bool

	aiProvider      ai.Provider
	geminiAPIKey    string
	ollamaHost      string
	ollamaModel     string
	aiTargetPatent  domain.Patent
	usptoConfigured bool
	geminiAnalyzer  ai.Analyzer
	ollamaAnalyzer  ai.Analyzer

	notesExportDir string
}

type Option func(*App)

func WithLastProject(id domain.ProjectID) Option {
	return func(a *App) { a.lastProjectID = id }
}

func WithLastProjectSaver(save func(domain.ProjectID) error) Option {
	return func(a *App) { a.saveLastProject = save }
}

func WithAIConfig(provider string, geminiKey string, ollamaHost string, ollamaModel string) Option {
	return func(a *App) {
		a.aiProvider = ai.Provider(provider)
		a.geminiAPIKey = geminiKey
		a.ollamaHost = ollamaHost
		a.ollamaModel = ollamaModel
		a.geminiAnalyzer = ai.NewGeminiAnalyzer(geminiKey)
		a.ollamaAnalyzer = ai.NewOllamaAnalyzer(ollamaHost, ollamaModel)
	}
}

// buildAnalyzer returns the cached analyzer for the active provider.
func (a *App) buildAnalyzer() ai.Analyzer {
	if a.aiProvider == ai.ProviderGemini {
		if a.geminiAnalyzer == nil {
			a.geminiAnalyzer = ai.NewGeminiAnalyzer(a.geminiAPIKey)
		}
		return a.geminiAnalyzer
	}
	if a.ollamaAnalyzer == nil {
		a.ollamaAnalyzer = ai.NewOllamaAnalyzer(a.ollamaHost, a.ollamaModel)
	}
	return a.ollamaAnalyzer
}

func WithUSPTOKey(key string) Option {
	return func(a *App) {
		if key != "" {
			a.usptoConfigured = true
		}
	}
}

func (a *App) activeAIString() string {
	switch a.aiProvider {
	case ai.ProviderGemini:
		return "AI: Gemini"
	case ai.ProviderOllama:
		return "AI: Ollama"
	default:
		return "AI: None"
	}
}

func (a *App) activeSearchString() string {
	if a.usptoConfigured {
		return "Search: Google, USPTO"
	}
	return "Search: Google"
}

// WithTelemetry attaches the process observability runtime so the App can log
// structured events and record metrics for user-facing actions.
func WithTelemetry(rt *observability.Runtime) Option {
	return func(a *App) {
		if rt == nil {
			return
		}
		a.logger = rt.Logger
		a.activity = rt.Activity
		a.activityDir = rt.LogsDir
		a.metrics = rt.Metrics
	}
}

// WithActivityMinDuration sets the minimum focused/hover duration before the
// TUI writes an activity record. Values <= 0 keep the default 100ms threshold.
func WithActivityMinDuration(d time.Duration) Option {
	return func(a *App) {
		if d > 0 {
			a.activityMin = d
		}
	}
}

// WithNotesExportDir sets the directory for exported notes .md files.
// Empty string falls back to the user's home directory.
func WithNotesExportDir(dir string) Option {
	return func(a *App) { a.notesExportDir = dir }
}

// log returns the App's structured logger, or the process default when no
// telemetry runtime was attached.
func (a *App) log() *slog.Logger {
	if a.logger != nil {
		return a.logger
	}
	return slog.Default()
}

// New builds the App with the splash/project selector as the initial pane. It
// fails when the keymap, command registry, and handlers are not consistent —
// see validateWiring — so a wiring mistake is caught at startup.
func New(client *rpc.Client, registry *command.Registry, keymaps *keymap.Keymaps, catalog *text.Catalog, opts ...Option) (*App, error) {
	if err := validateWiring(registry, keymaps, catalog); err != nil {
		return nil, err
	}
	theme := render.NewTheme()
	hints, err := keymap.NewHintCatalog(registry, keymap.DefaultHints())
	if err != nil {
		return nil, err
	}
	app := &App{
		client:        client,
		registry:      registry,
		keymaps:       keymaps,
		hints:         hints,
		theme:         theme,
		text:          catalog,
		tuiVersion:    appversion.String(),
		daemonVersion: "connecting",
		openURL:       openExternalURL,
		notes:         newNotesAccumulator(),
		activityMin:   100 * time.Millisecond,
	}
	app.status = catalog.T(text.StatusWelcome)
	for _, opt := range opts {
		opt(app)
	}
	app.panes = []pane.Pane{pane.NewSplash(client, theme, app.lastProjectID,
		app.splashFooterHint(), app.splashEmptyHint(), app.activeAIString(), app.activeSearchString())}
	return app, nil
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{
		a.panes[0].Init(),
		a.fetchPing(),
		func() tea.Msg {
			return pane.ServiceStatusChangedMsg{
				ActiveAI:     a.activeAIString(),
				ActiveSearch: a.activeSearchString(),
			}
		},
	}
	if a.client != nil {
		cmds = append(cmds, a.listen())
	}
	return tea.Batch(cmds...)
}

// listen waits for one daemon event and delivers it as a message.
func (a *App) listen() tea.Cmd {
	client := a.client
	return func() tea.Msg {
		ev, ok := <-client.Events()
		if !ok {
			return eventsClosedMsg{}
		}
		return busEventMsg{event: ev}
	}
}

// fetchPing asks the daemon which build version it is running.
func (a *App) fetchPing() tea.Cmd {
	if a.client == nil {
		return nil
	}
	client := a.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()
		var res proto.PingResult
		err := client.Call(ctx, proto.MethodPing, nil, &res)
		return pingLoadedMsg{version: res.Version, usptoConfigured: res.USPTOConfigured, err: err}
	}
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case idsHeaderSavedMsg:
		saved := m.project
		a.activeProject = &saved
		a.setStatus(text.StatusUsage, "IDS header saved")
		return a, nil
	case aiPatentLoadedMsg:
		if m.err != nil {
			a.setErr(text.StatusAIAnalysisFailed, m.err.Error())
			return a, nil
		}
		a.aiTargetPatent = m.patent
		o := overlay.NewAIMenu(a.theme, m.patent, a.aiProvider, a.buildAnalyzer())
		a.overlays = append(a.overlays, o)
		return a, nil
	case overlay.AISwitchProviderMsg:
		a.aiProvider = ai.Provider(m.NewProvider)
		if len(a.overlays) > 0 {
			if menu, ok := a.overlays[len(a.overlays)-1].(*overlay.AIMenu); ok {
				a.overlays[len(a.overlays)-1] = overlay.NewAIMenu(a.theme, menu.Patent(), a.aiProvider, a.buildAnalyzer())
			} else if settings, ok := a.overlays[len(a.overlays)-1].(*overlay.SettingsOverlay); ok {
				settings.SetActiveAI(a.aiProvider)
			}
		}
		return a, a.broadcast(pane.ServiceStatusChangedMsg{
			ActiveAI:     a.activeAIString(),
			ActiveSearch: a.activeSearchString(),
		})
	case overlay.AIAnalyzeTriggerMsg:
		a.popOverlay()
		a.setStatus(text.StatusAIAnalysisStarted, m.Patent.Number.String(), string(a.aiProvider))
		return a, a.runAIAnalysis(m.Patent, m.ActionType, m.CustomPrompt)
	case aiAnalysisResultMsg:
		if m.err != nil {
			a.setErr(text.StatusAIAnalysisFailed, m.err.Error())
			return a, nil
		}
		a.notes.Add(m.number, m.locator, time.Now(), m.result)
		a.setStatus(text.StatusAIAnalysisComplete, m.number.String())
		a.overlays = append(a.overlays, newNotesBufferOverlay(a.theme, m.number, m.patent, a))
		return a, nil
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		return a, tea.Batch(
			a.broadcast(pane.ResizeMsg{Width: m.Width, Height: max(m.Height-statusRows, 1)}),
			a.broadcastOverlays(m),
		)
	case tea.KeyMsg:
		updated, cmd := a.handleKey(m)
		return updated, tea.Batch(cmd, a.observeFocus())
	case focusDwellMsg:
		return a, a.handleFocusDwell(m)
	case overlay.ConfirmAcceptMsg:
		a.popOverlay()
		cmd := a.confirmCmd
		a.confirmCmd = nil
		return a, cmd
	case overlay.ConfirmRejectMsg:
		a.popOverlay()
		a.confirmCmd = nil
		return a, nil
	case overlay.PromptSubmitMsg:
		a.popOverlay()
		return a.executeTypedCommand(m.Input)
	case overlay.PromptCloseMsg:
		a.popOverlay()
		return a, nil
	case overlay.TextSubmitMsg:
		a.popOverlay()
		return a.handleTextSubmit(m)
	case overlay.IDSHeaderSubmitMsg:
		a.popOverlay()
		return a.handleIDSHeaderSubmit(m.Project)
	case idsExportPreviewedMsg:
		return a.handleIDSExportPreviewed(m)
	case overlay.IDSExportSubmitMsg:
		a.popOverlay()
		return a.handleIDSExportSubmit(m)
	case pane.EditIDSFieldMsg:
		return a.openIDSEditInput(m.Field)
	case pane.SearchAppliedMsg:
		var projectID domain.ProjectID
		if a.activeProject != nil {
			projectID = a.activeProject.ID
		}
		return a, a.recordActivity(observability.Record{
			Action:   observability.ActionFilterApply,
			Entity:   "filter",
			EntityID: m.Query,
			Status:   "requested",
			Metadata: map[string]any{
				"search":  m.Query,
				"project": projectID,
			},
		})
	case overlay.JumpSelectMsg:
		a.popOverlay()
		if provider, ok := a.focusedPane().(pane.JumpProvider); ok {
			provider.JumpTo(m.Line)
		}
		return a, nil
	case historyCheckResultMsg:
		if m.err != nil || !m.exists {
			isNotFound := false
			var rpcErr *proto.Error
			if errors.As(m.err, &rpcErr) {
				if rpcErr.Code == proto.CodeNotFound {
					isNotFound = true
				}
			}
			if isNotFound {
				if m.targetIndex >= 0 && m.targetIndex < len(a.history) {
					a.history = append(a.history[:m.targetIndex], a.history[m.targetIndex+1:]...)
					if a.historyCursor >= len(a.history) {
						a.historyCursor = len(a.history) - 1
					}
				}
			}
			var errMessage string
			if isNotFound {
				errMessage = fmt.Sprintf("Patent %s is no longer available in the database (it may have been deleted).", m.number.String())
			} else if m.err != nil {
				errMessage = fmt.Sprintf("Failed to load patent %s: %s", m.number.String(), m.err.Error())
			} else {
				errMessage = fmt.Sprintf("Patent %s is not available.", m.number.String())
			}
			a.overlays = append(a.overlays, overlay.NewErrorOverlay(a.theme, errMessage))
			return a, nil
		}
		a.historyCursor = m.targetIndex
		return a.navigateHistory(m.number)
	case overlay.HistoryReplayMsg:
		a.popOverlay()
		return a.handleHistoryReplay(m.Record, false)
	case overlay.HistoryFilterAppliedMsg:
		if a.activity == nil {
			return a, nil
		}
		query := strings.TrimSpace(m.Query)
		metadata := observability.TableFilter{
			Source:      "tui.history_overlay",
			TableType:   string(domain.TableIDSActivityHistory),
			Search:      query,
			SearchTerms: len(strings.Fields(query)),
		}.Metadata()
		metadata["sort_ascending"] = m.SortAscending
		metadata["result_count"] = m.ResultCount
		metadata["total_count"] = m.TotalCount
		return a, a.recordActivity(observability.Record{
			Action:   observability.ActionTableFilterApply,
			Entity:   "table_filter",
			EntityID: string(domain.TableIDSActivityHistory),
			Status:   "requested",
			Metadata: metadata,
		})
	case overlay.ConfirmHistoryReplayMsg:
		return a.handleHistoryReplay(m.Record, true)
	case overlay.ReplayActivityMsg:
		a.popOverlay()
		var project domain.ProjectID
		if a.activeProject != nil {
			project = a.activeProject.ID
		}
		bound := a.keymaps.BoundLetters(command.ScopeDetail)
		updated, cmd := a.pushPane(pane.NewDetail(a.client, a.theme, m.Number, project, bound).WithLogger(a.log()))
		return updated, tea.Batch(cmd, a.recordReplayHistory(m.Record))
	case overlay.CloseOverlayMsg:
		a.popOverlay()
		return a, nil
	case overlay.OpenPatentDetailMsg:
		a.popOverlay()
		var project domain.ProjectID
		if a.activeProject != nil {
			project = a.activeProject.ID
		}
		bound := a.keymaps.BoundLetters(command.ScopeDetail)
		return a.pushPane(pane.NewDetail(a.client, a.theme, m.Number, project, bound).WithLogger(a.log()))
	case overlay.OpenTagPatentOverlayMsg:
		o, cmd := overlay.NewTagPatentOverlay(a.client, a.theme, a.text, a.activeProject.ID, m.Patents)
		a.overlays = append(a.overlays, o)
		return a, cmd
	case overlay.ApplyClassificationFilterMsg:
		if target, ok := a.focusedPane().(pane.ClassificationFilterTarget); ok {
			return a, target.ApplyClassificationFilter(m.Codes)
		}
		return a, nil
	case pane.ExportNotesPreparedMsg:
		a.confirmCmd = m.WriteCmd
		a.overlays = append(a.overlays, overlay.NewExportNotesConfirm(a.theme, m.Path, m.ExistingFiles))
		a.metrics.IncCounter("tui.notes.export.initiated", 1)
		a.log().Info("notes export initiated",
			slog.String("path", m.Path),
			slog.Int("existing_files", len(m.ExistingFiles)))
		return a, nil
	case pane.StatusMsg:
		a.status, a.statusErr = a.text.Tf(m.Key, m.Args...), m.Error
		if m.Key == text.StatusNotesExportDone {
			count, _ := m.Args[0].(int)
			path, _ := m.Args[1].(string)
			a.metrics.IncCounter("tui.notes.export.ok", 1)
			a.log().Info("notes exported", slog.Int("count", count), slog.String("path", path))
			var projectID domain.ProjectID
			var projectName string
			if a.activeProject != nil {
				projectID = a.activeProject.ID
				projectName = a.activeProject.Name
			}
			return a, a.recordActivity(observability.Record{
				Action:   observability.ActionNotesExport,
				Entity:   "project",
				EntityID: string(projectID),
				Status:   "done",
				Metadata: map[string]any{
					"count":        count,
					"path":         path,
					"project_name": projectName,
				},
			})
		} else if m.Key == text.StatusNotesExportFailed {
			a.metrics.IncCounter("tui.notes.export.error", 1)
			if len(m.Args) > 0 {
				a.log().Error("notes export failed", slog.String("error", fmt.Sprint(m.Args[0])))
			}
		}
		if m.Key == text.StatusCrawlStarted && len(m.Args) >= 2 {
			jobID, _ := m.Args[1].(string)
			isLookup := len(m.Args) >= 3
			if d, ok := m.Args[2].(int); ok && d == 0 {
				isLookup = true
			}
			verb := "Crawling"
			if isLookup {
				verb = "Looking up"
			}
			title := verb + " " + m.Args[0].(string)
			loading := overlay.NewLoading(a.theme, []string{jobID}, title, isLookup)
			a.overlays = append(a.overlays, loading)
			return a, loading.Init()
		}
		return a, nil
	case pane.ReviewStateChangedMsg:
		if len(m.Patents) == 1 {
			a.setStatus(text.StatusSetState, m.Patents[0].String(), string(m.State), string(m.Project))
		} else {
			a.setStatus(text.StatusSetState, fmt.Sprintf("%d patents", len(m.Patents)), string(m.State), string(m.Project))
		}
		return a, a.broadcast(m)
	case pane.IDSEntrySavedMsg:
		if m.Err != nil {
			a.setErr(text.StatusExportFailed, m.Err.Error())
			return a, nil
		}
		a.setStatus(text.StatusFilter, "IDS updated")
		a.log().Info("tui.ids_entry.broadcast.save",
			slog.String("project_id", string(m.Project)),
			slog.String("patent", m.Entry.Patent.String()),
			slog.String("status", string(m.Entry.Status)))

		entryCopy := m.Entry
		return a, a.broadcast(pane.IDSEntryChangedMsg{
			Project: m.Project,
			Patent:  m.Entry.Patent,
			Entry:   &entryCopy,
		})
	case pane.IDSEntryDeletedMsg:
		if m.Err != nil {
			a.setErr(text.StatusExportFailed, m.Err.Error())
			return a, nil
		}
		a.setStatus(text.StatusFilter, "IDS entry removed")
		a.log().Info("tui.ids_entry.broadcast.delete",
			slog.String("project_id", string(m.Project)),
			slog.String("patent", m.Patent.String()))
		return a, a.broadcast(pane.IDSEntryChangedMsg{
			Project: m.Project,
			Patent:  m.Patent,
			Entry:   nil,
		})
	case pane.IDSEntryChangedMsg:
		var statusStr string
		if m.Entry != nil {
			statusStr = string(m.Entry.Status)
		} else {
			statusStr = "deleted"
		}
		a.log().Info("tui.ids_entry.broadcast",
			slog.String("project_id", string(m.Project)),
			slog.String("patent", m.Patent.String()),
			slog.String("status", statusStr))
		return a, a.broadcast(m)
	case overlay.IDSSetStatusMsg:
		a.popOverlay()
		if a.activeProject == nil {
			a.setErr(text.StatusNoActiveProject)
			return a, nil
		}
		if a.client == nil {
			a.setErr(text.StatusDaemonUnavailable)
			return a, nil
		}
		a.metrics.IncCounter("tui.ids_entry.bulk_set_status.requested", 1)
		a.log().Info("tui.ids_entry.bulk_set_status.requested",
			slog.String("project_id", string(a.activeProject.ID)),
			slog.String("status", string(m.Status)),
			slog.Int("patents", len(m.Patents)))
		return a, pane.BulkSetIDSStatusCmd(a.client, a.activeProject.ID, m.Patents, m.Status)
	case pane.IDSEntriesChangedMsg:
		if m.Err != nil {
			a.metrics.IncCounter("tui.ids_entry.bulk_set_status.error", 1)
			a.setErr(text.StatusIDSUpdateFailed, m.Err.Error())
			a.log().Error("tui.ids_entry.bulk_set_status.failed",
				slog.Int("entries", len(m.Entries)),
				slog.String("error", m.Err.Error()))
			return a, nil
		}
		a.metrics.IncCounter("tui.ids_entry.bulk_set_status.ok", 1)
		a.metrics.IncCounter("tui.ids_entry.bulk_set_status.entries", int64(len(m.Entries)))
		a.setStatus(text.StatusIDSCycled, len(m.Entries))
		a.log().Info("tui.ids_entry.bulk_set_status.applied",
			slog.Int("entries", len(m.Entries)))
		return a, a.broadcast(m)
	case pane.MultiCrawlStartedMsg:
		isLookup := m.Depth == 0
		verb := "Crawling"
		if isLookup {
			verb = "Looking up"
		}
		title := fmt.Sprintf("%s %d patents", verb, len(m.Numbers))
		loading := overlay.NewLoading(a.theme, m.JobIDs, title, isLookup)
		a.overlays = append(a.overlays, loading)
		return a, loading.Init()
	case pingLoadedMsg:
		if m.err != nil {
			a.daemonVersion = "unavailable"
			return a, nil
		}
		a.daemonVersion = m.version
		a.usptoConfigured = m.usptoConfigured
		return a, a.broadcast(pane.ServiceStatusChangedMsg{
			ActiveAI:     a.activeAIString(),
			ActiveSearch: a.activeSearchString(),
		})
	case pane.FullTextLoadedMsg:
		failed := m.Err != nil
		a.metrics.ObserveDuration("tui.fulltext.fetch", m.Duration, failed)
		if failed {
			a.log().Error("full text fetch failed",
				slog.String("number", m.Number.String()),
				slog.Int64("duration_ms", m.Duration.Milliseconds()),
				slog.String("error", m.Err.Error()))
		} else {
			claims, paragraphs := 0, 0
			if m.FullText != nil {
				claims, paragraphs = len(m.FullText.Claims), len(m.FullText.Paragraphs)
			}
			a.log().Info("full text fetched",
				slog.String("number", m.Number.String()),
				slog.Int("claims", claims),
				slog.Int("paragraphs", paragraphs),
				slog.Int64("duration_ms", m.Duration.Milliseconds()))
		}
		return a, tea.Batch(a.broadcast(msg), a.broadcastOverlays(msg))
	case pane.CopyToClipboardMsg:
		if err := copyToClipboard(m.Text); err != nil {
			a.metrics.IncCounter("tui.clipboard.copy.error", 1)
			a.log().Error("clipboard copy failed", slog.String("error", err.Error()))
			a.setErr(text.StatusClipboardFailed, err.Error())
		} else {
			a.metrics.IncCounter("tui.clipboard.copy.ok", 1)
			a.log().Info("copied to clipboard", slog.Int("bytes", len(m.Text)))
			a.setStatus(text.StatusCopiedToClipboard, len(m.Text))
		}
		return a, nil
	case pane.NoteAddMsg:
		if a.notes.Add(m.Number, m.Locator, m.CapturedAt, m.Text) {
			a.metrics.IncCounter("tui.notes.add", 1)
			a.log().Info("note added",
				slog.String("number", m.Number.String()),
				slog.String("locator", m.Locator))
			a.setStatus(text.StatusNotesAdded, m.Locator)
		}
		return a, nil
	case pane.NoteOpenMsg:
		a.overlays = append(a.overlays, newNotesBufferOverlay(a.theme, m.Number, m.Patent, a))
		return a, nil
	case pane.PatentNoteOpenMsg:
		if a.client == nil {
			a.setErr(text.StatusDaemonUnavailable)
			return a, nil
		}
		if a.activeProject == nil {
			a.setErr(text.StatusNoActiveProject)
			return a, nil
		}
		o := overlay.NewPatentNoteEditor(a.client, a.theme, a.activeProject.ID, m.Number)
		a.overlays = append(a.overlays, o)
		return a, o.Init()
	case busEventMsg:
		return a, tea.Batch(
			a.handleEvent(m.event),
			a.broadcastOverlays(m.event),
			a.listen())
	case eventsClosedMsg:
		a.setErr(text.StatusDaemonClosed)
		return a, nil
	default:
		// rpc results, spinner ticks and the like — let every pane/overlay consume what is theirs.
		return a, tea.Batch(a.broadcast(msg), a.broadcastOverlays(msg), a.observeFocus())
	}
}

// broadcast forwards a message to every pane.
func (a *App) broadcast(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for i, p := range a.panes {
		updated, cmd := p.Update(msg)
		a.panes[i] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// broadcastOverlays forwards a message to every overlay.
func (a *App) broadcastOverlays(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for i, ov := range a.overlays {
		if u, ok := ov.(interface {
			Update(tea.Msg) (overlay.Overlay, tea.Cmd)
		}); ok {
			updated, cmd := u.Update(msg)
			a.overlays[i] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}
	return tea.Batch(cmds...)
}

// handleKey feeds a key press through the chord reader and keymap.
func (a *App) handleKey(m tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(a.overlays) > 0 {
		if handler, ok := a.focusedOverlay().(overlay.KeyHandler); ok {
			updated, cmd, consumed := handler.HandleKey(m)
			a.overlays[len(a.overlays)-1] = updated
			if consumed {
				return a, cmd
			}
		}
	}
	if len(a.overlays) == 0 {
		if interceptor, ok := a.focusedPane().(pane.KeyHandler); ok {
			updated, cmd, consumed := interceptor.HandleKey(m)
			a.panes[len(a.panes)-1] = updated
			if consumed {
				return a, cmd
			}
		}
	}
	stack := a.keyStack()
	chord, ok := a.reader.Feed(keys.Key(m.String()), stack.Match)
	if !ok {
		return a, nil // buffered as part of a longer sequence
	}
	id, ok := stack.Resolve(chord.Sequence())
	if !ok {
		return a, nil // unbound sequence
	}
	return a.invoke(id, invocation{repeat: chord.Repeat()})
}

// keyStack composes the active keymap. With an overlay open the pane layer is
// deliberately left out so only the overlay and global bindings apply.
func (a *App) keyStack() *keymap.Stack {
	stack := keymap.NewStack(a.keymaps.Base())
	var scope command.Scope
	if len(a.overlays) > 0 {
		scope = command.ScopeOverlay
	} else {
		scope = a.focusedPane().Scope()
	}
	if layer := a.keymaps.Scope(scope); layer != nil {
		stack.Push(layer)
	}
	return stack
}

// invoke carries out a resolved command. App-level commands run from the
// appHandlers table; everything else is forwarded to the focused overlay or
// pane — but only when that overlay or pane lists the command in Handles, so a
// command can never be silently dropped.
func (a *App) invoke(id command.ID, inv invocation) (tea.Model, tea.Cmd) {
	if handler, ok := appHandlers[id]; ok {
		return handler(a, inv)
	}
	if len(a.overlays) > 0 {
		ov := a.focusedOverlay()
		if !slices.Contains(ov.Handles(), id) {
			return a.unhandled(id)
		}
		updated, cmd := ov.Command(id, inv.repeat)
		a.overlays[len(a.overlays)-1] = updated
		return a, cmd
	}
	p := a.focusedPane()
	if !slices.Contains(p.Handles(), id) {
		return a.unhandled(id)
	}
	updated, cmd := p.Command(id, pane.Invocation{Repeat: inv.repeat, Args: inv.args})
	a.panes[len(a.panes)-1] = updated
	return a, cmd
}

// unhandled reports a command that reached invoke with no handler. validateWiring
// makes this unreachable for bound keys and typed commands; it stays as a
// visible last line of defence rather than a silent no-op.
func (a *App) unhandled(id command.ID) (tea.Model, tea.Cmd) {
	a.setErr(text.StatusUnhandledCommand, string(id))
	return a, nil
}

type aiAnalysisResultMsg struct {
	number  domain.PatentNumber
	patent  domain.Patent
	locator string
	result  string
	err     error
}

func (a *App) runAIAnalysis(patent domain.Patent, actionType, customPrompt string) tea.Cmd {
	analyzer := a.buildAnalyzer()
	prompt, locator := ai.PromptForAction(actionType, a.aiProvider)
	if customPrompt != "" {
		prompt = customPrompt
		locator = "AI Custom: " + customPrompt
		if len(customPrompt) > 20 {
			locator = "AI Custom: " + customPrompt[:20] + "..."
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		res, err := analyzer.AnalyzePatent(ctx, patent, prompt)
		return aiAnalysisResultMsg{
			number:  patent.Number,
			patent:  patent,
			locator: locator,
			result:  res,
			err:     err,
		}
	}
}

func (a *App) checkPatentExists(targetIndex int, number domain.PatentNumber) tea.Cmd {
	client := a.client
	project := ""
	if a.activeProject != nil {
		project = string(a.activeProject.ID)
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.PatentResult
		err := client.Call(ctx, proto.MethodPatentGet, proto.PatentGetParams{Number: number, Project: domain.ProjectID(project)}, &res)
		exists := err == nil
		return historyCheckResultMsg{
			targetIndex: targetIndex,
			number:      number,
			exists:      exists,
			err:         err,
		}
	}
}
