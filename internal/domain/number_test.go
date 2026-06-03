package domain

import "testing"

func TestParsePatentNumber(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want PatentNumber
	}{
		{"compact", "US11611785B2", PatentNumber{"US", "11611785", "B2"}},
		{"hyphens", "US-11611785-B2", PatentNumber{"US", "11611785", "B2"}},
		{"commas and spaces", "US 11,611,785 B2", PatentNumber{"US", "11611785", "B2"}},
		{"lowercase", "us11611785b2", PatentNumber{"US", "11611785", "B2"}},
		{"european", "EP1234567A1", PatentNumber{"EP", "1234567", "A1"}},
		{"wipo", "WO2020123456A1", PatentNumber{"WO", "2020123456", "A1"}},
		{"no country", "11611785B2", PatentNumber{"", "11611785", "B2"}},
		{"no kind", "US11611785", PatentNumber{"US", "11611785", ""}},
		{"serial only", "11611785", PatentNumber{"", "11611785", ""}},
		{"surrounding space", "  US11611785B2  ", PatentNumber{"US", "11611785", "B2"}},
		{"with brackets", "[US11611785B2]", PatentNumber{"US", "11611785", "B2"}},
		{"with application brackets", "[17696256]", PatentNumber{"", "17696256", ""}},
		{"google application with colon", "US: 12/820,712", PatentNumber{"US", "12820712", ""}},
		{"google publication with colon", "US: 2010/0282272 A1", PatentNumber{"US", "20100282272", "A1"}},
		{"leading-zero grant stripped", "US09658068B2", PatentNumber{"US", "9658068", "B2"}},
		{"leading-zero grant no country", "09658068B2", PatentNumber{"", "9658068", "B2"}},
		{"bare application keeps series zero", "09658068", PatentNumber{"", "09658068", ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParsePatentNumber(c.raw)
			if err != nil {
				t.Fatalf("ParsePatentNumber(%q) error: %v", c.raw, err)
			}
			if got != c.want {
				t.Fatalf("ParsePatentNumber(%q) = %+v, want %+v", c.raw, got, c.want)
			}
		})
	}
}

func TestParsePatentNumberErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"letters only", "ABCDEF"},
		{"country only", "US"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParsePatentNumber(c.raw); err == nil {
				t.Fatalf("ParsePatentNumber(%q) expected error, got nil", c.raw)
			}
		})
	}
}

func TestPatentNumberEqualAndNormalize(t *testing.T) {
	a := MustParsePatentNumber("US-11,611,785-B2")
	b := MustParsePatentNumber("us 11611785 b2")
	if !a.Equal(b) {
		t.Fatalf("expected %v equal to %v", a, b)
	}
	if a.Normalized() != "US11611785B2" {
		t.Fatalf("Normalized() = %q, want US11611785B2", a.Normalized())
	}
	if (PatentNumber{}).IsZero() != true {
		t.Fatal("zero value should report IsZero")
	}
	if a.IsZero() {
		t.Fatal("parsed number should not report IsZero")
	}
}

func TestPatentNumberDisplayString(t *testing.T) {
	cases := []struct {
		name string
		num  PatentNumber
		want string
	}{
		{"grant", PatentNumber{"US", "11611785", "B2"}, "US11611785B2"},
		{"application", PatentNumber{"US", "16123456", ""}, "US16123456 (App)"},
		{"publication", PatentNumber{"US", "20200123456", "A1"}, "US20200123456A1 (Pub)"},
		{"zero", PatentNumber{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.num.DisplayString(); got != c.want {
				t.Errorf("%v.DisplayString() = %q, want %q", c.num, got, c.want)
			}
		})
	}
}

