package pane

import "patentmine/internal/domain"

func patentRowActivity(scope, title string, row domain.PatentRow, project domain.ProjectID, filter PatentFilter) ActivityFocus {
	metadata := map[string]any{
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
		metadata["publication_date"] = row.PublicationDate.Format("2006-01-02")
	}
	if project != "" {
		metadata["project"] = project
	}
	return ActivityFocus{Entity: "patent", EntityID: row.Number.String(), Label: title, Metadata: metadata}
}

func activeProjectID(project *domain.Project) domain.ProjectID {
	if project == nil {
		return ""
	}
	return project.ID
}
