package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/text"
	"patentmine/internal/tui/overlay"
	"patentmine/internal/tui/pane"
)

// cmdEditIDSHeader opens the IDS-header editor for the active project.
func (a *App) cmdEditIDSHeader(invocation) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	a.overlays = append(a.overlays, overlay.NewIDSHeader(a.theme, *a.activeProject))
	return a, nil
}

// handleIDSHeaderSubmit persists the edited IDS header via the daemon.
func (a *App) handleIDSHeaderSubmit(p domain.Project) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.ProjectResult
		if err := a.client.Call(ctx, proto.MethodProjectUpdate,
			proto.ProjectUpdateParams{Project: p}, &res); err != nil {
			return pane.StatusMsg{Key: text.StatusUsage, Args: []any{err.Error()}, Error: true}
		}
		return idsHeaderSavedMsg{project: res.Project}
	}
}

type idsHeaderSavedMsg struct{ project domain.Project }

// cmdIDSExportPDF kicks off the export by first asking the daemon for a
// preview, then opening the confirm overlay so the user can review the target
// and add 08c signer info before any PDF is written.
func (a *App) cmdIDSExportPDF(invocation) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	project := a.activeProject.ID
	return a, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var res proto.IDSPDFPreviewResult
		if err := a.client.Call(ctx, proto.MethodIDSPDFPreview,
			proto.IDSPDFExportParams{Project: project}, &res); err != nil {
			return pane.StatusMsg{Key: text.StatusUsage, Args: []any{err.Error()}, Error: true}
		}
		return idsExportPreviewedMsg{preview: res}
	}
}

type idsExportPreviewedMsg struct{ preview proto.IDSPDFPreviewResult }

// handleIDSExportPreviewed opens the confirm overlay with the preview data.
func (a *App) handleIDSExportPreviewed(m idsExportPreviewedMsg) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		return a, nil
	}
	summary := overlay.IDSExportSummary{
		BaseDir:         m.preview.BaseDir,
		USCount:         m.preview.USCount,
		ForeignCount:    m.preview.ForeignCount,
		Sheets:          m.preview.Sheets,
		FeeTier:         m.preview.FeeTier,
		CumulativeCount: m.preview.CumulativeCount,
		ExistingDirs:    m.preview.ExistingDirs,
		MissingFields:   m.preview.MissingFields,
	}
	a.overlays = append(a.overlays, overlay.NewExportIDSConfirm(a.theme, *a.activeProject, summary))
	return a, nil
}

// handleIDSExportSubmit fires the actual PDF render now that the user
// confirmed the export and optionally filled in the 08c signer block.
func (a *App) handleIDSExportSubmit(m overlay.IDSExportSubmitMsg) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	project := a.activeProject.ID
	params := proto.IDSPDFExportParams{
		Project:         project,
		FeeAmount:       m.FeeAmount,
		DepositAccount:  m.DepositAccount,
		SignerName:      m.SignerName,
		SignerSignature: m.SignerSignature,
		SignerRegNumber: m.SignerRegNumber,
	}
	return a, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var res proto.IDSPDFExportResult
		if err := a.client.Call(ctx, proto.MethodIDSPDFExport, params, &res); err != nil {
			return pane.StatusMsg{Key: text.StatusUsage, Args: []any{err.Error()}, Error: true}
		}
		msg := fmt.Sprintf("IDS PDF exported to %s (%d files, fee tier %d)", res.Dir, len(res.Files), res.FeeTier)
		return pane.StatusMsg{Key: text.StatusUsage, Args: []any{msg}}
	}
}
