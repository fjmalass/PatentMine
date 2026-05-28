package overlay

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/tui/render"
)

// LoadingCompareSourcesMsg asks the App to open the source comparison overlay
// for the loading overlay's root patent. It is emitted when the user presses C
// after a multi-source lookup completed with at least two contributing
// providers, so the diff overlay can be reached without leaving the popup.
type LoadingCompareSourcesMsg struct {
	Patent string
}

type LoadingCloseMsg struct {
	Patent string
}

// Loading is a modal overlay that shows a throbber, progress bar, and ETA
// while one or more background daemon jobs (like crawls) are running.
// Single-job and multi-job modes are both supported.
type Loading struct {
	theme    render.Theme
	jobIDs   []string
	title    string
	message  string
	isLookup bool // when true, hide relations breakdown (parents/children etc.)

	// rootPatent is the patent number whose lookup spawned this overlay (when
	// known). It is only set for single-patent flows like :add — multi-job
	// lookups leave it empty. Used by the "press C to compare" affordance to
	// know which patent to reconcile.
	rootPatent string

	// sourceMode is the daemon's runtime provider policy in effect for this
	// crawl (compare / uspto-first / uspto-only / google-only). Carried on the
	// CrawlProgress events; the done view uses it to explain why a provider is
	// absent (skipped by mode vs. tried but no result).
	sourceMode string

	// recordResolved is the canonical record number the root resolved to (the
	// crawler may rewrite a grant number to its application). Empty until at
	// least one CrawlProgress event for the root arrives.
	recordResolved string

	// membershipsAdded is the count of depth-1 neighbor stubs that received
	// project membership through the engine's auto-assign goroutine. Populated
	// when the EventMembershipAutoAssign event arrives.
	membershipsAdded int
	membershipsKnown bool // set once the auto-assign event lands

	// Per-job progress; keyed by job ID so progress events from concurrent
	// jobs are aggregated independently.
	progresses map[string]proto.CrawlProgress
	doneCount  int // number of jobs that have finished
	doneErrors []string

	// Numbers seen across all progress events for this job set; ordered by
	// first-seen and deduped so the done view can list them.
	savedNumbers  []string
	savedSet      map[string]bool
	stubNumbers   []string
	stubSet       map[string]bool
	recordSources map[string][]string // recordNumber → ordered unique source names

	spinner    spinner.Model
	finished   bool // true when all jobs are done; overlay stays open until user dismisses
	finishedAt time.Time

	startTime time.Time
	lastTime  time.Time
	lastTotal int // summed CrawledCount from last progress event
	eta       time.Duration
}

// NewLoading builds a loading overlay for one or more jobs.
func NewLoading(theme render.Theme, jobIDs []string, title string, isLookup ...bool) *Loading {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	if jobIDs == nil {
		jobIDs = []string{}
	}
	lk := false
	if len(isLookup) > 0 {
		lk = isLookup[0]
	}
	return &Loading{
		theme:         theme,
		jobIDs:        jobIDs,
		title:         title,
		isLookup:      lk,
		message:       "Starting…",
		progresses:    make(map[string]proto.CrawlProgress, len(jobIDs)),
		savedSet:      make(map[string]bool),
		stubSet:       make(map[string]bool),
		recordSources: make(map[string][]string),
		spinner:       s,
		startTime:     time.Now(),
		lastTime:      time.Now(),
	}
}

// WithRoot tags the overlay with the patent it is loading. The compare
// affordance becomes active once the lookup completes and at least two
// providers contributed data for that record.
func (l *Loading) WithRoot(patent string) *Loading {
	l.rootPatent = patent
	return l
}

// canCompare reports whether the overlay should offer the "compare sources"
// affordance — it needs a known root patent and at least two distinct provider
// snapshots saved for it.
func (l *Loading) canCompare() bool {
	if l.rootPatent == "" {
		return false
	}
	var sources []string
	sources = append(sources, l.recordSources[l.rootPatent]...)
	if l.recordResolved != "" {
		sources = append(sources, l.recordSources[l.recordResolved]...)
	}
	seen := map[string]bool{}
	for _, s := range sources {
		seen[canonicalProvider(s)] = true
	}
	return len(seen) >= 2
}

func (l *Loading) Title() string { return l.title }

func (l *Loading) Init() tea.Cmd {
	return l.spinner.Tick
}

// OverlaySize implements DynamicSize. For the multi-selection "L" (lookup)
// case we deliberately request a tall box (~65% of screen height) so that
// progress details + the rich "done" summary (errors, provider breakdown,
// stub lists, etc.) have room and long messages/errors can be wrapped instead
// of brutally truncated.
func (l *Loading) OverlaySize(termW, termH int) (int, int) {
	if len(l.jobIDs) > 1 || l.isLookup {
		// Multi-selection "L" (batch lookup / "Looking up N patents") gets
		// a wide, substantial panel (not the old 76x22 toy box).
		// We go very wide so long daemon error messages and lists actually
		// fit and wrap nicely. Height is generous but not a forced 65%
		// empty frame — we want the content to feel substantial without
		// ridiculous wasted space inside the borders.
		return PctSize(termW, termH, 90, 42, 70, 20)
	}
	// Single-job case still gets more space than the old hard 22-line cap.
	return PctSize(termW, termH, 60, 48, 38, 12)
}

func (l *Loading) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if l.finished && (id == command.CloseOverlay || id == command.Back) {
		return l, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return l, nil
}

func (l *Loading) Handles() []command.ID {
	if l.finished {
		return []command.ID{command.CloseOverlay, command.Back}
	}
	return nil
}

func (l *Loading) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if msg.String() == "ctrl+c" {
		return l, tea.Quit, true
	}
	if !l.finished {
		return l, nil, false
	}
	switch msg.String() {
	case "esc", "enter", "q":
		patent := l.rootPatent
		if l.recordResolved != "" {
			patent = l.recordResolved
		}
		return l, func() tea.Msg {
			return LoadingCloseMsg{
				Patent: patent,
			}
		}, true
	case "c", "C":
		if l.canCompare() {
			patent := l.rootPatent
			if l.recordResolved != "" {
				patent = l.recordResolved
			}
			return l, func() tea.Msg { return LoadingCompareSourcesMsg{Patent: patent} }, true
		}
	}
	return l, nil, false
}

// matchJob reports whether an event belongs to one of the tracked jobs.
func (l *Loading) matchJob(jobID string) bool {
	return slices.Contains(l.jobIDs, jobID)
}

func (l *Loading) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		l.spinner, cmd = l.spinner.Update(m)
		return l, cmd
	case proto.Event:
		switch m.Method {
		case proto.EventCrawlProgress:
			var p proto.CrawlProgress
			if err := json.Unmarshal(m.Params, &p); err == nil && l.matchJob(p.JobID) {
				l.progresses[p.JobID] = p
				l.message = p.Message
				if p.Mode != "" {
					l.sourceMode = p.Mode
				}
				if p.RecordNumber != "" {
					if l.rootPatent != "" && l.recordResolved == "" && p.RecordNumber != l.rootPatent {
						l.recordResolved = p.RecordNumber
					}
					if !l.savedSet[p.RecordNumber] {
						l.savedSet[p.RecordNumber] = true
						l.savedNumbers = append(l.savedNumbers, p.RecordNumber)
					}
					existing := l.recordSources[p.RecordNumber]
					seenSrc := make(map[string]bool, len(existing))
					for _, s := range existing {
						seenSrc[s] = true
					}
					for _, s := range p.Sources {
						if s == "" || seenSrc[s] {
							continue
						}
						seenSrc[s] = true
						existing = append(existing, s)
					}
					l.recordSources[p.RecordNumber] = existing
				}
				for _, s := range p.Stubs {
					if !l.stubSet[s] {
						l.stubSet[s] = true
						l.stubNumbers = append(l.stubNumbers, s)
					}
				}

				totalCrawled := l.sumProgress(func(p proto.CrawlProgress) int { return p.CrawledCount })
				totalJobs := len(l.jobIDs)

				if totalCrawled > 0 {
					l.message = fmt.Sprintf("%s (%d crawled)", p.Message, totalCrawled)
				}

				now := time.Now()
				elapsed := now.Sub(l.startTime)
				denom := totalJobs
				if !l.isLookup {
					totalDiscovered := l.sumProgress(func(p proto.CrawlProgress) int { return p.DiscoveredCount })
					denom = max(totalDiscovered, totalJobs)
				}
				if totalCrawled > l.lastTotal && elapsed.Seconds() > 1 {
					rate := float64(totalCrawled) / elapsed.Seconds()
					remaining := float64(max(denom-totalCrawled, 0))
					if remaining > 0 && rate > 0 {
						l.eta = time.Duration(remaining/rate) * time.Second
					}
				}
				l.lastTime = now
				l.lastTotal = totalCrawled
			}
		case proto.EventCrawlDone:
			var d proto.CrawlDone
			if err := json.Unmarshal(m.Params, &d); err == nil && l.matchJob(d.JobID) {
				l.doneCount++
				if d.Error != "" {
					l.doneErrors = append(l.doneErrors, d.Error)
				}
				if l.doneCount >= len(l.jobIDs) {
					l.finished = true
					l.finishedAt = time.Now()
				}
			}
		case proto.EventMembershipAutoAssign:
			var ma proto.MembershipAutoAssign
			if err := json.Unmarshal(m.Params, &ma); err == nil && l.matchJob(ma.JobID) {
				l.membershipsAdded = ma.Added
				l.membershipsKnown = true
			}
		}
	}
	return l, nil
}

// sumProgress adds up a field across all tracked jobs.
func (l *Loading) sumProgress(fn func(proto.CrawlProgress) int) int {
	var total int
	for _, p := range l.progresses {
		total += fn(p)
	}
	return total
}

func (l *Loading) depthLabel() string {
	current, maxDepth := 0, 0
	for _, p := range l.progresses {
		if p.MaxDepth > maxDepth {
			maxDepth = p.MaxDepth
		}
		if p.Depth > current {
			current = p.Depth
		}
	}
	if maxDepth <= 0 {
		return ""
	}
	return fmt.Sprintf("  depth %d/%d", current, maxDepth)
}

func (l *Loading) View(w, h int) string {
	if l.finished {
		return l.viewDone(w)
	}
	var b strings.Builder
	b.WriteString("\n")
	// Wrap the status message (instead of hard truncate) so long progress
	// descriptions from the daemon don't get cut off in the multi-L popup.
	msg := lipgloss.NewStyle().Width(w - 6).Render(l.message)
	for i, line := range strings.Split(msg, "\n") {
		if i == 0 {
			b.WriteString("  " + l.spinner.View() + " " + line)
		} else {
			b.WriteString("    " + line) // continuation indent
		}
		b.WriteByte('\n')
	}

	totalCrawled := l.sumProgress(func(p proto.CrawlProgress) int { return p.CrawledCount })
	totalJobs := len(l.jobIDs)

	if l.isLookup {
		denom := max(totalJobs, 1)
		pct := float64(totalCrawled) / float64(denom)
		barWidth := min(w-16, 30)
		label := fmt.Sprintf(" %d/%d", totalCrawled, denom)
		if totalJobs > 1 {
			label = fmt.Sprintf(" %d/%d (%d patents)", totalCrawled, denom, totalJobs)
		}
		if barWidth > 0 {
			filled := int(math.Round(pct * float64(barWidth)))
			filled = max(filled, 0)
			filled = min(filled, barWidth)
			bar := "[" + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + "]"

			etaStr := ""
			if l.eta > 0 {
				etaStr = "  (~" + formatDuration(l.eta) + " remaining)"
			}
			line := l.theme.Dim.Render(bar + label + etaStr)
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	} else {
		totalDiscovered := l.sumProgress(func(p proto.CrawlProgress) int { return p.DiscoveredCount })
		depthLabel := l.depthLabel()

		if totalDiscovered > 0 {
			pct := float64(totalCrawled) / float64(totalDiscovered)
			barWidth := min(w-16, 30)
			nJobs := len(l.jobIDs)
			label := fmt.Sprintf(" %d/%d", totalCrawled, totalDiscovered)
			if nJobs > 1 {
				label = fmt.Sprintf(" %d/%d (%d patents)", totalCrawled, totalDiscovered, nJobs)
			}
			label += depthLabel
			if barWidth > 0 {
				filled := int(math.Round(pct * float64(barWidth)))
				filled = max(filled, 0)
				filled = min(filled, barWidth)
				bar := "[" + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + "]"

				etaStr := ""
				if l.eta > 0 {
					etaStr = "  (~" + formatDuration(l.eta) + " remaining)"
				}
				line := l.theme.Dim.Render(bar + label + etaStr)
				b.WriteString("  ")
				b.WriteString(line)
				b.WriteByte('\n')
			}
		}

		if totalDiscovered > 0 {
			totalPending := l.sumProgress(func(p proto.CrawlProgress) int { return p.PendingCount })
			totalCites := l.sumProgress(func(p proto.CrawlProgress) int { return p.CitationsCount })
			totalCitedBy := l.sumProgress(func(p proto.CrawlProgress) int { return p.CitedByCount })
			totalParents := l.sumProgress(func(p proto.CrawlProgress) int { return p.ParentsCount })
			totalChildren := l.sumProgress(func(p proto.CrawlProgress) int { return p.ChildrenCount })

			var parts []string
			parts = append(parts, fmt.Sprintf("discovered: %d", totalDiscovered))
			if totalPending > 0 {
				parts = append(parts, fmt.Sprintf("pending: %d", totalPending))
			}
			if totalCites > 0 {
				parts = append(parts, fmt.Sprintf("cites: %d", totalCites))
			}
			if totalCitedBy > 0 {
				parts = append(parts, fmt.Sprintf("cited-by: %d", totalCitedBy))
			}
			if totalParents > 0 {
				parts = append(parts, fmt.Sprintf("parents: %d", totalParents))
			}
			if totalChildren > 0 {
				parts = append(parts, fmt.Sprintf("children: %d", totalChildren))
			}
			if len(parts) > 0 {
				b.WriteString("     ")
				b.WriteString(l.theme.Dim.Render(strings.Join(parts, " · ")))
				b.WriteByte('\n')
			}
		}
	}

	b.WriteByte('\n')
	if len(l.jobIDs) == 1 {
		b.WriteString(l.theme.Dim.Render(render.Pad("JobID: "+l.jobIDs[0], w-2)))
	} else {
		b.WriteString(l.theme.Dim.Render(render.Pad(fmt.Sprintf("%d jobs", len(l.jobIDs)), w-2)))
	}
	return b.String()
}

func (l *Loading) viewDone(w int) string {
	var b strings.Builder
	b.WriteString("\n")
	elapsed := l.finishedAt.Sub(l.startTime).Round(time.Millisecond)
	if len(l.doneErrors) > 0 {
		b.WriteString("  " + l.theme.Error.Render("Failed") + "\n")
		for _, e := range l.doneErrors {
			// Wrap instead of truncate so long error messages (e.g. from
			// many patents in a multi-L) remain readable.
			wrapped := lipgloss.NewStyle().Width(w - 4).Render(e)
			for _, line := range strings.Split(wrapped, "\n") {
				b.WriteString("  " + line + "\n")
			}
		}
	} else {
		totalCrawled := l.sumProgress(func(p proto.CrawlProgress) int { return p.CrawledCount })
		newStubs := len(l.stubNumbers)
		summary := fmt.Sprintf("Done in %s", formatDuration(elapsed))
		if totalCrawled > 0 {
			summary += fmt.Sprintf(" · %d patent(s) saved", totalCrawled)
		}
		if newStubs > 0 {
			summary += fmt.Sprintf(" · %d new stub(s)", newStubs)
		}
		summaryStyle := l.theme.OK
		if totalCrawled == 0 {
			summaryStyle = l.theme.Warn
		}
		b.WriteString("  " + summaryStyle.Render(summary) + "\n")

		if l.rootPatent != "" && l.recordResolved != "" {
			b.WriteString("  " + l.theme.Dim.Render(fmt.Sprintf("resolved: %s → %s", l.rootPatent, l.recordResolved)) + "\n")
		}
		if l.sourceMode != "" {
			b.WriteString("  " + l.theme.Dim.Render("mode: "+l.sourceMode) + "\n")
		}
		for _, line := range l.providerStatusLines() {
			b.WriteString("  " + line + "\n")
		}
		if l.membershipsKnown && l.membershipsAdded > 0 {
			b.WriteString("  " + l.theme.OK.Render(fmt.Sprintf("memberships granted: %d neighbor(s)", l.membershipsAdded)) + "\n")
		} else if l.membershipsKnown {
			b.WriteString("  " + l.theme.Dim.Render("memberships granted: 0 (all neighbors already in project, or no active project)") + "\n")
		}
		if len(l.stubNumbers) > 0 {
			b.WriteString("  " + l.theme.Dim.Render("new stubs: "+formatNumberList(l.stubNumbers, w-12)) + "\n")
		}
		if totalCrawled == 0 {
			b.WriteString("  " + l.theme.Warn.Render("No patent body saved — record left as a stub.") + "\n")
			b.WriteString("  " + l.theme.Dim.Render("Try :add.uspto or :add.google to force one provider, or press L to retry.") + "\n")
		} else if l.isLookup {
			b.WriteString("  " + l.theme.Dim.Render("Navigate to the patent in your catalog to see the loaded data.") + "\n")
			b.WriteString("  " + l.theme.Dim.Render("Press L on any stub patent to re-fetch its data.") + "\n")
		}
	}
	b.WriteByte('\n')
	if l.canCompare() {
		b.WriteString("  " + l.theme.OK.Render("USPTO + Google both contributed — press C to compare/edit/validate diffs") + "\n")
	}
	b.WriteString(l.theme.Dim.Render(render.Pad("press Esc to close", w-2)))
	return b.String()
}

// providerStatusLines returns one styled line per known provider explaining
// whether it contributed data, was tried with no result, or was skipped by the
// active source-mode policy.
func (l *Loading) providerStatusLines() []string {
	if l.rootPatent == "" {
		return nil
	}
	contributed := map[domain.Source]bool{}
	collect := func(record string) {
		for _, s := range l.recordSources[record] {
			contributed[domain.Source(canonicalProvider(s))] = true
		}
	}
	collect(l.rootPatent)
	if l.recordResolved != "" {
		collect(l.recordResolved)
	}
	providers := []domain.Source{domain.SourceUSPTO, domain.SourceGoogle}
	var lines []string
	for _, p := range providers {
		status, style := l.providerStatus(p, contributed[p])
		lines = append(lines, style.Render(fmt.Sprintf("%-7s %s", strings.ToUpper(string(p))+":", status)))
	}
	return lines
}

// providerStatus returns the human-readable state of one provider given the
// active source mode and whether the provider actually saved data.
func (l *Loading) providerStatus(provider domain.Source, contributed bool) (string, lipgloss.Style) {
	if contributed {
		return "saved", l.theme.OK
	}
	mode := domain.SourceMode(l.sourceMode)
	skipped := (mode == domain.SourceModeUSPTOOnly && provider == domain.SourceGoogle) ||
		(mode == domain.SourceModeGoogleOnly && provider == domain.SourceUSPTO)
	if skipped {
		return "skipped (mode=" + string(mode) + ")", l.theme.Dim
	}
	if mode == domain.SourceModeCompare {
		return "not loaded", l.theme.Warn
	}
	return "no result", l.theme.Dim
}

// canonicalProvider folds snapshot source strings like "uspto_file_wrapper" or
// "uspto_grant_xml" back to the provider that emitted them ("uspto"), so the
// popup groups by USPTO vs Google rather than by parser variant.
func canonicalProvider(s string) string {
	if i := strings.Index(s, "_"); i > 0 {
		return s[:i]
	}
	return s
}

// savedBySourceLines groups saved record numbers by source. A record present in
// multiple sources is listed under each one so the user can see exactly what
// every provider returned.
func (l *Loading) savedBySourceLines(maxWidth int) []string {
	if len(l.savedNumbers) == 0 {
		return nil
	}
	// Collect ordered list of sources as they first appear, plus an "unknown"
	// bucket for saved records that arrived without a source label.
	var sourceOrder []string
	byOrder := map[string]bool{}
	bySource := map[string][]string{}
	const unknown = "(unknown source)"
	for _, num := range l.savedNumbers {
		sources := l.recordSources[num]
		if len(sources) == 0 {
			if !byOrder[unknown] {
				byOrder[unknown] = true
				sourceOrder = append(sourceOrder, unknown)
			}
			bySource[unknown] = append(bySource[unknown], num)
			continue
		}
		// Fold parser-specific names back to provider before grouping so
		// "uspto_file_wrapper" and "uspto_grant_xml" both count as USPTO.
		seenForRecord := map[string]bool{}
		for _, raw := range sources {
			s := canonicalProvider(raw)
			if seenForRecord[s] {
				continue
			}
			seenForRecord[s] = true
			if !byOrder[s] {
				byOrder[s] = true
				sourceOrder = append(sourceOrder, s)
			}
			bySource[s] = append(bySource[s], num)
		}
	}
	lines := make([]string, 0, len(sourceOrder))
	for _, s := range sourceOrder {
		label := strings.ToUpper(s) + " saved: "
		lines = append(lines, label+formatNumberList(bySource[s], maxWidth-len(label)))
	}
	return lines
}

// formatNumberList joins patent numbers, truncating with a "(+N more)" tail
// when the inline form would overflow the popup width.
func formatNumberList(nums []string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 1
	}
	if len(nums) == 0 {
		return ""
	}
	full := strings.Join(nums, ", ")
	if len(full) <= maxWidth {
		return full
	}
	for keep := len(nums) - 1; keep > 0; keep-- {
		tail := fmt.Sprintf(" (+%d more)", len(nums)-keep)
		head := strings.Join(nums[:keep], ", ")
		if len(head)+len(tail) <= maxWidth {
			return head + tail
		}
	}
	tail := fmt.Sprintf("(+%d more)", len(nums)-1)
	return nums[0] + " " + tail
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m >= 60 {
		h := m / 60
		m = m % 60
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
}
