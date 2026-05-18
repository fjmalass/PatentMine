package keys

import "strings"

// sequenceSep joins keys in a canonical sequence string.
const sequenceSep = " "

// Chord is a resolved unit of input: an optional repeat count and one or more
// keys. "3j" parses to Chord{Count: 3, Keys: ["j"]}; "gg" to Chord{Keys:
// ["g", "g"]}.
type Chord struct {
	Count int // 0 means no count was typed; apply as 1.
	Keys  []Key
}

// Sequence renders a key slice as the canonical, count-independent string used
// as a keymap lookup key: keys joined by a space ("g g", "j", "ctrl+d").
func Sequence(seq []Key) string {
	parts := make([]string, len(seq))
	for i, k := range seq {
		parts[i] = string(k)
	}
	return strings.Join(parts, sequenceSep)
}

// Sequence returns the chord's canonical key-sequence string.
func (c Chord) Sequence() string {
	return Sequence(c.Keys)
}

// Repeat returns the effective repeat count, never less than 1.
func (c Chord) Repeat() int {
	if c.Count <= 0 {
		return 1
	}
	return c.Count
}
