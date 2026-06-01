// Saved table-view payloads.
package proto

import "patentmine/internal/domain"

// TableViewListParams selects saved views for an owner and optional table type.
type TableViewListParams struct {
	Owner     string           `json:"owner,omitempty"`
	TableType domain.TableType `json:"table_type,omitempty"`
}

// TableViewGetParams identifies one saved table view.
type TableViewGetParams struct {
	Owner string `json:"owner,omitempty"`
	ID    string `json:"id"`
}

// TableViewSaveParams carries a saved table view to insert or update.
type TableViewSaveParams struct {
	View domain.SavedTableView `json:"view"`
}

// TableViewResult carries one saved table view.
type TableViewResult struct {
	View domain.SavedTableView `json:"view"`
}

// TableViewListResult carries saved table views.
type TableViewListResult struct {
	Views []domain.SavedTableView `json:"views"`
}
