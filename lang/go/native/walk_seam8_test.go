package native

import (
	"testing"
)

// --- core `walk` word (walk_core.go) ---

func TestW8WalkCoreModePaths(t *testing.T) {
	// mode given as an atom -> accepted (AsConcreteAtom branch).
	if _, err := w8Run(t, `walk {mode: breadth/q} {a:1} (m:Any => [m.path])`); err != nil {
		t.Fatalf("walk with atom mode: %v", err)
	}
	// mode given as a non-string, non-atom (Integer) -> error (walkModeValue
	// returns false).
	if _, err := w8Run(t, `walk {mode: 5} {a:1} (m:Any => [m.path])`); err == nil {
		t.Fatal("walk with an integer mode must error")
	}
}

func TestW8WalkCoreHookNotConcrete(t *testing.T) {
	// The descend hook is a bare type literal: not a closure, not a
	// Function, and not concrete -> the classify guard errors.
	if _, err := w8Run(t, `walk {} {a:1} Integer`); err == nil {
		t.Fatal("walk with a type-literal hook must error")
	}
}

func TestW8CallWalkHookAbsent(t *testing.T) {
	// An absent (optional) ascend hook is a no-op.
	r := w8Reg(t)
	if err := callWalkHook(r, walkHook{present: false}, NewInteger(1)); err != nil {
		t.Fatalf("callWalkHook of an absent hook should be a no-op, got %v", err)
	}
}
