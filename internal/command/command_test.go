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
		Command{ID: NavDown, Kind: KindView},
		Command{ID: NavDown, Kind: KindView},
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

func TestRegistryRejectsDuplicateTypedName(t *testing.T) {
	_, err := NewRegistry(
		Command{ID: "x", Kind: KindView, Name: "open.projects"},
		Command{ID: "y", Kind: KindView, Name: "open.projects"},
	)
	if err == nil {
		t.Fatal("NewRegistry should reject a duplicate typed command name")
	}
}

func TestLookupName(t *testing.T) {
	reg, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	got, ok := reg.LookupName("project.use")
	if !ok || got.ID != ProjectActivate {
		t.Fatalf("LookupName(project.use) = %+v ok=%v, want ProjectActivate", got, ok)
	}
	got, ok = reg.LookupName("use-project")
	if !ok || got.ID != ProjectActivate {
		t.Fatalf("LookupName(use-project) = %+v ok=%v, want ProjectActivate", got, ok)
	}
	got, ok = reg.LookupName("filter.clear")
	if !ok || got.ID != Filter {
		t.Fatalf("LookupName(filter.clear) = %+v ok=%v, want Filter", got, ok)
	}
}

func TestRegistryRejectsAliasCollision(t *testing.T) {
	_, err := NewRegistry(
		Command{ID: "x", Kind: KindView, Name: "project.use"},
		Command{ID: "y", Kind: KindView, Name: "open.projects", Aliases: []string{"project.use"}},
	)
	if err == nil {
		t.Fatal("NewRegistry should reject a typed alias colliding with another name")
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

func TestInScopeScoping(t *testing.T) {
	reg, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	// Quit is global, so it appears in every scope.
	for _, scope := range []Scope{ScopeCatalog, ScopeDetail, ScopeOverlay} {
		found := false
		for _, c := range reg.InScope(scope) {
			if c.ID == Quit {
				found = true
			}
		}
		if !found {
			t.Errorf("global command Quit missing from scope %q", scope)
		}
	}
	// ProjectCreate is scoped to the projects scope only, while ProjectActivate
	// remains typed-command accessible in other scopes so project.use <id> works
	// everywhere.
	for _, c := range reg.InScope(ScopeCatalog) {
		if c.ID == ProjectCreate {
			t.Error("ProjectCreate should not be offered in the catalog scope")
		}
	}
	inProjects := false
	activateInProjects := false
	activateInCatalog := false
	for _, c := range reg.InScope(ScopeCatalog) {
		if c.ID == ProjectActivate {
			activateInCatalog = true
		}
	}
	for _, c := range reg.InScope(ScopeProjects) {
		if c.ID == ProjectCreate {
			inProjects = true
		}
		if c.ID == ProjectActivate {
			activateInProjects = true
		}
	}
	if !inProjects {
		t.Error("ProjectCreate should be offered in the projects scope")
	}
	if !activateInProjects {
		t.Error("ProjectActivate should be offered in the projects scope")
	}
	if !activateInCatalog {
		t.Error("ProjectActivate should be offered in the catalog scope")
	}
}

func TestTypedCommandsInScope(t *testing.T) {
	reg, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	for _, c := range reg.TypedInScope(ScopeCatalog) {
		if c.Name == "" {
			t.Fatalf("TypedInScope returned command %q with empty Name", c.ID)
		}
	}
}
