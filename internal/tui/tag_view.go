package tui

import (
	"fmt"
	"strings"

	"patentmine/internal/domain"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) viewProjectTags() string {
	tags := m.projectTags
	base := overlayBase()
	subtleStyle := base.Foreground(lipgloss.Color(ColorSubtle))
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)

	var b strings.Builder
	b.WriteString(m.renderPopupHeader("Project Tags"))

	if len(tags) == 0 {
		b.WriteString(dimStyle.Render("No tags defined. Create one with :tag add <name> (shortcut: ta <name>)"))
		b.WriteString("\n\n")
		b.WriteString(subtleStyle.Render("Tags help you categorize patents across different projects."))
	} else {
		sel := clamp(m.projectTagsSelected, 0, len(tags)-1)
		window := pageWindow(sel, len(tags), m.overlayPageSize())

		b.WriteString(subtleStyle.Render(pageStatus(m.text.T(TextValuePageStatus), window)))
		b.WriteString("\n\n")

		headerStyle := base.Foreground(lipgloss.Color(ColorSubtle)).Underline(true)
		b.WriteString(headerStyle.Render(fmt.Sprintf("  %-3s %-20s %-10s %s", "#", "Tag Name", "Usage", "Created")))
		b.WriteString("\n")

		for i := window.Start; i < window.End; i++ {
			t := tags[i]
			rowStyle := base.Foreground(lipgloss.Color(ColorSubtle))
			tagStyle := tagStyleBase()
			if i == sel {
				rowStyle = rowStyle.Bold(true).Reverse(true)
				tagStyle = tagStyle.Bold(true).Reverse(true)
			}
			prefix := "  "
			if i == sel {
				prefix = "→ "
			}

			usage := fmt.Sprintf("%d", t.PatentCount)
			created := t.CreatedAt.Format(dateFmtDate)

			b.WriteString(rowStyle.Render(prefix))
			b.WriteString(rowStyle.Render(fmt.Sprintf("%-3d ", i+1)))
			b.WriteString(tagStyle.Render(fmt.Sprintf("%-20s ", t.Name)))
			b.WriteString(rowStyle.Render(fmt.Sprintf("%-10s ", usage)))
			b.WriteString(rowStyle.Render(created))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m *Model) viewTagSelect() string {
	tags := m.availableTags
	base := overlayBase()
	subtleStyle := base.Foreground(lipgloss.Color(ColorSubtle))
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)

	title := "Tags · " + m.activeSelection.livePatent
	if m.activeSelection.IsMulti() {
		title = fmt.Sprintf("Tags · %d patents", m.activeSelection.Count())
	}

	var b strings.Builder
	b.WriteString(m.renderPopupHeader(title))

	if len(tags) == 0 {
		b.WriteString(dimStyle.Render("No tags defined. Create one with :tag add <name>"))
	} else {
		sel := clamp(m.tagSelectSelected, 0, len(tags)-1)
		window := pageWindow(sel, len(tags), m.overlayPageSize())

		b.WriteString(subtleStyle.Render(pageStatus(m.text.T(TextValuePageStatus), window)))
		b.WriteString("\n\n")

		for i := window.Start; i < window.End; i++ {
			t := tags[i]
			rowStyle := base.Foreground(lipgloss.Color(ColorSubtle))
			tagStyle := tagStyleBase()
			if i == sel {
				rowStyle = rowStyle.Bold(true).Reverse(true)
				tagStyle = tagStyle.Bold(true).Reverse(true)
			}
			prefix := "  "
			if i == sel {
				prefix = "→ "
			}

			checked := "[ ]"
			if m.selectedPatentTags[t.ID] {
				checked = "[x]"
				if i != sel {
					tagStyle = tagStyle.Foreground(lipgloss.Color(ColorSuccess))
				}
			}

			b.WriteString(rowStyle.Render(prefix))
			b.WriteString(rowStyle.Render(fmt.Sprintf("%-4s ", checked)))
			b.WriteString(tagStyle.Render(t.Name))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m *Model) reloadProjectTags() *Model {
	tags, err := m.repo.ListTagsWithCounts(m.ctx, m.ProjectID)
	if err != nil {
		m.err = "failed to load tags: " + err.Error()
		return m
	}
	m.projectTags = tags
	return m
}

func (m *Model) reloadAvailableTags() *Model {
	tags, err := m.repo.ListTagsWithCounts(m.ctx, m.ProjectID)
	if err != nil {
		m.err = "failed to load tags: " + err.Error()
		return m
	}
	m.availableTags = make([]domain.Tag, len(tags))
	for i, t := range tags {
		m.availableTags[i] = t.Tag
	}

	m.selectedPatentTags = make(map[int64]bool)
	preloadNum := m.activeSelection.livePatent
	if preloadNum == "" {
		preloadNum = m.current.Number
	}
	if preloadNum != "" {
		pTags, err := m.repo.GetPatentTags(m.ctx, preloadNum)
		if err == nil {
			for _, pt := range pTags {
				m.selectedPatentTags[pt.ID] = true
			}
		}
	}
	return m
}

// applyTagsToSelection applies the current selectedPatentTags state to all
// patents in the active selection. livePatent was already mutated live via
// space-toggle; this propagates the same tag state to the rest.
func (m *Model) applyTagsToSelection() (tea.Model, tea.Cmd) {
	sel := m.activeSelection
	patentNumbers, err := sel.PatentNumbers(m)
	if err != nil || len(patentNumbers) == 0 {
		m.activeSelection = selectionContext{}
		return m.goBack()
	}
	m.logger.Info("bulk tag apply started", "project", m.ProjectID, "patents", len(patentNumbers), "tags", len(m.availableTags))
	count := 0
	for _, num := range patentNumbers {
		if num == sel.livePatent {
			count++
			continue // already applied live via space-toggle
		}
		for _, tag := range m.availableTags {
			if m.selectedPatentTags[tag.ID] {
				if err := m.repo.ApplyTagToPatent(m.ctx, num, tag.ID); err != nil {
					m.logger.Error("tag apply failed", "project", m.ProjectID, "patent", num, "tag", tag.Name, "error", err)
				} else {
					m.logActivity(ActivityTagApply, num, tag.Name)
				}
			} else {
				if err := m.repo.RemoveTagFromPatent(m.ctx, num, tag.ID); err != nil {
					m.logger.Error("tag remove failed", "project", m.ProjectID, "patent", num, "tag", tag.Name, "error", err)
				} else {
					m.logActivity(ActivityTagRemove, num, tag.Name)
				}
			}
		}
		count++
	}
	m.logger.Info("bulk tag apply completed", "project", m.ProjectID, "patents", count)
	if count > 1 {
		m.message = fmt.Sprintf("tags updated for %d patents", count)
	}
	m.activeSelection = selectionContext{}
	m.clearVisualMode()
	if len(m.backStack) > 0 {
		m.backStack[len(m.backStack)-1].visualMode = false
	}
	m = m.markDirty(dirtyDetail | dirtyProjectTags)
	return m.goBack()
}

type tagSearchable struct{ m *Model }

func (s tagSearchable) ItemCount() int    { return len(s.m.projectTags) }
func (s tagSearchable) GetSelected() int  { return s.m.projectTagsSelected }
func (s tagSearchable) SetSelected(i int) { s.m.projectTagsSelected = i }
func (s tagSearchable) Match(i int, query string, ignoreCase bool) bool {
	return containsMatch(s.m.projectTags[i].Name, query, ignoreCase)
}
func (s tagSearchable) MatchLabel(i int) string { return s.m.projectTags[i].Name }

type tagSelectSearchable struct{ m *Model }

func (s tagSelectSearchable) ItemCount() int    { return len(s.m.availableTags) }
func (s tagSelectSearchable) GetSelected() int  { return s.m.tagSelectSelected }
func (s tagSelectSearchable) SetSelected(i int) { s.m.tagSelectSelected = i }
func (s tagSelectSearchable) Match(i int, query string, ignoreCase bool) bool {
	return containsMatch(s.m.availableTags[i].Name, query, ignoreCase)
}
func (s tagSelectSearchable) MatchLabel(i int) string { return s.m.availableTags[i].Name }

func tagStyleBase() lipgloss.Style {
	return overlayBase().Foreground(lipgloss.Color(ColorThemeTags))
}

func formatTags(tags []domain.Tag) string {
	if len(tags) == 0 {
		return "-"
	}
	var names []string
	for _, t := range tags {
		names = append(names, t.Name)
	}
	return strings.Join(names, ",")
}

func (m *Model) handleViewProjectTagsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyVimDown, keyArrowDown:
		m.projectTagsSelected = clamp(m.projectTagsSelected+m.consumeCount(1), 0, len(m.projectTags)-1)
	case keyVimUp, keyArrowUp:
		m.projectTagsSelected = max(0, m.projectTagsSelected-m.consumeCount(1))
	case keyDelete:
		if m.projectTagsSelected >= 0 && m.projectTagsSelected < len(m.projectTags) {
			tag := m.projectTags[m.projectTagsSelected]
			if err := m.repo.DeleteTag(m.ctx, tag.ID); err != nil {
				m.logger.Error("tag delete failed", "project", m.ProjectID, "tag", tag.Name, "error", err)
				m.err = err.Error()
			} else {
				m.logger.Info("tag deleted", "project", m.ProjectID, "tag", tag.Name)
				m.logActivity(ActivityTagDelete, tag.Name, "")
				m.message = fmt.Sprintf("tag '%s' deleted", tag.Name)
				m.reloadProjectTags()
			}
		}
	case "r":
		if m.projectTagsSelected >= 0 && m.projectTagsSelected < len(m.projectTags) {
			tag := m.projectTags[m.projectTagsSelected]
			m.input.Focus()
			m.input.SetValue(fmt.Sprintf(":tag rename %s ", tag.Name))
			return m, nil
		}
	case keyAddToIDS:
		m.input.Focus()
		m.input.SetValue(":tag add ")
		return m, nil
	case keyEsc, keyBack:
		return m.goBack()
	default:
		m.tryAccumulateCount(msg.String())
	}
	return m, nil
}

func (m *Model) handleViewTagSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyVimDown, keyArrowDown:
		m.tagSelectSelected = clamp(m.tagSelectSelected+m.consumeCount(1), 0, len(m.availableTags)-1)
	case keyVimUp, keyArrowUp:
		m.tagSelectSelected = max(0, m.tagSelectSelected-m.consumeCount(1))
	case " ":
		if m.tagSelectSelected >= 0 && m.tagSelectSelected < len(m.availableTags) {
			tag := m.availableTags[m.tagSelectSelected]
			liveNum := m.activeSelection.livePatent
			if liveNum == "" {
				liveNum = m.current.Number
			}
			if m.selectedPatentTags[tag.ID] {
				if err := m.repo.RemoveTagFromPatent(m.ctx, liveNum, tag.ID); err != nil {
					m.logger.Error("tag remove failed", "project", m.ProjectID, "patent", liveNum, "tag", tag.Name, "error", err)
					m.err = err.Error()
				} else {
					m.logger.Info("tag removed", "project", m.ProjectID, "patent", liveNum, "tag", tag.Name)
					m.logActivity(ActivityTagRemove, liveNum, tag.Name)
					m.selectedPatentTags[tag.ID] = false
				}
			} else {
				if err := m.repo.ApplyTagToPatent(m.ctx, liveNum, tag.ID); err != nil {
					m.logger.Error("tag apply failed", "project", m.ProjectID, "patent", liveNum, "tag", tag.Name, "error", err)
					m.err = err.Error()
				} else {
					m.logger.Info("tag applied", "project", m.ProjectID, "patent", liveNum, "tag", tag.Name)
					m.logActivity(ActivityTagApply, liveNum, tag.Name)
					m.selectedPatentTags[tag.ID] = true
				}
			}
		}
	case keyEnter, keyCtrlS:
		return m.applyTagsToSelection()
	case keyEsc, keyBack:
		m.activeSelection = selectionContext{}
		return m.goBack()
	}
	return m, nil
}
