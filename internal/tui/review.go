package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

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
	m.setMode(viewPreview)
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
		m.bulkActionIndices = indices
		return m.navigateTo(viewBulkConfirm), nil
	}

	idx := indices[0]
	edge := edges[idx]
	if _, err := m.repo.GetPatent(m.ctx, m.ProjectID, edge.TargetPatent); err != nil {
		return m.openSelectedReviewCitation()
	}
	if err := m.repo.UpdateCitationReviewState(m.ctx, m.ProjectID, edge, domain.ReviewStateStored); err != nil {
		m.err = err.Error()
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
		m.bulkActionIndices = indices
		if status == domain.ReviewStateIgnored {
			m.bulkAction = bulkActionIgnore
		} else {
			m.bulkAction = bulkActionUnderReview
		}
		return m.navigateTo(viewBulkConfirm), nil
	}

	edge := edges[indices[0]]
	if err := m.repo.UpdateCitationReviewState(m.ctx, m.ProjectID, edge, status); err != nil {
		m.err = err.Error()
		return m, nil
	}
	if status != m.reviewState {
		edges, _ := m.currentReviewCitationEdges()
		m.reviewSelected = clamp(m.reviewSelected, 0, max(0, len(edges)-1))
	}
	m.message = fmt.Sprintf(m.text.T(messageKey), edge.TargetPatent)
	m.visualMode = false
	return m, nil
}

func (m *Model) executeBulkAction() (tea.Model, tea.Cmd) {
	indices := m.bulkActionIndices
	if len(indices) == 0 {
		return m.goBack()
	}

	var edges []domain.CitationEdge
	var err error
	if m.mode == viewReview {
		edges, err = m.currentReviewCitationEdges()
	} else {
		edges, err = m.currentCitationEdges()
	}

	if err != nil || len(edges) == 0 {
		m.err = "bulk action failed: " + err.Error()
		return m.goBack()
	}

	var cmds []tea.Cmd
	action := m.bulkAction
	status := domain.ReviewStateStored
	if action == bulkActionIgnore {
		status = domain.ReviewStateIgnored
	} else if action == bulkActionUnderReview {
		status = domain.ReviewStateUnderReview
	}

	executedCount := 0
	importCount := 0
	for _, idx := range indices {
		if idx < 0 || idx >= len(edges) {
			continue
		}
		edge := edges[idx]
		if err := m.repo.UpdateCitationReviewState(m.ctx, m.ProjectID, edge, status); err == nil {
			m.logActivity(ActivityBulkPrefix+string(action), edge.TargetPatent, "")
			executedCount++

			if action == bulkActionStore {
				if _, err := m.repo.GetPatent(m.ctx, m.ProjectID, edge.TargetPatent); err != nil {
					cmds = append(cmds, m.importCitationDetailsCommand(edge))
					importCount++
				}
			}
		}
	}

	m.message = fmt.Sprintf("performed bulk %s on %d items", action, executedCount)
	m.visualMode = false
	m.bulkActionIndices = nil

	model, _ := m.goBack()
	if importCount > 0 {
		m.loading = true
		m.loadingMsg = fmt.Sprintf("downloading %d patents...", importCount)
		cmds = append(cmds, m.spinner.Tick)
	}
	return model, tea.Batch(cmds...)
}
