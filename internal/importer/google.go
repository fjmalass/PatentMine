package importer

import (
	"fmt"
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

func ImportGooglePatents(rawURL string) (domain.PatentBundle, error) {
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

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return domain.PatentBundle{}, err
	}
	req.Header.Set("User-Agent", "PatentMine sample importer")
	resp, err := client.Do(req)
	if err != nil {
		return domain.PatentBundle{}, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return domain.PatentBundle{}, fmt.Errorf("fetch failed: HTTP %d", resp.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return domain.PatentBundle{}, fmt.Errorf("parse failed: %w", err)
	}

	patent := domain.Patent{
		Number:          number,
		Title:           clean(firstText(doc.Selection, "span[itemprop='title']", "meta[name='DC.title']", "title")),
		Abstract:        clean(firstText(doc.Selection, "div.abstract", "section[itemprop='abstract']", "abstract")),
		Assignee:        clean(firstText(doc.Selection, "dd[itemprop='assigneeOriginal']", "span[itemprop='assigneeOriginal']", "dd[itemprop='assigneeCurrent']")),
		Inventors:       texts(doc.Selection, "dd[itemprop='inventor']", "span[itemprop='inventor']"),
		PublicationDate: attr(doc.Selection, "time[itemprop='publicationDate']", "datetime"),
		GrantDate:       attr(doc.Selection, "time[itemprop='grantDate']", "datetime"),
		SourceURL:       rawURL,
	}
	if expirationDate := estimatedExpirationDate(patent.PublicationDate, patent.GrantDate); expirationDate != "" {
		patent.ExpirationDate = expirationDate
		patent.ExpirationEstimated = true
	}
	if patent.Title == "" {
		patent.Title = number
	}
	bundle := domain.PatentBundle{Patent: patent}
	if patent.Abstract != "" {
		bundle.Sections = append(bundle.Sections, domain.PatentTextSection{PatentNumber: number, SectionType: "abstract", Ordinal: 1, Text: patent.Abstract})
	}
	addSection(doc.Selection, &bundle, number, "claims", "section[itemprop='claims'] .claim, .claims .claim")
	addSection(doc.Selection, &bundle, number, "description", "section[itemprop='description'] p, .description p")
	// Extract all classifications (CPC and USPC) from the tree and specific tags
	doc.Find("[itemprop='classification'], classification-cpc, .classification-cpc, classification-item, .classification-item").Each(func(_ int, row *goquery.Selection) {
		code := clean(firstText(row, "[itemprop='code']", "[itemprop='Code']", ".code"))
		description := clean(firstText(row, "[itemprop='description']", "[itemprop='Description']", ".description"))
		
		if code == "" {
			// Fallback: search for "Code - Description" pattern in text
			text := clean(row.Text())
			if strings.Contains(text, " — ") {
				parts := strings.SplitN(text, " — ", 2)
				code = clean(parts[0])
				description = clean(parts[1])
			}
		}

		if code != "" {
			cls := domain.ParseClassification(code)
			cls.PatentNumber = number
			cls.Description = description

			found := false
			for i, existing := range bundle.Classifications {
				if existing.Code == cls.Code && existing.System == cls.System {
					if existing.Description == "" && cls.Description != "" {
						bundle.Classifications[i].Description = cls.Description
					}
					found = true
					break
				}
			}
			if !found {
				bundle.Classifications = append(bundle.Classifications, cls)
			}
		}
	})
	bundle.Citations = extractCitationEdges(doc, number)
	bundle.References = append(bundle.References, domain.ReferenceEntry{PatentNumber: number, CitationLabel: fmt.Sprintf("%s, %s", number, patent.Title)})
	return bundle, nil
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

func addSection(s *goquery.Selection, bundle *domain.PatentBundle, number, sectionType, selector string) {
	ordinal := 1
	s.Find(selector).Each(func(_ int, sel *goquery.Selection) {
		text := clean(sel.Text())
		if text == "" {
			return
		}
		bundle.Sections = append(bundle.Sections, domain.PatentTextSection{PatentNumber: number, SectionType: sectionType, Ordinal: ordinal, Text: text})
		ordinal++
	})
}

func extractCitationEdges(doc *goquery.Document, source string) []domain.CitationEdge {
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
		edges = append(edges, domain.CitationEdge{SourcePatent: source, TargetPatent: target, RelationType: relation})
	}

	extractRows := func(selector, relation string) {
		doc.Find(selector).Each(func(_ int, row *goquery.Selection) {
			row.Find("[itemprop='publicationNumber'], td.publication, a[href*='/patent/']").Each(func(_ int, s *goquery.Selection) {
				if href, ok := s.Attr("href"); ok {
					add(patentNumberFromURL(href), relation)
				}
				add(s.Text(), relation)
			})
		})
	}
	extractRows("[itemprop='backwardReferences']", "cites")
	extractRows("[itemprop='backwardReferencesFamily']", "cites")
	extractRows("[itemprop='forwardReferences']", "cited_by")
	extractRows("[itemprop='forwardReferencesFamily']", "cited_by")

	doc.Find("a[href*='/patent/']").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		target := patentNumberFromURL(href)
		if target == "" {
			return
		}
		
		// Get all text from direct parent containers to avoid capturing headers from far away
		parent := s.Closest("section, table, div")
		if parent.Length() == 0 {
			return
		}
		
		// Only check the header of this specific container
		headerText := strings.ToLower(clean(parent.Find("h1, h2, h3, h4, th").First().Text()))
		if headerText == "" {
			// Fallback to searching the parent's immediate ID or class if no header
			headerText = strings.ToLower(parent.AttrOr("id", "") + " " + parent.AttrOr("class", ""))
		}

		switch {
		case strings.Contains(headerText, "cited by") || strings.Contains(headerText, "forward references"):
			add(target, "cited_by")
		case strings.Contains(headerText, "patent citations") || strings.Contains(headerText, "backward references") || strings.Contains(headerText, "references cited"):
			add(target, "cites")
		}
	})
	return edges
}

func normalizePatentID(value string) string {
	value = strings.ToUpper(clean(value))
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
