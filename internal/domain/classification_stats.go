package domain

// ClassificationStats carries aggregated database statistics for one classification code.
type ClassificationStats struct {
	Classification Classification `json:"classification"`
	Total          int            `json:"total"`
	States         map[string]int `json:"states"`
	Tags           map[string]int `json:"tags"`
}
