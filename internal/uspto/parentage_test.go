package uspto

import "testing"

func TestParentageLabel(t *testing.T) {
	cases := []struct {
		name string
		code string
		desc string
		want string
	}{
		{"known code wins over desc", "CON", "whatever", "Continuation of"},
		{"lower-case code normalizes", "cip", "", "Continuation-in-part of"},
		{"code with spaces", "  DIV ", "", "Division of"},
		{"provisional code variant", "PRO", "", "Provisional for"},
		{"unknown code falls back to desc", "ZZZ", "Some relation of", "Some relation of"},
		{"unknown code, no desc, returns code", "ZZZ", "", "ZZZ"},
		{"empty everything returns dash", "", "", "—"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParentageLabel(tc.code, tc.desc); got != tc.want {
				t.Errorf("ParentageLabel(%q,%q) = %q, want %q", tc.code, tc.desc, got, tc.want)
			}
		})
	}
}

func TestParentageCategory(t *testing.T) {
	cases := []struct {
		code string
		desc string
		want string
	}{
		{"CON", "", ParentageContinuation},
		{"CIP", "", ParentageContinuationInPart},
		{"DIV", "", ParentageDivision},
		{"PRO", "", ParentageProvisional},
		{"PROV", "", ParentageProvisional},
		{"", "Provisional application", ParentageProvisional},
		{"371", "", ParentageNationalStage},
		{"NST", "", ParentageNationalStage},
		{"REI", "", ParentageReissue},
		{"REISSUE", "", ParentageReissue},
		{"SUB", "", ParentageOther},
		{"WAT", "", ParentageOther},
	}
	for _, tc := range cases {
		if got := ParentageCategory(tc.code, tc.desc); got != tc.want {
			t.Errorf("ParentageCategory(%q,%q) = %q, want %q", tc.code, tc.desc, got, tc.want)
		}
	}
}
