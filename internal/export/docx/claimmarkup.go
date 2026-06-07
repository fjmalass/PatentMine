package docx

import (
	"strings"

	"patentmine/internal/textdiff"
)

// amendmentRuns renders the transition from base to amended claim text as a run
// sequence: unchanged words plain, inserted words underlined, deleted words
// struck through — the MPEP 714 markup convention. The word-level diff is the
// shared textdiff.Words (computed in code, not by a model), so the legal markup
// is always correct regardless of how the amended text was authored.
//
// When base is empty the amended text is returned as a single plain run (a new
// or original claim carries no markup).
func amendmentRuns(base, amended string) []Run {
	if strings.TrimSpace(base) == "" {
		return []Run{{Text: amended}}
	}
	var runs []Run
	for _, op := range textdiff.Words(base, amended) {
		switch op.Kind {
		case textdiff.OpEqual:
			runs = append(runs, Run{Text: op.Text})
		case textdiff.OpInsert:
			runs = append(runs, Run{Text: op.Text, Underline: true})
		case textdiff.OpDelete:
			runs = append(runs, Run{Text: op.Text, Strike: true})
		}
	}
	return coalesceRuns(runs)
}

// coalesceRuns merges adjacent runs that share formatting, so the output XML is
// compact and reads as whole inserted/deleted phrases rather than word-by-word.
func coalesceRuns(in []Run) []Run {
	if len(in) == 0 {
		return in
	}
	out := []Run{in[0]}
	for _, r := range in[1:] {
		last := &out[len(out)-1]
		if last.Underline == r.Underline && last.Strike == r.Strike &&
			last.Bold == r.Bold && last.Italic == r.Italic {
			last.Text += r.Text
			continue
		}
		out = append(out, r)
	}
	return out
}
