package tui

const (
	commandSearch             = "search"
	commandOpen               = "open"
	commandAdd                = "add"
	commandImport             = "import"
	commandRefresh            = "refresh"
	commandRefreshRefsDetails = "refresh-refs-details"
	commandCitedBy            = "citedby"
	commandClassification     = "cpc"
	commandFullText           = "full-text"
	commandRefs               = "refs"
	commandNotes              = "notes"
	commandDate               = "date"
	commandNum                = "num"
	commandSummarize          = "summarize"
	commandCompare            = "compare"
	commandRef                = "ref"
	commandReview             = "review"
	commandIgnored            = "ignored"
	commandUnderReview        = "under-review"
	commandHelp               = "help"
	commandHelpShort          = "h"
	commandBrowser            = "browser"
	commandWeb                = "web"
	commandProject            = "project"
	commandSort               = "sort"
	commandFilter             = "filter"
	commandClassFilter        = "classfilter"
	commandInventorFilter     = "inventorfilter"
	// commandStatusFilter removed — use :filter review_state instead
	commandFamily     = "family"
	commandPurge      = "purge"
	commandCompact    = "compact"
	commandNote       = "note"
	commandIDS        = "ids"
	commandVersion    = "version"
	commandKeymap     = "keymap"
	commandExit       = "exit"
	commandFamilyPull = "pull"
	commandTag        = "tag"

	// :country subcommands
	commandCountry = "country"
	countrySubList = "list"
	countrySubHelp = "help"

	// :tag subcommands
	tagSubAdd    = "add"
	tagSubList   = "list"
	tagSubDelete = "delete"
	tagSubRename = "rename"
	tagSubColor  = "color"
	tagSubFilter = "filter"
	tagSubHelp   = "help"

	// short tag aliases
	aliasTagAdd    = "ta" // :tag add
	aliasTagList   = "tl" // :tag list
	aliasTagDelete = "td" // :tag delete
	aliasTagRename = "tr" // :tag rename
	aliasTagColor  = "tc" // :tag color
	aliasTagFilter = "tf" // :tag filter
	aliasTagHelp   = "th" // :tag help

	aliasCountry = "co"

	// :project subcommands
	projectSubID            = "id"
	projectSubList          = "list"
	projectSubCreate        = "create"
	projectSubAdd           = "add"
	projectSubSwitch        = "switch"
	projectSubStatus        = "status"
	projectSubSummaryStatus = "summary-status"
	projectSubSummary       = "summary"
	projectSubComment       = "comment"
	projectSubDelete        = "delete"
	projectSubEvent         = "event"
	projectSubEvents        = "events"
	projectSubInvoice       = "invoice"
	projectSubInvoices      = "invoices"
	projectSubIDS           = "ids"
	projectSubExport        = "export"

	// :project ids subcommands
	idsSubAdd  = "add"
	idsSubMeta = "meta"

	// :ids edit subcommands
	idsEditSubNote     = "note"
	idsEditSubKind     = "kind"
	idsEditSubCountry  = "country"
	idsEditSubPassages = "passages"
	idsEditSubFull     = "full"

	// :project export subcommands
	exportSubIDS         = "ids"
	exportSubReviewState = "review_state"
	exportSubState       = "state"

	// :filter subcommands
	filterSubReviewState = "review_state"
	filterSubClass       = "class"
	filterSubInventor    = "inventor"
	filterSubClear       = "clear"

	// :family subcommands
	familySubParent = "parent"
	familySubChild  = "child"
	familySubRemove = "remove"
	familySubView   = "view"

	// activity actions
	activityPatentAdd           = "patent.add"
	activityPatentReviewState   = "patent.review_state"
	activityPatentImport        = "patent.import"
	activityCitationReviewState = "citation.review_state"
	activityCitationStore       = "citation.store"
	activityNoteAdd             = "note.add"
	activityRefAdd              = "ref.add"
	activityIDSAdd              = "ids.add"
	activityIDSStatus           = "ids.status"

	// export state aliases
	exportStateAll         = "all"
	exportStateNone        = "none"
	exportStateStored      = "stored"
	exportStateIgnored     = "ignored"
	exportStateUnderReview = "under-review"

	// invoice/event argument keywords
	argDate      = "date"
	argDue       = "due"
	argRef       = "ref"
	argNote      = "note"
	argCurrency  = "currency"
	argDirection = "direction"
	argFirm      = "firm"
	argStatus    = "status"

	refActionAdd    = "add"
	refActionExport = "export"

	refreshTargetAll       = "all"
	refreshTargetCitations = "citations"
	refreshTargetCitedBy   = "citedby"
	refreshArgDetails      = "details"

	importActionAdded     = "added"
	importActionImported  = "imported"
	importActionOpened    = "opened"
	importActionRefreshed = "refreshed"
)
