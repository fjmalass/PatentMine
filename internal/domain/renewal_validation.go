package domain

import (
	"fmt"
	"strings"
	"time"
)

// RenewalValidationStatus is the country-phase status used for EP post-grant
// national validations. A designated state is only potential until a legal-status
// or national-register event confirms validation.
type RenewalValidationStatus string

const (
	RenewalValidationPotential RenewalValidationStatus = "potential"
	RenewalValidationValidated RenewalValidationStatus = "validated"
	RenewalValidationLapsed    RenewalValidationStatus = "lapsed"
	RenewalValidationUnknown   RenewalValidationStatus = "unknown"
)

func (s RenewalValidationStatus) Valid() bool {
	switch s {
	case RenewalValidationPotential, RenewalValidationValidated, RenewalValidationLapsed, RenewalValidationUnknown:
		return true
	default:
		return false
	}
}

func ParseRenewalValidationStatus(s string) (RenewalValidationStatus, error) {
	v := RenewalValidationStatus(strings.ToLower(strings.TrimSpace(s)))
	if !v.Valid() {
		return "", fmt.Errorf("domain: unknown validation status %q", s)
	}
	return v, nil
}

// RenewalCertainty describes how strongly a renewal/validation fact is known.
type RenewalCertainty string

const (
	RenewalCertaintyExact     RenewalCertainty = "exact"
	RenewalCertaintyDerived   RenewalCertainty = "derived"
	RenewalCertaintyEstimated RenewalCertainty = "estimated"
	RenewalCertaintyInferred  RenewalCertainty = "inferred"
)

func (c RenewalCertainty) Valid() bool {
	switch c {
	case RenewalCertaintyExact, RenewalCertaintyDerived, RenewalCertaintyEstimated, RenewalCertaintyInferred:
		return true
	default:
		return false
	}
}

// PatentValidation is one country-phase row for a patent, primarily used for EP
// post-grant national validations and country-specific annuity predictions.
type PatentValidation struct {
	PatentNumber  PatentNumber            `json:"patent_number"`
	Country       string                  `json:"country"`
	Status        RenewalValidationStatus `json:"status"`
	Source        string                  `json:"source,omitempty"`
	Certainty     RenewalCertainty        `json:"certainty,omitempty"`
	EventCode     string                  `json:"event_code,omitempty"`
	EventDate     time.Time               `json:"event_date,omitempty"`
	LastCheckedAt time.Time               `json:"last_checked_at,omitempty"`
	Notes         string                  `json:"notes,omitempty"`
}

// RenewalLegalEvent preserves a raw legal-status event from an authority such as
// EPO OPS/INPADOC. PatentValidation rows are derived from these auditable facts.
type RenewalLegalEvent struct {
	PatentNumber PatentNumber `json:"patent_number"`
	Authority    string       `json:"authority"`
	Country      string       `json:"country,omitempty"`
	Code         string       `json:"code,omitempty"`
	Description  string       `json:"description,omitempty"`
	EventDate    time.Time    `json:"event_date,omitempty"`
	RawXML       string       `json:"raw_xml,omitempty"`
	FetchedAt    time.Time    `json:"fetched_at"`
}
