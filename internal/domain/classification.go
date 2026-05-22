package domain

import (
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

// ParseClassification attempts to determine the system (CPC or USPC) and 
// break down the code into its hierarchical components.
func ParseClassification(rawCode string) Classification {
	code := strings.TrimSpace(rawCode)
	if code == "" {
		return Classification{}
	}

	// Basic heuristic: CPC codes usually start with a letter (A-H, Y).
	// USPC codes usually start with digits.
	if len(code) > 0 && unicode.IsLetter(rune(code[0])) {
		return parseCPC(code)
	}
	return parseUSPC(code)
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
