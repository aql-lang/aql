package eng

import (
	"strings"
	"testing"
)

// fnDefValue builds a Function value with the given name, one sig of the
// given param types, and a body, plus a non-nil Registry whose exports we
// can detect if it ever leaks into rendering.
func fnDefValue(name string, paramTypes []*Type, body []Value) Value {
	params := make([]FnParam, len(paramTypes))
	for i, t := range paramTypes {
		params[i] = FnParam{Type: t}
	}
	reg, _ := NewRegistry()
	// A sentinel binding whose name would appear in any registry dump.
	reg.Defs.Push("LEAK_SENTINEL_EXPORT", NewInteger(1))
	return NewFunction(FnDefInfo{
		Name:       name,
		Sigs:       []FnSig{{Params: params, Returns: []*Type{TInteger}, Body: body, BarrierPos: -1}},
		Signatures: []Signature{{Args: paramTypes, BarrierPos: -1}},
		Registry:   reg,
	})
}

// A function value must render as a compact `fn name(args)` summary and
// must NEVER spill its captured Registry / exports into String(). This is
// the regression guard for the "600-line registry dump on dispatch
// failure" report (DX 3.4).
func TestFnDefStringIsCompactAndHidesRegistry(t *testing.T) {
	v := fnDefValue("inc", []*Type{TInteger}, []Value{NewWord("n"), NewWord("add"), NewInteger(1)})

	got := v.String()
	if want := "fn inc(Integer)"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if strings.Contains(got, "LEAK_SENTINEL_EXPORT") {
		t.Errorf("String() leaked the registry: %q", got)
	}
	// The raw FnDefInfo struct dump (the old %v default) is also forbidden.
	if strings.Contains(got, "Registry") || strings.Contains(got, "0x") {
		t.Errorf("String() looks like a struct dump: %q", got)
	}
}

// A function value embedded in a list renders compactly there too — the
// path that produced the original verbatim body dump.
func TestFnDefInListStringIsCompact(t *testing.T) {
	fn := fnDefValue("inc", []*Type{TInteger}, []Value{NewWord("n")})
	lst := NewList([]Value{fn})
	got := lst.String()
	if want := "[fn inc(Integer)]"; got != want {
		t.Errorf("list String() = %q, want %q", got, want)
	}
	if strings.Contains(got, "LEAK_SENTINEL_EXPORT") {
		t.Errorf("list String() leaked the registry: %q", got)
	}
}

// CanonValue (the ordering surface) must stay DISCRIMINATING: two fns
// that String() renders identically but have different bodies must not
// collapse to one canon string — otherwise cmp/sort treat them as equal.
// It must still never dump the registry.
func TestCanonFnDefDiscriminatesWithoutLeaking(t *testing.T) {
	pos := fnDefValue("P", []*Type{TInteger}, []Value{NewWord("n"), NewWord("gt"), NewInteger(0)})
	neg := fnDefValue("P", []*Type{TInteger}, []Value{NewWord("n"), NewWord("lt"), NewInteger(0)})

	cpos, cneg := CanonValue(pos), CanonValue(neg)
	if cpos == cneg {
		t.Errorf("CanonValue collapsed distinct-bodied fns: both = %q", cpos)
	}
	for _, c := range []string{cpos, cneg} {
		if strings.Contains(c, "LEAK_SENTINEL_EXPORT") {
			t.Errorf("CanonValue leaked the registry: %q", c)
		}
	}
}

// diagValue truncates a long list (e.g. a big quoted fn body) to a head
// plus `… (N more)`, but leaves short lists and non-lists untouched.
func TestDiagValueTruncatesLongList(t *testing.T) {
	elems := make([]Value, 0, 20)
	for i := 0; i < 20; i++ {
		elems = append(elems, NewInteger(int64(i)))
	}
	got := diagValue(NewList(elems))
	if !strings.Contains(got, "… (12 more)") {
		t.Errorf("diagValue long list = %q, want a `… (12 more)` tail", got)
	}
	if strings.Contains(got, "19") {
		t.Errorf("diagValue should have dropped tail elements, got %q", got)
	}

	short := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	if got := diagValue(short); got != "[1 2 3]" {
		t.Errorf("diagValue short list = %q, want [1 2 3]", got)
	}
}
