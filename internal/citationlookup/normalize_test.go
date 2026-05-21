package citationlookup

import "testing"

func TestNormalizePatentID(t *testing.T) {
	tests := map[string]string{
		"8164048":       "US8164048",
		"us-8164048-b2": "US8164048B2",
		" US8164048B2 ": "US8164048B2",
		"EP1234567A1":   "EP1234567A1",
	}
	for input, want := range tests {
		if got := NormalizePatentID(input); string(got) != want {
			t.Fatalf("NormalizePatentID(%q) = %q, want %q", input, got, want)
		}
	}
}
