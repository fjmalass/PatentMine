package tui

import (
	"fmt"

	"patentmine/internal/command"
	"patentmine/internal/domain"
)

// confirmFn decides whether to show a yes/no dialog before an action runs.
// Returns the dialog message and whether confirmation is required.
type confirmFn func(numbers []domain.PatentNumber) (msg string, needs bool)

// CommandPolicy declares an optional pre-action confirmation for a command.
// A nil Confirm is a no-op (action fires immediately).
type CommandPolicy struct {
	Confirm confirmFn
}

// commandPolicies maps command IDs to their confirmation behaviour.
// Commands absent from this map fire immediately with no dialog.
var commandPolicies = map[command.ID]CommandPolicy{
	command.MarkDeleted: {
		Confirm: func(ns []domain.PatentNumber) (string, bool) {
			if len(ns) == 1 {
				return "Mark " + ns[0].String() + " as deleted?", true
			}
			return fmt.Sprintf("Mark %d patents as deleted?", len(ns)), true
		},
	},
	command.PatentDelete: {
		Confirm: func(ns []domain.PatentNumber) (string, bool) {
			if len(ns) == 1 {
				return "Delete " + ns[0].String() + "? This cannot be undone.", true
			}
			return fmt.Sprintf("Delete %d patents? This cannot be undone.", len(ns)), true
		},
	},
	command.MarkStored: {
		Confirm: func(ns []domain.PatentNumber) (string, bool) {
			if len(ns) >= 2 {
				return fmt.Sprintf("Mark %d patents as stored?", len(ns)), true
			}
			return "", false
		},
	},
	command.MarkUnderReview: {
		Confirm: func(ns []domain.PatentNumber) (string, bool) {
			if len(ns) >= 2 {
				return fmt.Sprintf("Mark %d patents as under review?", len(ns)), true
			}
			return "", false
		},
	},
	command.MarkIgnored: {
		Confirm: func(ns []domain.PatentNumber) (string, bool) {
			if len(ns) >= 2 {
				return fmt.Sprintf("Mark %d patents as ignored?", len(ns)), true
			}
			return "", false
		},
	},
}
