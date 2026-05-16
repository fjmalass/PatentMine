package tui

import "patentmine/internal/changes"

type dirtyMask uint8

const (
	dirtyDetail      dirtyMask = 1 << iota // re-fetch m.current and repopulate detailCache
	dirtyFamily                            // invalidate family caches
	dirtyProjectTags                       // reload m.projectTags
	dirtyList                              // re-read the patent list from the DB
)

type dirtyState struct {
	mask dirtyMask
}

func (m *Model) markDirty(mask dirtyMask) *Model {
	m.dirty.mask |= mask
	return m
}

// scopeDirty maps a changes.Scope to the matching cache-invalidation flags.
func scopeDirty(s changes.Scope) dirtyMask {
	var d dirtyMask
	if s&changes.ScopeList != 0 {
		d |= dirtyList
	}
	if s&changes.ScopeDetail != 0 {
		d |= dirtyDetail
	}
	if s&changes.ScopeFamily != 0 {
		d |= dirtyFamily
	}
	if s&changes.ScopeTags != 0 {
		d |= dirtyProjectTags
	}
	return d
}

// applyChange runs c through the History and marks the affected views dirty so
// flushDirty refreshes them at the end of the turn. On failure it records the
// error and returns false; the caller must not proceed as if it succeeded.
func (m *Model) applyChange(c changes.Change) bool {
	scope, err := m.history.Apply(m.ctx, c)
	if err != nil {
		m.err = err.Error()
		return false
	}
	m.markDirty(scopeDirty(scope))
	return true
}

// flushDirty runs every pending cache refresh and clears the dirty mask. It is
// called once at the end of Update() so any change applied during the turn is
// reflected on screen regardless of how the handler returned.
func (m *Model) flushDirty() *Model {
	if m.dirty.mask == 0 {
		return m
	}
	if m.dirty.mask&dirtyList != 0 {
		m = m.reloadPatents()
	}
	if m.dirty.mask&dirtyDetail != 0 {
		if m.current.Number != "" {
			if p, err := m.repo.GetPatent(m.ctx, m.ProjectID, m.current.Number); err == nil {
				m.current = p
			}
		}
		m.populateDetailCache()
	}
	if m.dirty.mask&dirtyFamily != 0 {
		m.invalidateFamilyCaches()
	}
	if m.dirty.mask&dirtyProjectTags != 0 {
		m = m.reloadProjectTags()
	}
	m.dirty.mask = 0
	return m
}
