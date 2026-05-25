package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// IDSExportSummary is the preflight view of an IDS export: where it will be
// written, how many entries fall into each bucket, and which IDS-header
// fields are still empty. The App computes this once and hands it to the
// confirm overlay below.
type IDSExportSummary struct {
	BaseDir         string
	USCount         int
	ForeignCount    int
	Sheets          int
	FeeTier         int
	CumulativeCount int
	ExistingDirs    []string
	MissingFields   []string
}

// IDSExportSubmitMsg is delivered when the user confirms the export. The App
// turns it into an RPC call.
type IDSExportSubmitMsg struct {
	FeeAmount       string
	DepositAccount  string
	SignerName      string
	SignerSignature string
	SignerRegNumber string
}

// ExportIDSConfirm asks the user to review the export target + summary before
// any PDF is written, and to fill in the 08c signer / deposit-account fields
// when they apply. Required IDS header fields that are still empty are flagged
// here too — the export is not run while any are missing.
type ExportIDSConfirm struct {
	theme   render.Theme
	project domain.Project
	summary IDSExportSummary
	values  [5]string
	focus   int
}

const (
	ecFieldFeeAmount = iota
	ecFieldDeposit
	ecFieldSignerName
	ecFieldSignerSig
	ecFieldRegNumber
)

var ecLabels = [5]string{
	"Fee Amount (USD)",
	"Deposit Account #",
	"Signer Name (printed)",
	"Signer Signature  (/Doe/)",
	"Registration Number",
}

// NewExportIDSConfirm builds the overlay. base is the directory the export
// will land in (a timestamped subdir of this is created at write time).
func NewExportIDSConfirm(theme render.Theme, project domain.Project, summary IDSExportSummary) *ExportIDSConfirm {
	return &ExportIDSConfirm{theme: theme, project: project, summary: summary}
}

// Title implements Overlay.
func (o *ExportIDSConfirm) Title() string { return "Export IDS PDF" }

// Command implements Overlay.
func (o *ExportIDSConfirm) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

// Handles implements Overlay.
func (o *ExportIDSConfirm) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler.
func (o *ExportIDSConfirm) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyEsc:
		return o, func() tea.Msg { return ConfirmRejectMsg{} }, true
	case tea.KeyTab, tea.KeyDown:
		o.focus = (o.focus + 1) % len(o.values)
		return o, nil, true
	case tea.KeyShiftTab, tea.KeyUp:
		o.focus = (o.focus - 1 + len(o.values)) % len(o.values)
		return o, nil, true
	case tea.KeyBackspace:
		if len(o.values[o.focus]) > 0 {
			r := []rune(o.values[o.focus])
			o.values[o.focus] = string(r[:len(r)-1])
		}
		return o, nil, true
	case tea.KeyCtrlU:
		o.values[o.focus] = ""
		return o, nil, true
	case tea.KeyEnter:
		// Block confirm when required header fields are missing — the
		// generated PDFs would be unusable for filing.
		if len(o.summary.MissingFields) > 0 {
			return o, nil, true
		}
		return o, func() tea.Msg {
			return IDSExportSubmitMsg{
				FeeAmount:       strings.TrimSpace(o.values[ecFieldFeeAmount]),
				DepositAccount:  strings.TrimSpace(o.values[ecFieldDeposit]),
				SignerName:      strings.TrimSpace(o.values[ecFieldSignerName]),
				SignerSignature: strings.TrimSpace(o.values[ecFieldSignerSig]),
				SignerRegNumber: strings.TrimSpace(o.values[ecFieldRegNumber]),
			}
		}, true
	case tea.KeyRunes, tea.KeySpace:
		// On a typeable key with no field focused yet, also accept 'y'/'Y' as
		// quick confirm when nothing in the form needs filling.
		s := msg.String()
		if (s == "y" || s == "Y") && o.focus == ecFieldFeeAmount && o.values[ecFieldFeeAmount] == "" &&
			len(o.summary.MissingFields) == 0 {
			return o, func() tea.Msg {
				return IDSExportSubmitMsg{}
			}, true
		}
		o.values[o.focus] += s
		return o, nil, true
	}
	return o, nil, true
}

// View implements Overlay.
func (o *ExportIDSConfirm) View(maxW, _ int) string {
	var b strings.Builder
	writeRow := func(s string, style func(...string) string) {
		b.WriteString(style(render.Truncate(s, maxW)))
		b.WriteByte('\n')
	}
	writeRow("Project: "+o.project.Name+"  ("+string(o.project.ID)+")", o.theme.Dim.Render)
	writeRow("Target:  "+o.summary.BaseDir, o.theme.Dim.Render)
	b.WriteByte('\n')

	summary := fmt.Sprintf("Entries: %d US + %d foreign  ·  Sheets: %d  ·  Cumulative items: %d  ·  Fee tier: %d (1.17(v))",
		o.summary.USCount, o.summary.ForeignCount, o.summary.Sheets,
		o.summary.CumulativeCount, o.summary.FeeTier)
	writeRow(summary, o.theme.Row.Render)
	if o.summary.FeeTier == 0 {
		writeRow("  → no 1.17(v) size fee owed; signer block still required.", o.theme.Dim.Render)
	} else {
		writeRow(fmt.Sprintf("  → an IDS size fee under 1.17(v)(%d) is due — fill the fee+deposit fields below.", o.summary.FeeTier),
			o.theme.Dim.Render)
	}
	b.WriteByte('\n')

	if len(o.summary.MissingFields) > 0 {
		writeRow("⚠ Missing IDS header fields — fix in :project.ids-header before exporting:",
			o.theme.Row.Render)
		for _, f := range o.summary.MissingFields {
			writeRow("    • "+f, o.theme.Row.Render)
		}
		b.WriteByte('\n')
	}

	if len(o.summary.ExistingDirs) > 0 {
		writeRow(fmt.Sprintf("Existing exports for this project (%d):", len(o.summary.ExistingDirs)),
			o.theme.Dim.Render)
		for _, d := range o.summary.ExistingDirs {
			writeRow("    · "+d, o.theme.Dim.Render)
		}
		b.WriteByte('\n')
	}

	writeRow("PTO/SB/08c signer block (fields stay blank if left empty):", o.theme.Dim.Render)
	for i, label := range ecLabels {
		marker := "  "
		if i == o.focus {
			marker = "▸ "
		}
		line := fmt.Sprintf("%s%-26s %s", marker, label+":", o.values[i])
		if i == o.focus {
			b.WriteString(o.theme.Title.Render(render.Truncate(line, maxW)))
		} else {
			b.WriteString(o.theme.Row.Render(render.Truncate(line, maxW)))
		}
		b.WriteByte('\n')
	}
	b.WriteByte('\n')

	if len(o.summary.MissingFields) > 0 {
		writeRow("[esc] Cancel  ·  [enter] blocked by missing fields", o.theme.Dim.Render)
	} else {
		writeRow("[tab]/[shift+tab] field  ·  [enter] Export  ·  [esc] Cancel  ·  [y] quick-export", o.theme.Dim.Render)
	}
	return b.String()
}
