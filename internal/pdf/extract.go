// Package pdf extracts plain text from PDF documents so imported office actions,
// responses, and references can be read in the TUI and copied into a drafted
// response. It wraps github.com/ledongthuc/pdf — a pure-Go reader (no native
// deps, no OCR) — behind a single ExtractText entry point that cleans up the
// raw, run-together text the extractor emits.
//
// Scope: digital (text-layer) PDFs only. A scanned/image-only PDF carries no
// text layer, so ExtractText returns little or nothing and the caller falls back
// to manual paste; OCR is intentionally out of scope.
package pdf

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"
)

var (
	// hyphenBreak joins a word split across a line break ("exam-\nple" →
	// "example"), a common artifact of justified PDF text.
	hyphenBreak = regexp.MustCompile(`(\p{L})-\n(\p{L})`)
	// manyBlankLines collapses runs of blank lines down to a single separator.
	manyBlankLines = regexp.MustCompile(`\n{3,}`)
	// trailingSpaces trims spaces/tabs sitting at the end of a line.
	trailingSpaces = regexp.MustCompile(`[ \t]+\n`)
	// runsOfSpaces collapses spaces/tabs within a line to a single space (a lone
	// tab becomes a space too).
	runsOfSpaces = regexp.MustCompile(`[ \t]+`)
)

// ExtractText reads path and returns its plain text, cleaned of the most common
// extraction artifacts (broken hyphenation, excess whitespace). It never panics:
// the underlying reader can panic on a malformed PDF, so a recovered panic is
// returned as an error rather than crashing the daemon. An empty result with a
// nil error means the PDF carried no extractable text (e.g. a scanned image).
func ExtractText(path string) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			text, err = "", fmt.Errorf("pdf: extract %s: %v", path, r)
		}
	}()

	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("pdf: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	reader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("pdf: read %s: %w", path, err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		return "", fmt.Errorf("pdf: read %s: %w", path, err)
	}
	return clean(buf.String()), nil
}

// clean tidies the raw extracted text: it rejoins hyphenated line breaks,
// collapses repeated spaces and blank lines, and trims trailing whitespace, so
// the reference pane reads cleanly and copied passages paste sensibly.
func clean(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = hyphenBreak.ReplaceAllString(s, "$1$2")
	s = runsOfSpaces.ReplaceAllString(s, " ")
	s = trailingSpaces.ReplaceAllString(s, "\n")
	s = manyBlankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
