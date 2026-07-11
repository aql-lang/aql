package eng

import (
	"sort"
	"strings"
)

// This file owns the did-you-mean engine (design/DIAGNOSTICS.0.md,
// phase 4): near-miss name suggestions for undefined words, unknown
// type names, and strict-read key misses. It runs ONLY on failure
// paths — candidate sets are enumerated after an error is already
// certain, never during dispatch.

// suggestMaxResults caps how many near-misses one error offers.
const suggestMaxResults = 3

// suggestDistanceCap scales the edit-distance budget with the typed
// name's length: a short name tolerates one edit, a mid-length name
// two, anything longer three. Below the cap almost every suggestion is
// the intended word; above it they degenerate into noise.
func suggestDistanceCap(name string) int {
	switch n := len(name); {
	case n <= 4:
		return 1
	case n <= 8:
		return 2
	default:
		return 3
	}
}

// SuggestNames returns up to suggestMaxResults candidate names closest
// to name under the OSA (optimal string alignment) Damerau-Levenshtein
// distance, within suggestDistanceCap edits. Case-only misspellings
// (`upper` vs `Upper`) rank first, then by distance, then
// lexicographically — a deterministic order for stable diagnostics.
// name itself is never suggested.
func SuggestNames(name string, candidates []string) []string {
	if name == "" || len(candidates) == 0 {
		return nil
	}
	budget := suggestDistanceCap(name)
	lower := strings.ToLower(name)
	type scored struct {
		name     string
		dist     int
		caseOnly bool
	}
	var hits []scored
	for _, c := range candidates {
		if c == name || c == "" {
			continue
		}
		// Length band: |len(c) - len(name)| is a lower bound on the
		// distance, so anything outside the cap cannot qualify.
		if d := len(c) - len(name); d > budget || d < -budget {
			continue
		}
		if strings.ToLower(c) == lower {
			hits = append(hits, scored{name: c, dist: 0, caseOnly: true})
			continue
		}
		d, ok := osaDistanceWithin(lower, strings.ToLower(c), budget)
		if !ok {
			continue
		}
		hits = append(hits, scored{name: c, dist: d})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].caseOnly != hits[j].caseOnly {
			return hits[i].caseOnly
		}
		if hits[i].dist != hits[j].dist {
			return hits[i].dist < hits[j].dist
		}
		return hits[i].name < hits[j].name
	})
	if len(hits) == 0 {
		return nil
	}
	if len(hits) > suggestMaxResults {
		hits = hits[:suggestMaxResults]
	}
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.name
	}
	return out
}

// osaDistanceWithin computes the OSA Damerau-Levenshtein distance
// between a and b (substitution, insertion, deletion, and adjacent
// transposition, each cost 1), bailing out early — ok=false — as soon
// as the distance provably exceeds the budget (banded rows: when every cell
// in a row exceeds cap, no later row can come back under it).
func osaDistanceWithin(a, b string, budget int) (int, bool) {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb, lb <= budget
	}
	if lb == 0 {
		return la, la <= budget
	}
	prev2 := make([]int, lb+1) // row i-2
	prev := make([]int, lb+1)  // row i-1
	cur := make([]int, lb+1)   // row i
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d := prev[j] + 1 // deletion
			if ins := cur[j-1] + 1; ins < d {
				d = ins // insertion
			}
			if sub := prev[j-1] + cost; sub < d {
				d = sub // substitution / match
			}
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				if tr := prev2[j-2] + 1; tr < d {
					d = tr // adjacent transposition
				}
			}
			cur[j] = d
			if d < rowMin {
				rowMin = d
			}
		}
		if rowMin > budget {
			return 0, false
		}
		prev2, prev, cur = prev, cur, prev2
	}
	d := prev[lb]
	return d, d <= budget
}

// SuggestionCandidates enumerates every name a bare word could have
// meant in this registry: dispatchable words (natives + host words +
// live defs, via RegisteredWordNames) plus the kernel type names —
// so a misspelt TYPE (`Intger`) suggests its type as readily as a
// misspelt word. Built lazily on the failure path only.
func (r *Registry) SuggestionCandidates() []string {
	if r == nil {
		return nil
	}
	names := r.RegisteredWordNames()
	for name := range typeNames {
		names = append(names, name)
	}
	// The literal keywords stepWord resolves inline (they are not
	// registered words, but a bare `treu` should still suggest `true`).
	names = append(names, "true", "false", "none", "null")
	return names
}
