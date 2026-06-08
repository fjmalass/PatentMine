package overlay

import (
	"strings"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

func TestOfficeActionPickerShowsMultiSelectState(t *testing.T) {
	p1 := domain.MustParsePatentNumber("US0000001B2")
	p2 := domain.MustParsePatentNumber("US0000002B2")
	o := NewOfficeActionPicker(nil, render.NewTheme(), "p1", []domain.PatentNumber{p1, p2}, false)

	ov, _ := o.Update(oaPickerLoadedMsg{
		items: []domain.OfficeAction{
			{ID: "oa-all", Name: "All Assigned"},
			{ID: "oa-some", Name: "Some Assigned"},
			{ID: "oa-none", Name: "None Assigned"},
		},
		counts: map[string]int{
			"oa-all":  2,
			"oa-some": 1,
		},
	})
	o = ov.(*OfficeActionPicker)

	if got := o.checked["oa-all"]; got != CheckChecked {
		t.Fatalf("oa-all state = %v, want checked", got)
	}
	if got := o.checked["oa-some"]; got != CheckIndeterminate {
		t.Fatalf("oa-some state = %v, want indeterminate", got)
	}
	if got := o.checked["oa-none"]; got != CheckUnchecked {
		t.Fatalf("oa-none state = %v, want unchecked", got)
	}

	out := o.View(80, 12)
	for _, want := range []string{"[x] All Assigned", "[-] Some Assigned", "[ ] None Assigned"} {
		if !strings.Contains(out, want) {
			t.Fatalf("picker view missing %q:\n%s", want, out)
		}
	}
}

func TestOfficeActionPickerEnterValidatesNoChanges(t *testing.T) {
	p1 := domain.MustParsePatentNumber("US0000001B2")
	o := NewOfficeActionPicker(nil, render.NewTheme(), "p1", []domain.PatentNumber{p1}, false)
	ov, _ := o.Update(oaPickerLoadedMsg{items: []domain.OfficeAction{{ID: "oa1", Name: "Final OA"}}})
	o = ov.(*OfficeActionPicker)

	_, cmd, handled := o.HandleKey(key("enter"))
	if !handled {
		t.Fatal("enter should be handled")
	}
	if cmd != nil {
		t.Fatal("enter with no checklist changes should not apply")
	}
	if !strings.Contains(o.msg, "change") {
		t.Fatalf("validation message = %q, want change prompt", o.msg)
	}
}

func TestOfficeActionPickerSpaceCyclesLikeTagPicker(t *testing.T) {
	p1 := domain.MustParsePatentNumber("US0000001B2")
	p2 := domain.MustParsePatentNumber("US0000002B2")
	o := NewOfficeActionPicker(nil, render.NewTheme(), "p1", []domain.PatentNumber{p1, p2}, false)
	ov, _ := o.Update(oaPickerLoadedMsg{
		items:  []domain.OfficeAction{{ID: "oa1", Name: "Final OA"}},
		counts: map[string]int{"oa1": 1},
	})
	o = ov.(*OfficeActionPicker)

	o.HandleKey(key(" "))
	if got := o.checked["oa1"]; got != CheckChecked {
		t.Fatalf("indeterminate + space = %v, want checked", got)
	}
	_, cmd, _ := o.HandleKey(key("enter"))
	if cmd == nil {
		t.Fatal("enter after a checklist change should apply")
	}
}
