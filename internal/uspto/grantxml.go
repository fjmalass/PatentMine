package uspto

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// USPTOGrantXML is the parsed shape of a USPTO us-patent-grant or
// us-patent-application XML document (DTD v4.x). The struct accepts either
// root element so the same code path ingests grants and pre-grant
// publications. Field names mirror the XML element names so the mapping back
// to the source is obvious. Sections absent from a given XML stay zero.
type USPTOGrantXML struct {
	Lang             string `xml:"lang,attr"`
	DTDVersion       string `xml:"dtd-version,attr"`
	File             string `xml:"file,attr"`
	Status           string `xml:"status,attr"`
	Country          string `xml:"country,attr"`
	DateProduced     string `xml:"date-produced,attr"`
	DatePublished    string `xml:"date-publ,attr"`
	// Two bibliographic fields: grant XML uses <us-bibliographic-data-grant>,
	// pgpub XML uses <us-bibliographic-data-application>. The decoder only
	// populates the field whose tag matches the element it sees, so for any
	// given document exactly one will be non-empty. USPTOGrantToIngest picks
	// whichever has data.
	Bibliographic    USPTOGrantBibliographic `xml:"us-bibliographic-data-grant"`
	BibliographicApp USPTOGrantBibliographic `xml:"us-bibliographic-data-application"`
	Abstract         USPTOGrantAbstract
	Drawings         USPTOGrantDrawings
	Description      USPTOGrantDescription
	ClaimStatement   string           `xml:"us-claim-statement"`
	Claims           USPTOGrantClaims `xml:"claims"`
}

// USPTOGrantBibliographic holds the bibliographic block (either grant or
// application). The XMLName is intentionally absent so the same struct can be
// targeted by both root element tags above.
type USPTOGrantBibliographic struct {
	PublicationRef          USPTOGrantDocRef               `xml:"publication-reference>document-id"`
	ApplicationRef          USPTOGrantApplicationRef       `xml:"application-reference"`
	ApplicationSeriesCode   string                         `xml:"us-application-series-code"`
	TermExtensionDays       string                         `xml:"us-term-of-grant>us-term-extension"`
	IPCRClassifications     []USPTOGrantIPCR               `xml:"classifications-ipcr>classification-ipcr"`
	MainCPC                 []USPTOGrantCPC                `xml:"classifications-cpc>main-cpc>classification-cpc"`
	FurtherCPC              []USPTOGrantCPC                `xml:"classifications-cpc>further-cpc>classification-cpc"`
	InventionTitle          string                         `xml:"invention-title"`
	ReferencesCited         []USPTOGrantCitation           `xml:"us-references-cited>us-citation"`
	NumberOfClaims          string                         `xml:"number-of-claims"`
	ExemplaryClaim          string                         `xml:"us-exemplary-claim"`
	FieldOfClassification   []string                       `xml:"us-field-of-classification-search>classification-cpc-text"`
	NumberOfDrawingSheets   string                         `xml:"figures>number-of-drawing-sheets"`
	NumberOfFigures         string                         `xml:"figures>number-of-figures"`
	Continuations           []USPTOGrantRelation           `xml:"us-related-documents>continuation>relation"`
	Continuations2          []USPTOGrantRelation           `xml:"us-related-documents>continuation-in-part>relation"`
	Divisions               []USPTOGrantRelation           `xml:"us-related-documents>division>relation"`
	Reissues                []USPTOGrantRelation           `xml:"us-related-documents>reissue>relation"`
	ProvisionalApplications []USPTOGrantDocRef             `xml:"us-related-documents>us-provisional-application>document-id"`
	RelatedPublications     []USPTOGrantDocRef             `xml:"us-related-documents>related-publication>document-id"`
	Applicants              []USPTOGrantParty              `xml:"us-parties>us-applicants>us-applicant"`
	Inventors               []USPTOGrantParty              `xml:"us-parties>inventors>inventor"`
	Agents                  []USPTOGrantAgent              `xml:"us-parties>agents>agent"`
	Assignees               []USPTOGrantAssignee           `xml:"assignees>assignee"`
	PrimaryExaminer         USPTOGrantExaminer             `xml:"examiners>primary-examiner"`
	AssistantExaminers      []USPTOGrantExaminer           `xml:"examiners>assistant-examiner"`
}

type USPTOGrantApplicationRef struct {
	ApplType   string           `xml:"appl-type,attr"`
	DocumentID USPTOGrantDocRef `xml:"document-id"`
}

type USPTOGrantDocRef struct {
	Country   string `xml:"country"`
	DocNumber string `xml:"doc-number"`
	Kind      string `xml:"kind"`
	Name      string `xml:"name"`
	Date      string `xml:"date"`
}

type USPTOGrantIPCR struct {
	IPCDate            string `xml:"ipc-version-indicator>date"`
	Level              string `xml:"classification-level"`
	Section            string `xml:"section"`
	Class              string `xml:"class"`
	Subclass           string `xml:"subclass"`
	MainGroup          string `xml:"main-group"`
	Subgroup           string `xml:"subgroup"`
	SymbolPosition     string `xml:"symbol-position"`
	ClassificationVal  string `xml:"classification-value"`
	ActionDate         string `xml:"action-date>date"`
	GeneratingOffice   string `xml:"generating-office>country"`
	ClassificationStat string `xml:"classification-status"`
	DataSource         string `xml:"classification-data-source"`
}

type USPTOGrantCPC struct {
	CPCDate            string `xml:"cpc-version-indicator>date"`
	Section            string `xml:"section"`
	Class              string `xml:"class"`
	Subclass           string `xml:"subclass"`
	MainGroup          string `xml:"main-group"`
	Subgroup           string `xml:"subgroup"`
	SymbolPosition     string `xml:"symbol-position"`
	ClassificationVal  string `xml:"classification-value"`
	ActionDate         string `xml:"action-date>date"`
	GeneratingOffice   string `xml:"generating-office>country"`
	ClassificationStat string `xml:"classification-status"`
	DataSource         string `xml:"classification-data-source"`
	SchemeOrigination  string `xml:"scheme-origination-code"`
}

// Code formats a CPC entry as the canonical "Section Class Subclass MainGroup/Subgroup" string.
func (c USPTOGrantCPC) Code() string {
	return strings.TrimSpace(fmt.Sprintf("%s%s%s %s/%s", c.Section, c.Class, c.Subclass, c.MainGroup, c.Subgroup))
}

// Code formats an IPCR entry the same way.
func (c USPTOGrantIPCR) Code() string {
	return strings.TrimSpace(fmt.Sprintf("%s%s%s %s/%s", c.Section, c.Class, c.Subclass, c.MainGroup, c.Subgroup))
}

type USPTOGrantCitation struct {
	PatCit                struct {
		Num        string           `xml:"num,attr"`
		DocumentID USPTOGrantDocRef `xml:"document-id"`
	} `xml:"patcit"`
	NPLCit struct {
		Num       string `xml:"num,attr"`
		OtherCite string `xml:"othercit"`
	} `xml:"nplcit"`
	Category               string `xml:"category"`
	ClassificationCPCText  string `xml:"classification-cpc-text"`
	ClassificationNational struct {
		Country            string `xml:"country"`
		MainClassification string `xml:"main-classification"`
	} `xml:"classification-national"`
}

type USPTOGrantRelation struct {
	ParentDoc struct {
		DocumentID    USPTOGrantDocRef `xml:"document-id"`
		ParentGrant   USPTOGrantDocRef `xml:"parent-grant-document>document-id"`
	} `xml:"parent-doc"`
	ChildDoc struct {
		DocumentID USPTOGrantDocRef `xml:"document-id"`
	} `xml:"child-doc"`
}

type USPTOGrantParty struct {
	Sequence    string `xml:"sequence,attr"`
	Designation string `xml:"designation,attr"`
	AppType     string `xml:"app-type,attr"`
	Addressbook USPTOGrantAddressbook `xml:"addressbook"`
	Residence   struct {
		Country string `xml:"country"`
	} `xml:"residence"`
}

type USPTOGrantAgent struct {
	Sequence    string `xml:"sequence,attr"`
	RepType     string `xml:"rep-type,attr"`
	Addressbook USPTOGrantAddressbook `xml:"addressbook"`
}

type USPTOGrantAssignee struct {
	Addressbook USPTOGrantAddressbook `xml:"addressbook"`
	OrgName     string                `xml:"orgname"`
	FirstName   string                `xml:"first-name"`
	LastName    string                `xml:"last-name"`
	Role        string                `xml:"role"`
	Address     USPTOGrantAddress     `xml:"address"`
}

type USPTOGrantExaminer struct {
	LastName   string `xml:"last-name"`
	FirstName  string `xml:"first-name"`
	Department string `xml:"department"`
}

type USPTOGrantAddressbook struct {
	OrgName   string            `xml:"orgname"`
	FirstName string            `xml:"first-name"`
	LastName  string            `xml:"last-name"`
	Role      string            `xml:"role"`
	Address   USPTOGrantAddress `xml:"address"`
}

type USPTOGrantAddress struct {
	Street     string `xml:"street"`
	City       string `xml:"city"`
	State      string `xml:"state"`
	Country    string `xml:"country"`
	PostalCode string `xml:"postcode"`
}

// USPTOGrantAbstract carries the <abstract> block as raw inner XML so callers
// can either present the text directly or strip tags themselves.
type USPTOGrantAbstract struct {
	XMLName xml.Name `xml:"abstract"`
	ID      string   `xml:"id,attr"`
	Inner   string   `xml:",innerxml"`
}

// Text returns the abstract with markup stripped.
func (a USPTOGrantAbstract) Text() string { return stripXMLTags(a.Inner) }

// USPTOGrantDrawings carries the <drawings> block. Individual figure metadata
// is exposed so callers can list filenames or generate a thumbnail manifest.
type USPTOGrantDrawings struct {
	XMLName xml.Name           `xml:"drawings"`
	ID      string             `xml:"id,attr"`
	Figures []USPTOGrantFigure `xml:"figure"`
}

type USPTOGrantFigure struct {
	ID  string `xml:"id,attr"`
	Num string `xml:"num,attr"`
	Img struct {
		ID         string `xml:"id,attr"`
		Height     string `xml:"he,attr"`
		Width      string `xml:"wi,attr"`
		File       string `xml:"file,attr"`
		Alt        string `xml:"alt,attr"`
		Content    string `xml:"img-content,attr"`
		Format     string `xml:"img-format,attr"`
	} `xml:"img"`
}

// USPTOGrantDescription is the long-form spec, broken into paragraphs and
// headings. Raw inner XML preserves all structure (figrefs, tables, lists)
// for downstream rendering.
type USPTOGrantDescription struct {
	XMLName xml.Name `xml:"description"`
	ID      string   `xml:"id,attr"`
	Inner   string   `xml:",innerxml"`
}

// Text returns the description text with all markup stripped.
func (d USPTOGrantDescription) Text() string { return stripXMLTags(d.Inner) }

// USPTOGrantClaims carries the patent claims. Inner XML is retained so claim
// renderers can keep claim references and emphasis intact.
type USPTOGrantClaims struct {
	XMLName xml.Name          `xml:"claims"`
	ID      string            `xml:"id,attr"`
	Items   []USPTOGrantClaim `xml:"claim"`
}

type USPTOGrantClaim struct {
	ID    string `xml:"id,attr"`
	Num   string `xml:"num,attr"`
	Inner string `xml:",innerxml"`
}

// Text strips XML markup from a single claim's body.
func (c USPTOGrantClaim) Text() string { return stripXMLTags(c.Inner) }

// ParseUSPTOGrantXML decodes a us-patent-grant XML document.
func ParseUSPTOGrantXML(r io.Reader) (*USPTOGrantXML, error) {
	dec := xml.NewDecoder(r)
	dec.Strict = false
	// us-patent-grant documents reference an external DTD; we do not fetch it.
	dec.Entity = xml.HTMLEntity
	var doc USPTOGrantXML
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("crawl: parse uspto grant xml: %w", err)
	}
	return &doc, nil
}

// stripXMLTags returns the visible text of a fragment of XML by dropping every
// tag and collapsing runs of whitespace into single spaces. It is intentionally
// permissive — the input is trusted USPTO XML, not arbitrary HTML.
func stripXMLTags(s string) string {
	var b strings.Builder
	depth := 0
	pi := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if pi {
			if c == '>' {
				pi = false
			}
			continue
		}
		if c == '<' {
			if i+1 < len(s) && s[i+1] == '?' {
				pi = true
				i++
				continue
			}
			depth++
			continue
		}
		if c == '>' {
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth == 0 {
			b.WriteByte(c)
		}
	}
	return collapseWhitespace(b.String())
}

func collapseWhitespace(s string) string {
	var b strings.Builder
	prev := byte(' ')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\n' || c == '\r' || c == '\t' {
			c = ' '
		}
		if c == ' ' && prev == ' ' {
			continue
		}
		b.WriteByte(c)
		prev = c
	}
	return strings.TrimSpace(b.String())
}
