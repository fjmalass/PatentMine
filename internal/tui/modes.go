package tui

type modeSpec struct {
	title      string
	themeColor string
	isOverlay  bool
	helpHint   string
}

var allViewModes = []viewMode{
	viewList,
	viewDetail,
	viewCites,
	viewCitedBy,
	viewClassifications,
	viewText,
	viewNotes,
	viewRefs,
	viewAI,
	viewHelp,
	viewHelpPopup,
	viewPreview,
	viewReview,
	viewConfirmDelete,
	viewClassificationDetail,
	viewInventors,
	viewFamily,
	viewSplash,
	viewProjectEvents,
	viewProjectInvoices,
	viewProjectIDS,
	viewProjectInfo,
	viewNoteEdit,
	viewIDSEdit,
	viewAbstract,
	viewClaim,
	viewUSPTOKeyWarning,
	viewBulkConfirm,
	viewStatusSelect,
	viewProjectTags,
	viewTagSelect,
}

var modeSpecs = map[viewMode]modeSpec{
	viewList: {
		title:      "Patent List",
		themeColor: ColorThemeList,
	},
	viewDetail: {
		title:      "Detail",
		themeColor: ColorThemeDetail,
	},
	viewCites: {
		title:      "Citations",
		themeColor: ColorThemeCitations,
		isOverlay:  true,
		helpHint:   "j/k: move · enter/y: save · i: ignore · r: review · /: search · esc: back",
	},
	viewCitedBy: {
		title:      "Cited By",
		themeColor: ColorThemeCitedBy,
		isOverlay:  true,
		helpHint:   "j/k: move · enter/y: save · i: ignore · r: review · /: search · esc: back",
	},
	viewClassifications: {
		title:      "Classifications",
		themeColor: ColorThemeClassifications,
		isOverlay:  true,
		helpHint:   "j/k: move · enter: filter · /: search · esc: back",
	},
	viewText: {
		title:      "Full Text",
		themeColor: ColorThemeText,
		isOverlay:  true,
		helpHint:   "j/k: scroll · esc: back",
	},
	viewNotes: {
		title:      "Notes",
		themeColor: ColorThemeNotes,
		isOverlay:  true,
		helpHint:   "N: add note · esc: back",
	},
	viewRefs: {
		title:      "References",
		themeColor: ColorThemeReferences,
		isOverlay:  true,
		helpHint:   "esc: back",
	},
	viewAI: {
		title:      "AI Analysis",
		themeColor: ColorThemeAI,
		isOverlay:  true,
		helpHint:   "esc: back",
	},
	viewHelp: {
		title:      "Help",
		themeColor: ColorThemeHelp,
	},
	viewHelpPopup: {
		title:      "Help",
		themeColor: ColorThemeHelp,
		isOverlay:  true,
		helpHint:   "j/k: scroll · /: search · esc: back",
	},
	viewPreview: {
		title:      "Reference Preview",
		themeColor: ColorThemePreview,
		isOverlay:  true,
		helpHint:   "y: save · i: ignore · r: review · n: skip · esc: back",
	},
	viewReview: {
		title:      "Review Queue",
		themeColor: ColorThemeReview,
	},
	viewConfirmDelete: {
		title:      "Confirm Delete",
		themeColor: ColorThemeDelete,
		isOverlay:  true,
		helpHint:   "y: confirm · n: cancel",
	},
	viewClassificationDetail: {
		title:      "Classification Detail",
		themeColor: ColorThemeClassifications,
		isOverlay:  true,
		helpHint:   "esc: back",
	},
	viewInventors: {
		title:      "Inventors",
		themeColor: ColorThemeDetail,
		isOverlay:  true,
		helpHint:   "j/k: move · enter: filter · esc: back",
	},
	viewFamily: {
		title:      "Patent Family",
		themeColor: ColorThemeFamily,
		isOverlay:  true,
		helpHint:   "j/k: move · enter: open · ctrl+r: refresh selected · esc: back",
	},
	viewSplash: {
		title:      "Splash",
		themeColor: ColorThemeList,
	},
	viewProjectEvents: {
		title:      "Project Events",
		themeColor: ColorThemeList,
		isOverlay:  true,
		helpHint:   "j/k: move · D: delete · esc: back",
	},
	viewProjectInvoices: {
		title:      "Project Invoices",
		themeColor: ColorThemeList,
		isOverlay:  true,
		helpHint:   "j/k: move · s: mark paid · D: delete · esc: back",
	},
	viewProjectIDS: {
		title:      "IDS",
		themeColor: ColorThemeIDS,
		isOverlay:  true,
		helpHint:   "j/k: move · s: cycle status · D: remove · esc: back",
	},
	viewProjectInfo: {
		title:      "Project Info",
		themeColor: ColorThemeList,
		isOverlay:  true,
		helpHint:   "s/m/c/S: edit fields · esc: back",
	},
	viewNoteEdit: {
		title:      "Note",
		themeColor: ColorThemeNotes,
		isOverlay:  true,
		helpHint:   "ctrl+s: save · esc: cancel",
	},
	viewIDSEdit: {
		title:      "IDS Entry",
		themeColor: ColorThemeIDS,
		isOverlay:  true,
		helpHint:   "s=status · n=note · k=kind · c=country · p=passages · f=in-full · D=remove · esc=back",
	},
	viewDateEdit: {
		title:      "Edit Date",
		themeColor: ColorThemeDetail,
		isOverlay:  true,
		helpHint:   "enter: save · esc: cancel",
	},
	viewAbstract: {
		title:      "Abstract",
		themeColor: ColorThemeDetail,
		isOverlay:  true,
		helpHint:   "j/k: scroll · esc: back",
	},
	viewClaim: {
		title:      "Claim 1",
		themeColor: ColorThemeDetail,
		isOverlay:  true,
		helpHint:   "j/k: scroll · esc: back",
	},
	viewUSPTOKeyWarning: {
		title:      "USPTO API Key Missing",
		themeColor: ColorThemeList,
		isOverlay:  true,
		helpHint:   "any key: continue · ctrl+c: quit",
	},
	viewBulkConfirm: {
		title:      "Bulk Action Confirmation",
		themeColor: ColorThemeReview,
		isOverlay:  true,
		helpHint:   "y: confirm · n: cancel",
	},
	viewStatusSelect: {
		title:      "Select Status",
		themeColor: ColorThemeList,
		isOverlay:  true,
		helpHint:   "j/k: move · enter: select · esc: back",
	},
	viewProjectTags: {
		title:      "Project Tags",
		themeColor: ColorThemeTags,
		isOverlay:  true,
		helpHint:   "j/k: move · D: delete · r: rename · esc: back",
	},
	viewTagSelect: {
		title:      "Select Tags",
		themeColor: ColorThemeTags,
		isOverlay:  true,
		helpHint:   "j/k: move · space: toggle · esc: back",
	},
}

func lookupModeSpec(mode viewMode) (modeSpec, bool) {
	spec, ok := modeSpecs[mode]
	return spec, ok
}

func mustModeSpec(mode viewMode) modeSpec {
	spec, ok := lookupModeSpec(mode)
	if !ok {
		panic("unregistered view mode: " + string(mode))
	}
	return spec
}

func previousModeOr(m *Model, fallback viewMode) viewMode {
	if len(m.backStack) == 0 {
		return fallback
	}
	mode := m.backStack[len(m.backStack)-1].mode
	if mode == viewHelpPopup || mode == viewNoteEdit || mode == viewBulkConfirm {
		return fallback
	}
	return mode
}
