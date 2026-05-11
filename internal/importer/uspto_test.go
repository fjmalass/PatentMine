package importer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"patentmine/internal/domain"
)

func TestExtractODPNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"US11611785B2", "11611785"},
		{"US20230000001A1", "20230000001"},
		{"12345678", "12345678"},
		{"US12345678", "12345678"},
		{"RE12345E1", "12345"},
	}

	for _, tt := range tests {
		if got := extractODPNumber(tt.input); got != tt.expected {
			t.Errorf("extractODPNumber(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestUSPTONormalizePatentNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"11611785", "US11611785"},
		{"US11611785", "US11611785"},
		{"  12345  ", "US12345"},
		{"us123", "US123"},
	}

	for _, tt := range tests {
		if got := usptoNormalizePatentNumber(tt.input); got != tt.expected {
			t.Errorf("usptoNormalizePatentNumber(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestBuildUSPTOBundle(t *testing.T) {
	data := usptoApplicationData{
		ApplicationNumberText: "17123456",
		ApplicationMetaData: usptoMetaData{
			PatentNumber:            "11611785",
			InventionTitle:          "Test Patent",
			EarliestPublicationDate: "2023-03-21",
			GrantDate:               "2023-03-21",
			InventorBag: []usptoInventor{
				{FirstName: "Avery", LastName: "Chen"},
				{InventorNameText: "Morgan Patel"},
			},
			CPCClassificationBag: []string{"G06F16/33", "G06N20/00"},
			ReferenceCitedBag: []usptoReferenceCited{
				{PatentNumber: "10123456", PatentKindCode: "B2"},
			},
		},
		AssignmentBag: []usptoAssignment{
			{
				AssigneeBag: []usptoAssignee{
					{AssigneeNameText: "Example Corp"},
				},
			},
		},
		ParentContinuityBag: []usptoContinuity{
			{ParentPatentNumber: "9000000", ClaimParentageTypeCode: "DIV"},
		},
		ChildContinuityBag: []usptoContinuity{
			{ChildPatentNumber: "12000000", ClaimParentageTypeCode: "CON"},
		},
	}

	continuity := usptoContinuityResponse{}
	forwardNums := []string{"US11720555"}

	bundle := buildUSPTOBundle("US11611785B2", data, continuity, forwardNums, nil)

	if bundle.Patent.Number != "US11611785" {
		t.Errorf("expected number US11611785, got %q", bundle.Patent.Number)
	}
	if bundle.Patent.Assignee != "Example Corp" {
		t.Errorf("expected assignee 'Example Corp', got %q", bundle.Patent.Assignee)
	}

	// Verify Citations
	foundBackward := false
	foundForward := false
	for _, c := range bundle.Citations {
		if c.TargetPatent == "US10123456" && c.RelationType == domain.RelationCites {
			foundBackward = true
		}
		if c.TargetPatent == "US11720555" && c.RelationType == domain.RelationCitedBy {
			foundForward = true
		}
	}
	if !foundBackward {
		t.Error("backward citation (US10123456) missing")
	}
	if !foundForward {
		t.Error("forward citation (US11720555) missing")
	}

	// Verify Family Edges
	foundParent := false
	foundChild := false
	for _, f := range bundle.FamilyEdges {
		if f.ParentNumber == "US9000000" && f.ChildNumber == "US11611785" {
			foundParent = true
		}
		if f.ParentNumber == "US11611785" && f.ChildNumber == "US12000000" {
			foundChild = true
		}
	}
	if !foundParent {
		t.Error("parent family edge missing")
	}
	if !foundChild {
		t.Error("child family edge missing")
	}
}

func TestImportUSPTOUsesAPIKey(t *testing.T) {
	apiKey := "test-secret-key"
	patentNum := "US11611785B2"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify API Key header
		if got := r.Header.Get("X-API-KEY"); got != apiKey {
			t.Errorf("expected X-API-KEY %q, got %q", apiKey, got)
		}

		// Mock responses based on URL
		if strings.Contains(r.URL.Path, "/applications/search") {
			if strings.Contains(r.URL.RawQuery, "forwardReferencedPatentNumber") {
				// Forward citations
				json.NewEncoder(w).Encode(usptoSearchResponse{Count: 0})
				return
			}
			// Initial search
			json.NewEncoder(w).Encode(usptoSearchResponse{
				Count: 1,
				PatentFileWrapperDataBag: []usptoApplicationData{
					{
						ApplicationNumberText: "17123456",
						ApplicationMetaData: usptoMetaData{
							PatentNumber: "11611785",
						},
					},
				},
			})
		} else if strings.Contains(r.URL.Path, "/applications/17123456/continuity") {
			json.NewEncoder(w).Encode(usptoContinuityResponse{})
		} else if strings.Contains(r.URL.Path, "/applications/17123456") {
			// Full metadata
			json.NewEncoder(w).Encode(usptoSearchResponse{
				Count: 1,
				PatentFileWrapperDataBag: []usptoApplicationData{
					{
						ApplicationNumberText: "17123456",
						ApplicationMetaData: usptoMetaData{
							PatentNumber:   "11611785",
							InventionTitle: "Mocked USPTO Patent",
						},
					},
				},
			})
		}
	}))
	defer server.Close()

	// Override base URL for test
	oldURL := usptoBaseURL
	usptoBaseURL = server.URL
	defer func() { usptoBaseURL = oldURL }()

	bundle, err := ImportUSPTO(patentNum, apiKey, nil)
	if err != nil {
		t.Fatalf("ImportUSPTO failed: %v", err)
	}

	if bundle.Patent.Title == "" {
		t.Errorf("expected a title, got empty")
	}
}

func TestImportUSPTORespectsMissingAPIKey(t *testing.T) {
	_, err := ImportUSPTO("US11611785B2", "  ", nil)
	if err == nil || !strings.Contains(err.Error(), "API key is required") {
		t.Errorf("expected 'API key is required' error, got %v", err)
	}
}

func TestToTitleCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SYSTEM AND METHOD FOR DATA PROCESSING", "System and Method for Data Processing"},
		{"A NOVEL APPARATUS WITH IMPROVED EFFICIENCY", "A Novel Apparatus with Improved Efficiency"},
		{"METHOD OF OPERATING AN ENGINE", "Method of Operating an Engine"},
		{"THE QUICK BROWN FOX JUMPS OVER THE LAZY DOG", "The Quick Brown Fox Jumps Over the Lazy Dog"},
		{"highly optimized system", "Highly Optimized System"}, // Adverbs capitalized
	}

	for _, tt := range tests {
		if got := toTitleCase(tt.input); got != tt.expected {
			t.Errorf("toTitleCase(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestImportUSPTOHandles404(t *testing.T) {
	apiKey := "test-key"
	patentNum := "US11611785B2"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/applications/search") {
			// Initial search succeeds
			json.NewEncoder(w).Encode(usptoSearchResponse{
				Count: 1,
				PatentFileWrapperDataBag: []usptoApplicationData{
					{
						ApplicationNumberText: "17123456",
						ApplicationMetaData: usptoMetaData{
							PatentNumber: "11611785",
						},
					},
				},
			})
		} else if strings.Contains(r.URL.Path, "17123456/meta-data") {
			// Meta-data succeeds
			json.NewEncoder(w).Encode(usptoSearchResponse{
				Count: 1,
				PatentFileWrapperDataBag: []usptoApplicationData{
					{
						ApplicationNumberText: "17123456",
						ApplicationMetaData: usptoMetaData{
							PatentNumber:   "11611785",
							InventionTitle: "Mocked USPTO Patent",
						},
					},
				},
			})
		} else {
			// Everything else returns 404 (assignments, adjustments, continuity, forward citations)
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"404","message":"Not Found"}`))
		}
	}))
	defer server.Close()

	oldURL := usptoBaseURL
	usptoBaseURL = server.URL
	defer func() { usptoBaseURL = oldURL }()

	bundle, err := ImportUSPTO(patentNum, apiKey, nil)
	if err != nil {
		t.Fatalf("ImportUSPTO failed on 404s: %v", err)
	}

	if bundle.Patent.Title != "Mocked Uspto Patent" {
		t.Errorf("expected title 'Mocked Uspto Patent', got %q", bundle.Patent.Title)
	}
}
