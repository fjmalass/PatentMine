package uspto

import (
	"strings"
	"testing"
)

// oldFormatGrant uses the pre-2013 DTD element names
// (<references-cited>/<citation>), matching the 10035937_07034695 sample.
const oldFormatGrant = `<?xml version="1.0" encoding="UTF-8"?>
<us-patent-grant>
<us-bibliographic-data-grant>
<publication-reference><document-id>
<country>US</country><doc-number>07034695</doc-number><kind>B2</kind><date>20060425</date>
</document-id></publication-reference>
<application-reference appl-type="utility"><document-id>
<country>US</country><doc-number>10035937</doc-number><date>20011226</date>
</document-id></application-reference>
<references-cited>
<citation>
<patcit num="00001"><document-id>
<country>US</country><doc-number>5351059</doc-number><kind>A</kind><name>Tsuyuki</name><date>19940900</date>
</document-id></patcit>
<category>cited by examiner</category>
</citation>
<citation>
<patcit num="00002"><document-id>
<country>US</country><doc-number>5381129</doc-number><kind>A</kind><name>Boardman</name><date>19950100</date>
</document-id></patcit>
<category>cited by other</category>
</citation>
<citation>
<nplcit num="00003"><othercit>Smith, J.   "GPS   correction techniques", IEEE 2001.</othercit></nplcit>
<category>cited by applicant</category>
</citation>
</references-cited>
</us-bibliographic-data-grant>
</us-patent-grant>`

// newFormatGrant uses the later DTD element names
// (<us-references-cited>/<us-citation>) to prove both wire shapes parse.
const newFormatGrant = `<?xml version="1.0"?>
<us-patent-grant>
<us-bibliographic-data-grant>
<publication-reference><document-id>
<country>US</country><doc-number>09999999</doc-number><kind>B2</kind>
</document-id></publication-reference>
<application-reference appl-type="utility"><document-id>
<country>US</country><doc-number>14000000</doc-number>
</document-id></application-reference>
<us-references-cited>
<us-citation>
<patcit num="00001"><document-id>
<country>US</country><doc-number>8000000</doc-number><kind>B2</kind><name>Doe</name>
</document-id></patcit>
<category>cited by examiner</category>
</us-citation>
</us-references-cited>
</us-bibliographic-data-grant>
</us-patent-grant>`

func TestParseCitationsOldFormat(t *testing.T) {
	res, err := ParseCitations(strings.NewReader(oldFormatGrant))
	if err != nil {
		t.Fatalf("ParseCitations: %v", err)
	}
	if res.ApplicationNumber != "10035937" {
		t.Errorf("application number = %q, want 10035937", res.ApplicationNumber)
	}
	if res.RecordNumber != "07034695" {
		t.Errorf("record number = %q, want 07034695", res.RecordNumber)
	}
	rec, ok := res.Record()
	if !ok {
		t.Fatalf("Record() not ok")
	}
	if rec.Serial != "07034695" {
		t.Errorf("record serial = %q, want 07034695", rec.Serial)
	}
	if len(res.Citations) != 3 {
		t.Fatalf("citations = %d, want 3", len(res.Citations))
	}
	if res.PatentCount != 2 || res.NPLCount != 1 {
		t.Errorf("patent/npl = %d/%d, want 2/1", res.PatentCount, res.NPLCount)
	}
	if res.ExaminerCount != 1 || res.ApplicantCount != 1 || res.OtherCount != 1 {
		t.Errorf("examiner/applicant/other = %d/%d/%d, want 1/1/1",
			res.ExaminerCount, res.ApplicantCount, res.OtherCount)
	}

	first := res.Citations[0]
	if first.CitationType != CitationTypePatent || first.CitedDocNumber != "5351059" || first.CitedName != "Tsuyuki" {
		t.Errorf("first citation = %+v, want patent 5351059 Tsuyuki", first)
	}
	npl := res.Citations[2]
	if npl.CitationType != CitationTypeNPL {
		t.Errorf("third citation type = %q, want npl", npl.CitationType)
	}
	if strings.Contains(npl.NPLText, "  ") {
		t.Errorf("npl text not space-collapsed: %q", npl.NPLText)
	}
}

func TestParseCitationsNewFormat(t *testing.T) {
	res, err := ParseCitations(strings.NewReader(newFormatGrant))
	if err != nil {
		t.Fatalf("ParseCitations: %v", err)
	}
	if len(res.Citations) != 1 {
		t.Fatalf("citations = %d, want 1", len(res.Citations))
	}
	if res.Citations[0].CitedDocNumber != "8000000" {
		t.Errorf("cited doc = %q, want 8000000", res.Citations[0].CitedDocNumber)
	}
	if res.ApplicationNumber != "14000000" {
		t.Errorf("application number = %q, want 14000000", res.ApplicationNumber)
	}
}
