package engine

import (
	"context"
	"errors"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

func TestEngineClassification(t *testing.T) {
	ctx := context.Background()

	// Mock CPCLookup that returns a mock description
	mockLookupCalled := false
	mockCPCLookup := func(ctx context.Context, code string) (string, error) {
		mockLookupCalled = true
		if code == "G06F16/00" {
			return "Information retrieval", nil
		}
		return "", errors.New("not found")
	}

	eng, repo := newTestEngine(t, nil)
	eng.cpcLookup = mockCPCLookup

	// 1. Initially, no classification definitions are in the database
	list, err := eng.ListClassificationDefinitions(ctx)
	if err != nil {
		t.Fatalf("ListClassificationDefinitions: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 cached definitions, got %d", len(list))
	}

	// 2. Perform LookupClassification on G06F16/00
	c, err := eng.LookupClassification(ctx, "G06F16/00")
	if err != nil {
		t.Fatalf("LookupClassification G06F16/00 failed: %v", err)
	}
	if !mockLookupCalled {
		t.Errorf("expected CPCLookup function to be called")
	}
	if c.Description != "Information retrieval" {
		t.Errorf("expected description 'Information retrieval', got %q", c.Description)
	}

	// 3. Lookup again, mockCPCLookup should NOT be called since it is cached in the DB
	mockLookupCalled = false
	c2, err := eng.LookupClassification(ctx, "G06F16/00")
	if err != nil {
		t.Fatalf("LookupClassification cached G06F16/00 failed: %v", err)
	}
	if mockLookupCalled {
		t.Errorf("expected cache hit, but CPCLookup was called again")
	}
	if c2.Description != "Information retrieval" {
		t.Errorf("expected cached description 'Information retrieval', got %q", c2.Description)
	}

	// 4. Verify in-DB persistence
	cached, err := eng.ClassificationDefinition(ctx, "CPC", "G06F16/00")
	if err != nil {
		t.Fatalf("ClassificationDefinition failed: %v", err)
	}
	if cached.Description != "Information retrieval" {
		t.Errorf("unexpected cached description: %q", cached.Description)
	}

	// 5. Test SaveClassificationDefinition
	c.Description = "Overridden description"
	err = eng.SaveClassificationDefinition(ctx, c)
	if err != nil {
		t.Fatalf("SaveClassificationDefinition: %v", err)
	}

	cached2, _ := eng.ClassificationDefinition(ctx, "CPC", "G06F16/00")
	if cached2.Description != "Overridden description" {
		t.Errorf("expected Overridden description, got %q", cached2.Description)
	}

	// 6. Test DeleteClassificationDefinition
	err = eng.DeleteClassificationDefinition(ctx, "CPC", "G06F16/00")
	if err != nil {
		t.Fatalf("DeleteClassificationDefinition: %v", err)
	}

	_, err = eng.ClassificationDefinition(ctx, "CPC", "G06F16/00")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected store.ErrNotFound after delete, got %v", err)
	}

	// 7. Test ListPatentClassifications
	patNum, err := domain.ParsePatentNumber("US123456")
	if err != nil {
		t.Fatalf("ParsePatentNumber: %v", err)
	}

	pat := domain.Patent{
		Number:          patNum,
		DisplayNumber:   patNum,
		Classifications: []string{"G06F16/00", "707/123"},
		FetchState:      domain.FetchCached,
	}
	err = repo.SavePatent(ctx, pat)
	if err != nil {
		t.Fatalf("SavePatent failed: %v", err)
	}

	// Seed one cached definition, leave other parsed but uncached
	err = eng.SaveClassificationDefinition(ctx, domain.Classification{
		System:      "CPC",
		Code:        "G06F16/00",
		Description: "Information retrieval",
	})
	if err != nil {
		t.Fatalf("SaveClassificationDefinition failed: %v", err)
	}

	patClassifications, err := eng.ListPatentClassifications(ctx, "", patNum)
	if err != nil {
		t.Fatalf("ListPatentClassifications failed: %v", err)
	}

	if len(patClassifications) != 2 {
		t.Fatalf("expected 2 classifications, got %d", len(patClassifications))
	}

	// First classification is CPC/G06F16/00 with cached description
	if patClassifications[0].System != "CPC" || patClassifications[0].Description != "Information retrieval" {
		t.Errorf("unexpected first classification: %+v", patClassifications[0])
	}

	// Second classification is USPC-707/123 (parsed but not cached, description empty)
	if patClassifications[1].System != "USPC" || patClassifications[1].Description != "" {
		t.Errorf("unexpected second classification: %+v", patClassifications[1])
	}
}
