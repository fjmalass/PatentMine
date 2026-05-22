package crawl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"patentmine/internal/domain"
)

// LookupCPCDescription queries the public European Patent Office (EPO) Linked Open Data SPARQL
// endpoint to find the description for a given CPC code.
func LookupCPCDescription(ctx context.Context, code string) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", errors.New("crawl: empty classification code")
	}

	c := domain.ParseClassification(code)
	if c.System != "CPC" {
		return "", fmt.Errorf("crawl: lookup only supported for CPC codes, got %s", c.System)
	}

	// Format normalized label (e.g. "G06F 17/30" or "A01B")
	label := fmt.Sprintf("%s%s%s", c.Section, c.Class, c.Subclass)
	if c.MainGroup != "" {
		label += " " + c.MainGroup
		if c.Subgroup != "" {
			label += "/" + c.Subgroup
		}
	}

	// Try lookup with normalized label first
	desc, err := queryEPO(ctx, label)
	if err == nil && desc != "" {
		return desc, nil
	}

	// Fallback to original code if normalized failed or was empty
	if label != code {
		desc, err = queryEPO(ctx, code)
		if err == nil && desc != "" {
			return desc, nil
		}
	}

	return "", fmt.Errorf("crawl: classification code %s not found on EPO", code)
}

func queryEPO(ctx context.Context, label string) (string, error) {
	// Standard SPARQL query to get the title (description) of a CPC classification
	sparqlQuery := fmt.Sprintf(`PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX cpc: <http://data.epo.org/linked-data/def/cpc/>
PREFIX dcterms: <http://purl.org/dc/terms/>

SELECT DISTINCT ?title WHERE {
  ?CPC rdf:type/rdfs:subClassOf cpc:Classification ;
       rdfs:label "%s" ;
       dcterms:title ?title .
}
LIMIT 1`, label)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://data.epo.org/linked-data/query", bytes.NewBufferString(sparqlQuery))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/sparql-query")
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("User-Agent", "PatentMine/1.0 (https://github.com/fjmalass/PatentMine)")

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("decode json response: %w", err)
	}

	if len(res.Results.Bindings) == 0 {
		return "", nil
	}

	binding := res.Results.Bindings[0]
	if t, ok := binding["title"]; ok {
		return t.Value, nil
	}

	return "", nil
}
