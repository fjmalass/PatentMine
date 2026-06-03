// Crawl, source-mode, import, relations and family-graph payloads.
package proto

import "patentmine/internal/domain"

// CrawlFamilyParams starts a family-graph crawl rooted at one patent. Depth 0
// fetches only the root; a negative depth uses the configured family depth.
// Force bypasses the local file cache and re-fetches from the web.
type CrawlFamilyParams struct {
	Project domain.ProjectID    `json:"project,omitempty"`
	Root    domain.PatentNumber `json:"root"`
	Depth   int                 `json:"depth"`
	Profile domain.CrawlProfile `json:"profile,omitempty"`
	Force   bool                `json:"force,omitempty"`
}

// CrawlStartResult returns the id of an enqueued job.
type CrawlStartResult struct {
	JobID string `json:"job_id"`
}

// CrawlConfigResult reports daemon-owned crawl defaults.
type CrawlConfigResult struct {
	MaxDepth int `json:"max_depth"`
}

// SourceModeParams changes normal provider behavior: compare, fallback, or off.
type SourceModeParams struct {
	Mode string `json:"mode"`
}

// SourceModeResult reports normal provider behavior.
type SourceModeResult struct {
	Mode string `json:"mode"`
}

// ImportFileParams loads a patent record from a local fixture file.
type ImportFileParams struct {
	Path string `json:"path"`
}

// CrawlCancelParams cancels a running job.
type CrawlCancelParams struct {
	JobID string `json:"job_id"`
}

// RelationsParams selects family-graph edges of one kind from one patent.
type RelationsParams struct {
	Number         domain.PatentNumber `json:"number"`
	Kind           domain.RelationKind `json:"kind"`
	Project        domain.ProjectID    `json:"project,omitempty"`
	Filter         string              `json:"filter,omitempty"`
	ReviewState    domain.ReviewState  `json:"review_state,omitempty"`
	IDSStatus      string              `json:"ids_status,omitempty"`
	Search         string              `json:"search,omitempty"`
	SearchScope    string              `json:"search_scope,omitempty"`
	Classification string              `json:"classification,omitempty"`
	Inventor       string              `json:"inventor,omitempty"`
	Limit          int                 `json:"limit,omitempty"`
	Offset         int                 `json:"offset,omitempty"`
	SortColumn     domain.SortColumn   `json:"sort_column,omitempty"`
	SortAscending  bool                `json:"sort_ascending,omitempty"`
}

// RelationsResult carries the requested family-graph edges.
type RelationsResult struct {
	Patents []domain.PatentRow `json:"patents"`
	Total   int                `json:"total"`
}

// FamilyGraphParams selects a bounded family-only DAG around one patent.
type FamilyGraphParams struct {
	Root      domain.PatentNumber `json:"root"`
	Depth     int                 `json:"depth,omitempty"`
	MaxNodes  int                 `json:"max_nodes,omitempty"`
	Project   domain.ProjectID    `json:"project,omitempty"`
	Countries []string            `json:"countries,omitempty"`
}

// FamilyGraphNode is one patent in a family DAG result.
type FamilyGraphNode struct {
	Patent   domain.PatentRow      `json:"patent"`
	Depth    int                   `json:"depth"`
	Parents  []domain.PatentNumber `json:"parents,omitempty"`
	Children []domain.PatentNumber `json:"children,omitempty"`
}

// FamilyGraphEdge is one canonical parent->child edge in a family DAG.
type FamilyGraphEdge struct {
	Parent       domain.PatentNumber `json:"parent"`
	Child        domain.PatentNumber `json:"child"`
	Inconsistent bool                `json:"inconsistent,omitempty"`
}

// FamilyGraphResult carries a bounded parent/child graph plus traversal attrs.
type FamilyGraphResult struct {
	Root               domain.PatentNumber `json:"root"`
	Depth              int                 `json:"depth"`
	MaxNodes           int                 `json:"max_nodes"`
	Countries          []string            `json:"countries,omitempty"`
	Nodes              []FamilyGraphNode   `json:"nodes"`
	Edges              []FamilyGraphEdge   `json:"edges"`
	Truncated          bool                `json:"truncated,omitempty"`
	HiddenByCountry    int                 `json:"hidden_by_country,omitempty"`
	InconsistencyCount int                 `json:"inconsistency_count,omitempty"`
}

// FamilyGraphExportParams carries options for exporting the family graph.
type FamilyGraphExportParams struct {
	Root      domain.PatentNumber `json:"root"`
	Depth     int                 `json:"depth,omitempty"`
	MaxNodes  int                 `json:"max_nodes,omitempty"`
	Project   domain.ProjectID    `json:"project,omitempty"`
	Countries []string            `json:"countries,omitempty"`
	Path      string              `json:"path,omitempty"`
}

// FamilyGraphExportResult describes the output of exporting the family graph.
type FamilyGraphExportResult struct {
	Path      string `json:"path,omitempty"`
	Mermaid   string `json:"mermaid,omitempty"`
	ASCIIPath string `json:"ascii_path,omitempty"`
}
