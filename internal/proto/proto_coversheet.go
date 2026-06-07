package proto

import "patentmine/internal/domain"

// ProvisionalCoverSheetGetParams selects a project's PTO/SB/16 cover sheet.
type ProvisionalCoverSheetGetParams struct {
	Project domain.ProjectID `json:"project"`
}

// ProvisionalCoverSheetSaveParams carries the cover sheet to persist (one per project).
type ProvisionalCoverSheetSaveParams struct {
	ProvisionalCoverSheet domain.ProvisionalCoverSheet `json:"cover_sheet"`
}

// ProvisionalCoverSheetResult carries one cover sheet.
type ProvisionalCoverSheetResult struct {
	ProvisionalCoverSheet domain.ProvisionalCoverSheet `json:"cover_sheet"`
}

// ProvisionalCoverSheetPreviewResult reports where a (non-persisted) filled preview PDF was
// written. The daemon writes it to a temp path the client can open.
type ProvisionalCoverSheetPreviewResult struct {
	Path string `json:"path"`
}

// ProvisionalCoverSheetApproveResult reports the outcome of approving a cover sheet: the
// rendered/filed PDF path, or the missing required fields when approval was
// refused for an incomplete sheet (the "cannot submit" popup).
type ProvisionalCoverSheetApproveResult struct {
	ProvisionalCoverSheet domain.ProvisionalCoverSheet `json:"cover_sheet"`
	Path                  string                       `json:"path,omitempty"`
	Missing               []string                     `json:"missing,omitempty"`
}
