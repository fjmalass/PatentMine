package sqlite

import (
	"testing"

	"patentmine/internal/domain"
)

func TestAllPatentSortColumnsHaveSQLExpressions(t *testing.T) {
	for _, col := range domain.PatentSortColumns {
		if expr := sortExpr(col, domain.SortOrderAsc); expr == "" {
			t.Fatalf("sort column %q missing SQL sort expression", col)
		}
	}
}
