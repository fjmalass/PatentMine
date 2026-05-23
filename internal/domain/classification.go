package domain

import (
	"regexp"
	"strings"
	"unicode"
)

// Classification represents a parsed classification code (like CPC or USPC).
type Classification struct {
	System      string `json:"system"`
	Code        string `json:"code"`
	Section     string `json:"section"`
	Class       string `json:"class"`
	Subclass    string `json:"subclass"`
	MainGroup   string `json:"main_group"`
	Subgroup    string `json:"subgroup"`
	Description string `json:"description"`
}

// cpcCodeRE matches a syntactically valid CPC code. CPC sections are A–H and Y
// only; class is exactly two digits; subclass is one letter; group is 1–4
// digits with an optional 1–6 digit subgroup after a "/". Spaces between the
// subclass and the group are tolerated (some sources render "G06F 17/30").
var cpcCodeRE = regexp.MustCompile(`^[A-HY][0-9]{2}[A-Z]( ?[0-9]{1,4}(/[0-9]{1,6})?)?$`)

// ParseClassification determines the system (CPC, USPC, or Other) and breaks
// CPC codes into their hierarchical components. Strings that begin with a
// letter but don't satisfy the CPC syntax (e.g. USPTO status codes like
// "STCB") are returned with System="Other" so callers know to skip the
// CPC-only EPO lookup.
func ParseClassification(rawCode string) Classification {
	code := strings.TrimSpace(rawCode)
	if code == "" {
		return Classification{}
	}

	if unicode.IsDigit(rune(code[0])) {
		return parseUSPC(code)
	}
	if cpcCodeRE.MatchString(strings.ReplaceAll(code, " ", "")) {
		return parseCPC(code)
	}
	return Classification{System: "Other", Code: code}
}

func parseCPC(code string) Classification {
	c := Classification{
		System: "CPC",
		Code:   code,
	}

	// Remove spaces for easier parsing of components
	cleanCode := strings.ReplaceAll(code, " ", "")

	// CPC Structure: Section (1) -> Class (2) -> Subclass (1) -> Group/Subgroup
	if len(cleanCode) >= 1 {
		c.Section = cleanCode[0:1]
	}
	if len(cleanCode) >= 3 {
		c.Class = cleanCode[1:3]
	}
	if len(cleanCode) >= 4 {
		c.Subclass = cleanCode[3:4]
	}
	if len(cleanCode) > 4 {
		rest := cleanCode[4:]
		parts := strings.Split(rest, "/")
		c.MainGroup = parts[0]
		if len(parts) > 1 {
			c.Subgroup = parts[1]
		}
	}

	return c
}

func parseUSPC(code string) Classification {
	c := Classification{
		System: "USPC",
		Code:   code,
	}

	// USPC Structure: Class / Subclass
	parts := strings.Split(code, "/")
	if len(parts) >= 1 {
		c.Class = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 2 {
		c.Subclass = strings.TrimSpace(parts[1])
	}

	return c
}
