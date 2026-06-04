package command

// draftingCommands returns the drafting / office-action command registrations.
// Names follow the <verb>.<object> convention (add.officeaction, …), matching
// the add.file / open.notes / export.ids.pdf family.
func draftingCommands() []Command {
	return []Command{
		Command{ID: AddOfficeAction, Name: "add.officeaction", Aliases: []string{"add-officeaction"}, Usage: ":add.officeaction [path]", Kind: KindView, Scopes: projectScopes},
		Command{ID: OpenOfficeAction, Name: "open.officeaction", Aliases: []string{"open-officeaction", "officeactions"}, Usage: ":open.officeaction", Kind: KindView, Scopes: projectScopes},
	}
}
