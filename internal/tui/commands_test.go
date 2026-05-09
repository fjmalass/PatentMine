package tui

import "testing"

func TestParseCommand(t *testing.T) {
	tests := []struct {
		input string
		name  string
		args  []string
	}{
		{"/machine learning", "search", []string{"machine learning"}},
		{":add US11611785B2", "add", []string{"US11611785B2"}},
		{":open US11611785B2", "open", []string{"US11611785B2"}},
		{":import https://patents.google.com/patent/US11611785B2/en?oq=US11611785B2+", "import", []string{"https://patents.google.com/patent/US11611785B2/en?oq=US11611785B2+"}},
		{":refresh citedby", "refresh", []string{"citedby"}},
		{":refresh-refs-details", "refresh-refs-details", nil},
		{":ref export", "ref", []string{"export"}},
		{":help", "help", nil},
	}
	for _, tt := range tests {
		got := ParseCommand(tt.input)
		if got.Name != tt.name {
			t.Fatalf("%q: expected %q, got %q", tt.input, tt.name, got.Name)
		}
		if len(got.Args) != len(tt.args) {
			t.Fatalf("%q: expected args %v, got %v", tt.input, tt.args, got.Args)
		}
		for i := range tt.args {
			if got.Args[i] != tt.args[i] {
				t.Fatalf("%q: expected args %v, got %v", tt.input, tt.args, got.Args)
			}
		}
	}
}
