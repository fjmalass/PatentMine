package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// TableType identifies which table a saved view belongs to.
type TableType string

const (
	TableIDSActivityHistory TableType = "ids_activity_history"
	TablePatents            TableType = "patents"
	TableCitations          TableType = "citations"
	TableInventorStats      TableType = "inventor_stats"
)

// TableViewScope describes who can see a saved table view.
type TableViewScope string

const (
	TableViewScopeUser   TableViewScope = "user"
	TableViewScopeSystem TableViewScope = "system"
	TableViewScopeTeam   TableViewScope = "team"
)

// DefaultTableViewOwner is used until the product has authenticated users.
const DefaultTableViewOwner = "local"

// SavedTableView stores reusable table state. ViewJSON is table-specific JSON;
// it may contain search text, filters, sort order, visible columns, and other
// table presentation state.
type SavedTableView struct {
	ID        string          `json:"id"`
	Owner     string          `json:"owner"`
	TableType TableType       `json:"table_type"`
	Name      string          `json:"name"`
	Scope     TableViewScope  `json:"scope"`
	IsDefault bool            `json:"is_default,omitempty"`
	ViewJSON  json.RawMessage `json:"view_json"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Normalize fills defaults and trims user-facing strings.
func (v *SavedTableView) Normalize() {
	v.Owner = strings.TrimSpace(v.Owner)
	if v.Owner == "" {
		v.Owner = DefaultTableViewOwner
	}
	v.Name = strings.TrimSpace(v.Name)
	v.TableType = TableType(strings.TrimSpace(string(v.TableType)))
	v.Scope = TableViewScope(strings.TrimSpace(string(v.Scope)))
	if v.Scope == "" {
		v.Scope = TableViewScopeUser
	}
	if len(v.ViewJSON) == 0 {
		v.ViewJSON = json.RawMessage(`{}`)
	}
}

// Validate checks the generic saved-view envelope. Individual table schemas can
// validate ViewJSON as they grow richer.
func (v SavedTableView) Validate() error {
	if v.Owner == "" {
		return errors.New("table view owner must not be empty")
	}
	if !IsKnownTableType(v.TableType) {
		return errors.New("unknown table type")
	}
	if v.Name == "" {
		return errors.New("table view name must not be empty")
	}
	if !IsKnownTableViewScope(v.Scope) {
		return errors.New("unknown table view scope")
	}
	if !json.Valid(v.ViewJSON) {
		return errors.New("table view JSON must be valid JSON")
	}
	var object map[string]any
	if err := json.Unmarshal(v.ViewJSON, &object); err != nil {
		return errors.New("table view JSON must be an object")
	}
	if object == nil {
		return errors.New("table view JSON must be an object")
	}
	return nil
}

func IsKnownTableType(t TableType) bool {
	switch t {
	case TableIDSActivityHistory, TablePatents, TableCitations, TableInventorStats:
		return true
	default:
		return false
	}
}

func IsKnownTableViewScope(scope TableViewScope) bool {
	switch scope {
	case TableViewScopeUser, TableViewScopeSystem, TableViewScopeTeam:
		return true
	default:
		return false
	}
}
