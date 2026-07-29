package eng

// Seam-6b cluster B2: unit tests for the previously-unreached blocks in
// deftable.go — the nil-receiver guards on every DefTable method plus the
// Replace-on-empty and Truncate-negative-want arms. All calls are direct
// and in-package per design/TEST-SEAMS.10.md.

import "testing"

func TestS6b2DefTableNilReceiverGuards(t *testing.T) {
	var dt *DefTable

	if _, ok := dt.TopEntry("x"); ok {
		t.Error("nil TopEntry must miss")
	}
	dt.Push("x", NewInteger(1))                          // must not panic
	dt.PushType("X", TInteger, NewTypeLiteral(TInteger)) // must not panic
	if _, ok := dt.PopEntry("x"); ok {
		t.Error("nil PopEntry must miss")
	}
	if got := dt.Mutations(); got != 0 {
		t.Errorf("nil Mutations = %d, want 0", got)
	}
	if !dt.TruncationCoveredBy(nil, func(string) bool { return false }) {
		t.Error("nil TruncationCoveredBy must be vacuously true")
	}
	if dt.Has("x") {
		t.Error("nil Has must be false")
	}
	if dt.IsType("x") {
		t.Error("nil IsType must be false")
	}
	if got := dt.Depth("x"); got != 0 {
		t.Errorf("nil Depth = %d, want 0", got)
	}
	if dt.Replace("x", NewInteger(1)) {
		t.Error("nil Replace must be false")
	}
	dt.Truncate("x", 0)                 // must not panic
	dt.Delete("x")                      // must not panic
	dt.Set("x", []Value{NewInteger(1)}) // must not panic
	if got := dt.Stack("x"); got != nil {
		t.Errorf("nil Stack = %v, want nil", got)
	}
	if got := dt.Names(); got != nil {
		t.Errorf("nil Names = %v, want nil", got)
	}
	if got := dt.Snapshot(); got != nil {
		t.Errorf("nil Snapshot = %v, want nil", got)
	}
	dt.Restore(map[string]int{"x": 1}) // must not panic
	if got := dt.Clone(); got == nil {
		t.Error("nil Clone must return a fresh empty table")
	} else if got.Depth("x") != 0 {
		t.Error("nil Clone must be empty")
	}
	dt.IsolateValues() // must not panic
}

// TestDefTableIsolateValuesDeepCopies pins the primitive behind
// ForkConcurrentIsolated at the unit level, where the aliasing is visible
// directly rather than through await's goroutines.
//
// Clone alone copies the binding STACKS, so the clone can be pushed to and
// popped from independently. It does not copy the bound VALUES: a
// pointer-backed payload stays shared, and an in-place write through one
// table lands in the other. IsolateValues is what breaks that link.
func TestDefTableIsolateValuesDeepCopies(t *testing.T) {
	orig := NewDefTable()
	shared := NewMap(NewOrderedMap())
	orig.Push("m", shared)
	orig.Push("n", NewInteger(7))

	// Clone WITHOUT isolation: the payload pointer is shared, which is the
	// property the concurrent path could not tolerate.
	aliased := orig.Clone()
	got, _ := aliased.Top("m")
	if am, _ := AsMap(got); am == nil {
		t.Fatal("clone lost the map payload")
	} else if om, _ := AsMap(shared); om != am {
		t.Fatal("precondition failed: a plain Clone should still ALIAS the " +
			"payload — if it no longer does, IsolateValues is redundant and " +
			"this whole mechanism needs re-examining")
	}

	// Clone WITH isolation: same value, independent payload.
	isolated := orig.Clone()
	isolated.IsolateValues()
	iso, _ := isolated.Top("m")
	im, _ := AsMap(iso)
	om, _ := AsMap(shared)
	if im == nil {
		t.Fatal("isolated clone lost the map payload")
	}
	if im == om {
		t.Error("IsolateValues must give the clone its own payload")
	}
	// An immutable scalar needs no copy and must survive intact.
	if n, ok := isolated.Top("n"); !ok || fmtInt(t, n) != 7 {
		t.Errorf("scalar binding must survive isolation, got %v (ok=%v)", n, ok)
	}
	// The original is untouched by any of it.
	if orig.Depth("m") != 1 {
		t.Errorf("isolating a CLONE must not disturb the original: depth %d",
			orig.Depth("m"))
	}
}

func fmtInt(t *testing.T, v Value) int64 {
	t.Helper()
	n, err := AsInteger(v)
	if err != nil {
		t.Fatalf("AsInteger: %v", err)
	}
	return n
}

func TestS6b2DefTableReplaceEmptyStack(t *testing.T) {
	dt := NewDefTable()
	if dt.Replace("s6b2gone", NewInteger(1)) {
		t.Error("Replace on an unbound name must be false")
	}
	// Positive twin: a bound name replaces.
	dt.Push("s6b2here", NewInteger(1))
	if !dt.Replace("s6b2here", NewInteger(2)) {
		t.Error("Replace on a bound name must succeed")
	}
	top, _ := dt.Top("s6b2here")
	if n, err := AsInteger(top); err != nil || n != 2 {
		t.Errorf("replaced binding = %v (%v), want 2", top, err)
	}
}

// SetAt — the OpBindGlobal write-back primitive: replace at a specific
// 1-based depth, preserving the entry's TypeDef; out-of-range depths (a slot
// a check-time undef popped) are a false no-op, never a panic or a push.
func TestDefTableSetAt(t *testing.T) {
	var nilDT *DefTable
	if nilDT.SetAt("x", 1, NewInteger(1)) {
		t.Error("nil SetAt must be false")
	}

	dt := NewDefTable()
	if dt.SetAt("x", 1, NewInteger(1)) {
		t.Error("SetAt on an unbound name must be false")
	}
	dt.Push("x", NewInteger(10))
	dt.Push("x", NewInteger(20))
	if dt.SetAt("x", 0, NewInteger(99)) {
		t.Error("SetAt depth 0 must be false (depths are 1-based)")
	}
	if dt.SetAt("x", 3, NewInteger(99)) {
		t.Error("SetAt past the stack depth must be false (popped slot skips)")
	}
	if !dt.SetAt("x", 1, NewInteger(11)) || !dt.SetAt("x", 2, NewInteger(22)) {
		t.Fatal("SetAt at live depths must succeed")
	}
	if top, _ := dt.Top("x"); mustInt(t, top) != 22 {
		t.Errorf("top after SetAt = %v, want 22", top)
	}
	if !dt.Pop("x") {
		t.Fatal("pop")
	}
	if top, _ := dt.Top("x"); mustInt(t, top) != 11 {
		t.Errorf("depth-1 slot after SetAt = %v, want 11", top)
	}

	// TypeDef preservation: a type binding's minted def survives the body swap.
	dt.PushType("T", TInteger, NewTypeLiteral(TInteger))
	if !dt.SetAt("T", 1, NewTypeLiteral(TNumber)) {
		t.Fatal("SetAt on a type binding must succeed")
	}
	if e, ok := dt.TopEntry("T"); !ok || e.TypeDef != TInteger {
		t.Errorf("SetAt must preserve TypeDef, got %+v", e)
	}
}

func mustInt(t *testing.T, v Value) int64 {
	t.Helper()
	n, err := AsInteger(v)
	if err != nil {
		t.Fatalf("not an integer: %v (%v)", v, err)
	}
	return n
}

func TestS6b2DefTableTruncateNegativeWant(t *testing.T) {
	dt := NewDefTable()
	dt.Push("s6b2t", NewInteger(1))
	dt.Push("s6b2t", NewInteger(2))
	// want < 0 clamps to 0 and removes the entry entirely.
	dt.Truncate("s6b2t", -3)
	if dt.Has("s6b2t") {
		t.Error("Truncate with negative want must remove the whole stack")
	}
}
