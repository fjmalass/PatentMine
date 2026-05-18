// Package text is the UI string catalog. Every user-facing string is named by
// a Key and resolved through a Catalog, so the application is translated by
// swapping the catalog at startup rather than by editing call sites. A Key with
// no catalog entry resolves to a visibly-marked placeholder, and the wiring
// check fails the boot when a command's strings are missing — a gap surfaces
// loudly instead of shipping a blank label.
package text

import (
	"fmt"
	"maps"
)

// Key names one catalog entry.
type Key string

// CmdTitle is the Key for a command's short label, derived from its ID so the
// command package carries no display strings of its own.
func CmdTitle(id string) Key { return Key("cmd." + id + ".title") }

// CmdHelp is the Key for a command's one-line description.
func CmdHelp(id string) Key { return Key("cmd." + id + ".help") }

// Catalog resolves Keys to display strings for one locale. It is built once at
// startup and injected; there is no package-level catalog state.
type Catalog struct {
	locale  string
	entries map[Key]string
}

// New builds a catalog for a locale from an entry table. The table is copied,
// so the caller may reuse or discard the map afterwards.
func New(locale string, entries map[Key]string) *Catalog {
	cloned := make(map[Key]string, len(entries))
	maps.Copy(cloned, entries)
	return &Catalog{locale: locale, entries: cloned}
}

// Locale reports the catalog's locale tag.
func (c *Catalog) Locale() string { return c.locale }

// Has reports whether the catalog defines key.
func (c *Catalog) Has(key Key) bool {
	_, ok := c.entries[key]
	return ok
}

// Keys returns every defined key, unordered.
func (c *Catalog) Keys() []Key {
	out := make([]Key, 0, len(c.entries))
	for k := range c.entries {
		out = append(out, k)
	}
	return out
}

// T resolves key to its display string. An undefined key resolves to the key
// wrapped in guillemets so the gap is visible on screen rather than blank.
func (c *Catalog) T(key Key) string {
	if v, ok := c.entries[key]; ok {
		return v
	}
	return "«" + string(key) + "»"
}

// Tf resolves key as a printf-style template and applies args.
func (c *Catalog) Tf(key Key, args ...any) string {
	return fmt.Sprintf(c.T(key), args...)
}
