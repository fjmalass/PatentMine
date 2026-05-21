package citationlookup

import (
	"strings"
	"unicode"

	"patentmine/internal/domain"
)

// NormalizePatentID canonicalizes common US patent input forms for cache keys.
// Examples: 8164048 -> US8164048, us-8164048-b2 -> US8164048B2.
func NormalizePatentID(raw string) domain.PatentNumber {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	value = b.String()
	if value == "" {
		return ""
	}

	if unicode.IsDigit(rune(value[0])) {
		value = "US" + value
	}
	return domain.PatentNumber(value)
}
