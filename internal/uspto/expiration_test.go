package uspto

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

type mockRoundTripper struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

type mockRepository struct {
	store.Repository
	continuities map[string][]domain.USPTOContinuity
}

func (m *mockRepository) USPTOContinuities(ctx context.Context, appNum string) ([]domain.USPTOContinuity, error) {
	if m.continuities == nil {
		return nil, nil
	}
	return m.continuities[appNum], nil
}

func TestPatentExpirationCalculation(t *testing.T) {
	filingDate := time.Date(2010, 6, 15, 0, 0, 0, 0, time.UTC)
	grantDate := time.Date(2013, 3, 12, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name                   string
		patentType             string
		filingDate             time.Time
		grantDate              time.Time
		earliestTermFilingDate time.Time
		ptaDays                int
		pteDays                int
		terminalDisclaimer     time.Time
		want                   time.Time
	}{
		{
			name:       "Utility standard 20 year",
			patentType: "utility",
			filingDate: filingDate,
			grantDate:  grantDate,
			want:       time.Date(2030, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:                   "Utility with earliest term filing date",
			patentType:             "utility",
			filingDate:             filingDate,
			grantDate:              grantDate,
			earliestTermFilingDate: time.Date(2008, 2, 20, 0, 0, 0, 0, time.UTC),
			want:                   time.Date(2028, 2, 20, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "Utility with PTA",
			patentType: "utility",
			filingDate: filingDate,
			grantDate:  grantDate,
			ptaDays:    150,
			want:       time.Date(2030, 6, 15, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 150),
		},
		{
			name:       "Utility with PTE and PTA",
			patentType: "utility",
			filingDate: filingDate,
			grantDate:  grantDate,
			ptaDays:    100,
			pteDays:    50,
			want:       time.Date(2030, 6, 15, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 150),
		},
		{
			name:               "Utility with terminal disclaimer cap",
			patentType:         "utility",
			filingDate:         filingDate,
			grantDate:          grantDate,
			ptaDays:            500,
			terminalDisclaimer: time.Date(2029, 12, 31, 0, 0, 0, 0, time.UTC),
			want:               time.Date(2029, 12, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "Design standard 15 year",
			patentType: "design",
			filingDate: filingDate,
			grantDate:  grantDate,
			want:       time.Date(2028, 3, 12, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "Plant standard 20 year",
			patentType: "plant",
			filingDate: filingDate,
			grantDate:  grantDate,
			want:       time.Date(2030, 6, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PatentExpiration(tc.patentType, tc.filingDate, tc.grantDate, tc.earliestTermFilingDate, tc.ptaDays, tc.pteDays, tc.terminalDisclaimer)
			if !got.Equal(tc.want) {
				t.Errorf("got %s, want %s", got.Format("2006-01-02"), tc.want.Format("2006-01-02"))
			}
		})
	}
}

func TestDeterminePatentType(t *testing.T) {
	cases := []struct {
		name        string
		number      domain.PatentNumber
		appLabel    string
		appCategory string
		grantKind   string
		want        string
	}{
		{
			name:   "Utility by default",
			number: domain.PatentNumber{Country: "US", Serial: "12345678", Kind: "B2"},
			want:   "utility",
		},
		{
			name:     "Design by app label",
			number:   domain.PatentNumber{Country: "US", Serial: "29123456", Kind: ""},
			appLabel: "design application",
			want:     "design",
		},
		{
			name:        "Design by app category",
			number:      domain.PatentNumber{Country: "US", Serial: "29123456", Kind: ""},
			appCategory: "Design",
			want:        "design",
		},
		{
			name:      "Design by grant kind",
			number:    domain.PatentNumber{Country: "US", Serial: "123456", Kind: ""},
			grantKind: "S1",
			want:      "design",
		},
		{
			name:      "Plant by grant kind",
			number:    domain.PatentNumber{Country: "US", Serial: "123456", Kind: ""},
			grantKind: "P3",
			want:      "plant",
		},
		{
			name:     "Plant by app label",
			number:   domain.PatentNumber{Country: "US", Serial: "123456", Kind: ""},
			appLabel: "plant patent application",
			want:     "plant",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeterminePatentType(tc.number, tc.appLabel, tc.appCategory, tc.grantKind)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFetchPTADays(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	var reqHeader http.Header
	var reqURL string

	http.DefaultTransport = &mockRoundTripper{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			reqHeader = req.Header
			reqURL = req.URL.String()

			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
			}
			resp.Header.Set("Content-Type", "application/json")
			resp.Body = io.NopCloser(strings.NewReader(`{"patentTermAdjustmentQuantity": 123}`))
			return resp, nil
		},
	}

	ctx := context.Background()
	pta, err := FetchPTADays(ctx, "16123456", "my-secret-key", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pta != 123 {
		t.Errorf("expected 123 PTA days, got %d", pta)
	}

	if reqHeader.Get("X-API-KEY") != "my-secret-key" {
		t.Errorf("expected X-API-KEY header to be 'my-secret-key', got %q", reqHeader.Get("X-API-KEY"))
	}

	expectedURL := "https://api.uspto.gov/api/v1/patent/applications/16123456/patent-term-adjustment"
	if reqURL != expectedURL {
		t.Errorf("expected URL %q, got %q", expectedURL, reqURL)
	}
}

func TestFetchTerminalDisclaimerDate(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	http.DefaultTransport = &mockRoundTripper{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
			}
			resp.Header.Set("Content-Type", "application/json")
			resp.Body = io.NopCloser(strings.NewReader(`{
				"documentBag": [
					{
						"documentCode": "DIST",
						"documentCodeDescriptionText": "Terminal Disclaimer Filed",
						"mailDate": "2021-06-15"
					},
					{
						"documentCode": "OTHER",
						"documentCodeDescriptionText": "Regular document",
						"mailDate": "2021-07-20"
					}
				]
			}`))
			return resp, nil
		},
	}

	ctx := context.Background()
	tdDate, found, err := FetchTerminalDisclaimerDate(ctx, "16123456", "my-secret-key", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !found {
		t.Errorf("expected terminal disclaimer to be found")
	}

	expectedDate := time.Date(2021, 6, 15, 0, 0, 0, 0, time.UTC)
	if !tdDate.Equal(expectedDate) {
		t.Errorf("expected terminal disclaimer date %v, got %v", expectedDate, tdDate)
	}
}

func TestFetchTerminalDisclaimerDate_Extended(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	http.DefaultTransport = &mockRoundTripper{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
			}
			resp.Header.Set("Content-Type", "application/json")
			resp.Body = io.NopCloser(strings.NewReader(`{
				"documentBag": [
					{
						"documentCode": "DIST.E.FILE",
						"documentCodeDescriptionText": "Terminal Disclaimer",
						"officialDate": "2022-08-10"
					},
					{
						"documentCode": "DIST",
						"documentCodeDescriptionText": "Terminal Disclaimer",
						"mailRoomDate": "2022-08-11"
					}
				]
			}`))
			return resp, nil
		},
	}

	ctx := context.Background()
	tdDate, found, err := FetchTerminalDisclaimerDate(ctx, "16123456", "my-secret-key", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !found {
		t.Errorf("expected terminal disclaimer to be found")
	}

	expectedDate := time.Date(2022, 8, 10, 0, 0, 0, 0, time.UTC)
	if !tdDate.Equal(expectedDate) {
		t.Errorf("expected terminal disclaimer date %v, got %v", expectedDate, tdDate)
	}
}

func TestComputeEarliestTermFilingDate(t *testing.T) {
	mockRepo := &mockRepository{
		continuities: map[string][]domain.USPTOContinuity{
			"17123456": {
				{
					ParentApplicationNumberText: "16987654",
					ParentApplicationFilingDate: "2020-05-01",
					ClaimParentageTypeCode:      "CON",
				},
			},
			"16987654": {
				{
					ParentApplicationNumberText: "15112233",
					ParentApplicationFilingDate: "2018-10-15",
					ClaimParentageTypeCode:      "CIP",
				},
			},
			"15112233": {
				{
					ParentApplicationNumberText: "62111111",
					ParentApplicationFilingDate: "2017-04-20",
					ClaimParentageTypeCode:      "PROV",
				},
			},
		},
	}

	ctx := context.Background()
	filingDate := time.Date(2022, 11, 30, 0, 0, 0, 0, time.UTC)
	earliest, _, err := ComputeEarliestTermFilingDate(ctx, mockRepo, "17123456", filingDate, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := time.Date(2018, 10, 15, 0, 0, 0, 0, time.UTC)
	if !earliest.Equal(expected) {
		t.Errorf("expected earliest date %v, got %v", expected, earliest)
	}
}

func TestComputeEarliestTermFilingDate_Circular(t *testing.T) {
	mockRepo := &mockRepository{
		continuities: map[string][]domain.USPTOContinuity{
			"APP1": {
				{
					ParentApplicationNumberText: "APP2",
					ParentApplicationFilingDate: "2020-01-01",
				},
			},
			"APP2": {
				{
					ParentApplicationNumberText: "APP1",
					ParentApplicationFilingDate: "2021-01-01",
				},
			},
		},
	}

	ctx := context.Background()
	earliest, _, err := ComputeEarliestTermFilingDate(ctx, mockRepo, "APP1", time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if earliest.IsZero() {
		t.Errorf("expected a valid time, got zero")
	}
}
