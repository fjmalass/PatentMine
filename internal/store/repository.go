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
	// Numbers, when set, restricts results to only these specific patent numbers.
	Numbers []domain.PatentNumber
	// Project, when set, restricts results to that project's members.
	Project domain.ProjectID
	// Filter is the boolean patent filter expression used by the TUI command path.
	Filter string
	// ReviewState, when set together with Project, filters by review state.
	ReviewState domain.ReviewState
	// IDSStatus, when set together with Project, filters by curated IDS entry status.
	IDSStatus string
	// Relation, when set together with RelationKind, restricts results to
	// patents having that relation to the given patent number.
	Relation     domain.PatentNumber
	RelationKind domain.RelationKind
	// Search, when set, is a case-insensitive substring match on number/title.
	Search string
	// SearchScope, when set, narrows Search to specific fields.
	SearchScope string
	// Classification, when set, filters patents by prefix matching of their classifications.
	Classification string
	// ClassificationCode, when set, filters patents by exact classification code match.
	ClassificationCode string
	// Inventor, when set, filters patents by exact match of their inventors.
	Inventor string
	// Assignee, when set, filters patents by exact match of their assignee.
	Assignee string
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
	Patent               domain.Patent
	Documents            []domain.Document
	Stubs                []StubRecord
	Relations            []domain.Relation
	AuthorityIdentifiers []domain.AuthorityIdentifier
	USPTOApplication     *domain.USPTOApplication
	USPTOParties         []domain.USPTOParty
	USPTOEvents          []domain.USPTOEvent
	USPTOContinuities    []domain.USPTOContinuity
	USPTOForeignPriority []domain.USPTOForeignPriority
	SourceSnapshots      []domain.SourceSnapshot
	SourceDiffs          []domain.SourceDiff
	// SourceBibs holds each source's bibliographic snapshot for this record, the
	// substrate for side-by-side source comparison.
	SourceBibs []domain.SourceBib
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
	// DeletePatents permanently removes multiple patents and all their associated
	// documents, relations, and memberships in one transaction.
	DeletePatents(ctx context.Context, patents []domain.PatentNumber) error
	// SoftDeletePatent demotes a patent to FetchStub: documents, memberships,
	// project notes, tag assignments, and outbound family-graph edges are
	// removed; inbound edges from other patents are kept so the family-graph
	// topology survives. When no inbound edge remains, the patent row is
	// hard-purged to avoid an orphan stub.
	SoftDeletePatent(ctx context.Context, n domain.PatentNumber) error
	// SoftDeletePatents is the batch variant of SoftDeletePatent. Orphan-stub
	// purging is computed after every patent in the batch has been demoted,
	// so mutually-referencing pairs deleted together are both purged.
	SoftDeletePatents(ctx context.Context, patents []domain.PatentNumber) error
	// Patent returns one patent, or ErrNotFound.
	Patent(ctx context.Context, n domain.PatentNumber) (domain.Patent, error)
	// USPTOApplication returns the saved USPTO application attrs for a patent record, or ErrNotFound.
	USPTOApplication(ctx context.Context, n domain.PatentNumber) (domain.USPTOApplication, error)
	// USPTOContinuities returns the parent/child continuity rows touching appNum,
	// in ordinal order, for walking the patent-term continuity chain.
	USPTOContinuities(ctx context.Context, appNum string) ([]domain.USPTOContinuity, error)
	// USPTOApplicationByAppNum returns the saved USPTO application keyed by its
	// application number, or ErrNotFound.
	USPTOApplicationByAppNum(ctx context.Context, appNum string) (domain.USPTOApplication, error)
	// USPTOXMLDownload returns the per-document download record, or ErrNotFound
	// when nothing has been fetched yet for that (application, kind) pair.
	USPTOXMLDownload(ctx context.Context, applicationNumber, kind string) (domain.USPTOXMLDownload, error)
	// RecordUSPTOXMLDownload inserts or updates a download record. The record's
	// DownloadCount is taken as-is; callers should pre-increment.
	RecordUSPTOXMLDownload(ctx context.Context, rec domain.USPTOXMLDownload) error
	// SaveUSPTOGrantIngest persists a parsed grant XML bundle for one
	// application: summary fields are merged onto uspto_application; body,
	// drawings, citations, classifications, and relations are written into
	// their child tables. The save is atomic.
	SaveUSPTOGrantIngest(ctx context.Context, ingest domain.USPTOGrantIngest) error
	// ClearUSPTOGrantBodies clears cached parsed patent bodies in uspto_grant_body.
	// If applicationNumbers is empty, it clears all cached patent bodies in the table.
	ClearUSPTOGrantBodies(ctx context.Context, applicationNumbers []string) (int64, int64, error)
	// USPTOGrantBody returns the body for one (application_number, kind), or
	// ErrNotFound when the XML has not been ingested yet.
	USPTOGrantBody(ctx context.Context, applicationNumber, kind string) (domain.USPTOGrantBody, error)
	// USPTOGrantParties returns every applicant/inventor/assignee/agent row
	// extracted from the XML for one (application_number, kind). An empty
	// result is returned when no party rows exist (not an error).
	USPTOGrantParties(ctx context.Context, applicationNumber, kind string) ([]domain.USPTOGrantParty, error)
	// SaveUSPTOAssignments persists every recorded assignment for one
	// application as a full replacement. Each USPTOAssignment carries its
	// own Parties slice; both tables are written in one transaction.
	SaveUSPTOAssignments(ctx context.Context, applicationNumber string, assignments []domain.USPTOAssignment) error
	// USPTOAssignments returns every recorded assignment for one application
	// in document order, with their parties attached. Empty result, no error.
	USPTOAssignments(ctx context.Context, applicationNumber string) ([]domain.USPTOAssignment, error)
	// RebuildAssigneeHistoryForNumber recomputes the record.id-keyed ownership
	// timeline (at-grant owner + recorded assignments) for one patent. pulledAt is
	// the provenance timestamp stamped on the derived rows. No record row: no-op.
	RebuildAssigneeHistoryForNumber(ctx context.Context, number domain.PatentNumber, pulledAt string) error
	// AssigneeHistory returns one record's ownership timeline, oldest first, with
	// the current owner(s) flagged (IsLatest). Empty result, no error.
	AssigneeHistory(ctx context.Context, number domain.PatentNumber) ([]domain.AssigneeHistoryEntry, error)
	// ProjectAssignees rolls up assignee ownership across a project's members:
	// current owners (deduped, live patents only) and every assignee ever seen.
	ProjectAssignees(ctx context.Context, project domain.ProjectID) (domain.ProjectAssigneeRollup, error)
	// ListPatents returns one page of lightweight listing rows matching q.
	ListPatents(ctx context.Context, q PatentQuery) ([]domain.PatentRow, error)
	// CountPatents returns the total rows matching q, ignoring its paging.
	CountPatents(ctx context.Context, q PatentQuery) (int, error)
	// ListOrphanPatents returns patents that have no membership in any project,
	// paged by limit/offset. The total reports the full orphan count regardless
	// of paging. Tags are not attached: orphan patents belong to no project.
	ListOrphanPatents(ctx context.Context, limit, offset int) ([]domain.PatentRow, int, error)
	// PatentInventorStats aggregates database statistics for a set of inventors within a project.
	PatentInventorStats(ctx context.Context, project domain.ProjectID, inventors []domain.Inventor) ([]domain.InventorStats, error)
	// AllAssigneesHistory aggregates database history/statistics for assignees within a project.
	AllAssigneesHistory(ctx context.Context, project domain.ProjectID) ([]domain.AllAssigneesHistory, error)
	// PatentClassificationStats aggregates database statistics for classification codes within a project.
	PatentClassificationStats(ctx context.Context, project domain.ProjectID) ([]domain.ClassificationStats, error)

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
	// SetReviewStates changes multiple memberships' states in a transaction, or returns ErrNotFound.
	SetReviewStates(ctx context.Context, project domain.ProjectID, patents []domain.PatentNumber, state domain.ReviewState) error
	// PromotePatentMemberships changes review states from one to another for a patent across all projects.
	PromotePatentMemberships(ctx context.Context, patent domain.PatentNumber, from, to domain.ReviewState) error
	// DeleteMembership permanently removes a patent from a project. No-op if not a member.
	DeleteMembership(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) error
	// Memberships returns every membership of a project.
	Memberships(ctx context.Context, project domain.ProjectID) ([]domain.Membership, error)
	// ProjectsForPatent returns the IDs of every project the patent belongs to.
	ProjectsForPatent(ctx context.Context, patent domain.PatentNumber) ([]domain.ProjectID, error)

	// CreateTag returns the project's tag of the given name, creating it when
	// the project does not have it yet. The name is matched as stored.
	CreateTag(ctx context.Context, project domain.ProjectID, name string) (domain.Tag, error)
	// DeleteTag removes a tag from the project's taxonomy.
	DeleteTag(ctx context.Context, project domain.ProjectID, name string) error
	// ProjectTags returns every tag of a project, ordered by name.
	ProjectTags(ctx context.Context, project domain.ProjectID) ([]domain.Tag, error)
	// TagByName returns one taxonomy tag in a project, matching name case-insensitively.
	TagByName(ctx context.Context, project domain.ProjectID, name string) (domain.Tag, error)
	// TagPatents assigns a tag to multiple patents in a transaction.
	TagPatents(ctx context.Context, tagID int64, patents []domain.PatentNumber, assignedAt time.Time) error
	// UntagPatents removes a tag from multiple patents in a transaction.
	UntagPatents(ctx context.Context, tagID int64, patents []domain.PatentNumber) error
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

	// PatentNote returns one project-scoped patent note.
	PatentNote(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (domain.PatentNote, error)
	// ListPatentNotes returns all project-scoped notes sorted by date (updated_at DESC) or patent number.
	ListPatentNotes(ctx context.Context, project domain.ProjectID, sortByDate bool) ([]domain.PatentNote, error)
	// SavePatentNote inserts or updates one project-scoped patent note.
	SavePatentNote(ctx context.Context, note domain.PatentNote) (domain.PatentNote, error)
	// DeletePatentNote removes one project-scoped patent note.
	DeletePatentNote(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) error

	// SaveClassificationDefinition inserts or updates a classification definition.
	SaveClassificationDefinition(ctx context.Context, c domain.Classification) error
	// ClassificationDefinition returns one classification definition, or ErrNotFound.
	ClassificationDefinition(ctx context.Context, system, code string) (domain.Classification, error)
	// ListClassificationDefinitions returns all classification definitions.
	ListClassificationDefinitions(ctx context.Context) ([]domain.Classification, error)
	// ListClassificationDefinitionsByCodes returns cached definitions matching the given raw codes.
	ListClassificationDefinitionsByCodes(ctx context.Context, codes []string) ([]domain.Classification, error)
	// DeleteClassificationDefinition removes a classification definition.
	DeleteClassificationDefinition(ctx context.Context, system, code string) error
	// SaveMutationGroup appends an auditable mutation journal entry.
	SaveMutationGroup(ctx context.Context, group domain.MutationGroup, items []domain.MutationItem) error

	// SaveTableView inserts or updates one named saved table view.
	SaveTableView(ctx context.Context, view domain.SavedTableView) (domain.SavedTableView, error)
	// TableView returns one saved table view owned by owner, or ErrNotFound.
	TableView(ctx context.Context, owner, id string) (domain.SavedTableView, error)
	// ListTableViews returns saved table views for an owner and optional table type.
	ListTableViews(ctx context.Context, owner string, tableType domain.TableType) ([]domain.SavedTableView, error)
	// DeleteTableView removes one saved table view owned by owner.
	DeleteTableView(ctx context.Context, owner, id string) error

	// ListSourceDiffs returns source comparison/reconciliation diffs for a patent
	// (newest first). Used by the TUI comparison overlay and resolve flow (Option A).
	ListSourceDiffs(ctx context.Context, patent domain.PatentNumber) ([]domain.SourceDiff, error)

	// ListSourceSnapshots returns fetch provenance snapshots for a patent (newest first).
	// Supports audit and future diff reconstruction.
	ListSourceSnapshots(ctx context.Context, patent domain.PatentNumber) ([]domain.SourceSnapshot, error)

	// SourceBibs returns each source's bibliographic snapshot for a record, for
	// side-by-side comparison ("see both versions"). Ordered by source.
	SourceBibs(ctx context.Context, patent domain.PatentNumber) ([]domain.SourceBib, error)

	// CheckCollisions scans the database for collision/merge candidates.
	CheckCollisions(ctx context.Context) ([]CollisionCandidate, error)

	// Close releases all database resources.
	Close() error
}

// CollisionCandidate represents a duplicate stage patent collision.
type CollisionCandidate struct {
	ApplicationNumber string                `json:"application_number"`
	RecordNumbers     []domain.PatentNumber `json:"record_numbers"`
	Titles            []string              `json:"titles"`
}
