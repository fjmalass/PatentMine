package coversheet

import "fmt"

// PTO/SB/16 AcroForm field names, taken verbatim from the official form (dumped
// with pdfcpu's form exporter). They are awkward strings the form designer left
// behind, so they are pinned here as named constants rather than scattered as
// literals through the mapping.
const (
	fieldTitle             = "TITLE OF THE INVENTION 500 characters maxRow1"
	fieldAdditionalSheets  = "Additional inventors are being named on the"
	fieldCustomerNumber    = "CORRESPONDENCE ADDRESS"
	fieldFirmName          = "Firm or Individual Name"
	fieldAddress           = "Address"
	fieldCity              = "City"
	fieldState             = "State"
	fieldCountry           = "Country"
	fieldZip               = "zip code"
	fieldTelephone         = "Telephone_2"
	fieldEmail             = "Email_2"
	fieldSpecPages         = "Number of Pages"
	fieldDrawingSheets     = "Number of Sheets"
	fieldCDCount           = "CDs Number of CDs"
	fieldOtherSpecify      = "undefined_2"
	fieldTotalFee          = "TOTAL FEE AMOUNT"
	fieldDepositAccount    = "Account Number"
	fieldAgencyName        = "The US Government agency name is"
	fieldContractNumber    = "The contract number is"
	fieldSignature         = "signature"
	fieldSignerName        = "TYPED OR PRINTED NAME"
	fieldRegistrationNo    = "REGISTRATION NO"
	fieldSignatureDate     = "DATE"
	fieldSignerTelephone   = "TELEPHONE"
	fieldDocketNumber      = "DOCKET NUMBER"
	fieldPriorityMailLabel = "Priority Mail Express Label No"

	// Checkboxes.
	boxDirectToCustomerNumber = "The address corresponding to Customer Number"
	boxDirectToFirm           = "undefined"
	boxSpec                   = "Specification eg description of the invention"
	boxDrawings               = "Drawings"
	boxADS                    = "ADS checkbox"
	boxCDs                    = "CD(s)"
	boxOther                  = "Other specify"
	boxSmallEntity            = "SE"
	boxMicroEntity            = "ME"
	boxPayDeposit             = "DA"
	boxPayCheck               = "Check or money order"
	boxPayCreditCard          = "CC payment"
	boxGovNo                  = "No"
	boxGovAgency              = "Yes the invention was made by an agency of the US Government The US Government agency name is"
	boxGovContract            = "Yes the invention was made under a contract with an agency of the US Government"
)

// inventorGivenField returns the AcroForm field name for one inventor's given
// name (1-indexed rows 1..5). The form's rows are simply suffixed RowN.
func inventorGivenField(row int) string {
	return fmt.Sprintf("Given Name first and middle if anyRow%d", row)
}

func inventorFamilyField(row int) string {
	return fmt.Sprintf("Family Name or SurnameRow%d", row)
}

func inventorResidenceField(row int) string {
	return fmt.Sprintf("Residence City and either State or Foreign CountryRow%d", row)
}

// maxFormInventors is how many inventor rows the SB/16 prints; beyond this the
// form points to attached continuation sheets.
const maxFormInventors = 5
