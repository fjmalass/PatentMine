package domain

import (
	"strings"
	"time"
)

// ProvisionalCoverSheetInventor is one inventor named on a provisional cover sheet
// (PTO/SB/16). The official form has room for five; further inventors are
// declared by attaching numbered continuation sheets (AdditionalInventorSheets).
type ProvisionalCoverSheetInventor struct {
	GivenName  string `json:"given_name"`  // first and middle (if any)
	FamilyName string `json:"family_name"` // family name or surname
	Residence  string `json:"residence"`   // city and either state or foreign country
}

// Named reports whether the inventor carries any name text.
func (i ProvisionalCoverSheetInventor) Named() bool {
	return strings.TrimSpace(i.GivenName) != "" || strings.TrimSpace(i.FamilyName) != ""
}

// ProvisionalCoverSheetGovInterest classifies the federally-sponsored-research status the
// SB/16 asks for: not government-made, made by a U.S. government agency, or made
// under a contract with one.
type ProvisionalCoverSheetGovInterest string

const (
	GovInterestNone     ProvisionalCoverSheetGovInterest = "none"
	GovInterestAgency   ProvisionalCoverSheetGovInterest = "agency"
	GovInterestContract ProvisionalCoverSheetGovInterest = "contract"
)

// Valid reports whether the ProvisionalCoverSheetGovInterest is a known value (empty is
// treated as "none" by the renderer, so it is accepted too).
func (g ProvisionalCoverSheetGovInterest) Valid() bool {
	switch g {
	case "", GovInterestNone, GovInterestAgency, GovInterestContract:
		return true
	default:
		return false
	}
}

// ProvisionalCoverSheet is the structured data behind a USPTO Provisional Application for
// Patent Cover Sheet (PTO/SB/16). It is one record per project, stored so it can
// be searched and re-rendered into the official fillable PDF on approval — the
// same view/fill/approve/save pattern the IDS bundle uses. The field set mirrors
// the form: invention title, up to five inventors, the correspondence block,
// the enclosure checkboxes (with their counts), entity status, the fee and how
// it is paid, the federal-interest statement, and the signature block.
type ProvisionalCoverSheet struct {
	ID        string                          `json:"id"`
	Project   ProjectID                       `json:"project"`
	Title     string                          `json:"title"`
	Inventors []ProvisionalCoverSheetInventor `json:"inventors,omitempty"`
	// AdditionalInventorSheets is the number of separately numbered continuation
	// sheets attached to name inventors beyond the five the form holds.
	AdditionalInventorSheets int `json:"additional_inventor_sheets,omitempty"`

	// Correspondence — either a Customer Number or a firm/individual address.
	DirectToCustomerNumber bool   `json:"direct_to_customer_number,omitempty"`
	CustomerNumber         string `json:"customer_number,omitempty"`
	DirectToFirm           bool   `json:"direct_to_firm,omitempty"`
	FirmName               string `json:"firm_name,omitempty"`
	Address                string `json:"address,omitempty"`
	City                   string `json:"city,omitempty"`
	State                  string `json:"state,omitempty"`
	Country                string `json:"country,omitempty"`
	Zip                    string `json:"zip,omitempty"`
	Telephone              string `json:"telephone,omitempty"`
	Email                  string `json:"email,omitempty"`

	// Enclosures — each checkbox plus the count the form prints beside it.
	SpecEnclosed     bool   `json:"spec_enclosed,omitempty"`
	SpecPages        int    `json:"spec_pages,omitempty"`
	DrawingsEnclosed bool   `json:"drawings_enclosed,omitempty"`
	DrawingSheets    int    `json:"drawing_sheets,omitempty"`
	ADSEnclosed      bool   `json:"ads_enclosed,omitempty"`
	CDsEnclosed      bool   `json:"cds_enclosed,omitempty"`
	CDCount          int    `json:"cd_count,omitempty"`
	OtherEnclosed    bool   `json:"other_enclosed,omitempty"`
	OtherSpecify     string `json:"other_specify,omitempty"`

	// Entity status (mutually exclusive in practice; the renderer checks whichever
	// is set). Neither set means an undiscounted (large-entity) filing.
	SmallEntity bool `json:"small_entity,omitempty"`
	MicroEntity bool `json:"micro_entity,omitempty"`

	// Fee and how it is paid.
	TotalFeeAmount string `json:"total_fee_amount,omitempty"`
	PayDeposit     bool   `json:"pay_deposit,omitempty"`
	DepositAccount string `json:"deposit_account,omitempty"`
	PayCheck       bool   `json:"pay_check,omitempty"`
	PayCreditCard  bool   `json:"pay_credit_card,omitempty"`

	// Federal-interest statement.
	GovInterest    ProvisionalCoverSheetGovInterest `json:"gov_interest,omitempty"`
	AgencyName     string                           `json:"agency_name,omitempty"`
	ContractNumber string                           `json:"contract_number,omitempty"`

	// Signature block.
	SignerName      string    `json:"signer_name,omitempty"`
	RegistrationNo  string    `json:"registration_no,omitempty"`
	SignatureText   string    `json:"signature_text,omitempty"`
	SignatureDate   time.Time `json:"signature_date,omitempty"`
	SignerTelephone string    `json:"signer_telephone,omitempty"`
	DocketNumber    string    `json:"docket_number,omitempty"`

	PriorityMailLabel string `json:"priority_mail_label,omitempty"`

	// Approved marks the sheet reviewed and ready to render to the final PDF; the
	// engine stamps it when the attorney approves (see RenderProvisionalCoverSheetPDF).
	Approved  bool      `json:"approved,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NamedInventors returns only the inventors that carry name text, dropping the
// blank rows a form edit may leave behind.
func (c ProvisionalCoverSheet) NamedInventors() []ProvisionalCoverSheetInventor {
	out := make([]ProvisionalCoverSheetInventor, 0, len(c.Inventors))
	for _, inv := range c.Inventors {
		if inv.Named() {
			out = append(out, inv)
		}
	}
	return out
}

// Missing returns the human-readable names of the fields a complete provisional
// cover sheet requires but this one lacks — the basis for the "cannot submit"
// block. An empty result means the required minimum is present. It deliberately
// checks only what the USPTO strictly needs on the SB/16: a title, at least one
// named inventor, and a signature.
func (c ProvisionalCoverSheet) Missing() []string {
	var missing []string
	if strings.TrimSpace(c.Title) == "" {
		missing = append(missing, "invention title")
	}
	if len(c.NamedInventors()) == 0 {
		missing = append(missing, "at least one inventor")
	}
	if strings.TrimSpace(c.SignatureText) == "" && strings.TrimSpace(c.SignerName) == "" {
		missing = append(missing, "signature")
	}
	return missing
}

// Complete reports whether the cover sheet carries the required minimum.
func (c ProvisionalCoverSheet) Complete() bool { return len(c.Missing()) == 0 }
