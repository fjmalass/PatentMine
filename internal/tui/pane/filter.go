package pane

import (
	"fmt"
	"slices"
	"strings"

	"patentmine/internal/filterexpr"
	"patentmine/internal/tui/render"
)

// PatentFilter holds all filters applied to a patent list or relation view.
type PatentFilter struct {
	Search     string
	Expression string
	expr       filterexpr.Expr
}

const patentFilterClearName = "clear"

var patentFilterClearAliases = []string{"clear", "none", "default", "reset"}

// IsActive reports whether any filter is set.
func (f PatentFilter) IsActive() bool {
	return f.Search != "" || f.expr != nil || f.Expression != ""
}

// Labels returns human-readable filter labels for the current state.
func (f PatentFilter) Labels() []string {
	var labels []string
	if f.Search != "" {
		labels = append(labels, "search:"+f.Search)
	}
	if f.Expression != "" {
		labels = append(labels, f.Expression)
	}
	return labels
}

// filterBar renders a line showing active filters.
func (f PatentFilter) View(w int, theme render.Theme) string {
	labels := f.Labels()
	if len(labels) == 0 {
		return ""
	}
	s := " filters: " + strings.Join(labels, " · ")
	return theme.Dim.Render(render.Pad(s, w))
}

func (f *PatentFilter) parse(args []string, projectActive bool) (string, error) {
	if len(args) == 0 || isPatentFilterClearToken(args[0]) {
		*f = PatentFilter{}
		return "filters cleared", nil
	}
	expr, err := filterexpr.Parse(strings.Join(args, " "))
	if err != nil {
		return "", err
	}
	if err := filterexpr.ValidateProjectScope(expr, projectActive); err != nil {
		return "", err
	}
	f.setExpression(expr)
	return fmt.Sprintf("filtering: %s", f.Expression), nil
}

func (f *PatentFilter) setExpression(expr filterexpr.Expr) {
	if expr == nil {
		f.Expression = ""
		f.expr = nil
		return
	}
	f.Expression = filterexpr.CanonicalString(expr)
	f.expr = expr
}

func (f *PatentFilter) replaceClassifications(codes []string) error {
	base := filterexpr.RemoveField(f.expr, filterexpr.FieldClass)
	var classExpr filterexpr.Expr
	for _, code := range codes {
		term, err := filterexpr.ParseClassTerm(code)
		if err != nil {
			return err
		}
		classExpr = filterexpr.JoinAnd(classExpr, term)
	}
	f.setExpression(filterexpr.JoinAnd(base, classExpr))
	return nil
}

func isPatentFilterClearToken(s string) bool {
	return slices.Contains(patentFilterClearAliases, s)
}

func patentFilterUsage() string {
	return strings.Join([]string{patentFilterClearName, "tag:prior_art and not state:under_review", "ids_status:pending", "(tag:prior_art or tag:blocker) and state:under_review", "class:S04*"}, " | ")
}
