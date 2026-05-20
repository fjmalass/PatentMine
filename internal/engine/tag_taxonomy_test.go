package engine_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"patentmine/internal/api"
	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/store/sqlite"
)

func TestTagNameValidation(t *testing.T) {
	tests := []struct {
		name    string
		isValid bool
	}{
		{"prior_art", true},
		{"tag123", true},
		{"tag_123_abc", true},
		{"prior-art", false},
		{"Prior_art", false},
		{"prior_art!", false},
		{"prior art", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := domain.ValidateTagName(tc.name)
			if tc.isValid && err != nil {
				t.Fatalf("expected name %q to be valid, got error: %v", tc.name, err)
			}
			if !tc.isValid && err == nil {
				t.Fatalf("expected name %q to be invalid, but got no error", tc.name)
			}
		})
	}
}

func TestTagTaxonomyEngineDecoupled(t *testing.T) {
	// Create fresh sqlite store
	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "taxonomy.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	eng := engine.New(ctx, repo, nil)
	t.Cleanup(func() {
		eng.Close()
		cancel()
		_ = repo.Close()
	})

	project, err := eng.CreateProject(ctx, "Taxonomy Test")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	patent := domain.Patent{
		Number:     domain.MustParsePatentNumber("US11611785B2"),
		FetchState: domain.FetchCached,
	}
	if err := eng.SavePatent(ctx, patent); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	if _, err := eng.AddToProject(ctx, project.ID, patent.Number); err != nil {
		t.Fatalf("AddToProject: %v", err)
	}

	// 1. Assigning a tag that does not exist in taxonomy should fail
	err = eng.AssignPatentTag(ctx, project.ID, patent.Number, "prior_art")
	if err == nil {
		t.Fatal("expected AssignPatentTag to fail for non-existent taxonomy tag")
	}
	if !strings.Contains(err.Error(), "does not exist in the project taxonomy") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 2. Create the tag in taxonomy
	tag, err := eng.CreateTaxonomyTag(ctx, project.ID, "prior_art")
	if err != nil {
		t.Fatalf("CreateTaxonomyTag: %v", err)
	}
	if tag.Name != "prior_art" {
		t.Fatalf("expected tag name prior_art, got %q", tag.Name)
	}

	// 3. Assigning should now succeed
	err = eng.AssignPatentTag(ctx, project.ID, patent.Number, "prior_art")
	if err != nil {
		t.Fatalf("expected AssignPatentTag to succeed, got: %v", err)
	}

	// 4. Listing patent tags should show the assigned tag and non-zero AssignedAt
	tags, err := eng.PatentTags(ctx, project.ID, patent.Number)
	if err != nil {
		t.Fatalf("PatentTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "prior_art" {
		t.Fatalf("unexpected patent tags: %+v", tags)
	}
	if tags[0].AssignedAt.IsZero() {
		t.Fatal("expected AssignedAt timestamp to be populated, got zero time")
	}
	if time.Since(tags[0].AssignedAt) > 10*time.Second {
		t.Fatalf("expected AssignedAt to be close to now, got: %v", tags[0].AssignedAt)
	}

	// 5. Deleting the tag from taxonomy should cascade-delete patent tag assignment
	err = eng.DeleteTaxonomyTag(ctx, project.ID, "prior_art")
	if err != nil {
		t.Fatalf("DeleteTaxonomyTag: %v", err)
	}

	// Check taxonomy list is empty
	taxTags, err := eng.ListTaxonomyTags(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListTaxonomyTags: %v", err)
	}
	if len(taxTags) != 0 {
		t.Fatalf("expected taxonomy to be empty, got: %+v", taxTags)
	}

	// Check patent tags are empty (cascade deleted)
	tags, err = eng.PatentTags(ctx, project.ID, patent.Number)
	if err != nil {
		t.Fatalf("PatentTags: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected patent tags to be cascade deleted, got: %+v", tags)
	}
}

func TestTagTaxonomyActivityLoggingAndReplay(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "logs")
	obs, err := observability.Open(logsDir, "test", "test-version")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = obs.Close() })

	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "activity.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	eng := engine.New(ctx, repo, nil, engine.WithActivityRecorder(obs.Activity), engine.WithLogger(obs.Logger))
	t.Cleanup(eng.Close)

	project, err := eng.CreateProject(ctx, "Activity Project")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	patent := domain.Patent{
		Number:     domain.MustParsePatentNumber("US11611785B2"),
		FetchState: domain.FetchCached,
	}
	if err := eng.SavePatent(ctx, patent); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	if _, err := eng.AddToProject(ctx, project.ID, patent.Number); err != nil {
		t.Fatalf("AddToProject: %v", err)
	}

	// Make changes that generate activity records
	_, _ = eng.CreateTaxonomyTag(ctx, project.ID, "prior_art")
	_ = eng.AssignPatentTag(ctx, project.ID, patent.Number, "prior_art")
	_ = eng.RemovePatentTag(ctx, project.ID, patent.Number, "prior_art")
	_ = eng.DeleteTaxonomyTag(ctx, project.ID, "prior_art")

	date := time.Now().In(time.Local).Format("2006-01-02")
	body, err := os.ReadFile(filepath.Join(logsDir, "activity-"+date+".jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	requiredActions := []string{
		`"action":"tag.create"`,
		`"action":"tag.delete"`,
		`"action":"patent.tag_assign"`,
		`"action":"patent.tag_remove"`,
	}
	for _, act := range requiredActions {
		if !strings.Contains(string(body), act) {
			t.Errorf("expected activity log to record action %q, but it was missing in: %s", act, body)
		}
	}
}

func TestTagTaxonomyRESTAPI(t *testing.T) {
	// Setup same simple client & api server wrapper
	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "rest.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	eng := engine.New(ctx, repo, nil)

	// Since we are not dialing a real socket in this HTTP-only test, we can mock the client
	// or use a real in-memory connection/server to dial. Let's do it cleanly via a real listener.
	socket := filepath.Join(t.TempDir(), "test_rest.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	go func() { _ = rpc.NewServer(eng).Serve(ctx, ln) }()

	client, err := rpc.Dial(socket)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	registry, err := command.Default()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	apiServer := api.NewServer(client, registry)
	handler := apiServer.Handler()

	t.Cleanup(func() {
		_ = client.Close()
		cancel()
		eng.Close()
		_ = repo.Close()
	})

	// Seed database: Create project
	project, err := eng.CreateProject(ctx, "REST Test")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	patent := domain.Patent{
		Number:     domain.MustParsePatentNumber("US11611785B2"),
		FetchState: domain.FetchCached,
	}
	if err := eng.SavePatent(ctx, patent); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	if _, err := eng.AddToProject(ctx, project.ID, patent.Number); err != nil {
		t.Fatalf("AddToProject: %v", err)
	}

	// 1. Create taxonomy tag via HTTP POST
	w := doRequest(t, handler, http.MethodPost, "/projects/"+string(project.ID)+"/tags", `{"name":"prior_art"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /projects/{id}/tags = %d: %s", w.Code, w.Body.String())
	}
	var tag domain.Tag
	if err := json.Unmarshal(w.Body.Bytes(), &tag); err != nil {
		t.Fatalf("decode tag: %v", err)
	}
	if tag.Name != "prior_art" {
		t.Fatalf("expected prior_art, got %q", tag.Name)
	}

	// 2. List taxonomy tags via HTTP GET
	w = doRequest(t, handler, http.MethodGet, "/projects/"+string(project.ID)+"/tags", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /projects/{id}/tags = %d: %s", w.Code, w.Body.String())
	}
	var taxList proto.TagListResult
	if err := json.Unmarshal(w.Body.Bytes(), &taxList); err != nil {
		t.Fatalf("decode taxonomy tags: %v", err)
	}
	if len(taxList.Tags) != 1 || taxList.Tags[0].Name != "prior_art" {
		t.Fatalf("unexpected taxonomy list: %+v", taxList)
	}

	// 3. Assign tag to patent via HTTP POST
	w = doRequest(t, handler, http.MethodPost, "/projects/"+string(project.ID)+"/patents/US11611785B2/tags", `{"name":"prior_art"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST assign patent tag = %d: %s", w.Code, w.Body.String())
	}

	// 4. List patent tags via HTTP GET
	w = doRequest(t, handler, http.MethodGet, "/projects/"+string(project.ID)+"/patents/US11611785B2/tags", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET patent tags = %d: %s", w.Code, w.Body.String())
	}
	var patTags proto.PatentTagListResult
	if err := json.Unmarshal(w.Body.Bytes(), &patTags); err != nil {
		t.Fatalf("decode patent tags: %v", err)
	}
	if len(patTags.Tags) != 1 || patTags.Tags[0].Name != "prior_art" {
		t.Fatalf("unexpected patent tags: %+v", patTags)
	}
	if patTags.Tags[0].AssignedAt.IsZero() {
		t.Fatal("expected AssignedAt to be non-zero in REST response")
	}

	// 5. Remove tag from patent via HTTP DELETE
	w = doRequest(t, handler, http.MethodDelete, "/projects/"+string(project.ID)+"/patents/US11611785B2/tags/prior_art", "")
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE patent tag = %d: %s", w.Code, w.Body.String())
	}

	// Verify it's removed
	w = doRequest(t, handler, http.MethodGet, "/projects/"+string(project.ID)+"/patents/US11611785B2/tags", "")
	json.Unmarshal(w.Body.Bytes(), &patTags)
	if len(patTags.Tags) != 0 {
		t.Fatalf("expected 0 patent tags after delete, got %d", len(patTags.Tags))
	}

	// 6. Delete tag from taxonomy via HTTP DELETE
	w = doRequest(t, handler, http.MethodDelete, "/projects/"+string(project.ID)+"/tags/prior_art", "")
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE taxonomy tag = %d: %s", w.Code, w.Body.String())
	}

	// Verify it's deleted from taxonomy
	w = doRequest(t, handler, http.MethodGet, "/projects/"+string(project.ID)+"/tags", "")
	json.Unmarshal(w.Body.Bytes(), &taxList)
	if len(taxList.Tags) != 0 {
		t.Fatalf("expected 0 taxonomy tags after delete, got %d", len(taxList.Tags))
	}
}

// Helpers


func doRequest(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
