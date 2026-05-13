package tui

type modeSpec struct {
	title      string
	themeColor string
	isOverlay  bool
	helpHint   string
	background func(*Model) viewMode
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
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewCitedBy: {
		title:      "Cited By",
		themeColor: ColorThemeCitedBy,
		isOverlay:  true,
		helpHint:   "j/k: move · enter/y: save · i: ignore · r: review · /: search · esc: back",
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewClassifications: {
		title:      "Classifications",
		themeColor: ColorThemeClassifications,
		isOverlay:  true,
		helpHint:   "j/k: move · enter: filter · /: search · esc: back",
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewText: {
		title:      "Full Text",
		themeColor: ColorThemeText,
		isOverlay:  true,
		helpHint:   "j/k: scroll · esc: back",
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewNotes: {
		title:      "Notes",
		themeColor: ColorThemeNotes,
		isOverlay:  true,
		helpHint:   "N: add note · esc: back",
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewRefs: {
		title:      "References",
		themeColor: ColorThemeReferences,
		isOverlay:  true,
		helpHint:   "esc: back",
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewAI: {
		title:      "AI Analysis",
		themeColor: ColorThemeAI,
		isOverlay:  true,
		helpHint:   "esc: back",
		background: func(*Model) viewMode {
			return viewDetail
		},
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
		background: func(m *Model) viewMode {
			return previousModeOr(m, viewDetail)
		},
	},
	viewPreview: {
		title:      "Reference Preview",
		themeColor: ColorThemePreview,
		isOverlay:  true,
		helpHint:   "y: save · i: ignore · r: review · n: skip · esc: back",
		background: func(*Model) viewMode {
			return viewDetail
		},
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
		background: func(*Model) viewMode {
			return viewList
		},
	},
	viewClassificationDetail: {
		title:      "Classification Detail",
		themeColor: ColorThemeClassifications,
		isOverlay:  true,
		helpHint:   "esc: back",
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewInventors: {
		title:      "Inventors",
		themeColor: ColorThemeDetail,
		isOverlay:  true,
		helpHint:   "j/k: move · enter: filter · esc: back",
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewFamily: {
		title:      "Patent Family",
		themeColor: ColorThemeFamily,
		isOverlay:  true,
		helpHint:   "j/k: move · h: parent · l: child · enter: open · esc: back",
		background: func(*Model) viewMode {
			return viewDetail
		},
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
		background: func(*Model) viewMode {
			return viewSplash
		},
	},
	viewProjectInvoices: {
		title:      "Project Invoices",
		themeColor: ColorThemeList,
		isOverlay:  true,
		helpHint:   "j/k: move · s: mark paid · D: delete · esc: back",
		background: func(*Model) viewMode {
			return viewSplash
		},
	},
	viewProjectIDS: {
		title:      "IDS",
		themeColor: ColorThemeIDS,
		isOverlay:  true,
		helpHint:   "j/k: move · s: cycle status · D: remove · esc: back",
		background: func(*Model) viewMode {
			return viewSplash
		},
	},
	viewProjectInfo: {
		title:      "Project Info",
		themeColor: ColorThemeList,
		isOverlay:  true,
		helpHint:   "s/m/c/S: edit fields · esc: back",
		background: func(m *Model) viewMode {
			return previousModeOr(m, viewList)
		},
	},
	viewNoteEdit: {
		title:      "Note",
		themeColor: ColorThemeNotes,
		isOverlay:  true,
		helpHint:   "ctrl+s: save · esc: cancel",
		background: func(m *Model) viewMode {
			return previousModeOr(m, viewDetail)
		},
	},
	viewIDSEdit: {
		title:      "IDS Entry",
		themeColor: ColorThemeIDS,
		isOverlay:  true,
		helpHint:   "s/n/k/c/p/f: edit · D: remove · esc: back",
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewDateEdit: {
		title:      "Edit Date",
		themeColor: ColorThemeDetail,
		isOverlay:  true,
		helpHint:   "enter: save · esc: cancel",
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewAbstract: {
		title:      "Abstract",
		themeColor: ColorThemeDetail,
		isOverlay:  true,
		helpHint:   "j/k: scroll · esc: back",
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewClaim: {
		title:      "Claim 1",
		themeColor: ColorThemeDetail,
		isOverlay:  true,
		helpHint:   "j/k: scroll · esc: back",
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewUSPTOKeyWarning: {
		title:      "USPTO API Key Missing",
		themeColor: ColorThemeList,
		isOverlay:  true,
		helpHint:   "any key: continue · ctrl+c: quit",
		background: func(*Model) viewMode {
			return viewSplash
		},
	},
	viewBulkConfirm: {
		title:      "Bulk Action Confirmation",
		themeColor: ColorThemeReview,
		isOverlay:  true,
		helpHint:   "y: confirm · n: cancel",
		background: func(m *Model) viewMode {
			return previousModeOr(m, viewReview)
		},
	},
	viewStatusSelect: {
		title:      "Select Status",
		themeColor: ColorThemeList,
		isOverlay:  true,
		helpHint:   "j/k: move · enter: select · esc: back",
		background: func(m *Model) viewMode {
			return previousModeOr(m, viewList)
		},
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
