package domain

// AssigneeStats carries aggregated database statistics for an assignee.
type AssigneeStats struct {
	Assignee string         `json:"assignee"`
	Total    int            `json:"total"`
	States   map[string]int `json:"states"`
	Tags     map[string]int `json:"tags"`
}
