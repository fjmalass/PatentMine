// Patent-note payloads.
package proto

import "patentmine/internal/domain"

// PatentNoteParams identifies one project-scoped patent note.
type PatentNoteParams struct {
	Project domain.ProjectID    `json:"project"`
	Patent  domain.PatentNumber `json:"patent"`
}

// PatentNoteSaveParams carries the patent note to insert or update.
type PatentNoteSaveParams struct {
	Note domain.PatentNote `json:"note"`
}

// PatentNoteResult carries one project-scoped patent note.
type PatentNoteResult struct {
	Note domain.PatentNote `json:"note"`
}

// NoteSortBy names how a patent note listing is ordered.
type NoteSortBy string

const (
	NoteSortByDate   NoteSortBy = "date"
	NoteSortByPatent NoteSortBy = "patent"
)

// PatentNoteListParams filters and sorts a patent note listing.
type PatentNoteListParams struct {
	Project domain.ProjectID `json:"project"`
	SortBy  NoteSortBy       `json:"sort_by"`
}

// PatentNoteListResult carries every note for a project.
type PatentNoteListResult struct {
	Notes []domain.PatentNote `json:"notes"`
}

// PatentNoteExportParams requests a markdown export of all notes for a project.
// When OutputPath is set the server writes the file and returns attrs.
// When OutputPath is empty the server returns the markdown in Content.
type PatentNoteExportParams struct {
	Project    domain.ProjectID `json:"project"`
	SortBy     NoteSortBy       `json:"sort_by"`
	OutputPath string           `json:"output_path,omitempty"`
}

// PatentNoteExportResult is returned by a successful export.
type PatentNoteExportResult struct {
	Path    string `json:"path"`    // absolute path written; empty when Content is set
	Count   int    `json:"count"`   // number of notes exported
	Bytes   int    `json:"bytes"`   // size of generated markdown in bytes
	Content string `json:"content"` // markdown body; populated only when OutputPath was empty
}
