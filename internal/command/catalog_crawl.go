package command

import "patentmine/internal/proto"

// crawlCommands returns the crawlCommands command registrations.
func crawlCommands() []Command {
	return []Command{
		Command{ID: CrawlFamily, Name: "crawl.family", Aliases: []string{"family"}, Usage: ":crawl.family", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: CrawlDepthMax, Name: "crawl.depth.max", Aliases: []string{"family.depth.max", "crawl-depth-max"}, Usage: ":crawl.depth.max", Kind: KindView},
		Command{ID: CrawlCitations, Name: "crawl.citations", Aliases: []string{"crawl-citations"}, Usage: ":crawl.citations", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: CrawlCitedBy, Name: "crawl.citedby", Aliases: []string{"crawl-citedby"}, Usage: ":crawl.citedby", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: CrawlAll, Name: "crawl.all", Aliases: []string{"crawl", "recursion"}, Usage: ":crawl.all", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: LookupPatent, Name: "lookup", Aliases: []string{"lookup-patent"}, Usage: ":lookup", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: Import, Name: "import", Aliases: []string{"import-patent"}, Usage: ":import <number|file> [force]", Kind: KindEngine, Method: proto.MethodCrawlFamily},
		Command{ID: AddFile, Name: "add.file", Aliases: []string{"add-file", "load"}, Usage: ":add.file [path]", Kind: KindView, Scopes: projectScopes},
		Command{ID: ExportAdded, Name: "export.added", Aliases: []string{"export-added", "add.export"}, Usage: ":export.added [path]", Kind: KindView, Scopes: projectScopes},
		Command{ID: SourceMode, Name: "source.mode", Aliases: []string{"source-mode"}, Usage: ":source.mode <compare|uspto-first|uspto-only|google-only>", Kind: KindView},
		Command{ID: SourceCompare, Name: "source.compare", Aliases: []string{"compare", "compare-sources"}, Usage: ":source.compare", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: SourceBibs, Name: "source.bibs", Aliases: []string{"sources", "both"}, Usage: ":source.bibs", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: CrawlCancel, Kind: KindEngine, Method: proto.MethodCrawlCancel},
	}
}
