package engine

import (
	"fmt"
	"sort"

	"github.com/aql-lang/aql/eng/go"
)

// sort VALUE
//
// Reorder a List or Map ascending by each element's natural order.
// The comparison dispatches through the kernel's Comparer capability
// (eng.CompareValues), so:
//
//   - Lists of homogeneous scalars sort numerically / lexically /
//     by atom-name as appropriate (Integer / Decimal / String /
//     Boolean / Atom kernel Comparers).
//   - Lists of domain values (Date / DateTime / Instant /
//     ClkDuration) sort chronologically via their native Comparers.
//   - Lists of user-typed instances sort using the comparator
//     installed via `behave compare/q (fn [[T T] [Integer] [body]])`.
//
// Maps are sorted BY VALUE — the result is a new Map whose entries
// appear in ascending value order. Keys are preserved.
//
// Mixed-type pairs that CompareValues can't order fall back to
// lexical comparison of their canonical Value.String forms, matching
// the existing `array.sort` semantics — sort stays total even when
// the lattice walk doesn't find a Comparer for the pair.
var sortNative = NativeFunc{
	Name:        "sort",
	ForwardArgs: true,
	Signatures: []NativeSig{
		{
			Args:    []*Type{TList},
			Handler: sortListHandler,
			Returns: []*Type{TList},
		},
		{
			Args:    []*Type{TMap},
			Handler: sortMapHandler,
			Returns: []*Type{TMap},
		},
	},
}

func sortListHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	lst, err := eng.AsList(args[0])
	if err != nil {
		return nil, fmt.Errorf("sort: %w", err)
	}
	n := lst.Len()
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		out[i] = lst.Get(i)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return compareForSort(out[i], out[j]) < 0
	})
	return []Value{eng.NewList(out)}, nil
}

func sortMapHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	m, err := eng.AsMap(args[0])
	if err != nil {
		return nil, fmt.Errorf("sort: %w", err)
	}
	if m == nil {
		return []Value{args[0]}, nil
	}
	keys := m.Keys()
	type kv struct {
		k string
		v Value
	}
	pairs := make([]kv, len(keys))
	for i, k := range keys {
		v, _ := m.Get(k)
		pairs[i] = kv{k: k, v: v}
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		return compareForSort(pairs[i].v, pairs[j].v) < 0
	})
	out := eng.NewOrderedMap()
	for _, p := range pairs {
		out.Set(p.k, p.v)
	}
	return []Value{eng.NewMap(out)}, nil
}

// compareForSort wraps eng.CompareValues with a string-form fallback
// for pairs the kernel can't order (different scalar branches, value
// shapes without a Comparer). Mirrors the lang/go/native array-sort
// pattern so sort stays total.
func compareForSort(a, b Value) int {
	cmp, err := eng.CompareValues(a, b)
	if err == nil {
		return cmp
	}
	as, bs := a.String(), b.String()
	switch {
	case as < bs:
		return -1
	case as > bs:
		return 1
	default:
		return 0
	}
}
