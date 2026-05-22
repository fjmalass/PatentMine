// Package store is the persistence boundary. It defines the Repository
// interface that the engine depends on; concrete database implementations
// (today SQLite, later Postgres) live in subpackages.
package store

import (
	"context"
	"errors"
	"time"

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
	// ReviewState, when set together with Project, filters by review state.
	ReviewState domain.ReviewState
	// Relation, when set together with RelationKind, restricts results to
	// patents having that relation to the given patent number.
	Relation     domain.PatentNumber
	RelationKind domain.RelationKind
	// Search, when set, is a case-insensitive substring match on number/title.
	Search string
	// Classification, when set, filters patents by prefix matching of their classifications.
	Classification string
	// Limit is the page size; values <= 0 fall back to DefaultPageSize.
	Limit int
	// Offset is the number of rows to skip.
	Offset int
	// SortColumn names the column to order by; zero falls back to number order.
	SortColumn domain.SortColumn
	// SortAscending, when true, sorts ascending; false sorts descending.
	SortAscending bool
}

// StubRecord is a placeholder patent created for a neighbour discovered through
// a family-graph edge but not yet fetched. It is just a number and a guessed
// life stage; an existing record of the same number is never overwritten.
type StubRecord struct {
	Number domain.PatentNumber
	Stage  domain.Stage
}

// NodeBatch is every write a crawler makes for one fetched patent: the record
// itself, its life-stage documents, stub records for newly discovered
// neighbours, and the family-graph edges. SaveNode applies the whole batch in a
// single transaction, so a node with hundreds of citations costs one commit
// instead of hundreds. A zero-value Patent is skipped, so a batch may carry
// only stubs (used when a referenced patent could not be fetched).
type NodeBatch struct {
	Patent    domain.Patent
	Documents []domain.Document
	Stubs     []StubRecord
	Relations []domain.Relation
}

// Repository is the persistence contract. Every method takes a context so the
// interface is cancellation-aware and ready for a pooled/async backend without
// changing any caller — the "future non-blocking local DB" requirement.
type Repository interface {
	// SavePatent inserts or updates a patent by its number.
	SavePatent(ctx context.Context, p domain.Patent) error
	// SaveNode atomically writes one crawled patent: its record, documents,
	// neighbour stubs, and family-graph edges.
	SaveNode(ctx context.Context, batch NodeBatch) error
	// DeletePatent permanently removes a patent and all its associated
	// documents, relations, and memberships.
	DeletePatent(ctx context.Context, n domain.PatentNumber) error
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
	// AllRelations returns every family-graph edge where n is either the origin (from) or destination (to).
	AllRelations(ctx context.Context, n domain.PatentNumber) ([]domain.Relation, error)

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
	// SetReviewState changes a membership's state, or returns ErrNotFound.
	SetReviewState(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, state domain.ReviewState) error
	// DeleteMembership permanently removes a patent from a project. No-op if not a member.
	DeleteMembership(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) error
	// Memberships returns every membership of a project.
	Memberships(ctx context.Context, project domain.ProjectID) ([]domain.Membership, error)

	// CreateTag returns the project's tag of the given name, creating it when
	// the project does not have it yet. The name is matched as stored.
	CreateTag(ctx context.Context, project domain.ProjectID, name string) (domain.Tag, error)
	// DeleteTag removes a tag from the project's taxonomy.
	DeleteTag(ctx context.Context, project domain.ProjectID, name string) error
	// ProjectTags returns every tag of a project, ordered by name.
	ProjectTags(ctx context.Context, project domain.ProjectID) ([]domain.Tag, error)
	// TagByName returns one taxonomy tag in a project, matching name case-insensitively.
	TagByName(ctx context.Context, project domain.ProjectID, name string) (domain.Tag, error)
	// TagPatent assigns a tag to a patent; an existing assignment is left as is.
	TagPatent(ctx context.Context, tagID int64, patent domain.PatentNumber, assignedAt time.Time) (bool, error)
	// UntagPatent removes a tag from a patent; a missing assignment is a no-op.
	UntagPatent(ctx context.Context, tagID int64, patent domain.PatentNumber) (bool, error)
	// PatentTag returns one assigned tag on a patent in a project, matching name case-insensitively.
	PatentTag(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, name string) (domain.Tag, error)
	// PatentTags returns the tags a patent carries within a project, ordered by
	// name.
	PatentTags(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) ([]domain.Tag, error)

	// IDSEntry returns the curated IDS entry for one (project, patent) pair.
	IDSEntry(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (domain.IDSEntry, error)
	// SaveIDSEntry inserts or updates one curated IDS entry.
	SaveIDSEntry(ctx context.Context, entry domain.IDSEntry) (domain.IDSEntry, error)
	// DeleteIDSEntry removes the curated IDS entry for one (project, patent) pair.
	DeleteIDSEntry(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) error
	// ListIDSEntries returns every curated IDS entry of a project.
	ListIDSEntries(ctx context.Context, project domain.ProjectID) ([]domain.IDSEntry, error)

	// Close releases all database resources.
	Close() error
}
