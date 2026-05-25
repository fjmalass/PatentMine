package ids

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"

	"patentmine/internal/domain"
)

func TestBucket_SplitsUSAndForeign(t *testing.T) {
	us := Entry{Number: domain.MustParsePatentNumber("US11611785B2"), Patentee: "Apple"}
	app := Entry{Number: domain.MustParsePatentNumber("US20210000001A1"), Patentee: "Apple"}
	ep := Entry{Number: domain.MustParsePatentNumber("EP1234567A1"), Patentee: "Nokia"}
	jp := Entry{Number: domain.MustParsePatentNumber("JP2020100100A"), Patentee: "Sony"}

	gotUS, gotForeign := Bucket([]Entry{us, app, ep, jp})

	if len(gotUS) != 2 {
		t.Fatalf("us bucket size = %d, want 2", len(gotUS))
	}
	if len(gotForeign) != 2 {
		t.Fatalf("foreign bucket size = %d, want 2", len(gotForeign))
	}
}

func TestFeeTier_Boundaries(t *testing.T) {
	cases := []struct {
		count int
		want  int
	}{
		{0, 0}, {50, 0}, {51, 1}, {100, 1}, {101, 2}, {200, 2}, {201, 3}, {1000, 3},
	}
	for _, c := range cases {
		if got := FeeTier(c.count); got != c.want {
			t.Errorf("FeeTier(%d) = %d, want %d", c.count, got, c.want)
		}
	}
}

func TestPaginate_Counts(t *testing.T) {
	mk := func(n int, country string) []Entry {
		out := make([]Entry, n)
		for i := range out {
			out[i] = Entry{Number: domain.PatentNumber{Country: country, Serial: "1", Kind: "B2"}}
		}
		return out
	}
	cases := []struct {
		us, fr, sheets int
	}{
		{0, 0, 1},
		{1, 0, 1},
		{20, 0, 1},
		{21, 0, 2},
		{0, 6, 1},
		{0, 7, 2},
		{20, 6, 1},
		{21, 7, 2},
		{40, 6, 2},
		{100, 30, 5},
	}
	for _, c := range cases {
		got := Paginate(mk(c.us, "US"), mk(c.fr, "EP"))
		if len(got) != c.sheets {
			t.Errorf("Paginate(us=%d, fr=%d) = %d sheets, want %d", c.us, c.fr, len(got), c.sheets)
		}
	}
}

// TestExport_EndToEnd renders a sample bundle and round-trips each PDF through
// pdfcpu's form exporter to assert the fields we filled actually carry the
// values we set.
func TestExport_EndToEnd(t *testing.T) {
	tmp := t.TempDir()
	in := Input{
		Project: domain.Project{
			ID:                   "p-test",
			Name:                 "Test",
			ApplicationNumber:    "16/123,456",
			FilingDate:           time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Inventors:            []string{"Doe, Jane", "Roe, Richard"},
			ArtUnit:              "2628",
			Examiners: []domain.ProjectExaminer{
				{Name: "Smith, J.", RecordedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
				{Name: "Lee, K.", RecordedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)},
			},
			AttorneyDocketNumber: "ACME-001",
		},
		Entries: []Entry{
			{Number: domain.MustParsePatentNumber("US11611785B2"), Patentee: "Apple", PubDate: time.Date(2023, 3, 21, 0, 0, 0, 0, time.UTC)},
			{Number: domain.MustParsePatentNumber("EP1234567A1"), Patentee: "Nokia", PubDate: time.Date(2010, 5, 5, 0, 0, 0, 0, time.UTC), InFull: true, Passages: "para 12"},
		},
		CumulativeCount: 2,
		GeneratedAt:     time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC),
	}

	res, err := Export(context.Background(), in, tmp, nil)
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	if len(res.Files) != 2 || res.Files[0] != "08a.pdf" || res.Files[1] != "08c.pdf" {
		t.Fatalf("Files = %v, want [08a.pdf 08c.pdf]", res.Files)
	}

	exported := map[string]string{}
	for _, f := range res.Files {
		path := filepath.Join(res.Dir, f)
		jsonPath := path + ".json"
		if err := api.ExportFormFile(path, jsonPath, nil); err != nil {
			t.Fatalf("ExportFormFile %s: %v", f, err)
		}
		bs, err := os.ReadFile(jsonPath)
		if err != nil {
			t.Fatalf("read %s: %v", jsonPath, err)
		}
		exported[f] = string(bs)
	}

	mustContain(t, exported["08a.pdf"], `"name": "text3"`, `"value": "16/123,456"`)
	mustContain(t, exported["08a.pdf"], `"name": "text5"`, `"value": "Doe, Jane"`)
	mustContain(t, exported["08a.pdf"], `"name": "text7"`, `"value": "Lee, K."`)
	mustContain(t, exported["08a.pdf"], `"name": "text10"`, `"value": "11611785-B2"`)
	mustContain(t, exported["08a.pdf"], `"name": "text105"`, `"value": "EP-1234567-A1"`)
	mustContain(t, exported["08c.pdf"], `"name": "Check Box1"`, `"value": true`)

	if res.FeeTier != 0 || res.PageCount != 2 {
		t.Fatalf("Result = %+v", res)
	}
}

func mustContain(t *testing.T, hay string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(hay, n) {
			t.Errorf("missing %q in form export", n)
		}
	}
}
