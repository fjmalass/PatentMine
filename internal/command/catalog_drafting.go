package command

// draftingCommands returns the drafting / office-action command registrations.
// Canonical typed names use resource.action.
func draftingCommands() []Command {
	return []Command{
		Command{ID: AddOfficeAction, Name: "officeaction.add", Usage: ":officeaction.add [path]", Kind: KindView, Scopes: projectScopes},
		Command{ID: ListOfficeActions, Name: "officeaction.list", Aliases: []string{"officeactions"}, Usage: ":officeaction.list", Kind: KindView, Scopes: projectScopes},
		Command{ID: AssignOfficeAction, Name: "officeaction.assign", Kind: KindView, Scopes: patentScopes, Usage: ":officeaction.assign [name]"},
		Command{ID: ReleaseOfficeAction, Name: "officeaction.release", Kind: KindView, Scopes: patentScopes, Usage: ":officeaction.release [name]"},
		Command{ID: AddDocument, Name: "matter.document.add", Usage: ":matter.document.add [path]", Kind: KindView, Scopes: projectScopes},
		Command{ID: OpenDocuments, Name: "matter.document.open", Aliases: []string{"documents"}, Usage: ":matter.document.open", Kind: KindView, Scopes: projectScopes},
		Command{ID: SetMatterType, Name: "matter.type.set", Aliases: []string{"project.matter-type"}, Usage: ":matter.type.set <provisional|nonprovisional|in_prosecution|issued>", Kind: KindView, Scopes: projectScopes},
		Command{ID: DraftResponse, Name: "officeaction.respond", Usage: ":officeaction.respond", Kind: KindView, Scopes: projectScopes},
		Command{ID: LogComm, Name: "matter.comm.log", Usage: ":matter.comm.log", Kind: KindView, Scopes: projectScopes},
		Command{ID: OpenComms, Name: "matter.comm.open", Aliases: []string{"communications"}, Usage: ":matter.comm.open", Kind: KindView, Scopes: projectScopes},
		Command{ID: FlagConflict, Name: "conflict.flag", Aliases: []string{"conflict"}, Usage: ":conflict.flag <patent-number> [reason]", Kind: KindView, Scopes: projectScopes},
		Command{ID: ListConflicts, Name: "conflict.list", Aliases: []string{"conflicts"}, Usage: ":conflict.list", Kind: KindView, Scopes: projectScopes},
		Command{ID: LogTime, Name: "time.log", Usage: ":time.log <reading|writing|ai|call|admin> <duration> [note]", Kind: KindView, Scopes: projectScopes},
		Command{ID: ValidateTime, Name: "time.validate", Usage: ":time.validate", Kind: KindView, Scopes: projectScopes},
		Command{ID: ShowTime, Name: "time.show", Aliases: []string{"worklog"}, Usage: ":time.show", Kind: KindView, Scopes: projectScopes},
	}
}
