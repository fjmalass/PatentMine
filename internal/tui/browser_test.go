package tui

import "testing"

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
