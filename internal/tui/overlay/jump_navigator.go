package overlay

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// JumpNavigator manages Vim-like jump and navigation state for form overlays.
type JumpNavigator struct {
	Active       bool
	PendingCount int
	PendingG     bool
}

// HandleKey processes a key message in jump mode. If the key is handled,
// it updates the focus index and returns the new focus and true.
func (j *JumpNavigator) HandleKey(msg tea.KeyMsg, currentFocus int, numFields int) (int, bool) {
	if !j.Active {
		if msg.Type == tea.KeyRunes && msg.String() == ";" {
			j.Active = true
			j.PendingCount = 0
			j.PendingG = false
			return currentFocus, true
		}
		return currentFocus, false
	}

	// An empty list has nothing to jump to; consume the key (exiting jump mode)
	// rather than dividing by zero in the movement cases below.
	if numFields <= 0 {
		j.reset()
		return currentFocus, true
	}

	switch msg.String() {
	case "j", "down":
		count := j.PendingCount
		if count == 0 {
			count = 1
		}
		newFocus := (currentFocus + count) % numFields
		j.reset()
		return newFocus, true
	case "k", "up":
		count := j.PendingCount
		if count == 0 {
			count = 1
		}
		newFocus := (currentFocus - count) % numFields
		if newFocus < 0 {
			newFocus = (newFocus + numFields) % numFields
		}
		j.reset()
		return newFocus, true
	case "g":
		if j.PendingG {
			j.reset()
			return 0, true
		}
		if j.PendingCount > 0 {
			line := j.PendingCount
			if line < 1 {
				line = 1
			}
			if line > numFields {
				line = numFields
			}
			j.reset()
			return line - 1, true
		}
		j.PendingG = true
		return currentFocus, true
	case "G":
		if j.PendingCount > 0 {
			line := j.PendingCount
			if line < 1 {
				line = 1
			}
			if line > numFields {
				line = numFields
			}
			j.reset()
			return line - 1, true
		}
		j.reset()
		return numFields - 1, true
	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		digit := int(msg.String()[0] - '0')
		j.PendingCount = j.PendingCount*10 + digit
		return currentFocus, true
	case "enter", "Enter":
		if j.PendingCount > 0 {
			line := j.PendingCount
			if line < 1 {
				line = 1
			}
			if line > numFields {
				line = numFields
			}
			j.reset()
			return line - 1, true
		}
		j.reset()
		return currentFocus, true
	case "esc", "Esc":
		j.reset()
		return currentFocus, true
	default:
		// Any other key exits jump mode
		j.reset()
		return currentFocus, true
	}
}

func (j *JumpNavigator) reset() {
	j.Active = false
	j.PendingCount = 0
	j.PendingG = false
}

// GutterPrefix returns the prefix string to display in the gutter for the given line number.
func (j *JumpNavigator) GutterPrefix(lineNum int) string {
	if j.Active {
		if j.PendingCount > 0 {
			return fmt.Sprintf("%d? ", j.PendingCount)
		}
		return fmt.Sprintf(";%d ", lineNum)
	}
	return fmt.Sprintf(" %d ", lineNum)
}

// HintSuffix returns the contextual shortcut hint based on jumpMode status.
func (j *JumpNavigator) HintSuffix(focus, typeFieldIndex int, isEdit bool) string {
	if j.Active {
		return "[jump mode] type line number + [enter]/[g] to jump · [count]+[j]/[k] to move · [esc] cancel"
	}
	
	// Default meta-form focus-specific help text
	var base string
	if isEdit {
		base = "[tab] field · [enter] save · [esc] cancel · [ctrl+u] clear · deadline auto-recomputed from mail date"
	} else {
		base = "[tab] field · [enter] load · [esc] cancel · [ctrl+u] clear · deadline auto-set from mail date"
	}
	
	if focus == typeFieldIndex {
		cycleStr := "[tab] field · [enter] cycle type · [esc] cancel · deadline auto-set from mail date"
		if isEdit {
			cycleStr = "[tab] field · [enter] cycle type · [esc] cancel · deadline auto-recomputed from mail date"
		}
		return cycleStr + "\n[space] cycle type · first letter (f/r/a/o/n) to set type directly · [;] jump mode"
	}
	return base + " · [;] jump mode"
}
