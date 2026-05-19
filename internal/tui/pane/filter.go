package pane

import (
	"fmt"
	"strings"

	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// PatentFilter holds all filters applied to a patent list or relation view.
type PatentFilter struct {
	Search      string
	ReviewState domain.ReviewState
}

// IsActive reports whether any filter is set.
func (f PatentFilter) IsActive() bool {
	return f.Search != "" || f.ReviewState != domain.ReviewStateNone
}

// Labels returns human-readable filter labels for the current state.
func (f PatentFilter) Labels() []string {
	var labels []string
	if f.ReviewState != domain.ReviewStateNone {
		labels = append(labels, "state:"+string(f.ReviewState))
	}
	if f.Search != "" {
		labels = append(labels, "search:"+f.Search)
	}
	return labels
}

// filterBar renders a line showing active filters.
func (f PatentFilter) View(w int, theme render.Theme) string {
	labels := f.Labels()
	if len(labels) == 0 {
		return ""
	}
	s := " filters: " + strings.Join(labels, " · ")
	return theme.Dim.Render(render.Pad(s, w))
}

// parseFilter updates the filter from a command argument slice.
func (f *PatentFilter) parse(args []string) (msg string, err error) {
	if len(args) == 0 || args[0] == "clear" || args[0] == "none" || args[0] == "default" || args[0] == "reset" {
		*f = PatentFilter{}
		return "filters cleared", nil
	}

	switch args[0] {
	case "state", "status", "review_state", "s":
		if len(args) < 2 || args[1] == "clear" || args[1] == "none" {
			f.ReviewState = domain.ReviewStateNone
			return "review state filter cleared", nil
		}
		state, err := domain.ParseReviewState(args[1])
		if err != nil {
			return "", err
		}
		f.ReviewState = state
		return fmt.Sprintf("filtering by review state: %s", state), nil

	case "search", "find", "inventor", "inv":
		if len(args) < 2 || args[1] == "clear" || args[1] == "none" {
			f.Search = ""
			return "search filter cleared", nil
		}
		f.Search = strings.Join(args[1:], " ")
		return fmt.Sprintf("filtering by search: %s", f.Search), nil

	default:
		return "", fmt.Errorf("unknown filter type %q (try: state, search, inventor, clear)", args[0])
	}
}
