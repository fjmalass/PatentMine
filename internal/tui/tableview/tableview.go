// Package tableview persists catalog filter/sort state via the daemon table_view API.
package tableview

import (
	"context"
	"encoding/json"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
)

const ownerLocal = "local"

// LoadDefaultPatents returns the default saved view for the patents table, if any.
func LoadDefaultPatents(ctx context.Context, client *rpc.Client) (domain.SavedTableView, bool, error) {
	var res proto.TableViewListResult
	if err := client.Call(ctx, proto.MethodTableViewList,
		proto.TableViewListParams{Owner: ownerLocal, TableType: domain.TablePatents}, &res); err != nil {
		return domain.SavedTableView{}, false, err
	}
	for _, v := range res.Views {
		if v.IsDefault {
			return v, true, nil
		}
	}
	return domain.SavedTableView{}, false, nil
}

// SavePatentsCatalog persists filter and sort fields from the catalog pane.
func SavePatentsCatalog(ctx context.Context, client *rpc.Client, name string, filter string, sortColumn domain.SortColumn, sortAscending bool, isDefault bool) (domain.SavedTableView, error) {
	viewJSON, err := json.Marshal(map[string]any{
		"filter":         filter,
		"sort_column":    sortColumn,
		"sort_ascending": sortAscending,
	})
	if err != nil {
		return domain.SavedTableView{}, err
	}
	view := domain.SavedTableView{
		Owner:     ownerLocal,
		TableType: domain.TablePatents,
		Name:      name,
		Scope:     "user",
		IsDefault: isDefault,
		ViewJSON:  viewJSON,
	}
	var res proto.TableViewResult
	if err := client.Call(ctx, proto.MethodTableViewSave, proto.TableViewSaveParams{View: view}, &res); err != nil {
		return domain.SavedTableView{}, err
	}
	return res.View, nil
}

// FilterFromView extracts the filter expression string from a saved view.
func FilterFromView(view domain.SavedTableView) string {
	var payload struct {
		Filter string `json:"filter"`
	}
	if len(view.ViewJSON) == 0 {
		return ""
	}
	if err := json.Unmarshal(view.ViewJSON, &payload); err != nil {
		return ""
	}
	return payload.Filter
}
