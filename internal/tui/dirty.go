package tui

type dirtyMask uint8

const (
	dirtyDetail     dirtyMask = 1 << iota // repopulate detailCache
	dirtyFamily                            // invalidate family caches
	dirtyProjectTags                       // reload m.projectTags
)

type dirtyState struct {
	mask dirtyMask
}

func (m *Model) markDirty(mask dirtyMask) *Model {
	m.dirty.mask |= mask
	return m
}

// flushDirty is called in goBack() after restore(). Runs pending cache
// refreshes against the now-restored m.current / m.ProjectID.
func (m *Model) flushDirty() *Model {
	if m.dirty.mask == 0 {
		return m
	}
	if m.dirty.mask&dirtyDetail != 0 {
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
