package tui

import (
	"testing"

	"patentmine/internal/command"
	"patentmine/internal/text"
	"patentmine/internal/tui/keymap"
	"patentmine/internal/tui/overlay"
)

// TestValidateWiringAcceptsDefaults proves the shipped keymap, registry, and
// catalog are mutually consistent — the wiring check is not vacuously true.
func TestValidateWiringAcceptsDefaults(t *testing.T) {
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	if err := validateWiring(reg, keymap.Default(), text.English()); err != nil {
		t.Fatalf("default wiring should validate: %v", err)
	}
}

// TestValidateWiringRejectsDeadBinding is the core guarantee: a key bound to a
// command that no handler services fails the boot rather than becoming a silent
// no-op at runtime.
func TestValidateWiringRejectsDeadBinding(t *testing.T) {
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	km := keymap.Default()
	// PatentList is an engine read with no key handler in any pane or the App.
	km.Scope(command.ScopeCatalog).Bind("Z", command.PatentList)
	if _, err := New(nil, reg, km, text.English()); err == nil {
		t.Fatal("New should reject a key bound to an unhandled command")
	}
}

// TestValidateWiringRejectsMissingCatalogText proves a command without display
// strings fails the boot.
func TestValidateWiringRejectsMissingCatalogText(t *testing.T) {
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	if err := validateWiring(reg, keymap.Default(), text.New("en", nil)); err == nil {
		t.Fatal("validateWiring should reject a catalog missing command strings")
	}
}

// TestProjectCreateKeyOpensNameOverlay covers the reported bug: the 'n' key now
// reaches a handler that opens the name-entry overlay.
func TestProjectCreateKeyOpensNameOverlay(t *testing.T) {
	app := newTestApp(t)
	app.Update(runeKey('n'))
	if len(app.overlays) != 1 {
		t.Fatalf("'n' should open one overlay, got %d", len(app.overlays))
	}
	if _, ok := app.focusedOverlay().(*overlay.TextInput); !ok {
		t.Fatalf("'n' should open a TextInput overlay, got %T", app.focusedOverlay())
	}
}

// TestProjectCreateTypedCommandOpensNameOverlay covers the other half of the
// bug: ':project.create' is no longer a dead typed command.
func TestProjectCreateTypedCommandOpensNameOverlay(t *testing.T) {
	app := newTestApp(t)
	app.executeTypedCommand("project.create")
	if len(app.overlays) != 1 {
		t.Fatalf("':project.create' should open one overlay, got %d", len(app.overlays))
	}
	if _, ok := app.focusedOverlay().(*overlay.TextInput); !ok {
		t.Fatalf("':project.create' should open a TextInput overlay, got %T", app.focusedOverlay())
	}
}

// TestTextSubmitEmptyNameReportsError checks the name overlay rejects a blank
// project name with a visible status error.
func TestTextSubmitEmptyNameReportsError(t *testing.T) {
	app := newTestApp(t)
	app.handleTextSubmit(overlay.TextSubmitMsg{Purpose: overlay.PurposeCreateProject, Value: "   "})
	if !app.statusErr {
		t.Fatal("submitting a blank project name should report an error")
	}
}
