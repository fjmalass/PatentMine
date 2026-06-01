// Project, membership, add-related, orphan and review-state payloads.
package proto

import "patentmine/internal/domain"

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
