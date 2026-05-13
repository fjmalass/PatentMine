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
	Key     string
	Label   string
	Handler KeyHandler
	DualUse bool // intentional global key shadow, suppress warning
}

type KeyMap map[string]KeyBinding

var (
	globalKeys = KeyMap{}
	modeKeys   = map[viewMode]KeyMap{}
	detailFieldLabels []detailFieldLabelReg
)

type detailFieldLabelReg struct {
	JumpLabel string
	TextKey   TextKey
	Optional  bool // only present under conditions (e.g. p.Number != "")
}

func registerDetailField(jumpLabel string, textKey TextKey, optional bool) {
	for _, r := range detailFieldLabels {
		if r.JumpLabel == jumpLabel {
			panic(fmt.Sprintf("detail field jump label %q already registered for %q", jumpLabel, r.TextKey))
		}
	}
	detailFieldLabels = append(detailFieldLabels, detailFieldLabelReg{
		JumpLabel: jumpLabel,
		TextKey:   textKey,
		Optional:  optional,
	})
}

func validateDetailFieldLabels() error {
	var errs []error
	for _, reg := range detailFieldLabels {
		if reg.JumpLabel == "" {
			continue
		}
	}
	return errors.Join(errs...)
}

// DetailFieldLabelRegistrations returns registered detail field jump labels for test validation.
func DetailFieldLabelRegistrations() []detailFieldLabelReg {
	r := make([]detailFieldLabelReg, len(detailFieldLabels))
	copy(r, detailFieldLabels)
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
	for k, kb := range globalKeys {
		rows = append(rows, keymapCSVRow{Mode: keymapGlobalMode, Key: k, Label: kb.Label})
	}
	for m, km := range modeKeys {
		for k, kb := range km {
			rows = append(rows, keymapCSVRow{Mode: string(m), Key: k, Label: kb.Label})
		}
	}
	for _, reg := range detailFieldLabels {
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

func registerKey(key string, modes []viewMode, label string, handler KeyHandler, dualUse bool) {
	kb := KeyBinding{Key: key, Label: label, Handler: handler, DualUse: dualUse}
	if modes == nil {
		if _, dup := globalKeys[key]; dup {
			panic(fmt.Sprintf("global key collision: %q already registered", key))
		}
		globalKeys[key] = kb
	} else {
		for _, mode := range modes {
			if modeKeys[mode] == nil {
				modeKeys[mode] = KeyMap{}
			}
			if _, dup := modeKeys[mode][key]; dup {
				panic(fmt.Sprintf("mode %q key collision: %q already registered", mode, key))
			}
			modeKeys[mode][key] = kb
		}
	}
}

// call at startup by init() to verify no collisions within any mode
func validateKeyBindings() error {
	var errs []error
	for mode, km := range modeKeys {
		seen := map[string]string{}
		for key, kb := range km {
			if prev, ok := seen[key]; ok {
				errs = append(errs, fmt.Errorf("key %q collision in mode %q: %q vs %q", key, mode, prev, kb.Label))
			}
			seen[key] = kb.Label
			if !kb.DualUse {
				if gb, exists := globalKeys[key]; exists {
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
	for k, v := range globalKeys {
		m.activeKeys[k] = v
	}
	for k, v := range modeKeys[mode] {
		m.activeKeys[k] = v
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
	registerDetailField(jumpLabelCitationCount, TextDetailCitationCount, false)
	registerDetailField(jumpLabelCitedByCount, TextDetailCitedByCount, false)
	registerDetailField(jumpLabelFamilyParents, TextDetailFamilyParents, true)
	registerDetailField(jumpLabelFamilyChildren, TextDetailFamilyChildren, true)
	registerDetailField(jumpLabelTags, TextDetailTags, true)
	registerDetailField(jumpLabelFirstClaim, TextDetailFirstClaim, false)
	registerDetailField(jumpLabelAbstract, TextDetailAbstract, false)
	registerDetailField(jumpLabelNotes, TextDetailNotes, false)
	registerDetailField(jumpLabelImportSource, TextDetailImportSource, false)
	registerDetailField(jumpLabelSource, TextDetailSource, false)
	registerDetailField(jumpLabelStoredLocal, TextDetailStoredLocal, false)
	registerDetailField(jumpLabelUpdated, TextDetailUpdated, false)

	registerKey(keyVimDown, nil, "Move down", nil, false)
	registerKey(keyVimUp, nil, "Move up", nil, false)
	registerKey(keyGoto, nil, "Go to (gg=top)", nil, false)
	registerKey(keyBottom, nil, "Go to last", nil, false)
	registerKey(keyBack, nil, "Go back", nil, false)
	registerKey(keyQuit, nil, "Quit", nil, false)
	registerKey(keyCommand, nil, "Command bar", nil, false)
	registerKey(keySearch, nil, "Search", nil, false)
	registerKey(keyHelp, nil, "Help", nil, false)
	registerKey(keyText, nil, "Text view", nil, false)
	registerKey(keyTag, nil, "Tag selector", nil, false)
	registerKey(keyCites, nil, "Citations view", nil, false)
	registerKey(keyCitedBy, nil, "Cited-by view", nil, false)
	registerKey(keyFamily, nil, "Family view", nil, false)
	registerKey(keyClassification, nil, "Classifications view", nil, false)
	registerKey(keyNotes, nil, "Notes / search next / cancel", nil, false) // "n" shared by keyNotes/keyNo/keyNew
	registerKey(keyRefs, nil, "References / refresh", nil, false)
	registerKey(keyAI, nil, "AI view", nil, false)
	registerKey(keyWeb, nil, "Open browser", nil, false)
	registerKey(keyDelete, nil, "Delete", nil, false)
	registerKey(keyStatus, nil, "Status", nil, false)
	registerKey(keyOpen, nil, "Open / select", nil, false)
	registerKey(keyFirstClaim, nil, "First claim", nil, false)
	registerKey(keyEditSummary, nil, "Abstract / summary", nil, false)
	registerKey(keyJump, nil, "Toggle jump mode", nil, false)
	registerKey(keyProject, nil, "Project splash", nil, false)
	registerKey(keyIDS, nil, "IDS view", nil, false)
	registerKey(keyAddToIDS, nil, "Add to IDS", nil, false)
	registerKey(keyNoteEdit, nil, "Note editor", nil, false)
	registerKey(keyRefreshAll, nil, "Refresh all", nil, false)
	registerKey(keyYes, nil, "Yes / store", nil, false)
	registerKey(keyEvents, nil, "Project events", nil, false)
	registerKey(keyInvoices, nil, "Project invoices / ignore", nil, false) // "i" shared by keyInvoices/keyIgnore/keyProjectInfo
	registerKey(keyMarkPaid, nil, "Mark paid", nil, false)
	registerKey(keyUnreview, nil, "Under review", nil, false)

	// viewProjectInfo overrides
	registerKey(keyEditAppStatus, []viewMode{viewProjectInfo}, "Edit summary status", nil, true)
	registerKey(keyEditComment, []viewMode{viewProjectInfo}, "Edit comment", nil, true)
	registerKey(keyEditProjectStatus, []viewMode{viewProjectInfo}, "Edit project status", nil, false)

	// viewIDSEdit overrides
	registerKey("s", []viewMode{viewIDSEdit}, "Cycle IDS status", nil, true)
	registerKey("n", []viewMode{viewIDSEdit}, "Edit note", nil, true)
	registerKey("k", []viewMode{viewIDSEdit}, "Edit kind code", nil, true)
	registerKey("c", []viewMode{viewIDSEdit}, "Edit country code", nil, true)
	registerKey("p", []viewMode{viewIDSEdit}, "Edit passages", nil, true)
	registerKey("f", []viewMode{viewIDSEdit}, "Toggle in-full", nil, true)

	// viewTagSelect
	registerKey(" ", []viewMode{viewTagSelect}, "Toggle tag", nil, true)
	registerKey("a", []viewMode{viewTagSelect}, "Add tag", nil, true)
	registerKey(keyEnter, []viewMode{viewTagSelect}, "Toggle tag", nil, false)

	// viewProjectTags
	registerKey("r", []viewMode{viewProjectTags}, "Rename tag", nil, true)

	if err := validateKeyBindings(); err != nil {
		panic(fmt.Sprintf("key binding validation failed:\n%s", err))
	}
	if err := validateDetailFieldLabels(); err != nil {
		panic(fmt.Sprintf("detail field label validation failed:\n%s", err))
	}
}
