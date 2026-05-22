package render

import (
	"fmt"
	"slices"
	"strings"
)

// FormatEntityStats formats aggregated counts for any database entity (inventor, tag, assignee)
// in a premium, standardized layout with perfect horizontal alignment:
// "[xx]:  state: [y] stored, [a] under_review, [o] other, tags: [t1] tag1, [t2] tag2, .."
func FormatEntityStats(total int, states map[string]int, tags map[string]int) string {
	stored := states["stored"]
	underReview := states["under_review"]
	other := states["other"]

	var tagParts []string
	if len(tags) > 0 {
		var tagNames []string
		for name := range tags {
			tagNames = append(tagNames, name)
		}
		slices.Sort(tagNames)
		for _, name := range tagNames {
			tagParts = append(tagParts, fmt.Sprintf("[%d] %s", tags[name], name))
		}
	}

	tagsStr := "none"
	if len(tagParts) > 0 {
		tagsStr = strings.Join(tagParts, ", ")
	}

	// Pad the "[total]:" part to 6 columns so the "state:" label is perfectly aligned across rows.
	totalLabel := fmt.Sprintf("[%d]:", total)
	paddedTotal := Pad(totalLabel, 6)

	return fmt.Sprintf("%s state: [%d] stored, [%d] under_review, [%d] other, tags: %s",
		paddedTotal, stored, underReview, other, tagsStr)
}
