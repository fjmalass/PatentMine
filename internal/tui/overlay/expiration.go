package overlay

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/tui/render"
)

// ExpirationRefreshMsg requests a re-computation/refresh of expiration data.
type ExpirationRefreshMsg struct {
	Number domain.PatentNumber
}

// ExpirationOverlay displays the USPTO expiration calculation result in a popup.
type ExpirationOverlay struct {
	theme  render.Theme
	result proto.USPTOExpirationCalculateResult
}

var _ Overlay = (*ExpirationOverlay)(nil)

func NewExpirationOverlay(theme render.Theme, result proto.USPTOExpirationCalculateResult) *ExpirationOverlay {
	return &ExpirationOverlay{theme: theme, result: result}
}

func (o *ExpirationOverlay) Title() string { return "Patent Expiration Analysis" }

func (o *ExpirationOverlay) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *ExpirationOverlay) Handles() []command.ID { return nil }

func (o *ExpirationOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.String() {
	case "esc", "q", "enter":
		return o, func() tea.Msg { return PromptCloseMsg{} }, true
	case "r", "R":
		if pNum, err := domain.ParsePatentNumber(o.result.PatentNumber); err == nil {
			return o, func() tea.Msg { return ExpirationRefreshMsg{Number: pNum} }, true
		}
	}
	return o, nil, false
}

func (o *ExpirationOverlay) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, 70, 55, 54, 22)
}

func (o *ExpirationOverlay) View(maxW, maxH int) string {
	r := o.result
	sep := strings.Repeat("─", maxW)
	labelW := 30

	var b strings.Builder

	fmt.Fprintf(&b, "%s  %s\n", o.theme.Title.Render("Patent:"), r.PatentNumber)
	fmt.Fprintf(&b, "%s  %s\n", o.theme.Title.Render("Application:"), r.ApplicationNumber)
	if r.Title != "" {
		fmt.Fprintf(&b, "%s  %s\n", o.theme.Title.Render("Title:"), r.Title)
	}
	b.WriteString(sep + "\n")

	// USPTO section
	b.WriteString(o.theme.Header.Render("USPTO Data") + "\n")
	writeField(&b, labelW, "Filing Date", r.FilingDate, maxW)
	writeField(&b, labelW, "Grant Date", r.GrantDate, maxW)
	writeField(&b, labelW, "Earliest Filing Date", r.EarliestTermFilingDate, maxW)
	writeField(&b, labelW, "PTA", fmt.Sprintf("%d days", r.PatentTermAdjustmentDays), maxW)
	writeField(&b, labelW, "PTE", fmt.Sprintf("%d days", r.PatentTermExtensionDays), maxW)
	writeField(&b, labelW, "Terminal Disclaimer", r.TerminalDisclaimerDate, maxW)
	writeField(&b, labelW, "Computed Expiration", r.ComputedExpirationDate, maxW)

	computedOn := "—"
	if r.ComputedAt != "" {
		if t, err := time.Parse(time.RFC3339, r.ComputedAt); err == nil {
			computedOn = t.Local().Format("2006-01-02 15:04:05")
		} else {
			computedOn = r.ComputedAt
		}
	}
	writeField(&b, labelW, "Computed On", computedOn, maxW)

	// Google section
	if r.GoogleExpirationDate != "" {
		b.WriteString(sep + "\n")
		b.WriteString(o.theme.Header.Render("Google Patents Data") + "\n")
		writeField(&b, labelW, "Parsed Expiration", r.GoogleExpirationDate, maxW)
	}

	// Comparison
	if line := r.ComparisonLine(); line != "" {
		b.WriteString(sep + "\n")
		b.WriteString(o.theme.Header.Render("Comparison: ") + line + "\n")
	}

	b.WriteString(sep + "\n")
	b.WriteString(o.theme.MutedItalic.Render("Press [r] to refresh/recompute, Esc to close"))

	return b.String()
}

func writeField(b *strings.Builder, labelW int, label, value string, maxW int) {
	if value == "" {
		value = "—"
	}
	line := render.Pad(label, labelW) + value
	if len(line) > maxW {
		line = line[:maxW]
	}
	b.WriteString(line + "\n")
}
