package tui

import (
	"fmt"
	"strings"

	"patentmine/internal/changes"
	"patentmine/internal/domain"
	"patentmine/internal/storage"

	tea "github.com/charmbracelet/bubbletea"
)

// edgesAtIndices resolves selection indices to citation edges, dropping any
// out-of-range index. Resolving at selection time keeps a later bulk action
// bound to the rows the user actually picked, even if sort/filter changes.
func edgesAtIndices(edges []domain.CitationEdge, indices []int) []domain.CitationEdge {
	out := make([]domain.CitationEdge, 0, len(indices))
	for _, idx := range indices {
		if idx >= 0 && idx < len(edges) {
			out = append(out, edges[idx])
		}
	}
	return out
}

func (m *Model) reviewCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) != 1 {
		m.err = m.text.T(TextMessageReviewUsage)
		return m, nil
	}
	switch strings.ToLower(args[0]) {
	case domain.ReviewStateIgnored:
		return m.openReviewQueue(domain.ReviewStateIgnored)
	case domain.ReviewStateUnderReview:
		return m.openReviewQueue(domain.ReviewStateUnderReview)
	default:
		m.err = m.text.T(TextMessageReviewUsage)
		return m, nil
	}
}

func (m *Model) openReviewQueue(status string) (tea.Model, tea.Cmd) {
	m.backStack = append(m.backStack, m.snapshot())
	m.setMode(viewReview)
	m.reviewState = status
	m.reviewSelected = 0
	m.err = EmptyError
	m.message = EmptyMessage
	return m, nil
}

func (m *Model) currentReviewCitationEdges() ([]domain.CitationEdge, error) {
	if strings.TrimSpace(m.reviewState) == "" {
		return nil, nil
	}
	opts := storage.ListCitationsOptions{
		SortColumn: m.sortColumn,
		SortOrder:  m.sortOrder,
	}
	return m.repo.ListCitationsByReviewState(m.ctx, m.ProjectID, m.reviewState, opts)
}

func (m *Model) selectedReviewCitationEdge() (domain.CitationEdge, bool, error) {
	edges, err := m.currentReviewCitationEdges()
	if err != nil {
		return domain.CitationEdge{}, false, err
	}
	if len(edges) == 0 {
		return domain.CitationEdge{}, false, nil
	}
	selected := clamp(m.reviewSelected, 0, len(edges)-1)
	return edges[selected], true, nil
}

func (m *Model) openSelectedReviewCitation() (tea.Model, tea.Cmd) {
	edge, ok, err := m.selectedReviewCitationEdge()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if !ok {
		return m, nil
	}
	target := edge.TargetPatent
	var bundle domain.PatentBundle
	if p, err := m.repo.GetPatent(m.ctx, m.ProjectID, target); err == nil {
		bundle.Patent = p
	} else {
		bundle, err = m.importPatent(target)
		if err != nil {
			m.err = fmt.Sprintf("%s: %v", m.text.T(TextCitationsOpenFailed), err)
			return m, nil
		}
	}
	m.backStack = append(m.backStack, m.snapshot())
	m.pendingBundle = bundle
	m.pendingCitation = edge
	m.current = bundle.Patent
	m.current.ReviewState = edge.ReviewState // popup shows citation edge state, matching list column
	m.detailSelected = 0
	m.populateDetailCache()
	m.setMode(viewPopupPatentDetail)
	m.message = fmt.Sprintf(m.text.T(TextMessagePreviewLoaded), bundle.Patent.Number)
	return m, nil
}

func (m *Model) storeSelectedReviewCitation() (tea.Model, tea.Cmd) {
	indices := m.selectedIndices()
	edges, err := m.currentReviewCitationEdges()
	if err != nil || len(edges) == 0 {
		return m, nil
	}

	if len(indices) > 1 {
		m.bulkAction = bulkActionStore
		m.bulkActionEdges = edgesAtIndices(edges, indices)
		return m.navigateTo(viewBulkConfirm), nil
	}

	idx := indices[0]
	edge := edges[idx]
	if _, err := m.repo.GetPatent(m.ctx, m.ProjectID, edge.TargetPatent); err != nil {
		return m.openSelectedReviewCitation()
	}
	if !m.applyChange(changes.SetCitationReviewState(m.ProjectID, []domain.CitationEdge{edge}, domain.ReviewStateStored, false)) {
		return m, nil
	}
	m.message = fmt.Sprintf(m.text.T(TextMessageStoredPatent), edge.TargetPatent)
	return m, nil
}

func (m *Model) updateSelectedReviewCitationReviewState(status string, messageKey TextKey) (tea.Model, tea.Cmd) {
	indices := m.selectedIndices()
	edges, err := m.currentReviewCitationEdges()
	if err != nil || len(edges) == 0 {
		return m, nil
	}

	if len(indices) > 1 {
		m.bulkActionEdges = edgesAtIndices(edges, indices)
		if status == domain.ReviewStateIgnored {
			m.bulkAction = bulkActionIgnore
		} else {
			m.bulkAction = bulkActionUnderReview
		}
		return m.navigateTo(viewBulkConfirm), nil
	}

	edge := edges[indices[0]]
	if !m.applyChange(changes.SetCitationReviewState(m.ProjectID, []domain.CitationEdge{edge}, status, false)) {
		return m, nil
	}
	if status != m.reviewState {
		edges, _ := m.currentReviewCitationEdges()
		m.reviewSelected = clamp(m.reviewSelected, 0, max(0, len(edges)-1))
	}
	m.message = fmt.Sprintf(m.text.T(messageKey), edge.TargetPatent)
	m.clearVisualMode()
	return m, nil
}

func (m *Model) executeBulkAction() (tea.Model, tea.Cmd) {
	if m.bulkAction == bulkActionDelete {
		return m.executeBulkDelete(m.bulkActionNumbers)
	}

	edges := m.bulkActionEdges
	if len(edges) == 0 {
		return m.goBack()
	}

	action := m.bulkAction
	status := domain.ReviewStateStored
	if action == bulkActionIgnore {
		status = domain.ReviewStateIgnored
	} else if action == bulkActionUnderReview {
		status = domain.ReviewStateUnderReview
	}

	m.logger.Info("bulk citation action started", "project", m.ProjectID, "action", action, "status", status, "count", len(edges))

	// One transaction: all edges move together or none do.
	if !m.applyChange(changes.SetCitationReviewState(m.ProjectID, edges, status, false)) {
		m.clearVisualMode()
		m.bulkActionEdges = nil
		return m.goBack()
	}

	var cmds []tea.Cmd
	importCount := 0
	for _, edge := range edges {
		m.logActivityFrom(ActivityBulkPrefix+string(action), edge.TargetPatent, "", edge.SourcePatent)
		if action == bulkActionStore {
			if _, err := m.repo.GetPatent(m.ctx, m.ProjectID, edge.TargetPatent); err != nil {
				cmds = append(cmds, m.importCitationDetailsCommand(edge))
				importCount++
			}
		}
	}

	m.logger.Info("bulk citation action completed", "project", m.ProjectID, "action", action, "count", len(edges))
	m.message = fmt.Sprintf("performed bulk %s on %d items", action, len(edges))
	m.clearVisualMode()
	m.bulkActionEdges = nil

	model, _ := m.goBack()
	if importCount > 0 {
		m.loading = true
		m.loadingMsg = fmt.Sprintf("downloading %d patents...", importCount)
		cmds = append(cmds, m.spinner.Tick)
	}
	return model, tea.Batch(cmds...)
}

func (m *Model) executeBulkDelete(nums []string) (tea.Model, tea.Cmd) {
	finish := func(msg string) (tea.Model, tea.Cmd) {
		m.bulkActionNumbers = nil
		m.clearVisualMode()
		// pop the viewBulkConfirm snapshot (always pushed from viewList)
		if len(m.backStack) > 0 {
			m.backStack = m.backStack[:len(m.backStack)-1]
		}
		if msg != "" {
			m.message = msg
		}
		m.setMode(viewList)
		return m.refreshList()
	}

	if len(nums) == 0 {
		return finish("")
	}

	m.logger.Info("bulk delete started", "project", m.ProjectID, "count", len(nums), "patents", nums)
	if !m.applyChange(changes.DeletePatents(m.ProjectID, nums)) {
		return finish("")
	}
	for _, num := range nums {
		m.logActivity(ActivityPatentDelete, num, fmt.Sprintf("bulk-%03d", len(nums)))
	}
	m.logger.Info("bulk delete completed", "project", m.ProjectID, "count", len(nums))
	return finish(fmt.Sprintf("Deleted %d patent(s)", len(nums)))
}
