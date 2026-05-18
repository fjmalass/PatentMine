package rpc_test

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/store/sqlite"
)

// testHarness starts a daemon over a temp socket and returns a connected client.
func testHarness(t *testing.T) *rpc.Client {
	t.Helper()

	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "rpc.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	// A factory whose job emits one progress event then finishes.
	factory := func(root domain.PatentNumber, _ int) engine.Job {
		return engine.JobFunc(func(_ context.Context, id engine.JobID, emit func(proto.Event)) error {
			emit(proto.NewEvent(proto.EventIngestProgress, proto.IngestProgress{
				JobID: string(id), Found: 1, Message: root.String(),
			}))
			return nil
		})
	}
	eng := engine.New(ctx, repo, factory)

	socket := filepath.Join(t.TempDir(), "rpc.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = rpc.NewServer(eng).Serve(ctx, ln) }()

	client, err := rpc.Dial(socket)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		cancel()
		eng.Close()
		_ = repo.Close()
	})
	return client
}

func TestRPCPing(t *testing.T) {
	client := testHarness(t)
	var pong proto.PingResult
	if err := client.Call(context.Background(), proto.MethodPing, nil, &pong); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if !pong.Pong {
		t.Fatal("ping did not pong")
	}
}

func TestRPCMetricsSnapshot(t *testing.T) {
	client := testHarness(t)
	var metrics proto.MetricsResult
	if err := client.Call(context.Background(), proto.MethodMetricsGet, nil, &metrics); err != nil {
		t.Fatalf("metrics.get: %v", err)
	}
	if metrics.Metrics.Timestamp.IsZero() {
		t.Fatal("metrics snapshot timestamp should be set")
	}
	if metrics.Metrics.Timings == nil || metrics.Metrics.Counters == nil || metrics.Metrics.Gauges == nil {
		t.Fatalf("metrics snapshot maps must be initialized: %+v", metrics.Metrics)
	}
}

func TestRPCProjectRoundTrip(t *testing.T) {
	client := testHarness(t)
	ctx := context.Background()

	var created proto.ProjectResult
	if err := client.Call(ctx, proto.MethodProjectCreate,
		proto.ProjectCreateParams{Name: "Acme v Globex"}, &created); err != nil {
		t.Fatalf("project.create: %v", err)
	}
	if created.Project.Name != "Acme v Globex" {
		t.Fatalf("created %+v", created.Project)
	}

	var list proto.ProjectListResult
	if err := client.Call(ctx, proto.MethodProjectList, nil, &list); err != nil {
		t.Fatalf("project.list: %v", err)
	}
	if len(list.Projects) != 1 || list.Projects[0].ID != created.Project.ID {
		t.Fatalf("project.list = %+v", list.Projects)
	}
}

func TestRPCNotFoundError(t *testing.T) {
	client := testHarness(t)
	var got proto.PatentResult
	err := client.Call(context.Background(), proto.MethodPatentGet,
		proto.PatentGetParams{Number: domain.MustParsePatentNumber("US9999999B2")}, &got)
	if err == nil {
		t.Fatal("expected an error for a missing patent")
	}
	var protoErr *proto.Error
	if !errors.As(err, &protoErr) {
		t.Fatalf("error type = %T, want *proto.Error", err)
	}
	if protoErr.Code != proto.CodeNotFound {
		t.Fatalf("error code = %d, want %d", protoErr.Code, proto.CodeNotFound)
	}
}

func TestRPCUnknownMethod(t *testing.T) {
	client := testHarness(t)
	err := client.Call(context.Background(), proto.Method("does.not.exist"), nil, nil)
	var protoErr *proto.Error
	if !errors.As(err, &protoErr) || protoErr.Code != proto.CodeNoMethod {
		t.Fatalf("err = %v, want no-method error", err)
	}
}

func TestRPCIngestStreamsEvents(t *testing.T) {
	client := testHarness(t)
	ctx := context.Background()

	var started proto.IngestStartResult
	if err := client.Call(ctx, proto.MethodIngestFamily,
		proto.IngestFamilyParams{Root: domain.MustParsePatentNumber("US11611785B2"), Depth: 1},
		&started); err != nil {
		t.Fatalf("ingest.family: %v", err)
	}
	if started.JobID == "" {
		t.Fatal("ingest.family returned no job id")
	}

	// The job's progress event and the engine's done event must both arrive.
	var gotProgress, gotDone bool
	deadline := time.After(3 * time.Second)
	for !gotProgress || !gotDone {
		select {
		case ev, ok := <-client.Events():
			if !ok {
				t.Fatal("event stream closed early")
			}
			switch ev.Method {
			case proto.EventIngestProgress:
				gotProgress = true
			case proto.EventIngestDone:
				gotDone = true
			}
		case <-deadline:
			t.Fatalf("timed out (progress=%v done=%v)", gotProgress, gotDone)
		}
	}
}
