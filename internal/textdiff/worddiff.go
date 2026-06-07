// Package textdiff computes a deterministic word-level diff between two strings.
// It is the shared engine behind MPEP 714 amendment markup: the claim renderers
// (docx and markdown) call Words to learn which words were inserted, deleted, or
// left unchanged when a claim was amended, and decorate each accordingly. The
// diff is computed in code — never by a model — so the legal markup is always
// correct regardless of how the amended text was authored.
package textdiff

import "strings"

// OpKind classifies one span of a word diff.
type OpKind int

const (
	OpEqual  OpKind = iota // present unchanged in both base and amended
	OpInsert               // present only in amended (an insertion)
	OpDelete               // present only in base (a deletion)
)

// Op is one span of a word diff: a run of text and what happened to it. Text
// carries its trailing whitespace so concatenating every Op's Text reproduces
// the amended/base spacing exactly.
type Op struct {
	Kind OpKind
	Text string
}

// Words diffs base against amended at word granularity and returns the ordered
// edit operations. The token counts in a claim are small (tens of words), so the
// O(n*m) longest-common-subsequence table is inexpensive and the result is
// stable and easy to reason about.
func Words(base, amended string) []Op {
	oldWords := splitWords(base)
	newWords := splitWords(amended)
	return diffWords(oldWords, newWords)
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

// diffWords computes a word-level diff via the classic LCS dynamic program.
func diffWords(a, b []string) []Op {
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

	var ops []Op
	i, j := 0, 0
	for i < n && j < m {
		if equalToken(a[i], b[j]) {
			ops = append(ops, Op{OpEqual, b[j]})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			ops = append(ops, Op{OpDelete, a[i]})
			i++
		} else {
			ops = append(ops, Op{OpInsert, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, Op{OpDelete, a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, Op{OpInsert, b[j]})
	}
	return ops
}

// equalToken compares two whitespace-bearing tokens by their non-space content,
// so a word that only changed the whitespace around it is still "equal".
func equalToken(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}
