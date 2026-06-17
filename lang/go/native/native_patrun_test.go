package native

import (
	"fmt"
	"strconv"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
)

// Performance characterisation for patrun. The matcher is a flat scan: `find`
// inspects EVERY rule (no early exit — a later rule may be more specific), so
// a lookup is O(n × keys-per-rule). These benchmarks pin that scaling and the
// per-find allocation cost so a future trie optimisation (patrun's real
// O(query-depth) structure) can be measured against a baseline.

// buildBenchPatrun makes a matcher with n distinct 2-key rules
// {cmd:"c<i>" role:"r<i>"} → "v<i>" (keys pre-sorted, as coercePattern leaves
// them).
func buildBenchPatrun(n int) *patrunMatcher {
	m := &patrunMatcher{}
	for i := 0; i < n; i++ {
		s := strconv.Itoa(i)
		m.entries = append(m.entries, &patrunEntry{
			keys: []string{"cmd", "role"},
			pat:  map[string]string{"cmd": "c" + s, "role": "r" + s},
			val:  NewString("v" + s),
		})
	}
	return m
}

// BenchmarkPatrunFindMatcher — the core match, no AQL-value overhead: one
// find over a table of n rules, subject matching the LAST rule plus an extra
// (ignored) key. The flat scan visits all n regardless of match position.
func BenchmarkPatrunFindMatcher(b *testing.B) {
	for _, n := range []int{1, 10, 100, 1000} {
		m := buildBenchPatrun(n)
		last := strconv.Itoa(n - 1)
		subj := map[string]string{"role": "r" + last, "cmd": "c" + last, "x": "extra"}
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if patrunBestMatch(m, subj, false) == nil {
					b.Fatal("expected a match")
				}
			}
		})
	}
}

// BenchmarkPatrunFindMiss — worst case: no rule matches, so every entry is
// inspected and rejected.
func BenchmarkPatrunFindMiss(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		m := buildBenchPatrun(n)
		subj := map[string]string{"role": "none", "cmd": "none"}
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if patrunBestMatch(m, subj, false) != nil {
					b.Fatal("expected no match")
				}
			}
		})
	}
}

// BenchmarkPatrunFindHandler — end-to-end `find`: a subject Value through
// coerceSubject (which allocates a key→string map per call) + the match +
// the result, the cost a `find {subj} pm` pays minus parse / word dispatch.
func BenchmarkPatrunFindHandler(b *testing.B) {
	r, err := DefaultRegistry()
	if err != nil {
		b.Fatal(err)
	}
	for _, n := range []int{10, 100, 1000} {
		pm := eng.NewExtension(TPatrun, buildBenchPatrun(n))
		last := strconv.Itoa(n - 1)
		subjOM := NewOrderedMap()
		subjOM.Set("role", NewString("r"+last))
		subjOM.Set("cmd", NewString("c"+last))
		subjOM.Set("x", NewString("extra"))
		args := []Value{NewMap(subjOM), pm}
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := patrunFindHandler(args, nil, nil, r); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkPatrunAdd — building a table of 100 rules via the add handler.
// add scans the existing entries to overwrite a duplicate pattern, so a build
// is O(n²); this measures the full coercion + dedupe + append cost.
func BenchmarkPatrunAdd(b *testing.B) {
	r, err := DefaultRegistry()
	if err != nil {
		b.Fatal(err)
	}
	const n = 100
	patterns := make([]Value, n)
	for i := 0; i < n; i++ {
		s := strconv.Itoa(i)
		om := NewOrderedMap()
		om.Set("role", NewString("r"+s))
		om.Set("cmd", NewString("c"+s))
		patterns[i] = NewMap(om)
	}
	val := NewString("v")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pm := NewPatrun()
		for j := 0; j < n; j++ {
			if _, err := patrunAddHandler([]Value{patterns[j], val, pm}, nil, nil, r); err != nil {
				b.Fatal(err)
			}
		}
	}
}
