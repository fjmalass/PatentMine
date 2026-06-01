// Package patentlist reads and writes the plain-text patent list exchanged by
// the :add.file and :export.added commands (and their REST equivalents). The
// format is one patent number per line, with a mandatory magic header so a file
// of the wrong kind cannot be fed to the importer by accident.
//
// Example:
//
//	# patentmine added-patents v1
//	# project: Acme Prior Art
//	# exported: 2026-05-31
//	US11611785B2
//	EP1234567A1
package patentlist

import (
	"bufio"
	"fmt"
	"sort"
	"strings"

	"patentmine/internal/domain"
)

// Header is the magic first line every patent-list file must carry. The trailing
// version token lets the format evolve without silently misreading old files.
// It is the single source of truth for the marker; no other file spells it.
const Header = "# patentmine added-patents v1"

// commentPrefix marks informational and ignorable lines.
const commentPrefix = "#"

// Parse reads list content, verifying the magic Header, and returns the patent
// numbers in file order. Blank lines and comment lines (those starting with
// '#') are ignored. A missing or mismatched header, or any unparseable number,
// is an error so a malformed or wrong-kind file fails loudly rather than
// importing garbage.
func Parse(content string) ([]domain.PatentNumber, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	headerSeen := false
	var numbers []domain.PatentNumber
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if !headerSeen {
			if raw == "" {
				continue
			}
			if raw != Header {
				return nil, fmt.Errorf("patentlist: missing or wrong header on line %d: want %q", line, Header)
			}
			headerSeen = true
			continue
		}
		if raw == "" || strings.HasPrefix(raw, commentPrefix) {
			continue
		}
		number, err := domain.ParsePatentNumber(raw)
		if err != nil {
			return nil, fmt.Errorf("patentlist: line %d: %w", line, err)
		}
		numbers = append(numbers, number)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("patentlist: read: %w", err)
	}
	if !headerSeen {
		return nil, fmt.Errorf("patentlist: missing header %q", Header)
	}
	return numbers, nil
}

// Format renders a patent list with the magic Header followed by informational
// project and date comments and one normalized patent number per line, sorted
// for a stable, diff-friendly round trip. project and date may be empty, in
// which case their comment lines are omitted.
func Format(numbers []domain.PatentNumber, project, date string) string {
	sorted := make([]domain.PatentNumber, len(numbers))
	copy(sorted, numbers)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Normalized() < sorted[j].Normalized()
	})

	var b strings.Builder
	b.WriteString(Header)
	b.WriteByte('\n')
	if project != "" {
		fmt.Fprintf(&b, "# project: %s\n", project)
	}
	if date != "" {
		fmt.Fprintf(&b, "# exported: %s\n", date)
	}
	for _, n := range sorted {
		b.WriteString(n.String())
		b.WriteByte('\n')
	}
	return b.String()
}
