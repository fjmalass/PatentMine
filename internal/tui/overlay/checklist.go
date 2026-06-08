package overlay

// CheckState is the shared three-state checkbox model used by assignment
// overlays that target one or more subjects.
type CheckState int

const (
	CheckUnchecked CheckState = iota
	CheckChecked
	CheckIndeterminate
)

type checklistChange struct {
	key string
	on  bool
}

func checklistStateForCount(count, targetCount int) CheckState {
	switch {
	case targetCount > 0 && count == targetCount:
		return CheckChecked
	case count > 0:
		return CheckIndeterminate
	default:
		return CheckUnchecked
	}
}

func checklistStates(keys []string, counts map[string]int, targetCount int) (map[string]CheckState, map[string]CheckState) {
	checked := make(map[string]CheckState, len(keys))
	initial := make(map[string]CheckState, len(keys))
	for _, key := range keys {
		state := checklistStateForCount(counts[key], targetCount)
		checked[key] = state
		initial[key] = state
	}
	return checked, initial
}

func toggleChecklistState(state CheckState) CheckState {
	switch state {
	case CheckIndeterminate:
		return CheckChecked
	case CheckChecked:
		return CheckUnchecked
	default:
		return CheckChecked
	}
}

func checklistChanges(keys []string, checked, initial map[string]CheckState) []checklistChange {
	changes := make([]checklistChange, 0, len(keys))
	for _, key := range keys {
		finalState := checked[key]
		if finalState == initial[key] {
			continue
		}
		switch finalState {
		case CheckChecked:
			changes = append(changes, checklistChange{key: key, on: true})
		case CheckUnchecked:
			changes = append(changes, checklistChange{key: key})
		}
	}
	return changes
}
