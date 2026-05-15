package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"patentmine/internal/domain"
)

func (m *Model) navigateTo(mode viewMode) *Model {
	if m.mode == mode {
		return m
	}
	m.backStack = append(m.backStack, m.snapshot())
	m.setMode(mode)
	m.err = EmptyError
	m.message = EmptyMessage

	return m
}

func (m *Model) snapshot() navSnapshot {
	patents := m.patents
	projects := make([]domain.Project, len(m.projects))
	copy(projects, m.projects)
	return navSnapshot{
		mode:                         m.mode,
		patents:                      patents,
		projects:                     projects,
		selected:                     m.selected,
		projectSelected:              m.projectSelected,
		projectEventsSelected:        m.projectEventsSelected,
		projectInvoicesSelected:      m.projectInvoicesSelected,
		projectIDSSelected:           m.projectIDSSelected,
		detailSelected:               m.detailSelected,
		citesSelected:                m.citesSelected,
		citedBySelected:              m.citedBySelected,
		reviewSelected:               m.reviewSelected,
		classificationSelected:       m.classificationSelected,
		inventorSelected:             m.inventorSelected,
		familySelected:               m.familySelected,
		visualMode:                   m.visualMode,
		selectionStart:               m.selectionStart,
		current:                      m.current,
		pendingBundle:                m.pendingBundle,
		pendingCitation:              m.pendingCitation,
		reviewState:                  m.reviewState,
		listFilter: PatentFilter{
			Text:        m.listFilter.Text,
			ReviewState: m.listFilter.ReviewState,
			Class:       m.listFilter.Class,
			Classes:     append([]string(nil), m.listFilter.Classes...),
			ClassOp:     m.listFilter.ClassOp,
			Tag:         m.listFilter.Tag,
			Country:     m.listFilter.Country,
		},
		message:                      m.message,
		err:                          m.err,
		countBuffer:                  m.countBuffer,
		ProjectID:                    m.ProjectID,
		sortColumn:                   m.sortColumn,
		sortOrder:                    m.sortOrder,
		sortColumn2:                  m.sortColumn2,
		sortOrder2:                   m.sortOrder2,
		citesReviewStateFilter:       m.citesReviewStateFilter,
		listNumWidth:                 m.listNumWidth,
		classificationQuery:          m.classificationQuery,
		classificationSearchActive:   m.classificationSearchActive,
		listSearchQuery:              m.listSearchQuery,
		listSearchActive:             m.listSearchActive,
		popupSearchQuery:             m.popupSearchQuery,
		popupSearchActive:            m.popupSearchActive,
		reviewStateSelected:          m.reviewStateSelected,
		width:                        m.effectiveWidth(),
	}
}

func (m *Model) effectiveWidth() int {
	if mustModeSpec(m.mode).isOverlay {
		return m.overlayWidth()
	}
	return m.width
}

func (m *Model) restore(snapshot navSnapshot) *Model {
	m.patents = snapshot.patents
	m.projects = snapshot.projects
	m.selected = snapshot.selected
	m.projectSelected = snapshot.projectSelected
	m.projectEventsSelected = snapshot.projectEventsSelected
	m.projectInvoicesSelected = snapshot.projectInvoicesSelected
	m.projectIDSSelected = snapshot.projectIDSSelected
	m.detailSelected = snapshot.detailSelected
	m.citesSelected = snapshot.citesSelected
	m.citedBySelected = snapshot.citedBySelected
	m.reviewSelected = snapshot.reviewSelected
	m.classificationSelected = snapshot.classificationSelected
	m.inventorSelected = snapshot.inventorSelected
	m.familySelected = snapshot.familySelected
	m.visualMode = snapshot.visualMode
	m.selectionStart = snapshot.selectionStart
	m.current = snapshot.current
	m.pendingBundle = snapshot.pendingBundle
	m.pendingCitation = snapshot.pendingCitation
	m.reviewState = snapshot.reviewState
	m.listFilter = snapshot.listFilter
	m.message = snapshot.message
	m.err = snapshot.err
	m.countBuffer = snapshot.countBuffer
	m.ProjectID = snapshot.ProjectID
	m.sortColumn = snapshot.sortColumn
	m.sortOrder = snapshot.sortOrder
	m.sortColumn2 = snapshot.sortColumn2
	m.sortOrder2 = snapshot.sortOrder2
	m.citesReviewStateFilter = snapshot.citesReviewStateFilter
	m.listNumWidth = snapshot.listNumWidth
	m.classificationQuery = snapshot.classificationQuery
	m.classificationSearchActive = snapshot.classificationSearchActive
	m.listSearchQuery = snapshot.listSearchQuery
	m.listSearchActive = snapshot.listSearchActive
	m.popupSearchQuery = snapshot.popupSearchQuery
	m.popupSearchActive = snapshot.popupSearchActive
	m.reviewStateSelected = snapshot.reviewStateSelected
	m.setMode(snapshot.mode)
	return m
}

func (m *Model) goBack() (tea.Model, tea.Cmd) {
	if (m.mode == viewHelp || m.mode == viewKeymap) && m.helpSearchActive {
		m.helpSearchActive = false
		m.helpQuery = ""
		m.helpScroll = 0
		return m, nil
	}
	if m.isPopupSearchMode() && m.popupSearchActive {
		m.popupSearchActive = false
		m.popupSearchQuery = ""
		return m, nil
	}
	if m.mode == viewList && m.listSearchActive {
		m.listSearchActive = false
		m.listSearchQuery = ""
		return m, nil
	}
	if len(m.backStack) > 0 {
		last := m.backStack[len(m.backStack)-1]
		m.backStack = m.backStack[:len(m.backStack)-1]
		m = m.restore(last)
		if m.mode == viewList {
			return m.refreshList()
		}
		return m, nil
	}
	if m.mode == viewList && m.listFilter.Text != EmptyFilter {
		m.listFilter.Text = EmptyFilter
		return m.refreshList()
	}
	if m.mode != viewList {
		m.setMode(viewList)
		return m.refreshList()
	}
	return m, nil
}
