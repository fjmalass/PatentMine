package importer

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"patentmine/internal/domain"
)

var patentNumberPattern = regexp.MustCompile(`(?i)/patent/([^/?#]+)`)
var patentIDPattern = regexp.MustCompile(`(?i)\b[A-Z]{2,}[0-9][A-Z0-9]*\b`)
var classificationCodePattern = regexp.MustCompile(`\b(?:[A-HY]\d{2}[A-Z](?:\d+(?:/\d+)?)?|[A-HY]\d{2}|\d{1,3}/[A-Za-z0-9.]+)\b`)
var datePattern = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)

func GooglePatentsURL(patentNumber string) (string, error) {
	number := strings.ToUpper(strings.TrimSpace(patentNumber))
	if number == "" {
		return "", fmt.Errorf("patent number is required")
	}
	if strings.Contains(number, "://") {
		return "", fmt.Errorf("use :import for full URLs or :add with a patent number")
	}
	if strings.ContainsAny(number, " /?#") {
		return "", fmt.Errorf("patent number must not contain spaces or URL separators")
	}
	escaped := url.PathEscape(number)
	return fmt.Sprintf("https://patents.google.com/patent/%s/en?oq=%s+", escaped, url.QueryEscape(number)), nil
}

func ImportGooglePatents(rawURL string, logger *slog.Logger) (domain.PatentBundle, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return domain.PatentBundle{}, fmt.Errorf("invalid URL: %w", err)
	}
	if !strings.Contains(parsed.Host, "patents.google.com") {
		return domain.PatentBundle{}, fmt.Errorf("only patents.google.com URLs are supported")
	}
	number := patentNumberFromURL(rawURL)
	if number == "" {
		return domain.PatentBundle{}, fmt.Errorf("could not determine patent number from URL")
	}

	logger.Info("google.import", "number", number, "url", rawURL)
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return domain.PatentBundle{}, err
	}
	req.Header.Set("User-Agent", "PatentMine sample importer")
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("google.import fetch failed", "number", number, "error", err)
		return domain.PatentBundle{}, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()
	logger.Debug("google.import response", "number", number, "status", resp.StatusCode)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return domain.PatentBundle{}, fmt.Errorf("fetch failed: HTTP %d", resp.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return domain.PatentBundle{}, fmt.Errorf("parse failed: %w", err)
	}

	patent := domain.Patent{
		Number:            domain.PatentNumber(number),
		Title:             clean(firstText(doc.Selection, "span[itemprop='title']", "meta[name='DC.title']", "title")),
		Abstract:          clean(firstText(doc.Selection, "div.abstract", "section[itemprop='abstract']", "abstract")),
		Assignee:          clean(firstText(doc.Selection, "dd[itemprop='assigneeOriginal']", "span[itemprop='assigneeOriginal']", "dd[itemprop='assigneeCurrent']")),
		Inventors:         texts(doc.Selection, "dd[itemprop='inventor']", "span[itemprop='inventor']"),
		PublicationDate:   attr(doc.Selection, "time[itemprop='publicationDate']", "datetime"),
		GrantDate:         attr(doc.Selection, "time[itemprop='grantDate']", "datetime"),
		SourceGoogleURL:   rawURL,
		ApplicationNumber: clean(firstText(doc.Selection, "span[itemprop='applicationNumber']", "meta[name='DC.description']")), // Fallback meta
		ApplicationDate:   attr(doc.Selection, "time[itemprop='filingDate']", "datetime"),
		PublicationNumber: clean(firstText(doc.Selection, "span[itemprop='publicationNumber']")),
		GrantNumber:       clean(firstText(doc.Selection, "span[itemprop='grantNumber']")),
		FirstClaim:        clean(doc.Find("section[itemprop='claims'] .claim").First().Text()),
	}
	if expirationDate := adjustedExpirationDate(doc); expirationDate != "" {
		patent.ExpirationDate = expirationDate
		patent.ExpirationSource = domain.ExpirationSourceImported
	} else if expirationDate := estimatedExpirationDate(patent.PublicationDate, patent.GrantDate); expirationDate != "" {
		patent.ExpirationDate = expirationDate
		patent.ExpirationSource = domain.ExpirationSourceEstimated
	}
	if patent.Title == "" {
		patent.Title = number
	}
	bundle := domain.PatentBundle{Patent: patent}
	if patent.Abstract != "" {
		bundle.Sections = append(bundle.Sections, domain.PatentTextSection{PatentNumber: domain.PatentNumber(number), SectionType: "abstract", Ordinal: 1, Text: patent.Abstract})
	}
	addSection(doc.Selection, &bundle, number, "claims", "section[itemprop='claims'] .claim, .claims .claim")
	addSection(doc.Selection, &bundle, number, "description", "section[itemprop='description'] p, .description p")
	extractClassifications(doc, &bundle, number, logger)
	bundle.Citations = extractCitationEdges(doc, number, logger)
	bundle.FamilyEdges = extractFamilyEdges(doc, number, logger)
	bundle.ExpectedCitations, bundle.ExpectedCitedBy = extractExpectedCitationCounts(doc, logger)
	bundle.Patent.ExpectedCitations = bundle.ExpectedCitations
	bundle.Patent.ExpectedCitedBy = bundle.ExpectedCitedBy

	bundle.References = append(bundle.References, domain.ReferenceEntry{PatentNumber: domain.PatentNumber(number), CitationLabel: fmt.Sprintf("%s, %s", number, patent.Title)})

	// Format counts for logging
	countStr := func(actual, expected int) string {
		eStr := "unkn"
		if expected >= 0 {
			eStr = fmt.Sprintf("%d", expected)
		}
		aStr := fmt.Sprintf("%d", actual)
		if actual == 0 && expected > 0 {
			aStr = "unkn"
		}
		if actual == 0 && expected <= 0 {
			aStr = "unkn"
			eStr = "unkn"
		}
		return fmt.Sprintf("%s/%s", aStr, eStr)
	}

	backActual, fwdActual := 0, 0
	for _, c := range bundle.Citations {
		if c.RelationType == domain.RelationCites {
			backActual++
		} else {
			fwdActual++
		}
	}

	logger.Info("google.import ok", "number", number, "citations", countStr(backActual, bundle.ExpectedCitations), "cited_by", countStr(fwdActual, bundle.ExpectedCitedBy), "family_edges", len(bundle.FamilyEdges), "sections", len(bundle.Sections), "classifications", len(bundle.Classifications))
	return bundle, nil
}

func extractExpectedCitationCounts(doc *goquery.Document, logger *slog.Logger) (int, int) {
	// Selectors for citation headers: "Patent Citations (190)", "Cited By (3)", "FAMILY CITES FAMILIES (13)"
	countPattern := regexp.MustCompile(`\((\d+)\)`)
	backward, forward := -1, -1

	doc.Find("h3, h2").Each(func(_ int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if strings.Contains(text, "Patent Citations") || strings.Contains(text, "FAMILY CITES FAMILIES") {
			matches := countPattern.FindStringSubmatch(text)
			if len(matches) > 1 {
				fmt.Sscanf(matches[1], "%d", &backward)
			}
		} else if strings.Contains(text, "Cited By") || strings.Contains(text, "FAMILIES CITING THIS FAMILY") {
			matches := countPattern.FindStringSubmatch(text)
			if len(matches) > 1 {
				fmt.Sscanf(matches[1], "%d", &forward)
			}
		}
	})

	return backward, forward
}

func extractClassifications(doc *goquery.Document, bundle *domain.PatentBundle, number string, logger *slog.Logger) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	before := len(bundle.Classifications)
	doc.Find("[itemprop='classifications'], [itemprop='classification'], classification-cpc, .classification-cpc, classification-item, .classification-item").Each(func(_ int, row *goquery.Selection) {
		code := clean(classificationField(row, "[itemprop='code']", "[itemprop='Code']", ".code"))
		description := clean(classificationField(row, "[itemprop='description']", "[itemprop='Description']", ".description"))

		if code == "" {
			code, description = classificationFromText(row.Text())
		}
		addClassification(bundle, domain.PatentNumber(number), code, description)
	})
	logger.Debug("google.classifications", "number", number, "count", len(bundle.Classifications)-before)
}

func classificationField(row *goquery.Selection, selectors ...string) string {
	for _, selector := range selectors {
		if row.Is(selector) {
			if content, ok := row.Attr("content"); ok {
				return content
			}
			return row.Text()
		}
		found := row.Find(selector).First()
		if found.Length() == 0 {
			continue
		}
		if content, ok := found.Attr("content"); ok {
			return content
		}
		if text := found.Text(); text != "" {
			return text
		}
	}
	return ""
}

func classificationFromText(text string) (string, string) {
	text = clean(text)
	match := classificationCodePattern.FindStringIndex(text)
	if match == nil || match[0] > 3 {
		return "", ""
	}
	code := text[match[0]:match[1]]
	description := strings.TrimSpace(text[match[1]:])
	description = strings.TrimLeft(description, "-—–: ")
	return code, clean(description)
}

func addClassification(bundle *domain.PatentBundle, number domain.PatentNumber, code, description string) {
	if strings.TrimSpace(code) == "" {
		return
	}
	cls := domain.ParseClassification(code)
	cls.PatentNumber = number
	cls.Description = description

	for i, existing := range bundle.Classifications {
		if existing.Code == cls.Code && existing.System == cls.System {
			if existing.Description == "" && cls.Description != "" {
				bundle.Classifications[i].Description = cls.Description
			}
			return
		}
	}
	bundle.Classifications = append(bundle.Classifications, cls)
}

func patentNumberFromURL(rawURL string) string {
	matches := patentNumberPattern.FindStringSubmatch(rawURL)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func firstText(s *goquery.Selection, selectors ...string) string {
	for _, selector := range selectors {
		found := s.Find(selector).First()
		if found.Length() == 0 {
			continue
		}
		if content, ok := found.Attr("content"); ok {
			return content
		}
		if text := found.Text(); text != "" {
			return text
		}
	}
	return ""
}

func attr(s *goquery.Selection, selector, name string) string {
	value, _ := s.Find(selector).First().Attr(name)
	return strings.TrimSpace(value)
}

func texts(s *goquery.Selection, selectors ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, selector := range selectors {
		s.Find(selector).Each(func(_ int, sel *goquery.Selection) {
			text := clean(sel.Text())
			if text != "" && !seen[text] {
				seen[text] = true
				out = append(out, text)
			}
		})
	}
	return out
}

func addSection(s *goquery.Selection, bundle *domain.PatentBundle, number string, sectionType, selector string) {
	ordinal := 1
	s.Find(selector).Each(func(_ int, sel *goquery.Selection) {
		text := clean(sel.Text())
		if text == "" {
			return
		}
		bundle.Sections = append(bundle.Sections, domain.PatentTextSection{PatentNumber: domain.PatentNumber(number), SectionType: sectionType, Ordinal: ordinal, Text: text})
		ordinal++
	})
}

// extractFamilyEdges parses the "Priority And Related Applications" tables from Google Patents HTML.
// parentApps rows → source is a child of the listed patent.
// priorityApps rows → source is a parent of the listed patent.
// Falls back to the legacy backwardReferencesFamily/forwardReferencesFamily itemprop selectors.
func extractFamilyEdges(doc *goquery.Document, source string, logger *slog.Logger) []domain.FamilyEdge {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	logger.Debug("google.extract_family", "source", source)
	seen := map[string]bool{}
	var edges []domain.FamilyEdge

	// representativePublication is the publication number within a parentApps/priorityApps row.
	// Falls back to the first patent href in the row if absent.
	extractRowNumber := func(row *goquery.Selection) string {
		if n := normalizePatentID(row.Find("[itemprop='representativePublication']").First().Text()); n != "" {
			return n
		}
		var n string
		row.Find("a[href*='/patent/']").EachWithBreak(func(_ int, a *goquery.Selection) bool {
			if href, ok := a.Attr("href"); ok {
				if candidate := normalizePatentID(patentNumberFromURL(href)); candidate != "" {
					n = candidate
					return false
				}
			}
			return true
		})
		return n
	}

	mapRelationType := func(raw string) string {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "divisional":
			return domain.FamilyRelationDivisional
		case "continuation-in-part", "cip":
			return domain.FamilyRelationCIP
		case "pct":
			return domain.FamilyRelationPCT
		default:
			return domain.FamilyRelationContinuation
		}
	}

	addEdge := func(e domain.FamilyEdge) {
		key := string(e.ParentNumber) + "\x00" + string(e.ChildNumber)
		if seen[key] || strings.EqualFold(string(e.ParentNumber), string(e.ChildNumber)) {
			return
		}
		if e.ParentNumber == "" || e.ChildNumber == "" {
			return
		}
		seen[key] = true
		edges = append(edges, e)
	}

	// parentApps: source continues from parent (source is child).
	doc.Find("[itemprop='parentApps']").Each(func(_ int, row *goquery.Selection) {
		parentNum := extractRowNumber(row)
		if parentNum == "" || strings.EqualFold(parentNum, source) {
			return
		}
		relRaw := row.Find("[itemprop='relationType']").First().Text()
		addEdge(domain.FamilyEdge{
			ParentNumber: domain.PatentNumber(parentNum),
			ChildNumber:  domain.PatentNumber(source),
			RelationType: mapRelationType(relRaw),
		})
	})

	// priorityApps: source is the priority parent; these apps continue from source (source is parent).
	doc.Find("[itemprop='priorityApps']").Each(func(_ int, row *goquery.Selection) {
		childNum := extractRowNumber(row)
		if childNum == "" || strings.EqualFold(childNum, source) {
			return
		}
		relRaw := row.Find("[itemprop='relationType']").First().Text()
		addEdge(domain.FamilyEdge{
			ParentNumber: domain.PatentNumber(source),
			ChildNumber:  domain.PatentNumber(childNum),
			RelationType: mapRelationType(relRaw),
		})
	})

	// continuationApps: source is the parent; these are continuation/related apps filed after source.
	doc.Find("[itemprop='continuationApps']").Each(func(_ int, row *goquery.Selection) {
		childNum := extractRowNumber(row)
		if childNum == "" || strings.EqualFold(childNum, source) {
			return
		}
		relRaw := row.Find("[itemprop='relationType']").First().Text()
		addEdge(domain.FamilyEdge{
			ParentNumber: domain.PatentNumber(source),
			ChildNumber:  domain.PatentNumber(childNum),
			RelationType: mapRelationType(relRaw),
		})
	})

	// Legacy fallback: older Google Patents pages use these itemprop values.
	if len(edges) == 0 {
		legacySeen := map[string]bool{}
		collectLegacy := func(selector string) []string {
			var numbers []string
			doc.Find(selector).Each(func(_ int, row *goquery.Selection) {
				row.Find("[itemprop='publicationNumber'], a[href*='/patent/']").Each(func(_ int, s *goquery.Selection) {
					var n string
					if href, ok := s.Attr("href"); ok {
						n = normalizePatentID(patentNumberFromURL(href))
					}
					if n == "" {
						n = normalizePatentID(s.Text())
					}
					if n != "" && !strings.EqualFold(n, source) && !legacySeen[n] {
						legacySeen[n] = true
						numbers = append(numbers, n)
					}
				})
			})
			return numbers
		}
		for _, child := range collectLegacy("[itemprop='forwardReferencesFamily']") {
			addEdge(domain.FamilyEdge{ParentNumber: domain.PatentNumber(source), ChildNumber: domain.PatentNumber(child), RelationType: domain.FamilyRelationContinuation})
		}
		for _, parent := range collectLegacy("[itemprop='backwardReferencesFamily']") {
			addEdge(domain.FamilyEdge{ParentNumber: domain.PatentNumber(parent), ChildNumber: domain.PatentNumber(source), RelationType: domain.FamilyRelationContinuation})
		}
	}

	logger.Debug("google.extract_family done", "source", source, "count", len(edges))
	return edges
}

func extractCitationEdges(doc *goquery.Document, source string, logger *slog.Logger) []domain.CitationEdge {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	logger.Debug("google.extract_citations", "source", source)
	seen := map[string]bool{}
	var edges []domain.CitationEdge
	add := func(target, relation string) {
		target = normalizePatentID(target)
		if target == "" || strings.EqualFold(target, source) {
			return
		}
		key := source + "\x00" + target + "\x00" + relation
		if seen[key] {
			return
		}
		seen[key] = true
		edges = append(edges, domain.CitationEdge{SourcePatent: domain.PatentNumber(source), TargetPatent: domain.PatentNumber(target), RelationType: relation})
	}

	extractRows := func(selector, relation string) {
		doc.Find(selector).Each(func(_ int, section *goquery.Selection) {
			// Find anything that looks like a patent link or result
			section.Find("tr, .patent-result, .result, .citation, [itemprop='publicationNumber'], [itemprop='patentCitation'], [itemprop='forwardReferencesFamily'], td.publication, a[href*='/patent/']").Each(func(_ int, s *goquery.Selection) {
				if href, ok := s.Attr("href"); ok {
					if path := strings.TrimPrefix(href, "/patent/"); path != href {
						add(patentNumberFromURL(href), relation)
					}
				}
				// Also check text content if it looks like a patent ID
				text := strings.TrimSpace(s.Text())
				// Patent IDs are typically 5-15 chars, no spaces
				if len(text) > 4 && len(text) < 16 && !strings.Contains(text, " ") {
					add(text, relation)
				}
			})
		})
	}
	// Try multiple selector variations for the sections themselves
	extractRows("[itemprop='backwardReferences'], #patentCitations, #backwardReferences", domain.RelationCites)
	extractRows("[itemprop='forwardReferences'], #citedBy, #forwardReferences", domain.RelationCitedBy)

	logger.Debug("google.extract_citations done", "source", source, "count", len(edges))
	return edges
}

func normalizePatentID(value string) string {
	// Strip spaces and commas which are common in scraped text
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, ",", "")
	value = strings.ToUpper(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "/PATENT/") {
		return patentNumberFromURL(value)
	}
	match := patentIDPattern.FindString(value)
	return strings.TrimSpace(match)
}

func clean(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func adjustedExpirationDate(doc *goquery.Document) string {
	if doc == nil {
		return ""
	}
	var expiration string
	doc.Find("[itemprop='events'], tr, li, dd").EachWithBreak(func(_ int, row *goquery.Selection) bool {
		date := adjustedExpirationDateFromText(row.Text())
		if date == "" {
			return true
		}
		expiration = date
		return false
	})
	if expiration != "" {
		return expiration
	}
	return adjustedExpirationDateFromText(doc.Text())
}

func adjustedExpirationDateFromText(text string) string {
	text = clean(text)
	lower := strings.ToLower(text)
	labelIndex := strings.Index(lower, "adjusted expiration")
	if labelIndex < 0 {
		return ""
	}
	prefix := text[:labelIndex]
	dates := datePattern.FindAllString(prefix, -1)
	if len(dates) == 0 {
		return ""
	}
	date := dates[len(dates)-1]
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return ""
	}
	return date
}

func estimatedExpirationDate(publicationDate, grantDate string) string {
	base := firstNonEmpty(publicationDate, grantDate)
	if base == "" {
		return ""
	}
	parsed, err := time.Parse("2006-01-02", firstDate(base))
	if err != nil {
		return ""
	}
	return parsed.AddDate(20, 0, 0).Format("2006-01-02")
}

func firstDate(value string) string {
	if len(value) >= len("2006-01-02") {
		return value[:len("2006-01-02")]
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
