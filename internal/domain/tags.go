package domain

import "time"

// Tag is a user-defined label scoped to one project. The same patent can carry
// several tags, and a patent shared across projects is tagged independently in
// each — so a tag belongs to exactly one project, never to a patent globally.
type Tag struct {
	// ID is the tag's store-assigned identifier, stable for its lifetime.
	ID int64 `json:"id"`
	// Project is the project the tag belongs to.
	Project ProjectID `json:"project"`
	// Name is the label shown to the user; unique within its project.
	Name string `json:"name"`
	// CreatedAt is when the tag was first created.
	CreatedAt time.Time `json:"created_at"`
}
