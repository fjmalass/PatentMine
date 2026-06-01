// Patent, project, membership and review-state payloads.
package proto

import "patentmine/internal/domain"

// PingResult confirms the daemon is alive.
type PingResult struct {
	Pong             bool   `json:"pong"`
	Version          string `json:"version"`
	USPTOConfigured  bool   `json:"uspto_configured"`
	BackupConfigured bool   `json:"backup_configured"`
}

// PatentDeleteParams identifies the patent to permanently remove.
type PatentDeleteParams struct {
	Number domain.PatentNumber `json:"number"`
}

// PatentDeleteBulkParams identifies the patents to permanently remove.
type PatentDeleteBulkParams struct {
	Patents []domain.PatentNumber `json:"patents"`
}

// PatentGetParams selects a single patent. Project, when set, scopes the
// project-relative fields of the result — review state and tags — to that
// project; the patent record itself is project-independent.
type PatentGetParams struct {
	Number  domain.PatentNumber `json:"number"`
	Project domain.ProjectID    `json:"project,omitempty"`
}

// PatentResult carries one patent. State and Tags are populated only when the
// request named a project the patent is a member of, and are empty otherwise:
// they describe the (patent, project) pair, not the patent.
type PatentResult struct {
	Patent           domain.Patent            `json:"patent"`
	ReviewState      domain.ReviewState       `json:"review_state,omitempty"`
	Tags             []domain.Tag             `json:"tags,omitempty"`
	IDSEntry         *domain.IDSEntry         `json:"ids_entry,omitempty"`
	PatentNote       *domain.PatentNote       `json:"patent_note,omitempty"`
	USPTOApplication *domain.USPTOApplication `json:"uspto_application,omitempty"`
	Membership       *domain.Membership       `json:"membership,omitempty"`
}

// PatentListParams selects and paginates a patent listing.
type PatentListParams struct {
	Numbers            []domain.PatentNumber `json:"numbers,omitempty"`
	Project            domain.ProjectID      `json:"project,omitempty"`
	Filter             string                `json:"filter,omitempty"`
	ReviewState        domain.ReviewState    `json:"review_state,omitempty"`
	IDSStatus          string                `json:"ids_status,omitempty"`
	Search             string                `json:"search,omitempty"`
	Classification     string                `json:"classification,omitempty"`
	ClassificationCode string                `json:"classification_code,omitempty"`
	Inventor           string                `json:"inventor,omitempty"`
	Assignee           string                `json:"assignee,omitempty"`
	Limit              int                   `json:"limit,omitempty"`
	Offset             int                   `json:"offset,omitempty"`
	SortColumn         domain.SortColumn     `json:"sort_column,omitempty"`
	SortAscending      bool                  `json:"sort_ascending,omitempty"`
}

// PatentListResult carries one page of patents plus the unpaged total.
type PatentListResult struct {
	Patents []domain.PatentRow `json:"patents"`
	Total   int                `json:"total"`
}

// PatentAssigneeStatsParams selects assignee analytics within an optional project scope.
type PatentAssigneeStatsParams struct {
	Project domain.ProjectID `json:"project,omitempty"`
}

// PatentAssigneeStatsResult carries database statistics for assignees.
type PatentAssigneeStatsResult struct {
	Stats []domain.AssigneeStats `json:"stats"`
}

// PatentClassificationStatsParams selects classification analytics within an optional project scope.
type PatentClassificationStatsParams struct {
	Project domain.ProjectID `json:"project,omitempty"`
}

// PatentClassificationStatsResult carries database statistics for classifications.
type PatentClassificationStatsResult struct {
	Stats []domain.ClassificationStats `json:"stats"`
}

// PatentInventorStatsResult carries database statistics for multiple inventors.
type PatentInventorStatsResult struct {
	Stats []domain.InventorStats `json:"stats"`
}

// ProjectListResult carries every project.
type ProjectListResult struct {
	Projects []domain.Project `json:"projects"`
}

// ProjectCreateParams names a new project.
type ProjectCreateParams struct {
	Name string `json:"name"`
}

// ProjectResult carries one project.
type ProjectResult struct {
	Project domain.Project `json:"project"`
}

// MergeWarning reports that a patent being added maps to an existing record
// in the project, representing a merge of different document stages.
type MergeWarning struct {
	Requested      string `json:"requested"`
	ExistingRecord string `json:"existing_record"`
	Message        string `json:"message"`
}

// MembershipParams identifies a (project, patent) pair.
type MembershipParams struct {
	Project           domain.ProjectID    `json:"project"`
	Patent            domain.PatentNumber `json:"patent"`
	Source            domain.Source       `json:"source,omitempty"`
	ApplicationNumber string              `json:"application_number,omitempty"`
	ConfirmMerge      bool                `json:"confirm_merge,omitempty"`
}

// MembershipAddResult reports the outcome of adding a patent to a project.
type MembershipAddResult struct {
	FetchStarted          bool                    `json:"fetch_started"`
	JobID                 string                  `json:"job_id,omitempty"`
	Candidates            []domain.USPTOCandidate `json:"candidates,omitempty"`
	MergeWarning          *MergeWarning           `json:"merge_warning,omitempty"`
	AutoSelectedCandidate *domain.USPTOCandidate  `json:"auto_selected_candidate,omitempty"`
	AlreadyInDB           bool                    `json:"already_in_db,omitempty"`
}

// AddRelatedParams asks the daemon to grant project membership to every
// family-graph neighbor of patent that does not yet have one.
type AddRelatedParams struct {
	Project domain.ProjectID    `json:"project"`
	Patent  domain.PatentNumber `json:"patent"`
}

// AddRelatedResult reports how many neighbor memberships were added.
type AddRelatedResult struct {
	Added     int                   `json:"added"`
	Neighbors []domain.PatentNumber `json:"neighbors,omitempty"`
}

// OrphanListParams selects a page of patents with no membership in any project.
type OrphanListParams struct {
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// OrphanListResult carries one page of orphan patent rows.
type OrphanListResult struct {
	Patents []domain.PatentRow `json:"patents"`
	Total   int                `json:"total"`
}

// ReviewStateParams sets one or more memberships' states.
type ReviewStateParams struct {
	Project     domain.ProjectID      `json:"project"`
	Patents     []domain.PatentNumber `json:"patents"`
	State       string                `json:"state"`
	AddedMethod string                `json:"added_method,omitempty"`
}

// ReviewStateResult reports the canonical patent records whose state changed.
type ReviewStateResult struct {
	Patents []domain.PatentNumber `json:"patents"`
}
