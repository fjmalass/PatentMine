package tui

import (
	"fmt"
	"strings"
)

// searchable is the generic interface for any navigable list that supports
// incremental search. Implement it to get wrap-around smart-case search for free.
type searchable interface {
	ItemCount() int
	Match(idx int, query string, ignoreCase bool) bool
	SetSelected(idx int)
	GetSelected() int
	MatchLabel(idx int) string
}

// searchFrom is the single implementation of wrap-around smart-case search.
// It is shared by list search, popup search, and any future paginated view.
func (m *Model) searchFrom(s searchable, query string, next bool) *Model {
	if query == "" || s == nil {
		return m
	}
	count := s.ItemCount()
	if count == 0 {
		return m
	}

	ignoreCase := strings.ToLower(query) == query
	start := 0
	if next {
		start = s.GetSelected() + 1
	}

	for i := 0; i < count; i++ {
		idx := (start + i) % count
		if s.Match(idx, query, ignoreCase) {
			s.SetSelected(idx)
			if label := s.MatchLabel(idx); label != "" {
				m.message = fmt.Sprintf("found match: %s", label)
			} else {
				m.message = "found match"
			}
			return m
		}
	}
	m.message = "no matches found for: " + query
	return m
}

// — Patent list ——————————————————————————————————————————————————————————————

type patentListSearchable struct{ m *Model }

func (s patentListSearchable) ItemCount() int  { return len(s.m.patents) }
func (s patentListSearchable) GetSelected() int { return s.m.patentSelected }
func (s patentListSearchable) SetSelected(idx int) { s.m.patentSelected = idx }
func (s patentListSearchable) MatchLabel(idx int) string { return s.m.patents[idx].Number }
func (s patentListSearchable) Match(idx int, query string, ignoreCase bool) bool {
	p := s.m.patents[idx]
	if containsMatch(p.Number, query, ignoreCase) ||
		containsMatch(p.Title, query, ignoreCase) ||
		containsMatch(p.Abstract, query, ignoreCase) ||
		containsMatch(p.Assignee, query, ignoreCase) ||
		containsMatch(p.ClassificationLabel, query, ignoreCase) {
		return true
	}
	for _, inv := range p.Inventors {
		if containsMatch(inv, query, ignoreCase) {
			return true
		}
	}
	return false
}

func (m *Model) listSearchNext() *Model {
	return m.searchFrom(patentListSearchable{m}, m.listSearchQuery, true)
}

func (m *Model) listSearchFirst() *Model {
	return m.searchFrom(patentListSearchable{m}, m.listSearchQuery, false)
}

// — Popup / overlay ——————————————————————————————————————————————————————————

func (m *Model) currentPopupSearchable() searchable {
	switch m.mode {
	case viewClassifications:
		return classificationSearchable{m}
	case viewInventors:
		return inventorSearchable{m}
	case viewProjectEvents:
		return eventSearchable{m}
	case viewProjectInvoices:
		return invoiceSearchable{m}
	case viewProjectIDS:
		return idsSearchable{m}
	case viewProjectTags:
		return tagSearchable{m}
	case viewTagSelect:
		return tagSelectSearchable{m}
	case viewFamily:
		return familySearchable{m}
	}
	return nil
}

func (m *Model) popupSearchNext() *Model {
	return m.searchFrom(m.currentPopupSearchable(), m.popupSearchQuery, true)
}

func (m *Model) popupSearchFirst() *Model {
	return m.searchFrom(m.currentPopupSearchable(), m.popupSearchQuery, false)
}

// — Searchable implementations ————————————————————————————————————————————————

type classificationSearchable struct{ m *Model }

func (s classificationSearchable) ItemCount() int {
	cls, _ := s.m.repo.ListClassifications(s.m.ctx, s.m.ProjectID, s.m.current.Number)
	return len(cls)
}
func (s classificationSearchable) Match(idx int, query string, ignoreCase bool) bool {
	cls, _ := s.m.repo.ListClassifications(s.m.ctx, s.m.ProjectID, s.m.current.Number)
	if idx < 0 || idx >= len(cls) {
		return false
	}
	c := cls[idx]
	return containsMatch(c.Code, query, ignoreCase) || containsMatch(c.Description, query, ignoreCase)
}
func (s classificationSearchable) SetSelected(idx int) { s.m.classificationSelected = idx }
func (s classificationSearchable) GetSelected() int    { return s.m.classificationSelected }
func (s classificationSearchable) MatchLabel(idx int) string {
	cls, _ := s.m.repo.ListClassifications(s.m.ctx, s.m.ProjectID, s.m.current.Number)
	return cls[idx].Code
}

type inventorSearchable struct{ m *Model }

func (s inventorSearchable) ItemCount() int { return len(s.m.current.Inventors) }
func (s inventorSearchable) Match(idx int, query string, ignoreCase bool) bool {
	return containsMatch(s.m.current.Inventors[idx], query, ignoreCase)
}
func (s inventorSearchable) SetSelected(idx int)       { s.m.inventorSelected = idx }
func (s inventorSearchable) GetSelected() int          { return s.m.inventorSelected }
func (s inventorSearchable) MatchLabel(idx int) string { return s.m.current.Inventors[idx] }

type eventSearchable struct{ m *Model }

func (s eventSearchable) ItemCount() int {
	events, _ := s.m.repo.ListProjectEvents(s.m.ctx, s.m.ProjectID)
	return len(events)
}
func (s eventSearchable) Match(idx int, query string, ignoreCase bool) bool {
	events, _ := s.m.repo.ListProjectEvents(s.m.ctx, s.m.ProjectID)
	e := events[idx]
	return containsMatch(e.EventType, query, ignoreCase) || containsMatch(e.Notes, query, ignoreCase) || containsMatch(e.Reference, query, ignoreCase)
}
func (s eventSearchable) SetSelected(idx int)       { s.m.projectEventsSelected = idx }
func (s eventSearchable) GetSelected() int          { return s.m.projectEventsSelected }
func (s eventSearchable) MatchLabel(idx int) string { return "" }

type invoiceSearchable struct{ m *Model }

func (s invoiceSearchable) ItemCount() int {
	invoices, _ := s.m.repo.ListProjectInvoices(s.m.ctx, s.m.ProjectID)
	return len(invoices)
}
func (s invoiceSearchable) Match(idx int, query string, ignoreCase bool) bool {
	invoices, _ := s.m.repo.ListProjectInvoices(s.m.ctx, s.m.ProjectID)
	inv := invoices[idx]
	return containsMatch(inv.FirmName, query, ignoreCase) || containsMatch(inv.Description, query, ignoreCase) || containsMatch(inv.InvoiceNumber, query, ignoreCase)
}
func (s invoiceSearchable) SetSelected(idx int)       { s.m.projectInvoicesSelected = idx }
func (s invoiceSearchable) GetSelected() int          { return s.m.projectInvoicesSelected }
func (s invoiceSearchable) MatchLabel(idx int) string { return "" }

type idsSearchable struct{ m *Model }

func (s idsSearchable) ItemCount() int {
	entries, _ := s.m.repo.ListIDSEntries(s.m.ctx, s.m.ProjectID)
	return len(entries)
}
func (s idsSearchable) Match(idx int, query string, ignoreCase bool) bool {
	entries, _ := s.m.repo.ListIDSEntries(s.m.ctx, s.m.ProjectID)
	e := entries[idx]
	return containsMatch(e.PatentNumber, query, ignoreCase) || containsMatch(e.Notes, query, ignoreCase)
}
func (s idsSearchable) SetSelected(idx int)       { s.m.projectIDSSelected = idx }
func (s idsSearchable) GetSelected() int          { return s.m.projectIDSSelected }
func (s idsSearchable) MatchLabel(idx int) string { return "" }

type familySearchable struct{ m *Model }

func (s familySearchable) ItemCount() int {
	nodes := s.m.buildFamilyTree()
	return len(nodes)
}
func (s familySearchable) Match(idx int, query string, ignoreCase bool) bool {
	nodes := s.m.buildFamilyTree()
	if idx < 0 || idx >= len(nodes) {
		return false
	}
	node := nodes[idx]
	return containsMatch(node.number, query, ignoreCase) || containsMatch(node.relType, query, ignoreCase) || containsMatch(node.grantYear, query, ignoreCase)
}
func (s familySearchable) SetSelected(idx int) { s.m.familySelected = idx }
func (s familySearchable) GetSelected() int    { return s.m.familySelected }
func (s familySearchable) MatchLabel(idx int) string {
	nodes := s.m.buildFamilyTree()
	if idx < 0 || idx >= len(nodes) {
		return ""
	}
	return nodes[idx].number
}

func containsMatch(s, query string, ignoreCase bool) bool {
	if ignoreCase {
		return strings.Contains(strings.ToLower(s), strings.ToLower(query))
	}
	return strings.Contains(s, query)
}

// highlightTerm bolds and underlines the first occurrence of query in s using
// the same smart-case logic as containsMatch. Returns s unchanged if no match.
//
// Uses targeted SGR resets (\033[22;24m = no-bold + no-underline) instead of
// the full reset \033[m, so surrounding foreground/background survive intact.
func highlightTerm(s, query string) string {
	if query == "" || s == "" {
		return s
	}
	ignoreCase := strings.ToLower(query) == query
	compare, q := s, query
	if ignoreCase {
		compare = strings.ToLower(s)
		q = strings.ToLower(query)
	}
	idx := strings.Index(compare, q)
	if idx < 0 {
		return s
	}
	return s[:idx] + "\033[1;4m" + s[idx:idx+len(query)] + "\033[22;24m" + s[idx+len(query):]
}
