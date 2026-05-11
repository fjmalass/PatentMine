package importer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	}

	continuity := usptoContinuityResponse{}
	forwardNums := []string{"US11720555"}

	bundle := buildUSPTOBundle("US11611785B2", data, continuity, forwardNums, nil)

	if bundle.Patent.Number != "US11611785" {
		t.Errorf("expected number US11611785, got %q", bundle.Patent.Number)
	}
	if bundle.Patent.Title != "Test Patent" {
		t.Errorf("expected title 'Test Patent', got %q", bundle.Patent.Title)
	}
	if bundle.Patent.Assignee != "Example Corp" {
		t.Errorf("expected assignee 'Example Corp', got %q", bundle.Patent.Assignee)
	}
	if len(bundle.Patent.Inventors) != 2 {
		t.Errorf("expected 2 inventors, got %d", len(bundle.Patent.Inventors))
	}
	if bundle.Patent.Inventors[0] != "Avery Chen" {
		t.Errorf("expected first inventor Avery Chen, got %q", bundle.Patent.Inventors[0])
	}
	if len(bundle.Classifications) != 2 {
		t.Errorf("expected 2 classifications, got %d", len(bundle.Classifications))
	}
	if len(bundle.Citations) != 2 {
		t.Errorf("expected 2 citations, got %d", len(bundle.Citations))
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

	if bundle.Patent.Title != "Mocked USPTO Patent" {
		t.Errorf("expected title 'Mocked USPTO Patent', got %q", bundle.Patent.Title)
	}
}

func TestImportUSPTORespectsMissingAPIKey(t *testing.T) {
	_, err := ImportUSPTO("US11611785B2", "  ", nil)
	if err == nil || !strings.Contains(err.Error(), "API key is required") {
		t.Errorf("expected 'API key is required' error, got %v", err)
	}
}
