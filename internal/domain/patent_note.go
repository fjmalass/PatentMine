package domain

import "time"

// PatentNote is one project-scoped markdown note for a patent.
type PatentNote struct {
	Project   ProjectID    `json:"project"`
	Patent    PatentNumber `json:"patent"`
	Markdown  string       `json:"markdown,omitempty"`
	AddedAt   time.Time    `json:"added_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}
