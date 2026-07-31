package eng

import (
	"errors"
	"testing"
)

// W9 registry.go coverage: pooling / debug-stack guards, key/string
// converters, context helpers, nil-receiver guards, and the fn fallback
// dispatch handler arms — all reached by direct calls.

func TestW9PutSubEngineGuards(t *testing.T) {
	r := newTestRegistry(t)
	// nil engine → dropped.
	r.putSubEngine(nil)
	if len(r.enginePool) != 0 {
		t.Error("a nil engine must not be pooled")
	}
	// An oversized tape → dropped (not pooled).
	e := &Engine{}
	e.tape = NewTape(make([]Value, pooledTapeMaxEntries+1), stackHeadroom)
	r.putSubEngine(e)
	if len(r.enginePool) != 0 {
		t.Error("an oversized-tape engine must not be pooled")
	}
}

func TestW9CurrentStackGuards(t *testing.T) {
	r := newTestRegistry(t)
	// No engine running → (nil, false).
	if _, ok := r.CurrentStack(); ok {
		t.Error("no running engine should report false")
	}
	// A pushed engine with no tape → (nil, false).
	r.pushEngine(&Engine{})
	if _, ok := r.CurrentStack(); ok {
		t.Error("an engine with no tape should report false")
	}
	r.popEngine()

	// Negative pointer clamps to 0.
	eNeg := &Engine{}
	eNeg.tape = NewTape([]Value{NewInteger(1)}, stackHeadroom)
	eNeg.pointer = -1
	r.pushEngine(eNeg)
	if stk, ok := r.CurrentStack(); !ok || len(stk) != 0 {
		t.Errorf("negative pointer should clamp to an empty stack, got %v %v", stk, ok)
	}
	r.popEngine()

	// Pointer past the snapshot clamps to the snapshot length.
	eBig := &Engine{}
	eBig.tape = NewTape([]Value{NewInteger(1), NewInteger(2)}, stackHeadroom)
	eBig.pointer = 999
	r.pushEngine(eBig)
	if _, ok := r.CurrentStack(); !ok {
		t.Error("an over-large pointer should still report a stack")
	}
	r.popEngine()
}

func TestW9SeverityForUnknown(t *testing.T) {
	if SeverityFor("w9-not-a-real-code") != SeverityInfo {
		t.Error("an unknown code should default to SeverityInfo")
	}
}

func TestW9PopFnBaselineEmpty(t *testing.T) {
	r := newTestRegistry(t)
	r.PopFnBaseline() // empty stack → no-op, must not panic
}

func TestW9RegisteredWordNamesNil(t *testing.T) {
	if (*Registry)(nil).RegisteredWordNames() != nil {
		t.Error("nil registry should return nil word names")
	}
}

func TestW9FnFallbackSigHandler(t *testing.T) {
	r := newTestRegistry(t)

	// Undefined name → error (top not found).
	undef := r.fnFallbackSig("w9undef")
	if _, err := undef.dispatchHandler()(nil, nil, nil, r); err == nil {
		t.Error("fallback for an undefined name should error")
	}

	// A plain value binding (not a fn) → the signature_error return.
	r.Defs.Push("w9val", NewInteger(3))
	valFb := r.fnFallbackSig("w9val")
	if _, err := valFb.dispatchHandler()(nil, nil, nil, r); err == nil {
		t.Error("fallback for a non-fn binding should error")
	}

	// A 0-arg fn → the courtesy 0-arg dispatch.
	InstallFnDef(r, "w9zero", FnDefInfo{
		Signatures: []Signature{{
			Returns: []*Type{TInteger},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewInteger(42)}, nil
			}),
			BarrierPos: -1,
		}},
	})
	zeroFb := r.fnFallbackSig("w9zero")
	out, err := zeroFb.dispatchHandler()(nil, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("0-arg courtesy dispatch = %v (%v)", out, err)
	}
	if n, _ := AsInteger(out[0]); n != 42 {
		t.Errorf("courtesy dispatch result = %v, want 42", out[0])
	}
}

func TestW9NewRegistryInitError(t *testing.T) {
	orig := builtinInitError
	t.Cleanup(func() { builtinInitError = orig })
	builtinInitError = func() error { return errors.New("w9 init boom") }
	if _, err := NewRegistry(); err == nil {
		t.Error("a builtin init error should propagate from NewRegistry")
	}
}

func TestW9AggregateDispatchSkipsFallback(t *testing.T) {
	r := newTestRegistry(t)
	// A single def entry carrying BOTH a Fallback sig and a boru-bodied sig:
	// the aggregate walk skips the Fallback sig (continue) and unions the
	// real one. Pushed directly so the Fallback flag survives verbatim.
	fd := FnDefInfo{
		Name: "w9agg",
		Signatures: []Signature{
			{Fallback: true, BarrierPos: 0, Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			})},
			{Params: []FnParam{{Name: "n", Type: TInteger}}, Impl: Boru([]Value{NewWord("n")})},
		},
	}
	r.Defs.Push("w9agg", NewFunction(fd))
	agg := r.Lookup("w9agg")
	if agg == nil {
		t.Fatal("aggregate lookup returned nil")
	}
	// The aggregate keeps at most one fallback signature (the synthetic
	// fallback is appended in place of the authored one).
	fallbacks := 0
	for _, s := range agg.Signatures {
		if s.Fallback && len(s.Params) == 0 {
			fallbacks++
		}
	}
	if fallbacks > 1 {
		t.Errorf("aggregate has %d fallback signatures, want at most 1", fallbacks)
	}
}

func TestW9BinaryNumOpNativeError(t *testing.T) {
	nf := BinaryNumOpNative("w9boom", func(_, _ float64) (float64, error) {
		return 0, errors.New("boom")
	})
	h := nf.Signatures[0].dispatchHandler()
	if _, err := h([]Value{NewFloat(1), NewFloat(2)}, nil, nil, nil); err == nil {
		t.Error("an erroring op should propagate the error")
	}
}

func TestW9ValToStringPathon(t *testing.T) {
	p := NewPathon([]string{"a", "b"}, false)
	if got := ValToString(p); got == "" {
		t.Error("ValToString(pathon) should render a non-empty path")
	}
}

func TestW9ContextHelpers(t *testing.T) {
	// A raw registry has NO context layer yet (InitRootContext not called).
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	// Lookup with no context layer misses (store == nil arm).
	if _, ok := ContextStoreLookup(r, "k"); ok {
		t.Error("lookup with no context should miss")
	}
	// ContextSet initialises the root context on first use (store == nil arm).
	r.ContextSet("k", NewInteger(9))
	got, ok := ContextStoreLookup(r, "k")
	if !ok {
		t.Fatal("after ContextSet the key should resolve")
	}
	if n, _ := AsInteger(got); n != 9 {
		t.Errorf("context value = %v, want 9", got)
	}
}

func TestW9RegisterPartNil(t *testing.T) {
	(*Registry)(nil).RegisterPart("x") // nil registry → no-op, must not panic
}

func TestW9StoreKeyKinds(t *testing.T) {
	if got := StoreKey(NewWord("w")); got != "w" {
		t.Errorf("word key = %q", got)
	}
	if got := StoreKey(NewFloat(1.5)); got == "" {
		t.Error("float key should be non-empty")
	}
	if got := StoreKey(NewBoolean(true)); got != "true" {
		t.Errorf("true key = %q", got)
	}
	if got := StoreKey(NewBoolean(false)); got != "false" {
		t.Errorf("false key = %q", got)
	}
	// A concrete list falls through to the default %v rendering.
	if got := StoreKey(NewList([]Value{NewInteger(1)})); got == "" {
		t.Error("list key should render via the default arm")
	}
}

func TestW9CallBoruArgsPushError(t *testing.T) {
	r := newTestRegistry(t)
	r.Args = nil // nil args stack → Push fails, the baseline must unwind
	sig := &FnSig{
		Params: []FnParam{{Name: "n", Type: TInteger}},
		Impl:   Boru([]Value{NewWord("n")}),
	}
	if _, err := r.CallBoru(sig, []Value{NewInteger(1)}, nil); err == nil {
		t.Error("a nil args stack should fail CallBoru")
	}
}

func TestW9TopTypeBodyAndLookupTypeNameNil(t *testing.T) {
	if _, ok := (*Registry)(nil).TopTypeBody("x"); ok {
		t.Error("nil registry TopTypeBody should report false")
	}
	if (*Registry)(nil).LookupTypeName("x") != nil {
		t.Error("nil registry LookupTypeName should return nil")
	}
}
