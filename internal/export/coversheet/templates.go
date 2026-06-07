// Package coversheet renders a domain.ProvisionalCoverSheet into the official USPTO
// Provisional Application for Patent Cover Sheet (PTO/SB/16) by filling the
// form's AcroForm fields with pdfcpu — the same fill-the-official-PDF approach
// the IDS bundle uses for PTO/SB/08a. The blank form is embedded so rendering
// never depends on a file being present at runtime; a caller may override it
// with a template from the configured templates directory.
package coversheet

import _ "embed"

//go:embed templates/sb0016.pdf
var templateSB0016 []byte

const (
	// ProvisionalBlankFilename is the canonical name of a blank PTO/SB/16 a user may
	// drop under <templates>/officeactions/provisional/ to override the embedded
	// form. It must keep the official AcroForm field names to fill correctly.
	ProvisionalBlankFilename = "coversheet-sb0016.pdf"
	// ProvisionalRenderedFilename is the stable name of the filled, rendered cover-sheet PDF.
	ProvisionalRenderedFilename = "coversheet-sb16.pdf"
	// ProvisionalDocumentName is the matter-document label for the rendered sheet.
	ProvisionalDocumentName = "Provisional Cover Sheet (SB/16)"
)

// ProvisionalTemplate returns the embedded blank PTO/SB/16 form bytes.
func ProvisionalTemplate() []byte { return templateSB0016 }
