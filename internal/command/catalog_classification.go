package command

import "patentmine/internal/proto"

// classificationCommands returns the classificationCommands command registrations.
func classificationCommands() []Command {
	return []Command{
		Command{ID: ClassificationTaxonomyList, Name: "class.list", Aliases: []string{"classifications"}, Usage: ":class.list", Kind: KindEngine, Method: proto.MethodClassificationList},
		Command{ID: ClassificationLookup, Name: "class.lookup", Usage: ":class.lookup <code>", Kind: KindEngine, Method: proto.MethodClassificationLookup},
		Command{ID: OpenPatentClassifications, Name: "open.classifications", Aliases: []string{"class", "patent-classifications"}, Usage: ":open.classifications", Kind: KindView, Scopes: patentScopes},
	}
}
