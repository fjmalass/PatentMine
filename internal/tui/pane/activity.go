package pane

import "patentmine/internal/domain"

func patentRowActivity(scope, title string, row domain.PatentRow, project domain.ProjectID, filter PatentFilter) ActivityFocus {
	attrs := map[string]any{
		"scope":           scope,
		"display_number":  row.DisplayNumber.String(),
		"title":           row.Title,
		"review_state":    row.ReviewState,
		"tags":            row.Tags,
		"classifications": row.Classifications,
		"filter":           filter.Expression,
		"search":           filter.Search,
		"inventors_short":  formatInventorsShort(row.Inventors),
	}
	if !row.PublicationDate.IsZero() {
		attrs["publication_date"] = row.PublicationDate.Format(domain.DateLayout)
	}
	if project != "" {
		attrs["project"] = project
	}
	return ActivityFocus{Entity: "patent", EntityID: row.Number.String(), Label: title, Attributes: attrs}
}

func activeProjectID(project *domain.Project) domain.ProjectID {
	if project == nil {
		return ""
	}
	return project.ID
}
