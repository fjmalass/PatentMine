package render

import (
	"fmt"
	"slices"
	"strings"
)

// FormatEntityStats formats aggregated counts for any database entity (inventor, tag, assignee)
// in a fixed-width ASCII layout so long rows do not spill outside popup borders.
func FormatEntityStats(total int, states map[string]int, tags map[string]int) string {
	stored := states["stored"]
	underReview := states["under_review"]
	cached := states["cached"]
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

	// Reserve enough room for hundreds or thousands of patents without shifting the following columns.
	totalLabel := fmt.Sprintf("[%4d]", total)
	paddedTotal := Pad(totalLabel, 8)

	return fmt.Sprintf("%s state: [%d] stored, [%d] review, [%d] cached, [%d] other, tags: %s",
		paddedTotal, stored, underReview, cached, other, tagsStr)
}
