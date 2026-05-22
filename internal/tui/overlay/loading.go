package overlay

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/command"
	"patentmine/internal/proto"
	"patentmine/internal/tui/render"
)

// Loading is a modal overlay that shows a throbber, progress bar, and ETA
// while one or more background daemon jobs (like crawls) are running.
// Single-job and multi-job modes are both supported.
type Loading struct {
	theme    render.Theme
	jobIDs   []string
	title    string
	message  string
	isLookup bool // when true, hide relations breakdown (parents/children etc.)

	// Per-job progress; keyed by job ID so progress events from concurrent
	// jobs are aggregated independently.
	progresses map[string]proto.CrawlProgress
	doneCount  int // number of jobs that have finished

	spinner  spinner.Model
	finished bool // true when all jobs are done

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
		theme:      theme,
		jobIDs:     jobIDs,
		title:      title,
		isLookup:   lk,
		message:    "Starting…",
		progresses: make(map[string]proto.CrawlProgress, len(jobIDs)),
		spinner:    s,
		startTime:  time.Now(),
		lastTime:   time.Now(),
	}
}

func (l *Loading) Title() string { return l.title }

func (l *Loading) Init() tea.Cmd {
	return l.spinner.Tick
}

func (l *Loading) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	return l, nil
}

func (l *Loading) Handles() []command.ID { return nil }

// matchJob reports whether an event belongs to one of the tracked jobs.
func (l *Loading) matchJob(jobID string) bool {
	for _, id := range l.jobIDs {
		if id == jobID {
			return true
		}
	}
	return false
}

func (l *Loading) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		if m.String() == "ctrl+c" {
			return l, tea.Quit
		}
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
				if l.doneCount >= len(l.jobIDs) {
					l.finished = true
					return l, func() tea.Msg { return CloseOverlayMsg{} }
				}
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
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + l.spinner.View() + " " + render.Truncate(l.message, w-6))
	b.WriteByte('\n')

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
