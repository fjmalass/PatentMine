package crawl

import (
	"context"
	"strings"
	"testing"
)

func TestLookupCPCDescription(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	tests := []struct {
		code         string
		expectSubstr string
		expectErr    bool
	}{
		{
			code:         "G06F16/00",
			expectSubstr: "Information retrieval",
			expectErr:    false,
		},
		{
			code:         "G06F 16/00",
			expectSubstr: "Information retrieval",
			expectErr:    false,
		},
		{
			code:         "USPC-NotSupported",
			expectSubstr: "",
			expectErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			desc, err := LookupCPCDescription(context.Background(), tc.code)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error for code %q, got nil", tc.code)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error for code %q: %v", tc.code, err)
			}

			if desc == "" {
				t.Fatalf("expected non-empty description for code %q", tc.code)
			}

			if !strings.Contains(strings.ToLower(desc), strings.ToLower(tc.expectSubstr)) {
				t.Errorf("description %q does not contain expected substring %q", desc, tc.expectSubstr)
			}
		})
	}
}
