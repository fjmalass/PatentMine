package command

// draftingCommands returns the drafting / office-action command registrations.
// Names follow the <verb>.<object> convention (add.officeaction, …), matching
// the add.file / open.notes / export.ids.pdf family.
func draftingCommands() []Command {
	return []Command{
		Command{ID: AddOfficeAction, Name: "add.officeaction", Aliases: []string{"add-officeaction"}, Usage: ":add.officeaction [path]", Kind: KindView, Scopes: projectScopes},
		Command{ID: OpenOfficeAction, Name: "open.officeaction", Aliases: []string{"open-officeaction", "officeactions"}, Usage: ":open.officeaction", Kind: KindView, Scopes: projectScopes},
		Command{ID: AddDocument, Name: "add.document", Aliases: []string{"add-document"}, Usage: ":add.document [path]", Kind: KindView, Scopes: projectScopes},
		Command{ID: OpenDocuments, Name: "open.documents", Aliases: []string{"open-documents", "documents"}, Usage: ":open.documents", Kind: KindView, Scopes: projectScopes},
		Command{ID: SetMatterType, Name: "set.matter", Aliases: []string{"set-matter"}, Usage: ":set.matter <provisional|nonprovisional|in_prosecution|issued>", Kind: KindView, Scopes: projectScopes},
		Command{ID: DraftResponse, Name: "draft.response", Aliases: []string{"draft-response", "respond"}, Usage: ":draft.response", Kind: KindView, Scopes: projectScopes},
		Command{ID: LogComm, Name: "log.comm", Aliases: []string{"log-comm", "log.communication"}, Usage: ":log.comm", Kind: KindView, Scopes: projectScopes},
		Command{ID: OpenComms, Name: "open.comms", Aliases: []string{"open-comms", "communications"}, Usage: ":open.comms", Kind: KindView, Scopes: projectScopes},
		Command{ID: LogTime, Name: "log.time", Aliases: []string{"log-time"}, Usage: ":log.time <reading|writing|ai|call|admin> <duration> [note]", Kind: KindView, Scopes: projectScopes},
		Command{ID: ValidateTime, Name: "validate.time", Aliases: []string{"validate-time"}, Usage: ":validate.time", Kind: KindView, Scopes: projectScopes},
		Command{ID: ShowTime, Name: "show.time", Aliases: []string{"show-time"}, Usage: ":show.time", Kind: KindView, Scopes: projectScopes},
		Command{ID: ShowDeadlines, Name: "show.deadlines", Aliases: []string{"deadlines", "show-deadlines"}, Usage: ":show.deadlines", Kind: KindView, Scopes: projectScopes},
		Command{ID: TrackRenewals, Name: "track.renewals", Aliases: []string{"track-renewals", "renewals"}, Usage: ":track.renewals <patent-number>", Kind: KindView, Scopes: projectScopes},
	}
}
