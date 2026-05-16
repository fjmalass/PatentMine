package tui

import (
	"fmt"
	"strings"

	"patentmine/internal/changes"
	"patentmine/internal/domain"
	"patentmine/internal/storage"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) isCitationView() bool {
	return m.mode == viewCites || m.mode == viewCitedBy
}


func (m *Model) citationEdgesForRelation(relation string) ([]domain.CitationEdge, error) {
	if m.current.Number == EmptyFilter {
		return nil, nil
	}
	opts := storage.ListCitationsOptions{
		SortColumn:        m.sortColumn,
		SortOrder:         m.sortOrder,
		ReviewStateFilter: m.citesReviewStateFilter,
	}
	return m.repo.ListCitations(m.ctx, m.ProjectID, m.current.Number, relation, opts)
}


func (m *Model) filterCitationEdges(edges []domain.CitationEdge, query string) []domain.CitationEdge {
	ignoreCase := strings.ToLower(query) == query
	out := edges[:0:len(edges)]
	for _, e := range edges {
		if containsMatch(e.TargetPatent, query, ignoreCase) || containsMatch(e.TargetTitle, query, ignoreCase) {
			out = append(out, e)
			continue
		}
		for _, inv := range e.TargetInventors {
			if containsMatch(inv, query, ignoreCase) {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

func (m *Model) currentCitationEdges() ([]domain.CitationEdge, error) {
	if m.current.Number == EmptyFilter {
		return nil, nil
	}
	relation := domain.RelationCites
	if m.mode == viewCitedBy {
		relation = domain.RelationCitedBy
	}
	opts := storage.ListCitationsOptions{
		SortColumn:        m.sortColumn,
		SortOrder:         m.sortOrder,
		ReviewStateFilter: m.citesReviewStateFilter,
	}
	return m.repo.ListCitations(m.ctx, m.ProjectID, m.current.Number, relation, opts)
}

func (m *Model) visibleCitationEdges() ([]domain.CitationEdge, error) {
	var edges []domain.CitationEdge
	var selected int
	var err error
	switch {
	case m.isCitationView():
		edges, err = m.currentCitationEdges()
		selected = m.citationLocalIdx
	case m.mode == viewReview:
		edges, err = m.currentReviewCitationEdges()
		selected = m.reviewSelected
	case m.mode == viewDetail || m.mode == viewList:
		if m.current.Number == "" {
			return nil, nil
		}
		cites, e1 := m.repo.ListCitations(m.ctx, m.ProjectID, m.current.Number, domain.RelationCites, storage.ListCitationsOptions{})
		citedBy, e2 := m.repo.ListCitations(m.ctx, m.ProjectID, m.current.Number, domain.RelationCitedBy, storage.ListCitationsOptions{})
		if e1 != nil {
			return nil, e1
		}
		if e2 != nil {
			return nil, e2
		}
		edges = append(cites, citedBy...)
		if len(edges) == 0 {
			return nil, nil
		}
		return edges, nil
	default:
		return nil, nil
	}
	if err != nil || len(edges) == 0 {
		return nil, err
	}
	selected = clamp(selected, 0, len(edges)-1)
	window := pageWindow(selected, len(edges), m.pageSize())
	out := make([]domain.CitationEdge, window.End-window.Start)
	copy(out, edges[window.Start:window.End])
	return out, nil
}

func (m *Model) refreshList() (tea.Model, tea.Cmd) {
	return m.reloadPatents(), nil
}

// reloadPatents re-reads the patent list from the DB applying the current
// filter and sort, re-anchoring the cursor to the same patent number.
func (m *Model) reloadPatents() *Model {
	if m.repo == nil {
		return m
	}
	// Capture selected patent number before re-fetch so sort/filter changes
	// re-resolve the cursor to the same patent rather than the same index.
	anchorNumber := ""
	if m.patentSelected >= 0 && m.patentSelected < len(m.patents) {
		anchorNumber = m.patents[m.patentSelected].Number
	}
	opts := m.listFilter.toStorageOpts(m.sortColumn, m.sortOrder, m.sortColumn2, m.sortOrder2)
	patents, err := m.repo.ListPatents(m.ctx, m.ProjectID, opts)
	if err != nil {
		m.err = err.Error()
		if m.logger != nil {
			m.logger.Error("list patents failed", "filter", m.listFilter.FreeFormSearch, "error", err)
		}
		return m
	}
	m.patents = patents

	all, err2 := m.repo.ListPatents(m.ctx, m.ProjectID, storage.ListPatentsOptions{
		ReviewStateFilter: reviewStateFilterNone,
	})
	if err2 == nil {
		m.totalPatents = len(all)
	}

	m.numberColWidth = 6
	for _, p := range m.patents {
		w := lipgloss.Width(p.Number)
		if w > m.numberColWidth {
			m.numberColWidth = w
		}
	}
	if anchorNumber != "" {
		for i, p := range m.patents {
			if p.Number == anchorNumber {
				m.patentSelected = i
				break
			}
		}
	}
	if m.patentSelected >= len(m.patents) {
		m.patentSelected = max(0, len(m.patents)-1)
	}
	if len(patents) > 0 && m.current.Number == "" {
		m.current = patents[0]
	}
	return m
}

func (m *Model) openSelectedCitation() (tea.Model, tea.Cmd) {
	edge, ok, err := m.selectedCitationEdge()
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

func (m *Model) selectedCitationEdge() (domain.CitationEdge, bool, error) {
	edges, err := m.currentCitationEdges()
	if err != nil {
		return domain.CitationEdge{}, false, err
	}
	if len(edges) == 0 {
		return domain.CitationEdge{}, false, nil
	}
	selected := clamp(m.citationLocalIdx, 0, len(edges)-1)
	return edges[selected], true, nil
}

func (m *Model) storeSelectedCitation() (tea.Model, tea.Cmd) {
	indices := m.selectedIndices()
	edges, err := m.currentCitationEdges()
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
		return m.openSelectedCitation()
	}
	if !m.applyChange(changes.SetCitationReviewState(m.ProjectID, []domain.CitationEdge{edge}, domain.ReviewStateStored, false)) {
		return m, nil
	}
	m.logActivityFrom(ActivityCitationStore, edge.TargetPatent, "", m.current.Number)
	m.message = fmt.Sprintf(m.text.T(TextMessageStoredPatent), edge.TargetPatent)
	m.clearVisualMode()
	return m, nil
}

func (m *Model) storePendingPatent() (tea.Model, tea.Cmd) {
	if m.pendingBundle.Patent.Number == "" {
		return m, nil
	}
	m.pendingBundle.Patent.ReviewState = domain.ReviewStateStored
	if err := m.repo.UpsertPatentBundle(m.ctx, m.ProjectID, m.pendingBundle); err != nil {
		m.err = err.Error()
		return m, nil
	}
	number := m.pendingBundle.Patent.Number
	m.logActivity(ActivityPatentImport, number, string(m.importCfg.ImportSource))
	if m.pendingCitation.TargetPatent != "" {
		if err := m.repo.UpdateCitationReviewState(m.ctx, m.ProjectID, m.pendingCitation, domain.ReviewStateStored); err != nil {
			m.err = err.Error()
			return m, nil
		}
	}
	m.pendingBundle = domain.PatentBundle{}
	m.pendingCitation = domain.CitationEdge{}
	p, err := m.repo.GetPatent(m.ctx, m.ProjectID, number)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.current = p
	m.populateDetailCache()
	m.setMode(viewDetail)
	m.message = fmt.Sprintf(m.text.T(TextMessageStoredPatent), number)
	return m.refreshList()
}

func (m *Model) skipPendingPatent() (tea.Model, tea.Cmd) {
	number := m.pendingBundle.Patent.Number
	m.pendingBundle = domain.PatentBundle{}
	m.pendingCitation = domain.CitationEdge{}
	model, cmd := m.goBack()
	updated := model.(*Model)
	if number != "" {
		updated.message = fmt.Sprintf(updated.text.T(TextMessageSkippedPatent), number)
	}
	return updated, cmd
}

func (m *Model) updatePendingCitation(reviewState string, messageKey TextKey) (tea.Model, tea.Cmd) {
	number := m.pendingBundle.Patent.Number
	if m.pendingCitation.TargetPatent != "" {
		if !m.applyChange(changes.SetCitationReviewState(m.ProjectID, []domain.CitationEdge{m.pendingCitation}, reviewState, false)) {
			return m, nil
		}
	}
	m.pendingBundle = domain.PatentBundle{}
	m.pendingCitation = domain.CitationEdge{}
	model, cmd := m.goBack()
	updated := model.(*Model)
	if number != "" {
		updated.message = fmt.Sprintf(updated.text.T(messageKey), number)
	}
	return updated, cmd
}

func (m *Model) updateSelectedCitationReviewState(status string, messageKey TextKey) (tea.Model, tea.Cmd) {
	indices := m.selectedIndices()
	edges, err := m.currentCitationEdges()
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
	m.logActivityFrom(ActivityCitationReviewState, edge.TargetPatent, status, m.current.Number)
	m.message = fmt.Sprintf(m.text.T(messageKey), edge.TargetPatent)

	m.clearVisualMode()
	return m, nil
}
