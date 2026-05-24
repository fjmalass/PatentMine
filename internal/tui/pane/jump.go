package pane

import (
	tea "github.com/charmbracelet/bubbletea"
	"patentmine/internal/tui/render"
)

// JumpController encapsulates the standard Jump Mode state, key assignment,
// and keyboard event handling. By embedding this controller, panes can
// completely delegate Jump Mode behavior and avoid duplication.
type JumpController struct {
	Active   bool
	JumpKeys map[string]rune
	Anchors  []render.JumpAnchor
}

// NewJumpController builds a new JumpController instance.
func NewJumpController() *JumpController {
	return &JumpController{
		JumpKeys: make(map[string]rune),
	}
}

// SetJumpActive updates the jump mode active status.
func (j *JumpController) SetJumpActive(active bool) {
	j.Active = active
}

// JumpActive reports whether jump mode is active.
func (j *JumpController) JumpActive() bool {
	return j.Active
}

// JumpAnchors returns the slice of recorded jump targets.
func (j *JumpController) JumpAnchors() []render.JumpAnchor {
	return j.Anchors
}

// ClearAnchors clears the current set of jump anchors.
func (j *JumpController) ClearAnchors() {
	j.Anchors = j.Anchors[:0]
}

// AddAnchor appends a new scroll/jump target anchor.
func (j *JumpController) AddAnchor(label, value string, line int, local bool, key rune) {
	j.Anchors = append(j.Anchors, render.JumpAnchor{
		Key:   key,
		Label: label,
		Line:  line,
		Value: value,
		Local: local,
	})
}

// JumpKey returns the assigned key for a label, or 0 if none is assigned.
func (j *JumpController) JumpKey(label string) rune {
	if j.JumpKeys != nil {
		if key, ok := j.JumpKeys[label]; ok {
			return key
		}
	}
	return 0
}

// Compute assigns stable single-key jump targets to labels, ensuring they
// never collide with keys in the bound slice (which are marked used first).
// An optional overrideFunc allows specialized custom key bindings first.
func (j *JumpController) Compute(labels []string, bound []rune, overrideFunc func(label string, used map[rune]bool) rune, prioritizeDigits bool) {
	used := make(map[rune]bool, len(labels))

	j.JumpKeys = make(map[string]rune, len(labels))

	// Pass 1: Satisfy manual overrides/specific bindings first
	if overrideFunc != nil {
		for _, label := range labels {
			if key := overrideFunc(label, used); key != 0 {
				j.JumpKeys[label] = key
				used[key] = true
			}
		}
	}

	// Pass 2: Dynamically assign remaining keys
	for _, label := range labels {
		if _, ok := j.JumpKeys[label]; !ok {
			key := render.AssignKey(label, used, prioritizeDigits)
			j.JumpKeys[label] = key
			used[key] = true
		}
	}
}

// HandleKey processes a raw keyboard event when jump mode is active.
// It returns a boolean indicating whether the key event was consumed by jump mode.
func (j *JumpController) HandleKey(msg tea.KeyMsg, onJump func(line int)) bool {
	if !j.Active {
		return false
	}

	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		r := msg.Runes[0]
		for _, a := range j.Anchors {
			if a.Key == r {
				onJump(a.Line)
				j.Active = false // Automatically turn off jump mode upon a successful jump
				return true
			}
		}
	}

	// Any other key (including esc) exits jump mode and is safely consumed so it doesn't leak
	j.Active = false
	return true
}
