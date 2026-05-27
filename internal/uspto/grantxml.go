package uspto

import (
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"strings"
	"time"

	"patentmine/internal/observability"
)

// Metrics is wired up by the engine to track telemetry and durations.
var Metrics *observability.Metrics

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

// Code formats a CPC entry as the compact "SectionClassSubclassMainGroup/Subgroup" string
// (no space) to match the normalization used by googleClassifications / cleanClassification.
func (c USPTOGrantCPC) Code() string {
	sec := strings.ToUpper(strings.TrimSpace(c.Section))
	cls := strings.ToUpper(strings.TrimSpace(c.Class))
	sub := strings.ToUpper(strings.TrimSpace(c.Subclass))
	mg := strings.TrimSpace(c.MainGroup)
	sg := strings.TrimSpace(c.Subgroup)
	return fmt.Sprintf("%s%s%s%s/%s", sec, cls, sub, mg, sg)
}

// Code formats an IPCR entry the same way (matching Google Patents classification strings).
func (c USPTOGrantIPCR) Code() string {
	sec := strings.ToUpper(strings.TrimSpace(c.Section))
	cls := strings.ToUpper(strings.TrimSpace(c.Class))
	sub := strings.ToUpper(strings.TrimSpace(c.Subclass))
	mg := strings.TrimSpace(c.MainGroup)
	sg := strings.TrimSpace(c.Subgroup)
	return fmt.Sprintf("%s%s%s%s/%s", sec, cls, sub, mg, sg)
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

// FormatDescriptionText decodes a USPTO description's inner XML, formatting headings,
// paragraphs, lists, and line breaks with proper spacing and indentation.
func FormatDescriptionText(innerXML string) string {
	start := time.Now()
	var parseErr error
	defer func() {
		if Metrics != nil {
			Metrics.ObserveDuration("uspto.description.parse", time.Since(start), parseErr != nil)
			Metrics.IncCounter("uspto.description.parse.count", 1)
			if parseErr != nil {
				Metrics.IncCounter("uspto.description.parse.fallback", 1)
			}
		}
	}()

	// Wrap in a dummy root tag to ensure it's a valid single-rooted XML fragment.
	wrapped := "<root>" + innerXML + "</root>"
	dec := xml.NewDecoder(strings.NewReader(wrapped))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity

	var buf strings.Builder
	var currentText strings.Builder

	listDepth := 0
	linePrefix := ""
	lineSpacing := "\n\n" // Default spacing between blocks

	flushCurrentText := func(nextSpacing string, nextPrefix string) {
		trimmed := collapseWhitespace(currentText.String())
		currentText.Reset()
		if trimmed == "" {
			// Update spacing and prefix even if no text was flushed
			linePrefix = nextPrefix
			lineSpacing = nextSpacing
			return
		}

		if buf.Len() > 0 {
			buf.WriteString(lineSpacing)
		}
		buf.WriteString(linePrefix + trimmed)

		linePrefix = nextPrefix
		lineSpacing = nextSpacing
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			parseErr = err
			// On any XML parsing error, fallback to permissive formatting
			return permissiveFormatDescription(innerXML)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if name == "heading" {
				flushCurrentText("\n\n", "")
			} else if name == "p" {
				// Extract paragraph number from 'num' attribute
				pNum := ""
				for _, attr := range t.Attr {
					if attr.Name.Local == "num" {
						pNum = attr.Value
						break
					}
				}
				prefix := ""
				if pNum != "" {
					prefix = fmt.Sprintf("[%s] ", pNum)
				}
				flushCurrentText("\n\n", prefix)
			} else if name == "ul" || name == "ol" || name == "list" {
				listDepth++
			} else if name == "li" || name == "list-item" {
				indent := strings.Repeat(" ", listDepth*ClaimIndentSize)
				flushCurrentText("\n", indent)
			} else if name == "br" {
				nextPrefix := ""
				if listDepth > 0 {
					nextPrefix = strings.Repeat(" ", listDepth*ClaimIndentSize)
				}
				flushCurrentText("\n", nextPrefix)
			}
		case xml.CharData:
			currentText.Write(t)
		case xml.EndElement:
			name := t.Name.Local
			if name == "heading" || name == "p" {
				flushCurrentText("\n\n", "")
			} else if name == "ul" || name == "ol" || name == "list" {
				if listDepth > 0 {
					listDepth--
				}
			} else if name == "li" || name == "list-item" {
				flushCurrentText("\n\n", "")
			}
		}
	}

	// Flush any remaining text
	flushCurrentText("\n\n", "")

	res := buf.String()
	if res == "" {
		return permissiveFormatDescription(innerXML)
	}
	return res
}

// permissiveFormatDescription formats descriptions without strict XML validation,
// preserving paragraphs, headings, lists, line breaks and resolving all entities.
func permissiveFormatDescription(innerXML string) string {
	var buf strings.Builder
	var currentText strings.Builder

	listDepth := 0
	linePrefix := ""
	lineSpacing := "\n\n"

	flushCurrentText := func(nextSpacing string, nextPrefix string) {
		trimmed := collapseWhitespace(currentText.String())
		currentText.Reset()
		if trimmed == "" {
			linePrefix = nextPrefix
			lineSpacing = nextSpacing
			return
		}

		if buf.Len() > 0 {
			buf.WriteString(lineSpacing)
		}
		buf.WriteString(linePrefix + trimmed)

		linePrefix = nextPrefix
		lineSpacing = nextSpacing
	}

	i := 0
	n := len(innerXML)
	for i < n {
		if innerXML[i] == '<' {
			j := i + 1
			for j < n && innerXML[j] != '>' {
				j++
			}
			if j >= n {
				currentText.WriteByte('<')
				i++
				continue
			}
			tagContent := innerXML[i+1 : j]
			i = j + 1

			tagContent = strings.TrimSpace(tagContent)
			if tagContent == "" {
				continue
			}

			if tagContent[0] == '?' || tagContent[0] == '!' {
				continue
			}

			isEnd := tagContent[0] == '/'
			if isEnd {
				tagName := strings.ToLower(strings.TrimSpace(tagContent[1:]))
				if tagName == "heading" || tagName == "p" {
					flushCurrentText("\n\n", "")
				} else if tagName == "ul" || tagName == "ol" || tagName == "list" {
					if listDepth > 0 {
						listDepth--
					}
				} else if tagName == "li" || tagName == "list-item" {
					flushCurrentText("\n\n", "")
				}
			} else {
				parts := strings.Fields(tagContent)
				tagName := strings.ToLower(parts[0])
				isSelfClosing := strings.HasSuffix(tagName, "/")
				if isSelfClosing {
					tagName = strings.TrimSuffix(tagName, "/")
				}

				if tagName == "heading" {
					flushCurrentText("\n\n", "")
				} else if tagName == "p" {
					pNum := ""
					for _, part := range parts[1:] {
						lowerPart := strings.ToLower(part)
						if strings.HasPrefix(lowerPart, "num=") {
							pNum = strings.Trim(part[len("num="):], `"' `)
							break
						}
					}
					prefix := ""
					if pNum != "" {
						prefix = fmt.Sprintf("[%s] ", pNum)
					}
					flushCurrentText("\n\n", prefix)
				} else if tagName == "ul" || tagName == "ol" || tagName == "list" {
					listDepth++
				} else if tagName == "li" || tagName == "list-item" {
					indent := strings.Repeat(" ", listDepth*ClaimIndentSize)
					flushCurrentText("\n", indent)
				} else if tagName == "br" || tagName == "br/" {
					nextPrefix := ""
					if listDepth > 0 {
						nextPrefix = strings.Repeat(" ", listDepth*ClaimIndentSize)
					}
					flushCurrentText("\n", nextPrefix)
				}
			}
		} else {
			currentText.WriteByte(innerXML[i])
			i++
		}
	}

	flushCurrentText("\n\n", "")
	return html.UnescapeString(buf.String())
}

// Text returns the description text with all markup stripped, formatting headings and paragraphs.
func (d USPTOGrantDescription) Text() string { return FormatDescriptionText(d.Inner) }

// USPTOGrantClaims carries the patent claims. Inner XML is retained so claim
// renderers can keep claim references and emphasis intact.
type USPTOGrantClaims struct {
	XMLName xml.Name          `xml:"claims"`
	ID      string            `xml:"id,attr"`
	Items   []USPTOGrantClaim `xml:"claim"`
}

// ClaimIndentSize is the number of spaces used per nesting level in a claim's text hierarchy.
const ClaimIndentSize = 4

// FormatClaimText decodes a USPTO claim's inner XML, preserving the nested
// <claim-text> structure as proper patent indentation (new lines and spaces).
func FormatClaimText(innerXML string) string {
	start := time.Now()
	var parseErr error
	defer func() {
		if Metrics != nil {
			Metrics.ObserveDuration("uspto.claims.parse", time.Since(start), parseErr != nil)
			Metrics.IncCounter("uspto.claims.parse.count", 1)
			if parseErr != nil {
				Metrics.IncCounter("uspto.claims.parse.fallback", 1)
			}
		}
	}()

	// Wrap in a dummy root tag to ensure it's a valid single-rooted XML fragment.
	wrapped := "<root>" + innerXML + "</root>"
	dec := xml.NewDecoder(strings.NewReader(wrapped))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity

	var buf strings.Builder
	claimTextDepth := 0
	var currentText strings.Builder

	writeTextLine := func(text string, depth int) {
		trimmed := collapseWhitespace(text)
		if trimmed == "" {
			return
		}
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		if depth > 1 {
			// Indent beyond the root claim-text
			buf.WriteString(strings.Repeat(" ", (depth-1)*ClaimIndentSize))
		}
		buf.WriteString(trimmed)
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			parseErr = err
			// On any XML parsing error, fallback to permissive claims formatting
			return permissiveFormatClaim(innerXML)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "claim-text" {
				writeTextLine(currentText.String(), claimTextDepth)
				currentText.Reset()
				claimTextDepth++
			}
		case xml.CharData:
			currentText.Write(t)
		case xml.EndElement:
			if t.Name.Local == "claim-text" {
				writeTextLine(currentText.String(), claimTextDepth)
				currentText.Reset()
				if claimTextDepth > 0 {
					claimTextDepth--
				}
			}
		}
	}

	res := buf.String()
	if res == "" {
		return permissiveFormatClaim(innerXML)
	}
	return res
}

// permissiveFormatClaim formats claim text permissively, preserving nested claim-text indentations and entities.
func permissiveFormatClaim(innerXML string) string {
	var buf strings.Builder
	claimTextDepth := 0
	var currentText strings.Builder

	writeTextLine := func(text string, depth int) {
		trimmed := collapseWhitespace(text)
		if trimmed == "" {
			return
		}
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		if depth > 1 {
			buf.WriteString(strings.Repeat(" ", (depth-1)*ClaimIndentSize))
		}
		buf.WriteString(trimmed)
	}

	i := 0
	n := len(innerXML)
	for i < n {
		if innerXML[i] == '<' {
			j := i + 1
			for j < n && innerXML[j] != '>' {
				j++
			}
			if j >= n {
				currentText.WriteByte('<')
				i++
				continue
			}
			tagContent := innerXML[i+1 : j]
			i = j + 1

			tagContent = strings.TrimSpace(tagContent)
			if tagContent == "" {
				continue
			}
			if tagContent[0] == '?' || tagContent[0] == '!' {
				continue
			}

			isEnd := tagContent[0] == '/'
			if isEnd {
				tagName := strings.ToLower(strings.TrimSpace(tagContent[1:]))
				if tagName == "claim-text" {
					writeTextLine(currentText.String(), claimTextDepth)
					currentText.Reset()
					if claimTextDepth > 0 {
						claimTextDepth--
					}
				}
			} else {
				parts := strings.Fields(tagContent)
				tagName := strings.ToLower(parts[0])
				if tagName == "claim-text" {
					writeTextLine(currentText.String(), claimTextDepth)
					currentText.Reset()
					claimTextDepth++
				}
			}
		} else {
			currentText.WriteByte(innerXML[i])
			i++
		}
	}
	writeTextLine(currentText.String(), claimTextDepth)
	return html.UnescapeString(buf.String())
}

type USPTOGrantClaim struct {
	ID    string `xml:"id,attr"`
	Num   string `xml:"num,attr"`
	Inner string `xml:",innerxml"`
}

// Text strips XML markup from a single claim's body, preserving hierarchical nested indentations.
func (c USPTOGrantClaim) Text() string { return FormatClaimText(c.Inner) }

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
