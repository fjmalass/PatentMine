// Package changes models every data mutation as a Change value applied through
// a single History. History is the one chokepoint for writes: it runs the
// change, records it for undo/redo, and reports which views must refresh.
//
// The package is transport-agnostic — it depends only on the storage and
// domain packages, never on any UI toolkit — so a future web frontend can
// reuse the same Change set and map Scope to its own invalidation.
package changes

import (
	"context"
	"errors"
	"fmt"

	"patentmine/internal/storage"
)

// Scope reports which views are stale after a change is applied.
type Scope uint8

const (
	ScopeList Scope = 1 << iota
	ScopeDetail
	ScopeFamily
	ScopeTags
)

// Change is a single reversible data mutation. The methods are unexported so
// only this package can define or run changes; callers build one with a
// builder (SetPatentReviewState, AddNote, ...) and apply it through History.
type Change interface {
	// run performs the change against the repository.
	run(ctx context.Context, repo storage.Repository) error
	// reverse returns the change that undoes this one, or nil if the change
	// cannot be undone. It is only valid to call after run has succeeded.
	reverse() Change
	// affects reports which views are stale once the change is applied.
	affects() Scope
	// label is a short human description, e.g. "review 3 patents".
	label() string
	// kind is the stable tag used to serialize the change in a recording.
	kind() string
}

var (
	ErrNothingToUndo = errors.New("nothing to undo")
	ErrNothingToRedo = errors.New("nothing to redo")
	ErrNotReversible = errors.New("this change cannot be undone")
)

// History applies changes and tracks them for undo/redo. It also keeps an
// ordered log of every change actually run — the session recording.
type History struct {
	repo storage.Repository
	undo []Change
	redo []Change
	log  []Change
}

func NewHistory(repo storage.Repository) *History {
	return &History{repo: repo}
}

// Apply runs the change, then records it so it can be undone. A fresh apply
// clears the redo stack. It returns the views made stale by the change.
func (h *History) Apply(ctx context.Context, c Change) (Scope, error) {
	if err := c.run(ctx, h.repo); err != nil {
		return 0, err
	}
	h.undo = append(h.undo, c)
	h.redo = h.redo[:0]
	h.log = append(h.log, c)
	return c.affects(), nil
}

func (h *History) CanUndo() bool { return len(h.undo) > 0 }
func (h *History) CanRedo() bool { return len(h.redo) > 0 }

// UndoLabel describes the change that Undo would reverse.
func (h *History) UndoLabel() string {
	if len(h.undo) == 0 {
		return ""
	}
	return h.undo[len(h.undo)-1].label()
}

// RedoLabel describes the change that Redo would re-apply.
func (h *History) RedoLabel() string {
	if len(h.redo) == 0 {
		return ""
	}
	return h.redo[len(h.redo)-1].label()
}

// Undo reverses the most recently applied change. The reversed change moves to
// the redo stack. It returns the views made stale and a description of the
// change that was undone.
func (h *History) Undo(ctx context.Context) (Scope, string, error) {
	if len(h.undo) == 0 {
		return 0, "", ErrNothingToUndo
	}
	c := h.undo[len(h.undo)-1]
	rev := c.reverse()
	if rev == nil {
		return 0, "", fmt.Errorf("%w: %s", ErrNotReversible, c.label())
	}
	if err := rev.run(ctx, h.repo); err != nil {
		return 0, "", err
	}
	h.undo = h.undo[:len(h.undo)-1]
	h.redo = append(h.redo, c)
	h.log = append(h.log, rev)
	return c.affects() | rev.affects(), c.label(), nil
}

// Redo re-applies the most recently undone change.
func (h *History) Redo(ctx context.Context) (Scope, string, error) {
	if len(h.redo) == 0 {
		return 0, "", ErrNothingToRedo
	}
	c := h.redo[len(h.redo)-1]
	if err := c.run(ctx, h.repo); err != nil {
		return 0, "", err
	}
	h.redo = h.redo[:len(h.redo)-1]
	h.undo = append(h.undo, c)
	h.log = append(h.log, c)
	return c.affects(), c.label(), nil
}

// RecordingLen is the number of changes run so far this session.
func (h *History) RecordingLen() int { return len(h.log) }

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
