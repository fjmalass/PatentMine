package engine

import (
	"context"
	"fmt"
	"log/slog"

	"patentmine/internal/domain"
)

// SaveClassificationDefinition inserts or updates a classification definition.
func (e *Engine) SaveClassificationDefinition(ctx context.Context, c domain.Classification) error {
	if err := e.repo.SaveClassificationDefinition(ctx, c); err != nil {
		return err
	}
	e.announceChange()
	return nil
}

// ClassificationDefinition returns one classification definition, or store.ErrNotFound.
func (e *Engine) ClassificationDefinition(ctx context.Context, system, code string) (domain.Classification, error) {
	return e.repo.ClassificationDefinition(ctx, system, code)
}

// ListClassificationDefinitions returns all classification definitions.
func (e *Engine) ListClassificationDefinitions(ctx context.Context) ([]domain.Classification, error) {
	return e.repo.ListClassificationDefinitions(ctx)
}

// ListClassificationDefinitionsByCodes returns cached classification definitions
// for the given raw codes (codes are parsed and matched case-insensitively).
func (e *Engine) ListClassificationDefinitionsByCodes(ctx context.Context, codes []string) ([]domain.Classification, error) {
	return e.repo.ListClassificationDefinitionsByCodes(ctx, codes)
}

// DeleteClassificationDefinition removes a classification definition.
func (e *Engine) DeleteClassificationDefinition(ctx context.Context, system, code string) error {
	if err := e.repo.DeleteClassificationDefinition(ctx, system, code); err != nil {
		return err
	}
	e.announceChange()
	return nil
}

// ClassificationDescriptionUnavailable is the sentinel description persisted
// after a successful EPO query that returned no record. It lets the cache
// distinguish "tried, EPO has no entry" from "never tried" without a separate
// schema column.
const ClassificationDescriptionUnavailable = "(no description available)"

// LookupClassification parses the code, checks the cache, and (for CPC codes
// only) calls the EPO lookup when a description is not yet known. EPO misses
// are recorded with the ClassificationDescriptionUnavailable sentinel so a
// retry doesn't keep hammering EPO for codes it doesn't index. Non-CPC codes
// (USPC and "Other") return parsed-only.
func (e *Engine) LookupClassification(ctx context.Context, code string) (domain.Classification, error) {
	parsed := domain.ParseClassification(code)
	if parsed.System == "" {
		return domain.Classification{}, fmt.Errorf("invalid classification code %q", code)
	}

	// 1. Cache hit with any non-empty description (real text or sentinel) wins.
	cached, err := e.repo.ClassificationDefinition(ctx, parsed.System, parsed.Code)
	if err == nil && cached.Description != "" {
		return cached, nil
	}

	// 2. Only CPC codes are looked up on the web. "Other" and USPC return
	//    parsed-only — the UI labels those distinctly.
	if parsed.System != "CPC" || e.cpcLookup == nil {
		return parsed, nil
	}

	desc, lookupErr := e.cpcLookup(ctx, parsed.Code)
	if lookupErr != nil {
		// Network or remote error: leave the cache untouched so the next
		// attempt can succeed.
		e.log(ctx, slog.LevelWarn, "classification web lookup failed",
			slog.String("code", parsed.Code), slog.String("system", parsed.System),
			slog.String("error", lookupErr.Error()))
		return parsed, nil
	}
	if desc != "" {
		parsed.Description = desc
	} else {
		// Successful query, EPO returned nothing — persist the sentinel.
		parsed.Description = ClassificationDescriptionUnavailable
	}
	if saveErr := e.repo.SaveClassificationDefinition(ctx, parsed); saveErr != nil {
		e.log(ctx, slog.LevelWarn, "classification cache save failed",
			slog.String("code", parsed.Code), slog.String("system", parsed.System),
			slog.String("error", saveErr.Error()))
	}
	e.announceChange()
	return parsed, nil
}

// ListPatentClassifications returns the detailed classification definitions for all classifications on a patent.
func (e *Engine) ListPatentClassifications(ctx context.Context, projectID domain.ProjectID, patentNumber domain.PatentNumber) ([]domain.Classification, error) {
	p, err := e.repo.Patent(ctx, patentNumber)
	if err != nil {
		return nil, err
	}

	out := make([]domain.Classification, 0, len(p.Classifications))
	for _, rawCode := range p.Classifications {
		parsed := domain.ParseClassification(rawCode)
		if parsed.System == "" {
			continue
		}

		cached, err := e.repo.ClassificationDefinition(ctx, parsed.System, parsed.Code)
		if err == nil {
			out = append(out, cached)
		} else {
			out = append(out, parsed)
		}
	}
	return out, nil
}
