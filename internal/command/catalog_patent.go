package command

import "patentmine/internal/proto"

// patentCommands returns the patentCommands command registrations.
func patentCommands() []Command {
	return []Command{
		Command{ID: PatentList, Kind: KindEngine, Method: proto.MethodPatentList},
		Command{ID: PatentGet, Kind: KindEngine, Method: proto.MethodPatentGet},
		Command{ID: PatentRelations, Kind: KindEngine, Method: proto.MethodRelations},
		Command{ID: PatentFamilyGraph, Kind: KindEngine, Method: proto.MethodFamilyGraph},
		Command{ID: ProjectList, Kind: KindEngine, Method: proto.MethodProjectList},
		Command{ID: ExportIDS, Kind: KindEngine, Method: proto.MethodIDSExport, Scopes: []Scope{ScopeProjects}},

		// --- patent review state (engine) ---
		Command{ID: MarkActive, Name: "review-state.active", Aliases: []string{"active", "review_state.active"}, Usage: ":review-state.active", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: MarkUnderReview, Name: "review-state.under-review", Aliases: []string{"under-review", "review_state.under_review"}, Usage: ":review-state.under-review", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: MarkIgnored, Name: "review-state.ignored", Aliases: []string{"ignored", "review_state.ignored"}, Usage: ":review-state.ignored", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: MarkDeleted, Name: "review-state.deleted", Aliases: []string{"deleted", "review_state.deleted"}, Usage: ":review-state.deleted", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: AddToProject, Name: "add", Usage: ":add [PATENT]", Kind: KindEngine, Method: proto.MethodMembershipAdd, Scopes: patentScopes},
		Command{ID: AddUSPTO, Name: "add.uspto", Usage: ":add.uspto [PATENT]", Kind: KindEngine, Method: proto.MethodMembershipAdd},
		Command{ID: AddGoogle, Name: "add.google", Usage: ":add.google [PATENT]", Kind: KindEngine, Method: proto.MethodMembershipAdd},
		Command{ID: AddRelated, Name: "add.related", Usage: ":add.related", Kind: KindEngine, Method: proto.MethodAddRelated, Scopes: patentScopes},
		Command{ID: FetchUSPTO, Name: "fetch.uspto", Usage: ":fetch.uspto", Kind: KindEngine, Method: proto.MethodUSPTOFetchXML, Scopes: patentScopes},
		Command{ID: FetchUSPTOPGPub, Name: "fetch.uspto.pgpub", Usage: ":fetch.uspto.pgpub", Kind: KindEngine, Method: proto.MethodUSPTOFetchXML, Scopes: patentScopes},
		Command{ID: FetchUSPTOGrant, Name: "fetch.uspto.grant", Usage: ":fetch.uspto.grant", Kind: KindEngine, Method: proto.MethodUSPTOFetchXML, Scopes: patentScopes},
		Command{ID: FetchUSPTOAssignments, Name: "fetch.uspto.assignments", Usage: ":fetch.uspto.assignments", Kind: KindEngine, Method: proto.MethodUSPTOFetchAssignments, Scopes: patentScopes},
		Command{ID: FetchAssignmentsProject, Name: "fetch.uspto.assignments.project", Usage: ":fetch.uspto.assignments.project [include-expired]", Kind: KindEngine, Method: proto.MethodUSPTOFetchAssignmentsBatch, Scopes: projectScopes},
		// TODO(assignee-commands): still to register once their backends land —
		//   fetch.uspto.assignments.file <path>  (batch from a :add.file list)
		//   find.assignee <name>                 (remote USPTO ODP discovery)
		//   export.assignees [path]              (rollup report, export.* family)
		// The per-patent timeline reuses OpenAssignees (view.assignees); enriching
		// that overlay to render assignee_history is still pending.
		Command{ID: PatentDelete, Name: "delete", Usage: ":delete", Kind: KindEngine, Method: proto.MethodPatentDelete, Scopes: patentScopes},
		Command{ID: ClearPatentCache, Name: "patent.clear-cache", Aliases: []string{"patent.clear_cache"}, Usage: ":patent.clear-cache [PATENT ...]", Kind: KindEngine, Method: proto.MethodPatentClearCache, Scopes: patentScopes},
	}
}
