package epo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"patentmine/internal/domain"
)

const defaultTimeout = 20 * time.Second

// FetchValidations pulls EPO OPS legal-status data and derives EP post-grant
// country-phase validation rows. It intentionally derives conservatively: events
// without country codes remain raw legal events but do not become payable
// national-renewal obligations.
func FetchValidations(ctx context.Context, baseURL, consumerKey, consumerSecret string, number domain.PatentNumber) ([]domain.PatentValidation, []domain.RenewalLegalEvent, error) {
	consumerKey = strings.TrimSpace(consumerKey)
	consumerSecret = strings.TrimSpace(consumerSecret)
	if consumerKey == "" || consumerSecret == "" {
		return nil, nil, fmt.Errorf("epo: OPS credentials not configured")
	}
	if baseURL == "" {
		baseURL = "https://ops.epo.org/3.2"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	client := &http.Client{Timeout: defaultTimeout}
	token, err := accessToken(ctx, client, baseURL, consumerKey, consumerSecret)
	if err != nil {
		return nil, nil, err
	}
	body, err := fetchLegalXML(ctx, client, baseURL, token, number)
	if err != nil {
		return nil, nil, err
	}
	events, err := parseLegalEvents(number, body)
	if err != nil {
		return nil, nil, err
	}
	return deriveValidations(events), events, nil
}

func accessToken(ctx context.Context, client *http.Client, baseURL, consumerKey, consumerSecret string) (string, error) {
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/auth/accesstoken", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(consumerKey+":"+consumerSecret)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("epo: auth request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("epo: auth HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("epo: parse auth response: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", fmt.Errorf("epo: auth response missing access_token")
	}
	return parsed.AccessToken, nil
}

func fetchLegalXML(ctx context.Context, client *http.Client, baseURL, token string, number domain.PatentNumber) ([]byte, error) {
	for _, id := range epoDocIDs(number) {
		endpoint := baseURL + "/rest-services/legal/publication/epodoc/" + url.PathEscape(id)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/xml")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("epo: legal request: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode == http.StatusNotFound {
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("epo: legal HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return body, nil
	}
	return nil, fmt.Errorf("epo: no legal-status record found for %s", number)
}

func epoDocIDs(n domain.PatentNumber) []string {
	base := strings.ToUpper(n.Country + n.Serial)
	full := strings.ToUpper(n.Normalized())
	if n.Country == "" {
		base = "EP" + n.Serial
	}
	if full == "" || full == base {
		return []string{base}
	}
	return []string{full, base}
}

type legalEventXML struct {
	country string
	code    string
	desc    string
	date    time.Time
}

func parseLegalEvents(number domain.PatentNumber, body []byte) ([]domain.RenewalLegalEvent, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	fetchedAt := time.Now().UTC()
	var current *legalEventXML
	var currentField string
	var out []domain.RenewalLegalEvent
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("epo: parse legal XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if name == "legal-event" {
				current = &legalEventXML{}
				for _, a := range t.Attr {
					switch strings.ToLower(a.Name.Local) {
					case "country":
						current.country = strings.ToUpper(strings.TrimSpace(a.Value))
					case "date", "event-date":
						current.date = parseEPODate(a.Value)
					}
				}
				continue
			}
			if current != nil {
				switch name {
				case "code", "event-code":
					currentField = "code"
				case "desc", "description", "event-description":
					currentField = "desc"
				case "date", "event-date":
					currentField = "date"
				}
			}
		case xml.CharData:
			if current == nil || currentField == "" {
				continue
			}
			text := strings.TrimSpace(string(t))
			if text == "" {
				continue
			}
			switch currentField {
			case "code":
				current.code += text
			case "desc":
				if current.desc != "" {
					current.desc += " "
				}
				current.desc += text
			case "date":
				if current.date.IsZero() {
					current.date = parseEPODate(text)
				}
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			if name == "legal-event" && current != nil {
				out = append(out, domain.RenewalLegalEvent{
					PatentNumber: number,
					Authority:    "epo_ops",
					Country:      current.country,
					Code:         strings.TrimSpace(current.code),
					Description:  strings.TrimSpace(current.desc),
					EventDate:    current.date,
					FetchedAt:    fetchedAt,
				})
				current = nil
				currentField = ""
				continue
			}
			if current != nil {
				currentField = ""
			}
		}
	}
	if len(out) == 0 {
		out = append(out, domain.RenewalLegalEvent{PatentNumber: number, Authority: "epo_ops", Description: "legal-status XML fetched; no legal-event entries parsed", RawXML: string(body), FetchedAt: fetchedAt})
	}
	return out, nil
}

func parseEPODate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{domain.DateLayout, domain.CompactDateLayout, domain.USDateLayout} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func deriveValidations(events []domain.RenewalLegalEvent) []domain.PatentValidation {
	byCountry := make(map[string]domain.PatentValidation)
	for _, event := range events {
		country := strings.ToUpper(strings.TrimSpace(event.Country))
		if country == "" || country == "EP" || country == "WO" {
			continue
		}
		status := inferValidationStatus(event.Code, event.Description)
		if status == domain.RenewalValidationUnknown {
			continue
		}
		prev, ok := byCountry[country]
		if ok && !event.EventDate.IsZero() && !prev.EventDate.IsZero() && event.EventDate.Before(prev.EventDate) {
			continue
		}
		byCountry[country] = domain.PatentValidation{
			PatentNumber:  event.PatentNumber,
			Country:       country,
			Status:        status,
			Source:        event.Authority,
			Certainty:     domain.RenewalCertaintyDerived,
			EventCode:     event.Code,
			EventDate:     event.EventDate,
			LastCheckedAt: event.FetchedAt,
			Notes:         event.Description,
		}
	}
	out := make([]domain.PatentValidation, 0, len(byCountry))
	for _, v := range byCountry {
		out = append(out, v)
	}
	return out
}

func inferValidationStatus(code, desc string) domain.RenewalValidationStatus {
	text := strings.ToLower(code + " " + desc)
	switch {
	case strings.Contains(text, "lapse"), strings.Contains(text, "lapsed"), strings.Contains(text, "not in force"), strings.Contains(text, "ceased"), strings.Contains(text, "expired"), strings.Contains(text, "revoked"):
		return domain.RenewalValidationLapsed
	case strings.Contains(text, "validation"), strings.Contains(text, "validated"), strings.Contains(text, "translation filed"), strings.Contains(text, "national phase"), strings.Contains(text, "renewal fee"):
		return domain.RenewalValidationValidated
	case strings.Contains(text, "designated"), strings.Contains(text, "designation"):
		return domain.RenewalValidationPotential
	default:
		return domain.RenewalValidationUnknown
	}
}
