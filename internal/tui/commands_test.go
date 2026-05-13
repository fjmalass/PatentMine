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
		{":version", "version", nil},

		// tag command aliases
		{":ta mytag", "tag", []string{"add", "mytag"}},
		{":ta mytag 118", "tag", []string{"add", "mytag", "118"}},
		{":tl", "tag", []string{"list"}},
		{":td mytag", "tag", []string{"delete", "mytag"}},
		{":tr old new", "tag", []string{"rename", "old", "new"}},
		{":tc mytag 208", "tag", []string{"color", "mytag", "208"}},
		{":tf mytag", "tag", []string{"filter", "mytag"}},
		{":tf", "tag", []string{"filter"}},
		{":th", "tag", []string{"help"}},

		// full :tag subcommands
		{":tag add mytag", "tag", []string{"add", "mytag"}},
		{":tag list", "tag", []string{"list"}},
		{":tag delete mytag", "tag", []string{"delete", "mytag"}},
		{":tag rename old new", "tag", []string{"rename", "old", "new"}},
		{":tag color mytag 208", "tag", []string{"color", "mytag", "208"}},
		{":tag filter mytag", "tag", []string{"filter", "mytag"}},
		{":tag help", "tag", []string{"help"}},
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
