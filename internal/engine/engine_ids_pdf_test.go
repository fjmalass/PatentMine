package engine

import (
	"testing"

	"patentmine/internal/domain"
)

func TestIDSCountryFor(t *testing.T) {
	cases := []struct {
		name   string
		patent domain.Patent
		want   string
	}{
		{
			name:   "stored country wins",
			patent: domain.Patent{Number: domain.MustParsePatentNumber("US11611785B2")},
			want:   "US",
		},
		{
			name:   "missing country falls back to placeholder",
			patent: domain.Patent{Number: domain.PatentNumber{Serial: "11611785", Kind: "B2"}},
			want:   idsUnknownPlaceholder,
		},
		{
			name:   "foreign country preserved",
			patent: domain.Patent{Number: domain.MustParsePatentNumber("EP1234567A1")},
			want:   "EP",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := idsCountryFor(c.patent); got != c.want {
				t.Fatalf("idsCountryFor = %q, want %q", got, c.want)
			}
		})
	}
}

func TestIDSKindFor(t *testing.T) {
	cases := []struct {
		name string
		doc  domain.Document
		want string
	}{
		{"A1 accepted", domain.Document{Number: domain.MustParsePatentNumber("US20210000001A1")}, "A1"},
		{"A2 accepted", domain.Document{Number: domain.MustParsePatentNumber("US20210000001A2")}, "A2"},
		{"B1 accepted", domain.Document{Number: domain.MustParsePatentNumber("US11611785B1")}, "B1"},
		{"B2 accepted", domain.Document{Number: domain.MustParsePatentNumber("US11611785B2")}, "B2"},
		{"missing kind defaults", domain.Document{Number: domain.MustParsePatentNumber("US11611785")}, idsUnknownPlaceholder},
		{"non-whitelisted kind defaults", domain.Document{Number: domain.PatentNumber{Country: "US", Serial: "11611785", Kind: "C9"}}, idsUnknownPlaceholder},
		{"empty number defaults", domain.Document{}, idsUnknownPlaceholder},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := idsKindFor(c.doc); got != c.want {
				t.Fatalf("idsKindFor = %q, want %q", got, c.want)
			}
		})
	}
}
