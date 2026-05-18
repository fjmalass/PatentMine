package ingest

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"patentmine/internal/domain"
)

// googleMinInterval keeps requests to Google Patents polite.
const googleMinInterval = 2 * time.Second

// NewGoogleSource builds a Source backed by Google Patents.
func NewGoogleSource() Source {
	return newHTTPSource(
		domain.SourceGoogle,
		googleMinInterval,
		func(n domain.PatentNumber) string {
			return "https://patents.google.com/patent/" + n.Normalized() + "/en"
		},
		parseGoogle,
	)
}

var (
	// googlePatentURLRe pulls the patent number out of a /patent/<num>/ path.
	googlePatentURLRe = regexp.MustCompile(`(?i)/patent/([^/?#]+)`)
	// googlePatentIDRe matches a bare patent identifier in scraped text.
	googlePatentIDRe = regexp.MustCompile(`(?i)\b[A-Z]{2,}[0-9][A-Z0-9]*\b`)
)

// googleDateLayouts are the date forms seen on Google Patents.
var googleDateLayouts = []string{"2006-01-02", "2006/01/02"}

// parseGoogle extracts a patent's record from a Google Patents HTML page. The
// bibliographic fields come from the page's itemprop microdata (with meta-tag
// fallbacks); citation and family edges come from the page body, so a fetched
// patent's Citations / Cited-by / family views populate.
func parseGoogle(number domain.PatentNumber, body []byte) (Result, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("ingest/google: parse HTML: %w", err)
	}
	title := clean(googleText(doc.Selection, "span[itemprop='title']", "meta[name='DC.title']", "title"))
	if title == "" {
		// Not a parseable patent page — let the registry fall through.
		return Result{}, ErrNotAvailable
	}

	patent := domain.Patent{
		Number:          number,
		DisplayNumber:   number,
		Title:           title,
		Abstract:        clean(googleText(doc.Selection, "div.abstract", "section[itemprop='abstract']", "meta[name='DC.description']")),
		Assignee:        clean(googleText(doc.Selection, "dd[itemprop='assigneeOriginal']", "span[itemprop='assigneeOriginal']", "dd[itemprop='assigneeCurrent']")),
		Inventors:       googleTexts(doc.Selection, "dd[itemprop='inventor']", "span[itemprop='inventor']"),
		FetchState:      domain.FetchCached,
		Source:          domain.SourceGoogle,
		FetchedAt:       time.Now().UTC(),
		ApplicationDate: googleAttrDate(doc, "time[itemprop='filingDate']"),
		PublicationDate: googleAttrDate(doc, "time[itemprop='publicationDate']"),
		GrantDate:       googleAttrDate(doc, "time[itemprop='grantDate']"),
	}

	patent.FirstClaim = clean(googleText(doc.Selection, "section[itemprop='claims'] .claim", ".claims .claim"))
	patent.SourceURL = "https://patents.google.com/patent/" + number.Normalized() + "/en"
	// Google does not state a definitive expiration; estimate it as 20 years
	// from the earliest of publication or grant.
	if base := firstNonZeroTime(patent.PublicationDate, patent.GrantDate); !base.IsZero() {
		patent.ExpirationDate = base.AddDate(20, 0, 0)
		patent.ExpirationSource = domain.ExpirationEstimated
	}

	document := domain.Document{
		Number: number,
		Stage:  domain.GuessStage(number),
		Dated:  firstNonZeroTime(patent.GrantDate, patent.PublicationDate, patent.ApplicationDate),
	}
	return Result{
		Patent:    patent,
		Documents: []domain.Document{document},
		Relations: googleRelations(doc, number),
	}, nil
}

// googleRelations extracts every citation and family edge for number. The
// crawler overwrites Relation.From with the resolved record number, so only To
// and Kind need to be correct here.
func googleRelations(doc *goquery.Document, number domain.PatentNumber) []domain.Relation {
	var out []domain.Relation
	seen := map[string]bool{}
	add := func(to domain.PatentNumber, kind domain.RelationKind) {
		if to.IsZero() || to.Normalized() == number.Normalized() {
			return
		}
		key := string(kind) + "\x00" + to.Normalized()
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, domain.Relation{From: number, To: to, Kind: kind})
	}

	// Citations: patents this one cites (backward) and patents citing it (forward).
	googleCitationRows(doc, "[itemprop='backwardReferences'], #patentCitations, #backwardReferences",
		func(n domain.PatentNumber) { add(n, domain.RelationCites) })
	googleCitationRows(doc, "[itemprop='forwardReferences'], #citedBy, #forwardReferences",
		func(n domain.PatentNumber) { add(n, domain.RelationCitedBy) })

	// Family: parentApps list patents this one continues from; priority and
	// continuation apps list patents that continue from this one.
	doc.Find("[itemprop='parentApps']").Each(func(_ int, row *goquery.Selection) {
		if n, ok := googleRowNumber(row); ok {
			add(n, domain.RelationParent)
		}
	})
	for _, selector := range []string{"[itemprop='priorityApps']", "[itemprop='continuationApps']"} {
		doc.Find(selector).Each(func(_ int, row *goquery.Selection) {
			if n, ok := googleRowNumber(row); ok {
				add(n, domain.RelationChild)
			}
		})
	}
	return out
}

// googleCitationRows scans every citation section matched by selector and emits
// each patent number it finds, by link or by bare text.
func googleCitationRows(doc *goquery.Document, selector string, emit func(domain.PatentNumber)) {
	doc.Find(selector).Each(func(_ int, section *goquery.Selection) {
		section.Find("tr, .patent-result, .result, .citation, [itemprop='publicationNumber'], a[href*='/patent/']").Each(func(_ int, s *goquery.Selection) {
			if href, ok := s.Attr("href"); ok && strings.Contains(href, "/patent/") {
				if n, ok := googleParseNumber(googlePatentNumberFromURL(href)); ok {
					emit(n)
				}
			}
			text := strings.TrimSpace(s.Text())
			if len(text) > 4 && len(text) < 16 && !strings.Contains(text, " ") {
				if n, ok := googleParseNumber(text); ok {
					emit(n)
				}
			}
		})
	})
}

// googleRowNumber returns the patent number a family-table row refers to.
func googleRowNumber(row *goquery.Selection) (domain.PatentNumber, bool) {
	if n, ok := googleParseNumber(row.Find("[itemprop='representativePublication']").First().Text()); ok {
		return n, true
	}
	var found domain.PatentNumber
	var ok bool
	row.Find("a[href*='/patent/']").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		if href, hrefOK := a.Attr("href"); hrefOK {
			if n, parsed := googleParseNumber(googlePatentNumberFromURL(href)); parsed {
				found, ok = n, true
				return false
			}
		}
		return true
	})
	return found, ok
}

// googleText returns the first non-empty value among selectors: a meta tag's
// content attribute, or an element's text.
func googleText(s *goquery.Selection, selectors ...string) string {
	for _, selector := range selectors {
		found := s.Find(selector).First()
		if found.Length() == 0 {
			continue
		}
		if content, ok := found.Attr("content"); ok && strings.TrimSpace(content) != "" {
			return content
		}
		if text := found.Text(); strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

// googleTexts returns the de-duplicated text of every element matched by any
// selector.
func googleTexts(s *goquery.Selection, selectors ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, selector := range selectors {
		s.Find(selector).Each(func(_ int, el *goquery.Selection) {
			text := clean(el.Text())
			if text != "" && !seen[text] {
				seen[text] = true
				out = append(out, text)
			}
		})
	}
	return out
}

// googleAttrDate parses the datetime attribute of the first matched element.
func googleAttrDate(doc *goquery.Document, selector string) time.Time {
	value, _ := doc.Find(selector).First().Attr("datetime")
	return parseGoogleDate(value)
}

// googleParseNumber normalizes a scraped string and parses it as a patent
// number, reporting false when it is not a valid number.
func googleParseNumber(raw string) (domain.PatentNumber, bool) {
	id := googleNormalizeID(raw)
	if id == "" {
		return domain.PatentNumber{}, false
	}
	n, err := domain.ParsePatentNumber(id)
	if err != nil {
		return domain.PatentNumber{}, false
	}
	return n, true
}

// googleNormalizeID strips scraped-text noise and returns a bare patent id.
func googleNormalizeID(value string) string {
	value = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(value, " ", ""), ",", ""))
	if value == "" {
		return ""
	}
	if strings.Contains(value, "/PATENT/") {
		return googlePatentNumberFromURL(value)
	}
	return strings.TrimSpace(googlePatentIDRe.FindString(value))
}

// googlePatentNumberFromURL pulls the patent number from a /patent/<num>/ path.
func googlePatentNumberFromURL(rawURL string) string {
	m := googlePatentURLRe.FindStringSubmatch(rawURL)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// parseGoogleDate parses a date in any layout Google uses, or the zero time.
func parseGoogleDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range googleDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// firstNonZeroTime returns the first non-zero time, or the zero time.
func firstNonZeroTime(times ...time.Time) time.Time {
	for _, t := range times {
		if !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

// clean collapses runs of whitespace in scraped text to single spaces.
func clean(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
