package uspto

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"patentmine/internal/domain"
)

// USPTOGrantToIngest converts a parsed XML document into the persistence
// bundle the store accepts. kind is the role of this XML ("pgpub" or "grant").
// applicationNumber overrides the value from the XML when set — useful for
// pre-grant publications whose XML carries the publication number rather
// than the application number.
func USPTOGrantToIngest(doc *USPTOGrantXML, applicationNumber, kind string) domain.USPTOGrantIngest {
	now := time.Now().UTC().Format(time.RFC3339)
	// Both roots map to the same bibliographic struct via separate fields;
	// pick whichever the decoder populated.
	bib := doc.Bibliographic
	if bib.ApplicationRef.DocumentID.DocNumber == "" && bib.PublicationRef.DocNumber == "" {
		bib = doc.BibliographicApp
	}
	if applicationNumber == "" {
		applicationNumber = strings.TrimSpace(bib.ApplicationRef.DocumentID.DocNumber)
	}

	summary := domain.USPTOGrantSummary{
		ApplicationNumber:     applicationNumber,
		FilingDate:            strings.TrimSpace(bib.ApplicationRef.DocumentID.Date),
		ApplicationSeriesCode: strings.TrimSpace(bib.ApplicationSeriesCode),
		InventionTitle:        strings.TrimSpace(bib.InventionTitle),
		GrantDocNumber:        bib.PublicationRef.DocNumber,
		GrantKind:             bib.PublicationRef.Kind,
		GrantDate:             bib.PublicationRef.Date,
		GrantDTDVersion:       doc.DTDVersion,
		GrantStatus:           doc.Status,
		GrantDateProduced:     doc.DateProduced,
		GrantFileName:         doc.File,
		GrantLang:             doc.Lang,
		TermExtensionDays:     atoi(bib.TermExtensionDays),
		NumberOfClaims:        atoi(bib.NumberOfClaims),
		ExemplaryClaim:        bib.ExemplaryClaim,
		NumberOfDrawingSheets: atoi(bib.NumberOfDrawingSheets),
		NumberOfFigures:       atoi(bib.NumberOfFigures),
		PrimaryExaminerFirst:  bib.PrimaryExaminer.FirstName,
		PrimaryExaminerLast:   bib.PrimaryExaminer.LastName,
		PrimaryExaminerDept:   bib.PrimaryExaminer.Department,
		FieldOfSearch:         bib.FieldOfClassification,
		ParsedAt:              now,
	}
	for _, ax := range bib.AssistantExaminers {
		name := strings.TrimSpace(strings.TrimSpace(ax.FirstName) + " " + strings.TrimSpace(ax.LastName))
		if name != "" {
			summary.AssistantExaminers = append(summary.AssistantExaminers, name)
		}
	}
	if len(bib.Agents) > 0 {
		a := bib.Agents[0]
		summary.AttorneyOrg = a.Addressbook.OrgName
		summary.AttorneyType = a.RepType
	}

	body := domain.USPTOGrantBody{
		ApplicationNumber: applicationNumber,
		Kind:              kind,
		AbstractText:      doc.Abstract.Text(),
		AbstractXML:       doc.Abstract.Inner,
		DescriptionText:   doc.Description.Text(),
		DescriptionXML:    doc.Description.Inner,
		ClaimStatement:    strings.TrimSpace(doc.ClaimStatement),
		ParsedAt:          now,
	}
	var claimsBuf strings.Builder
	for _, c := range doc.Claims.Items {
		body.Claims = append(body.Claims, domain.USPTOGrantClaim{
			Number: c.Num,
			Text:   c.Text(),
			XML:    c.Inner,
		})
		claimsBuf.WriteString(c.Text())
		claimsBuf.WriteString("\n\n")
	}
	body.ClaimsText = strings.TrimSpace(claimsBuf.String())

	drawings := make([]domain.USPTODrawing, 0, len(doc.Drawings.Figures))
	for i, f := range doc.Drawings.Figures {
		drawings = append(drawings, domain.USPTODrawing{
			ApplicationNumber: applicationNumber,
			Kind:              kind,
			Ordinal:           i,
			FigureNum:         f.Num,
			FigureID:          f.ID,
			ImgID:             f.Img.ID,
			FileName:          f.Img.File,
			Width:             f.Img.Width,
			Height:            f.Img.Height,
			AltText:           f.Img.Alt,
			ImgFormat:         f.Img.Format,
			ImgContent:        f.Img.Content,
		})
	}

	refsCited := bib.ReferencesCited()
	citations := make([]domain.USPTOGrantCitation, 0, len(refsCited))
	for i, c := range refsCited {
		citations = append(citations, citationToDomain(c, applicationNumber, kind, i))
	}

	var classifications []domain.USPTOGrantClassification
	for i, c := range bib.IPCRClassifications {
		role := "main"
		if sp := strings.ToUpper(strings.TrimSpace(c.SymbolPosition)); sp == "L" {
			role = "further"
		}
		classifications = append(classifications, domain.USPTOGrantClassification{
			ApplicationNumber:    applicationNumber,
			Kind:                 kind,
			Scheme:               "ipcr",
			Role:                 role,
			Ordinal:              i,
			FullCode:             c.Code(),
			Section:              c.Section,
			Class:                c.Class,
			Subclass:             c.Subclass,
			MainGroup:            c.MainGroup,
			Subgroup:             c.Subgroup,
			SymbolPosition:       c.SymbolPosition,
			ClassificationValue:  c.ClassificationVal,
			ClassificationLevel:  c.Level,
			ClassificationStatus: c.ClassificationStat,
			DataSource:           c.DataSource,
			ActionDate:           c.ActionDate,
			GeneratingOffice:     c.GeneratingOffice,
			VersionDate:          c.IPCDate,
		})
	}
	addCPC := func(role string, list []USPTOGrantCPC) {
		for i, c := range list {
			classifications = append(classifications, domain.USPTOGrantClassification{
				ApplicationNumber:    applicationNumber,
				Kind:                 kind,
				Scheme:               "cpc",
				Role:                 role,
				Ordinal:              i,
				FullCode:             c.Code(),
				Section:              c.Section,
				Class:                c.Class,
				Subclass:             c.Subclass,
				MainGroup:            c.MainGroup,
				Subgroup:             c.Subgroup,
				SymbolPosition:       c.SymbolPosition,
				ClassificationValue:  c.ClassificationVal,
				ClassificationStatus: c.ClassificationStat,
				DataSource:           c.DataSource,
				ActionDate:           c.ActionDate,
				GeneratingOffice:     c.GeneratingOffice,
				VersionDate:          c.CPCDate,
				SchemeOrigination:    c.SchemeOrigination,
			})
		}
	}
	addCPC("main", bib.MainCPC)
	addCPC("further", bib.FurtherCPC)
	for i, code := range bib.FieldOfClassification {
		classifications = append(classifications, domain.USPTOGrantClassification{
			ApplicationNumber: applicationNumber,
			Kind:              kind,
			Scheme:            "cpc",
			Role:              "search",
			Ordinal:           i,
			FullCode:          code,
		})
	}

	var relations []domain.USPTOGrantRelation
	addRelations := func(label string, list []USPTOGrantRelation) {
		for i, r := range list {
			relations = append(relations, domain.USPTOGrantRelation{
				ApplicationNumber:  applicationNumber,
				Kind:               kind,
				Ordinal:            len(relations),
				RelationKind:       label,
				ParentCountry:      r.ParentDoc.DocumentID.Country,
				ParentAppNumber:    r.ParentDoc.DocumentID.DocNumber,
				ParentAppDate:      r.ParentDoc.DocumentID.Date,
				ParentGrantCountry: r.ParentDoc.ParentGrant.Country,
				ParentGrantNumber:  r.ParentDoc.ParentGrant.DocNumber,
				ParentGrantDate:    r.ParentDoc.ParentGrant.Date,
				ChildCountry:       r.ChildDoc.DocumentID.Country,
				ChildAppNumber:     r.ChildDoc.DocumentID.DocNumber,
			})
			_ = i
		}
	}
	addRelations("continuation", bib.Continuations)
	addRelations("continuation-in-part", bib.Continuations2)
	addRelations("division", bib.Divisions)
	addRelations("reissue", bib.Reissues)
	for _, p := range bib.ProvisionalApplications {
		relations = append(relations, domain.USPTOGrantRelation{
			ApplicationNumber: applicationNumber,
			Kind:              kind,
			Ordinal:           len(relations),
			RelationKind:      "provisional",
			ParentCountry:     p.Country,
			ParentAppNumber:   p.DocNumber,
			ParentAppDate:     p.Date,
		})
	}
	for _, p := range bib.RelatedPublications {
		relations = append(relations, domain.USPTOGrantRelation{
			ApplicationNumber: applicationNumber,
			Kind:              kind,
			Ordinal:           len(relations),
			RelationKind:      "related-publication",
			RelatedCountry:    p.Country,
			RelatedDocNumber:  p.DocNumber,
			RelatedKind:       p.Kind,
			RelatedDate:       p.Date,
		})
	}

	var parties []domain.USPTOGrantParty
	pushParty := func(p domain.USPTOGrantParty) {
		p.ApplicationNumber = applicationNumber
		p.Kind = kind
		parties = append(parties, p)
	}
	for i, a := range bib.Applicants {
		pushParty(domain.USPTOGrantParty{
			Role:             "applicant",
			Ordinal:          i,
			Sequence:         a.Sequence,
			Designation:      a.Designation,
			AppType:          a.AppType,
			OrgName:          a.Addressbook.OrgName,
			FirstName:        a.Addressbook.FirstName,
			LastName:         a.Addressbook.LastName,
			ResidenceCountry: a.Residence.Country,
			AddressStreet:    a.Addressbook.Address.Street,
			AddressCity:      a.Addressbook.Address.City,
			AddressState:     a.Addressbook.Address.State,
			AddressCountry:   a.Addressbook.Address.Country,
			AddressPostal:    a.Addressbook.Address.PostalCode,
		})
	}
	for i, inv := range bib.Inventors {
		pushParty(domain.USPTOGrantParty{
			Role:             "inventor",
			Ordinal:          i,
			Sequence:         inv.Sequence,
			Designation:      inv.Designation,
			OrgName:          inv.Addressbook.OrgName,
			FirstName:        inv.Addressbook.FirstName,
			LastName:         inv.Addressbook.LastName,
			ResidenceCountry: inv.Residence.Country,
			AddressStreet:    inv.Addressbook.Address.Street,
			AddressCity:      inv.Addressbook.Address.City,
			AddressState:     inv.Addressbook.Address.State,
			AddressCountry:   inv.Addressbook.Address.Country,
			AddressPostal:    inv.Addressbook.Address.PostalCode,
		})
	}
	for i, ag := range bib.Agents {
		pushParty(domain.USPTOGrantParty{
			Role:           "agent",
			Ordinal:        i,
			Sequence:       ag.Sequence,
			RepType:        ag.RepType,
			OrgName:        ag.Addressbook.OrgName,
			FirstName:      ag.Addressbook.FirstName,
			LastName:       ag.Addressbook.LastName,
			AddressStreet:  ag.Addressbook.Address.Street,
			AddressCity:    ag.Addressbook.Address.City,
			AddressState:   ag.Addressbook.Address.State,
			AddressCountry: ag.Addressbook.Address.Country,
			AddressPostal:  ag.Addressbook.Address.PostalCode,
		})
	}
	for i, as := range bib.Assignees {
		// Assignees may carry name/address either at the assignee level
		// directly or nested in an addressbook; pgpub places role inside
		// addressbook while grant XML places it as a direct child. Merge.
		org := as.OrgName
		first := as.FirstName
		last := as.LastName
		role := as.Role
		if org == "" {
			org = as.Addressbook.OrgName
		}
		if first == "" {
			first = as.Addressbook.FirstName
		}
		if last == "" {
			last = as.Addressbook.LastName
		}
		if role == "" {
			role = as.Addressbook.Role
		}
		addr := as.Addressbook.Address
		if addr.City == "" && addr.State == "" && addr.Country == "" {
			addr = as.Address
		}
		pushParty(domain.USPTOGrantParty{
			Role:           "assignee",
			Ordinal:        i,
			AssigneeRole:   role,
			OrgName:        org,
			FirstName:      first,
			LastName:       last,
			AddressStreet:  addr.Street,
			AddressCity:    addr.City,
			AddressState:   addr.State,
			AddressCountry: addr.Country,
			AddressPostal:  addr.PostalCode,
		})
	}

	return domain.USPTOGrantIngest{
		Summary:         summary,
		Body:            body,
		Drawings:        drawings,
		Citations:       citations,
		Classifications: classifications,
		Relations:       relations,
		Parties:         parties,
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// _ silences unused fmt import when this file evolves; intentionally kept for
// downstream callers adding formatted errors here.
var _ = fmt.Sprintf
