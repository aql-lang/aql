package native

import (
	"fmt"
	"sort"

	"github.com/boru-lang/boru/eng/go"
)

// sort VALUE
//
// Reorder a List or Map ascending by each element's natural order.
// The comparison dispatches through the kernel's Comparer capability
// (eng.CompareValues), so:
//
//   - Lists of homogeneous scalars sort numerically / lexically /
//     by atom-name as appropriate (Integer / Float / String /
//     Boolean / Atom kernel Comparers).
//   - Lists of domain values (Date / DateTime / Instant /
//     ClockDuration) sort chronologically via their native Comparers.
//   - Lists of user-typed instances sort using the comparator
//     installed via `behave compare/q (fn [[T T] [Integer] [body]])`.
//
// Maps are sorted BY VALUE — the result is a new Map whose entries
// appear in ascending value order. Keys are preserved.
//
// Mixed-type pairs that CompareValues can't order fall back to
// lexical comparison of their canonical Value.String forms, matching
// the existing `ArrayUtil.sort` semantics — sort stays total even when
// the lattice walk doesn't find a Comparer for the pair.
var sortNative = NativeFunc{
	Name: "sort",

	Signatures: []Signature{
		{
			Args:    []*Type{TList},
			Impl:    Go(sortListHandler),
			Returns: []*Type{TList}, BarrierPos: -1,
		},
		{
			Args:    []*Type{TMap},
			Impl:    Go(sortMapHandler),
			Returns: []*Type{TMap}, BarrierPos: -1,
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
	// #4 (round 3): sort reorders the list in place of a copy — retain the
	// source [:T] tag so downstream reads/writes stay typed.
	return []Value{d2RetainElem(eng.NewList(out), args[0])}, nil
}

func sortMapHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	m, err := eng.AsMap(args[0])
	if err != nil {
		return nil, fmt.Errorf("sort: %w", err)
	}
	if m == nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
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
	// #4 (round 3): sorting a {:T} map by value reorders its entries without
	// changing them — the result is still {:T}, so retain the tag (a map DOES
	// carry an element tag, symmetric with the list reorders above).
	return []Value{d2RetainElem(eng.NewMap(out), args[0])}, nil
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
