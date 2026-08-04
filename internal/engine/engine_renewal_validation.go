package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"patentmine/internal/domain"
)

// PatentValidations returns EP/national country-phase validation rows for one patent.
func (e *Engine) PatentValidations(ctx context.Context, patentNumber string) ([]domain.PatentValidation, error) {
	n, err := domain.ParsePatentNumber(patentNumber)
	if err != nil {
		return nil, fmt.Errorf("engine: list patent validations: %w", err)
	}
	if rec, err := e.repo.RecordOf(ctx, n); err == nil {
		n = rec
	}
	return e.repo.PatentValidations(ctx, n)
}

// SetPatentValidation records a manually reviewed country-phase status. This is
// the safe fallback when EPO only exposes designated states or a national register
// must be checked by a human/agent.
func (e *Engine) SetPatentValidation(ctx context.Context, patentNumber, country string, status domain.RenewalValidationStatus) error {
	n, err := domain.ParsePatentNumber(patentNumber)
	if err != nil {
		return fmt.Errorf("engine: set patent validation: %w", err)
	}
	if !status.Valid() {
		return fmt.Errorf("engine: invalid validation status %q", status)
	}
	if rec, err := e.repo.RecordOf(ctx, n); err == nil {
		n = rec
	}
	now := time.Now().UTC()
	v := domain.PatentValidation{
		PatentNumber:  n,
		Country:       strings.ToUpper(strings.TrimSpace(country)),
		Status:        status,
		Source:        "manual",
		Certainty:     domain.RenewalCertaintyExact,
		LastCheckedAt: now,
		Notes:         "manual country-phase review",
	}
	if v.Country == "" {
		return fmt.Errorf("engine: validation country required")
	}
	if err := e.repo.SavePatentValidations(ctx, []domain.PatentValidation{v}); err != nil {
		return err
	}
	e.announceChange()
	return nil
}

// FetchEPOPatentValidations pulls EPO OPS legal-status events for an EP patent,
// stores the raw events, and upserts derived country-phase validations.
func (e *Engine) FetchEPOPatentValidations(ctx context.Context, patentNumber string) ([]domain.PatentValidation, []domain.RenewalLegalEvent, error) {
	if e.epoValidations == nil {
		return nil, nil, fmt.Errorf("engine: EPO validation fetcher is not configured")
	}
	n, err := domain.ParsePatentNumber(patentNumber)
	if err != nil {
		return nil, nil, fmt.Errorf("engine: fetch EPO validations: %w", err)
	}
	if rec, err := e.repo.RecordOf(ctx, n); err == nil {
		n = rec
	}
	validations, events, err := e.epoValidations(ctx, n)
	if err != nil {
		return nil, nil, err
	}
	if err := e.repo.SaveRenewalLegalEvents(ctx, events); err != nil {
		return nil, nil, err
	}
	if err := e.repo.SavePatentValidations(ctx, validations); err != nil {
		return nil, nil, err
	}
	e.announceChange()
	return validations, events, nil
}
