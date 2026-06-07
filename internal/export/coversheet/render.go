package coversheet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"

	"patentmine/internal/domain"
)

// form JSON shapes pdfcpu's FillForm accepts (page-1 text fields and checkboxes).
type textValue struct {
	Pages []int  `json:"pages"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type boxValue struct {
	Pages []int  `json:"pages"`
	Name  string `json:"name"`
	Value bool   `json:"value"`
}

type formPage struct {
	Textfields []textValue `json:"textfield,omitempty"`
	Checkboxes []boxValue  `json:"checkbox,omitempty"`
}

type formData struct {
	Forms []formPage `json:"forms"`
}

func (f *formPage) text(name, value string) {
	if name == "" || value == "" {
		return
	}
	f.Textfields = append(f.Textfields, textValue{Pages: []int{1}, Name: name, Value: value})
}

func (f *formPage) box(name string, on bool) {
	if name == "" || !on {
		return
	}
	f.Checkboxes = append(f.Checkboxes, boxValue{Pages: []int{1}, Name: name, Value: true})
}

// RenderProvisional fills the embedded PTO/SB/16 with a cover sheet's data and returns the
// completed PDF bytes. Pass a non-nil template to fill a caller-supplied blank
// (e.g. one loaded from the configured templates directory) instead of the
// embedded copy.
func RenderProvisional(cs domain.ProvisionalCoverSheet, template []byte) ([]byte, error) {
	if len(template) == 0 {
		template = templateSB0016
	}
	page := buildPage(cs)
	data, err := json.Marshal(formData{Forms: []formPage{page}})
	if err != nil {
		return nil, fmt.Errorf("coversheet: marshal form data: %w", err)
	}
	var out bytes.Buffer
	if err := api.FillForm(bytes.NewReader(template), bytes.NewReader(data), &out, nil); err != nil {
		return nil, fmt.Errorf("coversheet: fill form: %w", err)
	}
	return out.Bytes(), nil
}

// buildPage maps a ProvisionalCoverSheet onto the SB/16's page-1 fields.
func buildPage(cs domain.ProvisionalCoverSheet) formPage {
	var p formPage

	p.text(fieldTitle, strings.TrimSpace(cs.Title))

	inventors := cs.NamedInventors()
	for i, inv := range inventors {
		if i >= maxFormInventors {
			break
		}
		row := i + 1
		p.text(inventorGivenField(row), strings.TrimSpace(inv.GivenName))
		p.text(inventorFamilyField(row), strings.TrimSpace(inv.FamilyName))
		p.text(inventorResidenceField(row), strings.TrimSpace(inv.Residence))
	}
	// Inventors beyond the five printed rows are declared on continuation sheets.
	extra := cs.AdditionalInventorSheets
	if overflow := len(inventors) - maxFormInventors; extra == 0 && overflow > 0 {
		extra = overflow
	}
	p.text(fieldAdditionalSheets, countText(extra))

	// Correspondence.
	p.box(boxDirectToCustomerNumber, cs.DirectToCustomerNumber)
	p.text(fieldCustomerNumber, strings.TrimSpace(cs.CustomerNumber))
	p.box(boxDirectToFirm, cs.DirectToFirm)
	p.text(fieldFirmName, strings.TrimSpace(cs.FirmName))
	p.text(fieldAddress, strings.TrimSpace(cs.Address))
	p.text(fieldCity, strings.TrimSpace(cs.City))
	p.text(fieldState, strings.TrimSpace(cs.State))
	p.text(fieldCountry, strings.TrimSpace(cs.Country))
	p.text(fieldZip, strings.TrimSpace(cs.Zip))
	p.text(fieldTelephone, strings.TrimSpace(cs.Telephone))
	p.text(fieldEmail, strings.TrimSpace(cs.Email))

	// Enclosures.
	p.box(boxSpec, cs.SpecEnclosed)
	p.text(fieldSpecPages, countText(cs.SpecPages))
	p.box(boxDrawings, cs.DrawingsEnclosed)
	p.text(fieldDrawingSheets, countText(cs.DrawingSheets))
	p.box(boxADS, cs.ADSEnclosed)
	p.box(boxCDs, cs.CDsEnclosed)
	p.text(fieldCDCount, countText(cs.CDCount))
	p.box(boxOther, cs.OtherEnclosed)
	p.text(fieldOtherSpecify, strings.TrimSpace(cs.OtherSpecify))

	// Entity status.
	p.box(boxSmallEntity, cs.SmallEntity)
	p.box(boxMicroEntity, cs.MicroEntity)

	// Fee and payment.
	p.text(fieldTotalFee, strings.TrimSpace(cs.TotalFeeAmount))
	p.box(boxPayDeposit, cs.PayDeposit)
	p.text(fieldDepositAccount, strings.TrimSpace(cs.DepositAccount))
	p.box(boxPayCheck, cs.PayCheck)
	p.box(boxPayCreditCard, cs.PayCreditCard)

	// Federal-interest statement.
	switch cs.GovInterest {
	case domain.GovInterestAgency:
		p.box(boxGovAgency, true)
		p.text(fieldAgencyName, strings.TrimSpace(cs.AgencyName))
	case domain.GovInterestContract:
		p.box(boxGovContract, true)
		p.text(fieldContractNumber, strings.TrimSpace(cs.ContractNumber))
	default:
		p.box(boxGovNo, true)
	}

	// Signature block.
	p.text(fieldSignature, strings.TrimSpace(cs.SignatureText))
	p.text(fieldSignerName, strings.TrimSpace(cs.SignerName))
	p.text(fieldRegistrationNo, strings.TrimSpace(cs.RegistrationNo))
	if !cs.SignatureDate.IsZero() {
		p.text(fieldSignatureDate, cs.SignatureDate.Format(domain.USDateLayout))
	}
	p.text(fieldSignerTelephone, strings.TrimSpace(cs.SignerTelephone))
	p.text(fieldDocketNumber, strings.TrimSpace(cs.DocketNumber))
	p.text(fieldPriorityMailLabel, strings.TrimSpace(cs.PriorityMailLabel))

	return p
}

// countText renders a positive count as its decimal string; zero/negative is
// blank so the form cell stays empty.
func countText(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}
