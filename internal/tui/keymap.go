package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type KeyHandler func(*Model) (tea.Model, tea.Cmd)

type KeyBinding struct {
	Key        string
	Label      string
	Handler    KeyHandler
	DualUse    bool    // intentional global key shadow, suppress warning
	Section    string  // Help section title (e.g., "Navigation", "Patents", "Views")
	Help       TextKey // TextKey for the help description
	ShowInHint bool
}

type KeyMap map[string]KeyBinding

var (
	GlobalKeys        = KeyMap{}
	ModeKeys          = map[viewMode]KeyMap{}
	DetailFieldLabels []detailFieldLabelReg
)

type detailFieldLabelReg struct {
	JumpLabel string
	TextKey   TextKey
	Optional  bool // only present under conditions (e.g. p.Number != "")
}

func registerDetailField(jumpLabel string, textKey TextKey, optional bool) {
	for _, r := range DetailFieldLabels {
		if r.JumpLabel == jumpLabel {
			panic(fmt.Sprintf("detail field jump label %q already registered for %q", jumpLabel, r.TextKey))
		}
	}
	DetailFieldLabels = append(DetailFieldLabels, detailFieldLabelReg{
		JumpLabel: jumpLabel,
		TextKey:   textKey,
		Optional:  optional,
	})
}

func validateDetailFieldLabels() error {
	var errs []error
	for _, reg := range DetailFieldLabels {
		if reg.JumpLabel == "" {
			continue
		}
	}
	return errors.Join(errs...)
}

// DetailFieldLabelRegistrations returns registered detail field jump labels for test validation.
func DetailFieldLabelRegistrations() []detailFieldLabelReg {
	r := make([]detailFieldLabelReg, len(DetailFieldLabels))
	copy(r, DetailFieldLabels)
	return r
}

const (
	keymapGlobalMode = "*global*"
	keymapDetailMode = "detail-field"

	keymapModeFile = "keymap_mode.csv"
	keymapKeyFile  = "keymap_key.csv"
)

type keymapCSVRow struct {
	Mode  string
	Key   string
	Label string
}

func exportKeymapCSV(modePath, keyPath string) error {
	var rows []keymapCSVRow
	for k, kb := range GlobalKeys {
		rows = append(rows, keymapCSVRow{Mode: keymapGlobalMode, Key: k, Label: kb.Label})
	}
	for m, km := range ModeKeys {
		for k, kb := range km {
			rows = append(rows, keymapCSVRow{Mode: string(m), Key: k, Label: kb.Label})
		}
	}
	for _, reg := range DetailFieldLabels {
		rows = append(rows, keymapCSVRow{Mode: keymapDetailMode, Key: reg.JumpLabel, Label: string(reg.TextKey)})
	}

	// Write mode-sorted
	if err := writeCSV(modePath, rows, func(i, j int) bool {
		if rows[i].Mode != rows[j].Mode {
			mi, mj := strings.ToLower(rows[i].Mode), strings.ToLower(rows[j].Mode)
			if mi != mj {
				return mi < mj
			}
			return rows[i].Mode < rows[j].Mode
		}
		ki, kj := strings.ToLower(rows[i].Key), strings.ToLower(rows[j].Key)
		if ki != kj {
			return ki < kj
		}
		return rows[i].Key < rows[j].Key
	}); err != nil {
		return fmt.Errorf("mode file: %w", err)
	}

	// Write key-sorted
	if err := writeCSV(keyPath, rows, func(i, j int) bool {
		if rows[i].Key != rows[j].Key {
			ki, kj := strings.ToLower(rows[i].Key), strings.ToLower(rows[j].Key)
			if ki != kj {
				return ki < kj
			}
			return rows[i].Key < rows[j].Key
		}
		mi, mj := strings.ToLower(rows[i].Mode), strings.ToLower(rows[j].Mode)
		if mi != mj {
			return mi < mj
		}
		return rows[i].Mode < rows[j].Mode
	}); err != nil {
		return fmt.Errorf("key file: %w", err)
	}

	return nil
}

func writeCSV(path string, rows []keymapCSVRow, less func(i, j int) bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	type col struct{ mode, key, label string }
	all := make([]col, 0, 1+len(rows))
	all = append(all, col{"mode", "key", "label"})
	sort.Slice(rows, less)
	for _, r := range rows {
		all = append(all, col{r.Mode, r.Key, r.Label})
	}

	maxMode, maxKey, maxLabel := 0, 0, 0
	for _, c := range all {
		if len(c.mode) > maxMode {
			maxMode = len(c.mode)
		}
		if len(c.key) > maxKey {
			maxKey = len(c.key)
		}
		if len(c.label) > maxLabel {
			maxLabel = len(c.label)
		}
	}

	format := fmt.Sprintf("%%-%ds ,%%-%ds ,%%s\n", maxMode, maxKey)
	for _, c := range all {
		fmt.Fprintf(f, format, c.mode, c.key, c.label)
	}
	return nil
}

func registerKey(key string, modes []viewMode, label string, handler KeyHandler, dualUse bool, section string, help TextKey, showInHint bool) {
	kb := KeyBinding{Key: key, Label: label, Handler: handler, DualUse: dualUse, Section: section, Help: help, ShowInHint: showInHint}
	if modes == nil {
		if _, dup := GlobalKeys[key]; dup {
			panic(fmt.Sprintf("global key collision: %q already registered", key))
		}
		GlobalKeys[key] = kb
	} else {
		for _, mode := range modes {
			if ModeKeys[mode] == nil {
				ModeKeys[mode] = KeyMap{}
			}
			if _, dup := ModeKeys[mode][key]; dup {
				panic(fmt.Sprintf("mode %q key collision: %q already registered", mode, key))
			}
			ModeKeys[mode][key] = kb
		}
	}
}

// call at startup by init() to verify no collisions within any mode
func validateKeyBindings() error {
	var errs []error
	for mode, km := range ModeKeys {
		seen := map[string]string{}
		for key, kb := range km {
			if prev, ok := seen[key]; ok {
				errs = append(errs, fmt.Errorf("key %q collision in mode %q: %q vs %q", key, mode, prev, kb.Label))
			}
			seen[key] = kb.Label
			if !kb.DualUse {
				if gb, exists := GlobalKeys[key]; exists {
					errs = append(errs, fmt.Errorf("key %q in mode %q (%q) shadows global binding (%q). Set DualUse=true if intentional.", key, mode, kb.Label, gb.Label))
				}
			}
		}
	}
	return errors.Join(errs...)
}

func (m *Model) setMode(mode viewMode) {
	m.mode = mode
	m.activeKeys = make(KeyMap)
	for k, v := range GlobalKeys {
		m.activeKeys[k] = v
	}
	for k, v := range ModeKeys[mode] {
		m.activeKeys[k] = v
	}
	if spec, ok := modeSpecs[mode]; ok && spec.onActivate != nil {
		spec.onActivate(m)
	}
}

func init() {
	registerDetailField(jumpLabelAssignee, TextDetailAssignee, false)
	registerDetailField(jumpLabelLatestAssignment, TextDetailLatestAssignment, false)
	registerDetailField(jumpLabelInventors, TextDetailInventors, false)
	registerDetailField(jumpLabelApplication, TextDetailApplication, false)
	registerDetailField(jumpLabelPublication, TextDetailPublication, false)
	registerDetailField(jumpLabelGrant, TextDetailGrant, false)
	registerDetailField(jumpLabelExpiration, TextDetailExpiration, false)
	registerDetailField(jumpLabelClassification, TextDetailClassification, false)
	registerDetailField(ksCitations.jump, TextDetailCitationCount, false)
	registerDetailField(ksCitedBy.jump, TextDetailCitedByCount, false)
	registerDetailField(jumpLabelFamilyParents, TextDetailFamilyParents, true)
	registerDetailField(jumpLabelFamilyChildren, TextDetailFamilyChildren, true)
	registerDetailField(ksTags.jump, TextDetailTags, true)
	registerDetailField(jumpLabelFirstClaim, TextDetailFirstClaim, false)
	registerDetailField(jumpLabelAbstract, TextDetailAbstract, false)
	registerDetailField(ksNotes.jump, TextDetailNotes, false)
	registerDetailField(jumpLabelImportSource, TextDetailImportSource, false)
	registerDetailField(jumpLabelSource, TextDetailSource, false)
	registerDetailField(jumpLabelStoredLocal, TextDetailStoredLocal, false)
	registerDetailField(jumpLabelUpdated, TextDetailUpdated, false)
	registerDetailField(jumpLabelCountry, TextDetailCountry, true)

	registerKey(keyVimDown, nil, "move", nil, false, "Navigation", TextHelpMoveList, true)
	registerKey(keyVimUp, nil, "move", nil, false, "Navigation", TextHelpMoveList, true)
	registerKey(keyGoto, nil, "Go to (gg=top, gv=reselect)", nil, false, "", "", false)
	registerKey(keyBottom, nil, "Go to last", nil, false, "", "", false)
	registerKey(keyBack, nil, "back", nil, false, "Navigation", TextHelpBackOrQuit, true)
	registerKey(keyQuit, nil, "quit", nil, false, "Navigation", TextHelpQuitApp, true)
	registerKey(keyCommand, nil, "command", nil, false, "Navigation", "", true)
	registerKey(keySearch, nil, "search", nil, false, "Navigation", TextHelpSearchHint, true)
	registerKey(keyHelp, nil, "help", nil, false, "Navigation", TextHelpShortcutShowHelp, true)
	registerKey(keyFullText, nil, "Text view", nil, false, "Views", TextHelpJumpText, false)
	registerKey(keyTag, nil, "Tag selector", nil, false, "Views", TextHelpJumpTags, false)
	registerKey(keyCites, nil, "Citations view", nil, false, "Views", TextHelpJumpCitations, false)
	registerKey(keyCitedBy, nil, "Cited-by view", nil, false, "Views", TextHelpJumpCitedBy, false)
	registerKey(keyFamily, nil, "Family view", nil, false, "Views", TextHelpFamilyView, false)
	registerKey(keyClassification, nil, "Classifications view", nil, false, "Views", TextHelpJumpClassification, false)
	registerKey(keyNotes, nil, "Notes / search next / cancel", nil, false, "Views", TextHelpJumpNotes, false) // "n" shared by keyNotes/keyNo/keyNew
	registerKey(keyRefresh, nil, "refresh", nil, false, "Views", TextHelpJumpRefs, true)
	registerKey(keyAI, nil, "AI view", nil, false, "Views", TextHelpJumpAI, false)
	registerKey(keyWeb, nil, "Open browser", nil, false, "General", TextHelpOpenBrowser, false)
	registerKey(keyDelete, nil, "Delete", nil, false, "Patents", TextHelpDeletePatent, false)
	registerKey(keyReviewState, nil, "status", nil, false, "Patents", TextHelpCyclePatentReviewState, true)
	registerKey(keyCountry, nil, "Country filter", nil, false, "Patents", TextHelpCountryFilter, false)
	registerKey(keyOpen, nil, "open", nil, false, "Navigation", TextHelpOpenSelected, true)
	registerKey(keyFirstClaim, nil, "First claim", nil, false, "Views", "", false)
	registerKey(keyEditSummary, nil, "Abstract / summary", nil, false, "Views", "", false)
	registerKey(keyJump, nil, "Toggle jump mode", nil, false, "Navigation", TextNavJump, false)
	registerKey(keyProject, nil, "Project splash", nil, false, "Navigation", TextHelpJumpProject, false)
	registerKey(keyIDS, nil, "IDS view", nil, false, "Views", TextHelpProjectIDS, false)
	registerKey(keyAddToIDS, nil, "add", nil, false, "Patents", TextHelpProjectIDSAdd, true)
	registerKey(keyNoteEdit, nil, "note", nil, false, "Patents", TextHelpNote, true)
	registerKey(keyCtrlS, []viewMode{viewNoteEdit}, "save", nil, false, "Note", TextHelpNoteSave, true)
	registerKey(keyRefreshAll, nil, "refresh", nil, false, "General", "", true)
	registerKey(keyYes, nil, "save", nil, false, "General", "", true)
	registerKey(keyEvents, nil, "Project events", nil, false, "Views", TextHelpProjectEvents, false)
	registerKey(keyInvoices, nil, "ignore", nil, false, "Views", TextHelpProjectInvoices, true) // "i" shared by keyInvoices/keyIgnore/keyProjectInfo
	registerKey(keyMarkPaid, nil, "Mark paid", nil, false, "General", TextHelpMarkPaid, false)
	registerKey(keyUnreview, nil, "review", nil, false, "Patents", "", true)
	registerKey(keyUndo, nil, "undo", nil, false, "General", TextHelpUndo, false)
	registerKey(keyRedo, nil, "redo", nil, false, "General", TextHelpRedo, false)

	// viewProjectInfo overrides
	registerKey(keyEditAppStatus, []viewMode{viewProjectInfo}, "status", nil, true, "Project", TextHelpProjectSummaryStatus, true)
	registerKey(keyEditComment, []viewMode{viewProjectInfo}, "Edit comment", nil, true, "Project", TextHelpProjectComment, false)
	registerKey(keyEditProjectStatus, []viewMode{viewProjectInfo}, "Edit project status", nil, false, "Project", TextHelpProjectStatus, false)

	// viewIDSEdit overrides
	registerKey(keyEditAppStatus, []viewMode{viewIDSEdit}, "status", nil, true, "IDS", "", true)
	registerKey(keyNotes, []viewMode{viewIDSEdit}, "Edit note", nil, true, "IDS", "", false)
	registerKey(keyVimUp, []viewMode{viewIDSEdit}, "Edit kind code", nil, true, "IDS", "", false)
	registerKey(keyEditComment, []viewMode{viewIDSEdit}, "Edit country code", nil, true, "IDS", "", false)
	registerKey(keyMarkPaid, []viewMode{viewIDSEdit}, "Edit passages", nil, true, "IDS", "", false)
	registerKey(keyJump, []viewMode{viewIDSEdit}, "Toggle in-full", nil, true, "IDS", "", false)

	// viewTagSelect
	registerKey(keySpace, []viewMode{viewTagSelect}, "select", nil, true, "Tags", "", true)
	registerKey(keyAI, []viewMode{viewTagSelect}, "add", nil, true, "Tags", "", true)
	registerKey(keyEnter, []viewMode{viewTagSelect}, "save", nil, false, "Tags", "", true)
	registerKey(keyCtrlS, []viewMode{viewTagSelect}, "save", nil, false, "Tags", "", true)

	// viewProjectTags
	registerKey(keyUnreview, []viewMode{viewProjectTags}, "rename", nil, true, "Tags", "", true)

	// viewFamily overrides
	registerKey(keyPlus, []viewMode{viewFamily}, "add child", nil, true, "Family", TextHelpFamilyAddChild, true)

	if err := validateKeyBindings(); err != nil {
		panic(fmt.Sprintf("key binding validation failed:\n%s", err))
	}
	if err := validateDetailFieldLabels(); err != nil {
		panic(fmt.Sprintf("detail field label validation failed:\n%s", err))
	}
}
