package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

func TestFormatExpirationShowsEstimatedSuffix(t *testing.T) {
	got := (&Model{}).formatExpiration(domain.Patent{
		ExpirationDate:   "2043-03-21",
		ExpirationSource: domain.ExpirationSourceEstimated,
	})
	if got != "2043-03-21 (est.)" {
		t.Fatalf("expected estimated expiration label, got %q", got)
	}
}

func TestFormatExpirationShowsManualSuffix(t *testing.T) {
	got := (&Model{}).formatExpiration(domain.Patent{
		ExpirationDate:   "2043-03-21",
		ExpirationSource: domain.ExpirationSourceManual,
	})
	if got != "2043-03-21 (man.)" {
		t.Fatalf("expected manual expiration label, got %q", got)
	}
}

func TestFormatExpirationShowsImportSuffix(t *testing.T) {
	got := (&Model{}).formatExpiration(domain.Patent{
		ExpirationDate:   "2043-03-21",
		ExpirationSource: domain.ExpirationSourceImported,
	})
	if got != "2043-03-21 (import)" {
		t.Fatalf("expected imported expiration label, got %q", got)
	}
}

func TestFormatExpirationKeepsExpiredLabelText(t *testing.T) {
	got := (&Model{}).formatExpiration(domain.Patent{
		ExpirationDate:   "2001-01-01",
		ExpirationSource: domain.ExpirationSourceEstimated,
	})
	if got != "2001-01-01 (est.)" {
		t.Fatalf("expected expired expiration label text, got %q", got)
	}
}

func TestDetailRowAlignsValues(t *testing.T) {
	model := &Model{repo: stubRepo{}, text: EnglishText()}
	got := model.detailRow(TextDetailGrant, "2023-03-21", 12) + "\n" + model.detailRow(TextDetailExpiration, "2043-03-21 (est.)", 12) + "\n"
	want := "Grant:       2023-03-21\nExpiration:  2043-03-21 (est.)\n"
	if got != want {
		t.Fatalf("expected aligned detail rows:\n%q\ngot:\n%q", want, got)
	}
}

func TestDetailRowUsesTextCatalog(t *testing.T) {
	model := &Model{repo: stubRepo{}, text: TextCatalog{
		TextDetailAssignee: "Titulaire",
		TextValueUnknown:   "inconnu",
	}}
	got := model.detailRow(TextDetailAssignee, "", 12)
	want := "Titulaire:   inconnu"
	if got != want {
		t.Fatalf("expected localized detail row %q, got %q", want, got)
	}
}

func TestDetailRowWrapsLongValuesUnderValueColumn(t *testing.T) {
	model := &Model{repo: stubRepo{}, text: EnglishText(), width: 60}
	got := model.detailRow(TextDetailClassification, "A, A01, A01K, A01K15/00, A01K15/02, A01K15/021", 15)
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected wrapped detail row, got %q", got)
	}
	if !strings.Contains(got, "\n                A01K15/02") {
		t.Fatalf("expected continuation line indented under value column, got %q", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if lipgloss.Width(line) > model.width {
			t.Fatalf("expected wrapped detail line width <= %d, got %d for %q", model.width, lipgloss.Width(line), line)
		}
	}
}

func TestDetailRowWrapsLongValuesWithPrefixIndent(t *testing.T) {
	model := &Model{repo: stubRepo{}, text: EnglishText(), width: 60}
	got := model.detailRow(TextDetailClassification, "A, A01, A01K, A01K15/00, A01K15/02, A01K15/021", 15, 2)
	if !strings.Contains(got, "\n                  A01K15/02") {
		t.Fatalf("expected continuation line indented with row prefix width, got %q", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if lipgloss.Width(line)+2 > model.width {
			t.Fatalf("expected wrapped detail line with prefix to fit width %d, got %d for %q", model.width, lipgloss.Width(line)+2, line)
		}
	}
}

func TestViewDetailDoesNotInsertBlankLinesBetweenRows(t *testing.T) {
	model := &Model{
		repo:  stubRepo{},
		text:  EnglishText(),
		mode:  viewDetail,
		width: 120,
		current: domain.Patent{
			Number:           "US1",
			Title:            "Test Patent",
			Assignee:         "Troxler",
			LatestAssignment: "unknown",
		},
	}

	got := model.viewDetail()
	if strings.Contains(got, "Assignee:") && strings.Contains(got, "\n\n  Latest Assignment") {
		t.Fatalf("expected no blank line between detail rows, got %q", got)
	}
	if !strings.Contains(got, "Assignee:          Troxler") {
		t.Fatalf("expected assignee row in detail view, got %q", got)
	}
	if !strings.Contains(got, "Latest Assignment: unknown") {
		t.Fatalf("expected latest assignment row in detail view, got %q", got)
	}
}

func TestPreviewOverlayUsesBorderAndSmallerWidth(t *testing.T) {
	model := &Model{
		repo:   stubRepo{},
		text:   EnglishText(),
		width:  120,
		height: 30,
		mode:   viewFamily,
		backStack: []navSnapshot{{
			mode:  viewDetail,
			width: 120,
		}},
		current: domain.Patent{Number: "US1", Title: "Preview Backdrop"},
	}
	got := model.previewOverlay("US1\nPreview")
	if !strings.Contains(got, "┌") || !strings.Contains(got, "┘") {
		t.Fatalf("expected bordered overlay, got %q", got)
	}
	if !strings.Contains(got, "US1") {
		t.Fatalf("expected centered overlay content, got %q", got)
	}
	if !strings.Contains(got, "Assignee:") {
		t.Fatalf("expected backdrop content behind overlay, got %q", got)
	}
	if width := model.overlayWidth(); width >= model.width {
		t.Fatalf("expected overlay width below terminal width, got %d for terminal %d", width, model.width)
	}
}

func TestFamilyViewIgnoresHLNavigation(t *testing.T) {
	model := &Model{repo: stubRepo{}, mode: viewFamily, familySelected: 2}
	updated, _ := model.Update(teaKey(keyColLeft))
	got := updated.(*Model)
	if got.familySelected != 2 {
		t.Fatalf("expected h to leave family selection unchanged, got %d", got.familySelected)
	}
	updated, _ = got.Update(teaKey(keyColRight))
	got = updated.(*Model)
	if got.familySelected != 2 {
		t.Fatalf("expected l to leave family selection unchanged, got %d", got.familySelected)
	}
}

func TestFamilyViewJKMovesFromCurrentSelection(t *testing.T) {
	repo := familyRepo{edges: []domain.FamilyEdge{
		{ProjectID: "default", ParentNumber: "US-PARENT", ChildNumber: "US-CURRENT", RelationType: domain.FamilyRelationContinuation},
		{ProjectID: "default", ParentNumber: "US-CURRENT", ChildNumber: "US-CHILD-1", RelationType: domain.FamilyRelationContinuation},
		{ProjectID: "default", ParentNumber: "US-CURRENT", ChildNumber: "US-CHILD-2", RelationType: domain.FamilyRelationContinuation},
	}, patents: map[string]domain.Patent{
		"US-PARENT":  {Number: "US-PARENT", Title: "Parent Patent"},
		"US-CURRENT": {Number: "US-CURRENT", Title: "Current Patent"},
		"US-CHILD-1": {Number: "US-CHILD-1", Title: "Child One"},
		"US-CHILD-2": {Number: "US-CHILD-2", Title: "Child Two"},
	}}
	model := &Model{
		ctx:            t.Context(),
		repo:           repo,
		text:           EnglishText(),
		mode:           viewFamily,
		ProjectID:      "default",
		current:        domain.Patent{Number: "US-CURRENT"},
		familySelected: 2,
	}

	updated, _ := model.Update(teaKey(keyVimDown))
	got := updated.(*Model)
	if got.familySelected != 3 {
		t.Fatalf("expected j to move from selected child to next child, got %d", got.familySelected)
	}

	updated, _ = got.Update(teaKey(keyVimUp))
	got = updated.(*Model)
	if got.familySelected != 2 {
		t.Fatalf("expected k to move back to previous child, got %d", got.familySelected)
	}
}

func TestFamilyViewShowsSelectedNodePreview(t *testing.T) {
	repo := familyRepo{edges: []domain.FamilyEdge{
		{ProjectID: "default", ParentNumber: "US-PARENT", ChildNumber: "US-CURRENT", RelationType: domain.FamilyRelationContinuation},
	}, patents: map[string]domain.Patent{
		"US-PARENT": {
			Number:         "US-PARENT",
			Title:          "A Very Long Parent Patent Title That Should Be Shortened In The Preview Panel",
			Assignee:       "Parent Corp",
			ExpirationDate: "2035-01-01",
		},
		"US-CURRENT": {Number: "US-CURRENT", Title: "Current Patent"},
	}}
	model := &Model{
		ctx:            t.Context(),
		repo:           repo,
		text:           EnglishText(),
		mode:           viewFamily,
		ProjectID:      "default",
		width:          120,
		height:         30,
		current:        domain.Patent{Number: "US-CURRENT"},
		familySelected: 0,
	}

	got := model.viewFamilyOverlay()
	for _, want := range []string{"Number:", "US-PARENT", "Expires:", "2035-01-01", "Name:", "Parent Corp", "Title:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected family preview to contain %q, got %q", want, got)
		}
	}
}

func TestFamilyViewUsesCompactDepthAndRelationBadges(t *testing.T) {
	repo := familyRepo{edges: []domain.FamilyEdge{
		{ProjectID: "default", ParentNumber: "US-PARENT", ChildNumber: "US-CURRENT", RelationType: domain.FamilyRelationContinuation},
		{ProjectID: "default", ParentNumber: "US-CURRENT", ChildNumber: "US-CHILD", RelationType: domain.FamilyRelationContinuation},
	}, patents: map[string]domain.Patent{
		"US-PARENT":  {Number: "US-PARENT", Title: "Parent Patent"},
		"US-CURRENT": {Number: "US-CURRENT", Title: "Current Patent"},
		"US-CHILD":   {Number: "US-CHILD", Title: "Child Patent"},
	}}
	model := &Model{
		ctx:       t.Context(),
		repo:      repo,
		text:      EnglishText(),
		mode:      viewFamily,
		ProjectID: "default",
		width:     120,
		height:    30,
		current:   domain.Patent{Number: "US-CURRENT"},
	}

	got := model.viewFamilyOverlay()
	if strings.Contains(got, "«Parent»") || strings.Contains(got, "«Child»") || strings.Contains(got, "Descendant") {
		t.Fatalf("expected compact family depth badges, got %q", got)
	}
	for _, want := range []string{"«↑1»", "«↓1»", "[cont.]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected family overlay to contain %q, got %q", want, got)
		}
	}
}

func TestFamilyViewShowsRefreshHintForUnloadedSelectedNode(t *testing.T) {
	repo := familyRepo{edges: []domain.FamilyEdge{
		{ProjectID: "default", ParentNumber: "US-PARENT", ChildNumber: "US-CURRENT", RelationType: domain.FamilyRelationContinuation},
	}, patents: map[string]domain.Patent{
		"US-PARENT":  {Number: "US-PARENT"},
		"US-CURRENT": {Number: "US-CURRENT", Title: "Current Patent"},
	}}
	model := &Model{
		ctx:            t.Context(),
		repo:           repo,
		text:           EnglishText(),
		mode:           viewFamily,
		ProjectID:      "default",
		width:          120,
		height:         30,
		current:        domain.Patent{Number: "US-CURRENT"},
		familySelected: 0,
	}

	got := model.viewFamilyOverlay()
	if !strings.Contains(got, FamilyNodeStatusLabelUnloaded) {
		t.Fatalf("expected unloaded family badge, got %q", got)
	}
	if !strings.Contains(got, "Press ctrl+r to refresh selected") {
		t.Fatalf("expected family refresh hint for unloaded node, got %q", got)
	}
}

func TestScreenHeaderUsesActiveModeTitle(t *testing.T) {
	model := &Model{repo: stubRepo{}, mode: viewDetail}
	got := model.renderScreenHeader()
	if !strings.Contains(got, "Detail") {
		t.Fatalf("expected detail title, got %q", got)
	}

	model.mode = viewCites
	got = model.renderScreenHeader()
	if !strings.Contains(got, "Citations") {
		t.Fatalf("expected citations title for citations overlay, got %q", got)
	}
}

func TestSplashShowsVersion(t *testing.T) {
	model := &Model{repo: stubRepo{}, mode: viewSplash, version: "v1.2.3"}
	got := model.viewSplash()
	if !strings.Contains(got, "Version v1.2.3") {
		t.Fatalf("expected splash version, got %q", got)
	}
}

func TestAllViewModesHaveSpecs(t *testing.T) {
	for _, mode := range allViewModes {
		if _, ok := lookupModeSpec(mode); !ok {
			t.Fatalf("missing mode spec for %q", mode)
		}
	}
}

func TestOverlayModesHaveRenderableSpecs(t *testing.T) {
	for _, mode := range allViewModes {
		spec := mustModeSpec(mode)
		if !spec.isOverlay {
			continue
		}
		if spec.title == "" {
			t.Fatalf("overlay mode %q missing title", mode)
		}
		if spec.themeColor == "" {
			t.Fatalf("overlay mode %q missing theme color", mode)
		}
		if spec.helpHint == "" {
			t.Fatalf("overlay mode %q missing help hint", mode)
		}
	}
}

func TestHelpKeyOpensContextPopup(t *testing.T) {
	model := &Model{
		mode:    viewCites,
		text:    EnglishText(),
		repo:    citationRepo{edges: sampleCitationEdges(2)},
		current: domain.Patent{Number: "US10218760B2"},
		width:   100,
		height:  20,
	}
	updated, _ := model.Update(teaKey(keyHelp))
	got := updated.(*Model)
	if got.mode != viewHelpPopup {
		t.Fatalf("expected mode %q, got %q", viewHelpPopup, got.mode)
	}
	if !strings.Contains(got.View(), "Help · Citations") {
		t.Fatalf("expected contextual help title, got %q", got.View())
	}
	if !strings.Contains(got.View(), "This Screen") {
		t.Fatalf("expected contextual help popup, got %q", got.View())
	}
}

func TestEditSummaryKeyOpensAbstractView(t *testing.T) {
	model := &Model{
		repo:    stubRepo{},
		text:    EnglishText(),
		mode:    viewDetail,
		current: domain.Patent{Number: "US10218760B2", Abstract: "A patent abstract."},
	}
	updated, _ := model.Update(teaKey(keyEditSummary))
	got := updated.(*Model)
	if got.mode != viewAbstract {
		t.Fatalf("expected mode %q, got %q", viewAbstract, got.mode)
	}
	if !strings.Contains(got.View(), "Abstract") {
		t.Fatalf("expected abstract overlay, got %q", got.View())
	}
}

func TestVersionCommandShowsVersionMessage(t *testing.T) {
	model := &Model{repo: stubRepo{}, logger: slog.Default(), text: EnglishText(), version: "v1.2.3"}
	updated, _ := model.runCommand(ParseCommand(":version"))
	got := updated.(*Model)
	if got.message != "PatentMine v1.2.3" {
		t.Fatalf("expected version message, got %q", got.message)
	}
}

func TestPatentDateCommandClosesPopupAndMarksExpirationManual(t *testing.T) {
	repo := &dateMutationRepo{patent: domain.Patent{Number: "US10218760B2", ExpirationSource: domain.ExpirationSourceImported}}
	model := &Model{
		ctx:          t.Context(),
		repo:         repo,
		text:         EnglishText(),
		mode:         viewDateEdit,
		ProjectID:    "default",
		current:      domain.Patent{Number: "US10218760B2"},
		backStack:    []navSnapshot{{mode: viewDetail}},
		editDateType: domain.LifecycleTypeExp,
	}

	updated, _ := model.patentDateCommand([]string{domain.LifecycleTypeExp, "2037-02-26"})
	got := updated.(*Model)
	if repo.updatedType != domain.LifecycleTypeExp || repo.updatedValue != "2037-02-26" {
		t.Fatalf("expected expiration update, got type=%q value=%q", repo.updatedType, repo.updatedValue)
	}
	if got.mode != viewDetail {
		t.Fatalf("expected mode %q after popup save, got %q", viewDetail, got.mode)
	}
	if got.current.ExpirationDate != "2037-02-26" {
		t.Fatalf("expected refreshed expiration date, got %q", got.current.ExpirationDate)
	}
	if got.current.ExpirationSource != domain.ExpirationSourceManual {
		t.Fatalf("expected expiration source %q, got %q", domain.ExpirationSourceManual, got.current.ExpirationSource)
	}
	if got.message != "updated exp date: 2037-02-26" {
		t.Fatalf("expected update message, got %q", got.message)
	}
}

func TestNextCitationDetailReviewState(t *testing.T) {
	if got := nextCitationDetailReviewState(domain.ReviewStateIgnored, false); got != domain.ReviewStateCached {
		t.Fatalf("expected new detail import review state %q, got %q", domain.ReviewStateCached, got)
	}
	if got := nextCitationDetailReviewState(domain.ReviewStateUnderReview, true); got != domain.ReviewStateUnderReview {
		t.Fatalf("expected existing citation review state to be preserved, got %q", got)
	}
}

func TestListViewShowsColumnShortcutLabelsInJumpMode(t *testing.T) {
	model := &Model{
		repo:     stubRepo{},
		text:     EnglishText(),
		mode:     viewList,
		jumpMode: true,
		patents:  []domain.Patent{{Number: "US1", Title: "Test Patent"}},
		width:    120,
		height:   20,
	}
	got := model.viewList()
	for _, key := range []string{jumpLabelPublication, jumpLabelInventors, jumpLabelClassification, jumpLabelExpiration, keyReviewState, jumpLabelUpdated, jumpLabelNotes} {
		if !strings.Contains(got, key+" ") {
			t.Fatalf("expected list header to show shortcut %q, got %q", key, got)
		}
	}
}

func TestListViewShowsIDSColumnAndStatus(t *testing.T) {
	repo := &idsMutationRepo{entries: []domain.IDSEntry{{ID: 7, ProjectID: "default", PatentNumber: "US1", Status: domain.IDSStatusSubmitted}}}
	model := &Model{
		ctx:       t.Context(),
		repo:      repo,
		text:      EnglishText(),
		mode:      viewList,
		ProjectID: "default",
		patents:   []domain.Patent{{Number: "US1", Title: "Test Patent"}},
		width:     140,
		height:    20,
	}
	got := model.viewList()
	if !strings.Contains(got, "IDS") {
		t.Fatalf("expected IDS column header, got %q", got)
	}
	if !strings.Contains(got, "Notes") || strings.Index(got, "IDS") < strings.Index(got, "Notes") {
		t.Fatalf("expected IDS column after Notes, got %q", got)
	}
	if !strings.Contains(got, string(domain.IDSStatusSubmitted)) {
		t.Fatalf("expected IDS status in row, got %q", got)
	}
}

func TestListViewFitsNarrowWidthWithoutWrapping(t *testing.T) {
	model := &Model{
		ctx:       t.Context(),
		repo:      stubRepo{},
		text:      EnglishText(),
		mode:      viewList,
		ProjectID: "default",
		patents: []domain.Patent{{
			Number:              "US123456789B2",
			Title:               "Very Long Patent Title That Should Still Stay On One Visible Row",
			Inventors:           []string{"Firstname Lastname", "Second Inventor"},
			ClassificationLabel: "H04L12/58",
			ExpirationDate:      "2043-03-21",
			ReviewState:         domain.ReviewStateStored,
			NotesCount:          12,
		}},
		width:  120,
		height: 20,
	}

	for _, line := range strings.Split(model.viewList(), "\n") {
		if lipgloss.Width(line) > model.width {
			t.Fatalf("expected line width <= %d, got %d for %q", model.width, lipgloss.Width(line), line)
		}
	}
}

func TestSortableListColumnsAreRegisteredSortColumns(t *testing.T) {
	allowed := map[string]bool{}
	for _, col := range domain.PatentSortColumns {
		allowed[col] = true
	}
	model := &Model{}
	for _, col := range model.listColumns() {
		if col.id == "" {
			continue
		}
		if !allowed[col.id] {
			t.Fatalf("list column %q uses unregistered sort id %q", col.label, col.id)
		}
		if normalizeSortCol(col.id) == "" {
			t.Fatalf("list column %q sort id %q is not normalized", col.label, col.id)
		}
	}
}

func TestEnterOnDetailIDSOpensEditPopup(t *testing.T) {
	repo := &idsMutationRepo{entries: []domain.IDSEntry{{ID: 7, ProjectID: "default", PatentNumber: "US10218760B2", Status: domain.IDSStatusPending}}}
	model := &Model{
		ctx:       t.Context(),
		repo:      repo,
		logger:    slog.Default(),
		text:      EnglishText(),
		mode:      viewDetail,
		ProjectID: "default",
		current:   domain.Patent{Number: "US10218760B2"},
		detailCache: detailCache{
			Number:     "US10218760B2",
			IDSEntries: []domain.IDSEntry{{ID: 7, ProjectID: "default", PatentNumber: "US10218760B2", Status: domain.IDSStatusPending}},
		},
	}
	for i, field := range model.detailFields() {
		if field.label == TextDetailIDS {
			model.detailSelected = i
			break
		}
	}

	updated, _ := model.Update(teaKey(keyEnter))
	got := updated.(*Model)
	if got.mode != viewIDSEdit {
		t.Fatalf("expected mode %q, got %q", viewIDSEdit, got.mode)
	}
}

func TestIDSShortcutInDetailOpensEditPopup(t *testing.T) {
	repo := &idsMutationRepo{entries: []domain.IDSEntry{{ID: 7, ProjectID: "default", PatentNumber: "US10218760B2", Status: domain.IDSStatusPending}}}
	model := &Model{
		ctx:       t.Context(),
		repo:      repo,
		text:      EnglishText(),
		mode:      viewDetail,
		ProjectID: "default",
		current:   domain.Patent{Number: "US10218760B2"},
	}

	updated, _ := model.Update(teaKey(keyIDS))
	got := updated.(*Model)
	if got.mode != viewIDSEdit {
		t.Fatalf("expected mode %q, got %q", viewIDSEdit, got.mode)
	}
}

func TestIDSEditPopupSKeyCyclesStatus(t *testing.T) {
	repo := &idsMutationRepo{entries: []domain.IDSEntry{{ID: 7, ProjectID: "default", PatentNumber: "US10218760B2", Status: domain.IDSStatusPending}}}
	model := &Model{
		ctx:       t.Context(),
		repo:      repo,
		logger:    slog.Default(),
		text:      EnglishText(),
		mode:      viewIDSEdit,
		ProjectID: "default",
		current:   domain.Patent{Number: "US10218760B2"},
		detailCache: detailCache{
			Number:     "US10218760B2",
			IDSEntries: []domain.IDSEntry{{ID: 7, ProjectID: "default", PatentNumber: "US10218760B2", Status: domain.IDSStatusPending}},
		},
	}

	updated, _ := model.Update(teaKey("s"))
	got := updated.(*Model)
	if repo.updatedID != 7 || repo.updatedStatus != domain.IDSStatusSubmitted {
		t.Fatalf("expected IDS status update to submitted, got id=%d status=%q", repo.updatedID, repo.updatedStatus)
	}
	if got.message != "IDS status: "+string(domain.IDSStatusSubmitted) {
		t.Fatalf("expected IDS status message, got %q", got.message)
	}
}

func TestIDSEditPopupDKeyDeletesEntry(t *testing.T) {
	repo := &idsMutationRepo{entries: []domain.IDSEntry{{ID: 7, ProjectID: "default", PatentNumber: "US10218760B2", Status: domain.IDSStatusAccepted}}}
	model := &Model{
		ctx:       t.Context(),
		repo:      repo,
		logger:    slog.Default(),
		text:      EnglishText(),
		mode:      viewIDSEdit,
		ProjectID: "default",
		current:   domain.Patent{Number: "US10218760B2"},
		detailCache: detailCache{
			Number:     "US10218760B2",
			IDSEntries: []domain.IDSEntry{{ID: 7, ProjectID: "default", PatentNumber: "US10218760B2", Status: domain.IDSStatusAccepted}},
		},
	}

	updated, _ := model.Update(teaKey(keyDelete))
	got := updated.(*Model)
	if repo.deletedID != 7 {
		t.Fatalf("expected IDS entry deletion, got deleted id %d", repo.deletedID)
	}
	if got.message != "IDS entry removed" {
		t.Fatalf("expected IDS removal message, got %q", got.message)
	}
}

func TestSlashInClassificationPopupStartsSearchImmediately(t *testing.T) {
	model := &Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    classificationRepo{classifications: sampleClassifications(3)},
		mode:    viewClassifications,
		current: domain.Patent{Number: "US10218760B2"},
	}
	updated, _ := model.Update(teaKey(keySearch))
	got := updated.(*Model)
	if !got.popupSearchActive {
		t.Fatal("expected popup search to activate")
	}
	if got.input.Focused() {
		t.Fatal("expected popup search to avoid focusing command input")
	}
	if !strings.Contains(got.viewClassifications(), "search:/") {
		t.Fatalf("expected search badge in popup title, got %q", got.viewClassifications())
	}
}

func TestSlashInListStartsInlineSearchImmediately(t *testing.T) {
	model := &Model{repo: stubRepo{}, text: EnglishText(), mode: viewList, patents: []domain.Patent{{Number: "US1"}}}
	updated, _ := model.Update(teaKey(keySearch))
	got := updated.(*Model)
	if !got.listSearchActive {
		t.Fatal("expected list search to activate")
	}
	if got.input.Focused() {
		t.Fatal("expected list search to avoid focusing command input")
	}
}

func TestListSearchTypingRestartsFromTop(t *testing.T) {
	model := &Model{
		repo:             stubRepo{},
		text:             EnglishText(),
		mode:             viewList,
		patents:          []domain.Patent{{Number: "CCC", Title: "Gamma"}, {Number: "BBB", Title: "Beta"}, {Number: "AAA", Title: "Alpha"}},
		selected:         2,
		listSearchActive: true,
	}
	updated, _ := model.Update(teaKey("B"))
	got := updated.(*Model)
	if got.selected != 1 {
		t.Fatalf("expected edited list search to restart from top match, got selection %d", got.selected)
	}
}

func TestListSearchNextWorksAfterEnter(t *testing.T) {
	model := &Model{
		repo:             stubRepo{},
		text:             EnglishText(),
		mode:             viewList,
		patents:          []domain.Patent{{Number: "AAA"}, {Number: "AAB"}, {Number: "CCC"}},
		selected:         0,
		listSearchActive: true,
		listSearchQuery:  "A",
	}
	updated, _ := model.Update(teaKey(keyEnter))
	got := updated.(*Model)
	if got.listSearchActive {
		t.Fatal("expected enter to exit active list search mode")
	}
	updated, _ = got.Update(teaKey(keyNotes))
	got = updated.(*Model)
	if got.selected != 1 {
		t.Fatalf("expected n to advance to next list match, got selection %d", got.selected)
	}
}

func TestPopupSearchTypingAdvancesWithoutCommandInput(t *testing.T) {
	model := &Model{
		ctx:                    t.Context(),
		text:                   EnglishText(),
		repo:                   classificationRepo{classifications: sampleClassifications(8)},
		mode:                   viewClassifications,
		current:                domain.Patent{Number: "US10218760B2"},
		classificationSelected: 0,
		popupSearchActive:      true,
	}
	updated, _ := model.Update(teaKey("4"))
	got := updated.(*Model)
	if got.popupSearchQuery != "4" {
		t.Fatalf("expected popup search query %q, got %q", "4", got.popupSearchQuery)
	}
	if got.input.Focused() {
		t.Fatal("expected popup search typing to stay in popup")
	}
}

func TestPopupSearchTypingRestartsFromTop(t *testing.T) {
	model := &Model{
		ctx:  t.Context(),
		text: EnglishText(),
		repo: classificationRepo{classifications: []domain.Classification{
			{System: "CPC", Code: "AAA", Description: "Alpha"},
			{System: "CPC", Code: "BBB", Description: "Beta"},
			{System: "CPC", Code: "CCC", Description: "Gamma"},
		}},
		mode:                   viewClassifications,
		current:                domain.Patent{Number: "US10218760B2"},
		classificationSelected: 2,
		popupSearchActive:      true,
	}

	updated, _ := model.Update(teaKey("A"))
	got := updated.(*Model)
	if got.classificationSelected != 0 {
		t.Fatalf("expected edited popup search to restart from top match, got selection %d", got.classificationSelected)
	}
}

func TestPopupSearchNextWorksAfterEnter(t *testing.T) {
	model := &Model{
		ctx:  t.Context(),
		text: EnglishText(),
		repo: classificationRepo{classifications: []domain.Classification{
			{System: "CPC", Code: "AAA", Description: "Alpha"},
			{System: "CPC", Code: "AAB", Description: "Alpine"},
			{System: "CPC", Code: "CCC", Description: "Gamma"},
		}},
		mode:                   viewClassifications,
		current:                domain.Patent{Number: "US10218760B2"},
		classificationSelected: 0,
		popupSearchActive:      true,
		popupSearchQuery:       "A",
	}

	updated, _ := model.Update(teaKey(keyEnter))
	got := updated.(*Model)
	if got.popupSearchActive {
		t.Fatal("expected enter to exit active popup search mode")
	}
	if got.mode != viewClassifications {
		t.Fatalf("expected to remain in classifications after enter, got %q", got.mode)
	}

	updated, _ = got.Update(teaKey(keyNotes))
	got = updated.(*Model)
	if got.classificationSelected != 1 {
		t.Fatal("expected n to advance to the next popup search match after enter")
	}
}

func TestQuitClearsPopupSearchBeforeClosingOverlay(t *testing.T) {
	model := &Model{
		ctx:               t.Context(),
		text:              EnglishText(),
		repo:              classificationRepo{classifications: sampleClassifications(3)},
		mode:              viewClassifications,
		current:           domain.Patent{Number: "US10218760B2"},
		popupSearchActive: true,
		popupSearchQuery:  "H04N",
	}

	updated, _ := model.Update(teaKey(keyBack))
	got := updated.(*Model)
	if got.mode != viewClassifications {
		t.Fatalf("expected to stay in popup mode, got %q", got.mode)
	}
	if got.popupSearchActive || got.popupSearchQuery != "" {
		t.Fatalf("expected q/esc back behavior to clear popup search, got active=%v query=%q", got.popupSearchActive, got.popupSearchQuery)
	}
}

func TestQuitClearsHelpSearchBeforeClosingHelp(t *testing.T) {
	model := &Model{
		repo:             stubRepo{},
		text:             EnglishText(),
		mode:             viewHelp,
		helpSearchActive: true,
		helpQuery:        "filter",
		helpScroll:       3,
	}

	updated, cmd := model.Update(teaKey(keyBack))
	got := updated.(*Model)
	if cmd != nil {
		t.Fatal("expected q back behavior while help search is active to avoid closing help")
	}
	if got.mode != viewHelp {
		t.Fatalf("expected to stay in help mode, got %q", got.mode)
	}
	if got.helpSearchActive || got.helpQuery != "" || got.helpScroll != 0 {
		t.Fatalf("expected q/esc back behavior to clear help search state, got active=%v query=%q scroll=%d", got.helpSearchActive, got.helpQuery, got.helpScroll)
	}
}

func TestQuitClearsListSearchBeforeExiting(t *testing.T) {
	model := &Model{
		ctx:              t.Context(),
		text:             EnglishText(),
		repo:             stubRepo{},
		mode:             viewList,
		listSearchActive: true,
		listSearchQuery:  "US102",
	}

	updated, cmd := model.Update(teaKey(keyBack))
	got := updated.(*Model)
	if cmd != nil {
		t.Fatal("expected q back behavior while list search is active to avoid quit command")
	}
	if got.listSearchActive || got.listSearchQuery != "" {
		t.Fatalf("expected q/esc back behavior to clear list search, got active=%v query=%q", got.listSearchActive, got.listSearchQuery)
	}
	if got.mode != viewList {
		t.Fatalf("expected to remain in list mode, got %q", got.mode)
	}
}

func TestNavigationStackGoesBackToPreviousView(t *testing.T) {
	model := &Model{repo: stubRepo{}, mode: viewList, selected: 3}
	model = model.navigateTo(viewDetail)
	model.selected = 0

	updated, _ := model.goBack()
	got := updated.(*Model)
	if got.mode != viewList {
		t.Fatalf("expected view %q, got %q", viewList, got.mode)
	}
	if got.selected != 3 {
		t.Fatalf("expected selection to be restored, got %d", got.selected)
	}
}

func TestApplyJumpSelectsVisibleListTarget(t *testing.T) {
	model := &Model{
		repo: stubRepo{},
		mode: viewList,
	}
	model.patents, _ = model.repo.ListPatents(model.ctx, model.ProjectID, storage.ListPatentsOptions{})
	model.jumpLabelsCache = model.jumpLabels()

	// Select 3rd patent (index 2). Labels are: 0-9 (headers), 10, 11, 12 (US3)
	label := model.jumpLabelsCache[12]
	updated, _ := model.applyJump(label.key)
	got := updated.(*Model)
	if got.selected != 2 {
		t.Fatalf("expected list selection 2, got %d", got.selected)
	}
}

func TestApplyJumpSelectsDetailField(t *testing.T) {
	model := &Model{repo: stubRepo{},
		mode:     viewDetail,
		jumpMode: true,
		text:     EnglishText(),
		current:  domain.Patent{Assignee: "Divx LLC", Inventors: []string{"Kourosh Soroushian"}},
	}
	model.jumpLabelsCache = model.jumpLabels()
	updated, _ := model.applyJump(jumpLabelInventors)
	got := updated.(*Model)
	if got.detailSelected != 2 {
		t.Fatalf("expected detail selection 2, got %d", got.detailSelected)
	}
}

func TestDetailJumpLabelsMatchFields(t *testing.T) {
	model := &Model{repo: stubRepo{},

		mode:    viewDetail,
		text:    EnglishText(),
		current: domain.Patent{Assignee: "Divx LLC", Inventors: []string{"Kourosh Soroushian"}},
	}
	got := model.jumpLabels()

	// Build set of jump labels present in output
	present := map[string]bool{}
	for _, jl := range got {
		if jl.key != "" {
			present[jl.key] = true
		}
	}

	// Check every non-optional registered label appears
	for _, reg := range DetailFieldLabelRegistrations() {
		if reg.JumpLabel == "" || reg.Optional {
			continue
		}
		if !present[reg.JumpLabel] {
			t.Errorf("required jump label %q for field %q not found in jumpLabels()", reg.JumpLabel, reg.TextKey)
		}
	}
}

func TestDetailFieldsAlwaysIncludeClassification(t *testing.T) {
	model := &Model{repo: stubRepo{},

		text:    EnglishText(),
		current: domain.Patent{Assignee: "Divx LLC"},
	}
	fields := model.detailFields()
	found := false
	for _, field := range fields {
		if field.label == TextDetailClassification {
			found = true
			if field.displayValue != "Empty" {
				t.Fatalf("expected empty classification value, got %+v", field)
			}
			if field.action != detailActionClassification {
				t.Fatalf("expected classification action, got %+v", field)
			}
		}
	}
	if !found {
		t.Fatal("expected classification field")
	}
}

func TestViewClassificationsShowsEmptyState(t *testing.T) {
	model := &Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    emptyClassificationRepo{},
		current: domain.Patent{Number: "US10218760B2"},
		mode:    viewClassifications,
	}
	got := model.viewClassifications()
	if !strings.Contains(got, "No CPC/USPC") {
		t.Fatalf("expected empty classification message, got %q", got)
	}
}

func TestViewClassificationsShowsPagedIndexedRows(t *testing.T) {
	model := &Model{
		ctx:                    t.Context(),
		text:                   EnglishText(),
		repo:                   classificationRepo{classifications: sampleClassifications(8)},
		current:                domain.Patent{Number: "US10218760B2"},
		classificationSelected: 6,
		mode:                   viewClassifications,
		height:                 12,
		width:                  80,
	}
	got := model.viewClassifications()
	// Total 8 items. overlayPageSize with m.height=12 is max(3, 12-18) = 3.
	// selected=6 (item 7) -> Page 3/3 (items 7-8 of 8)
	if !strings.Contains(got, "Page 3/3 - items 7-8 of 8") {
		t.Fatalf("expected page status, got %q", got)
	}
	if strings.Contains(got, "[CPC]") {
		t.Fatalf("expected no CPC prefix, got %q", got)
	}
	if !strings.Contains(got, ">   7") {
		t.Fatalf("expected selected absolute row index, got %q", got)
	}
	if !strings.Contains(got, "H04N21/436") {
		t.Fatalf("expected selected classification code, got %q", got)
	}
}

func TestEnterOnDetailClassificationOpensClassificationList(t *testing.T) {
	model := &Model{
		ctx:            t.Context(),
		text:           EnglishText(),
		repo:           classificationRepo{classifications: sampleClassifications(3)},
		mode:           viewDetail,
		current:        domain.Patent{Number: "US10218760B2", ClassificationLabel: "H04N21/430, H04N21/431, H04N21/432"},
		detailSelected: detailFieldIndex(EnglishText(), TextDetailClassification),
		width:          100,
		height:         20,
	}
	updated, _ := model.Update(teaKey(keyEnter))
	got := updated.(*Model)
	if got.mode != viewClassifications {
		t.Fatalf("expected mode %q, got %q", viewClassifications, got.mode)
	}
	view := got.viewClassifications()
	if !strings.Contains(view, "H04N21/430") || !strings.Contains(view, "Page 1/1") {
		t.Fatalf("expected classification list, got %q", view)
	}
}

func TestOpenKeyOnDetailClassificationOpensClassificationList(t *testing.T) {
	model := &Model{
		ctx:            t.Context(),
		text:           EnglishText(),
		repo:           classificationRepo{classifications: sampleClassifications(3)},
		mode:           viewDetail,
		current:        domain.Patent{Number: "US10218760B2", ClassificationLabel: "H04N21/430"},
		detailSelected: detailFieldIndex(EnglishText(), TextDetailClassification),
		width:          100,
		height:         20,
	}
	updated, _ := model.Update(teaKey(keyOpen))
	got := updated.(*Model)
	if got.mode != viewClassifications {
		t.Fatalf("expected mode %q, got %q", viewClassifications, got.mode)
	}
}

func TestEnterOnDetailCountryFiltersStoredPatentsByCountry(t *testing.T) {
	model := &Model{
		ctx:       t.Context(),
		text:      EnglishText(),
		repo:      stubRepo{},
		mode:      viewDetail,
		ProjectID: "default",
		current:   domain.Patent{Number: "US10218760B2", CountryCode: domain.PatentCountryUS},
		width:     100,
		height:    20,
	}
	for i, field := range model.detailFields() {
		if field.label == TextDetailCountry {
			model.detailSelected = i
			break
		}
	}
	updated, _ := model.Update(teaKey(keyEnter))
	got := updated.(*Model)
	if got.mode != viewList {
		t.Fatalf("expected mode %q, got %q", viewList, got.mode)
	}
	if got.countryFilter != domain.PatentCountryUS {
		t.Fatalf("expected country filter %q, got %q", domain.PatentCountryUS, got.countryFilter)
	}
	if got.reviewStateFilter != domain.ReviewStateStored {
		t.Fatalf("expected stored review state filter, got %q", got.reviewStateFilter)
	}
}

func TestEnterOnClassificationListOpensDetail(t *testing.T) {
	model := &Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    classificationRepo{classifications: sampleClassifications(3)},
		mode:    viewClassifications,
		current: domain.Patent{Number: "US10218760B2"},
		width:   100,
		height:  20,
	}
	updated, _ := model.Update(teaKey(keyEnter))
	got := updated.(*Model)
	if got.mode != viewClassificationDetail {
		t.Fatalf("expected mode %q, got %q", viewClassificationDetail, got.mode)
	}
}

func TestOpenKeyOnClassificationListOpensDetail(t *testing.T) {
	model := &Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    classificationRepo{classifications: sampleClassifications(3)},
		mode:    viewClassifications,
		current: domain.Patent{Number: "US10218760B2"},
		width:   100,
		height:  20,
	}
	updated, _ := model.Update(teaKey(keyOpen))
	got := updated.(*Model)
	if got.mode != viewClassificationDetail {
		t.Fatalf("expected mode %q, got %q", viewClassificationDetail, got.mode)
	}
}

func TestClassificationListViewRendersPopup(t *testing.T) {
	model := &Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    classificationRepo{classifications: sampleClassifications(3)},
		mode:    viewClassifications,
		current: domain.Patent{Number: "US10218760B2"},
		width:   120,
		height:  40,
	}
	view := model.View()
	if !strings.Contains(view, "┌") {
		t.Fatalf("expected popup border in View(), got:\n%s", view)
	}
	if !strings.Contains(view, "H04N21/430") {
		t.Fatalf("expected classification code in View(), got:\n%s", view)
	}
}

func TestClassificationDetailViewRendersPopup(t *testing.T) {
	model := &Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    classificationRepo{classifications: sampleClassifications(3)},
		mode:    viewClassificationDetail,
		current: domain.Patent{Number: "US10218760B2"},
		width:   120,
		height:  40,
	}
	view := model.View()
	if !strings.Contains(view, "┌") {
		t.Fatalf("expected popup border in View(), got:\n%s", view)
	}
	if !strings.Contains(view, "H04N21/430") {
		t.Fatalf("expected classification code in View(), got:\n%s", view)
	}
}

func TestPressLFromListOpensClassificationPopup(t *testing.T) {
	patents := []domain.Patent{{Number: "US10218760B2", Title: "Test Patent"}}
	model := &Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    classificationRepo{classifications: sampleClassifications(3)},
		mode:    viewList,
		patents: patents,
		current: domain.Patent{},
		width:   120,
		height:  40,
	}
	updated, _ := model.Update(teaKey(keyClassification))
	got := updated.(*Model)
	if got.mode != viewClassifications {
		t.Fatalf("expected mode %q after pressing %q, got %q", viewClassifications, keyClassification, got.mode)
	}
	view := got.View()
	if !strings.Contains(view, "┌") {
		t.Fatalf("expected popup border in View() after pressing %q, got:\n%s", keyClassification, view)
	}
}

func TestClassificationPageKeysMoveByPage(t *testing.T) {
	model := &Model{
		ctx:                    t.Context(),
		text:                   EnglishText(),
		repo:                   classificationRepo{classifications: sampleClassifications(8)},
		mode:                   viewClassifications,
		current:                domain.Patent{Number: "US10218760B2"},
		classificationSelected: 0,
		height:                 12,
	}
	updated, _ := model.Update(teaKey(keyCtrlF))
	got := updated.(*Model)
	if got.classificationSelected != 5 {
		t.Fatalf("expected page-down classification selection 5, got %d", got.classificationSelected)
	}
	updated, _ = got.Update(teaKey(keyCtrlD))
	got = updated.(*Model)
	if got.classificationSelected != 0 {
		t.Fatalf("expected page-up classification selection 0, got %d", got.classificationSelected)
	}
}

func TestNumericPrefixMovesSelections(t *testing.T) {
	model := &Model{repo: stubRepo{},

		mode:     viewList,
		patents:  []domain.Patent{{Number: "US1"}, {Number: "US2"}, {Number: "US3"}, {Number: "US4"}},
		selected: 0,
	}
	updated, _ := model.Update(teaKey("3"))
	updated, _ = updated.Update(teaKey(keyVimDown))
	got := updated.(*Model)
	if got.selected != 3 {
		t.Fatalf("expected 3j to move to row 4, got %d", got.selected)
	}
	updated, _ = got.Update(teaKey("2"))
	updated, _ = updated.Update(teaKey(keyVimUp))
	got = updated.(*Model)
	if got.selected != 1 {
		t.Fatalf("expected 2k to move to row 2, got %d", got.selected)
	}
}

func TestNumericPrefixGoesToAbsoluteRow(t *testing.T) {
	model := &Model{
		ctx:  t.Context(),
		text: EnglishText(),
		repo: classificationRepo{classifications: sampleClassifications(12)},

		mode:    viewClassifications,
		current: domain.Patent{Number: "US10218760B2"},
	}
	updated, _ := model.Update(teaKey(keyFirstClaim))
	updated, _ = updated.Update(teaKey("0"))
	updated, _ = updated.Update(teaKey(keyGoto))
	got := updated.(*Model)
	if got.classificationSelected != 9 {
		t.Fatalf("expected 10g to jump to row 10, got %d", got.classificationSelected)
	}
}

func TestViewCitationsShowsIndexedRows(t *testing.T) {
	model := &Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    citationRepo{edges: sampleCitationEdges(3)},
		mode:    viewCites,
		current: domain.Patent{Number: "US10218760B2"},
		width:   100,
		height:  20,
	}
	got := model.viewCitations(domain.RelationCites)
	if !strings.Contains(got, "#") {
		t.Fatalf("expected index header, got %q", got)
	}
	if !strings.Contains(got, ">") || !strings.Contains(got, "  1 US1000001B2") {
		t.Fatalf("expected selected indexed citation row, got %q", got)
	}
}

func TestViewCitationsFitsOverlayAndOmitsImportSourceTag(t *testing.T) {
	edges := sampleCitationEdges(1)
	edges[0].ReviewState = domain.ReviewStateIgnored
	edges[0].TargetImportSource = ImportSourceGoogle
	edges[0].TargetTitle = "Paving-related measuring device incorporating a computer device and communication element therebetween and another very long suffix"
	model := &Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    citationRepo{edges: edges},
		mode:    viewCites,
		current: domain.Patent{Number: "US10218760B2"},
		width:   100,
		height:  20,
	}

	got := model.viewCitations(domain.RelationCites)
	if strings.Contains(got, "[g]") || strings.Contains(got, "[u]") {
		t.Fatalf("expected citation rows without inline import source tags, got %q", got)
	}
	if strings.Contains(got, "\n│  Status") {
		t.Fatalf("expected status column to stay on the header row, got %q", got)
	}
}

func TestOpenStoredCitationShowsPreviewOverlay(t *testing.T) {
	model := &Model{
		ctx:     t.Context(),
		text:    EnglishText(),
		repo:    storedCitationRepo{citationRepo: citationRepo{edges: sampleCitationEdges(1)}},
		mode:    viewCites,
		current: domain.Patent{Number: "US10218760B2"},
		width:   100,
		height:  20,
	}
	updated, _ := model.Update(teaKey(keyEnter))
	got := updated.(*Model)
	if got.mode != viewPreview {
		t.Fatalf("expected mode %q, got %q", viewPreview, got.mode)
	}
	if got.pendingBundle.Patent.Number != "US1000001B2" {
		t.Fatalf("expected pending preview patent, got %+v", got.pendingBundle.Patent)
	}
	if view := got.View(); !strings.Contains(view, "Reference preview") || !strings.Contains(view, "US1000001B2") {
		t.Fatalf("expected preview overlay content, got %q", view)
	}
}

func TestViewReviewQueueShowsIndexedRows(t *testing.T) {
	model := &Model{
		ctx:          t.Context(),
		text:         EnglishText(),
		repo:         citationRepo{edges: sampleCitationEdges(3)},
		mode:         viewReview,
		reviewState: domain.ReviewStateUnderReview,
		width:        100,
		height:       20,
	}
	got := model.viewReviewQueue()
	if !strings.Contains(got, "#") {
		t.Fatalf("expected index header, got %q", got)
	}
	if !strings.Contains(got, ">") || !strings.Contains(got, "  1 US1000001B2") {
		t.Fatalf("expected selected indexed review row, got %q", got)
	}
}

func TestVisibleCitationEdgesReturnsCurrentPage(t *testing.T) {
	model := &Model{
		ctx:           t.Context(),
		text:          EnglishText(),
		repo:          citationRepo{edges: sampleCitationEdges(12)},
		mode:          viewCites,
		current:       domain.Patent{Number: "US10218760B2"},
		citesSelected: 7,
		height:        12,
	}
	edges, err := model.visibleCitationEdges()
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 5 {
		t.Fatalf("expected 5 visible edges, got %d", len(edges))
	}
	if edges[0].TargetPatent != "US1000006B2" {
		t.Fatalf("expected current page to start at row 6, got %+v", edges[0])
	}
}

func TestRowIndexLabelUsesThreeSpacePaddedCharacters(t *testing.T) {
	tests := map[int]string{
		0:   "  1",
		11:  " 12",
		122: "123",
	}
	for input, want := range tests {
		if got := rowIndexLabel(input); got != want {
			t.Fatalf("expected rowIndexLabel(%d) = %q, got %q", input, want, got)
		}
	}
}

func TestDetailFieldsGroupInventors(t *testing.T) {
	model := &Model{repo: stubRepo{},

		text: EnglishText(),
		current: domain.Patent{
			Assignee:  "Divx LLC",
			Inventors: []string{"Inventor One", "Inventor Two"},
		},
	}
	fields := model.detailFields()
	// Index 2 is now the grouped inventors field
	if fields[2].label != TextDetailInventors || fields[2].value != "Inventor One, Inventor Two" || fields[2].displayValue != "(2) Inventor One, Inventor Two" {
		t.Fatalf("unexpected grouped inventors field: %+v", fields[2])
	}
	if fields[2].jumpLabel != jumpLabelInventors {
		t.Fatalf("unexpected inventor jump label: %q", fields[2].jumpLabel)
	}
}

func TestDeleteShortcutEntersConfirmationMode(t *testing.T) {
	model := &Model{repo: stubRepo{},

		mode:    viewList,
		patents: []domain.Patent{{Number: "US1"}},
	}
	updated, _ := model.Update(teaKey(keyDelete))
	got := updated.(*Model)
	if got.mode != viewConfirmDelete {
		t.Fatalf("expected mode %q, got %q", viewConfirmDelete, got.mode)
	}
}

func TestConfirmationNoReturnsToList(t *testing.T) {
	model := &Model{repo: stubRepo{},

		mode: viewConfirmDelete,
	}
	updated, _ := model.Update(teaKey(keyNo))
	got := updated.(*Model)
	if got.mode != viewList {
		t.Fatalf("expected mode %q, got %q", viewList, got.mode)
	}
}

func teaKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

type emptyClassificationRepo struct {
	stubRepo
}

func (emptyClassificationRepo) ListClassifications(context.Context, string, string) ([]domain.Classification, error) {
	return nil, nil
}

type classificationRepo struct {
	stubRepo
	classifications []domain.Classification
}

func (r classificationRepo) ListClassifications(context.Context, string, string) ([]domain.Classification, error) {
	return r.classifications, nil
}

type familyRepo struct {
	stubRepo
	edges   []domain.FamilyEdge
	patents map[string]domain.Patent
}

func (r familyRepo) ListAllFamilyEdges(context.Context, string) ([]domain.FamilyEdge, error) {
	return append([]domain.FamilyEdge(nil), r.edges...), nil
}

func (r familyRepo) GetPatent(_ context.Context, _ string, number string) (domain.Patent, error) {
	if p, ok := r.patents[number]; ok {
		return p, nil
	}
	return domain.Patent{}, nil
}

func (r familyRepo) ListFamilyPatents(_ context.Context, _ string, numbers []string) (map[string]domain.Patent, error) {
	out := make(map[string]domain.Patent, len(numbers))
	for _, number := range numbers {
		if p, ok := r.patents[number]; ok {
			out[number] = p
		}
	}
	return out, nil
}

type citationRepo struct {
	stubRepo
	edges []domain.CitationEdge
}

func (r citationRepo) ListCitations(context.Context, string, string, string, storage.ListCitationsOptions) ([]domain.CitationEdge, error) {
	return r.edges, nil
}

func (r citationRepo) ListCitationsByReviewState(context.Context, string, string, storage.ListCitationsOptions) ([]domain.CitationEdge, error) {
	return r.edges, nil
}

type storedCitationRepo struct {
	citationRepo
}

func (r storedCitationRepo) GetPatent(_ context.Context, _ string, number string) (domain.Patent, error) {
	return domain.Patent{
		Number:           number,
		Title:            "Stored citation patent",
		Inventors:        []string{"Inventor One"},
		PublicationDate:  "2019-01-01",
		GrantDate:        "2020-01-01",
		ExpirationDate:   "2040-01-01",
		ExpirationSource: domain.ExpirationSourceEstimated,
		SourceGoogleURL:  "https://patents.google.com/patent/" + number + "/en",
	}, nil
}

func sampleClassifications(count int) []domain.Classification {
	out := make([]domain.Classification, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, domain.Classification{
			System:      "CPC",
			Code:        fmt.Sprintf("H04N21/43%d", i),
			Description: fmt.Sprintf("Classification description %d", i+1),
		})
	}
	return out
}

func sampleCitationEdges(count int) []domain.CitationEdge {
	out := make([]domain.CitationEdge, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, domain.CitationEdge{
			SourcePatent:         "US10218760B2",
			TargetPatent:         fmt.Sprintf("US100000%dB2", i+1),
			RelationType:         domain.RelationCites,
			ReviewState:          domain.ReviewStateUnderReview,
			TargetTitle:          fmt.Sprintf("Citation %d", i+1),
			TargetInventors:      []string{fmt.Sprintf("Inventor %d", i+1)},
			TargetExpirationDate: "2030-01-01",
		})
	}
	return out
}

func detailFieldIndex(text TextCatalog, label TextKey) int {
	model := &Model{repo: stubRepo{}, text: text}
	for i, field := range model.detailFields() {
		if field.label == label {
			return i
		}
	}
	return 0
}

type stubRepo struct{}

type idsMutationRepo struct {
	stubRepo
	entries       []domain.IDSEntry
	updatedID     int64
	updatedStatus domain.IDSStatus
	deletedID     int64
}

type dateMutationRepo struct {
	stubRepo
	patent       domain.Patent
	updatedType  string
	updatedValue string
}

func (r *idsMutationRepo) ListIDSEntries(context.Context, string) ([]domain.IDSEntry, error) {
	return append([]domain.IDSEntry(nil), r.entries...), nil
}

func (r *idsMutationRepo) UpdateIDSEntryStatus(_ context.Context, id int64, status domain.IDSStatus) error {
	r.updatedID = id
	r.updatedStatus = status
	for i := range r.entries {
		if r.entries[i].ID == id {
			r.entries[i].Status = status
		}
	}
	return nil
}

func (r *idsMutationRepo) AddIDSEntry(_ context.Context, entry domain.IDSEntry) (domain.IDSEntry, error) {
	entry.ID = int64(len(r.entries) + 1)
	r.entries = append(r.entries, entry)
	return entry, nil
}

func (r *idsMutationRepo) DeleteIDSEntry(_ context.Context, id int64) error {
	r.deletedID = id
	filtered := r.entries[:0]
	for _, entry := range r.entries {
		if entry.ID != id {
			filtered = append(filtered, entry)
		}
	}
	r.entries = filtered
	return nil
}

func (r *idsMutationRepo) GetIDSMetadata(context.Context, string) (domain.IDSMetadata, error) {
	return domain.IDSMetadata{}, nil
}
func (r *idsMutationRepo) SaveIDSMetadata(context.Context, domain.IDSMetadata) error { return nil }
func (r *idsMutationRepo) AddIDSNPLEntry(_ context.Context, entry domain.IDSEntry) (domain.IDSEntry, error) {
	entry.ID = int64(len(r.entries) + 1)
	entry.IsNPL = true
	r.entries = append(r.entries, entry)
	return entry, nil
}
func (r *idsMutationRepo) ListIDSNPLEntries(context.Context, string) ([]domain.IDSEntry, error) {
	return nil, nil
}
func (r *idsMutationRepo) DeleteIDSNPLEntry(_ context.Context, id int64) error {
	filtered := r.entries[:0]
	for _, entry := range r.entries {
		if entry.ID != id {
			filtered = append(filtered, entry)
		}
	}
	r.entries = filtered
	return nil
}

func (r *dateMutationRepo) UpdatePatentDate(_ context.Context, number string, dateType string, value string) error {
	r.updatedType = dateType
	r.updatedValue = value
	r.patent.Number = number
	switch dateType {
	case domain.LifecycleTypeApp:
		r.patent.ApplicationDate = value
	case domain.LifecycleTypePub:
		r.patent.PublicationDate = value
	case domain.LifecycleTypeGrant:
		r.patent.GrantDate = value
	case domain.LifecycleTypeExp:
		r.patent.ExpirationDate = value
		r.patent.ExpirationSource = domain.ExpirationSourceManual
	}
	return nil
}

func (r *dateMutationRepo) GetPatent(context.Context, string, string) (domain.Patent, error) {
	return r.patent, nil
}

func (stubRepo) Close() error                                        { return nil }
func (stubRepo) Setup(context.Context) error                         { return nil }
func (stubRepo) CreateProject(context.Context, domain.Project) error { return nil }
func (stubRepo) GetProject(context.Context, string) (domain.Project, error) {
	return domain.Project{}, nil
}
func (stubRepo) ListProjects(context.Context) ([]domain.Project, error)                { return nil, nil }
func (stubRepo) UpdateProject(context.Context, domain.Project) error                   { return nil }
func (stubRepo) DeleteProject(context.Context, string) error                           { return nil }
func (stubRepo) AddPatentToProject(context.Context, string, string) error              { return nil }
func (stubRepo) RemovePatentFromProject(context.Context, string, string) error         { return nil }
func (stubRepo) UpsertPatentBundle(context.Context, string, domain.PatentBundle) error { return nil }
func (stubRepo) GetPatent(context.Context, string, string) (domain.Patent, error) {
	return domain.Patent{}, nil
}
func (stubRepo) ListFamilyPatents(context.Context, string, []string) (map[string]domain.Patent, error) {
	return map[string]domain.Patent{}, nil
}
func (stubRepo) ListPatents(context.Context, string, storage.ListPatentsOptions) ([]domain.Patent, error) {
	return []domain.Patent{{Number: "US1"}, {Number: "US2"}, {Number: "US3"}, {Number: "US4"}}, nil
}

func (stubRepo) ListCitations(context.Context, string, string, string, storage.ListCitationsOptions) ([]domain.CitationEdge, error) {
	return nil, nil
}
func (stubRepo) ListCitationsByReviewState(context.Context, string, string, storage.ListCitationsOptions) ([]domain.CitationEdge, error) {
	return nil, nil
}
func (stubRepo) UpdateCitationReviewState(context.Context, string, domain.CitationEdge, string) error {
	return nil
}
func (stubRepo) UpdatePatentReviewState(context.Context, string, string, string) error { return nil }
func (stubRepo) UpdatePatentDate(context.Context, string, string, string) error   { return nil }
func (stubRepo) UpdatePatentNumber(context.Context, string, string, string) error { return nil }
func (stubRepo) UpdateClassificationDescription(context.Context, string, string, string, string) error {
	return nil
}
func (stubRepo) DeletePatent(context.Context, string, string) error { return nil }
func (stubRepo) ListClassifications(context.Context, string, string) ([]domain.Classification, error) {
	return nil, nil
}
func (stubRepo) ListTextSections(context.Context, string, string) ([]domain.PatentTextSection, error) {
	return nil, nil
}
func (stubRepo) AddNote(context.Context, string, string, string) (domain.ResearchNote, error) {
	return domain.ResearchNote{}, nil
}
func (stubRepo) ListNotes(context.Context, string, string) ([]domain.ResearchNote, error) {
	return nil, nil
}
func (stubRepo) AddReference(context.Context, string, string, string) (domain.ReferenceEntry, error) {
	return domain.ReferenceEntry{}, nil
}
func (stubRepo) ListReferences(context.Context, string) ([]domain.ReferenceEntry, error) {
	return nil, nil
}
func (stubRepo) AddAIAnalysis(context.Context, string, domain.AIAnalysis) (domain.AIAnalysis, error) {
	return domain.AIAnalysis{}, nil
}
func (stubRepo) ListAIAnalyses(context.Context, string, string) ([]domain.AIAnalysis, error) {
	return nil, nil
}
func (stubRepo) AddProjectEvent(context.Context, domain.ProjectEvent) (domain.ProjectEvent, error) {
	return domain.ProjectEvent{}, nil
}
func (stubRepo) ListProjectEvents(context.Context, string) ([]domain.ProjectEvent, error) {
	return nil, nil
}
func (stubRepo) DeleteProjectEvent(context.Context, int64) error { return nil }
func (stubRepo) AddProjectInvoice(context.Context, domain.ProjectInvoice) (domain.ProjectInvoice, error) {
	return domain.ProjectInvoice{}, nil
}
func (stubRepo) ListProjectInvoices(context.Context, string) ([]domain.ProjectInvoice, error) {
	return nil, nil
}
func (stubRepo) UpdateProjectInvoice(context.Context, domain.ProjectInvoice) error { return nil }
func (stubRepo) DeleteProjectInvoice(context.Context, int64) error                 { return nil }
func (stubRepo) CountUnpaidInvoicesByProject(context.Context) (map[string]int, error) {
	return nil, nil
}
func (stubRepo) GetSetting(context.Context, string) (string, error)     { return "", nil }
func (stubRepo) SetSetting(context.Context, string, string) error       { return nil }
func (stubRepo) AddFamilyEdge(context.Context, domain.FamilyEdge) error { return nil }
func (stubRepo) ListFamilyEdges(context.Context, string, string) ([]domain.FamilyEdge, []domain.FamilyEdge, error) {
	return nil, nil, nil
}
func (stubRepo) ListAllFamilyEdges(context.Context, string) ([]domain.FamilyEdge, error) {
	return nil, nil
}
func (stubRepo) RemoveFamilyEdge(context.Context, string, string, string) error { return nil }
func (stubRepo) PurgeIgnored(context.Context, string) (int, error)              { return 0, nil }
func (stubRepo) Compact(context.Context) error                                  { return nil }
func (stubRepo) AddIDSEntry(context.Context, domain.IDSEntry) (domain.IDSEntry, error) {
	return domain.IDSEntry{}, nil
}
func (stubRepo) ListIDSEntries(context.Context, string) ([]domain.IDSEntry, error) {
	return nil, nil
}
func (stubRepo) UpdateIDSEntryStatus(context.Context, int64, domain.IDSStatus) error { return nil }
func (stubRepo) UpdateIDSEntry(context.Context, domain.IDSEntry) error               { return nil }
func (stubRepo) DeleteIDSEntry(context.Context, int64) error                         { return nil }
func (stubRepo) GetIDSMetadata(context.Context, string) (domain.IDSMetadata, error) {
	return domain.IDSMetadata{}, nil
}
func (stubRepo) SaveIDSMetadata(context.Context, domain.IDSMetadata) error { return nil }
func (stubRepo) AddIDSNPLEntry(context.Context, domain.IDSEntry) (domain.IDSEntry, error) {
	return domain.IDSEntry{}, nil
}
func (stubRepo) ListIDSNPLEntries(context.Context, string) ([]domain.IDSEntry, error) {
	return nil, nil
}
func (stubRepo) DeleteIDSNPLEntry(context.Context, int64) error { return nil }

// Tag stubs
func (stubRepo) CreateTag(context.Context, string, string, string) (int64, error)            { return 0, nil }
func (stubRepo) ListTagsWithCounts(context.Context, string) ([]domain.TagWithCount, error)    { return nil, nil }
func (stubRepo) DeleteTag(context.Context, int64) error                                       { return nil }
func (stubRepo) RenameTag(context.Context, int64, string) error                               { return nil }
func (stubRepo) UpdateTagColor(context.Context, int64, string) error                          { return nil }
func (stubRepo) GetTagByName(context.Context, string, string) (domain.Tag, error)             { return domain.Tag{}, nil }
func (stubRepo) ApplyTagToPatent(context.Context, string, int64) error                        { return nil }
func (stubRepo) RemoveTagFromPatent(context.Context, string, int64) error                     { return nil }
func (stubRepo) GetPatentTags(context.Context, string) ([]domain.Tag, error)                  { return nil, nil }
func (stubRepo) ListPatentTagsForProject(context.Context, string) (map[string][]domain.Tag, error) { return nil, nil }
