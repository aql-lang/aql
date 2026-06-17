package native

import (
	"fmt"
	"strconv"
	"testing"
)

// Performance: patrun's trie find is O(query-key-depth) — independent of table
// size. These benchmark the real (vendored) matcher backing the Patrun type,
// end-to-end through the find handler (subject coercion + trie + side lookup).

func buildBenchPatrun(b *testing.B, n int) (Value, *Registry) {
	r, err := DefaultRegistry()
	if err != nil {
		b.Fatal(err)
	}
	pm := NewPatrun()
	for i := 0; i < n; i++ {
		s := strconv.Itoa(i)
		om := NewOrderedMap()
		om.Set("role", NewString("r"+s))
		om.Set("cmd", NewString("c"+s))
		if _, err := patrunAddHandler([]Value{NewMap(om), NewString("v" + s), pm}, nil, nil, r); err != nil {
			b.Fatal(err)
		}
	}
	return pm, r
}

// BenchmarkPatrunFind — find over n ∈ {1..10000} rules; with the trie this is
// flat in n (the win over a linear scan).
func BenchmarkPatrunFind(b *testing.B) {
	for _, n := range []int{1, 10, 100, 1000, 10000} {
		pm, r := buildBenchPatrun(b, n)
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
