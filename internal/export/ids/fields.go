package ids

import "fmt"

// PTO/SB/08a layout: 20 US patent rows (5 cols each) followed by 6 foreign
// rows (5 text cols + 1 translation checkbox).
const (
	sb08aUSRows      = 20
	sb08aForeignRows = 6
)

// sb08aHeaderField returns the AcroForm field name for each header slot.
type sb08aHeader struct {
	AppNumber    string
	FilingDate   string
	Inventor     string
	ArtUnit      string
	Examiner     string
	DocketNumber string
	SheetN       string
	SheetOf      string
}

var sb08aHeaderFields = sb08aHeader{
	AppNumber:    "text3",
	FilingDate:   "text4",
	Inventor:     "text5",
	ArtUnit:      "text6",
	Examiner:     "text7",
	DocketNumber: "text8",
	SheetN:       "Text1",
	SheetOf:      "text2",
}

// sb08aUSRowFields returns the 5 text-field names for one US patent row
// (1-indexed). Layout: cite#, doc#, pub-date, patentee, pages.
func sb08aUSRowFields(row int) (cite, doc, date, patentee, pages string) {
	base := 9 + 5*(row-1)
	return field(base), field(base + 1), field(base + 2), field(base + 3), field(base + 4)
}

// sb08aForeignRowFields returns the 5 text-field names plus the translation
// checkbox name for one foreign row (1-indexed).
//
// The checkbox names in the official PDF are not strictly sequential — they
// reuse low ids the form designer left lying around. They are hardcoded here.
func sb08aForeignRowFields(row int) (cite, doc, date, patentee, pages, trans string) {
	// Rows have a 6-field stride. The checkbox replaces the would-be 6th
	// text slot, so the 5 text fields are at base..base+4.
	bases := []int{104, 110, 116, 122, 128, 134}
	checkboxes := []string{"box109", "box115", "box21", "box127", "box133", "box39"}
	if row < 1 || row > sb08aForeignRows {
		return "", "", "", "", "", ""
	}
	b := bases[row-1]
	return field(b), field(b + 1), field(b + 2), field(b + 3), field(b + 4), checkboxes[row-1]
}

func field(n int) string { return fmt.Sprintf("text%d", n) }

// PTO/SB/08c size-fee form field names.
const (
	sb08cAppNumber  = "Application or Patent No"
	sb08cInventor   = "First Named Inventor"
	sb08cArtUnit    = "Art Unit"
	sb08cFilingDate = "Filing Date"
	sb08cDocket     = "Attorney Docket Number"
	sb08cExaminer   = "Examiner"
	sb08cBoxNoFee   = "Check Box1"
	sb08cBoxTier1   = "Check Box2"
	sb08cBoxTier2   = "Check Box3"
	sb08cBoxTier3   = "Check Box4"
	sb08cFeeAmount  = "Fee amount to be charged"
	sb08cDepositNum = "Deposit Account Number"
	sb08cSignDate   = "Date"
	sb08cSignName   = "Name PrintTyped"
	sb08cRegNumber  = "Practitioner Registration Number if applicable"
	sb08cSignature  = "Signature"
)
