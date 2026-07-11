package lsp

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
)

// TestMemberCompleteHelpers covers the edge branches of the pure helpers so the
// member-completion path is fully exercised (empty stacks, missing defs,
// bounded windows, string skipping, out-of-range positions).
func TestMemberCompleteHelpers(t *testing.T) {
	if lastStackType(nil) != "" {
		t.Error("empty stack should type to \"\"")
	}
	if lastStackType([]string{"A", "B"}) != "B" {
		t.Error("stack top should be the last element")
	}

	// findDef / nextDef misses.
	if findDef("add 1 2", "y") != -1 {
		t.Error("findDef of an undefined name should be -1")
	}
	if nextDef("{a:1}") != -1 {
		t.Error("nextDef with no following def should be -1")
	}

	// mapLiteralKeys: no def, def-without-brace, and a bounded two-def window.
	if mapLiteralKeys("foo 1", "foo") != nil {
		t.Error("no `def foo` should yield nil keys")
	}
	if mapLiteralKeys("def m 5", "m") != nil {
		t.Error("`def m` with no brace should yield nil keys")
	}
	if ks := mapLiteralKeys("def m {a:1}\ndef n {b:2}", "m"); len(ks) != 1 || ks[0] != "a" {
		t.Errorf("bounded map keys = %v, want [a] (must not bleed into def n)", ks)
	}

	// topLevelKeys: empty map, numeric-key skip, and a closed block.
	if k := topLevelKeys("{}"); k != nil {
		t.Errorf("empty map keys = %v, want nil", k)
	}
	if k := topLevelKeys("{1:2 a:3}"); len(k) != 1 || k[0] != "a" {
		t.Errorf("numeric key should be skipped, got %v", k)
	}
	// An unterminated block (no closing brace) still returns the keys seen.
	if k := topLevelKeys("{a:1 b:2"); len(k) != 2 {
		t.Errorf("unterminated map keys = %v, want [a b]", k)
	}

	// skipString: escaped quote and unterminated.
	if got := skipString(`'a\'b' x`, 0); got != 6 {
		t.Errorf("skipString past an escaped quote = %d, want 6", got)
	}
	if got := skipString("'abc", 0); got != 4 {
		t.Errorf("skipString unterminated = %d, want 4", got)
	}

	// receiverBefore out-of-range and non-receiver positions.
	if _, ok := receiverBefore("x", Position{Line: 5, Character: 0}); ok {
		t.Error("a line beyond the buffer is not a member context")
	}
	if _, ok := receiverBefore("ab", Position{Line: 0, Character: 99}); ok {
		t.Error("a character beyond the line is not a member context")
	}
	if _, ok := receiverBefore("x .y", Position{Line: 0, Character: 4}); ok {
		t.Error("a dot preceded by whitespace is not a receiver")
	}
}

// TestModuleMemberItemsMisses covers moduleMemberItems' non-matching arms: an
// unresolvable module id, a receiver that is not a namespace of any imported
// module, and a nil registry.
func TestModuleMemberItemsMisses(t *testing.T) {
	s := newServer(strings.NewReader(""), io.Discard, io.Discard)

	// A valid import, but the receiver word is not one of its namespaces.
	if items := s.moduleMemberItems("import \"aql:rand\"\n", "NotANamespace"); items != nil {
		t.Errorf("non-namespace receiver = %v, want nil", items)
	}
	// A syntactically-valid but unknown module id fails to resolve.
	if items := s.moduleMemberItems("import \"aql:no-such-module\"\n", "Whatever"); items != nil {
		t.Errorf("unresolvable module = %v, want nil", items)
	}

	// A nil registry (init failure) yields nil.
	orig := nativeDefaultRegistry
	t.Cleanup(func() { nativeDefaultRegistry = orig })
	nativeDefaultRegistry = func(...func(*native.Registry)) (*native.Registry, error) {
		return nil, errors.New("boom")
	}
	s2 := newServer(strings.NewReader(""), io.Discard, io.Discard)
	if items := s2.moduleMemberItems("import \"aql:rand\"\n", "Rand"); items != nil {
		t.Errorf("nil registry = %v, want nil", items)
	}
}
