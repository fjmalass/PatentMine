// Patent read payloads: ping, get/list, delete and stats.
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
	SearchScope        string                `json:"search_scope,omitempty"`
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

// AllAssigneesHistoryParams selects assignee analytics within an optional project scope.
type AllAssigneesHistoryParams struct {
	Project domain.ProjectID `json:"project,omitempty"`
}

// AllAssigneesHistoryResult carries database history/statistics for assignees.
type AllAssigneesHistoryResult struct {
	Stats []domain.AllAssigneesHistory `json:"stats"`
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
