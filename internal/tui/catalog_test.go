package tui

import (
	"strings"
	"testing"
)

func TestRenderHelpUsesCommandCatalog(t *testing.T) {
	help := RenderHelp(EnglishText())
	for _, section := range commandHelpSections {
		for _, entry := range section.Entries {
			if !strings.Contains(help, entry.Usage) {
				t.Fatalf("help output missing command usage %q (section %q)", entry.Usage, section.Title)
			}
		}
	}
	for _, entry := range shortcutHelp {
		if !strings.Contains(help, entry.Usage) {
			t.Fatalf("help output missing shortcut usage %q", entry.Usage)
		}
	}
}
