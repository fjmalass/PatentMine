package domain

import (
	"testing"
	"time"
)

func TestUSMaintenanceDeadlines(t *testing.T) {
	grant := time.Date(2024, 2, 4, 0, 0, 0, 0, time.UTC)
	ds := USMaintenanceDeadlines("US10000000B2", grant)
	if len(ds) != 3 {
		t.Fatalf("want 3 maintenance deadlines, got %d", len(ds))
	}
	wantDue := []time.Time{
		grant.AddDate(0, 42, 0),  // 3.5 yr
		grant.AddDate(0, 90, 0),  // 7.5 yr
		grant.AddDate(0, 138, 0), // 11.5 yr
	}
	for i, d := range ds {
		if d.Kind != DeadlineMaintenanceFee {
			t.Errorf("deadline %d kind = %q, want maintenance_fee", i, d.Kind)
		}
		if !d.DueDate.Equal(wantDue[i]) {
			t.Errorf("deadline %d due = %v, want %v", i, d.DueDate, wantDue[i])
		}
		// Window opens 6 months before, grace ends 6 months after.
		if !d.WindowOpens.Equal(wantDue[i].AddDate(0, -6, 0)) {
			t.Errorf("deadline %d window = %v", i, d.WindowOpens)
		}
		if !d.GraceEnds.Equal(wantDue[i].AddDate(0, 6, 0)) {
			t.Errorf("deadline %d grace = %v", i, d.GraceEnds)
		}
		if d.PatentNumber != "US10000000B2" {
			t.Errorf("deadline %d patent = %q", i, d.PatentNumber)
		}
	}
}

func TestUSMaintenanceDeadlinesZeroGrant(t *testing.T) {
	if ds := USMaintenanceDeadlines("US1", time.Time{}); ds != nil {
		t.Fatalf("zero grant date should yield no deadlines, got %d", len(ds))
	}
}

func TestDaysUntilDue(t *testing.T) {
	d := Deadline{DueDate: time.Now().Add(48 * time.Hour)}
	if got := d.DaysUntilDue(); got < 1 || got > 2 {
		t.Fatalf("DaysUntilDue ~2, got %d", got)
	}
	overdue := Deadline{DueDate: time.Now().Add(-72 * time.Hour)}
	if got := overdue.DaysUntilDue(); got >= 0 {
		t.Fatalf("overdue DaysUntilDue should be negative, got %d", got)
	}
}
