package tui

type modeSpec struct {
	title      string
	accent     string
	isOverlay  bool
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
		title:  "Patent List",
		accent: "39",
	},
	viewDetail: {
		title:  "Detail",
		accent: "51",
	},
	viewCites: {
		title:  "Citations",
		accent: "214",
	},
	viewCitedBy: {
		title:  "Cited By",
		accent: "40",
	},
	viewClassifications: {
		title:     "Classifications",
		accent:    "170",
		isOverlay: true,
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewText: {
		title:  "Full Text",
		accent: "250",
	},
	viewNotes: {
		title:  "Notes",
		accent: "220",
	},
	viewRefs: {
		title:  "References",
		accent: "27",
	},
	viewAI: {
		title:  "AI",
		accent: "141",
	},
	viewHelp: {
		title:  "Help",
		accent: "245",
	},
	viewHelpPopup: {
		title:     "Help",
		accent:    "245",
		isOverlay: true,
		background: func(m *Model) viewMode {
			return previousModeOr(m, viewDetail)
		},
	},
	viewPreview: {
		title:     "Reference Preview",
		accent:    "81",
		isOverlay: true,
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewReview: {
		title:  "Review Queue",
		accent: "202",
	},
	viewConfirmDelete: {
		title:     "Confirm Delete",
		accent:    "196",
		isOverlay: true,
		background: func(*Model) viewMode {
			return viewList
		},
	},
	viewClassificationDetail: {
		title:     "Classification Detail",
		accent:    "170",
		isOverlay: true,
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewInventors: {
		title:     "Inventors",
		accent:    "51",
		isOverlay: true,
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewFamily: {
		title:     "Patent Family",
		accent:    "213",
		isOverlay: true,
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewSplash: {
		title:  "Splash",
		accent: "39",
	},
	viewProjectEvents: {
		title:     "Project Events",
		accent:    "39",
		isOverlay: true,
		background: func(*Model) viewMode {
			return viewSplash
		},
	},
	viewProjectInvoices: {
		title:     "Project Invoices",
		accent:    "39",
		isOverlay: true,
		background: func(*Model) viewMode {
			return viewSplash
		},
	},
	viewProjectIDS: {
		title:     "IDS",
		accent:    "75",
		isOverlay: true,
		background: func(*Model) viewMode {
			return viewSplash
		},
	},
	viewProjectInfo: {
		title:     "Project Info",
		accent:    "39",
		isOverlay: true,
		background: func(m *Model) viewMode {
			return previousModeOr(m, viewList)
		},
	},
	viewNoteEdit: {
		title:     "Note",
		accent:    "220",
		isOverlay: true,
		background: func(m *Model) viewMode {
			return previousModeOr(m, viewDetail)
		},
	},
	viewIDSEdit: {
		title:     "IDS Entry",
		accent:    "75",
		isOverlay: true,
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewAbstract: {
		title:     "Abstract",
		accent:    "51",
		isOverlay: true,
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewClaim: {
		title:     "Claim 1",
		accent:    "51",
		isOverlay: true,
		background: func(*Model) viewMode {
			return viewDetail
		},
	},
	viewUSPTOKeyWarning: {
		title:     "USPTO API Key Missing",
		accent:    "39",
		isOverlay: true,
		background: func(*Model) viewMode {
			return viewSplash
		},
	},
	viewBulkConfirm: {
		title:     "Bulk Action Confirmation",
		accent:    "202",
		isOverlay: true,
		background: func(m *Model) viewMode {
			return previousModeOr(m, viewReview)
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
