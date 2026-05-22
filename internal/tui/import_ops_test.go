package tui

import (
	"testing"

	"patentmine/internal/domain"
)

func TestBundleForCitationTargetRetargetsImportedMetadata(t *testing.T) {
	bundle := domain.PatentBundle{
		Patent: domain.Patent{Number: "US8164048", Title: "Canonical grant"},
		Sections: []domain.PatentTextSection{{
			PatentNumber: "US8164048",
			SectionType:  "abstract",
			Ordinal:      1,
			Text:         "abstract",
		}},
		Citations: []domain.CitationEdge{{
			SourcePatent: "US8164048",
			TargetPatent: "US1",
			RelationType: domain.RelationCites,
		}},
		FamilyEdges: []domain.FamilyEdge{{
			ParentNumber: "US8164048",
			ChildNumber:  "US2",
			RelationType: domain.FamilyRelationContinuation,
		}},
		Classifications: []domain.Classification{{
			PatentNumber: "US8164048",
			Code:         "G06F16/00",
		}},
		References: []domain.ReferenceEntry{{
			PatentNumber:  "US8164048",
			CitationLabel: "US8164048, Canonical grant",
		}},
	}

	got := bundleForCitationTarget(bundle, "US20090250599A1")

	if got.Patent.Number != "US20090250599A1" {
		t.Fatalf("expected patent number to be retargeted, got %+v", got.Patent)
	}
	if got.Sections[0].PatentNumber != "US20090250599A1" ||
		got.Citations[0].SourcePatent != "US20090250599A1" ||
		got.FamilyEdges[0].ParentNumber != "US20090250599A1" ||
		got.Classifications[0].PatentNumber != "US20090250599A1" ||
		got.References[0].PatentNumber != "US20090250599A1" {
		t.Fatalf("expected bundle children to be retargeted, got %+v", got)
	}
}
