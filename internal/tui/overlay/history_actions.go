package overlay

import "patentmine/internal/observability"

// HistoryClassification describes how the /history overlay treats one action.
//   - InHistory: the record appears in the overlay list.
//   - Hidden: emitted by the engine but intentionally not shown (background
//     events, low-signal noise).
//
// Every value in observability.AllActions must fall into one of those buckets;
// TestEveryActionClassifiedByHistory enforces the rule.
type HistoryClassification int

const (
	// HistoryHidden means the action is emitted but not shown in /history.
	HistoryHidden HistoryClassification = iota
	// HistoryListed means the action appears as a history row.
	HistoryListed
)

// classifyAction returns how the overlay should treat the given action.
func classifyAction(action string) HistoryClassification {
	if observability.IsHistoryFeedAction(action) {
		return HistoryListed
	}
	return HistoryHidden
}

// IsListedAction reports whether the action's record should be included when
// /history is opened. Call sites use this instead of an inline list to avoid
// drift when a new action is added.
func IsListedAction(action string) bool {
	return classifyAction(action) == HistoryListed
}

// IsProjectPatentAction reports whether the action's EntityID is shaped
// "<projectID>/<patentNumber>[/<extra>]". Used to parse project + patent out of
// the record for display and replay routing.
func IsProjectPatentAction(action string) bool {
	switch action {
	case observability.ActionMembershipSetState,
		observability.ActionPatentTagAssign,
		observability.ActionPatentTagRemove,
		observability.ActionIDSEntrySave,
		observability.ActionIDSEntryDelete,
		observability.ActionNotesSave,
		observability.ActionNotesDelete:
		return true
	}
	return false
}

// ReplayScope returns the pane scope an action's replay should land on, or ""
// to fall back to the record's metadata["scope"] (used by ui.focus).
func ReplayScope(action string) string {
	switch action {
	case observability.ActionIDSEntrySave, observability.ActionIDSEntryDelete:
		return "ids"
	}
	return ""
}
