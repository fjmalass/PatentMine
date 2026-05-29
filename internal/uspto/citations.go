package uspto

import (
	"encoding/xml"
	"io"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

// Citation type values stored on domain.USPTOGrantCitation.CitationType. Named
// so the patent/NPL branch never compares against a bare literal.
const (
	CitationTypePatent = "patent"
	CitationTypeNPL    = "npl"
)

// Citation category fragments. The USPTO writes free text such as
// "cited by examiner" / "cited by applicant" / "cited by other"; we classify
// on the substring rather than an exact match so minor wording drift is
// tolerated.
const (
	categoryExaminerMarker  = "examiner"
	categoryApplicantMarker = "applicant"
)

// Citation telemetry metric names. Kept together so the engine, the CLI, and
// any dashboard reference the same strings (no magic metric keys).
const (
	MetricCitationParseDuration = "uspto.citation.parse"
	MetricCitationParseRuns     = "uspto.citation.parse.count"
	MetricCitationParsed        = "uspto.citation.parsed"
	MetricCitationPatent        = "uspto.citation.patent"
	MetricCitationNPL           = "uspto.citation.npl"
	MetricCitationExaminer      = "uspto.citation.examiner"
	MetricCitationOther         = "uspto.citation.other"
	MetricCitationSaveDuration  = "uspto.citation.save"
	MetricCitationRows          = "uspto.citation.rows"
	MetricCitationGraphEdges    = "uspto.citation.graph_edges"
)

// CitationParseResult is what ParseCitations extracts from a grant/pgpub XML
// file: the citing document's identifiers plus the parsed citation rows and a
// breakdown of how many of each kind were seen (for telemetry and reporting).
type CitationParseResult struct {
	ApplicationNumber string
	RecordNumber      string             // citing patent/publication doc number
	RecordRef         USPTOGrantDocRef   // citing publication-reference document-id
	Kind              string             // proto.USPTOXMLKind value: "grant" or "pgpub"
	Citations         []domain.USPTOGrantCitation
	PatentCount       int
	NPLCount          int
	ExaminerCount     int // cited by examiner
	ApplicantCount    int // cited by applicant
	OtherCount        int // everything else (incl. "cited by other")
}

// Record builds the citing patent number used as the owning record for the
// saved citation rows and citation-graph edges. It is reported (ok=false) when
// no usable document number was present.
func (r CitationParseResult) Record() (domain.PatentNumber, bool) {
	return patentNumberFromDocRef(r.RecordRef.Country, recordDocNumber(r), r.RecordRef.Kind)
}

func recordDocNumber(r CitationParseResult) string {
	if d := strings.TrimSpace(r.RecordRef.DocNumber); d != "" {
		return d
	}
	return strings.TrimSpace(r.RecordNumber)
}

// citationsBlock is the references-cited subtree. The DTD changed element
// names over time: pre-2013 grants use <references-cited>/<citation>, later
// grants use <us-references-cited>/<us-citation>. We accept both so the
// standalone loader works on the full archive, not just recent files.
type citationsBlock struct {
	Old []USPTOGrantCitation `xml:"citation"`
	New []USPTOGrantCitation `xml:"us-citation"`
}

func (b citationsBlock) all() []USPTOGrantCitation {
	if len(b.New) > 0 {
		return b.New
	}
	return b.Old
}

// ParseCitations reads only the references-cited block (and the bibliographic
// document identifiers that precede it) from a USPTO grant or pgpub XML stream.
// It deliberately does not decode claims, description, drawings, or parties:
// citations sit near the top of the bibliographic data, so a single forward
// token scan reaches them cheaply and stops.
func ParseCitations(r io.Reader) (res CitationParseResult, err error) {
	start := time.Now()
	defer func() {
		if Metrics == nil {
			return
		}
		Metrics.ObserveDuration(MetricCitationParseDuration, time.Since(start), err != nil)
		Metrics.IncCounter(MetricCitationParseRuns, 1)
		Metrics.IncCounter(MetricCitationParsed, int64(len(res.Citations)))
		Metrics.IncCounter(MetricCitationPatent, int64(res.PatentCount))
		Metrics.IncCounter(MetricCitationNPL, int64(res.NPLCount))
		Metrics.IncCounter(MetricCitationExaminer, int64(res.ExaminerCount))
		Metrics.IncCounter(MetricCitationOther, int64(res.OtherCount))
	}()

	dec := xml.NewDecoder(r)
	dec.Strict = false
	dec.Entity = xml.HTMLEntity

	for {
		tok, terr := dec.Token()
		if terr == io.EOF {
			break
		}
		if terr != nil {
			return res, terr
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "us-patent-grant":
			res.Kind = string(proto.USPTOXMLKindGrant)
		case "us-patent-application":
			res.Kind = string(proto.USPTOXMLKindPGPub)
		case "publication-reference":
			var ref struct {
				DocumentID USPTOGrantDocRef `xml:"document-id"`
			}
			if derr := dec.DecodeElement(&ref, &se); derr != nil {
				return res, derr
			}
			res.RecordRef = ref.DocumentID
			res.RecordNumber = ref.DocumentID.DocNumber
		case "application-reference":
			var ref USPTOGrantApplicationRef
			if derr := dec.DecodeElement(&ref, &se); derr != nil {
				return res, derr
			}
			res.ApplicationNumber = ref.DocumentID.DocNumber
		case "references-cited", "us-references-cited":
			var block citationsBlock
			if derr := dec.DecodeElement(&block, &se); derr != nil {
				return res, derr
			}
			res.Citations = res.Citations[:0]
			for i, c := range block.all() {
				dc := citationToDomain(c, res.ApplicationNumber, res.Kind, i)
				res.Citations = append(res.Citations, dc)
				tallyCitation(&res, dc)
			}
			// references-cited follows the identifiers we need; stop scanning.
			return res, nil
		}
	}
	return res, nil
}

func tallyCitation(res *CitationParseResult, c domain.USPTOGrantCitation) {
	if c.CitationType == CitationTypeNPL {
		res.NPLCount++
	} else {
		res.PatentCount++
	}
	switch {
	case strings.Contains(strings.ToLower(c.Category), categoryExaminerMarker):
		res.ExaminerCount++
	case strings.Contains(strings.ToLower(c.Category), categoryApplicantMarker):
		res.ApplicantCount++
	default:
		res.OtherCount++
	}
}

// citationToDomain maps one parsed XML citation to the domain row. It is the
// single source of truth for the patent-vs-NPL branching, shared by the full
// grant ingest (USPTOGrantToIngest) and the standalone citation loader so the
// two never drift.
func citationToDomain(c USPTOGrantCitation, appNum, kind string, ordinal int) domain.USPTOGrantCitation {
	ct := domain.USPTOGrantCitation{
		ApplicationNumber: appNum,
		Kind:              kind,
		Ordinal:           ordinal,
		Category:          c.Category,
		CPCText:           c.ClassificationCPCText,
		NationalCountry:   c.ClassificationNational.Country,
		NationalClass:     c.ClassificationNational.MainClassification,
	}
	if c.PatCit.DocumentID.DocNumber != "" {
		ct.CitationType = CitationTypePatent
		ct.CitationNum = c.PatCit.Num
		ct.CitedCountry = c.PatCit.DocumentID.Country
		ct.CitedDocNumber = c.PatCit.DocumentID.DocNumber
		ct.CitedKind = c.PatCit.DocumentID.Kind
		ct.CitedDate = c.PatCit.DocumentID.Date
		ct.CitedName = c.PatCit.DocumentID.Name
	} else {
		ct.CitationType = CitationTypeNPL
		ct.CitationNum = c.NPLCit.Num
		ct.NPLText = collapseSpaces(c.NPLCit.OtherCite)
	}
	return ct
}

// patentNumberFromDocRef builds a normalized patent number from the separate
// country/doc-number/kind fields a USPTO document-id carries, trying the most
// specific combination first. It mirrors the citation-graph resolver in the
// engine so a citing record and its cited records normalize the same way.
func patentNumberFromDocRef(country, doc, kind string) (domain.PatentNumber, bool) {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return domain.PatentNumber{}, false
	}
	country = strings.ToUpper(strings.TrimSpace(country))
	kind = strings.ToUpper(strings.TrimSpace(kind))
	for _, raw := range []string{country + doc + kind, country + doc, doc + kind, doc} {
		n, err := domain.ParsePatentNumber(raw)
		if err != nil {
			continue
		}
		if n.Country == "" {
			n.Country = country
		}
		if n.Kind == "" {
			n.Kind = kind
		}
		return n, true
	}
	return domain.PatentNumber{}, false
}
