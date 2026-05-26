package eng

import (
	"strings"
	"testing"
)

// These tests exercise the engine's deeper machinery (type system,
// def stack, signature matching, multi-arg dispatch, value identity)
// using only the public NativeFunc registration API. No parser, no
// built-in word library.

// --- *Type system primitives ---------------------------------------------

func TestTypePathBuiltins(t *testing.T) {
	// Round-trip a few well-known type names through the canonical
	// table and confirm they reach the correct *Type values.
	table := TypeNameTable()
	cases := []struct {
		name string
		want *Type
	}{
		{"Integer", TInteger},
		{"String", TString},
		{"Boolean", TBoolean},
		{"List", TList},
		{"Map", TMap},
		{"Any", TAny},
	}
	for _, c := range cases {
		got, ok := table[c.name]
		if !ok {
			t.Errorf("%q missing from TypeNameTable", c.name)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("%q: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTypeMatchHierarchy(t *testing.T) {
	// Integer is a subtype of Number is a subtype of Scalar.
	if !TInteger.Matches(TNumber) {
		t.Error("Integer should match Number")
	}
	if !TInteger.Matches(TScalar) {
		t.Error("Integer should match Scalar")
	}
	if !TInteger.Matches(TAny) {
		t.Error("Integer should match Any")
	}
	if TNumber.Matches(TInteger) {
		t.Error("Number should NOT match Integer (only the reverse)")
	}
}

func TestCommonAncestorType(t *testing.T) {
	// Integer + Decimal → Number; String + Integer → Scalar; List + Integer → Any.
	if got := CommonAncestorType(TInteger, TDecimal); !got.Equal(TNumber) {
		t.Errorf("Integer+Decimal: got %v, want Number", got)
	}
	if got := CommonAncestorType(TString, TInteger); !got.Equal(TScalar) {
		t.Errorf("String+Integer: got %v, want Scalar", got)
	}
	if got := CommonAncestorType(TList, TInteger); !got.Equal(TAny) {
		t.Errorf("List+Integer: got %v, want Any", got)
	}
}

// --- Value constructors and identity ------------------------------------

func TestValueConstructors(t *testing.T) {
	cases := []struct {
		name  string
		value Value
		want  *Type
	}{
		{"integer", NewInteger(42), TInteger},
		{"decimal", NewDecimal(3.14), TDecimal},
		{"string", NewString("hi"), TString},
		{"boolean", NewBoolean(true), TBoolean},
		{"atom", NewAtom("x"), TAtom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !c.value.Parent.Matches(c.want) {
				t.Errorf("Parent = %v does not match expected %v", c.value.Parent, c.want)
			}
			if c.value.Data == nil {
				t.Error("Data should not be nil for a concrete value")
			}
		})
	}
}

func TestTypeLiteralVsConcrete(t *testing.T) {
	lit := NewTypeLiteral(TString)
	concrete := NewString("hello")
	if !IsTypeLiteral(lit) {
		t.Error("type literal should be IsTypeLiteral")
	}
	if IsConcrete(lit) {
		t.Error("type literal should NOT be IsConcrete")
	}
	if IsTypeLiteral(concrete) {
		t.Error("concrete string should NOT be IsTypeLiteral")
	}
	if !IsConcrete(concrete) {
		t.Error("concrete string should be IsConcrete")
	}
}

// --- Registry def-stack helpers ------------------------------------------

func TestDefStackPushPopShadow(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if r.Defs.Has("x") {
		t.Error("fresh registry should not have x")
	}

	r.Defs.Push("x", NewInteger(1))
	if !r.Defs.Has("x") {
		t.Error("after push, x should exist")
	}
	if d := r.Defs.Depth("x"); d != 1 {
		t.Errorf("depth = %d, want 1", d)
	}
	v, _ := r.Defs.Top("x")
	got, _ := AsInteger(v)
	if got != 1 {
		t.Errorf("top = %d, want 1", got)
	}

	// Shadow with a second push.
	r.Defs.Push("x", NewInteger(2))
	v, _ = r.Defs.Top("x")
	got, _ = AsInteger(v)
	if got != 2 {
		t.Errorf("after second push, top = %d, want 2", got)
	}

	// Pop reveals the original.
	r.Defs.Pop("x")
	v, _ = r.Defs.Top("x")
	got, _ = AsInteger(v)
	if got != 1 {
		t.Errorf("after pop, top = %d, want 1", got)
	}

	// Final pop empties the stack.
	r.Defs.Pop("x")
	if r.Defs.Has("x") {
		t.Error("after final pop, x should be gone")
	}
}

func TestSnapshotRestoreDefDepths(t *testing.T) {
	// SnapshotDefDepths captures a per-name depth map; RestoreToDefDepths
	// truncates each name back to its captured depth. This is the
	// mechanism fn-body sandboxing uses to drop temporary bindings.
	r, _ := NewRegistry()
	r.Defs.Push("a", NewInteger(1))
	snap := r.Defs.Snapshot()

	r.Defs.Push("a", NewInteger(2))
	r.Defs.Push("b", NewInteger(99))
	if d := r.Defs.Depth("a"); d != 2 {
		t.Errorf("depth after pushes: a=%d, want 2", d)
	}

	r.Defs.Restore(snap)
	if d := r.Defs.Depth("a"); d != 1 {
		t.Errorf("depth after restore: a=%d, want 1", d)
	}
	if r.Defs.Has("b") {
		t.Error("b should have been truncated to zero by restore")
	}
}

// --- Native dispatch: multi-arg, multi-overload --------------------------

func TestMultipleSignaturesDispatch(t *testing.T) {
	// Register a "describe" word with two overloads: one for Integer,
	// one for String. The engine must pick the right one based on arg
	// types.
	r, _ := NewRegistry()
	r.RegisterNativeFunc(NativeFunc{
		Name: "describe",

		Signatures: []NativeSig{
			{
				Args: []*Type{TInteger},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					n, _ := AsInteger(args[0])
					if n == 0 {
						return []Value{NewString("zero-int")}, nil
					}
					return []Value{NewString("nonzero-int")}, nil
				},
				Returns: []*Type{TString}, BarrierPos: -1,
			},
			{
				Args: []*Type{TString},
				Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					s, _ := AsString(args[0])
					return []Value{NewString("string:" + s)}, nil
				},
				Returns: []*Type{TString}, BarrierPos: -1,
			},
		},
	})
	r.InitRootContext()

	cases := []struct {
		input []Value
		want  string
	}{
		{[]Value{NewWord("describe"), NewInteger(5)}, "nonzero-int"},
		{[]Value{NewWord("describe"), NewInteger(0)}, "zero-int"},
		{[]Value{NewWord("describe"), NewString("hi")}, "string:hi"},
	}
	for _, c := range cases {
		out, err := NewTop(r).Run(c.input)
		if err != nil {
			t.Errorf("%v: error %v", c.input, err)
			continue
		}
		got, _ := AsString(out[0])
		if got != c.want {
			t.Errorf("%v: got %q, want %q", c.input, got, c.want)
		}
	}
}

func TestSignatureDispatchFavoursSpecificity(t *testing.T) {
	// Generic (Any) and specific (Integer) overloads of the same word.
	// A concrete integer arg must dispatch to the specific overload.
	hits := map[string]int{}
	r, _ := NewRegistry()
	r.RegisterNativeFunc(NativeFunc{
		Name: "tag",

		Signatures: []NativeSig{
			{
				Args: []*Type{TAny},
				Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					hits["any"]++
					return []Value{NewString("any")}, nil
				},
				Returns: []*Type{TString}, BarrierPos: -1,
			},
			{
				Args: []*Type{TInteger},
				Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					hits["int"]++
					return []Value{NewString("int")}, nil
				},
				Returns: []*Type{TString}, BarrierPos: -1,
			},
		},
	})
	r.InitRootContext()

	if _, err := NewTop(r).Run([]Value{NewWord("tag"), NewInteger(7)}); err != nil {
		t.Fatal(err)
	}
	if hits["int"] != 1 || hits["any"] != 0 {
		t.Errorf("specificity broken: hits=%v", hits)
	}

	if _, err := NewTop(r).Run([]Value{NewWord("tag"), NewString("foo")}); err != nil {
		t.Fatal(err)
	}
	if hits["any"] != 1 {
		t.Errorf("string should hit any-overload: hits=%v", hits)
	}
}

// --- Output capture ------------------------------------------------------

func TestOutputCapture(t *testing.T) {
	// Register an "emit" word that writes to r.Output. Confirm we can
	// redirect it to a builder and read back the data — this verifies
	// the engine threads r.Output through to handlers.
	var buf strings.Builder
	r, _ := NewRegistry()
	r.Output = &buf
	r.RegisterNativeFunc(NativeFunc{
		Name: "emit",

		Signatures: []NativeSig{{
			Args: []*Type{TString},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				s, _ := AsString(args[0])
				reg.Output.Write([]byte(s))
				return nil, nil
			},
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	r.InitRootContext()

	if _, err := NewTop(r).Run([]Value{NewWord("emit"), NewString("hello world")}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello world" {
		t.Errorf("captured output = %q, want %q", buf.String(), "hello world")
	}
}

// --- Error reporting ----------------------------------------------------

func TestAqlErrorPropagation(t *testing.T) {
	// A handler that explicitly returns an AqlError must surface that
	// error from Run with the same code.
	r, _ := NewRegistry()
	r.RegisterNativeFunc(NativeFunc{
		Name: "bork",

		Signatures: []NativeSig{{
			Args: []*Type{TInteger},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				return nil, reg.AqlError("test_failure", "always fails", "bork")
			}, BarrierPos: -1,
		}},
	})
	r.InitRootContext()

	_, err := NewTop(r).Run([]Value{NewWord("bork"), NewInteger(0)})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "test_failure") {
		t.Errorf("error should carry the code, got %v", err)
	}
}

// --- Value helpers ------------------------------------------------------

func TestRequireConcreteList(t *testing.T) {
	concrete := NewList([]Value{NewInteger(1), NewInteger(2)})
	rl, err := RequireConcreteList(concrete, "test")
	if err != nil {
		t.Fatalf("concrete list rejected: %v", err)
	}
	if rl.Len() != 2 {
		t.Errorf("len = %d, want 2", rl.Len())
	}

	// A bare TList literal must be rejected.
	lit := NewTypeLiteral(TList)
	if _, err := RequireConcreteList(lit, "test"); err == nil {
		t.Error("expected error for type literal, got nil")
	}
}

func TestNewReadList(t *testing.T) {
	// External constructor: this is the only way to build a ReadList
	// from outside eng.
	src := []Value{NewInteger(1), NewInteger(2), NewInteger(3)}
	rl := NewReadList(src)
	if rl.Len() != 3 {
		t.Fatalf("len = %d, want 3", rl.Len())
	}
	got, _ := AsInteger(rl.Get(1))
	if got != 2 {
		t.Errorf("Get(1) = %d, want 2", got)
	}
}

// --- DefaultFormats moved to the host package; verify formats slot is empty ---

func TestRegistryFormatsStartEmpty(t *testing.T) {
	// eng.NewRegistry deliberately exposes no host concerns —
	// no formats, no file ops, no SQL store, only a generic
	// capability slot. The host package wires every external
	// service in via Registry.SetCapability before running user
	// code. Pinned here so future drift surfaces in CI.
	r, _ := NewRegistry()
	names, err := r.Capabilities.Names()
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected zero capabilities on a fresh registry, got %v", names)
	}
}
