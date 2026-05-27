package pane

import (
	"strings"
	"testing"

	"patentmine/internal/filterexpr"
)

func TestPatentFilterParseExpression(t *testing.T) {
	f := PatentFilter{}

	msg, err := f.parse([]string{"classification:S04*", "and", "not", "status:under_review"}, true)
	if err != nil {
		t.Fatalf("parse expression error: %v", err)
	}
	if msg != "filtering: class:S04* and not state:under_review" {
		t.Fatalf("parse msg = %q", msg)
	}
	if f.Expression != "class:S04* and not state:under_review" {
		t.Fatalf("Expression = %q", f.Expression)
	}
	if got := strings.Join(f.Labels(), " | "); got != "class:S04* and not state:under_review" {
		t.Fatalf("Labels = %q", got)
	}
}

func TestPatentFilterParseClearAliases(t *testing.T) {
	f := PatentFilter{Search: "widget", Expression: "tag:prior_art", expr: filterexpr.TermExpr{Field: filterexpr.FieldTag, Value: "prior_art"}}

	msg, err := f.parse([]string{"reset"}, true)
	if err != nil {
		t.Fatalf("parse reset error: %v", err)
	}
	if msg != "filters cleared" {
		t.Fatalf("parse reset msg = %q", msg)
	}
	if f != (PatentFilter{}) {
		t.Fatalf("filter after reset = %+v, want zero value", f)
	}
}

func TestPatentFilterRejectsLegacySyntax(t *testing.T) {
	f := PatentFilter{}
	_, err := f.parse([]string{"state", "active"}, true)
	if err == nil {
		t.Fatal("expected error for legacy syntax")
	}
	if err.Error() != filterexpr.LegacySyntaxHint {
		t.Fatalf("legacy syntax error = %q", err)
	}
}

func TestPatentFilterRequiresProjectForState(t *testing.T) {
	f := PatentFilter{}
	_, err := f.parse([]string{"not", "state:under_review"}, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), "state filters require an active project"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestPatentFilterReplaceClassificationsPreservesOtherTerms(t *testing.T) {
	f := PatentFilter{}
	if _, err := f.parse([]string{"tag:prior_art", "and", "class:G06F"}, true); err != nil {
		t.Fatalf("parse setup error: %v", err)
	}
	if err := f.replaceClassifications([]string{"A61K", "B64C27/00"}); err != nil {
		t.Fatalf("replace classifications error: %v", err)
	}
	if got, want := f.Expression, "tag:prior_art and class:A61K and class:B64C27/00"; got != want {
		t.Fatalf("Expression = %q, want %q", got, want)
	}
}

func TestPatentFilterParseDefaultAlias(t *testing.T) {
	f := PatentFilter{}
	msg, err := f.parse([]string{"default"}, true)
	if err != nil {
		t.Fatalf("parse default error: %v", err)
	}
	if msg != "filtering: fetch_state:cached" {
		t.Fatalf("parse default msg = %q, want filtering: fetch_state:cached", msg)
	}
	if f.Expression != "fetch_state:cached" {
		t.Fatalf("filter Expression = %q, want fetch_state:cached", f.Expression)
	}
}

