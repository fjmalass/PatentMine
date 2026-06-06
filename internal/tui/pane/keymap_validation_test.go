package pane

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/keymap"
	"patentmine/internal/tui/render"
)

func TestKeymapInterceptionValidation(t *testing.T) {
	// Create default keymaps to inspect the layer bindings
	keymaps := keymap.Default()
	layers := keymaps.ScopeLayers()

	// Helper to parse key string into tea.KeyMsg
	parseKeyMsg := func(key string) tea.KeyMsg {
		key = strings.TrimSpace(key)
		if len(key) == 0 {
			return tea.KeyMsg{}
		}
		if parts := strings.Split(key, " "); len(parts) > 1 {
			key = parts[0]
		}
		switch key {
		case "up":
			return tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			return tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			return tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			return tea.KeyMsg{Type: tea.KeyRight}
		case "enter":
			return tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			return tea.KeyMsg{Type: tea.KeyEsc}
		case "backspace":
			return tea.KeyMsg{Type: tea.KeyBackspace}
		case "tab":
			return tea.KeyMsg{Type: tea.KeyTab}
		default:
			if strings.HasPrefix(key, "ctrl+") && len(key) == 6 {
				r := rune(key[5])
				return tea.KeyMsg{Type: tea.KeyCtrlA + tea.KeyType(r-'a')}
			}
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		}
	}

	theme := render.NewTheme()

	// Define list of panes and their respective scopes to test
	tests := []struct {
		name  string
		scope command.Scope
		pane  Pane
	}{
		{
			name:  "OfficeActions",
			scope: command.ScopeMatterOA,
			pane:  NewOfficeActions(nil, theme, "p1"),
		},
		{
			name:  "Orphans",
			scope: command.ScopeOrphans,
			pane:  NewOrphans(nil, theme),
		},
		{
			name:  "AllNotes",
			scope: command.ScopeNotes,
			pane:  NewAllNotes(nil, theme, &domain.Project{ID: "p1", Name: "Proj1"}),
		},
		{
			name:  "Catalog",
			scope: command.ScopeCatalog,
			pane:  NewCatalog(nil, theme),
		},
		{
			name:  "Citations",
			scope: command.ScopeCitations,
			pane:  NewCitations(nil, theme, domain.PatentNumber{}, domain.RelationCites),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			layer, exists := layers[tc.scope]
			if !exists || layer == nil {
				t.Logf("no keymap layer found for scope %q, skipping validation", tc.scope)
				return
			}

			// Iterate over all bound keys in the keymap layer
			for key := range layer.Bindings() {
				// We skip esc and slash (/) because they are often dynamically intercepted
				// to close overlays / close search / start search, which is expected.
				if key == "esc" || key == "/" {
					continue
				}

				msg := parseKeyMsg(key)
				if msg.Type == tea.KeyRunes && len(msg.Runes) == 0 {
					continue
				}

				// Call HandleKey if the pane implements KeyHandler
				if kh, ok := tc.pane.(KeyHandler); ok {
					_, _, handled := kh.HandleKey(msg)
					if handled {
						t.Errorf("Pane %q scope %q intercepted key %q in HandleKey, preventing it from being routed to the keymap command %v",
							tc.name, tc.scope, key, layer.Bindings()[key])
					}
				}
			}
		})
	}
}
