package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/store"
	"patentmine/internal/store/sqlite"
)

const replayUsage = `usage:
  patentmine replay <source_project_id> <dest_project_id>

Replays the loading history of a source project into a destination project.
The daemon ('patentmine serve') must be running.
`

func runReplay(args []string) int {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(fs.Output(), replayUsage) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}

	sourceProject := domain.ProjectID(fs.Arg(0))
	destProject := domain.ProjectID(fs.Arg(1))

	if sourceProject == "" || destProject == "" {
		fmt.Fprintln(os.Stderr, "patentmine replay: source and destination project IDs must be non-empty")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}

	ctx := context.Background()

	// 1. Open database to read memberships of the source project
	fmt.Printf("Reading database %s...\n", cfg.DBPath)
	repo, err := sqlite.Open(ctx, string(cfg.DBPath))
	if err != nil {
		return fail(fmt.Errorf("open sqlite database: %w", err))
	}
	defer repo.Close()

	// Verify source project exists and has memberships
	srcProj, err := repo.Project(ctx, sourceProject)
	if err != nil {
		if err == store.ErrNotFound {
			fmt.Fprintf(os.Stderr, "Error: source project %q not found\n", sourceProject)
			return 1
		}
		return fail(err)
	}

	memberships, err := repo.Memberships(ctx, sourceProject)
	if err != nil {
		return fail(err)
	}

	if len(memberships) == 0 {
		fmt.Fprintf(os.Stderr, "Source project %q (%s) has no memberships to replay\n", srcProj.Name, sourceProject)
		return 0
	}

	fmt.Printf("Found %d memberships in source project %q to replay\n", len(memberships), srcProj.Name)

	// 2. Connect to the active daemon via RPC
	fmt.Printf("Connecting to active daemon at %s...\n", cfg.SocketPath)
	client, err := rpc.Dial(string(cfg.SocketPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot connect to daemon: %v\nMake sure the daemon is running with 'patentmine serve'.\n", err)
		return 1
	}
	defer client.Close()

	// 3. Ensure destination project exists (create if not found)
	_, err = repo.Project(ctx, destProject)
	if err != nil {
		if err == store.ErrNotFound {
			fmt.Printf("Creating destination project %q...\n", destProject)
			var createRes proto.ProjectResult
			err = client.Call(ctx, proto.MethodProjectCreate, proto.ProjectCreateParams{
				Name: string(destProject),
			}, &createRes)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to create destination project: %v\n", err)
				return 1
			}
			fmt.Printf("Destination project created successfully: %s\n", createRes.Project.ID)
		} else {
			return fail(err)
		}
	}

	// 4. Replay memberships step-by-step
	fmt.Printf("Replaying %d loading events...\n", len(memberships))
	for idx, m := range memberships {
		fmt.Printf("[%d/%d] Replaying %s (via %s)...\n", idx+1, len(memberships), m.Patent.String(), m.AddedMethod)
		if m.AddedMethod == "related" {
			if m.ParentPatentNumber.IsZero() {
				fmt.Fprintf(os.Stderr, "Warning: membership %s was added via 'related' but has no parent patent recorded; skipping.\n", m.Patent.String())
				continue
			}
			var res proto.AddRelatedResult
			err = client.Call(ctx, proto.MethodAddRelated, proto.AddRelatedParams{
				Project: destProject,
				Patent:  m.ParentPatentNumber,
			}, &res)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to add related nodes for %s: %v\n", m.ParentPatentNumber.String(), err)
			} else {
				fmt.Printf("  Added %d related neighbors of %s\n", res.Added, m.ParentPatentNumber.String())
			}
		} else {
			// Direct add
			var res proto.MembershipAddResult
			err = client.Call(ctx, proto.MethodMembershipAdd, proto.MembershipParams{
				Project:      destProject,
				Patent:       m.Patent,
				Source:       domain.Source(m.SourceProvider),
				ConfirmMerge: true,
			}, &res)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to add patent %s: %v\n", m.Patent.String(), err)
			} else {
				if res.FetchStarted {
					fmt.Printf("  Added patent %s; crawler background job started (job: %s)\n", m.Patent.String(), res.JobID)
				} else {
					fmt.Printf("  Added patent %s (already cached)\n", m.Patent.String())
				}
			}
		}
		// Small delay to ensure database transactions and background crawlers are queued smoothly
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("Replay completed successfully.")
	return 0
}
