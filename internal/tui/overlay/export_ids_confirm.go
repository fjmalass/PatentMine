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

const (
	ecFieldFeeAmount = iota
	ecFieldDeposit
	ecFieldSignerName
	ecFieldSignerSig
	ecFieldRegNumber
)

// ecFields are the labeled PTO/SB/08c signer-block fields.
var ecFields = []fieldFormField{
	{Label: "Fee Amount (USD)", Kind: fieldText},
	{Label: "Deposit Account #", Kind: fieldText},
	{Label: "Signer Name (printed)", Kind: fieldText},
	{Label: "Signer Signature  (/Doe/)", Kind: fieldText},
	{Label: "Registration Number", Kind: fieldText},
}

// ExportIDSConfirm asks the user to review the export target + summary before
// any PDF is written, and to fill in the 08c signer / deposit-account fields
// when they apply. Required IDS header fields that are still empty are flagged
// here too — the export is not run while any are missing. It uses the shared
// view/edit field model: Ctrl+S exports, Esc cancels, and 'y' is a view-mode
// quick-export when nothing needs filling.
type ExportIDSConfirm struct {
	theme   render.Theme
	project domain.Project
	summary IDSExportSummary
	form    fieldForm
}

// NewExportIDSConfirm builds the overlay. base is the directory the export
// will land in (a timestamped subdir of this is created at write time).
func NewExportIDSConfirm(theme render.Theme, project domain.Project, summary IDSExportSummary) *ExportIDSConfirm {
	return &ExportIDSConfirm{theme: theme, project: project, summary: summary, form: newFieldForm(ecFields)}
}

// Title implements Overlay.
func (o *ExportIDSConfirm) Title() string { return "Export IDS PDF" }

// Command implements Overlay.
func (o *ExportIDSConfirm) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

// Handles implements Overlay.
func (o *ExportIDSConfirm) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler.
func (o *ExportIDSConfirm) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	// View-mode quick-export: 'y' confirms when nothing in the form needs
	// filling and no required header fields are missing.
	if !o.form.Editing() && (msg.String() == "y" || msg.String() == "Y") &&
		o.form.Focus() == ecFieldFeeAmount && o.form.Value(ecFieldFeeAmount) == "" &&
		len(o.summary.MissingFields) == 0 {
		return o, func() tea.Msg { return IDSExportSubmitMsg{} }, true
	}

	switch action, _ := o.form.HandleKey(msg); action {
	case fieldFormSubmit:
		// Block export when required header fields are missing — the generated
		// PDFs would be unusable for filing.
		if len(o.summary.MissingFields) > 0 {
			return o, nil, true
		}
		return o, o.submit(), true
	case fieldFormCancel:
		return o, func() tea.Msg { return ConfirmRejectMsg{} }, true
	default:
		return o, nil, true
	}
}

func (o *ExportIDSConfirm) submit() tea.Cmd {
	return func() tea.Msg {
		return IDSExportSubmitMsg{
			FeeAmount:       strings.TrimSpace(o.form.Value(ecFieldFeeAmount)),
			DepositAccount:  strings.TrimSpace(o.form.Value(ecFieldDeposit)),
			SignerName:      strings.TrimSpace(o.form.Value(ecFieldSignerName)),
			SignerSignature: strings.TrimSpace(o.form.Value(ecFieldSignerSig)),
			SignerRegNumber: strings.TrimSpace(o.form.Value(ecFieldRegNumber)),
		}
	}
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
	b.WriteString(o.form.RenderFields(o.theme, maxW, 26))
	b.WriteByte('\n')

	var hint string
	switch {
	case o.form.Editing():
		hint = o.form.Hint()
	case len(o.summary.MissingFields) > 0:
		hint = "[esc] cancel  ·  [ctrl+s] blocked until missing header fields are fixed  ·  [;] jump"
	default:
		hint = o.form.Hint() + "  ·  [y] quick-export"
	}
	writeRow(hint, o.theme.Dim.Render)
	return b.String()
}
