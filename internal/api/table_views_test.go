package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"patentmine/internal/proto"
)

func TestAPITableViewsRoundTrip(t *testing.T) {
	h := testAPI(t)

	create := do(t, h, http.MethodPost, "/table_views", `{
		"table_type":"ids_activity_history",
		"name":"Needs Attention",
		"is_default":true,
		"view_json":{"search":"failed","sort":[{"field":"activityDate","direction":"desc"}]}
	}`)
	if create.Code != http.StatusOK {
		t.Fatalf("POST /table_views = %d: %s", create.Code, create.Body.String())
	}
	var created proto.TableViewResult
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.View.ID == "" || created.View.Owner != "local" || created.View.TableType != "ids_activity_history" {
		t.Fatalf("created view = %+v", created.View)
	}

	list := do(t, h, http.MethodGet, "/table_views?table_type=ids_activity_history", "")
	if list.Code != http.StatusOK {
		t.Fatalf("GET /table_views = %d: %s", list.Code, list.Body.String())
	}
	var views proto.TableViewListResult
	if err := json.Unmarshal(list.Body.Bytes(), &views); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(views.Views) != 1 || views.Views[0].ID != created.View.ID {
		t.Fatalf("views = %+v, want created id %q", views.Views, created.View.ID)
	}

	get := do(t, h, http.MethodGet, "/table_views/"+created.View.ID, "")
	if get.Code != http.StatusOK {
		t.Fatalf("GET /table_views/{id} = %d: %s", get.Code, get.Body.String())
	}

	deleteResp := do(t, h, http.MethodDelete, "/table_views/"+created.View.ID, "")
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("DELETE /table_views/{id} = %d: %s", deleteResp.Code, deleteResp.Body.String())
	}
}
