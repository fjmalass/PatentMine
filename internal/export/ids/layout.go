package ids

import (
	"sort"
	"strings"
	"time"

	"patentmine/internal/domain"
)

// Entry is one curated prior-art reference, flattened for PDF rendering.
type Entry struct {
	Number   domain.PatentNumber
	Patentee string
	PubDate  time.Time
	Passages string
	InFull   bool
}

// Bucket splits entries into the two sections supported on PTO/SB/08a:
// US (any kind code — granted patents and published applications both go in
// the US PATENT DOCUMENTS table on 08a) and Foreign.
//
// Each bucket is sorted by publication date descending, then by normalized
// number for a stable order.
func Bucket(entries []Entry) (us, foreign []Entry) {
	for _, e := range entries {
		if strings.EqualFold(e.Number.Country, "US") || e.Number.Country == "" {
			us = append(us, e)
			continue
		}
		foreign = append(foreign, e)
	}
	sortEntries(us)
	sortEntries(foreign)
	return us, foreign
}

func sortEntries(es []Entry) {
	sort.SliceStable(es, func(i, j int) bool {
		if !es[i].PubDate.Equal(es[j].PubDate) {
			return es[i].PubDate.After(es[j].PubDate)
		}
		return es[i].Number.Normalized() < es[j].Number.Normalized()
	})
}

// Sheet is one rendered page worth of entries from one bucket.
type Sheet struct {
	US      []Entry // up to sb08aUSRows
	Foreign []Entry // up to sb08aForeignRows
}

// Paginate spreads bucketed entries across sheets such that each sheet holds
// at most sb08aUSRows US entries and at most sb08aForeignRows foreign entries.
// At least one sheet is always returned.
func Paginate(us, foreign []Entry) []Sheet {
	usPages := chunkUS(us)
	frPages := chunkForeign(foreign)
	n := max1(len(usPages), len(frPages))
	out := make([]Sheet, n)
	for i := range n {
		if i < len(usPages) {
			out[i].US = usPages[i]
		}
		if i < len(frPages) {
			out[i].Foreign = frPages[i]
		}
	}
	return out
}

func chunkUS(entries []Entry) [][]Entry { return chunk(entries, sb08aUSRows) }

func chunkForeign(entries []Entry) [][]Entry { return chunk(entries, sb08aForeignRows) }

func chunk(es []Entry, size int) [][]Entry {
	if len(es) == 0 {
		return nil
	}
	var out [][]Entry
	for i := 0; i < len(es); i += size {
		end := min(i+size, len(es))
		out = append(out, es[i:end])
	}
	return out
}

func max1(a, b int) int {
	if a == 0 && b == 0 {
		return 1
	}
	if a > b {
		return a
	}
	return b
}

// FeeTier returns the 37 CFR 1.17(v) tier for a cumulative-item count:
// 0 = no fee (≤50), 1 = (v)(1) (51–100), 2 = (v)(2) (101–200), 3 = (v)(3) (>200).
func FeeTier(count int) int {
	switch {
	case count <= 50:
		return 0
	case count <= 100:
		return 1
	case count <= 200:
		return 2
	default:
		return 3
	}
}
