// Package store is the persistence boundary. It defines the Repository
// interface that the engine depends on; concrete database implementations
// (today SQLite, later Postgres) live in subpackages.
package store

import (
	"context"
	"errors"

	"patentmine/internal/domain"
)

// ErrNotFound is returned by lookups when no matching row exists.
var ErrNotFound = errors.New("store: not found")

// DefaultPageSize is the listing page size used when a query asks for none.
const DefaultPageSize = 100

// PatentQuery selects and paginates a patent listing. Pagination is applied in
// the database (LIMIT/OFFSET) so a client never loads the whole table.
type PatentQuery struct {
	// Project, when set, restricts results to that project's members.
	Project domain.ProjectID
	// State, when set together with Project, filters by membership state.
	State domain.MembershipState
	// Search, when set, is a case-insensitive substring match on number/title.
	Search string
	// Limit is the page size; values <= 0 fall back to DefaultPageSize.
	Limit int
	// Offset is the number of rows to skip.
	Offset int
}

// Repository is the persistence contract. Every method takes a context so the
// interface is cancellation-aware and ready for a pooled/async backend without
// changing any caller — the "future non-blocking local DB" requirement.
type Repository interface {
	// SavePatent inserts or updates a patent by its number.
	SavePatent(ctx context.Context, p domain.Patent) error
	// Patent returns one patent, or ErrNotFound.
	Patent(ctx context.Context, n domain.PatentNumber) (domain.Patent, error)
	// ListPatents returns one page of lightweight listing rows matching q.
	ListPatents(ctx context.Context, q PatentQuery) ([]domain.PatentRow, error)
	// CountPatents returns the total rows matching q, ignoring its paging.
	CountPatents(ctx context.Context, q PatentQuery) (int, error)

	// SaveDocument inserts or updates one life-stage document of a record.
	SaveDocument(ctx context.Context, recordNumber domain.PatentNumber, doc domain.Document) error
	// Documents returns every life-stage document of a record.
	Documents(ctx context.Context, recordNumber domain.PatentNumber) ([]domain.Document, error)
	// RecordOf returns the record number a document number belongs to, or
	// ErrNotFound when the number is unknown.
	RecordOf(ctx context.Context, number domain.PatentNumber) (domain.PatentNumber, error)
	// MergeRecords folds the absorb record into keep: documents, memberships,
	// and relations are repointed and the absorb row is removed.
	MergeRecords(ctx context.Context, keep, absorb domain.PatentNumber) error

	// SaveRelation inserts a family-graph edge; duplicates are ignored.
	SaveRelation(ctx context.Context, r domain.Relation) error
	// Relations returns edges of the given kind originating at n.
	Relations(ctx context.Context, n domain.PatentNumber, kind domain.RelationKind) ([]domain.Relation, error)

	// SaveProject inserts or updates a project by its id.
	SaveProject(ctx context.Context, p domain.Project) error
	// Project returns one project, or ErrNotFound.
	Project(ctx context.Context, id domain.ProjectID) (domain.Project, error)
	// ListProjects returns every project, ordered by name.
	ListProjects(ctx context.Context) ([]domain.Project, error)

	// AddMembership links a patent to a project; an existing link is left as is.
	AddMembership(ctx context.Context, m domain.Membership) error
	// Membership returns one membership, or ErrNotFound.
	Membership(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (domain.Membership, error)
	// SetMembershipState changes a membership's state, or returns ErrNotFound.
	SetMembershipState(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, state domain.MembershipState) error
	// Memberships returns every membership of a project.
	Memberships(ctx context.Context, project domain.ProjectID) ([]domain.Membership, error)

	// Close releases all database resources.
	Close() error
}
