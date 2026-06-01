package domain

import "testing"

func TestNormalizeAssignee(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Acme Corp", "ACME"},
		{"Acme Corporation, Inc.", "ACME"},
		{"ACME CORP.", "ACME"},
		{"The Widget Company", "WIDGET"},
		{"Globex LLC", "GLOBEX"},
		{"Initech Inc", "INITECH"},
		{"  Stark   Industries  ", "STARK INDUSTRIES"},
		{"", ""},
		{"Co", "CO"}, // a single bare suffix token is kept (never empty out a name)
	}
	for _, c := range cases {
		if got := NormalizeAssignee(c.in); got != c.want {
			t.Errorf("NormalizeAssignee(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
