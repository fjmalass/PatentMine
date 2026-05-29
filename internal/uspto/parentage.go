package uspto

import "strings"

// USPTO claim-parentage type codes, as returned in claimParentageTypeCode.
// Named so no bare code string appears in the lookup or classification logic.
const (
	CodeContinuation         = "CON"
	CodeContinuationInPart   = "CIP"
	CodeDivision             = "DIV"
	CodeProvisional          = "PRO"
	CodeProvisionalLong      = "PROV"
	CodeReissue              = "REI"
	CodeReissueLong          = "REISSUE"
	CodeNationalStage371     = "371"
	CodeNationalStage        = "NST"
	CodeSubstitute           = "SUB"
	CodeContinuedProsecution = "CPA"
)

// parentageLabels maps a USPTO claim-parentage type code to a canonical human
// phrase. It is the source of truth for how a parent relationship is described,
// so the wording stays consistent regardless of the free-text the USPTO API
// returns. Codes are normalized (upper-cased, trimmed) before lookup.
var parentageLabels = map[string]string{
	CodeContinuation:         "Continuation of",
	CodeContinuationInPart:   "Continuation-in-part of",
	CodeDivision:             "Division of",
	CodeProvisional:          "Provisional for",
	CodeProvisionalLong:      "Provisional for",
	CodeReissue:              "Reissue of",
	CodeReissueLong:          "Reissue of",
	CodeNationalStage371:     "National Stage of",
	CodeNationalStage:        "National Stage of",
	CodeSubstitute:           "Substitute for",
	CodeContinuedProsecution: "Continued prosecution of",
}

// noLabel is shown when a relationship has neither a known code nor a
// description to fall back to.
const noLabel = "—"

// ParentageLabel returns the canonical phrase for a claim-parentage type code.
// It prefers the static table; when the code is unknown it falls back to the
// description text the USPTO supplied, then to the raw code, then to a dash.
func ParentageLabel(code, fallbackDesc string) string {
	key := normalizeCode(code)
	if phrase, ok := parentageLabels[key]; ok {
		return phrase
	}
	if d := strings.TrimSpace(fallbackDesc); d != "" {
		return d
	}
	if key != "" {
		return key
	}
	return noLabel
}

// Parentage metric buckets. Kept as named constants so the engine and any
// dashboards reference the same strings.
const (
	ParentageContinuation       = "continuation"
	ParentageContinuationInPart = "continuation_in_part"
	ParentageDivision           = "division"
	ParentageProvisional        = "provisional"
	ParentageNationalStage      = "national_stage"
	ParentageReissue            = "reissue"
	ParentageOther              = "other"
)

// ParentageCategory classifies a claim-parentage relationship into a canonical
// bucket used for grouping in metrics and logs.
func ParentageCategory(code, desc string) string {
	key := normalizeCode(code)
	// Provisionals can be flagged by code or description; reuse the same rule the
	// term-walk uses so the two stay in agreement, plus the short code variant.
	if key == CodeProvisional || isProvisional("", key, desc) {
		return ParentageProvisional
	}
	switch key {
	case CodeContinuation:
		return ParentageContinuation
	case CodeContinuationInPart:
		return ParentageContinuationInPart
	case CodeDivision:
		return ParentageDivision
	case CodeNationalStage371, CodeNationalStage:
		return ParentageNationalStage
	case CodeReissue, CodeReissueLong:
		return ParentageReissue
	}
	return ParentageOther
}

func normalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}
