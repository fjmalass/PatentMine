package tui

import (
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

func TestWithUSPTOAPIKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		key  string
		want string
	}{
		{
			name: "appends key to USPTO ODP URL",
			in:   "https://api.uspto.gov/api/v1/datasets/products/files/PTGRXML-SPLT/2024/ipg240305/17947573_11921100.xml",
			key:  "abc123",
			want: "https://api.uspto.gov/api/v1/datasets/products/files/PTGRXML-SPLT/2024/ipg240305/17947573_11921100.xml?api_key=abc123",
		},
		{
			name: "preserves existing query",
			in:   "https://api.uspto.gov/api/v1/patent/applications/search?q=foo",
			key:  "abc",
			want: "https://api.uspto.gov/api/v1/patent/applications/search?api_key=abc&q=foo",
		},
		{
			name: "skips when api_key already present",
			in:   "https://api.uspto.gov/api/v1/x?api_key=existing",
			key:  "new",
			want: "https://api.uspto.gov/api/v1/x?api_key=existing",
		},
		{
			name: "ignores non-USPTO host",
			in:   "https://patents.google.com/patent/US123",
			key:  "abc",
			want: "https://patents.google.com/patent/US123",
		},
		{
			name: "no-op when key empty",
			in:   "https://api.uspto.gov/api/v1/x",
			key:  "",
			want: "https://api.uspto.gov/api/v1/x",
		},
		{
			name: "case-insensitive host match",
			in:   "https://API.USPTO.GOV/x",
			key:  "k",
			want: "https://API.USPTO.GOV/x?api_key=k",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := withUSPTOAPIKey(tc.in, tc.key)
			if got != tc.want {
				t.Fatalf("withUSPTOAPIKey(%q, %q) = %q, want %q", tc.in, tc.key, got, tc.want)
			}
		})
	}
}

func TestResolveBrowseURLUSPTOSelection(t *testing.T) {
	number := domain.MustParsePatentNumber("US17730671")
	res := &proto.PatentResult{
		Patent: domain.Patent{Number: number, DisplayNumber: number, Source: domain.SourceUSPTO},
		USPTOApplication: &domain.USPTOApplication{
			PatentGrantXMLURL: "https://api.uspto.gov/grant.xml",
			PGPubXMLURL:       "https://api.uspto.gov/pub.xml",
		},
	}

	got, errText := resolveBrowseURL(number, res, browseTargetUSPTOSmart)
	if errText != "" {
		t.Fatalf("smart USPTO browse error = %q", errText)
	}
	if got.URL != "https://api.uspto.gov/grant.xml" || got.Kind != "grant" || got.Provider != "uspto" {
		t.Fatalf("smart USPTO browse = %+v, want grant URL", got)
	}

	res.USPTOApplication.PatentGrantXMLURL = ""
	got, errText = resolveBrowseURL(number, res, browseTargetUSPTOSmart)
	if errText != "" {
		t.Fatalf("smart USPTO browse fallback error = %q", errText)
	}
	if got.URL != "https://api.uspto.gov/pub.xml" || got.Kind != "pgpub" {
		t.Fatalf("smart USPTO fallback = %+v, want pgpub URL", got)
	}
}

func TestResolveBrowseURLForcedGoogle(t *testing.T) {
	number := domain.MustParsePatentNumber("US17730671")
	res := &proto.PatentResult{
		Patent: domain.Patent{
			Number:    number,
			Source:    domain.SourceUSPTO,
			SourceURL: "https://api.uspto.gov/source",
			Documents: []domain.Document{{Number: domain.MustParsePatentNumber("US11921100B2"), Stage: domain.StageGrant}},
		},
	}

	got, errText := resolveBrowseURL(number, res, browseTargetGoogle)
	if errText != "" {
		t.Fatalf("google browse error = %q", errText)
	}
	if got.URL != "https://patents.google.com/patent/US11921100B2/en" || got.Provider != "google" {
		t.Fatalf("google browse = %+v", got)
	}
}

// TestResolveBrowseURLDefaultUsesGoogleGrant verifies the plain "w" (default)
// browse target opens the granted Google Patents page rather than the recorded
// SourceURL — which in USPTO-only mode is the USPTO ODP API query, not a
// browsable patent page. This mirrors the detail view's Source URL override.
func TestResolveBrowseURLDefaultUsesGoogleGrant(t *testing.T) {
	number := domain.MustParsePatentNumber("US17730671")
	res := &proto.PatentResult{
		Patent: domain.Patent{
			Number:    number,
			Source:    domain.SourceUSPTO,
			SourceURL: "https://api.uspto.gov/api/v1/patent/applications/search?q=applicationNumberText:17730671",
			Documents: []domain.Document{{Number: domain.MustParsePatentNumber("US11921100B2"), Stage: domain.StageGrant}},
		},
	}

	got, errText := resolveBrowseURL(number, res, browseTargetDefault)
	if errText != "" {
		t.Fatalf("default browse error = %q", errText)
	}
	if got.URL != "https://patents.google.com/patent/US11921100B2/en" || got.Provider != "google" {
		t.Fatalf("default browse = %+v, want granted Google URL", got)
	}
}
