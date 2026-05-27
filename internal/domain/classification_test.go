package domain

import (
	"testing"
)

func TestParseClassification(t *testing.T) {
	tests := []struct {
		rawCode  string
		expected Classification
	}{
		{
			rawCode: "G06F 17/30",
			expected: Classification{
				System:    "CPC",
				Code:      "G06F 17/30",
				Section:   "G",
				Class:     "06",
				Subclass:  "F",
				MainGroup: "17",
				Subgroup:  "30",
			},
		},
		{
			rawCode: "A01B",
			expected: Classification{
				System:   "CPC",
				Code:     "A01B",
				Section:  "A",
				Class:    "01",
				Subclass: "B",
			},
		},
		{
			rawCode: "707/E17.014",
			expected: Classification{
				System:   "USPC",
				Code:     "707/E17.014",
				Class:    "707",
				Subclass: "E17.014",
			},
		},
		{
			rawCode:  "",
			expected: Classification{},
		},
		{
			rawCode: "E04B1/6108",
			expected: Classification{
				System:    "CPC",
				Code:      "E04B1/6108",
				Section:   "E",
				Class:     "04",
				Subclass:  "B",
				MainGroup: "1",
				Subgroup:  "6108",
			},
		},
		{
			rawCode: "Y10T403/00",
			expected: Classification{
				System:    "CPC",
				Code:      "Y10T403/00",
				Section:   "Y",
				Class:     "10",
				Subclass:  "T",
				MainGroup: "403",
				Subgroup:  "00",
			},
		},
		{
			rawCode: "H04L",
			expected: Classification{
				System:   "CPC",
				Code:     "H04L",
				Section:  "H",
				Class:    "04",
				Subclass: "L",
			},
		},
		{
			rawCode: "STCB",
			expected: Classification{
				System: "Other",
				Code:   "STCB",
			},
		},
		{
			rawCode: "STCF",
			expected: Classification{
				System: "Other",
				Code:   "STCF",
			},
		},
		{
			rawCode: "g06f 17/30",
			expected: Classification{
				System:    "CPC",
				Code:      "G06F 17/30",
				Section:   "G",
				Class:     "06",
				Subclass:  "F",
				MainGroup: "17",
				Subgroup:  "30",
			},
		},
	}

	for _, tc := range tests {
		got := ParseClassification(tc.rawCode)
		if got.System != tc.expected.System {
			t.Errorf("ParseClassification(%q) System = %q, want %q", tc.rawCode, got.System, tc.expected.System)
		}
		if got.Code != tc.expected.Code {
			t.Errorf("ParseClassification(%q) Code = %q, want %q", tc.rawCode, got.Code, tc.expected.Code)
		}
		if got.Section != tc.expected.Section {
			t.Errorf("ParseClassification(%q) Section = %q, want %q", tc.rawCode, got.Section, tc.expected.Section)
		}
		if got.Class != tc.expected.Class {
			t.Errorf("ParseClassification(%q) Class = %q, want %q", tc.rawCode, got.Class, tc.expected.Class)
		}
		if got.Subclass != tc.expected.Subclass {
			t.Errorf("ParseClassification(%q) Subclass = %q, want %q", tc.rawCode, got.Subclass, tc.expected.Subclass)
		}
		if got.MainGroup != tc.expected.MainGroup {
			t.Errorf("ParseClassification(%q) MainGroup = %q, want %q", tc.rawCode, got.MainGroup, tc.expected.MainGroup)
		}
		if got.Subgroup != tc.expected.Subgroup {
			t.Errorf("ParseClassification(%q) Subgroup = %q, want %q", tc.rawCode, got.Subgroup, tc.expected.Subgroup)
		}
	}
}
