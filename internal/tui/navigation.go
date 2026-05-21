package tui

import (
	tea "github.com/charmbracelet/bubbletea"
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
	selectedPatentNumber := ""
	if m.patentSelected >= 0 && m.patentSelected < len(m.patents) {
		selectedPatentNumber = string(m.patents[m.patentSelected].Number)
	}
	selectedProjectID := ""
	if m.projectSelected >= 0 && m.projectSelected < len(m.projects) {
		selectedProjectID = string(m.projects[m.projectSelected].ID)
	}
	return navSnapshot{
		mode:                    m.mode,
		selectedPatentNumber:    selectedPatentNumber,
		selectedProjectID:       selectedProjectID,
		patentSelected:          m.patentSelected,
		projectSelected:         m.projectSelected,
		projectEventsSelected:   m.projectEventsSelected,
		projectInvoicesSelected: m.projectInvoicesSelected,
		projectIDSSelected:      m.projectIDSSelected,
		detailSelected:          m.detailSelected,
		citationLocalIdx:        m.citationLocalIdx,
		citationKey:             m.citationKey,
		citesTextFilter:         m.citesTextFilter,
		reviewSelected:          m.reviewSelected,
		classificationSelected:  m.classificationSelected,
		inventorSelected:        m.inventorSelected,
		familySelected:          m.familySelected,
		visualMode:              m.visualMode,
		selectionStart:          m.selectionStart,
		current:                 m.current,
		pendingBundle:           m.pendingBundle,
		pendingCitation:         m.pendingCitation,
		reviewState:             m.reviewState,
		listFilter: PatentFilter{
			FreeFormSearch:   m.listFilter.FreeFormSearch,
			ReviewState:      m.listFilter.ReviewState,
			Classification:   m.listFilter.Classification,
			Classifications:  append([]string(nil), m.listFilter.Classifications...),
			ClassificationOp: m.listFilter.ClassificationOp,
			Tag:              m.listFilter.Tag,
			Country:          m.listFilter.Country,
			Title:            m.listFilter.Title,
			TitleTerms:       append([]string(nil), m.listFilter.TitleTerms...),
		},
		message:                    m.message,
		err:                        m.err,
		countBuffer:                m.countBuffer,
		ProjectID:                  m.ProjectID,
		sortColumn:                 m.sortColumn,
		sortOrder:                  m.sortOrder,
		sortColumn2:                m.sortColumn2,
		sortOrder2:                 m.sortOrder2,
		citesReviewStateFilter:     m.citesReviewStateFilter,
		numberColWidth:             m.numberColWidth,
		classificationQuery:        m.classificationQuery,
		classificationSearchActive: m.classificationSearchActive,
		listSearchQuery:            m.listSearchQuery,
		listSearchActive:           m.listSearchActive,
		popupSearchQuery:           m.popupSearchQuery,
		popupSearchActive:          m.popupSearchActive,
		reviewStateSelected:        m.reviewStateSelected,
		width:                      m.effectiveWidth(),
	}
}

func (m *Model) effectiveWidth() int {
	if mustModeSpec(m.mode).isOverlay {
		return m.overlayWidth()
	}
	return m.width
}

// findListIndex returns the position of id among count items, reading each
// item's id with idAt. When id is not found — the item was removed, or the
// list has not loaded yet — it falls back to prevIndex clamped to the list;
// an empty list keeps prevIndex so a later list reload can place the cursor.
func findListIndex(id string, prevIndex, count int, idAt func(int) string) int {
	if id != "" {
		for i := 0; i < count; i++ {
			if idAt(i) == id {
				return i
			}
		}
	}
	if count == 0 {
		return prevIndex
	}
	return clamp(prevIndex, 0, count-1)
}

func (m *Model) restore(snapshot navSnapshot) *Model {
	m.patentSelected = findListIndex(snapshot.selectedPatentNumber, snapshot.patentSelected, len(m.patents), func(i int) string { return string(m.patents[i].Number) })
	m.projectSelected = findListIndex(snapshot.selectedProjectID, snapshot.projectSelected, len(m.projects), func(i int) string { return string(m.projects[i].ID) })
	m.projectEventsSelected = snapshot.projectEventsSelected
	m.projectInvoicesSelected = snapshot.projectInvoicesSelected
	m.projectIDSSelected = snapshot.projectIDSSelected
	m.detailSelected = snapshot.detailSelected
	m.citationLocalIdx = snapshot.citationLocalIdx
	m.citationKey = snapshot.citationKey
	m.citesTextFilter = snapshot.citesTextFilter
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
	m.numberColWidth = snapshot.numberColWidth
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
	if m.isCitationView() && m.citesTextFilter != "" {
		m.citesTextFilter = ""
		return m, nil
	}
	if len(m.backStack) > 0 {
		last := m.backStack[len(m.backStack)-1]
		m.backStack = m.backStack[:len(m.backStack)-1]
		m = m.restore(last)
		m = m.flushDirty()
		if m.mode == viewList {
			return m.refreshList()
		}
		return m, nil
	}
	if m.mode == viewList && m.listFilter.FreeFormSearch != EmptyFilter {
		m.listFilter.FreeFormSearch = EmptyFilter
		return m.refreshList()
	}
	if m.mode != viewList {
		m.setMode(viewList)
		return m.refreshList()
	}
	return m, nil
}
