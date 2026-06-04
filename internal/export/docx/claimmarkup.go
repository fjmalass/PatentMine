package docx

import "strings"

// amendmentRuns renders the transition from base to amended claim text as a run
// sequence: unchanged words plain, inserted words underlined, deleted words
// struck through — the MPEP 714 markup convention. The diff is a deterministic
// word-level longest-common-subsequence, computed here in code rather than by a
// model, so the legal markup is always correct regardless of how the amended
// text was authored.
//
// When base is empty the amended text is returned as a single plain run (a new
// or original claim carries no markup).
func amendmentRuns(base, amended string) []Run {
	if strings.TrimSpace(base) == "" {
		return []Run{{Text: amended}}
	}
	oldWords := splitWords(base)
	newWords := splitWords(amended)
	ops := diffWords(oldWords, newWords)

	var runs []Run
	for _, op := range ops {
		switch op.kind {
		case opEqual:
			runs = append(runs, Run{Text: op.text})
		case opInsert:
			runs = append(runs, Run{Text: op.text, Underline: true})
		case opDelete:
			runs = append(runs, Run{Text: op.text, Strike: true})
		}
	}
	return coalesceRuns(runs)
}

// splitWords breaks text into tokens that each include their trailing
// whitespace, so reassembling the tokens reproduces the spacing exactly.
func splitWords(s string) []string {
	var out []string
	var cur strings.Builder
	inWord := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			cur.WriteRune(r)
			inWord = false
			continue
		}
		if !inWord && cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
		cur.WriteRune(r)
		inWord = true
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

type opKind int

const (
	opEqual opKind = iota
	opInsert
	opDelete
)

type diffOp struct {
	kind opKind
	text string
}

// diffWords computes a word-level diff via the classic LCS dynamic program.
// Token counts in a claim are small (tens of words), so the O(n*m) table is
// inexpensive and the result is stable and easy to reason about.
func diffWords(a, b []string) []diffOp {
	n, m := len(a), len(b)
	// lcs[i][j] = length of the LCS of a[i:] and b[j:].
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if equalToken(a[i], b[j]) {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		if equalToken(a[i], b[j]) {
			ops = append(ops, diffOp{opEqual, b[j]})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			ops = append(ops, diffOp{opDelete, a[i]})
			i++
		} else {
			ops = append(ops, diffOp{opInsert, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{opDelete, a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{opInsert, b[j]})
	}
	return ops
}

// equalToken compares two whitespace-bearing tokens by their non-space content,
// so a word that only changed the whitespace around it is still "equal".
func equalToken(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b)
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
