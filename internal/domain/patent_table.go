package domain

// PatentTableColumnKey identifies one column in a patent listing.
type PatentTableColumnKey string

const (
	PatentColumnIndex          PatentTableColumnKey = "index"
	PatentColumnNumber         PatentTableColumnKey = PatentTableColumnKey(SortByNumber)
	PatentColumnTitle          PatentTableColumnKey = PatentTableColumnKey(SortByTitle)
	PatentColumnInventor       PatentTableColumnKey = PatentTableColumnKey(SortByInventor)
	PatentColumnClassification PatentTableColumnKey = PatentTableColumnKey(SortByClassification)
	PatentColumnExpires        PatentTableColumnKey = PatentTableColumnKey(SortByExpires)
	PatentColumnCitations      PatentTableColumnKey = "citations"
	PatentColumnCitedBy        PatentTableColumnKey = "cited_by"
	PatentColumnParents        PatentTableColumnKey = "parents"
	PatentColumnParentage      PatentTableColumnKey = "parentage"
	PatentColumnTags           PatentTableColumnKey = PatentTableColumnKey(SortByTags)
	PatentColumnIDS            PatentTableColumnKey = PatentTableColumnKey(SortByIDS)
	PatentColumnReviewState    PatentTableColumnKey = PatentTableColumnKey(SortByReviewState)
)

// ColumnCellType identifies standard cell rendering/content types.
type ColumnCellType string

const (
	CellTypeDefault ColumnCellType = ""
	CellTypeState   ColumnCellType = "state"
)

// PatentTableColumn is the server-owned table contract consumed by frontends.
type PatentTableColumn struct {
	Key      PatentTableColumnKey `json:"key"`
	Label    string               `json:"label"`
	SortKey  SortColumn           `json:"sort_key,omitempty"`
	Sortable bool                 `json:"sortable"`
	Width    int                  `json:"width,omitempty"`
	CellType ColumnCellType       `json:"cell_type,omitempty"`
}

// PatentTableColumns returns the authoritative patent table column list for the
// given project context.
func PatentTableColumns(projectID ProjectID) []PatentTableColumn {
	stateLabel := "FETCH STATE"
	idsSortable := false
	tagsSortable := false
	if projectID != "" {
		stateLabel = "REVIEW STATE"
		idsSortable = true
		tagsSortable = true
	}
	return []PatentTableColumn{
		{Key: PatentColumnIndex, Label: "#", Width: 4},
		{Key: PatentColumnNumber, Label: "NUMBER", SortKey: SortByNumber, Sortable: true, Width: 16},
		{Key: PatentColumnTitle, Label: "TITLE", SortKey: SortByTitle, Sortable: true},
		{Key: PatentColumnInventor, Label: "INVENTOR", SortKey: SortByInventor, Sortable: true, Width: 18},
		{Key: PatentColumnClassification, Label: "CLASS", SortKey: SortByClassification, Sortable: true, Width: 16},
		{Key: PatentColumnExpires, Label: "EXPIRES", SortKey: SortByExpires, Sortable: true, Width: 10},
		{Key: PatentColumnCitations, Label: "CITES", Width: 5},
		{Key: PatentColumnCitedBy, Label: "CITED", Width: 5},
		{Key: PatentColumnParents, Label: "PARENTS", Width: 7},
		{Key: PatentColumnTags, Label: "TAGS", SortKey: SortByTags, Sortable: tagsSortable, Width: 14},
		{Key: PatentColumnIDS, Label: "IDS", SortKey: SortByIDS, Sortable: idsSortable, Width: 12},
		{Key: PatentColumnReviewState, Label: stateLabel, SortKey: SortByReviewState, Sortable: true, Width: 13, CellType: CellTypeState},
	}
}

// PatentTableAllowsSort reports whether the current table contract exposes the
// given sort key for the given project context.
func PatentTableAllowsSort(projectID ProjectID, sort SortColumn) bool {
	if sort == "" {
		return true
	}
	for _, col := range PatentTableColumns(projectID) {
		if col.Sortable && col.SortKey == sort {
			return true
		}
	}
	return false
}
