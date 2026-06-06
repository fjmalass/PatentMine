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

// fullTextStages are the document stages searched, newest first, so a granted
// patent's final text leads and the as-filed publication follows.
var fullTextStages = []proto.USPTOXMLKind{proto.USPTOXMLKindGrant, proto.USPTOXMLKindPGPub}

// FullTextSearch scans the locally-stored USPTO bodies of the given patents for
// query (case-insensitive substring) and returns every matching section. Each
// patent is searched at *both* the granted and the published (pgpub) stage when
// present, and every match is tagged with that stage's display number (the
// final "…B2" grant or the "…A1" publication), so the same number the detail
// view shows comes back — and so an office action can search either version.
//
// It never fetches over the network. Patents with no ingested body land in
// Missing; ingested patents with no hit land in NoMatch — together with the
// matched patents they account for every selected patent.
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
		patent, _ := e.Patent(ctx, n) // best-effort, for the per-stage display numbers
		anyPresent, patentMatches := false, 0
		for _, kind := range fullTextStages {
			body, _, _, present, err := e.USPTOGrantBody(ctx, n, kind)
			if err != nil || !present {
				continue
			}
			anyPresent = true
			display := stageDisplayNumber(patent, kind, n)
			full := domain.FullTextFromGrantBody(display, body)
			ms := searchFullText(display, kind, full, lower)
			patentMatches += len(ms)
			res.Matches = append(res.Matches, ms...)
		}
		switch {
		case !anyPresent:
			res.Missing = append(res.Missing, n)
		case patentMatches == 0:
			res.Scanned++
			res.NoMatch = append(res.NoMatch, n)
		default:
			res.Scanned++
		}
	}
	return res, nil
}

// stageDisplayNumber returns the patent's number for the given stage (the
// granted or published document), falling back to the searched number when the
// record carries no document for that stage.
func stageDisplayNumber(p domain.Patent, kind proto.USPTOXMLKind, fallback domain.PatentNumber) domain.PatentNumber {
	stage := domain.StagePublication
	if kind == proto.USPTOXMLKindGrant {
		stage = domain.StageGrant
	}
	if doc, ok := p.DocumentFor(stage); ok && !doc.Number.IsZero() {
		return doc.Number
	}
	return fallback
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
