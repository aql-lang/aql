package native

import (
	"strings"
	"testing"
)

// Wave-7 coverage for walk.go: makeWalkApply's already-errored short
// circuit, no-signature and body-error callback arms, and the empty-
// result "keep original" arm; plus the callErr propagation in
// walkBeforeHandler / walkBeforeAfterHandler (design/TEST-SEAMS.10.md).
//
// The anyToValue-error arms (walk.go:31, 68, 112, 137) are unreachable
// by input: their argument comes from valueToAny / voxgigstruct.Walk,
// both of which only ever produce JSON-shaped Go values that anyToValue
// accepts totally. Documented residual (would need an anyToValue seam).

func twoEntryMap() Value {
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	om.Set("b", NewInteger(2))
	return NewMap(om)
}

func TestWalkBeforeNoMatchingSignature(t *testing.T) {
	r := b2Reg(t)
	// A 2-param callback never matches the single {key,value,path} arg, so
	// makeWalkApply sets callErr; over a 2-entry map the second node call
	// takes the already-errored short circuit.
	cb := b2Lambda(t, r, "([a:Any b:Any] => [a])")
	_, err := walkBeforeHandler([]Value{cb, twoEntryMap()}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "before callback error") {
		t.Fatalf("walk with 2-param callback: expected sig error, got %v", err)
	}
}

func TestWalkBeforeCallbackBodyError(t *testing.T) {
	r := b2Reg(t)
	cb := b2Lambda(t, r, "([n:Any] => [zzz_undef_word_xyz])")
	_, err := walkBeforeHandler([]Value{cb, twoEntryMap()}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "before callback error") {
		t.Fatalf("walk with erroring callback: expected error, got %v", err)
	}
}

func TestWalkBeforeEmptyResultKeepsOriginal(t *testing.T) {
	r := b2Reg(t)
	// A callback that leaves the stack empty causes makeWalkApply to keep
	// the original node value (walk.go:91).
	cb := b2Lambda(t, r, "([n:Any] => [n drop])")
	out, err := walkBeforeHandler([]Value{cb, twoEntryMap()}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("walk with empty-result callback = %v / %v", out, err)
	}
}

func TestWalkBeforeAfterCallbackError(t *testing.T) {
	r := b2Reg(t)
	// The before/after form propagates a callback error through its own
	// callErr guard.
	bad := b2Lambda(t, r, "([n:Any] => [zzz_undef_word_xyz])")
	ok := b2Lambda(t, r, "([n:Any] => [n.value])")
	_, err := walkBeforeAfterHandler([]Value{bad, ok, twoEntryMap()}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "callback error") {
		t.Fatalf("before/after walk with erroring before: expected error, got %v", err)
	}
	// Positive pair: identity before + after callbacks walk cleanly.
	out, err := walkBeforeAfterHandler([]Value{ok, ok, twoEntryMap()}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("identity before/after walk = %v / %v", out, err)
	}
}

func TestWalkLeafCollection(t *testing.T) {
	r := b2Reg(t)
	// Positive: the 1-arg leaf-collecting form gathers {path,value} rows.
	out, err := walkHandler([]Value{twoEntryMap()}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("walk leaves = %v / %v", out, err)
	}
	rl, _ := AsList(out[0])
	if rl.Len() != 2 {
		t.Fatalf("expected 2 leaf rows, got %d", rl.Len())
	}
	// An empty structure yields an empty (non-nil) list.
	out, err = walkHandler([]Value{NewMap(NewOrderedMap())}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("walk empty = %v / %v", out, err)
	}
}
