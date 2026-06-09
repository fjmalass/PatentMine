package pane

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/tui/render"
)

func TestResponseDueLabel(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		oa   domain.OfficeAction
		want string
	}{
		{"no deadline", domain.OfficeAction{}, "—"},
		{"overdue", domain.OfficeAction{Status: domain.OAStatusOpen, ResponseDue: now.AddDate(0, 0, -3)}, "OVERDUE"},
		{"due soon shows day count", domain.OfficeAction{Status: domain.OAStatusOpen, ResponseDue: now.AddDate(0, 0, 5)}, "d ·"},
		{"responded shows plain date", domain.OfficeAction{Status: domain.OAStatusResponded, ResponseDue: now.AddDate(0, 0, -3)}, now.AddDate(0, 0, -3).Format(domain.DateLayout)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := responseDueLabel(c.oa)
			if !strings.Contains(got, c.want) {
				t.Fatalf("responseDueLabel = %q, want it to contain %q", got, c.want)
			}
		})
	}
}

func TestFmtDuration(t *testing.T) {
	cases := map[int]string{
		0:    "0:00",
		42:   "0:42",
		75:   "1:15",
		3600: "1:00",
		3725: "1:02",
	}
	for secs, want := range cases {
		if got := fmtDuration(secs); got != want {
			t.Errorf("fmtDuration(%d) = %q, want %q", secs, got, want)
		}
	}
}

func TestFormatTimeSummaryOrdersActivities(t *testing.T) {
	s := domain.TimeSummary{Seconds: map[domain.TimeActivity]int{
		domain.TimeWriting: 75, domain.TimeReading: 42, domain.TimeAI: 8,
	}}
	got := formatTimeSummary(s)
	// Reading must precede writing must precede ai.
	ri, wi, ai := strings.Index(got, "Reading"), strings.Index(got, "Writing"), strings.Index(got, "AI")
	if !(ri >= 0 && ri < wi && wi < ai) {
		t.Fatalf("formatTimeSummary ordering wrong: %q", got)
	}
}

func TestOfficeActionDetailNavMovesRowCursor(t *testing.T) {
	o := NewOfficeActionDetail(nil, render.NewTheme(), domain.OfficeAction{ID: "oa-1", Project: "p1"})
	o.loading = false

	o.Command(command.NavDown, Invocation{Repeat: 2})
	if o.row != 2 {
		t.Fatalf("row after nav down = %d, want 2", o.row)
	}
	o.Command(command.NavUp, Invocation{Repeat: 1})
	if o.row != 1 {
		t.Fatalf("row after nav up = %d, want 1", o.row)
	}
}

func TestOfficeActionDetailRendersAssignedPatentsDocumentsAndWorklogHint(t *testing.T) {
	o := NewOfficeActionDetail(nil, render.NewTheme(), domain.OfficeAction{ID: "oa-1", Name: "Final Rejection"})
	o.loading = false
	o.assigned = []proto.OfficeActionAssignedPatent{
		{Number: domain.MustParsePatentNumber("US7654321"), Title: "Widget system", Status: domain.OAReviewToReview},
	}
	o.documents = []domain.MatterDocument{
		{ID: "doc-1", OfficeActionIDs: []string{"oa-1"}, Kind: domain.MatterDocOA, Stage: domain.StageReceived, DisplayName: "Examiner action.pdf"},
	}

	out := o.View(120, 24)
	for _, want := range []string{"ASSIGNED PATENTS (1)", "US7,654,321", "Widget system", "DOCUMENTS (1)", "Examiner action.pdf", "review worklog"} {
		if !strings.Contains(out, want) {
			t.Fatalf("detail view missing %q:\n%s", want, out)
		}
	}
}

func TestOfficeActionDetailEnterOnDocumentRowOpensDocuments(t *testing.T) {
	o := NewOfficeActionDetail(nil, render.NewTheme(), domain.OfficeAction{ID: "oa-1", Name: "Final Rejection"})
	o.loading = false
	o.documents = []domain.MatterDocument{{ID: "doc-1", DisplayName: "Examiner action.pdf"}}
	o.row = 9 // first document row: nine metadata rows, no patents

	_, cmd, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled || cmd == nil {
		t.Fatalf("enter on document row not handled (handled=%v cmd=%v)", handled, cmd)
	}
	if _, ok := cmd().(OpenOfficeActionDocumentsMsg); !ok {
		t.Fatalf("enter on document row emitted %T, want OpenOfficeActionDocumentsMsg", cmd())
	}
}

func TestOfficeActionsTableRenders(t *testing.T) {
	o := NewOfficeActions(nil, render.NewTheme(), "p1")
	o, _ = asOfficeActions(o.Update(officeActionsLoadedMsg{
		items: []domain.OfficeAction{
			{ID: "oa-1", Name: "Final Rejection", Examiner: "Menefee", Type: domain.OANonFinal,
				MailDate: time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC), Status: domain.OAStatusOpen,
				StatusChangedAt: time.Now().Add(-48 * time.Hour)},
		},
		billable: map[string]int{"oa-1": 222}, // 3:42 of validated time
	}))
	out := o.View(120, 20)
	// The row carries the name, examiner, status-with-age, and billable worklog.
	for _, want := range []string{"NAME", "WORKLOG", "Final Rejection", "Menefee", "open · 2d", "3:42"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table view missing %q:\n%s", want, out)
		}
	}
}

func asOfficeActions(p Pane, cmd interface{}) (*OfficeActions, interface{}) {
	return p.(*OfficeActions), cmd
}

func TestOfficeActionsCtrlNOpensNotesEditor(t *testing.T) {
	o := NewOfficeActions(nil, render.NewTheme(), "p1")
	o, _ = asOfficeActions(o.Update(officeActionsLoadedMsg{
		items: []domain.OfficeAction{{ID: "oa-1", Name: "Final Rejection"}},
	}))
	_, cmd, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	if !handled || cmd == nil {
		t.Fatalf("ctrl+n not handled (handled=%v cmd=%v)", handled, cmd)
	}
	if _, ok := cmd().(OpenOfficeActionEditorMsg); !ok {
		t.Fatalf("ctrl+n did not emit OpenOfficeActionEditorMsg")
	}
}

func TestOfficeActionsTagOpensSharedOverlay(t *testing.T) {
	o := NewOfficeActions(nil, render.NewTheme(), "p1")
	o, _ = asOfficeActions(o.Update(officeActionsLoadedMsg{
		items: []domain.OfficeAction{{ID: "oa-1", Name: "Final Rejection"}},
	}))

	// 't' opens the shared tag-manager overlay (same as patents), emitted as a
	// message for the app to handle — no inline prompt.
	_, cmd, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if !handled || cmd == nil {
		t.Fatalf("'t' not handled or emitted no command (handled=%v)", handled)
	}
	msg, ok := cmd().(OpenOfficeActionTagOverlayMsg)
	if !ok {
		t.Fatalf("'t' did not emit OpenOfficeActionTagOverlayMsg, got %T", cmd())
	}
	if msg.OA.ID != "oa-1" {
		t.Fatalf("tag overlay opened for %q, want oa-1", msg.OA.ID)
	}
}

func TestOfficeActionsSortingAndFocus(t *testing.T) {
	o := NewOfficeActions(nil, render.NewTheme(), "p1")
	o, _ = asOfficeActions(o.Update(officeActionsLoadedMsg{
		items: []domain.OfficeAction{
			{ID: "oa-1", Name: "Final Rejection", Examiner: "Menefee", Type: domain.OAFinal,
				MailDate: time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC), Status: domain.OAStatusOpen,
				ResponseDue: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)},
			{ID: "oa-2", Name: "Advisory Action", Examiner: "Alonzo", Type: domain.OAAdvisory,
				MailDate: time.Date(2026, 2, 9, 0, 0, 0, 0, time.UTC), Status: domain.OAStatusResponded,
				ResponseDue: time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC)},
		},
		billable: map[string]int{"oa-1": 100, "oa-2": 200},
	}))

	// Initial active sort is empty, so sorted by default (original order: oa-1, then oa-2)
	if o.items[0].ID != "oa-1" || o.items[1].ID != "oa-2" {
		t.Fatalf("unexpected initial order: %v", o.items)
	}

	// Focus next column: from -1 (none) to 0 (name)
	o.focusNext()
	if o.table.FocusedColIdx != 0 {
		t.Fatalf("expected FocusedColIdx to be 0 after focusNext, got %d", o.table.FocusedColIdx)
	}

	// Sort by name
	o.applySort()
	if o.table.ActiveSort != "name" || !o.table.SortAscending {
		t.Fatalf("expected ActiveSort to be name, SortAscending true; got ActiveSort=%q, ascending=%v", o.table.ActiveSort, o.table.SortAscending)
	}
	// "Advisory Action" (oa-2) should come before "Final Rejection" (oa-1)
	if o.items[0].ID != "oa-2" || o.items[1].ID != "oa-1" {
		t.Fatalf("expected oa-2 then oa-1 when sorted by name asc, got %v", o.items)
	}

	// Sort by name again to toggle descending
	o.applySort()
	if o.table.ActiveSort != "name" || o.table.SortAscending {
		t.Fatalf("expected ActiveSort to be name, SortAscending false; got ActiveSort=%q, ascending=%v", o.table.ActiveSort, o.table.SortAscending)
	}
	// Descending: "Final Rejection" (oa-1) before "Advisory Action" (oa-2)
	if o.items[0].ID != "oa-1" || o.items[1].ID != "oa-2" {
		t.Fatalf("expected oa-1 then oa-2 when sorted by name desc, got %v", o.items)
	}

	// Focus next column (1: type)
	o.focusNext()
	if o.table.FocusedColIdx != 1 {
		t.Fatalf("expected FocusedColIdx to be 1 after focusNext, got %d", o.table.FocusedColIdx)
	}

	// Sort by type
	o.applySort()
	// Advisory (oa-2) before Final (oa-1)
	if o.items[0].ID != "oa-2" {
		t.Fatalf("expected oa-2 to be first when sorted by type asc")
	}

	// Focus next column (2: examiner)
	o.focusNext()
	// Sort by examiner
	o.applySort()
	// Alonzo (oa-2) before Menefee (oa-1)
	if o.items[0].ID != "oa-2" {
		t.Fatalf("expected oa-2 to be first when sorted by examiner asc")
	}

	// Focus next column (3: due)
	o.focusNext()
	// Sort by response due date
	o.applySort()
	// 2026-05-09 (oa-2) before 2026-06-09 (oa-1)
	if o.items[0].ID != "oa-2" {
		t.Fatalf("expected oa-2 to be first when sorted by due date asc")
	}

	// Focus next column (4: status)
	o.focusNext()
	// Sort by status
	o.applySort()
	// Open (oa-1) before Responded (oa-2)
	if o.items[0].ID != "oa-1" {
		t.Fatalf("expected oa-1 to be first when sorted by status asc")
	}

	// Focus next column (5: tags)
	o.focusNext()
	if o.table.FocusedColIdx != 5 {
		t.Fatalf("expected FocusedColIdx to be 5 (tags) after focusNext, got %d", o.table.FocusedColIdx)
	}

	// Focus next column (6: worklog)
	o.focusNext()
	// Sort by worklog
	o.applySort()
	// 100s (oa-1) before 200s (oa-2)
	if o.items[0].ID != "oa-1" {
		t.Fatalf("expected oa-1 to be first when sorted by worklog asc")
	}
}
