package proto

// DocSummary is one markdown document exposed by the daemon docs browser.
type DocSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// DocsListResult carries the sorted list of available project docs.
type DocsListResult struct {
	Docs []DocSummary `json:"docs"`
}

// DocsGetParams identifies a project doc to read.
type DocsGetParams struct {
	ID   string `json:"id"`
	Mode string `json:"mode,omitempty"`
}

// DocsGetResult carries a markdown document's content.
type DocsGetResult struct {
	Doc     DocSummary `json:"doc"`
	Content string     `json:"content"`
	Mode    string     `json:"mode"`
}
