package overlay

import (
	"testing"

	"patentmine/internal/command"
	"patentmine/internal/tui/keymap"
	"patentmine/internal/tui/render"
)

func TestPromptRanksExactAndPrefixMatchesFirst(t *testing.T) {
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	prompt := NewPrompt(reg, keymap.Default(), render.NewTheme(), command.ContextCatalog, PromptPalette)
	prompt.query = "proj"
	prompt.filter()
	if len(prompt.shown) == 0 {
		t.Fatal("expected ranked prompt results")
	}
	if got := prompt.shown[0].command.Name; got != "open.projects" {
		t.Fatalf("top result = %q, want open.projects", got)
	}
}

func TestPromptMatchesAliasesAndShortcuts(t *testing.T) {
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	prompt := NewPrompt(reg, keymap.Default(), render.NewTheme(), command.ContextProjects, PromptPalette)
	prompt.query = "use-project"
	prompt.filter()
	if len(prompt.shown) == 0 || prompt.shown[0].command.ID != command.ProjectActivate {
		t.Fatalf("alias search top result = %+v, want ProjectActivate", prompt.shown)
	}
	prompt.query = "enter"
	prompt.filter()
	if len(prompt.shown) == 0 || prompt.shown[0].command.ID != command.ProjectActivate {
		t.Fatalf("shortcut search top result = %+v, want ProjectActivate", prompt.shown)
	}
}
