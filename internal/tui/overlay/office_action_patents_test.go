package overlay

import (
	"strings"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/tui/render"
)

func TestOfficeActionPatentsAssignFlow(t *testing.T) {
	o := NewOfficeActionPatents(nil, render.NewTheme(), "p1", domain.OfficeAction{ID: "oa1", Name: "Final OA"})
	p1 := domain.MustParsePatentNumber("US0000001B2")
	p2 := domain.MustParsePatentNumber("US0000002B2")

	ov, _ := o.Update(oaPatentsLoadedMsg{
		assigned: []proto.OfficeActionAssignedPatent{{Number: p1, Status: domain.OAReviewToReview}},
		all:      []domain.PatentRow{{Number: p1}, {Number: p2}},
	})
	o = ov.(*OfficeActionPatents)
	if o.loading {
		t.Fatal("still loading after load message")
	}
	if len(o.assigned) != 1 || len(o.all) != 2 {
		t.Fatalf("panes not populated: assigned=%d all=%d", len(o.assigned), len(o.all))
	}
	if _, ok := o.status[p1.Normalized()]; !ok {
		t.Fatal("status map missing the assigned patent")
	}

	// 'a' on the top (assigned) pane is rejected with a hint.
	o.HandleKey(key("a"))
	if !strings.Contains(o.msg, "lower table") {
		t.Fatalf("assign on top pane should hint to switch tables, got %q", o.msg)
	}

	// tab to the all pane; 'a' there emits an assign command.
	o.HandleKey(key("tab"))
	if o.table.Active() != 1 {
		t.Fatal("tab did not switch to the all pane")
	}
	_, cmd, handled := o.HandleKey(key("a"))
	if !handled || cmd == nil {
		t.Fatal("'a' on the all pane should emit an assign command")
	}

	// 'x' on the assigned pane emits a release command.
	o.HandleKey(key("tab"))
	if _, cmd, _ = o.HandleKey(key("x")); cmd == nil {
		t.Fatal("'x' on the assigned pane should emit a release command")
	}

	// 'v' on the assigned pane emits a review-status command.
	if _, cmd, _ = o.HandleKey(key("v")); cmd == nil {
		t.Fatal("'v' on the assigned pane should emit a review-status command")
	}

	// enter on the assigned pane opens the patent detail.
	_, cmd, _ = o.HandleKey(key("enter"))
	if cmd == nil {
		t.Fatal("enter should emit an open-detail command")
	}
	if _, ok := cmd().(OpenPatentDetailMsg); !ok {
		t.Fatalf("enter emitted %T, want OpenPatentDetailMsg", cmd())
	}
}

func TestOfficeActionPatentsSearch(t *testing.T) {
	o := NewOfficeActionPatents(nil, render.NewTheme(), "p1", domain.OfficeAction{ID: "oa1"})
	p1 := domain.MustParsePatentNumber("US0000001B2")
	p2 := domain.MustParsePatentNumber("US0000002B2")
	ov, _ := o.Update(oaPatentsLoadedMsg{
		all: []domain.PatentRow{
			{Number: p1, DisplayNumber: p1, Title: "Video streaming", Assignee: "Acme"},
			{Number: p2, DisplayNumber: p2, Title: "Audio codec", Assignee: "Globex"},
		},
	})
	o = ov.(*OfficeActionPatents)
	if len(o.all) != 2 {
		t.Fatalf("all = %d, want 2", len(o.all))
	}

	// '/' starts search and focuses the browse (bottom) table.
	o.HandleKey(key("/"))
	if !o.searchActive || o.table.Active() != 1 {
		t.Fatalf("search not active or wrong pane (active=%v pane=%d)", o.searchActive, o.table.Active())
	}

	// Typing "video" narrows to the matching patent (by title).
	for _, r := range "video" {
		o.HandleKey(key(string(r)))
	}
	if len(o.all) != 1 || !o.all[0].Number.Equal(p1) {
		t.Fatalf("title filter -> %+v, want only p1", o.all)
	}

	// Esc clears the filter and restores the full list.
	o.HandleKey(key("esc"))
	if o.searchActive || len(o.all) != 2 {
		t.Fatalf("esc should clear search (active=%v all=%d)", o.searchActive, len(o.all))
	}

	// Search also matches assignee.
	o.HandleKey(key("/"))
	for _, r := range "globex" {
		o.HandleKey(key(string(r)))
	}
	if len(o.all) != 1 || !o.all[0].Number.Equal(p2) {
		t.Fatalf("assignee filter -> %+v, want only p2", o.all)
	}
}
