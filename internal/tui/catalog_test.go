package tui

import (
	"strings"
	"testing"
)

func TestRenderHelpUsesCommandCatalog(t *testing.T) {
	help := RenderHelp(EnglishText())
	for _, entry := range commandHelpEntries {
		if !strings.Contains(help, entry.Usage) {
			t.Fatalf("help output missing command usage %q", entry.Usage)
		}
	}
	for _, entry := range shortcutHelp {
		if !strings.Contains(help, entry.Usage) {
			t.Fatalf("help output missing shortcut usage %q", entry.Usage)
		}
	}
}
