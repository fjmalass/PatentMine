package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"patentmine/internal/domain"
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
			created := t.CreatedAt.Format("2006-01-02")

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

	var b strings.Builder
	b.WriteString(m.renderPopupHeader(fmt.Sprintf("Tags for %s", m.current.Number)))

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

	// Also load current patent's tags
	m.selectedPatentTags = make(map[int64]bool)
	if m.current.Number != "" {
		pTags, err := m.repo.GetPatentTags(m.ctx, m.current.Number)
		if err == nil {
			for _, pt := range pTags {
				m.selectedPatentTags[pt.ID] = true
			}
		}
	}
	return m
}

type tagSearchable struct{ m *Model }

func (s tagSearchable) ItemCount() int { return len(s.m.projectTags) }
func (s tagSearchable) GetSelected() int { return s.m.projectTagsSelected }
func (s tagSearchable) SetSelected(i int) { s.m.projectTagsSelected = i }
func (s tagSearchable) Match(i int, query string, ignoreCase bool) bool {
	return containsMatch(s.m.projectTags[i].Name, query, ignoreCase)
}
func (s tagSearchable) MatchLabel(i int) string { return s.m.projectTags[i].Name }

type tagSelectSearchable struct{ m *Model }

func (s tagSelectSearchable) ItemCount() int { return len(s.m.availableTags) }
func (s tagSelectSearchable) GetSelected() int { return s.m.tagSelectSelected }
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

