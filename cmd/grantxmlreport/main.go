// grantxmlreport parses a USPTO us-patent-grant XML file and prints every
// field present so callers can decide what to surface in the data model.
package main

import (
	"fmt"
	"os"
	"strings"

	"patentmine/internal/uspto"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: grantxmlreport <path-to-us-patent-grant.xml>")
		os.Exit(2)
	}
	path := os.Args[1]
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	doc, err := uspto.ParseUSPTOGrantXML(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}

	report(doc, path)
}

func report(d *uspto.USPTOGrantXML, path string) {
	pf := func(label, value string) {
		v := strings.TrimSpace(value)
		if v == "" {
			v = "—"
		}
		fmt.Printf("  %-30s %s\n", label+":", v)
	}
	header := func(s string) {
		fmt.Println()
		fmt.Println("== " + s + " ==")
	}

	header("File")
	pf("Path", path)
	pf("DTD version", d.DTDVersion)
	pf("Country", d.Country)
	pf("Status", d.Status)
	pf("Date produced", d.DateProduced)
	pf("Date published", d.DatePublished)
	pf("File attr", d.File)
	pf("Language", d.Lang)

	header("Publication reference")
	pf("Country", d.Bibliographic.PublicationRef.Country)
	pf("Doc number", d.Bibliographic.PublicationRef.DocNumber)
	pf("Kind", d.Bibliographic.PublicationRef.Kind)
	pf("Date", d.Bibliographic.PublicationRef.Date)

	header("Application reference")
	pf("Type", d.Bibliographic.ApplicationRef.ApplType)
	pf("Country", d.Bibliographic.ApplicationRef.DocumentID.Country)
	pf("App number", d.Bibliographic.ApplicationRef.DocumentID.DocNumber)
	pf("Filing date", d.Bibliographic.ApplicationRef.DocumentID.Date)
	pf("Series code", d.Bibliographic.ApplicationSeriesCode)
	pf("Term extension (days)", d.Bibliographic.TermExtensionDays)

	header(fmt.Sprintf("IPCR classifications (%d)", len(d.Bibliographic.IPCRClassifications)))
	for i, c := range d.Bibliographic.IPCRClassifications {
		pf(fmt.Sprintf("[%d] code", i), c.Code())
		pf(fmt.Sprintf("[%d] level", i), c.Level)
		pf(fmt.Sprintf("[%d] value", i), c.ClassificationVal)
		pf(fmt.Sprintf("[%d] action date", i), c.ActionDate)
		pf(fmt.Sprintf("[%d] status", i), c.ClassificationStat)
		pf(fmt.Sprintf("[%d] data source", i), c.DataSource)
	}

	header(fmt.Sprintf("Main CPC (%d)", len(d.Bibliographic.MainCPC)))
	for i, c := range d.Bibliographic.MainCPC {
		pf(fmt.Sprintf("[%d] code", i), c.Code())
		pf(fmt.Sprintf("[%d] origination", i), c.SchemeOrigination)
	}

	header(fmt.Sprintf("Further CPC (%d)", len(d.Bibliographic.FurtherCPC)))
	for i, c := range d.Bibliographic.FurtherCPC {
		pf(fmt.Sprintf("[%d] code", i), c.Code())
	}

	header("Title")
	pf("Invention title", d.Bibliographic.InventionTitle)

	refsCited := d.Bibliographic.ReferencesCited()
	header(fmt.Sprintf("Citations (%d)", len(refsCited)))
	for i, c := range refsCited {
		num := c.PatCit.Num
		if num == "" {
			num = c.NPLCit.Num
		}
		if c.PatCit.DocumentID.DocNumber != "" {
			ref := fmt.Sprintf("%s%s %s (%s, %s)",
				c.PatCit.DocumentID.Country, c.PatCit.DocumentID.DocNumber,
				c.PatCit.DocumentID.Kind, c.PatCit.DocumentID.Name, c.PatCit.DocumentID.Date)
			pf(fmt.Sprintf("[%s] %s", num, c.Category), ref)
		} else if c.NPLCit.OtherCite != "" {
			pf(fmt.Sprintf("[%s] %s NPL", num, c.Category), collapse(c.NPLCit.OtherCite))
		}
		if i >= 6 {
			fmt.Printf("  ... %d more citations\n", len(refsCited)-i-1)
			break
		}
	}

	header("Counts")
	pf("Number of claims", d.Bibliographic.NumberOfClaims)
	pf("Exemplary claim", d.Bibliographic.ExemplaryClaim)
	pf("Drawing sheets", d.Bibliographic.NumberOfDrawingSheets)
	pf("Number of figures", d.Bibliographic.NumberOfFigures)

	header(fmt.Sprintf("Field of classification search (%d)", len(d.Bibliographic.FieldOfClassification)))
	for i, c := range d.Bibliographic.FieldOfClassification {
		pf(fmt.Sprintf("[%d]", i), c)
	}

	header(fmt.Sprintf("Related: continuations (%d)", len(d.Bibliographic.Continuations)))
	for i, r := range d.Bibliographic.Continuations {
		pf(fmt.Sprintf("[%d] parent appl", i), r.ParentDoc.DocumentID.DocNumber+" "+r.ParentDoc.DocumentID.Date)
		pf(fmt.Sprintf("[%d] parent grant", i), r.ParentDoc.ParentGrant.DocNumber+" "+r.ParentDoc.ParentGrant.Date)
		pf(fmt.Sprintf("[%d] child", i), r.ChildDoc.DocumentID.DocNumber)
	}
	pf("Continuations-in-part", fmt.Sprintf("%d", len(d.Bibliographic.Continuations2)))
	pf("Divisions", fmt.Sprintf("%d", len(d.Bibliographic.Divisions)))
	pf("Reissues", fmt.Sprintf("%d", len(d.Bibliographic.Reissues)))

	header(fmt.Sprintf("Provisional applications (%d)", len(d.Bibliographic.ProvisionalApplications)))
	for i, p := range d.Bibliographic.ProvisionalApplications {
		pf(fmt.Sprintf("[%d]", i), p.DocNumber+" "+p.Date)
	}

	header(fmt.Sprintf("Related publications (%d)", len(d.Bibliographic.RelatedPublications)))
	for i, p := range d.Bibliographic.RelatedPublications {
		pf(fmt.Sprintf("[%d]", i), fmt.Sprintf("%s%s %s %s", p.Country, p.DocNumber, p.Kind, p.Date))
	}

	header(fmt.Sprintf("Applicants (%d)", len(d.Bibliographic.Applicants)))
	for i, p := range d.Bibliographic.Applicants {
		pf(fmt.Sprintf("[%d] name", i), personName(p.Addressbook))
		pf(fmt.Sprintf("[%d] residence", i), p.Addressbook.Address.City+", "+p.Addressbook.Address.State+", "+p.Addressbook.Address.Country)
	}

	header(fmt.Sprintf("Inventors (%d)", len(d.Bibliographic.Inventors)))
	for i, p := range d.Bibliographic.Inventors {
		pf(fmt.Sprintf("[%d] name", i), personName(p.Addressbook))
		pf(fmt.Sprintf("[%d] residence", i), p.Addressbook.Address.City+", "+p.Addressbook.Address.State+", "+p.Addressbook.Address.Country)
	}

	header(fmt.Sprintf("Agents (%d)", len(d.Bibliographic.Agents)))
	for i, a := range d.Bibliographic.Agents {
		pf(fmt.Sprintf("[%d] org", i), a.Addressbook.OrgName)
		pf(fmt.Sprintf("[%d] type", i), a.RepType)
	}

	header(fmt.Sprintf("Assignees (%d)", len(d.Bibliographic.Assignees)))
	for i, a := range d.Bibliographic.Assignees {
		name := a.OrgName
		if name == "" {
			name = a.Addressbook.OrgName
		}
		pf(fmt.Sprintf("[%d] name", i), name)
		pf(fmt.Sprintf("[%d] role", i), a.Role)
	}

	header("Primary examiner")
	pf("Last name", d.Bibliographic.PrimaryExaminer.LastName)
	pf("First name", d.Bibliographic.PrimaryExaminer.FirstName)
	pf("Department", d.Bibliographic.PrimaryExaminer.Department)

	header("Abstract")
	pf("ID", d.Abstract.ID)
	pf("Text (first 240ch)", truncate(d.Abstract.Text(), 240))
	pf("Text length", fmt.Sprintf("%d chars", len(d.Abstract.Text())))

	header(fmt.Sprintf("Drawings: %d figures", len(d.Drawings.Figures)))
	for i, f := range d.Drawings.Figures {
		pf(fmt.Sprintf("[%d] %s", i, f.Num), fmt.Sprintf("%s (%sx%s)", f.Img.File, f.Img.Width, f.Img.Height))
		if i >= 3 {
			fmt.Printf("  ... %d more figures\n", len(d.Drawings.Figures)-i-1)
			break
		}
	}

	header("Description")
	pf("ID", d.Description.ID)
	pf("Text (first 240ch)", truncate(d.Description.Text(), 240))
	pf("Text length", fmt.Sprintf("%d chars", len(d.Description.Text())))

	header("Claim statement")
	pf("Lead", d.ClaimStatement)

	header(fmt.Sprintf("Claims (%d)", len(d.Claims.Items)))
	for i, c := range d.Claims.Items {
		pf(fmt.Sprintf("[%s]", c.Num), truncate(c.Text(), 160))
		if i >= 3 {
			fmt.Printf("  ... %d more claims\n", len(d.Claims.Items)-i-1)
			break
		}
	}
}

func personName(a uspto.USPTOGrantAddressbook) string {
	parts := []string{}
	if a.FirstName != "" {
		parts = append(parts, a.FirstName)
	}
	if a.LastName != "" {
		parts = append(parts, a.LastName)
	}
	if a.OrgName != "" {
		parts = append(parts, a.OrgName)
	}
	return strings.Join(parts, " ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
