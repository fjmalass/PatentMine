package command

import "patentmine/internal/proto"

// tagCommands returns the tagCommands command registrations.
func tagCommands() []Command {
	return []Command{
		Command{ID: Tag, Name: "tag", Kind: KindEngine, Method: proto.MethodTagPatent, Scopes: patentScopes, Usage: ":tag <name>"},
		Command{ID: Untag, Name: "untag", Kind: KindEngine, Method: proto.MethodUntagPatent, Scopes: patentScopes, Usage: ":untag <name>"},
		Command{ID: TagTaxonomyAdd, Name: "tag.create", Aliases: []string{"tag.add"}, Usage: ":tag.create <name>", Kind: KindEngine, Method: proto.MethodTagCreate, Scopes: projectScopes},
		Command{ID: TagTaxonomyList, Name: "tag.list", Usage: ":tag.list", Kind: KindEngine, Method: proto.MethodTagList, Scopes: projectScopes},
		Command{ID: TagTaxonomyDelete, Name: "tag.delete", Usage: ":tag.delete <name>", Kind: KindEngine, Method: proto.MethodTagDelete, Scopes: projectScopes},
		Command{ID: TagStrict, Name: "tag.patent.add", Aliases: []string{"patent-tag"}, Usage: ":tag.patent.add <name>", Kind: KindEngine, Method: proto.MethodTagPatentStrict, Scopes: patentScopes},
		Command{ID: UntagStrict, Name: "tag.patent.delete", Aliases: []string{"patent-untag"}, Usage: ":tag.patent.delete <name>", Kind: KindEngine, Method: proto.MethodUntagPatentStrict, Scopes: patentScopes},
		Command{ID: TagPatentList, Name: "tag.patent.list", Aliases: []string{"patent-tags"}, Usage: ":tag.patent.list", Kind: KindEngine, Method: proto.MethodPatentTagList, Scopes: patentScopes},
		Command{ID: TagPatentManage, Name: "tag.patent", Aliases: []string{"tag-manage"}, Usage: ":tag.patent", Kind: KindEngine, Method: proto.MethodPatentTagList, Scopes: patentScopes},
	}
}
