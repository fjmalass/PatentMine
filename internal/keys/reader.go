package keys

import "strconv"

// Reader composes raw key presses into Chords. It supports a Vim-style numeric
// count prefix and multi-key sequences such as "g g". The Reader holds no
// binding data: the caller passes a Lookup describing the active keymap on
// every key, so one Reader serves any context or overlay.
type Reader struct {
	count   int
	pending []Key
}

// Lookup classifies a candidate sequence string against the active bindings.
type Lookup func(sequence string) Match

// Pending returns the in-progress input for display in a status line — "3",
// "3g", "g". It is empty when the Reader is idle.
func (r *Reader) Pending() string {
	prefix := ""
	if r.count > 0 {
		prefix = strconv.Itoa(r.count)
	}
	return prefix + Sequence(r.pending)
}

// Reset discards any in-progress input.
func (r *Reader) Reset() {
	r.count = 0
	r.pending = nil
}

// Feed consumes one key. It returns a completed Chord (ok == true) once the
// buffered input matches a binding; otherwise it buffers the key and returns
// false. Input that can never match a binding is discarded.
func (r *Reader) Feed(k Key, lookup Lookup) (Chord, bool) {
	// A digit typed before any sequence builds the repeat count. "0" counts
	// only when a count is already under way; otherwise it is an ordinary key.
	if len(r.pending) == 0 && isCountDigit(k, r.count) {
		r.count = r.count*10 + int(k[0]-'0')
		return Chord{}, false
	}

	r.pending = append(r.pending, k)
	switch lookup(Sequence(r.pending)) {
	case Complete, CompleteAndPrefix:
		chord := Chord{Count: r.count, Keys: append([]Key(nil), r.pending...)}
		r.Reset()
		return chord, true
	case Prefix:
		return Chord{}, false
	default: // NoMatch
		r.Reset()
		return Chord{}, false
	}
}

// isCountDigit reports whether k should extend the repeat count.
func isCountDigit(k Key, count int) bool {
	if len(k) != 1 || k[0] < '0' || k[0] > '9' {
		return false
	}
	return k != "0" || count > 0
}
