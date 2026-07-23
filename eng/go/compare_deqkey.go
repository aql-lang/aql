package eng

import (
	"sort"
	"strconv"
	"strings"
)

// This file gives DeepEqual-based scans a sub-quadratic shape. The
// collection words (unique / member / indices / group) need "find a
// deq-equal value among N seen so far" — a nested DeepEqual scan makes
// that O(N·M). DeqKey assigns each value a bucket such that deq-equal
// values always land in the same bucket, so a scan only has to compare
// within one bucket.
//
// The key is NOT an equality: two values with the same key may be
// deq-unequal (NaN shares the "NaN" bucket with itself; a float64
// projection can collide). Callers must still confirm candidates with
// DeepEqual. The contract is one-directional, and it mirrors
// DeepEqual's dispatch branch by branch:
//
//	DeepEqual(a, b) == true  ⇒  DeqKey(a) and DeqKey(b) return either
//	  - DeqKeyed with identical keys, or
//	  - DeqUnkeyed on at least one side.
//
// DeqUnkeyed marks the container shapes DeepEqual compares by RENDER
// when structural access fails (typed lists / tables / records — the
// `a.String() == b.String()` fallback arms): a plain container can
// deq-match one of those across the plain/typed boundary, so no
// per-value key is sound and the value must be scanned pairwise.
// Containers propagate unkeyedness up from their children.
//
// DeqNeverEqual marks values that reach DeepEqual's unsupported
// fall-through (stores, functions, timers, …): deq-equal to nothing,
// including themselves, so scans can skip them entirely.

// DeqKeyClass classifies how a value participates in bucketed deq
// scans — see the contract above.
type DeqKeyClass int

const (
	// DeqKeyed — the key is a sound bucket for this value.
	DeqKeyed DeqKeyClass = iota
	// DeqUnkeyed — no sound key; scan pairwise within the family.
	DeqUnkeyed
	// DeqNeverEqual — deq-equal to nothing (including itself).
	DeqNeverEqual
)

// deqKeyMaxDepth caps DeqKey's structural recursion. A self-referential
// flex container would otherwise recurse forever computing its own key
// — where the pairwise scans it replaces only recurse when a comparison
// actually happens. Past the cap the subtree is DeqUnkeyed, which is
// always sound (it just falls back to the pairwise scan).
const deqKeyMaxDepth = 64

// DeqKey returns the deq bucket key for v. See the contract at the top
// of this file; the branch structure deliberately mirrors DeepEqual's.
func DeqKey(v Value) (string, DeqKeyClass) {
	return deqKeyAtDepth(v, 0)
}

func deqKeyAtDepth(v Value, depth int) (string, DeqKeyClass) {
	if depth > deqKeyMaxDepth {
		return "", DeqUnkeyed
	}

	// none — DeepEqual's both-None branch.
	if ValueType(v).Equal(TNone) {
		return "N", DeqKeyed
	}

	// DepScalar — equal only to a DepScalar of the identical Parent.
	if v.IsDepScalar() {
		return "D:" + v.Parent.ID, DeqKeyed
	}

	// Numbers — scalarFamilyEqual compares int64-exact (int/int),
	// float64-projected (a Float on either side), or rat-exact (a Big
	// on either side). All three agree that equal values share the
	// same real number, so the correctly-rounded float64 projection of
	// that number is a sound shared bucket: equal int64s project
	// identically, and a Big rat-equal to any operand projects to the
	// same float64 as that operand. Collisions (2^53 vs 2^53+1) are
	// disambiguated pairwise. The error-ignoring AsNumber mirrors
	// scalarFamilyEqual's own accessor use, so whatever projection the
	// equality sees, the key sees too.
	if v.Parent.ConformsTo(TNumber) {
		var f float64
		if numIsBig(v) {
			var err error
			f, err = AsFloatApprox(v)
			if err != nil {
				// toRatExact fails on the same accessor error, so the
				// value is deq-equal to nothing.
				return "", DeqNeverEqual
			}
		} else {
			f, _ = AsNumber(v)
		}
		if f == 0 {
			f = 0 // fold -0.0 into +0.0 — they compare equal
		}
		return "n:" + strconv.FormatFloat(f, 'g', -1, 64), DeqKeyed
	}

	// The by-value scalar families — same error-ignoring accessors as
	// scalarFamilyEqual.
	if v.Parent.ConformsTo(TString) {
		s, _ := AsString(v)
		return "s:" + s, DeqKeyed
	}
	if v.Parent.ConformsTo(TBoolean) {
		b, _ := AsBoolean(v)
		return "b:" + strconv.FormatBool(b), DeqKeyed
	}
	if v.Parent.ConformsTo(TAtom) {
		a, _ := AsAtom(v)
		return "a:" + a, DeqKeyed
	}

	// Remaining scalars — scalarSemanticEqual equates values only when
	// their Parents coincide or one Parent is the other's ancestor, so
	// any equal pair shares its topmost family node under Scalar. One
	// bucket per family (Time, Micron, …) is sound; members are
	// disambiguated pairwise.
	if v.Parent.ConformsTo(TScalar) {
		return "c:" + scalarFamilyRoot(v.Parent).ID, DeqKeyed
	}

	// Lists — structural recursion, or the render fallback for shapes
	// AsMutableList can't open (typed lists, tables): those are
	// DeqUnkeyed because the render compare crosses the plain/typed
	// boundary.
	if nodeFamily(v.Parent).Equal(TList) {
		elems, err := AsMutableList(v)
		if err != nil {
			return "", DeqUnkeyed
		}
		var b strings.Builder
		b.WriteByte('[')
		for i, e := range elems {
			ek, ec := deqKeyAtDepth(e, depth+1)
			if ec != DeqKeyed {
				// An unkeyed child (or a never-equal one, whose pair
				// could still render-match a typed list) makes the
				// whole container render-reachable — no sound key.
				return "", DeqUnkeyed
			}
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(ek)
		}
		b.WriteByte(']')
		return b.String(), DeqKeyed
	}

	// Maps — key-order-insensitive structural recursion (DeepEqual
	// compares by key lookup), or the render fallback for records and
	// typed maps.
	if nodeFamily(v.Parent).Equal(TMap) {
		m, err := AsMutableMap(v)
		if err != nil {
			return "", DeqUnkeyed
		}
		keys := append([]string(nil), m.Keys()...)
		sort.Strings(keys)
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range keys {
			mv, _ := m.Get(k)
			vk, vc := deqKeyAtDepth(mv, depth+1)
			if vc != DeqKeyed {
				return "", DeqUnkeyed
			}
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Quote(k))
			b.WriteByte('=')
			b.WriteString(vk)
		}
		b.WriteByte('}')
		return b.String(), DeqKeyed
	}

	// XML — xmlElementsEqual requires equal tags, so the tag is a
	// sound bucket; attribute/child equality is confirmed pairwise.
	if tag, _, _, ok := xmlParts(v); ok {
		return "x:" + tag, DeqKeyed
	}

	// Flat instances — deq requires the SAME exact Parent and
	// field-wise deep equality over identical present-field sets, and
	// there is no render fallback for instances, so a never-equal
	// field makes the instance itself never-equal.
	if fields, _, ok := flatInstanceParts(v); ok {
		if fields == nil {
			return "", DeqNeverEqual
		}
		keys := append([]string(nil), fields.Keys()...)
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString("i:")
		b.WriteString(v.Parent.ID)
		b.WriteByte('{')
		for i, k := range keys {
			fv, _ := fields.Get(k)
			fk, fc := deqKeyAtDepth(fv, depth+1)
			switch fc {
			case DeqNeverEqual:
				return "", DeqNeverEqual
			case DeqUnkeyed:
				return "", DeqUnkeyed
			}
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Quote(k))
			b.WriteByte('=')
			b.WriteString(fk)
		}
		b.WriteByte('}')
		return b.String(), DeqKeyed
	}

	// DeepEqual's unsupported fall-through — stores, functions,
	// modules, errors, timers, words: deq-equal to nothing.
	return "", DeqNeverEqual
}

// scalarFamilyRoot walks up to the child-of-Scalar ancestor — the node
// scalarSemanticEqual's Parent/LCA rules guarantee equal values share.
func scalarFamilyRoot(t *Type) *Type {
	for t.Parent != nil && !t.Parent.Equal(TScalar) && !t.Equal(TScalar) {
		t = t.Parent
	}
	return t
}

// deqFam names the cross-scan family a DeqUnkeyed value can deq-match
// within: the render fallbacks only fire list-vs-list and map-vs-map,
// and instance equality only fires within one exact class. Values in
// families that can never contain an unkeyed member return "".
func deqFam(v Value) string {
	if nodeFamily(v.Parent).Equal(TList) {
		return "L"
	}
	if nodeFamily(v.Parent).Equal(TMap) {
		return "M"
	}
	if _, _, ok := flatInstanceParts(v); ok {
		return "I:" + v.Parent.ID
	}
	return ""
}

// DeqIndex answers "which of the values added so far is deq-equal to
// this one?" in near-linear total time. Values are held in insertion
// order; FirstMatch returns the position of the earliest deq-equal
// entry, exactly as a linear DeepEqual scan would, but touching only
// the candidate's bucket plus the (rare) unkeyed values of its family.
type DeqIndex struct {
	vals         []Value
	buckets      map[string][]int // DeqKeyed entries by key
	unkeyedByFam map[string][]int // DeqUnkeyed entries by family
	allByFam     map[string][]int // every keyed+unkeyed entry of a fam
}

// Add appends v and returns its position.
func (x *DeqIndex) Add(v Value) int {
	idx := len(x.vals)
	x.vals = append(x.vals, v)
	key, class := DeqKey(v)
	if class == DeqNeverEqual {
		return idx // matches nothing — no lookup structure needed
	}
	fam := deqFam(v)
	if x.buckets == nil {
		x.buckets = map[string][]int{}
		x.unkeyedByFam = map[string][]int{}
		x.allByFam = map[string][]int{}
	}
	if class == DeqKeyed {
		x.buckets[key] = append(x.buckets[key], idx)
	} else {
		x.unkeyedByFam[fam] = append(x.unkeyedByFam[fam], idx)
	}
	if fam != "" {
		x.allByFam[fam] = append(x.allByFam[fam], idx)
	}
	return idx
}

// FirstMatch returns the position of the earliest added value that is
// DeepEqual to v, or -1. It does not add v.
func (x *DeqIndex) FirstMatch(v Value) int {
	key, class := DeqKey(v)
	switch class {
	case DeqNeverEqual:
		return -1
	case DeqUnkeyed:
		// v is render-reachable: any keyed or unkeyed entry of its
		// family could match.
		for _, i := range x.allByFam[deqFam(v)] {
			if DeepEqual(x.vals[i], v) {
				return i
			}
		}
		return -1
	}
	// Keyed: the bucket, merged in insertion order with the unkeyed
	// entries of v's family (a plain container can deq-match a typed
	// one via the render fallback).
	bucket := x.buckets[key]
	unkeyed := x.unkeyedByFam[deqFam(v)]
	bi, ui := 0, 0
	for bi < len(bucket) || ui < len(unkeyed) {
		var i int
		if ui >= len(unkeyed) || (bi < len(bucket) && bucket[bi] < unkeyed[ui]) {
			i = bucket[bi]
			bi++
		} else {
			i = unkeyed[ui]
			ui++
		}
		if DeepEqual(x.vals[i], v) {
			return i
		}
	}
	return -1
}
