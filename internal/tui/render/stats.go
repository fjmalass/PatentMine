package render

import (
	"fmt"
	"slices"
	"strings"
)

// FormatEntityStats formats aggregated counts for any database entity (inventor, tag, assignee)
// in a fixed-width compact layout using theme glyphs so long rows do not spill outside popup borders.
func FormatEntityStats(theme Theme, total int, states map[string]int, tags map[string]int) string {
	unknown := states["unknown"]
	underReview := states["under_review"]
	active := states["active"]
	ignored := states["ignored"]

	tagsStr := FormatTagsForSort(tags)
	if tagsStr == "" {
		tagsStr = "-"
	}

	// Format: PATENTS (width 8)  STATES (width 25)  TAGS
	patentsStr := fmt.Sprintf("  %3d   ", total)
	statesStr := fmt.Sprintf("%s %d  %s %d  %s %d  %s %d",
		theme.Glyphs.ReviewStateUnknown, unknown,
		theme.Glyphs.ReviewStateUnderReview, underReview,
		theme.Glyphs.ReviewStateActive, active,
		theme.Glyphs.ReviewStateIgnored, ignored,
	)

	return fmt.Sprintf("%s %-25s  %s", patentsStr, statesStr, tagsStr)
}

// FormatTagsForSort returns a deterministic comma-separated string of tags and counts for sorting.
func FormatTagsForSort(tags map[string]int) string {
	if len(tags) == 0 {
		return ""
	}
	var tagNames []string
	for name := range tags {
		tagNames = append(tagNames, name)
	}
	slices.Sort(tagNames)
	var parts []string
	for _, name := range tagNames {
		parts = append(parts, fmt.Sprintf("[%d]%s", tags[name], name))
	}
	return strings.Join(parts, ", ")
}
