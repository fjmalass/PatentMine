package domain

import (
	"testing"
	"time"
)

func TestParseStage(t *testing.T) {
	for _, ok := range []string{"application", "publication", "grant"} {
		if _, err := ParseStage(ok); err != nil {
			t.Errorf("ParseStage(%q) error: %v", ok, err)
		}
	}
	if _, err := ParseStage("bogus"); err == nil {
		t.Error("ParseStage(bogus) should fail")
	}
}

func TestGuessStage(t *testing.T) {
	grant := MustParsePatentNumber("US11611785B2")
	if got := GuessStage(grant); got != StageGrant {
		t.Errorf("GuessStage(%s) = %s, want grant", grant, got)
	}
	pub := MustParsePatentNumber("US2020123456A1")
	if got := GuessStage(pub); got != StagePublication {
		t.Errorf("GuessStage(%s) = %s, want publication", pub, got)
	}
	app := MustParsePatentNumber("US16123456")
	if got := GuessStage(app); got != StageApplication {
		t.Errorf("GuessStage(%s) = %s, want application", app, got)
	}
}

func TestLatestDocumentPicksFurthestStage(t *testing.T) {
	app := MustParsePatentNumber("US16000001")
	pub := MustParsePatentNumber("US2020000001A1")
	grant := MustParsePatentNumber("US0000001B2")
	p := Patent{
		Number: app,
		Documents: []Document{
			{Number: app, Stage: StageApplication, Dated: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Number: grant, Stage: StageGrant, Dated: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Number: pub, Stage: StagePublication, Dated: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
	}
	latest, ok := p.LatestDocument()
	if !ok || latest.Stage != StageGrant {
		t.Fatalf("LatestDocument = %+v ok=%v, want the grant", latest, ok)
	}
	if !p.NumberToShow().Equal(grant) {
		t.Fatalf("NumberToShow = %s, want the grant %s", p.NumberToShow(), grant)
	}
}

func TestNumberToShowFallsBackToRecordNumber(t *testing.T) {
	record := MustParsePatentNumber("US16000001")
	p := Patent{Number: record} // no documents
	if !p.NumberToShow().Equal(record) {
		t.Fatalf("NumberToShow = %s, want the record number %s", p.NumberToShow(), record)
	}
}

func TestDocumentFor(t *testing.T) {
	app := MustParsePatentNumber("US16000001")
	grant := MustParsePatentNumber("US0000001B2")
	p := Patent{
		Number: app,
		Documents: []Document{
			{Number: app, Stage: StageApplication},
			{Number: grant, Stage: StageGrant},
		},
	}
	if d, ok := p.DocumentFor(StageGrant); !ok || !d.Number.Equal(grant) {
		t.Fatalf("DocumentFor(grant) = %+v ok=%v", d, ok)
	}
	if _, ok := p.DocumentFor(StagePublication); ok {
		t.Error("DocumentFor(publication) should report missing")
	}
}

func TestParsePatentNumberAcceptsApplicationSlash(t *testing.T) {
	n, err := ParsePatentNumber("US 16/123,456")
	if err != nil {
		t.Fatalf("ParsePatentNumber with a slash: %v", err)
	}
	if n.Country != "US" || n.Serial != "16123456" {
		t.Fatalf("parsed %+v, want US / 16123456", n)
	}
}
