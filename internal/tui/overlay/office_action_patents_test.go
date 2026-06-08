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
