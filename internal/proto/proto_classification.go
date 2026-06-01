// Classification-taxonomy and lookup payloads.
package proto

import "patentmine/internal/domain"

// ClassificationGetParams selects a single classification definition.
type ClassificationGetParams struct {
	System string `json:"system"`
	Code   string `json:"code"`
}

// ClassificationListResult carries all classification definitions.
type ClassificationListResult struct {
	Classifications []domain.Classification `json:"classifications"`
}

// ClassificationParams contains a classification definition to save.
type ClassificationParams struct {
	Classification domain.Classification `json:"classification"`
}

// ClassificationDeleteParams identifies the classification to delete.
type ClassificationDeleteParams struct {
	System string `json:"system"`
	Code   string `json:"code"`
}

// ClassificationLookupParams contains the CPC code to lookup.
type ClassificationLookupParams struct {
	Code string `json:"code"`
}

// ClassificationListByCodesParams selects cached classification definitions by
// a set of raw codes. Codes are parsed to (system, code) and matched
// case-insensitively against the cache.
type ClassificationListByCodesParams struct {
	Codes []string `json:"codes"`
}

// PatentClassificationListParams lists classifications for a patent.
type PatentClassificationListParams struct {
	Project domain.ProjectID    `json:"project"`
	Patent  domain.PatentNumber `json:"patent"`
}
