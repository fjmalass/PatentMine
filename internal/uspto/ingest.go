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
	if applicationNumber == "" {
		applicationNumber = strings.TrimSpace(doc.Bibliographic.ApplicationRef.DocumentID.DocNumber)
	}

	summary := domain.USPTOGrantSummary{
		ApplicationNumber:     applicationNumber,
		GrantDocNumber:        doc.Bibliographic.PublicationRef.DocNumber,
		GrantKind:             doc.Bibliographic.PublicationRef.Kind,
		GrantDate:             doc.Bibliographic.PublicationRef.Date,
		GrantDTDVersion:       doc.DTDVersion,
		GrantStatus:           doc.Status,
		GrantDateProduced:     doc.DateProduced,
		GrantFileName:         doc.File,
		GrantLang:             doc.Lang,
		TermExtensionDays:     atoi(doc.Bibliographic.TermExtensionDays),
		NumberOfClaims:        atoi(doc.Bibliographic.NumberOfClaims),
		ExemplaryClaim:        doc.Bibliographic.ExemplaryClaim,
		NumberOfDrawingSheets: atoi(doc.Bibliographic.NumberOfDrawingSheets),
		NumberOfFigures:       atoi(doc.Bibliographic.NumberOfFigures),
		PrimaryExaminerFirst:  doc.Bibliographic.PrimaryExaminer.FirstName,
		PrimaryExaminerLast:   doc.Bibliographic.PrimaryExaminer.LastName,
		PrimaryExaminerDept:   doc.Bibliographic.PrimaryExaminer.Department,
		FieldOfSearch:         doc.Bibliographic.FieldOfClassification,
		ParsedAt:              now,
	}
	for _, ax := range doc.Bibliographic.AssistantExaminers {
		name := strings.TrimSpace(strings.TrimSpace(ax.FirstName) + " " + strings.TrimSpace(ax.LastName))
		if name != "" {
			summary.AssistantExaminers = append(summary.AssistantExaminers, name)
		}
	}
	if len(doc.Bibliographic.Agents) > 0 {
		a := doc.Bibliographic.Agents[0]
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

	citations := make([]domain.USPTOGrantCitation, 0, len(doc.Bibliographic.ReferencesCited))
	for i, c := range doc.Bibliographic.ReferencesCited {
		ct := domain.USPTOGrantCitation{
			ApplicationNumber: applicationNumber,
			Kind:              kind,
			Ordinal:           i,
			Category:          c.Category,
			CPCText:           c.ClassificationCPCText,
			NationalCountry:   c.ClassificationNational.Country,
			NationalClass:     c.ClassificationNational.MainClassification,
		}
		if c.PatCit.DocumentID.DocNumber != "" {
			ct.CitationType = "patent"
			ct.CitationNum = c.PatCit.Num
			ct.CitedCountry = c.PatCit.DocumentID.Country
			ct.CitedDocNumber = c.PatCit.DocumentID.DocNumber
			ct.CitedKind = c.PatCit.DocumentID.Kind
			ct.CitedDate = c.PatCit.DocumentID.Date
			ct.CitedName = c.PatCit.DocumentID.Name
		} else {
			ct.CitationType = "npl"
			ct.CitationNum = c.NPLCit.Num
			ct.NPLText = collapseSpaces(c.NPLCit.OtherCite)
		}
		citations = append(citations, ct)
	}

	var classifications []domain.USPTOGrantClassification
	for i, c := range doc.Bibliographic.IPCRClassifications {
		classifications = append(classifications, domain.USPTOGrantClassification{
			ApplicationNumber:    applicationNumber,
			Kind:                 kind,
			Scheme:               "ipcr",
			Role:                 "main",
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
	addCPC("main", doc.Bibliographic.MainCPC)
	addCPC("further", doc.Bibliographic.FurtherCPC)
	for i, code := range doc.Bibliographic.FieldOfClassification {
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
	addRelations("continuation", doc.Bibliographic.Continuations)
	addRelations("continuation-in-part", doc.Bibliographic.Continuations2)
	addRelations("division", doc.Bibliographic.Divisions)
	addRelations("reissue", doc.Bibliographic.Reissues)
	for _, p := range doc.Bibliographic.ProvisionalApplications {
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
	for _, p := range doc.Bibliographic.RelatedPublications {
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

	return domain.USPTOGrantIngest{
		Summary:         summary,
		Body:            body,
		Drawings:        drawings,
		Citations:       citations,
		Classifications: classifications,
		Relations:       relations,
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
