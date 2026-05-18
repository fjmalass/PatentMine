package keys

import (
	"strings"
	"testing"
)

// lookupFrom builds a Lookup over a fixed set of bound sequences.
func lookupFrom(bound ...string) Lookup {
	set := make(map[string]bool, len(bound))
	for _, b := range bound {
		set[b] = true
	}
	return func(seq string) Match {
		complete := set[seq]
		prefix := false
		for b := range set {
			if strings.HasPrefix(b, seq+" ") {
				prefix = true
			}
		}
		switch {
		case complete && prefix:
			return CompleteAndPrefix
		case complete:
			return Complete
		case prefix:
			return Prefix
		default:
			return NoMatch
		}
	}
}

func TestReaderSingleKey(t *testing.T) {
	var r Reader
	lookup := lookupFrom("j")
	chord, ok := r.Feed("j", lookup)
	if !ok {
		t.Fatal("single bound key should complete a chord")
	}
	if chord.Sequence() != "j" || chord.Repeat() != 1 {
		t.Fatalf("chord = %+v, want j x1", chord)
	}
}

func TestReaderCountPrefix(t *testing.T) {
	var r Reader
	lookup := lookupFrom("j")
	if _, ok := r.Feed("3", lookup); ok {
		t.Fatal("a count digit should not complete a chord")
	}
	if r.Pending() != "3" {
		t.Fatalf("pending = %q, want 3", r.Pending())
	}
	chord, ok := r.Feed("j", lookup)
	if !ok {
		t.Fatal("chord should complete after the count")
	}
	if chord.Count != 3 || chord.Repeat() != 3 || chord.Sequence() != "j" {
		t.Fatalf("chord = %+v, want j x3", chord)
	}
}

func TestReaderMultiDigitCount(t *testing.T) {
	var r Reader
	lookup := lookupFrom("j")
	r.Feed("1", lookup)
	r.Feed("0", lookup) // "0" extends an in-progress count
	chord, ok := r.Feed("j", lookup)
	if !ok || chord.Count != 10 {
		t.Fatalf("chord = %+v ok=%v, want count 10", chord, ok)
	}
}

func TestReaderMultiKeySequence(t *testing.T) {
	var r Reader
	lookup := lookupFrom("g g", "G")
	if _, ok := r.Feed("g", lookup); ok {
		t.Fatal("first g should buffer as a prefix, not complete")
	}
	if r.Pending() != "g" {
		t.Fatalf("pending = %q, want g", r.Pending())
	}
	chord, ok := r.Feed("g", lookup)
	if !ok || chord.Sequence() != "g g" {
		t.Fatalf("chord = %+v ok=%v, want sequence 'g g'", chord, ok)
	}
}

func TestReaderCountThenMultiKey(t *testing.T) {
	var r Reader
	lookup := lookupFrom("g g")
	r.Feed("5", lookup)
	r.Feed("g", lookup)
	chord, ok := r.Feed("g", lookup)
	if !ok || chord.Count != 5 || chord.Sequence() != "g g" {
		t.Fatalf("chord = %+v ok=%v, want 'g g' x5", chord, ok)
	}
}

func TestReaderUnknownKeyResets(t *testing.T) {
	var r Reader
	lookup := lookupFrom("j")
	if _, ok := r.Feed("z", lookup); ok {
		t.Fatal("an unbound key should not complete a chord")
	}
	if r.Pending() != "" {
		t.Fatalf("pending = %q, want empty after an unbound key", r.Pending())
	}
}

func TestReaderZeroIsAKeyNotACount(t *testing.T) {
	var r Reader
	lookup := lookupFrom("0")
	chord, ok := r.Feed("0", lookup)
	if !ok || chord.Sequence() != "0" {
		t.Fatalf("chord = %+v ok=%v, want '0' treated as a key", chord, ok)
	}
}

func TestReaderResetDiscardsPending(t *testing.T) {
	var r Reader
	lookup := lookupFrom("g g")
	r.Feed("2", lookup)
	r.Feed("g", lookup)
	r.Reset()
	if r.Pending() != "" {
		t.Fatalf("pending = %q, want empty after Reset", r.Pending())
	}
}
