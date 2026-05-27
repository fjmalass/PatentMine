package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

// TestResolveAuthorityHit confirms the cross-source dedup primitive returns
// the right record_id when an authority_identifier row exists.
func TestResolveAuthorityHit(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	p := samplePatent("US11611785B2")
	if err := repo.SavePatent(ctx, p); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}

	// Write an authority_identifier row pointing at the patent. The write
	// goes through SaveNode so the same code path ingesters use is exercised.
	if err := repo.SaveNode(ctx, store.NodeBatch{
		Patent: p,
		AuthorityIdentifiers: []domain.AuthorityIdentifier{{
			Authority:      "US",
			IdentifierType: "application",
			Identifier:     "17812078",
			RecordNumber:   p.Number,
			Source:         "uspto",
			Confidence:     100,
		}},
	}); err != nil {
		t.Fatalf("SaveNode: %v", err)
	}

	gotID, err := repo.ResolveAuthority(ctx, domain.AuthorityRef{
		Authority: "US", IdentifierType: "application", Identifier: "17812078",
	})
	if err != nil {
		t.Fatalf("ResolveAuthority: %v", err)
	}
	if gotID.IsZero() {
		t.Fatalf("ResolveAuthority returned empty record id")
	}

	// The id must round-trip back to the same patent through PatentByRecordID.
	got, err := repo.PatentByRecordID(ctx, gotID)
	if err != nil {
		t.Fatalf("PatentByRecordID: %v", err)
	}
	if got.Number != p.Number {
		t.Fatalf("PatentByRecordID = %s, want %s", got.Number, p.Number)
	}
}

// TestResolveAuthorityMiss confirms ResolveAuthority returns store.ErrNotFound
// when no row matches, so callers can fall back to minting a new record.
func TestResolveAuthorityMiss(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	_, err := repo.ResolveAuthority(ctx, domain.AuthorityRef{
		Authority: "US", IdentifierType: "application", Identifier: "99999999",
	})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestUpsertPatentDedupsAcrossSources is the integration smoke for the actual
// bug user reported: a patent ingested from Google with kind suffix "B2",
// then re-ingested from a USPTO catalog response with bare application
// number, must converge to one patent row, not two. The authority_identifier
// table is the bridge that makes this work.
func TestUpsertPatentDedupsAcrossSources(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Google ingest of US11611785B2 — writes a GOOGLE authority_identifier.
	googleNumber := domain.MustParsePatentNumber("US11611785B2")
	if err := repo.SaveNode(ctx, store.NodeBatch{
		Patent: domain.Patent{
			Number:     googleNumber,
			Title:      "Original google title",
			FetchState: domain.FetchCached,
			Source:     domain.SourceGoogle,
			FetchedAt:  now,
		},
		AuthorityIdentifiers: []domain.AuthorityIdentifier{{
			Authority:      "GOOGLE",
			IdentifierType: "grant",
			Identifier:     "US11611785B2",
			RecordNumber:   googleNumber,
			Source:         "google",
			Confidence:     80,
		}, {
			// Imagine the Google ingest also captured the underlying US
			// application number — this is the bridge that lets USPTO ingest
			// find the same record. In practice the bridge gets written by
			// the USPTO catalog response itself; this test exercises the
			// "if it has been written, ResolveAuthority finds it" path.
			Authority:      "US",
			IdentifierType: "application",
			Identifier:     "17812078",
			RecordNumber:   googleNumber,
			Source:         "uspto",
			Confidence:     100,
		}},
	}); err != nil {
		t.Fatalf("Google SaveNode: %v", err)
	}

	// 2. USPTO catalog ingest synthesizes a record number with no kind
	//    ("US17812078"). Without dedup this would be a *different* patent
	//    row. We simulate the ingest path: resolve via authority_identifier,
	//    then save under the existing record number.
	rid, err := repo.ResolveAuthority(ctx, domain.AuthorityRef{
		Authority: "US", IdentifierType: "application", Identifier: "17812078",
	})
	if err != nil {
		t.Fatalf("ResolveAuthority: %v", err)
	}
	resolved, err := repo.PatentByRecordID(ctx, rid)
	if err != nil {
		t.Fatalf("PatentByRecordID: %v", err)
	}
	if resolved.Number != googleNumber {
		t.Fatalf("dedup resolved to %s, want %s", resolved.Number, googleNumber)
	}

	// 3. Re-save under the resolved number with USPTO enrichment.
	if err := repo.SaveNode(ctx, store.NodeBatch{
		Patent: domain.Patent{
			Number:     resolved.Number,
			Title:      "USPTO-enriched title",
			FetchState: domain.FetchCached,
			Source:     domain.SourceUSPTO,
			FetchedAt:  now,
		},
	}); err != nil {
		t.Fatalf("USPTO SaveNode: %v", err)
	}

	// 4. Confirm exactly one patent row exists.
	count, err := repo.CountPatents(ctx, store.PatentQuery{})
	if err != nil {
		t.Fatalf("CountPatents: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 patent after cross-source dedup, got %d", count)
	}

	// 5. Confirm the USPTO write overwrote the title (proves it landed on
	//    the same row, not a new one).
	got, err := repo.Patent(ctx, googleNumber)
	if err != nil {
		t.Fatalf("Patent: %v", err)
	}
	if got.Title != "USPTO-enriched title" {
		t.Fatalf("expected USPTO title to win, got %q", got.Title)
	}
}

// TestRecordIDForNumber confirms the back-compat shim that lets callers keep
// passing PatentNumber as a key.
func TestRecordIDForNumber(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	p := samplePatent("US11611785B2")
	if err := repo.SavePatent(ctx, p); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}

	id, err := repo.RecordIDForNumber(ctx, p.Number)
	if err != nil {
		t.Fatalf("RecordIDForNumber: %v", err)
	}
	if id.IsZero() {
		t.Fatalf("RecordIDForNumber returned empty id")
	}

	// Unknown number → ErrNotFound.
	_, err = repo.RecordIDForNumber(ctx, domain.MustParsePatentNumber("US99999999A1"))
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown number, got %v", err)
	}
}
