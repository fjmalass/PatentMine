package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/text"
	"patentmine/internal/tui/keymap"
	"patentmine/internal/tui/render"
)

func TestPromptRanksExactAndPrefixMatchesFirst(t *testing.T) {
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	prompt := NewPrompt(reg, keymap.Default(), render.NewTheme(), text.English(), command.ScopeCatalog, PromptPalette)
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
	prompt := NewPrompt(reg, keymap.Default(), render.NewTheme(), text.English(), command.ScopeProjects, PromptPalette)
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

func TestPromptDirectFooterStaysSingleLine(t *testing.T) {
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	prompt := NewPrompt(reg, keymap.Default(), render.NewTheme(), text.English(), command.ScopeCatalog, PromptDirect)
	prompt.query = "tag"
	prompt.filter()
	footer := prompt.footerLine(40)
	if got := len(strings.Split(footer, "\n")); got != 1 {
		t.Fatalf("footer rendered %d lines, want 1: %q", got, footer)
	}
	view := prompt.View(40, 12)
	last := view[strings.LastIndex(view, "\n")+1:]
	if got := len(strings.Split(last, "\n")); got != 1 {
		t.Fatalf("prompt footer wrapped in full view: %q", view)
	}
}

func TestPromptDirectListsBrowseVariants(t *testing.T) {
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	prompt := NewPrompt(reg, keymap.Default(), render.NewTheme(), text.English(), command.ScopeCatalog, PromptDirect)
	prompt.query = "browse.uspto"
	prompt.filter()

	want := map[string]bool{
		"browse.uspto":       false,
		"browse.uspto.grant": false,
		"browse.uspto.pgpub": false,
	}
	for _, entry := range prompt.shown {
		if _, ok := want[entry.command.Name]; ok {
			want[entry.command.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("direct prompt missing %s in shown commands: %+v", name, prompt.shown)
		}
	}

	prompt.query = "browse.google"
	prompt.filter()
	if len(prompt.shown) == 0 || prompt.shown[0].command.Name != "browse.google" {
		t.Fatalf("browse.google top result = %+v", prompt.shown)
	}
}

func TestPromptTabExpansion(t *testing.T) {
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	prompt := NewPrompt(reg, keymap.Default(), render.NewTheme(), text.English(), command.ScopeCatalog, PromptDirect)
	prompt.query = "open.proj"
	prompt.filter()

	// Simulate pressing Tab key
	updated, _, handled := prompt.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	if !handled {
		t.Fatal("expected Tab key to be handled")
	}
	p := updated.(*Prompt)
	if got := p.query; got != "open.projects" {
		t.Fatalf("expected query to expand to open.projects, got %q", got)
	}
	if got := p.cursor; got != len("open.projects") {
		t.Fatalf("expected cursor at end (%d), got %d", len("open.projects"), got)
	}
}
