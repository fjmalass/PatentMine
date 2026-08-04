package overlay

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
)

// OfficeActionPicker assigns (or, in release mode, removes) the selected
// patent(s) to/from one or more of the matter's office actions for review. It is
// the catalog/patent-detail entry point (:officeaction.assign /
// :officeaction.release) — the office-action-driven counterpart is the two-pane
// OfficeActionPatents overlay reached with `p` from the office-action detail.
type OfficeActionPicker struct {
	client  *rpc.Client
	theme   render.Theme
	project domain.ProjectID
	patents []domain.PatentNumber
	release bool

	items   []domain.OfficeAction
	checked map[string]CheckState
	initial map[string]CheckState
	cursor  int
	loading bool
	loadErr string
	msg     string
	jump    JumpNavigator
}

// NewOfficeActionPicker builds the picker for the given patents; release toggles
// between assign and remove semantics.
func NewOfficeActionPicker(client *rpc.Client, theme render.Theme, project domain.ProjectID, patents []domain.PatentNumber, release bool) *OfficeActionPicker {
	return &OfficeActionPicker{
		client:  client,
		theme:   theme,
		project: project,
		patents: patents,
		release: release,
		checked: map[string]CheckState{},
		initial: map[string]CheckState{},
		loading: true,
	}
}

func (o *OfficeActionPicker) Title() string {
	verb := "Assign to"
	if o.release {
		verb = "Remove from"
	}
	return fmt.Sprintf("%s office action — %d patent(s)", verb, len(o.patents))
}

func (o *OfficeActionPicker) Handles() []command.ID                      { return nil }
func (o *OfficeActionPicker) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *OfficeActionPicker) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, 60, 55, 50, 12)
}

type oaPickerLoadedMsg struct {
	items  []domain.OfficeAction
	counts map[string]int
	err    error
}

type oaPickerDoneMsg struct {
	status  pane.StatusMsg
	changed *pane.OfficeActionAssignmentsChangedMsg
}

func (o *OfficeActionPicker) Init() tea.Cmd {
	client, project, patents := o.client, o.project, o.patents
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.OfficeActionListResult
		if err := client.Call(ctx, proto.MethodOfficeActionList, proto.OfficeActionListParams{Project: project}, &res); err != nil {
			return oaPickerLoadedMsg{err: err}
		}

		counts := make(map[string]int)
		if len(patents) > 0 {
			var assigned proto.PatentOfficeActionsResult
			if err := client.Call(ctx, proto.MethodPatentOfficeActions, proto.PatentOfficeActionsParams{Patents: patents}, &assigned); err != nil {
				return oaPickerLoadedMsg{err: err}
			}
			for _, refs := range assigned.ByPatent {
				for _, ref := range refs {
					counts[ref.OfficeActionID]++
				}
			}
		}

		return oaPickerLoadedMsg{items: res.OfficeActions, counts: counts}
	}
}

func (o *OfficeActionPicker) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case oaPickerLoadedMsg:
		o.loading = false
		if m.err != nil {
			o.loadErr = m.err.Error()
			return o, nil
		}
		o.items = m.items
		keys := make([]string, 0, len(o.items))
		for _, it := range o.items {
			keys = append(keys, it.ID)
		}
		o.checked, o.initial = checklistStates(keys, m.counts, len(o.patents))
		if o.cursor >= len(o.items) {
			o.cursor = max(0, len(o.items)-1)
		}
	case oaPickerDoneMsg:
		cmds := []tea.Cmd{func() tea.Msg { return CloseOverlayMsg{} }}
		if m.changed != nil {
			cmds = append(cmds, func() tea.Msg { return *m.changed })
		} else {
			cmds = append(cmds, func() tea.Msg { return m.status })
		}
		return o, tea.Batch(cmds...)
	}
	return o, nil
}

func (o *OfficeActionPicker) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if o.loading {
		return o, nil, true
	}
	if c, ok := o.jump.HandleKey(msg, o.cursor, len(o.items)); ok {
		o.cursor = c
		return o, nil, true
	}
	switch msg.String() {
	case "esc", "q":
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "j", "down":
		o.move(1)
	case "k", "up":
		o.move(-1)
	case ";":
		o.jump.Active = true
		o.jump.PendingCount = 0
		o.jump.PendingG = false
	case " ":
		if i := o.cursor; i >= 0 && i < len(o.items) {
			id := o.items[i].ID
			o.checked[id] = toggleChecklistState(o.checked[id])
		}
	case "enter":
		return o, o.apply(), true
	}
	return o, nil, true
}

func (o *OfficeActionPicker) move(delta int) {
	if len(o.items) == 0 {
		return
	}
	o.cursor = max(0, min(o.cursor+delta, len(o.items)-1))
}

// apply assigns (or releases) the patents to every checked office action, then
// closes with a status. Office actions are mutated one call each, reusing the
// existing assign/release RPCs.
func (o *OfficeActionPicker) apply() tea.Cmd {
	keys := make([]string, 0, len(o.items))
	for _, it := range o.items {
		keys = append(keys, it.ID)
	}
	changes := checklistChanges(keys, o.checked, o.initial)
	if len(changes) == 0 {
		o.msg = "select at least one office-action change with [space]"
		return nil
	}
	client, patents := o.client, o.patents
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		for _, change := range changes {
			var res proto.OfficeActionPatentsResult
			var err error
			if change.on {
				err = client.Call(ctx, proto.MethodOfficeActionAssignPatents, proto.OfficeActionAssignPatentsParams{ID: change.key, Patents: patents}, &res)
			} else {
				err = client.Call(ctx, proto.MethodOfficeActionReleasePatents, proto.OfficeActionReleasePatentsParams{ID: change.key, Patents: patents}, &res)
			}
			if err != nil {
				return oaPickerDoneMsg{status: pane.StatusMsg{Key: text.StatusGeneric, Args: []any{"office-action assignment failed: " + err.Error()}, Error: true}}
			}
		}
		status := pane.StatusMsg{Key: text.StatusGeneric, Args: []any{
			fmt.Sprintf("Updated %d patent(s) · %d office action(s)", len(patents), len(changes)),
		}}
		return oaPickerDoneMsg{changed: &pane.OfficeActionAssignmentsChangedMsg{Project: o.project, Patents: patents, Status: status}}
	}
}

func (o *OfficeActionPicker) View(maxW, maxH int) string {
	if o.loading {
		return o.theme.Dim.Render("loading office actions…")
	}
	if o.loadErr != "" {
		return o.theme.Error.Render("error: " + o.loadErr)
	}
	if len(o.items) == 0 {
		return o.theme.Dim.Render(render.Truncate("No office actions in this matter. Add one with :officeaction.add", maxW))
	}

	var b strings.Builder
	verb := "Assign to"
	if o.release {
		verb = "Remove from"
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(
		fmt.Sprintf("%s which office action(s)?  (%d patent(s) selected)", verb, len(o.patents)), maxW)))
	b.WriteString("\n\n")

	rows := max(maxH-5, 1)
	start := 0
	if o.cursor >= rows {
		start = o.cursor - rows + 1
	}
	for i := start; i < len(o.items) && i < start+rows; i++ {
		it := o.items[i]
		box := "[ ]"
		switch o.checked[it.ID] {
		case CheckChecked:
			box = "[x]"
		case CheckIndeterminate:
			box = "[-]"
		}
		line := fmt.Sprintf("%s%s %s", o.jump.GutterPrefix(i+1), box, it.DisplayLabel())
		if i == o.cursor {
			b.WriteString(o.theme.Selected.Render(render.Pad(render.Truncate(line, maxW), maxW)))
		} else {
			b.WriteString(o.theme.Row.Render(render.Truncate(line, maxW)))
		}
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	hint := "[space] toggle  [enter] apply  [j/k] move  [;] jump  [esc] cancel"
	if o.msg != "" {
		hint = o.msg + "  ·  " + hint
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(hint, maxW)))
	return b.String()
}
