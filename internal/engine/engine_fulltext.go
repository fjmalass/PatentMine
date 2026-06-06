package engine

import (
	"context"
	"strings"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

// fullTextSnippetPad is how many characters of context to keep on each side of
// a match when building a result snippet.
const fullTextSnippetPad = 48

// FullTextSearch scans the locally-stored USPTO bodies of the given patents for
// query (case-insensitive substring) and returns every matching section. It
// never fetches over the network: patents whose body has not been ingested are
// reported in Missing so the caller can distinguish "no hits" from "nothing to
// search". Bodies are projected with the same helper the viewer renders from,
// so the returned locators address the same sections the pane shows.
func (e *Engine) FullTextSearch(ctx context.Context, numbers []domain.PatentNumber, query string) (proto.FullTextSearchResult, error) {
	res := proto.FullTextSearchResult{Query: strings.TrimSpace(query)}
	lower := strings.ToLower(res.Query)
	if lower == "" {
		return res, nil
	}
	for _, n := range numbers {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		body, _, kind, present, err := e.USPTOGrantBody(ctx, n, "")
		if err != nil || !present {
			// A load error here means we cannot search this patent; treat it the
			// same as a missing body rather than failing the whole batch.
			res.Missing = append(res.Missing, n)
			continue
		}
		res.Scanned++
		full := domain.FullTextFromGrantBody(n, body)
		res.Matches = append(res.Matches, searchFullText(n, kind, full, lower)...)
	}
	return res, nil
}

// searchFullText returns one match per occurrence of lowerQuery across the
// claims and disclosure paragraphs of full, tagged with the section locator.
func searchFullText(n domain.PatentNumber, kind proto.USPTOXMLKind, full domain.FullText, lowerQuery string) []proto.FullTextSearchMatch {
	var out []proto.FullTextSearchMatch
	collect := func(locator, text string) {
		for occ, snip := range fullTextSnippets(text, lowerQuery) {
			out = append(out, proto.FullTextSearchMatch{
				Number: n, Kind: kind, Locator: locator, Occurrence: occ, Snippet: snip,
			})
		}
	}
	for _, c := range full.Claims {
		collect(c.Locator(), c.Text)
	}
	for i, p := range full.Paragraphs {
		collect(p.Locator(i), p.Text)
	}
	return out
}

// fullTextSnippets returns a context window around each occurrence of
// lowerQuery in text (matched case-insensitively, rendered from the original
// text). Whitespace is collapsed so a snippet reads as one line.
func fullTextSnippets(text, lowerQuery string) []string {
	lower := strings.ToLower(text)
	var snippets []string
	from := 0
	for {
		rel := strings.Index(lower[from:], lowerQuery)
		if rel < 0 {
			break
		}
		at := from + rel
		start := max(at-fullTextSnippetPad, 0)
		end := min(at+len(lowerQuery)+fullTextSnippetPad, len(text))
		snippet := strings.Join(strings.Fields(text[start:end]), " ")
		if start > 0 {
			snippet = "…" + snippet
		}
		if end < len(text) {
			snippet += "…"
		}
		snippets = append(snippets, snippet)
		from = at + len(lowerQuery)
	}
	return snippets
}
