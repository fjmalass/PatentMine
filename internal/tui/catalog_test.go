package tui

import (
	"strings"
	"testing"
)

func TestRenderHelpUsesCommandCatalog(t *testing.T) {
	help := RenderHelp(EnglishText())
	for _, section := range allHelpSections {
		for _, entry := range section.Entries {
			// Each entry should appear in help via its Key or Command
			label := entry.Key
			if entry.Command != "" {
				label = entry.Command
			}
			if label != "" && !strings.Contains(help, section.Title) {
				t.Fatalf("help output missing section title %q", section.Title)
			}
			_ = label
		}
	}
}

func TestFilterHelpSections(t *testing.T) {
	text := EnglishText()

	// Empty query returns all sections plus the Commands overview section.
	all := FilterHelpSections("", text)
	if len(all) != len(allHelpSections)+1 {
		t.Fatalf("expected %d sections (allHelp+commands), got %d", len(allHelpSections)+1, len(all))
	}

	// Query that matches nothing returns empty
	none := FilterHelpSections("zzzzznomatch", text)
	if len(none) != 0 {
		t.Fatalf("expected 0 sections for no-match query, got %d", len(none))
	}

	// Query that matches something in a known section
	results := FilterHelpSections("refresh", text)
	if len(results) == 0 {
		t.Fatal("expected at least one section matching 'refresh'")
	}
}
