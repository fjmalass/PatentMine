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
	"patentmine/internal/tui/render"
)

const (
	tsWidthPct  = 60
	tsHeightPct = 50
	tsMinWidth  = 54
	tsMinHeight = 12
)

type timeSummaryLoadedMsg struct {
	res proto.TimeSummaryResult
	err error
}

// TimeSummaryOverlay is the matter's billing readout: recorded time by activity,
// the validated/unvalidated split, and AI usage. Read-only; the review/validate
// flow lives in TimeReview (:validate.time).
type TimeSummaryOverlay struct {
	client  *rpc.Client
	theme   render.Theme
	project domain.ProjectID

	res     proto.TimeSummaryResult
	loading bool
	loadErr string
}

func NewTimeSummary(client *rpc.Client, theme render.Theme, project domain.ProjectID) *TimeSummaryOverlay {
	return &TimeSummaryOverlay{client: client, theme: theme, project: project, loading: true}
}

func (o *TimeSummaryOverlay) Title() string { return "Time & AI Usage" }

func (o *TimeSummaryOverlay) Handles() []command.ID { return nil }

func (o *TimeSummaryOverlay) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *TimeSummaryOverlay) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, tsWidthPct, tsHeightPct, tsMinWidth, tsMinHeight)
}

func (o *TimeSummaryOverlay) Init() tea.Cmd {
	client, project := o.client, o.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.TimeSummaryResult
		err := client.Call(ctx, proto.MethodTimeSummary, proto.TimeListParams{Project: project}, &res)
		return timeSummaryLoadedMsg{res: res, err: err}
	}
}

func (o *TimeSummaryOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if m, ok := msg.(timeSummaryLoadedMsg); ok {
		o.loading = false
		if m.err != nil {
			o.loadErr = m.err.Error()
			return o, nil
		}
		o.res = m.res
		o.loadErr = ""
	}
	return o, nil
}

func (o *TimeSummaryOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if msg.Type == tea.KeyEsc || msg.String() == "q" {
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	return o, nil, true
}

func (o *TimeSummaryOverlay) View(maxW, maxH int) string {
	if o.loading {
		return o.theme.Dim.Render("loading time summary…")
	}
	if o.loadErr != "" {
		return o.theme.Error.Render("error: " + o.loadErr)
	}

	var b strings.Builder
	row := func(label, value string) {
		b.WriteString(o.theme.Dim.Render(render.Pad(label, 18)))
		b.WriteString(o.theme.Row.Render(render.Truncate(value, max(maxW-18, 6))))
		b.WriteByte('\n')
	}

	b.WriteString(o.theme.Header.Render(render.Pad("RECORDED TIME", maxW)))
	b.WriteByte('\n')
	t := o.res.Time
	order := []domain.TimeActivity{domain.TimeReading, domain.TimeWriting, domain.TimeAI, domain.TimeCall, domain.TimeAdmin}
	total := 0
	any := false
	for _, a := range order {
		if secs := t.Seconds[a]; secs > 0 {
			row(a.Label(), summaryDuration(secs))
			total += secs
			any = true
		}
	}
	if !any {
		row("(none yet)", "")
	}
	row("— total", summaryDuration(total))
	row("validated", summaryDuration(t.ValidatedSecs))
	if t.UnvalidatedSecs > 0 {
		row("unvalidated", fmt.Sprintf("%s · %d entries — :validate.time", summaryDuration(t.UnvalidatedSecs), t.UnvalidatedCount))
	}

	b.WriteByte('\n')
	b.WriteString(o.theme.Header.Render(render.Pad("AI USAGE", maxW)))
	b.WriteByte('\n')
	row("calls", fmt.Sprintf("%d", o.res.AI.Calls))
	if o.res.AI.TotalTokens > 0 {
		row("tokens", fmt.Sprintf("%d (prompt %d / completion %d)", o.res.AI.TotalTokens, o.res.AI.PromptTokens, o.res.AI.CompletionTokens))
	}

	b.WriteByte('\n')
	b.WriteString(o.theme.Dim.Render(render.Truncate("esc close · durations only (rates later)", maxW)))
	return b.String()
}

func summaryDuration(secs int) string {
	d := time.Duration(secs) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := secs % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	return fmt.Sprintf("%dm %02ds", m, s)
}
