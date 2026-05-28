package crawl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/uspto"
)

// Metrics is wired up by the engine to track google crawl full text parsing telemetry.
var Metrics *observability.Metrics

// googleMinInterval keeps requests to Google Patents polite.
const googleMinInterval = 2 * time.Second

// googlePatentURLPrefix is the base of a Google Patents patent page URL.
const googlePatentURLPrefix = "https://patents.google.com/patent/"

// googleLimiter and googleClient are shared by every path that talks to Google
// Patents — the bibliographic Source and FetchFullText alike — so the polite
// request interval and the timeout/body caps apply to all Google traffic, not
// just the crawl Source.
var (
	googleLimiter = newLimiter(googleMinInterval)
	googleClient  = &http.Client{Timeout: httpTimeout}
)

// googlePatentURL returns the Google Patents page URL for a patent number.
func googlePatentURL(n domain.PatentNumber) string {
	return googlePatentURLPrefix + n.Normalized() + "/en"
}

// NewGoogleSource builds a Source backed by Google Patents. It shares the
// package-level limiter and client so it cannot outpace FetchFullText.
func NewGoogleSource() Source {
	return &httpSource{
		name:    domain.SourceGoogle,
		client:  googleClient,
		limiter: googleLimiter,
		urlFor:  googlePatentURL,
		parse:   parseGoogle,
	}
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
		return Result{}, fmt.Errorf("crawl/google: parse HTML: %w", err)
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
		Classifications: googleClassifications(doc),
		FetchState:      domain.FetchCached,
		Source:          domain.SourceGoogle,
		FetchedAt:       time.Now().UTC(),
		ApplicationDate: googleAttrDate(doc, "time[itemprop='filingDate']"),
		PublicationDate: googleAttrDate(doc, "time[itemprop='publicationDate']"),
		GrantDate:       googleAttrDate(doc, "time[itemprop='grantDate']"),
	}

	patent.FirstClaim = clean(googleText(doc.Selection, "section[itemprop='claims'] .claim", ".claims .claim"))
	patent.SourceURL = googlePatentURL(number)
	// Try to find the exact "Anticipated expiration" or "Adjusted expiration" date from the events in the document
	var anticipatedExpiration time.Time
	doc.Find("span[itemprop='type'], span[itemprop='title']").Each(func(_ int, s *goquery.Selection) {
		text := strings.ToLower(strings.TrimSpace(s.Text()))
		if strings.Contains(text, "anticipated expiration") || strings.Contains(text, "adjusted expiration") || text == "expiration" {
			parent := s.Parent()
			if dateStr, ok := parent.Find("time[itemprop='date']").First().Attr("datetime"); ok {
				if t := parseGoogleDate(dateStr); !t.IsZero() {
					anticipatedExpiration = t
				}
			}
			if anticipatedExpiration.IsZero() {
				if dateStr := parent.Find("time").First().Text(); dateStr != "" {
					if t := parseGoogleDate(dateStr); !t.IsZero() {
						anticipatedExpiration = t
					}
				}
			}
		}
	})

	if anticipatedExpiration.IsZero() {
		if t := googleAttrDate(doc, "time[itemprop='anticipatedExpiration']"); !t.IsZero() {
			anticipatedExpiration = t
		}
	}

	if !anticipatedExpiration.IsZero() {
		patent.ExpirationDate = anticipatedExpiration
		patent.ExpirationSource = "anticipated"
	} else if !patent.ApplicationDate.IsZero() {
		// Standard U.S. term rule: 20 years from ApplicationDate (FilingDate)
		patent.ExpirationDate = patent.ApplicationDate.AddDate(20, 0, 0)
		patent.ExpirationSource = domain.ExpirationEstimated
	} else if base := firstNonZeroTime(patent.PublicationDate, patent.GrantDate); !base.IsZero() {
		patent.ExpirationDate = base.AddDate(20, 0, 0)
		patent.ExpirationSource = domain.ExpirationEstimated
	}

	document := domain.Document{
		Number: number,
		Stage:  domain.GuessStage(number),
		Dated:  firstNonZeroTime(patent.GrantDate, patent.PublicationDate, patent.ApplicationDate),
	}
	res := Result{
		Patent:    patent,
		Documents: []domain.Document{document},
		Relations: googleRelations(doc, number),
		AuthorityIdentifiers: []domain.AuthorityIdentifier{{
			Authority:      "GOOGLE",
			IdentifierType: string(domain.GuessStage(number)),
			Identifier:     number.Normalized(),
			RawIdentifier:  number.Normalized(),
			RecordNumber:   number,
			DocumentNumber: number.Normalized(),
			Country:        number.Country,
			Kind:           number.Kind,
			Source:         string(domain.SourceGoogle),
			Confidence:     80,
		}},
		SourceSnapshots: []domain.SourceSnapshot{{
			ID:             snapshotID(string(domain.SourceGoogle), number.Normalized(), body),
			PatentNumber:   number,
			Source:         "google",
			SourceRecordID: number.Normalized(),
			SourceURL:      patent.SourceURL,
			FetchedAt:      encodeRFC3339(patent.FetchedAt),
			PayloadKind:    "html",
			PayloadHash:    payloadHash(body),
			ResponseBytes:  int64(len(body)),
			SummaryJSON:    `{"parser":"google"}`,
		}},
	}

	extraDocs, extraIds := extractAdditionalGoogleDocuments(number, doc)
	res.Documents = append(res.Documents, extraDocs...)
	res.AuthorityIdentifiers = append(res.AuthorityIdentifiers, extraIds...)

	return res, nil
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
func googleTexts(s *goquery.Selection, selectors ...string) []domain.Inventor {
	seen := map[string]bool{}
	var out []domain.Inventor
	for _, selector := range selectors {
		s.Find(selector).Each(func(_ int, el *goquery.Selection) {
			text := clean(el.Text())
			if text != "" && !seen[text] {
				seen[text] = true
				out = append(out, domain.Inventor(text))
			}
		})
	}
	return out
}

// googleClassifications returns the parsed and de-duplicated classification codes for a patent.
func googleClassifications(doc *goquery.Document) []string {
	seen := map[string]bool{}
	var out []string

	// 1. Try finding elements with itemprop="classification"
	doc.Find("[itemprop='classification']").Each(func(_ int, s *goquery.Selection) {
		// It might be a meta tag with content attribute:
		if content, ok := s.Attr("content"); ok {
			c := cleanClassification(content)
			if c != "" && !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
		// It might contain text directly:
		text := cleanClassification(s.Text())
		if text != "" && !seen[text] {
			seen[text] = true
			out = append(out, text)
		}
	})

	// 2. Also look inside sub-elements of itemprop="classification" or general itemprop="code" / itemprop="Code"
	doc.Find("[itemprop='code'], [itemprop='Code']").Each(func(_ int, s *goquery.Selection) {
		text := cleanClassification(s.Text())
		if text != "" && !seen[text] {
			seen[text] = true
			out = append(out, text)
		}
	})

	return out
}

// cleanClassification strips whitespace and prefixes from classification strings.
func cleanClassification(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), "") // Remove all spaces
	if s == "" {
		return ""
	}
	// Strip prefixes like CPC/IPC/US (case-insensitive) followed by : or /
	lower := strings.ToLower(s)
	for _, prefix := range []string{"cpc:", "ipc:", "us:", "cpc/", "ipc/", "us/"} {
		if strings.HasPrefix(lower, prefix) {
			s = s[len(prefix):]
			lower = lower[len(prefix):]
		}
	}
	s = strings.Trim(s, " /:-")
	s = strings.ToUpper(s)
	if len(s) < 3 || len(s) > 25 {
		return ""
	}
	return s
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

// ParseAllClaims extracts every numbered claim from a Google Patents HTML body.
// It returns the claims in document order with their numbers stripped from the
// text and stored as ClaimSection.Number.
func ParseAllClaims(body []byte) ([]domain.ClaimSection, error) {
	start := time.Now()
	var parseErr error
	defer func() {
		if Metrics != nil {
			Metrics.ObserveDuration("crawl.google.claims.parse", time.Since(start), parseErr != nil)
			Metrics.IncCounter("crawl.google.claims.parse.count", 1)
		}
	}()

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		parseErr = err
		return nil, fmt.Errorf("crawl/google: parse claims HTML: %w", err)
	}
	var claims []domain.ClaimSection
	doc.Find("section[itemprop='claims'] .claim, .claims .claim").Each(func(i int, s *goquery.Selection) {
		raw := strings.TrimSpace(s.Text())
		if raw == "" {
			return
		}
		num := i + 1
		// Try to extract the claim number from the text ("1. ...", "Claim 1. ...")
		cleaned := clean(raw)
		claims = append(claims, domain.ClaimSection{Number: num, Text: cleaned})
	})
	if len(claims) == 0 {
		parseErr = fmt.Errorf("no claims found")
		return nil, fmt.Errorf("crawl/google: no claims found")
	}
	return claims, nil
}

// ParseDescription extracts the disclosure paragraphs from a Google Patents
// HTML body. Google numbers paragraphs with a "num" attribute (e.g. "0045").
// It returns an empty slice (not an error) when the page has no description.
func ParseDescription(body []byte) ([]domain.DescriptionParagraph, error) {
	start := time.Now()
	var parseErr error
	defer func() {
		if Metrics != nil {
			Metrics.ObserveDuration("crawl.google.description.parse", time.Since(start), parseErr != nil)
			Metrics.IncCounter("crawl.google.description.parse.count", 1)
		}
	}()

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		parseErr = err
		return nil, fmt.Errorf("crawl/google: parse description HTML: %w", err)
	}
	var paragraphs []domain.DescriptionParagraph
	doc.Find("section[itemprop='description'] .description-paragraph, .description .description-paragraph").Each(func(i int, s *goquery.Selection) {
		// Use HTML to preserve nested list structures and line breaks, formatted generically
		htmlStr, htmlErr := s.Html()
		var text string
		if htmlErr == nil {
			text = uspto.FormatDescriptionText(htmlStr)
		} else {
			text = clean(s.Text())
		}

		if text == "" {
			return
		}
		num, _ := s.Attr("num")
		paragraphs = append(paragraphs, domain.DescriptionParagraph{
			Number: strings.TrimSpace(num),
			Text:   text,
		})
	})
	return paragraphs, nil
}

// FetchFullText fetches a patent's full claims and disclosure text from Google
// Patents. It goes through the shared Google limiter and client, so it obeys
// the same polite interval, timeout, and body cap as the crawl Source. Every
// claim section is parsed; disclosure paragraphs are best-effort and may be
// absent.
func FetchFullText(ctx context.Context, number domain.PatentNumber) (*domain.FullText, error) {
	if err := googleLimiter.wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googlePatentURL(number), nil)
	if err != nil {
		return nil, fmt.Errorf("crawl/google: build full-text request: %w", err)
	}
	resp, err := googleClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crawl/google: fetch full text: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crawl/google: full text: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("crawl/google: read body: %w", err)
	}
	claims, err := ParseAllClaims(body)
	if err != nil {
		return nil, err
	}
	paragraphs, _ := ParseDescription(body)
	return &domain.FullText{
		Number:     number,
		Claims:     claims,
		Paragraphs: paragraphs,
	}, nil
}

func extractAdditionalGoogleDocuments(recordNumber domain.PatentNumber, doc *goquery.Document) ([]domain.Document, []domain.AuthorityIdentifier) {
	var docs []domain.Document
	var ids []domain.AuthorityIdentifier
	seenDocs := make(map[domain.PatentNumber]bool)

	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		name, _ := s.Attr("name")
		content, _ := s.Attr("content")
		scheme, _ := s.Attr("scheme")
		if content == "" {
			return
		}
		content = strings.TrimSpace(content)

		// Parse using domain.ParsePatentNumber
		if name == "citation_patent_application_number" || (name == "DC.relation" && scheme == "application") {
			if num, err := domain.ParsePatentNumber(content); err == nil && !seenDocs[num] {
				seenDocs[num] = true
				docs = append(docs, domain.Document{
					Number: num,
					Stage:  domain.StageApplication,
				})
				if num.Country != "" {
					// Use country-code authority + bare serial so USPTO's
					// resolveByAuthority can latch onto this record when it
					// ingests the same application (USPTO stores "17812078",
					// not "US17812078", as the identifier).
					ids = append(ids, domain.AuthorityIdentifier{
						Authority:      num.Country,
						IdentifierType: "application",
						Identifier:     num.Serial,
						RawIdentifier:  content,
						RecordNumber:   recordNumber,
						DocumentNumber: num.Normalized(),
						Country:        num.Country,
						Kind:           num.Kind,
						Source:         string(domain.SourceGoogle),
						Confidence:     80,
					})
					if Metrics != nil {
						Metrics.IncCounter("crawl.google.authority_id.application_crossref.count", 1)
					}
				}
			}
		}
		if name == "citation_patent_publication_number" || name == "citation_patent_number" || (name == "DC.relation" && scheme == "patent") {
			if num, err := domain.ParsePatentNumber(content); err == nil && !seenDocs[num] {
				seenDocs[num] = true
				stage := domain.GuessStage(num)
				docs = append(docs, domain.Document{
					Number: num,
					Stage:  stage,
				})
				ids = append(ids, domain.AuthorityIdentifier{
					Authority:      "GOOGLE",
					IdentifierType: string(stage),
					Identifier:     num.Normalized(),
					RawIdentifier:  content,
					RecordNumber:   recordNumber,
					DocumentNumber: num.Normalized(),
					Country:        num.Country,
					Kind:           num.Kind,
					Source:         string(domain.SourceGoogle),
					Confidence:     80,
				})
			}
		}
	})
	return docs, ids
}

// clean collapses runs of whitespace in scraped text to single spaces.
func clean(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
