package tui

import "testing"

// TestCitationKeysFromListAnchorCurrentPatent guards the bug where c / b from
// the list view opened the citations view against an empty m.current. Like
// L / F / t, the citation keys must anchor m.current to the highlighted row.
func TestCitationKeysFromListAnchorCurrentPatent(t *testing.T) {
	cases := []struct {
		name string
		key  string
		mode viewMode
	}{
		{"cites", keyCites, viewCites},
		{"cited-by", keyCitedBy, viewCitedBy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _ := newSeededDeleteModel(t, 5)
			m.patentSelected = 2
			want := m.patents[2].Number

			updated, _ := m.Update(teaKey(tc.key))
			got := updated.(*Model)
			if got.mode != tc.mode {
				t.Fatalf("%s did not open %q, got %q", tc.key, tc.mode, got.mode)
			}
			if got.current.Number != want {
				t.Fatalf("%s opened view for %q, want highlighted row %q",
					tc.key, got.current.Number, want)
			}
		})
	}
}
