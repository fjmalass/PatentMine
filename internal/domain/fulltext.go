package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// DisclosureLocator is the locator prefix for description paragraphs, e.g.
// "Disclosure ¶0045". Centralised so the viewer and the full-text search format
// locators identically.
const DisclosureLocator = "Disclosure"

// ClaimSection is one numbered claim of a patent's full text.
type ClaimSection struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

// Locator returns the section locator for the claim, e.g. "Claim 5".
func (c ClaimSection) Locator() string { return fmt.Sprintf("Claim %d", c.Number) }

// DescriptionParagraph is one numbered paragraph of a patent's disclosure.
// Number holds the source paragraph tag (e.g. "0045"); it may be empty when
// the source provides no paragraph numbering.
type DescriptionParagraph struct {
	Number string `json:"number"`
	Text   string `json:"text"`
}

// Locator returns the section locator for the paragraph at the given index,
// e.g. "Disclosure ¶0045" (or a positional "Disclosure ¶3" when unnumbered).
func (p DescriptionParagraph) Locator(index int) string {
	if p.Number != "" {
		return DisclosureLocator + " ¶" + p.Number
	}
	return fmt.Sprintf("%s ¶%d", DisclosureLocator, index+1)
}

// FullText holds the complete claims and disclosure text of a patent, fetched
// on-demand from the web rather than stored in the database.
type FullText struct {
	Number     PatentNumber           `json:"number"`
	Claims     []ClaimSection         `json:"claims"`
	Paragraphs []DescriptionParagraph `json:"paragraphs"`
}

// FullTextFromGrantBody projects a parsed USPTO body (claims, abstract,
// description) into the FullText shape that both the viewer pane and the
// full-text search render from. The abstract becomes the first disclosure
// paragraph; the description is split on blank lines, lifting a leading
// "[0001]" tag into the paragraph number when present.
func FullTextFromGrantBody(number PatentNumber, b USPTOGrantBody) FullText {
	full := FullText{Number: number}
	for _, c := range b.Claims {
		num := 0
		if n, err := strconv.Atoi(strings.TrimLeft(c.Number, "0")); err == nil {
			num = n
		}
		full.Claims = append(full.Claims, ClaimSection{Number: num, Text: c.Text})
	}
	if b.AbstractText != "" {
		full.Paragraphs = append(full.Paragraphs, DescriptionParagraph{
			Number: "Abstract",
			Text:   strings.TrimSpace(b.AbstractText),
		})
	}
	if d := strings.TrimSpace(b.DescriptionText); d != "" {
		for _, part := range strings.Split(d, "\n\n") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			num := ""
			ptext := part
			if strings.HasPrefix(part, "[") {
				if idx := strings.Index(part, "]"); idx > 1 {
					num = part[1:idx]
					ptext = strings.TrimSpace(part[idx+1:])
				}
			}
			full.Paragraphs = append(full.Paragraphs, DescriptionParagraph{Number: num, Text: ptext})
		}
	}
	return full
}

// String returns a compact summary of the full text.
func (f FullText) String() string {
	if len(f.Claims) == 0 && len(f.Paragraphs) == 0 {
		return "no full text"
	}
	return fmt.Sprintf("%d claims, %d paragraphs", len(f.Claims), len(f.Paragraphs))
}
