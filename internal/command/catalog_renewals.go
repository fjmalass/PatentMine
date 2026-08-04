package command

// renewalCommands returns deadline and patent-renewal command registrations.
// Keep these separate from drafting/prosecution commands because renewals are a
// cross-matter docketing domain, not an office-action drafting feature.
func renewalCommands() []Command {
	return []Command{
		Command{ID: ShowDeadlines, Name: "deadline.show", Aliases: []string{"deadlines"}, Usage: ":deadline.show", Kind: KindView, Scopes: projectScopes},
		Command{ID: TrackRenewals, Name: "renewal.track", Aliases: []string{"renewals", "deadline.track"}, Usage: ":renewal.track <patent-number> [large|small|micro]", Kind: KindView, Scopes: projectScopes},
		Command{ID: UntrackRenewals, Name: "renewal.untrack", Aliases: []string{"deadline.untrack"}, Usage: ":renewal.untrack <patent-number>", Kind: KindView, Scopes: projectScopes},
		Command{ID: FetchRenewalValidations, Name: "renewal.validation.fetch", Usage: ":renewal.validation.fetch <ep-patent-number>", Kind: KindView, Scopes: patentScopes},
		Command{ID: ListRenewalValidations, Name: "renewal.validation.list", Aliases: []string{"validations"}, Usage: ":renewal.validation.list <patent-number>", Kind: KindView, Scopes: patentScopes},
		Command{ID: SetRenewalValidation, Name: "renewal.validation.set", Aliases: []string{"validation.set"}, Usage: ":renewal.validation.set <patent-number> <country> <potential|validated|lapsed|unknown>", Kind: KindView, Scopes: patentScopes},
	}
}
