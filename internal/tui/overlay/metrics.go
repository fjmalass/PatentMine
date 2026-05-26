package overlay

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

const (
	metricsOverlayWidthPct  = 90
	metricsOverlayHeightPct = 90
	metricsRefreshInterval  = 2 * time.Second
	metricsFetchTimeout     = 5 * time.Second
	metricsHistoryLimit     = 30
	metricsBarWidth         = 16
	metricsPageStep         = 8
	metricsMinWidth         = 48
	metricsMinHeight        = 14
)

type metricsTab int

const (
	metricsTabTimings metricsTab = iota
	metricsTabCounters
	metricsTabGauges
	metricsTabCount
)

var metricsTabLabels = [...]string{"Timings", "Counters", "Gauges"}

type metricsLoadedMsg struct {
	requestID uint64
	snapshot  proto.MetricsSnapshot
	err       error
}

type metricsRefreshTickMsg struct{}

type MetricsOverlay struct {
	client       *rpc.Client
	theme        render.Theme
	catalog      *text.Catalog
	localMetrics *observability.Metrics

	tab       metricsTab
	selected  [metricsTabCount]int
	requestID uint64
	loading   bool
	err       error
	current   proto.MetricsSnapshot
	history   []proto.MetricsSnapshot
	handlers  map[command.ID]cmdHandler
}

type metricsSection struct {
	title string
	rows  []metricsRow
}

type metricsRow struct {
	name       string
	group      string
	tags       []string
	summary    string
	history    string
	importance int64
}

type metricsRenderLine struct {
	text     string
	selected bool
	row      bool
}

// NewMetricsOverlay opens a metrics dashboard overlay. localMetrics is the
// TUI process's own metrics sink; it is pushed to the daemon before each
// fetch so the overlay shows combined engine + TUI metrics.
func NewMetricsOverlay(client *rpc.Client, theme render.Theme, catalog *text.Catalog, localMetrics *observability.Metrics) (*MetricsOverlay, tea.Cmd) {
	o := &MetricsOverlay{
		client:       client,
		theme:        theme,
		catalog:      catalog,
		tab:          metricsTabTimings,
		localMetrics: localMetrics,
	}
	o.handlers = map[command.ID]cmdHandler{
		command.NavDown:     func(repeat int) tea.Cmd { o.moveSelection(max(repeat, 1)); return nil },
		command.NavUp:       func(repeat int) tea.Cmd { o.moveSelection(-max(repeat, 1)); return nil },
		command.NavPageDown: func(int) tea.Cmd { o.moveSelection(metricsPageStep); return nil },
		command.NavPageUp:   func(int) tea.Cmd { o.moveSelection(-metricsPageStep); return nil },
	}
	if client == nil {
		o.err = fmt.Errorf("daemon connection unavailable")
		return o, nil
	}
	return o, o.refreshNow()
}

func (o *MetricsOverlay) Title() string { return "Daemon Metrics" }

func (o *MetricsOverlay) Handles() []command.ID { return handlerIDs(o.handlers) }

func (o *MetricsOverlay) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if handler, ok := o.handlers[id]; ok {
		return o, handler(repeat)
	}
	return o, nil
}

func (o *MetricsOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.String() {
	case "tab", "l", "right":
		o.advanceTab(1)
		return o, nil, true
	case "shift+tab", "h", "H", "left":
		o.advanceTab(-1)
		return o, nil, true
	case "r", "ctrl+r":
		if o.loading || o.client == nil {
			return o, nil, true
		}
		return o, o.refreshNow(), true
	case "home", "g g":
		o.setSelection(0)
		return o, nil, true
	case "end", "G":
		o.setSelection(o.rowCountCurrentTab() - 1)
		return o, nil, true
	}
	return o, nil, false
}

func (o *MetricsOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case metricsLoadedMsg:
		if m.requestID != o.requestID {
			return o, nil
		}
		o.loading = false
		o.err = m.err
		if m.err == nil {
			o.current = m.snapshot
			o.history = appendMetricsHistory(o.history, m.snapshot)
			o.clampSelection()
		}
		if o.client == nil {
			return o, nil
		}
		return o, o.waitRefresh()
	case metricsRefreshTickMsg:
		if o.loading || o.client == nil {
			return o, nil
		}
		return o, o.refreshNow()
	}
	return o, nil
}

func (o *MetricsOverlay) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, metricsOverlayWidthPct, metricsOverlayHeightPct, metricsMinWidth, metricsMinHeight)
}

func (o *MetricsOverlay) View(maxW, maxH int) string {
	var lines []string
	lines = append(lines, o.renderTabs(maxW))
	lines = append(lines, o.theme.Dim.Render(render.Truncate(o.statusLine(), maxW)))
	lines = append(lines, o.theme.Dim.Render(render.Truncate(historyLegendLine(), maxW)))
	if o.err != nil {
		lines = append(lines, o.theme.Error.Render(render.Truncate("Error: "+o.err.Error(), maxW)))
	}

	sections := o.sectionsForTab(o.tab)
	if len(sections) == 0 {
		if o.loading {
			lines = append(lines, o.theme.MutedItalic.Render("Loading daemon metrics..."))
		} else {
			lines = append(lines, o.theme.MutedItalic.Render("No metrics recorded yet."))
		}
		lines = append(lines, "")
		lines = append(lines, o.theme.Dim.Render("[tab/shift+tab or l/h] switch tabs  [r] refresh  [q/Esc] close"))
		return fitOverlayLines(lines, maxH)
	}

	renderLines := o.renderSectionLines(sections, maxW)
	bodyHeight := max(maxH-len(lines)-1, 1)
	start, end := o.visibleWindow(renderLines, bodyHeight)
	for i := start; i < end; i++ {
		line := renderLines[i]
		text := render.Truncate(line.text, maxW)
		if line.selected {
			lines = append(lines, o.theme.Selected.Render(text))
			continue
		}
		if line.row {
			lines = append(lines, o.theme.Row.Render(text))
			continue
		}
		lines = append(lines, o.theme.Header.Render(text))
	}
	lines = append(lines, o.theme.Dim.Render("[tab/shift+tab or l/h] switch tabs  [j/k] move  [r] refresh  [q/Esc] close"))
	return fitOverlayLines(lines, maxH)
}

func (o *MetricsOverlay) advanceTab(delta int) {
	next := (int(o.tab) + delta + int(metricsTabCount)) % int(metricsTabCount)
	o.tab = metricsTab(next)
	o.clampSelection()
}

func (o *MetricsOverlay) moveSelection(delta int) {
	o.setSelection(o.selected[o.tab] + delta)
}

func (o *MetricsOverlay) setSelection(index int) {
	count := o.rowCountCurrentTab()
	if count <= 0 {
		o.selected[o.tab] = 0
		return
	}
	o.selected[o.tab] = min(max(index, 0), count-1)
}

func (o *MetricsOverlay) clampSelection() {
	o.setSelection(o.selected[o.tab])
}

func (o *MetricsOverlay) rowCountCurrentTab() int {
	total := 0
	for _, section := range o.sectionsForTab(o.tab) {
		total += len(section.rows)
	}
	return total
}

func (o *MetricsOverlay) refreshNow() tea.Cmd {
	o.loading = true
	o.requestID++
	requestID := o.requestID
	client := o.client
	var localSnap *proto.MetricsSnapshot
	if o.localMetrics != nil {
		s := snapshotToProto(o.localMetrics.Snapshot())
		localSnap = &s
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), metricsFetchTimeout)
		defer cancel()
		if localSnap != nil {
			_ = client.Call(ctx, proto.MethodMetricsPush,
				proto.MetricsPushParams{Component: string(observability.MetricNamespaceTUI), Snapshot: *localSnap}, &proto.Empty{})
		}
		var res proto.MetricsResult
		err := client.Call(ctx, proto.MethodMetricsGet, nil, &res)
		return metricsLoadedMsg{requestID: requestID, snapshot: res.Metrics, err: err}
	}
}

// snapshotToProto converts an observability.Snapshot to the proto wire type.
func snapshotToProto(snap observability.Snapshot) proto.MetricsSnapshot {
	timings := make(map[string]proto.TimingMetric, len(snap.Timings))
	for k, v := range snap.Timings {
		avgNanos := v.AvgNanos()
		timings[k] = proto.TimingMetric{
			Count:      v.Count,
			Errors:     v.Errors,
			TotalNanos: v.TotalNanos,
			AvgNanos:   avgNanos,
			AvgMillis:  avgNanos / int64(time.Millisecond),
			MinNanos:   v.MinNanos,
			MinMillis:  v.MinNanos / int64(time.Millisecond),
			MaxNanos:   v.MaxNanos,
			MaxMillis:  v.MaxNanos / int64(time.Millisecond),
			LastNanos:  v.LastNanos,
			LastMillis: v.LastNanos / int64(time.Millisecond),
		}
	}
	return proto.MetricsSnapshot{
		Timestamp: snap.Timestamp,
		Timings:   timings,
		Counters:  snap.Counters,
		Gauges:    snap.Gauges,
	}
}

func (o *MetricsOverlay) waitRefresh() tea.Cmd {
	return tea.Tick(metricsRefreshInterval, func(time.Time) tea.Msg { return metricsRefreshTickMsg{} })
}

func (o *MetricsOverlay) statusLine() string {
	if o.current.Timestamp.IsZero() {
		if o.loading {
			return fmt.Sprintf("polling daemon every %s", metricsRefreshInterval)
		}
		return "waiting for daemon metrics"
	}
	status := fmt.Sprintf("updated %s  |  history %d samples  |  refresh %s", o.current.Timestamp.Local().Format("15:04:05"), len(o.history), metricsRefreshInterval)
	if o.loading {
		status += "  |  refreshing..."
	}
	return status
}

func (o *MetricsOverlay) renderTabs(maxW int) string {
	parts := make([]string, 0, len(metricsTabLabels))
	for i, label := range metricsTabLabels {
		text := " " + label + " "
		if metricsTab(i) == o.tab {
			parts = append(parts, o.theme.Selected.Render(text))
		} else {
			parts = append(parts, o.theme.RowAlt.Render(text))
		}
	}
	return render.Truncate(strings.Join(parts, " "), maxW)
}

func (o *MetricsOverlay) visibleWindow(lines []metricsRenderLine, height int) (start, end int) {
	if len(lines) <= height {
		return 0, len(lines)
	}
	selectedLine := 0
	for i, line := range lines {
		if line.selected {
			selectedLine = i
			break
		}
	}
	start = max(0, selectedLine-height/2)
	end = min(len(lines), start+height)
	if end-start < height {
		start = max(0, end-height)
	}
	return start, end
}

func (o *MetricsOverlay) renderSectionLines(sections []metricsSection, maxW int) []metricsRenderLine {
	nameWidth := max(24, min(maxW/2, 60))
	selectedRow := o.selected[o.tab]
	rowIndex := 0
	lines := make([]metricsRenderLine, 0, len(sections)*4)
	for _, section := range sections {
		lines = append(lines, metricsRenderLine{text: section.title})
		if header := metricsSummaryHeader(o.tab, nameWidth); header != "" {
			lines = append(lines, metricsRenderLine{text: header})
		}
		for _, row := range section.rows {
			line := fmt.Sprintf("  %s  %s  %s", render.Pad(row.name, nameWidth), row.summary, row.history)
			lines = append(lines, metricsRenderLine{text: line, selected: rowIndex == selectedRow, row: true})
			rowIndex++
		}
		lines = append(lines, metricsRenderLine{text: ""})
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1].text) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func (o *MetricsOverlay) sectionsForTab(tab metricsTab) []metricsSection {
	switch tab {
	case metricsTabCounters:
		return buildCounterSections(o.current, o.history)
	case metricsTabGauges:
		return buildGaugeSections(o.current, o.history)
	default:
		return buildTimingSections(o.current, o.history)
	}
}

func buildTimingSections(current proto.MetricsSnapshot, history []proto.MetricsSnapshot) []metricsSection {
	groups := make(map[string][]metricsRow)
	for name, metric := range current.Timings {
		group := metricsGroup(name)
		groups[group] = append(groups[group], metricsRow{
			name:       formatMetricLabel(name),
			group:      group,
			tags:       metricTags(name),
			summary:    formatTimingSummaryColumns(metric),
			history:    fmt.Sprintf("hist %s", sparkline(timingIntervalSeries(history, name), metricsBarWidth)),
			importance: timingImportance(metric),
		})
	}
	return metricSections(groups)
}

func buildCounterSections(current proto.MetricsSnapshot, history []proto.MetricsSnapshot) []metricsSection {
	groups := make(map[string][]metricsRow)
	for name, value := range current.Counters {
		group := metricsGroup(name)
		delta := latestCounterDelta(history, name)
		groups[group] = append(groups[group], metricsRow{
			name:       formatMetricLabel(name),
			group:      group,
			tags:       metricTags(name),
			summary:    fmt.Sprintf("value %d  delta %+d", value, delta),
			history:    fmt.Sprintf("hist %s", sparkline(counterDeltaSeries(history, name), metricsBarWidth)),
			importance: counterImportance(value, delta),
		})
	}
	return metricSections(groups)
}

func buildGaugeSections(current proto.MetricsSnapshot, history []proto.MetricsSnapshot) []metricsSection {
	groups := make(map[string][]metricsRow)
	for name, value := range current.Gauges {
		group := metricsGroup(name)
		trend := latestGaugeTrend(history, name)
		groups[group] = append(groups[group], metricsRow{
			name:       formatMetricLabel(name),
			group:      group,
			tags:       metricTags(name),
			summary:    fmt.Sprintf("value %d  trend %+d", value, trend),
			history:    fmt.Sprintf("hist %s", sparkline(gaugeSeries(history, name), metricsBarWidth)),
			importance: gaugeImportance(value, trend),
		})
	}
	return metricSections(groups)
}

func metricSections(groups map[string][]metricsRow) []metricsSection {
	sections := make([]metricsSection, 0, len(groups))
	for _, group := range metricsGroupOrder() {
		rows := groups[group]
		if len(rows) == 0 {
			continue
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].importance == rows[j].importance {
				return rows[i].name < rows[j].name
			}
			return rows[i].importance > rows[j].importance
		})
		sections = append(sections, metricsSection{title: strings.ToUpper(group), rows: rows})
	}
	return sections
}

func metricsGroupOrder() []string {
	namespaces := observability.MetricNamespaceOrder()
	out := make([]string, len(namespaces))
	for i, namespace := range namespaces {
		out[i] = string(namespace)
	}
	return out
}

func metricsGroup(name string) string {
	for _, group := range metricsGroupOrder()[:len(metricsGroupOrder())-1] {
		if strings.HasPrefix(name, group+".") || name == group {
			return group
		}
	}
	// Special-case common resolution-related prefixes so they don't all fall to "other"
	if strings.HasPrefix(name, "uspto.") {
		return "uspto"
	}
	if strings.HasPrefix(name, "crawl.") {
		return "crawl"
	}
	return string(observability.MetricNamespaceOther)
}

func metricTags(name string) []string {
	seen := map[string]bool{metricsGroup(name): true}
	tags := []string{metricsGroup(name)}
	for _, part := range strings.Split(name, ".") {
		tag := normalizeMetricToken(part)
		if tag == "" || seen[tag] {
			continue
		}
		if tag == "total" || tag == "method" || tag == "sqlite" {
			if tag == "sqlite" {
				tags = append(tags, tag)
				seen[tag] = true
			}
			continue
		}
		tags = append(tags, tag)
		seen[tag] = true
		if len(tags) == 4 {
			break
		}
	}
	return tags
}

func normalizeMetricToken(token string) string {
	token = strings.ToLower(strings.TrimSpace(token))
	switch token {
	case "sort_tags":
		return "sort-tags"
	case "queue_depth":
		return "queue"
	case "error_total", "errors":
		return "error"
	case "list_patents":
		return "list"
	case "relations":
		return "relations"
	}
	token = strings.ReplaceAll(token, "_", "-")
	return token
}

func formatMetricLabel(name string) string {
	tags := metricTags(name)
	if len(tags) == 0 {
		return name
	}
	parts := make([]string, len(tags))
	for i, tag := range tags {
		parts[i] = "[" + tag + "]"
	}
	return name + " " + strings.Join(parts, "")
}

func metricsSummaryHeader(tab metricsTab, nameWidth int) string {
	switch tab {
	case metricsTabTimings:
		return fmt.Sprintf("  %s  %7s  %7s  %7s  %6s  %6s  HIST", render.Pad("", nameWidth), "AVG", "MAX", "LAST", "COUNT", "ERR")
	default:
		return ""
	}
}

func formatTimingSummary(metric proto.TimingMetric) string {
	return fmt.Sprintf(
		"avg %s  max %s  last %s  cnt %5s  err %4s",
		formatMillisColumn(metric.AvgMillis),
		formatMillisColumn(metric.MaxMillis),
		formatMillisColumn(metric.LastMillis),
		formatCountCompact(metric.Count),
		formatCountCompact(metric.Errors),
	)
}

func formatTimingSummaryColumns(metric proto.TimingMetric) string {
	return fmt.Sprintf(
		"%7s  %7s  %7s  %6s  %6s",
		formatMillisColumn(metric.AvgMillis),
		formatMillisColumn(metric.MaxMillis),
		formatMillisColumn(metric.LastMillis),
		formatCountColumn(metric.Count),
		formatCountColumn(metric.Errors),
	)
}

func formatMillisColumn(ms int64) string {
	return fmt.Sprintf("[%5s]", formatMillisCompact(ms))
}

func formatMillisCompact(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if ms >= 100_000 {
		return fmt.Sprintf("%.0fs", float64(ms)/1000)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

func formatCountColumn(v int64) string {
	return fmt.Sprintf("[%4s]", formatCountCompact(v))
}

func formatCountCompact(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}

	var out string
	switch {
	case v < 10_000:
		out = fmt.Sprintf("%d", v)
	case v < 1_000_000:
		out = fmt.Sprintf("%dk", v/1000)
	default:
		whole := v / 1_000_000
		frac := (v % 1_000_000) / 100_000
		if whole >= 10 || frac == 0 {
			out = fmt.Sprintf("%dm", whole)
		} else {
			out = fmt.Sprintf("%d.%dm", whole, frac)
		}
	}

	if neg {
		return "-" + out
	}
	return out
}

func timingImportance(metric proto.TimingMetric) int64 {
	return metric.Errors*1_000_000 + metric.LastMillis*1000 + metric.MaxMillis*100 + metric.AvgMillis*10 + metric.Count
}

func counterImportance(value, delta int64) int64 {
	return abs64(delta)*10_000 + abs64(value)
}

func gaugeImportance(value, trend int64) int64 {
	return abs64(trend)*10_000 + abs64(value)
}

func timingIntervalSeries(history []proto.MetricsSnapshot, name string) []float64 {
	if len(history) == 0 {
		return nil
	}
	series := make([]float64, 0, len(history))
	for i := 1; i < len(history); i++ {
		prev, okPrev := history[i-1].Timings[name]
		curr, okCurr := history[i].Timings[name]
		if !okCurr || !okPrev {
			continue
		}
		deltaCount := curr.Count - prev.Count
		deltaTotal := curr.TotalNanos - prev.TotalNanos
		if deltaCount <= 0 || deltaTotal < 0 {
			continue
		}
		series = append(series, float64(deltaTotal)/float64(deltaCount)/float64(time.Millisecond))
	}
	if len(series) == 0 {
		if metric, ok := history[len(history)-1].Timings[name]; ok {
			series = append(series, float64(metric.LastMillis))
		}
	}
	return series
}

func counterDeltaSeries(history []proto.MetricsSnapshot, name string) []float64 {
	if len(history) == 0 {
		return nil
	}
	series := make([]float64, 0, len(history))
	for i := 1; i < len(history); i++ {
		prev := history[i-1].Counters[name]
		curr, ok := history[i].Counters[name]
		if !ok {
			continue
		}
		series = append(series, float64(curr-prev))
	}
	if len(series) == 0 {
		series = append(series, float64(history[len(history)-1].Counters[name]))
	}
	return series
}

func gaugeSeries(history []proto.MetricsSnapshot, name string) []float64 {
	if len(history) == 0 {
		return nil
	}
	series := make([]float64, 0, len(history))
	for _, snapshot := range history {
		value, ok := snapshot.Gauges[name]
		if !ok {
			continue
		}
		series = append(series, float64(value))
	}
	return series
}

func latestCounterDelta(history []proto.MetricsSnapshot, name string) int64 {
	series := counterDeltaSeries(history, name)
	if len(series) == 0 {
		return 0
	}
	return int64(series[len(series)-1])
}

func latestGaugeTrend(history []proto.MetricsSnapshot, name string) int64 {
	if len(history) < 2 {
		if len(history) == 1 {
			return history[0].Gauges[name]
		}
		return 0
	}
	return history[len(history)-1].Gauges[name] - history[len(history)-2].Gauges[name]
}

func sparkline(series []float64, width int) string {
	if len(series) == 0 {
		return strings.Repeat("·", width)
	}
	ramp := []rune("▁▂▃▄▅▆▇█")
	if len(series) > width {
		series = series[len(series)-width:]
	}
	minValue, maxValue := series[0], series[0]
	for _, value := range series[1:] {
		minValue = math.Min(minValue, value)
		maxValue = math.Max(maxValue, value)
	}
	if maxValue == minValue {
		return strings.Repeat(string(ramp[len(ramp)/2]), width-len(series)) + strings.Repeat(string(ramp[len(ramp)/2]), len(series))
	}
	var b strings.Builder
	if pad := width - len(series); pad > 0 {
		b.WriteString(strings.Repeat("·", pad))
	}
	for _, value := range series {
		scaled := int(math.Round((value - minValue) / (maxValue - minValue) * float64(len(ramp)-1)))
		scaled = min(max(scaled, 0), len(ramp)-1)
		b.WriteRune(ramp[scaled])
	}
	return b.String()
}

func historyLegendLine() string {
	return "History: ▁ low  █ high  · no sample"
}

func appendMetricsHistory(history []proto.MetricsSnapshot, snapshot proto.MetricsSnapshot) []proto.MetricsSnapshot {
	history = append(history, snapshot)
	if len(history) > metricsHistoryLimit {
		history = history[len(history)-metricsHistoryLimit:]
	}
	return history
}

func fitOverlayLines(lines []string, maxH int) string {
	if len(lines) > maxH {
		lines = lines[:maxH]
	}
	for len(lines) < maxH {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
