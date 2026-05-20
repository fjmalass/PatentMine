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

// Internal message types for tag taxonomy operations
type loadedTagsMsg struct {
	tags []domain.Tag
	err  error
}

type tagCreatedMsg struct {
	tag domain.Tag
	err error
}

type tagDeletedMsg struct {
	name string
	err  error
}

// -----------------------------------------------------------------------------
// TagTaxonomyOverlay: For managing the project taxonomy tags (:tag.list)
// -----------------------------------------------------------------------------

type TagTaxonomyOverlay struct {
	client      *rpc.Client
	theme       render.Theme
	catalog     *text.Catalog
	project     domain.ProjectID
	tags        []domain.Tag
	selected    int
	adding      bool
	inputValue  string
	inputCursor int
	err         error
	msg         string
}

func NewTagTaxonomyOverlay(client *rpc.Client, theme render.Theme, catalog *text.Catalog, project domain.ProjectID) (*TagTaxonomyOverlay, tea.Cmd) {
	o := &TagTaxonomyOverlay{
		client:  client,
		theme:   theme,
		catalog: catalog,
		project: project,
	}
	return o, o.loadTagsCmd()
}

func (o *TagTaxonomyOverlay) Title() string {
	return "Project Tag Taxonomy"
}

func (o *TagTaxonomyOverlay) Handles() []command.ID {
	return []command.ID{
		command.CloseOverlay,
	}
}

func (o *TagTaxonomyOverlay) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return o, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return o, nil
}

func (o *TagTaxonomyOverlay) loadTagsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var res proto.TagListResult
		if err := o.client.Call(ctx, proto.MethodTagList, proto.TagListParams{Project: o.project}, &res); err != nil {
			return loadedTagsMsg{err: err}
		}
		return loadedTagsMsg{tags: res.Tags}
	}
}

func (o *TagTaxonomyOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case loadedTagsMsg:
		if m.err != nil {
			o.err = m.err
		} else {
			o.tags = m.tags
			if o.selected >= len(o.tags) {
				o.selected = max(0, len(o.tags)-1)
			}
		}
		return o, nil

	case tagCreatedMsg:
		if m.err != nil {
			o.err = m.err
		} else {
			o.adding = false
			o.inputValue = ""
			o.inputCursor = 0
			o.msg = fmt.Sprintf("Tag '%s' created", m.tag.Name)
			return o, o.loadTagsCmd()
		}
		return o, nil

	case tagDeletedMsg:
		if m.err != nil {
			o.err = m.err
		} else {
			o.msg = fmt.Sprintf("Tag '%s' deleted", m.name)
			return o, o.loadTagsCmd()
		}
		return o, nil
	}
	return o, nil
}

func (o *TagTaxonomyOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	o.err = nil
	o.msg = ""

	if o.adding {
		switch msg.Type {
		case tea.KeyEsc:
			o.adding = false
			o.inputValue = ""
			o.inputCursor = 0
			return o, nil, true
		case tea.KeyEnter:
			name := strings.TrimSpace(o.inputValue)
			if name == "" {
				return o, nil, true
			}
			if err := domain.ValidateTagName(name); err != nil {
				o.err = err
				return o, nil, true
			}
			return o, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				var res domain.Tag
				if err := o.client.Call(ctx, proto.MethodTagCreate, proto.TagParams{Project: o.project, Name: name}, &res); err != nil {
					return tagCreatedMsg{err: err}
				}
				return tagCreatedMsg{tag: res}
			}, true
		case tea.KeyBackspace:
			if o.inputCursor > 0 {
				runes := []rune(o.inputValue)
				o.inputValue = string(append(runes[:o.inputCursor-1], runes[o.inputCursor:]...))
				o.inputCursor--
			}
			return o, nil, true
		case tea.KeyLeft:
			if o.inputCursor > 0 {
				o.inputCursor--
			}
			return o, nil, true
		case tea.KeyRight:
			if o.inputCursor < len([]rune(o.inputValue)) {
				o.inputCursor++
			}
			return o, nil, true
		case tea.KeyRunes, tea.KeySpace:
			runes := []rune(o.inputValue)
			ins := []rune(msg.String())
			merged := make([]rune, 0, len(runes)+len(ins))
			merged = append(merged, runes[:o.inputCursor]...)
			merged = append(merged, ins...)
			merged = append(merged, runes[o.inputCursor:]...)
			o.inputValue = string(merged)
			o.inputCursor += len(ins)
			return o, nil, true
		}
		return o, nil, true
	}

	switch msg.String() {
	case "q", "esc":
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "j", "down":
		if len(o.tags) > 0 {
			o.selected = (o.selected + 1) % len(o.tags)
		}
		return o, nil, true
	case "k", "up":
		if len(o.tags) > 0 {
			o.selected = (o.selected - 1 + len(o.tags)) % len(o.tags)
		}
		return o, nil, true
	case "a", "n":
		o.adding = true
		o.inputValue = ""
		o.inputCursor = 0
		return o, nil, true
	case "x", "d", "delete":
		if len(o.tags) > 0 && o.selected >= 0 && o.selected < len(o.tags) {
			tagToDelete := o.tags[o.selected]
			return o, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				var res proto.Empty
				if err := o.client.Call(ctx, proto.MethodTagDelete, proto.TagDeleteParams{Project: o.project, Name: tagToDelete.Name}, &res); err != nil {
					return tagDeletedMsg{err: err}
				}
				return tagDeletedMsg{name: tagToDelete.Name}
			}, true
		}
		return o, nil, true
	}

	return o, nil, true
}

func (o *TagTaxonomyOverlay) View(maxW, maxH int) string {
	var b strings.Builder

	if o.err != nil {
		b.WriteString(o.theme.Error.Render(render.Truncate("Error: "+o.err.Error(), maxW)))
		b.WriteString("\n\n")
	} else if o.msg != "" {
		b.WriteString(o.theme.OK.Render(render.Truncate(o.msg, maxW)))
		b.WriteString("\n\n")
	}

	if len(o.tags) == 0 {
		b.WriteString(o.theme.MutedItalic.Render("No tags defined. Press [a] or [n] to create one."))
	} else {
		b.WriteString(o.theme.Dim.Render(fmt.Sprintf("Total Tags: %d", len(o.tags))))
		b.WriteString("\n\n")

		header := fmt.Sprintf("  %-3s %-20s %-12s", "#", "Tag Name", "Created At")
		b.WriteString(o.theme.Header.Underline(true).Render(render.Truncate(header, maxW)))
		b.WriteString("\n")

		pageSize := maxH - 8
		if o.adding {
			pageSize -= 4
		}
		if pageSize < 1 {
			pageSize = 1
		}

		start := max(0, o.selected-pageSize/2)
		end := min(len(o.tags), start+pageSize)
		if end-start < pageSize && start > 0 {
			start = max(0, end-pageSize)
		}

		for i := start; i < end; i++ {
			t := o.tags[i]
			var line string
			prefix := "  "
			if i == o.selected {
				prefix = "→ "
			}

			createdStr := t.CreatedAt.Format("2006-01-02")
			line = fmt.Sprintf("%s%-3d %-20s %-12s", prefix, i+1, t.Name, createdStr)

			if i == o.selected {
				b.WriteString(o.theme.Selected.Render(render.Truncate(line, maxW)))
			} else {
				if i%2 == 1 {
					b.WriteString(o.theme.RowAlt.Render(render.Truncate(line, maxW)))
				} else {
					b.WriteString(o.theme.Row.Render(render.Truncate(line, maxW)))
				}
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	if o.adding {
		b.WriteString(o.theme.Title.Render("Create New Tag (snake_case only):"))
		b.WriteString("\n")
		runes := []rune(o.inputValue)
		before := string(runes[:o.inputCursor])
		after := string(runes[o.inputCursor:])
		input := "> " + before + o.theme.Title.Render("█") + after
		b.WriteString(render.Truncate(input, maxW))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[Enter] Save  [Esc] Cancel"))
	} else {
		b.WriteString(o.theme.Dim.Render("[j/k/↑/↓] Scroll  [a/n] Add  [d/x/Del] Delete  [q/Esc] Close"))
	}

	return b.String()
}

// -----------------------------------------------------------------------------
// TagPatentOverlay: For managing the tags assigned to selected patent(s) (:tag.patent)
// -----------------------------------------------------------------------------

type loadedPatentTagsMsg struct {
	available []domain.Tag
	assigned  []domain.Tag
	err       error
}

type applyFinishedMsg struct {
	status pane.StatusMsg
}

type TagPatentOverlay struct {
	client      *rpc.Client
	theme       render.Theme
	catalog     *text.Catalog
	project     domain.ProjectID
	patents     []domain.PatentNumber
	available   []domain.Tag
	checked     map[string]bool
	selected    int
	adding      bool
	inputValue  string
	inputCursor int
	err         error
	msg         string
	applying    bool
}

func NewTagPatentOverlay(client *rpc.Client, theme render.Theme, catalog *text.Catalog, project domain.ProjectID, patents []domain.PatentNumber) (*TagPatentOverlay, tea.Cmd) {
	o := &TagPatentOverlay{
		client:  client,
		theme:   theme,
		catalog: catalog,
		project: project,
		patents: patents,
		checked: make(map[string]bool),
	}
	return o, o.loadTagsCmd()
}

func (o *TagPatentOverlay) Title() string {
	if len(o.patents) > 1 {
		return fmt.Sprintf("Manage Tags for %d Patents", len(o.patents))
	}
	return fmt.Sprintf("Manage Tags for Patent %s", o.patents[0].String())
}

func (o *TagPatentOverlay) Handles() []command.ID {
	return []command.ID{
		command.CloseOverlay,
	}
}

func (o *TagPatentOverlay) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return o, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return o, nil
}

func (o *TagPatentOverlay) loadTagsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		var taxRes proto.TagListResult
		if err := o.client.Call(ctx, proto.MethodTagList, proto.TagListParams{Project: o.project}, &taxRes); err != nil {
			return loadedPatentTagsMsg{err: err}
		}

		var patRes proto.PatentTagListResult
		if err := o.client.Call(ctx, proto.MethodPatentTagList, proto.PatentTagListParams{Project: o.project, Patent: o.patents[0]}, &patRes); err != nil {
			return loadedPatentTagsMsg{err: err}
		}

		return loadedPatentTagsMsg{
			available: taxRes.Tags,
			assigned:  patRes.Tags,
		}
	}
}

func (o *TagPatentOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case loadedPatentTagsMsg:
		if m.err != nil {
			o.err = m.err
		} else {
			o.available = m.available
			o.checked = make(map[string]bool)
			for _, t := range m.assigned {
				o.checked[t.Name] = true
			}
			if o.selected >= len(o.available) {
				o.selected = max(0, len(o.available)-1)
			}
		}
		return o, nil

	case tagCreatedMsg:
		if m.err != nil {
			o.err = m.err
		} else {
			o.adding = false
			o.inputValue = ""
			o.inputCursor = 0
			o.checked[m.tag.Name] = true
			o.msg = fmt.Sprintf("Tag '%s' created and selected", m.tag.Name)
			return o, o.loadTagsCmd()
		}
		return o, nil

	case applyFinishedMsg:
		o.applying = false
		return o, tea.Batch(
			func() tea.Msg { return m.status },
			func() tea.Msg { return CloseOverlayMsg{} },
		)
	}
	return o, nil
}

func (o *TagPatentOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	o.err = nil
	o.msg = ""

	if o.adding {
		switch msg.Type {
		case tea.KeyEsc:
			o.adding = false
			o.inputValue = ""
			o.inputCursor = 0
			return o, nil, true
		case tea.KeyEnter:
			name := strings.TrimSpace(o.inputValue)
			if name == "" {
				return o, nil, true
			}
			if err := domain.ValidateTagName(name); err != nil {
				o.err = err
				return o, nil, true
			}
			return o, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				var res domain.Tag
				if err := o.client.Call(ctx, proto.MethodTagCreate, proto.TagParams{Project: o.project, Name: name}, &res); err != nil {
					return tagCreatedMsg{err: err}
				}
				return tagCreatedMsg{tag: res}
			}, true
		case tea.KeyBackspace:
			if o.inputCursor > 0 {
				runes := []rune(o.inputValue)
				o.inputValue = string(append(runes[:o.inputCursor-1], runes[o.inputCursor:]...))
				o.inputCursor--
			}
			return o, nil, true
		case tea.KeyLeft:
			if o.inputCursor > 0 {
				o.inputCursor--
			}
			return o, nil, true
		case tea.KeyRight:
			if o.inputCursor < len([]rune(o.inputValue)) {
				o.inputCursor++
			}
			return o, nil, true
		case tea.KeyRunes, tea.KeySpace:
			runes := []rune(o.inputValue)
			ins := []rune(msg.String())
			merged := make([]rune, 0, len(runes)+len(ins))
			merged = append(merged, runes[:o.inputCursor]...)
			merged = append(merged, ins...)
			merged = append(merged, runes[o.inputCursor:]...)
			o.inputValue = string(merged)
			o.inputCursor += len(ins)
			return o, nil, true
		}
		return o, nil, true
	}

	switch msg.String() {
	case "q", "esc":
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "j", "down":
		if len(o.available) > 0 {
			o.selected = (o.selected + 1) % len(o.available)
		}
		return o, nil, true
	case "k", "up":
		if len(o.available) > 0 {
			o.selected = (o.selected - 1 + len(o.available)) % len(o.available)
		}
		return o, nil, true
	case " ":
		if len(o.available) > 0 && o.selected >= 0 && o.selected < len(o.available) {
			tagName := o.available[o.selected].Name
			o.checked[tagName] = !o.checked[tagName]
		}
		return o, nil, true
	case "a", "n":
		o.adding = true
		o.inputValue = ""
		o.inputCursor = 0
		return o, nil, true
	case "enter":
		o.applying = true
		return o, o.applyTagsCmd(), true
	}

	return o, nil, true
}

func (o *TagPatentOverlay) applyTagsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		for _, pat := range o.patents {
			var currentRes proto.PatentTagListResult
			if err := o.client.Call(ctx, proto.MethodPatentTagList, proto.PatentTagListParams{Project: o.project, Patent: pat}, &currentRes); err != nil {
				return applyFinishedMsg{status: pane.StatusMsg{Key: text.StatusTagPatentAddFailed, Args: []any{err.Error()}, Error: true}}
			}

			currentAssigned := make(map[string]bool)
			for _, t := range currentRes.Tags {
				currentAssigned[t.Name] = true
			}

			// Add checked but not assigned tags
			for tagName, checked := range o.checked {
				if checked && !currentAssigned[tagName] {
					var empty proto.Empty
					if err := o.client.Call(ctx, proto.MethodPatentTagAdd, proto.TagParams{Project: o.project, Patent: pat, Name: tagName}, &empty); err != nil {
						return applyFinishedMsg{status: pane.StatusMsg{Key: text.StatusTagPatentAddFailed, Args: []any{err.Error()}, Error: true}}
					}
				}
			}

			// Delete assigned but not checked tags
			for _, t := range o.available {
				if !o.checked[t.Name] && currentAssigned[t.Name] {
					var empty proto.Empty
					if err := o.client.Call(ctx, proto.MethodPatentTagDelete, proto.TagParams{Project: o.project, Patent: pat, Name: t.Name}, &empty); err != nil {
						return applyFinishedMsg{status: pane.StatusMsg{Key: text.StatusTagPatentDeleteFailed, Args: []any{err.Error()}, Error: true}}
					}
				}
			}
		}

		var summary string
		if len(o.patents) > 1 {
			summary = fmt.Sprintf("Tags updated for %d patents", len(o.patents))
		} else {
			summary = fmt.Sprintf("Tags updated for patent %s", o.patents[0].String())
		}

		return applyFinishedMsg{status: pane.StatusMsg{Key: text.StatusFilter, Args: []any{summary}}}
	}
}

func (o *TagPatentOverlay) View(maxW, maxH int) string {
	var b strings.Builder

	if o.err != nil {
		b.WriteString(o.theme.Error.Render(render.Truncate("Error: "+o.err.Error(), maxW)))
		b.WriteString("\n\n")
	} else if o.msg != "" {
		b.WriteString(o.theme.OK.Render(render.Truncate(o.msg, maxW)))
		b.WriteString("\n\n")
	}

	if o.applying {
		b.WriteString(o.theme.Title.Render("Applying tag changes..."))
		return b.String()
	}

	targetDesc := o.patents[0].String()
	if len(o.patents) > 1 {
		targetDesc = fmt.Sprintf("%d patents", len(o.patents))
	}
	b.WriteString(o.theme.Dim.Render(fmt.Sprintf("Target: %s", targetDesc)))
	b.WriteString("\n\n")

	if len(o.available) == 0 {
		b.WriteString(o.theme.MutedItalic.Render("No taxonomy tags defined in this project."))
		b.WriteString("\n")
		b.WriteString(o.theme.Dim.Render("Press [a] or [n] to create and register a new tag."))
	} else {
		b.WriteString(o.theme.Dim.Render(fmt.Sprintf("Select tags to assign (total taxonomy: %d):", len(o.available))))
		b.WriteString("\n\n")

		pageSize := maxH - 8
		if o.adding {
			pageSize -= 4
		}
		if pageSize < 1 {
			pageSize = 1
		}

		start := max(0, o.selected-pageSize/2)
		end := min(len(o.available), start+pageSize)
		if end-start < pageSize && start > 0 {
			start = max(0, end-pageSize)
		}

		for i := start; i < end; i++ {
			t := o.available[i]
			var line string
			prefix := "  "
			if i == o.selected {
				prefix = "→ "
			}

			checkedChar := "[ ]"
			if o.checked[t.Name] {
				checkedChar = "[x]"
			}

			line = fmt.Sprintf("%s%s %s", prefix, checkedChar, t.Name)

			if i == o.selected {
				b.WriteString(o.theme.Selected.Render(render.Truncate(line, maxW)))
			} else {
				if o.checked[t.Name] {
					b.WriteString(o.theme.OK.Render(render.Truncate(line, maxW)))
				} else {
					if i%2 == 1 {
						b.WriteString(o.theme.RowAlt.Render(render.Truncate(line, maxW)))
					} else {
						b.WriteString(o.theme.Row.Render(render.Truncate(line, maxW)))
					}
				}
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	if o.adding {
		b.WriteString(o.theme.Title.Render("Create New Tag (snake_case only):"))
		b.WriteString("\n")
		runes := []rune(o.inputValue)
		before := string(runes[:o.inputCursor])
		after := string(runes[o.inputCursor:])
		input := "> " + before + o.theme.Title.Render("█") + after
		b.WriteString(render.Truncate(input, maxW))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[Enter] Save  [Esc] Cancel"))
	} else {
		b.WriteString(o.theme.Dim.Render("[j/k/↑/↓] Scroll  [Space] Toggle  [a/n] Add Tag  [Enter] Apply  [q/Esc] Cancel"))
	}

	return b.String()
}
