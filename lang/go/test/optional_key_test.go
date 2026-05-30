package test

import (
	"testing"
)

// The `?` field-marker in map literals (`{x?:T, ...}`) means "None or
// absent, universally": the key may be absent, AND if present it
// may carry None (or any value satisfying T). The marker propagates
// uniformly through unify, is, and fn-arg signature matching.

func TestOptionalKey_UnifyOmitsAbsent(t *testing.T) {
	got := runOne(t, `{x?:1,y:Integer} unify {y:2}`)
	if len(got) != 2 || got[1] != "true" {
		t.Fatalf("expected success, got %v", got)
	}
	// Absent x is omitted from the unified result — not forced to None.
	if got[0] != "{y:2}" {
		t.Errorf("expected {y:2}, got %v", got[0])
	}
}

func TestOptionalKey_UnifyCommutative(t *testing.T) {
	got := runOne(t, `{y:2} unify {x?:1,y:Integer}`)
	if len(got) != 2 || got[1] != "true" {
		t.Fatalf("expected success, got %v", got)
	}
	if got[0] != "{y:2}" {
		t.Errorf("expected {y:2}, got %v", got[0])
	}
}

func TestOptionalKey_UnifyAcceptsExplicitNone(t *testing.T) {
	got := runOne(t, `{x?:1,y:Integer} unify {x:None,y:2}`)
	if len(got) != 2 || got[1] != "true" {
		t.Fatalf("expected success, got %v", got)
	}
	// Explicit None at an optional key flows through via the
	// disjunct(1, None) value wrap.
	if got[0] != "{x:None y:2}" {
		t.Errorf("expected {x:None y:2}, got %v", got[0])
	}
}

func TestOptionalKey_UnifyAcceptsExplicitConcrete(t *testing.T) {
	got := runOne(t, `{x?:1,y:Integer} unify {x:1,y:2}`)
	if len(got) != 2 || got[1] != "true" {
		t.Fatalf("expected success, got %v", got)
	}
	if got[0] != "{x:1 y:2}" {
		t.Errorf("expected {x:1 y:2}, got %v", got[0])
	}
}

func TestOptionalKey_UnifyRejectsWrongValue(t *testing.T) {
	got := runOne(t, `{x?:1,y:Integer} unify {x:5,y:2}`)
	if len(got) != 2 || got[1] != "false" {
		t.Fatalf("expected failure for x=5 (not in {1, None}), got %v", got)
	}
}

func TestOptionalKey_IsAdmitsAbsent(t *testing.T) {
	got := runOne(t, `{y:2} is {x?:1,y:Integer}`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("expected true (absent optional ok), got %v", got)
	}
}

func TestOptionalKey_IsAdmitsExplicitNone(t *testing.T) {
	got := runOne(t, `{x:None,y:2} is {x?:1,y:Integer}`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("expected true (None ok), got %v", got)
	}
}

func TestOptionalKey_IsRejectsWrongValue(t *testing.T) {
	got := runOne(t, `{x:5,y:2} is {x?:1,y:Integer}`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("expected false (5 ∉ {1, None}), got %v", got)
	}
}

func TestOptionalKey_IsRejectsExtraKey(t *testing.T) {
	got := runOne(t, `{y:2,z:3} is {x?:1,y:Integer}`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("expected false (extra key z), got %v", got)
	}
}

func TestOptionalKey_FnArgAdmitsAbsent(t *testing.T) {
	got := runOne(t, `def f fn [[m:{x?:1,y:Integer}] [Any] [m get y]]
f {y:2}`)
	if len(got) != 1 || got[0] != int64(2) {
		t.Errorf("expected f {y:2} to succeed and return 2, got %v", got)
	}
}

func TestOptionalKey_FnArgAdmitsExplicitNone(t *testing.T) {
	got := runOne(t, `def f fn [[m:{x?:1,y:Integer}] [Any] [m get y]]
f {x:None,y:2}`)
	if len(got) != 1 || got[0] != int64(2) {
		t.Errorf("expected f {x:None,y:2} to succeed, got %v", got)
	}
}

func TestOptionalKey_FnArgAdmitsExplicitConcrete(t *testing.T) {
	got := runOne(t, `def f fn [[m:{x?:1,y:Integer}] [Any] [m get y]]
f {x:1,y:2}`)
	if len(got) != 1 || got[0] != int64(2) {
		t.Errorf("expected f {x:1,y:2} to succeed, got %v", got)
	}
}
