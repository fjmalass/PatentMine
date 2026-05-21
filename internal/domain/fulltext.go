package domain

import "fmt"

// ClaimSection is one numbered claim of a patent's full text.
type ClaimSection struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

// DescriptionParagraph is one numbered paragraph of a patent's disclosure.
// Number holds the source paragraph tag (e.g. "0045"); it may be empty when
// the source provides no paragraph numbering.
type DescriptionParagraph struct {
	Number string `json:"number"`
	Text   string `json:"text"`
}

// FullText holds the complete claims and disclosure text of a patent, fetched
// on-demand from the web rather than stored in the database.
type FullText struct {
	Number     PatentNumber           `json:"number"`
	Claims     []ClaimSection         `json:"claims"`
	Paragraphs []DescriptionParagraph `json:"paragraphs"`
}

// String returns a compact summary of the full text.
func (f FullText) String() string {
	if len(f.Claims) == 0 && len(f.Paragraphs) == 0 {
		return "no full text"
	}
	return fmt.Sprintf("%d claims, %d paragraphs", len(f.Claims), len(f.Paragraphs))
}
