package api_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"patentmine/internal/api"
	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/store/sqlite"
)

// testAPI starts a daemon over a temp socket and returns the HTTP handler in
// front of it.
func testAPI(t *testing.T) http.Handler {
	t.Helper()

	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	factory := func(root domain.PatentNumber, _ int) engine.Job {
		return engine.JobFunc(func(_ context.Context, id engine.JobID, emit func(proto.Event)) error {
			emit(proto.NewEvent(proto.EventIngestProgress,
				proto.IngestProgress{JobID: string(id), Message: root.String()}))
			return nil
		})
	}
	eng := engine.New(ctx, repo, factory)

	socket := filepath.Join(t.TempDir(), "api.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
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
	t.Cleanup(func() {
		_ = client.Close()
		cancel()
		eng.Close()
		_ = repo.Close()
	})
	return api.NewServer(client, registry).Handler()
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
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

func TestAPIHealth(t *testing.T) {
	h := testAPI(t)
	w := do(t, h, http.MethodGet, "/healthz", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want 200", w.Code)
	}
	var res proto.PingResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || !res.Pong {
		t.Fatalf("health body = %s", w.Body.String())
	}
}

func TestAPIMetrics(t *testing.T) {
	h := testAPI(t)
	w := do(t, h, http.MethodGet, "/metricsz", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /metricsz = %d, want 200: %s", w.Code, w.Body.String())
	}
	var res proto.MetricsResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if res.Metrics.Timestamp.IsZero() {
		t.Fatal("metrics timestamp should be set")
	}
}

func TestAPIMetricsPrometheus(t *testing.T) {
	h := testAPI(t)
	w := do(t, h, http.MethodGet, "/metrics", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d, want 200: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("GET /metrics content-type = %q, want text/plain", got)
	}
}

func TestAPIProjectRoundTrip(t *testing.T) {
	h := testAPI(t)

	create := do(t, h, http.MethodPost, "/projects", `{"name":"Acme v Globex"}`)
	if create.Code != http.StatusOK {
		t.Fatalf("POST /projects = %d: %s", create.Code, create.Body.String())
	}
	var created proto.ProjectResult
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.Project.Name != "Acme v Globex" {
		t.Fatalf("created project = %+v", created.Project)
	}

	list := do(t, h, http.MethodGet, "/projects", "")
	var projects proto.ProjectListResult
	if err := json.Unmarshal(list.Body.Bytes(), &projects); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(projects.Projects) != 1 {
		t.Fatalf("GET /projects returned %d projects, want 1", len(projects.Projects))
	}
}

func TestAPIPatentNotFoundIs404(t *testing.T) {
	h := testAPI(t)
	w := do(t, h, http.MethodGet, "/patents/US9999999B2", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET missing patent = %d, want 404: %s", w.Code, w.Body.String())
	}
}

func TestAPIInvalidPatentNumberIs400(t *testing.T) {
	h := testAPI(t)
	w := do(t, h, http.MethodGet, "/patents/not-a-number", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("GET invalid number = %d, want 400", w.Code)
	}
}

func TestAPICommandsEndpoint(t *testing.T) {
	h := testAPI(t)
	w := do(t, h, http.MethodGet, "/commands", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /commands = %d", w.Code)
	}
	var commands []command.Command
	if err := json.Unmarshal(w.Body.Bytes(), &commands); err != nil {
		t.Fatalf("decode commands: %v", err)
	}
	if len(commands) == 0 {
		t.Fatal("GET /commands returned an empty registry")
	}
}

func TestAPIIngestStartsJob(t *testing.T) {
	h := testAPI(t)
	w := do(t, h, http.MethodPost, "/ingest", `{"root":"US11611785B2","depth":1}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /ingest = %d: %s", w.Code, w.Body.String())
	}
	var res proto.IngestStartResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || res.JobID == "" {
		t.Fatalf("ingest body = %s", w.Body.String())
	}
}
