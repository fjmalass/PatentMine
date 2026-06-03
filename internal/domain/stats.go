package domain

// AllAssigneesHistory carries aggregated database history for an assignee.
type AllAssigneesHistory struct {
	Assignee string         `json:"assignee"`
	Total    int            `json:"total"`
	States   map[string]int `json:"states"`
	Tags     map[string]int `json:"tags"`
}
