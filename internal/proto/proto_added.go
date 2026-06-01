// Added-list (bulk add/export) payloads.
package proto

import "patentmine/internal/domain"

// AddedExportParams requests the plain-text list of a project's manually-added
// patents. When OutputPath is set the server writes the file and returns its
// path; when empty the server returns the list body in Content.
type AddedExportParams struct {
	Project    domain.ProjectID `json:"project"`
	OutputPath string           `json:"output_path,omitempty"`
}

// AddedExportResult is returned by a successful added-list export.
type AddedExportResult struct {
	Path    string `json:"path"`    // absolute path written; empty when Content is set
	Count   int    `json:"count"`   // number of patents exported
	Content string `json:"content"` // list body; populated only when OutputPath was empty
}

// AddedImportParams loads a plain-text patent list into a project, adding each
// number with manual (direct) provenance. Content is used when set; otherwise
// the server reads the file at Path.
type AddedImportParams struct {
	Project domain.ProjectID `json:"project"`
	Path    string           `json:"path,omitempty"`
	Content string           `json:"content,omitempty"`
}

// AddedImportResult reports the outcome of an added-list import.
type AddedImportResult struct {
	Total    int      `json:"total"`              // numbers found in the file
	Added    int      `json:"added"`              // memberships granted (or already present)
	Failed   int      `json:"failed"`             // numbers that could not be added
	Failures []string `json:"failures,omitempty"` // "<number>: <reason>" for each failure
}
