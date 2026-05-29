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

// enterInput presses Enter and returns the PromptSubmitMsg input, or "" when
// Enter did not submit (e.g. autocomplete prefill).
func enterInput(t *testing.T, p *Prompt) string {
	t.Helper()
	_, cmd, handled := p.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("Enter not handled")
	}
	if cmd == nil {
		return ""
	}
	msg := cmd()
	sub, ok := msg.(PromptSubmitMsg)
	if !ok {
		return ""
	}
	return sub.Input
}

func TestPromptDirectEnterExecutesArgFreeSelection(t *testing.T) {
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	p := NewPrompt(reg, keymap.Default(), render.NewTheme(), text.English(), command.ScopeCatalog, PromptDirect)
	p.query = "metrics"
	p.filter()
	if got := enterInput(t, p); got != "metrics" {
		t.Fatalf("Enter on arg-free selection submitted %q, want metrics", got)
	}
}

func TestPromptDirectEnterAutocompletesArgCommand(t *testing.T) {
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	p := NewPrompt(reg, keymap.Default(), render.NewTheme(), text.English(), command.ScopeCatalog, PromptDirect)
	// Partial name selects the arg-taking "browse" command.
	p.query = "brow"
	p.filter()
	if got := enterInput(t, p); got != "" {
		t.Fatalf("first Enter on arg command should autocomplete, not submit; got %q", got)
	}
	if p.query != "browse " {
		t.Fatalf("autocomplete query = %q, want %q", p.query, "browse ")
	}
	// Second Enter (name filled, no args) runs it bare.
	if got := enterInput(t, p); got != "browse" {
		t.Fatalf("second Enter submitted %q, want browse", got)
	}
}

func TestPromptDirectEnterRunsTypedArgs(t *testing.T) {
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	p := NewPrompt(reg, keymap.Default(), render.NewTheme(), text.English(), command.ScopeCatalog, PromptDirect)
	p.query = "browse US123"
	p.filter()
	if got := enterInput(t, p); got != "browse US123" {
		t.Fatalf("Enter with typed args submitted %q, want %q", got, "browse US123")
	}
}

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
