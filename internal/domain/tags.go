package domain

import (
	"errors"
	"regexp"
	"time"
)

var tagRegex = regexp.MustCompile(`^[a-z0-9_]+$`)

// ValidateTagName verifies that a tag name matches the strict taxonomy naming
// convention (lowercase alphanumeric and snake_case underscores: `^[a-z0-9_]+$`).
func ValidateTagName(name string) error {
	if name == "" {
		return errors.New("domain: tag name must not be empty")
	}
	if !tagRegex.MatchString(name) {
		return errors.New("domain: tag name must be strictly lowercase alphanumeric with snake_case underscores (e.g. 'prior_art')")
	}
	return nil
}

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
	// AssignedAt is when this tag was assigned to a patent.
	// Only populated when querying patent tags.
	AssignedAt time.Time `json:"assigned_at,omitempty"`
}
