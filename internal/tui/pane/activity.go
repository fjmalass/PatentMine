package pane

import (
	"patentmine/internal/domain"
	"patentmine/internal/observability"
)

func patentRowActivity(scope, title string, row domain.PatentRow, project domain.ProjectID, filter PatentFilter) ActivityFocus {
	attrs := map[string]any{
		observability.AttrScope:          scope,
		observability.AttrDisplayNumber:  row.DisplayNumber.String(),
		observability.AttrTitle:          row.Title,
		"review_state":                   row.ReviewState,
		"tags":                           row.Tags,
		"classifications":                row.Classifications,
		observability.AttrFilter:         filter.Expression,
		observability.AttrSearch:         filter.Search,
		observability.AttrInventorsShort: formatInventorsShort(row.Inventors),
	}
	if !row.PublicationDate.IsZero() {
		attrs[observability.AttrPublicationDate] = row.PublicationDate.Format(domain.DateLayout)
	}
	if project != "" {
		attrs[observability.AttrProject] = project
	}
	return ActivityFocus{Entity: observability.EntityPatent, EntityID: row.Number.String(), Label: title, Attributes: attrs}
}

func activeProjectID(project *domain.Project) domain.ProjectID {
	if project == nil {
		return ""
	}
	return project.ID
}
