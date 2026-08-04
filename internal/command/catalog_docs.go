package command

func docsCommands() []Command {
	return []Command{
		Command{ID: Docs, Name: "docs", Usage: ":docs", Kind: KindView},
		Command{ID: DocsOpen, Name: "docs.open", Usage: ":docs.open", Kind: KindView},
		Command{ID: DocsShow, Name: "docs.show", Usage: ":docs.show <doc-id> [raw|preview]", Kind: KindView},
	}
}
