package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
)

func (m *Model) tagCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.projectTagsSelected = 0
		return m.navigateTo(viewProjectTags).reloadProjectTags(), nil
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case tagSubList:
		m.projectTagsSelected = 0
		return m.navigateTo(viewProjectTags).reloadProjectTags(), nil
	case tagSubAdd:
		if len(args) < 2 {
			m.err = "usage: :tag add <name> [color]"
			return m, nil
		}
		name := args[1]
		color := ""
		if len(args) >= 3 {
			color = args[2]
		}
		_, err := m.repo.CreateTag(m.ctx, m.ProjectID, name, color)
		if err != nil {
			m.err = "failed to create tag: " + err.Error()
		} else {
			m.message = fmt.Sprintf("tag '%s' created", name)
			m.reloadProjectTags()
		}
	case tagSubDelete:
		if len(args) < 2 {
			m.err = "usage: :tag delete <name>"
			return m, nil
		}
		name := args[1]
		tag, err := m.repo.GetTagByName(m.ctx, m.ProjectID, name)
		if err != nil {
			m.err = "tag not found: " + name
			return m, nil
		}
		if err := m.repo.DeleteTag(m.ctx, tag.ID); err != nil {
			m.err = "failed to delete tag: " + err.Error()
		} else {
			m.message = fmt.Sprintf("tag '%s' deleted", name)
			m.reloadProjectTags()
		}
	case tagSubRename:
		if len(args) < 3 {
			m.err = "usage: :tag rename <old> <new>"
			return m, nil
		}
		oldName := args[1]
		newName := args[2]
		tag, err := m.repo.GetTagByName(m.ctx, m.ProjectID, oldName)
		if err != nil {
			m.err = "tag not found: " + oldName
			return m, nil
		}
		if err := m.repo.RenameTag(m.ctx, tag.ID, newName); err != nil {
			m.err = "failed to rename tag: " + err.Error()
		} else {
			m.message = fmt.Sprintf("tag '%s' renamed to '%s'", oldName, newName)
			m.reloadProjectTags()
		}
	case tagSubColor:
		if len(args) < 3 {
			m.err = "usage: :tag color <name> <color>"
			return m, nil
		}
		name := args[1]
		color := args[2]
		tag, err := m.repo.GetTagByName(m.ctx, m.ProjectID, name)
		if err != nil {
			m.err = "tag not found: " + name
			return m, nil
		}
		if err := m.repo.UpdateTagColor(m.ctx, tag.ID, color); err != nil {
			m.err = "failed to update tag color: " + err.Error()
		} else {
			m.message = fmt.Sprintf("tag '%s' color set to '%s'", name, color)
			m.reloadProjectTags()
		}
	case tagSubFilter:
		if len(args) < 2 {
			m.tagFilter = EmptyFilter
			m.message = "tag filter cleared"
			m.setMode(viewList)
			return m.refreshList()
		}
		m.tagFilter = args[1]
		m.message = fmt.Sprintf("filtering by tag: %s", m.tagFilter)
		m.setMode(viewList)
		return m.refreshList()
	case tagSubHelp:
		m.message = "Tag Commands: list (tl), add (ta) <name> [color], delete (td) <name>, rename (tr) <old> <new>, color (tc) <name> <color>, filter (tf) <name>"
	default:
		m.err = "unknown tag subcommand: " + sub
	}

	return m, nil
}
