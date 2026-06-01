// Tag-taxonomy and patent-tag payloads.
package proto

import "patentmine/internal/domain"

// TagParams names a tag to assign to, or remove from, one or more patents within a
// project. On assign an unknown name creates the tag.
type TagParams struct {
	Project domain.ProjectID      `json:"project"`
	Patents []domain.PatentNumber `json:"patents"`
	Name    string                `json:"name"`
}

// TagCreateParams registers a new tag in the project's taxonomy.
type TagCreateParams struct {
	Project domain.ProjectID `json:"project"`
	Name    string           `json:"name"`
}

// TagDeleteParams removes a tag from the project's taxonomy.
type TagDeleteParams struct {
	Project domain.ProjectID `json:"project"`
	Name    string           `json:"name"`
}

// TagListParams lists all taxonomy tags in the project.
type TagListParams struct {
	Project domain.ProjectID `json:"project"`
}

// TagListResult carries the list of project taxonomy tags.
type TagListResult struct {
	Tags []domain.Tag `json:"tags"`
}

// PatentTagListParams lists tags assigned to a patent.
type PatentTagListParams struct {
	Project domain.ProjectID    `json:"project"`
	Patent  domain.PatentNumber `json:"patent"`
}

// PatentTagListResult carries the list of tags assigned to a patent.
type PatentTagListResult struct {
	Tags []domain.Tag `json:"tags"`
}
