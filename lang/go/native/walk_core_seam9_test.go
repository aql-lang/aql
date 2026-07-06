package native

import (
	"testing"
)

// Seam-9 coverage (W9_nativeC) for walk_core.go: walkModeValue's atom
// arm, childrenOf's Xml branch, and the DFS/BFS hook-error propagation
// paths (driven through the `walk` word with an erroring hook).

func TestW9WalkModeValueAtom(t *testing.T) {
	// An atom mode value is accepted. (Note: an atom is also concrete-string
	// convertible, so it is caught by the String branch — the dedicated Atom
	// arm at 117 is unreachable/dead.)
	s, ok := walkModeValue(NewAtom("depth"))
	if !ok || s != "depth" {
		t.Fatalf("walkModeValue(atom) = (%q,%v), want (depth,true)", s, ok)
	}
}

func TestW9ChildrenOfXml(t *testing.T) {
	// An Xml element with children enumerates them (322.3,323.26 /
	// 323.26,325.4 / 326.3,326.19).
	child := NewXmlElement("child", nil, nil)
	root := NewXmlElement("root", nil, []Value{child})
	entries, ok := childrenOf(root)
	if !ok || len(entries) != 1 {
		t.Fatalf("childrenOf(xml) = (%v,%v), want one child", entries, ok)
	}
	if entries[0].key != "0" {
		t.Fatalf("childrenOf(xml) first key = %q, want 0", entries[0].key)
	}
}

func TestW9WalkDFSAscendHookError(t *testing.T) {
	r := seam5Reg(t)
	// A 4-arg depth-first walk whose ascend hook errors at run time
	// propagates the error out of walkDFS (233.71,235.5).
	if _, err := seam5Run(r,
		`walk {} [1 2] (fn [[m:Any] [Any] [1]]) (fn [[m:Any] [Any] [(undefinedxyzwalkA)]])`); err == nil {
		t.Fatalf("walk DFS ascend-hook error must propagate")
	}
}

func TestW9WalkBFSHookErrors(t *testing.T) {
	// Breadth-first descend-hook error (266.71,268.4).
	r := seam5Reg(t)
	if _, err := seam5Run(r,
		`walk {mode: "breadth"} [1 2] (fn [[m:Any] [Any] [(undefinedxyzwalkB)]])`); err == nil {
		t.Fatalf("walk BFS descend-hook error must propagate")
	}
	// Breadth-first ascend-hook error (280.79,282.4).
	r2 := seam5Reg(t)
	if _, err := seam5Run(r2,
		`walk {mode: "breadth"} [1 2] (fn [[m:Any] [Any] [1]]) (fn [[m:Any] [Any] [(undefinedxyzwalkC)]])`); err == nil {
		t.Fatalf("walk BFS ascend-hook error must propagate")
	}
}
