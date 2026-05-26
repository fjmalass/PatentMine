package overlay

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

// Internal message types for classification operations
type loadedClassificationsMsg struct {
	classifications []domain.Classification
	err             error
}

type classificationLookupMsg struct {
	classification domain.Classification
	err            error
}

type classificationDeletedMsg struct {
	system string
	code   string
	err    error
}

// -----------------------------------------------------------------------------
// ClassificationListOverlay: For managing the cached classification definitions (:class.list)
// -----------------------------------------------------------------------------

type ClassificationListOverlay struct {
	client          *rpc.Client
	theme           render.Theme
	catalog         *text.Catalog
	classifications []domain.Classification
	selected        int
	adding          bool
	inputValue      string
	inputCursor     int
	err             error
	msg             string
}

func NewClassificationListOverlay(client *rpc.Client, theme render.Theme, catalog *text.Catalog) (*ClassificationListOverlay, tea.Cmd) {
	o := &ClassificationListOverlay{
		client:  client,
		theme:   theme,
		catalog: catalog,
	}
	return o, o.loadClassificationsCmd()
}

func (o *ClassificationListOverlay) Title() string {
	return "Patent Classification Registry"
}

func (o *ClassificationListOverlay) Handles() []command.ID {
	return []command.ID{
		command.CloseOverlay,
	}
}

func (o *ClassificationListOverlay) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return o, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return o, nil
}

func (o *ClassificationListOverlay) loadClassificationsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.ClassificationListResult
		if err := o.client.Call(ctx, proto.MethodClassificationList, proto.Empty{}, &res); err != nil {
			return loadedClassificationsMsg{err: err}
		}
		return loadedClassificationsMsg{classifications: res.Classifications}
	}
}

func (o *ClassificationListOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case loadedClassificationsMsg:
		if m.err != nil {
			o.err = m.err
		} else {
			o.classifications = m.classifications
			if o.selected >= len(o.classifications) {
				o.selected = max(0, len(o.classifications)-1)
			}
		}
		return o, nil

	case classificationLookupMsg:
		if m.err != nil {
			o.err = m.err
		} else {
			o.adding = false
			o.inputValue = ""
			o.inputCursor = 0
			o.msg = fmt.Sprintf("Classification '%s' (%s) cached successfully", m.classification.Code, m.classification.System)
			return o, o.loadClassificationsCmd()
		}
		return o, nil

	case classificationDeletedMsg:
		if m.err != nil {
			o.err = m.err
		} else {
			o.msg = fmt.Sprintf("Deleted classification %s %s", m.system, m.code)
			if o.selected >= len(o.classifications)-1 {
				o.selected = max(0, len(o.classifications)-2)
			}
			return o, o.loadClassificationsCmd()
		}
		return o, nil
	}
	return o, nil
}

func (o *ClassificationListOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
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
			code := strings.TrimSpace(o.inputValue)
			if code == "" {
				return o, nil, true
			}
			return o, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				var res domain.Classification
				if err := o.client.Call(ctx, proto.MethodClassificationLookup, proto.ClassificationLookupParams{Code: code}, &res); err != nil {
					return classificationLookupMsg{err: err}
				}
				return classificationLookupMsg{classification: res}
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
		if len(o.classifications) > 0 {
			o.selected = (o.selected + 1) % len(o.classifications)
		}
		return o, nil, true
	case "k", "up":
		if len(o.classifications) > 0 {
			o.selected = (o.selected - 1 + len(o.classifications)) % len(o.classifications)
		}
		return o, nil, true
	case "a", "n":
		o.adding = true
		o.inputValue = ""
		o.inputCursor = 0
		return o, nil, true
	case "x", "d", "delete":
		if len(o.classifications) > 0 && o.selected >= 0 && o.selected < len(o.classifications) {
			c := o.classifications[o.selected]
			return o, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				var res proto.Empty
				if err := o.client.Call(ctx, proto.MethodClassificationDelete, proto.ClassificationDeleteParams{System: c.System, Code: c.Code}, &res); err != nil {
					return classificationDeletedMsg{err: err}
				}
				return classificationDeletedMsg{system: c.System, code: c.Code}
			}, true
		}
		return o, nil, true
	}

	return o, nil, true
}

func (o *ClassificationListOverlay) View(maxW, maxH int) string {
	var b strings.Builder

	// Top alerts / error messages
	if o.err != nil {
		b.WriteString(o.theme.Error.Render(render.Truncate("Error: "+o.err.Error(), maxW)))
		b.WriteString("\n\n")
	} else if o.msg != "" {
		b.WriteString(o.theme.OK.Render(render.Truncate(o.msg, maxW)))
		b.WriteString("\n\n")
	}

	// Dynamic layout sizes
	leftW := maxW/2 - 2
	if leftW < 30 {
		leftW = 30
	}
	rightW := maxW - leftW - 6
	if rightW < 20 {
		rightW = 20
	}

	var leftLines []string
	var rightLines []string

	// Left: list definitions
	if len(o.classifications) == 0 {
		leftLines = append(leftLines, o.theme.MutedItalic.Render("No classifications cached."))
		leftLines = append(leftLines, o.theme.Dim.Render("Press [a]/[n] to fetch a CPC code."))
	} else {
		leftLines = append(leftLines, o.theme.Dim.Render(fmt.Sprintf("Stored definitions: %d", len(o.classifications))))
		leftLines = append(leftLines, "")

		header := fmt.Sprintf("  %-4s %-14s %-20s", "Sys", "Code", "Description")
		leftLines = append(leftLines, o.theme.Header.Underline(true).Render(render.Truncate(header, leftW)))

		pageSize := maxH - 8
		if o.adding {
			pageSize -= 4
		}
		if pageSize < 1 {
			pageSize = 1
		}

		start := max(0, o.selected-pageSize/2)
		end := min(len(o.classifications), start+pageSize)
		if end-start < pageSize && start > 0 {
			start = max(0, end-pageSize)
		}

		for i := start; i < end; i++ {
			c := o.classifications[i]
			cursorPart := o.theme.Glyphs.RowNoCursor
			if i == o.selected {
				cursorPart = o.theme.Glyphs.RowCursor
			}
			prefix := cursorPart + o.theme.Glyphs.RowNoMark + " "

			// Clean/truncate description for left side list
			descTrunc := render.Truncate(c.Description, leftW-22)
			line := fmt.Sprintf("%s%-4s %-14s %s", prefix, c.System, c.Code, descTrunc)

			if i == o.selected {
				leftLines = append(leftLines, o.theme.Selected.Render(render.Truncate(line, leftW)))
			} else {
				if i%2 == 1 {
					leftLines = append(leftLines, o.theme.RowAlt.Render(render.Truncate(line, leftW)))
				} else {
					leftLines = append(leftLines, o.theme.Row.Render(render.Truncate(line, leftW)))
				}
			}
		}
	}

	// Right: detail taxonomy card
	if len(o.classifications) > 0 && o.selected >= 0 && o.selected < len(o.classifications) {
		c := o.classifications[o.selected]
		rightLines = append(rightLines, o.theme.Title.Render("Attributes Definition Card"))
		rightLines = append(rightLines, o.theme.Dim.Render(strings.Repeat("─", rightW)))
		rightLines = append(rightLines, fmt.Sprintf("%s  %s", o.theme.HelpKey.Render("System:"), c.System))
		rightLines = append(rightLines, fmt.Sprintf("%s    %s", o.theme.HelpKey.Render("Code:"), c.Code))
		rightLines = append(rightLines, "")
		rightLines = append(rightLines, o.theme.Header.Render("Hierarchical Levels:"))
		rightLines = append(rightLines, fmt.Sprintf("  • %s %s", o.theme.Dim.Render("Section:"), c.Section))
		rightLines = append(rightLines, fmt.Sprintf("  • %s %s", o.theme.Dim.Render("Class:"), c.Class))
		rightLines = append(rightLines, fmt.Sprintf("  • %s %s", o.theme.Dim.Render("Subclass:"), c.Subclass))
		rightLines = append(rightLines, fmt.Sprintf("  • %s %s", o.theme.Dim.Render("Main Group:"), c.MainGroup))
		if c.Subgroup != "" {
			rightLines = append(rightLines, fmt.Sprintf("  • %s %s", o.theme.Dim.Render("Subgroup:"), c.Subgroup))
		}
		rightLines = append(rightLines, "")
		rightLines = append(rightLines, o.theme.Header.Render("EPO Linked Data Description:"))
		rightLines = append(rightLines, wrapText(c.Description, rightW)...)
	} else {
		rightLines = append(rightLines, o.theme.Title.Render("Patent Classification"))
		rightLines = append(rightLines, o.theme.Dim.Render(strings.Repeat("─", rightW)))
		rightLines = append(rightLines, o.theme.MutedItalic.Render("No classification selected."))
		rightLines = append(rightLines, "")
		rightLines = append(rightLines, wrapText("Lookup dynamically fetches cpc titles over SPARQL and splits hierarchies neatly.", rightW)...)
	}

	// Join left & right columns side-by-side
	finalMaxLines := max(len(leftLines), len(rightLines))
	for i := 0; i < finalMaxLines; i++ {
		leftPart := ""
		if i < len(leftLines) {
			leftPart = leftLines[i]
		}
		leftWidth := ansi.StringWidth(leftPart)
		if leftWidth < leftW {
			leftPart += strings.Repeat(" ", leftW-leftWidth)
		} else {
			leftPart = ansi.Cut(leftPart, 0, leftW)
		}

		rightPart := ""
		if i < len(rightLines) {
			rightPart = rightLines[i]
		}
		rightWidth := ansi.StringWidth(rightPart)
		if rightWidth > rightW {
			rightPart = ansi.Cut(rightPart, 0, rightW)
		}

		b.WriteString(leftPart + "   │   " + rightPart + "\n")
	}

	b.WriteString("\n")

	// Dynamic interactive input field / instructions footer
	if o.adding {
		b.WriteString(o.theme.Title.Render("Lookup CPC Code (e.g. G06F16/24):"))
		b.WriteString("\n")
		runes := []rune(o.inputValue)
		before := string(runes[:o.inputCursor])
		after := string(runes[o.inputCursor:])
		input := "> " + before + o.theme.Title.Render("█") + after
		b.WriteString(render.Truncate(input, maxW))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[Enter] Dispatch Lookup  [Esc] Cancel"))
	} else {
		b.WriteString(o.theme.Dim.Render("[j/k/↑/↓] Scroll  [a/n] Lookup Code  [d/x/Del] Delete Stored Definition  [q/Esc] Close"))
	}

	return b.String()
}

// wrapText breaks strings into lines not exceeding width columns
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var current strings.Builder
	for _, w := range words {
		if current.Len() == 0 {
			current.WriteString(w)
		} else if current.Len()+1+len(w) <= width {
			current.WriteByte(' ')
			current.WriteString(w)
		} else {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(w)
		}
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}
