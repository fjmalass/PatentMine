package proto

import "patentmine/internal/domain"

// PatentTableColumnsParams selects the patent-table schema for a project context.
type PatentTableColumnsParams struct {
	Project domain.ProjectID `json:"project,omitempty"`
}

// PatentTableColumnsResult carries the server-owned patent table schema.
type PatentTableColumnsResult struct {
	Columns []domain.PatentTableColumn `json:"columns"`
}
