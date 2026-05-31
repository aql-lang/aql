package native

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
)

// runUnpack parses and runs AQL source, returning the canonical render of the
// final stack. unpack is a source-level feature (forward collection of the
// names list, def-binding into the current scope), so the tests exercise it
// through the real parser.
func runUnpack(t *testing.T, src string) string {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, err := NewTop(r).Run(toks)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return Canon(out)
}

func runUnpackErr(t *testing.T, src string) error {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	toks, err := parser.Parse(src)
	if err != nil {
		return err
	}
	_, runErr := NewTop(r).Run(toks)
	return runErr
}

func TestUnpackBindsSingleName(t *testing.T) {
	if got := runUnpack(t, `def m {x:1} unpack [x] m x`); got != "1" {
		t.Errorf("unpack [x] m then x = %q, want 1", got)
	}
}

func TestUnpackBindsSeveralNames(t *testing.T) {
	if got := runUnpack(t, `def m {a:1 b:2} unpack [a b] m a add b`); got != "3" {
		t.Errorf("unpack [a b] m then a add b = %q, want 3", got)
	}
}

func TestUnpackBindsOnlyNamed(t *testing.T) {
	// y is requested, x is not — only y is bound.
	if got := runUnpack(t, `def m {x:1 y:2} unpack [y] m y`); got != "2" {
		t.Errorf("unpack [y] m then y = %q, want 2", got)
	}
}

func TestUnpackAllBindsEveryKey(t *testing.T) {
	if got := runUnpack(t, `def m {a:1 b:2} unpack all m a add b`); got != "3" {
		t.Errorf("unpack all m then a add b = %q, want 3", got)
	}
}

func TestUnpackAllEmptyMapIsNoop(t *testing.T) {
	if got := runUnpack(t, `def m {} unpack all m`); got != "" {
		t.Errorf("unpack all {} = %q, want empty", got)
	}
}

func TestUnpackAllWrongKeywordErrors(t *testing.T) {
	// Only the literal `all` selects every key; any other bare atom errors.
	err := runUnpackErr(t, `def m {a:1} unpack everything m`)
	if err == nil || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("expected keyword error, got %v", err)
	}
}

func TestUnpackRenameBindsTargets(t *testing.T) {
	// {srcKey: localName} binds each source key under the chosen name.
	if got := runUnpack(t, `def m {a:1 b:2} unpack {a: x b: y} m x add y`); got != "3" {
		t.Errorf("unpack {a:x b:y} m then x add y = %q, want 3", got)
	}
}

func TestUnpackRenamePartial(t *testing.T) {
	// Only the renamed keys are bound; others are untouched.
	if got := runUnpack(t, `def m {a:1 b:2} unpack {a: x} m x`); got != "1" {
		t.Errorf("unpack {a:x} m then x = %q, want 1", got)
	}
}

func TestUnpackRenameShorthand(t *testing.T) {
	// The parser desugars map shorthand `{foo}` to `{foo: foo}`, so a
	// shorthand rename map binds each key under its own name — making
	// `unpack {a b} m` behave like `unpack [a b] m`.
	if got := runUnpack(t, `def m {a:1 b:2} unpack {a b} m a add b`); got != "3" {
		t.Errorf("unpack {a b} m then a add b = %q, want 3", got)
	}
	// Shorthand and explicit rename can be mixed.
	if got := runUnpack(t, `def m {a:1 b:2} unpack {a b: y} m a add y`); got != "3" {
		t.Errorf("unpack {a b:y} m then a add y = %q, want 3", got)
	}
}

func TestUnpackRenameMissingKeyErrors(t *testing.T) {
	err := runUnpackErr(t, `def m {a:1} unpack {z: x} m`)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error for missing source key, got %v", err)
	}
}

func TestUnpackRenameToCapitalisedRejected(t *testing.T) {
	err := runUnpackErr(t, `def m {a:1} unpack {a: X} m`)
	if err == nil || !strings.Contains(err.Error(), "capitalised") {
		t.Fatalf("expected 'capitalised' error, got %v", err)
	}
}

func TestUnpackReturnsNothing(t *testing.T) {
	// unpack leaves nothing on the stack.
	if got := runUnpack(t, `def m {x:1} unpack [x] m`); got != "" {
		t.Errorf("unpack [x] m left %q on the stack, want empty", got)
	}
}

func TestUnpackEmptyNamesIsNoop(t *testing.T) {
	if got := runUnpack(t, `def m {x:1} unpack [] m`); got != "" {
		t.Errorf("unpack [] m = %q, want empty", got)
	}
}

func TestUnpackMissingKeyErrors(t *testing.T) {
	err := runUnpackErr(t, `def m {x:1} unpack [y] m y`)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got %v", err)
	}
}

func TestUnpackCapitalisedNameRejected(t *testing.T) {
	err := runUnpackErr(t, `def m {X:1} unpack [X] m`)
	if err == nil || !strings.Contains(err.Error(), "capitalised") {
		t.Fatalf("expected 'capitalised' error, got %v", err)
	}
}

func TestUnpackNonMapSourceRejected(t *testing.T) {
	err := runUnpackErr(t, `unpack [x] 42`)
	if err == nil {
		t.Fatal("expected error unpacking from a non-map source")
	}
}

func TestUnpackTypeLiteralSourceRejected(t *testing.T) {
	// A bare `Map` type literal does not satisfy the concrete-map sig slot,
	// so dispatch rejects it before the handler runs. Either way it must
	// error rather than bind anything.
	err := runUnpackErr(t, `unpack [x] Map`)
	if err == nil {
		t.Fatal("expected error for a type-literal source")
	}
}

// TestUnpackHandlerNoPanic feeds the handler type-literal args directly and
// asserts it returns an error rather than panicking (per the Panic Prevention
// rule). It mirrors the TestTypeLiteralNoPanicNative discipline for words that
// have no entry in a central table.
func TestUnpackHandlerNoPanic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	cases := [][2]Value{
		{NewTypeLiteral(TList), NewTypeLiteral(TMap)},          // both type literals
		{NewList([]Value{NewAtom("x")}), NewTypeLiteral(TMap)}, // type-literal map
		{NewTypeLiteral(TList), NewMap(NewOrderedMap())},       // type-literal names
	}
	handlers := map[string]func([]Value, map[string]Value, []Value, *Registry) ([]Value, error){
		"names":  unpackHandler,
		"all":    unpackAllHandler,
		"rename": unpackRenameHandler,
	}
	for name, h := range handlers {
		for i, c := range cases {
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Fatalf("%s case %d: handler panicked: %v", name, i, rec)
					}
				}()
				if _, err := h(c[:], nil, nil, r); err == nil {
					t.Errorf("%s case %d: expected an error, got nil", name, i)
				}
			}()
		}
	}
}

// TestUnpackScopedInFnBody confirms unpack bindings made inside a fn body are
// torn down at body exit by the existing depth-based def cleanup — the name
// must not leak to module scope.
func TestUnpackScopedInFnBody(t *testing.T) {
	src := `def f fn [[] [Integer] [def m {x:5} unpack [x] m x]]  f`
	if got := runUnpack(t, src); got != "5" {
		t.Fatalf("f returned %q, want 5", got)
	}
	// After the call, x must be unbound — referencing it errors.
	err := runUnpackErr(t, src+`  x`)
	if err == nil || !strings.Contains(err.Error(), "undefined") {
		t.Fatalf("expected x to be undefined after fn returns, got %v", err)
	}
}
