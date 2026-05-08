package importer

import (
	"encoding/json"
	"os"

	"patentmine/internal/domain"
)

type fixtureBundle struct {
	Patent struct {
		Number              string   `json:"number"`
		Title               string   `json:"title"`
		Abstract            string   `json:"abstract"`
		Assignee            string   `json:"assignee"`
		Inventors           []string `json:"inventors"`
		PublicationDate     string   `json:"publication_date"`
		GrantDate           string   `json:"grant_date"`
		ExpirationDate      string   `json:"expiration_date"`
		ExpirationEstimated bool     `json:"expiration_estimated"`
		SourceURL           string   `json:"source_url"`
	} `json:"patent"`
	Sections []struct {
		SectionType string `json:"section_type"`
		Ordinal     int    `json:"ordinal"`
		Text        string `json:"text"`
	} `json:"sections"`
	Classifications []struct {
		System      string `json:"system"`
		Code        string `json:"code"`
		Description string `json:"description"`
	} `json:"classifications"`
	CPCs []struct { // Keep for backward compatibility with old JSON fixtures
		Code        string `json:"code"`
		Description string `json:"description"`
	} `json:"cpcs"`
	Citations []struct {
		TargetPatent string `json:"target_patent"`
		RelationType string `json:"relation_type"`
	} `json:"citations"`
	References []struct {
		PatentNumber  string `json:"patent_number"`
		CitationLabel string `json:"citation_label"`
	} `json:"references"`
}

func LoadFixture(path string) (domain.PatentBundle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.PatentBundle{}, err
	}
	var fixture fixtureBundle
	if err := json.Unmarshal(data, &fixture); err != nil {
		return domain.PatentBundle{}, err
	}
	p := domain.Patent{
		Number:              fixture.Patent.Number,
		Title:               fixture.Patent.Title,
		Abstract:            fixture.Patent.Abstract,
		Assignee:            fixture.Patent.Assignee,
		Inventors:           fixture.Patent.Inventors,
		PublicationDate:     fixture.Patent.PublicationDate,
		GrantDate:           fixture.Patent.GrantDate,
		ExpirationDate:      fixture.Patent.ExpirationDate,
		ExpirationEstimated: fixture.Patent.ExpirationEstimated,
		SourceURL:           fixture.Patent.SourceURL,
	}
	bundle := domain.PatentBundle{Patent: p}
	for _, section := range fixture.Sections {
		bundle.Sections = append(bundle.Sections, domain.PatentTextSection{PatentNumber: p.Number, SectionType: section.SectionType, Ordinal: section.Ordinal, Text: section.Text})
	}
	for _, cls := range fixture.Classifications {
		c := domain.ParseClassification(cls.Code)
		c.PatentNumber = p.Number
		if cls.System != "" {
			c.System = cls.System
		}
		if cls.Description != "" {
			c.Description = cls.Description
		}
		bundle.Classifications = append(bundle.Classifications, c)
	}
	for _, cpc := range fixture.CPCs {
		c := domain.ParseClassification(cpc.Code)
		c.PatentNumber = p.Number
		c.Description = cpc.Description
		bundle.Classifications = append(bundle.Classifications, c)
	}
	for _, citation := range fixture.Citations {
		bundle.Citations = append(bundle.Citations, domain.CitationEdge{SourcePatent: p.Number, TargetPatent: citation.TargetPatent, RelationType: citation.RelationType})
	}
	for _, ref := range fixture.References {
		bundle.References = append(bundle.References, domain.ReferenceEntry{PatentNumber: ref.PatentNumber, CitationLabel: ref.CitationLabel})
	}
	return bundle, nil
}
