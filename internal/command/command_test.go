package command

import (
	"testing"

	"patentmine/internal/proto"
)

func TestDefaultRegistryBuilds(t *testing.T) {
	reg, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if reg.Len() == 0 {
		t.Fatal("default registry is empty")
	}
	// Every engine command must name a proto method; every view command must not.
	for _, c := range reg.All() {
		switch c.Kind {
		case KindEngine:
			if c.Method == "" {
				t.Errorf("engine command %q has no method", c.ID)
			}
		case KindView:
			if c.Method != "" {
				t.Errorf("view command %q sets a method", c.ID)
			}
		default:
			t.Errorf("command %q has unknown kind %q", c.ID, c.Kind)
		}
	}
}

func TestRegistryRejectsDuplicateID(t *testing.T) {
	_, err := NewRegistry(
		Command{ID: NavDown, Title: "a", Kind: KindView},
		Command{ID: NavDown, Title: "b", Kind: KindView},
	)
	if err == nil {
		t.Fatal("NewRegistry should reject a duplicate ID")
	}
}

func TestRegistryRejectsEngineCommandWithoutMethod(t *testing.T) {
	_, err := NewRegistry(Command{ID: "x", Kind: KindEngine})
	if err == nil {
		t.Fatal("NewRegistry should reject an engine command without a method")
	}
}

func TestRegistryRejectsViewCommandWithMethod(t *testing.T) {
	_, err := NewRegistry(Command{ID: "x", Kind: KindView, Method: proto.MethodPing})
	if err == nil {
		t.Fatal("NewRegistry should reject a view command that sets a method")
	}
}

func TestLookup(t *testing.T) {
	reg, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	got, ok := reg.Lookup(IngestFamily)
	if !ok {
		t.Fatal("IngestFamily not found in default registry")
	}
	if got.Kind != KindEngine || got.Method != proto.MethodIngestFamily {
		t.Fatalf("IngestFamily = %+v, want engine kind with ingest.family method", got)
	}
	if _, ok := reg.Lookup("does.not.exist"); ok {
		t.Fatal("Lookup of an unknown ID should fail")
	}
}

func TestInContextScoping(t *testing.T) {
	reg, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	// Quit is global, so it appears in every context.
	for _, ctx := range []Context{ContextCatalog, ContextDetail, ContextOverlay} {
		found := false
		for _, c := range reg.InContext(ctx) {
			if c.ID == Quit {
				found = true
			}
		}
		if !found {
			t.Errorf("global command Quit missing from context %q", ctx)
		}
	}
	// ProjectCreate is scoped to the projects context only.
	for _, c := range reg.InContext(ContextCatalog) {
		if c.ID == ProjectCreate {
			t.Error("ProjectCreate should not be offered in the catalog context")
		}
	}
	inProjects := false
	for _, c := range reg.InContext(ContextProjects) {
		if c.ID == ProjectCreate {
			inProjects = true
		}
	}
	if !inProjects {
		t.Error("ProjectCreate should be offered in the projects context")
	}
}
