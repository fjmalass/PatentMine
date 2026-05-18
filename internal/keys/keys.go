// Package keys models keyboard input independently of any UI framework. It
// provides typed key constants, the Chord type, and a Reader that composes raw
// key presses into Vim-style chords. It deliberately knows nothing about the
// keymap or commands, so the same input model serves the TUI, an SSH frontend,
// and a Neovim integration.
package keys

// Key is a single key press in the string form a terminal reports it. The
// values match bubbletea's KeyMsg.String() so a frontend can convert directly.
type Key string

// Named keys. Printable keys (letters, digits, punctuation) are simply
// Key("j"), Key("/"), and so on — they need no constant.
const (
	Enter     Key = "enter"
	Escape    Key = "esc"
	Space     Key = " "
	Tab       Key = "tab"
	Backspace Key = "backspace"
	Up        Key = "up"
	Down      Key = "down"
	Left      Key = "left"
	Right     Key = "right"
	PageUp    Key = "pgup"
	PageDown  Key = "pgdown"
	Home      Key = "home"
	End       Key = "end"
	CtrlC     Key = "ctrl+c"
	CtrlD     Key = "ctrl+d"
	CtrlU     Key = "ctrl+u"
	CtrlF     Key = "ctrl+f"
	CtrlB     Key = "ctrl+b"
	CtrlR     Key = "ctrl+r"
)

// Match classifies how a candidate key sequence relates to the bound sequences
// of the active keymap. The Reader uses it to decide whether to emit a chord,
// wait for more keys, or discard unrecognised input.
type Match int

const (
	// NoMatch: the sequence matches no binding and cannot grow into one.
	NoMatch Match = iota
	// Prefix: the sequence is a proper prefix of at least one binding.
	Prefix
	// Complete: the sequence is exactly a binding.
	Complete
	// CompleteAndPrefix: the sequence is a binding and also a prefix of a
	// longer one. A well-formed keymap avoids this; the Reader treats it as
	// Complete so input stays deterministic.
	CompleteAndPrefix
)
