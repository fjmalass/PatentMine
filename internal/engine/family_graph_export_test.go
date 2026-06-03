package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/store"
)

func TestExportFamilyGraph(t *testing.T) {
	eng, repo := newTestEngine(t, nil)
	ctx := context.Background()

	project, err := eng.CreateProject(ctx, "Test Project")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Seed root patent
	rootNum := domain.MustParsePatentNumber("US10000000B2")
	rootPat := domain.Patent{
		Number:          rootNum,
		DisplayNumber:   rootNum,
		Title:           "Root Nuclear Gauge Device",
		FetchState:      domain.FetchCached,
		Source:          domain.SourceUSPTO,
		ApplicationDate: time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
		PublicationDate: time.Date(2015, 7, 1, 0, 0, 0, 0, time.UTC),
		GrantDate:       time.Date(2018, 5, 1, 0, 0, 0, 0, time.UTC),
		ExpirationDate:  time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := repo.SavePatent(ctx, rootPat); err != nil {
		t.Fatalf("SavePatent root: %v", err)
	}

	// Seed parent patent with a continuity relation where rootNum is the parent of parentNum
	parentNum := domain.MustParsePatentNumber("US9000000B2")
	parentPat := domain.Patent{
		Number:          parentNum,
		DisplayNumber:   parentNum,
		Title:           "Base Gauge Sensor",
		FetchState:      domain.FetchCached,
		Source:          domain.SourceUSPTO,
		ApplicationDate: time.Date(2010, 2, 1, 0, 0, 0, 0, time.UTC),
		GrantDate:       time.Date(2014, 3, 1, 0, 0, 0, 0, time.UTC),
		ExpirationDate:  time.Date(2030, 2, 1, 0, 0, 0, 0, time.UTC),
	}
	continuity := domain.USPTOContinuity{
		ApplicationNumber:                 "9000000",
		ParentApplicationNumberText:       "10000000",
		ChildApplicationNumberText:        "9000000",
		ClaimParentageTypeCode:            "CON",
		ClaimParentageTypeDescriptionText: "continuation",
	}
	if err := repo.SaveNode(ctx, store.NodeBatch{
		Patent: parentPat,
		USPTOApplication: &domain.USPTOApplication{
			ApplicationNumber: "9000000",
			RecordNumber:      parentNum,
		},
		USPTOContinuities: []domain.USPTOContinuity{continuity},
	}); err != nil {
		t.Fatalf("SaveNode parent: %v", err)
	}

	// Save relations between root and parent
	rel1 := domain.Relation{
		From: parentNum,
		To:   rootNum,
		Kind: domain.RelationParent,
	}
	if err := repo.SaveRelation(ctx, rel1); err != nil {
		t.Fatalf("SaveRelation rel1: %v", err)
	}

	rel2 := domain.Relation{
		From: rootNum,
		To:   parentNum,
		Kind: domain.RelationChild,
	}
	if err := repo.SaveRelation(ctx, rel2); err != nil {
		t.Fatalf("SaveRelation rel2: %v", err)
	}

	// Run ExportFamilyGraph
	tmpDir := t.TempDir()
	exportPath := filepath.Join(tmpDir, "family.mermaid")

	params := proto.FamilyGraphParams{
		Root:     rootNum,
		Depth:    2,
		MaxNodes: 60,
		Project:  project.ID,
	}

	res, err := eng.ExportFamilyGraph(ctx, params, exportPath)
	if err != nil {
		t.Fatalf("ExportFamilyGraph: %v", err)
	}

	if res.Path != exportPath {
		t.Errorf("expected Path %q, got %q", exportPath, res.Path)
	}

	// Check exported flowchart Mermaid file
	mermaidBytes, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("ReadFile exportPath: %v", err)
	}
	mermaidStr := string(mermaidBytes)
	if !strings.Contains(mermaidStr, "flowchart TD") {
		t.Errorf("flowchart mermaid file missing flowchart TD header:\n%s", mermaidStr)
	}
	if !strings.Contains(mermaidStr, "p_us_10000000_b2") {
		t.Errorf("flowchart mermaid file missing root node:\n%s", mermaidStr)
	}
	if !strings.Contains(mermaidStr, "p_us_9000000_b2") {
		t.Errorf("flowchart mermaid file missing parent node:\n%s", mermaidStr)
	}
	if !strings.Contains(mermaidStr, "p_us_10000000_b2 -->|CON| p_us_9000000_b2") {
		t.Errorf("flowchart mermaid file missing CON relation arrow:\n%s", mermaidStr)
	}

	// Check exported ASCII preview (.txt) file
	txtPath := filepath.Join(tmpDir, "family.txt")
	txtBytes, err := os.ReadFile(txtPath)
	if err != nil {
		t.Fatalf("ReadFile txtPath: %v", err)
	}
	txtStr := string(txtBytes)
	if !strings.Contains(txtStr, "Root Patent:") {
		t.Errorf("ASCII preview missing section header:\n%s", txtStr)
	}
	if !strings.Contains(txtStr, "Family Timeline:") {
		t.Errorf("ASCII preview missing timeline section header:\n%s", txtStr)
	}
	if !strings.Contains(txtStr, "2010-02-01 : US9,000,000B2 🏆 Filed") {
		t.Errorf("ASCII preview missing parent filing event:\n%s", txtStr)
	}
	if !strings.Contains(txtStr, "2035-01-02") && !strings.Contains(txtStr, "2035-01-01 : US10,000,000B2 🏆 Expired") {
		t.Errorf("ASCII preview missing root expiration event:\n%s", txtStr)
	}

	// Check exported timeline Mermaid file
	timelinePath := filepath.Join(tmpDir, "family.timeline.mermaid")
	timelineBytes, err := os.ReadFile(timelinePath)
	if err != nil {
		t.Fatalf("ReadFile timelinePath: %v", err)
	}
	timelineStr := string(timelineBytes)
	if !strings.Contains(timelineStr, "timeline") {
		t.Errorf("timeline mermaid file missing timeline header:\n%s", timelineStr)
	}
	if !strings.Contains(timelineStr, "2010 : US9,000,000B2 🏆 Filed") {
		t.Errorf("timeline mermaid file missing parent filing year:\n%s", timelineStr)
	}
}
