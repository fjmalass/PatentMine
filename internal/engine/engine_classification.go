package engine

import (
	"context"
	"fmt"

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

// DeleteClassificationDefinition removes a classification definition.
func (e *Engine) DeleteClassificationDefinition(ctx context.Context, system, code string) error {
	if err := e.repo.DeleteClassificationDefinition(ctx, system, code); err != nil {
		return err
	}
	e.announceChange()
	return nil
}

// LookupClassification parses the code, checks if it is cached in the database, and if not
// (and it's a CPC code), performs a dynamic web lookup against the CPC lookup function if configured.
func (e *Engine) LookupClassification(ctx context.Context, code string) (domain.Classification, error) {
	parsed := domain.ParseClassification(code)
	if parsed.System == "" {
		return domain.Classification{}, fmt.Errorf("invalid classification code %q", code)
	}

	// 1. Check if already in cache with a description
	cached, err := e.repo.ClassificationDefinition(ctx, parsed.System, parsed.Code)
	if err == nil && cached.Description != "" {
		return cached, nil
	}

	// 2. If it is CPC and not cached or lacks description, perform a dynamic lookup if configured
	if parsed.System == "CPC" && e.cpcLookup != nil {
		desc, err := e.cpcLookup(ctx, parsed.Code)
		if err == nil && desc != "" {
			parsed.Description = desc
			// Cache the definition in the database
			_ = e.repo.SaveClassificationDefinition(ctx, parsed)
			e.announceChange()
			return parsed, nil
		}
	}

	// Fallback to returning the parsed classification code (possibly with empty description)
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
