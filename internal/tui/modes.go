package tui

import "strings"

type modeSpec struct {
	title      string
	themeColor string
	isOverlay  bool
	onActivate func(*Model)
	subtitle   func(*Model) string
}

var allViewModes = []viewMode{
	viewList,
	viewDetail,
	viewCites,
	viewCitedBy,
	viewClassifications,
	viewFullText,
	viewNotes,
	viewRefs,
	viewAI,
	viewHelp,
	viewHelpPopup,
	viewKeymap,
	viewKeymapPopup,
	viewPopupPatentDetail,
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
	viewDateEdit,
	viewAbstract,
	viewClaim,
	viewUSPTOKeyWarning,
	viewBulkConfirm,
	viewReviewStateSelect,
	viewProjectTags,
	viewTagSelect,
	viewCountrySelect,
	viewRecordingInfo,
}

// patentSubtitle returns "Title · Number" for overlay views, or just "Number" when no title.
func patentSubtitle(m *Model) string {
	if m.current.Number == "" {
		return ""
	}
	if t := strings.TrimSpace(m.current.Title); t != "" {
		return t + sepBullet + m.current.Number
	}
	return m.current.Number
}

// patentTitleOnly returns just the patent title (viewDetail: number is already prominent).
func patentTitleOnly(m *Model) string {
	return strings.TrimSpace(m.current.Title)
}

var modeSpecs = map[viewMode]modeSpec{
	viewList: {
		title:      "Patent List",
		themeColor: ColorThemeList,
	},
	viewDetail: {
		title:      "Detail",
		themeColor: ColorThemeDetail,
		subtitle:   patentTitleOnly,
		onActivate: func(m *Model) {
			if m.current.Number == "" || m.repo == nil {
				return
			}
			p, err := m.repo.GetPatent(m.ctx, m.ProjectID, m.current.Number)
			if err == nil {
				m.current = p
				for i, p2 := range m.patents {
					if p2.Number == m.current.Number {
						m.patents[i] = m.current
						break
					}
				}
			}
			m.populateDetailCache()
		},
	},
	viewCites: {
		title:      "Citations",
		themeColor: ColorThemeCitations,
		isOverlay:  true,
		subtitle:   patentSubtitle,
	},
	viewCitedBy: {
		title:      "Cited By",
		themeColor: ColorThemeCitedBy,
		isOverlay:  true,
		subtitle:   patentSubtitle,
	},
	viewClassifications: {
		title:      "Classifications",
		themeColor: ColorThemeClassifications,
		isOverlay:  true,
		subtitle:   patentSubtitle,
	},
	viewFullText: {
		title:      "Full Text",
		themeColor: ColorThemeText,
		isOverlay:  true,
		subtitle:   patentSubtitle,
	},
	viewNotes: {
		title:      "Notes",
		themeColor: ColorThemeNotes,
		isOverlay:  true,
		subtitle:   patentSubtitle,
	},
	viewRefs: {
		title:      "References",
		themeColor: ColorThemeReferences,
		isOverlay:  true,
	},
	viewAI: {
		title:      "AI Analysis",
		themeColor: ColorThemeAI,
		isOverlay:  true,
	},
	viewHelp: {
		title:      "Help",
		themeColor: ColorThemeHelp,
	},
	viewHelpPopup: {
		title:      "Help",
		themeColor: ColorThemeHelp,
		isOverlay:  true,
	},
	viewKeymap: {
		title:      "Keymap",
		themeColor: ColorThemeHelp,
	},
	viewKeymapPopup: {
		title:      "Keymap",
		themeColor: ColorThemeHelp,
		isOverlay:  true,
	},
	viewPopupPatentDetail: {
		title:      "Patent Detail",
		themeColor: ColorThemePreview,
		isOverlay:  true,
		subtitle:   func(m *Model) string { return m.current.Title },
	},
	viewReview: {
		title:      "Review Queue",
		themeColor: ColorThemeReview,
	},
	viewConfirmDelete: {
		title:      "Confirm Delete",
		themeColor: ColorThemeDelete,
		isOverlay:  true,
	},
	viewClassificationDetail: {
		title:      "Classification Detail",
		themeColor: ColorThemeClassifications,
		isOverlay:  true,
		subtitle:   patentSubtitle,
	},
	viewInventors: {
		title:      "Inventors",
		themeColor: ColorThemeDetail,
		isOverlay:  true,
		subtitle:   patentSubtitle,
	},
	viewFamily: {
		title:      "Patent Family",
		themeColor: ColorThemeFamily,
		isOverlay:  true,
		subtitle:   patentSubtitle,
	},
	viewSplash: {
		title:      "Splash",
		themeColor: ColorThemeList,
	},
	viewProjectEvents: {
		title:      "Project Events",
		themeColor: ColorThemeList,
		isOverlay:  true,
	},
	viewProjectInvoices: {
		title:      "Project Invoices",
		themeColor: ColorThemeList,
		isOverlay:  true,
	},
	viewProjectIDS: {
		title:      "IDS",
		themeColor: ColorThemeIDS,
		isOverlay:  true,
	},
	viewProjectInfo: {
		title:      "Project Info",
		themeColor: ColorThemeList,
		isOverlay:  true,
	},
	viewNoteEdit: {
		title:      "Note",
		themeColor: ColorThemeNotes,
		isOverlay:  true,
		subtitle:   patentSubtitle,
	},
	viewIDSEdit: {
		title:      "IDS Entry",
		themeColor: ColorThemeIDS,
		isOverlay:  true,
		subtitle:   patentSubtitle,
	},
	viewDateEdit: {
		title:      "Edit Date",
		themeColor: ColorThemeDetail,
		isOverlay:  true,
		subtitle:   patentSubtitle,
	},
	viewAbstract: {
		title:      "Abstract",
		themeColor: ColorThemeDetail,
		isOverlay:  true,
		subtitle:   patentSubtitle,
	},
	viewClaim: {
		title:      "Claim 1",
		themeColor: ColorThemeDetail,
		isOverlay:  true,
		subtitle:   patentSubtitle,
	},
	viewUSPTOKeyWarning: {
		title:      "USPTO API Key Missing",
		themeColor: ColorThemeList,
		isOverlay:  true,
	},
	viewBulkConfirm: {
		title:      "Bulk Action Confirmation",
		themeColor: ColorThemeReview,
		isOverlay:  true,
	},
	viewReviewStateSelect: {
		title:      "Select Review State",
		themeColor: ColorThemeList,
		isOverlay:  true,
	},
	viewProjectTags: {
		title:      "Project Tags",
		themeColor: ColorThemeTags,
		isOverlay:  true,
	},
	viewTagSelect: {
		title:      "Select Tags",
		themeColor: ColorThemeTags,
		isOverlay:  true,
	},
	viewCountrySelect: {
		title:      "Select Country",
		themeColor: ColorThemeList,
		isOverlay:  true,
	},
	viewRecordingInfo: {
		title:      "Recording",
		themeColor: ColorThemeReview,
		isOverlay:  true,
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
