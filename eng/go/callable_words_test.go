package eng

import "testing"

// TestRegisterNativeFuncCopiesCallable pins the §4.5 decoupling contract: a
// code-body higher-order word DECLARES its closure shape on its
// NativeFunc.Callable, and RegisterNativeFunc copies that pointer onto EVERY
// one of the word's signatures so the bytecode recorder reads it back via the
// resolved sig.Callable — eng holds no name-keyed callable table. The negative
// half proves an ordinary word's signatures carry no spec, so the recorder
// refuses a fn-valued body for words that do not opt in.
func TestRegisterNativeFuncCopiesCallable(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	spec := &CallableSpec{BodyPos: 1, BodyOut: 0, Inputs: func(_ []Value) []Value { return nil }}
	r.RegisterNativeFunc(NativeFunc{
		Name:     "probe-callable",
		Callable: spec,
		Signatures: []NativeSig{
			{Args: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TList, TMap}, BarrierPos: -1},
		},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("register probe-callable: %v", err)
	}

	fn := r.Lookup("probe-callable")
	if fn == nil {
		t.Fatal("probe-callable did not register")
	}
	own := 0
	for i := range fn.Signatures {
		if fn.Signatures[i].Fallback {
			continue // synthetic aggregate fallback carries no spec; skip it
		}
		own++
		got := fn.Signatures[i].Callable
		if got != spec {
			t.Errorf("sig %d: Callable = %p, want the one declared spec %p on every sig", i, got, spec)
			continue
		}
		if got.BodyPos != 1 || got.BodyOut != 0 {
			t.Errorf("sig %d: spec fields not carried: %+v", i, *got)
		}
	}
	if own != 2 {
		t.Fatalf("expected 2 own signatures to carry the spec, saw %d", own)
	}

	// NEGATIVE: an ordinary word (no Callable declared) carries no spec on any
	// signature — the recorder then refuses a fn-valued body at this word.
	r.RegisterNativeFunc(NativeFunc{
		Name:       "probe-plain",
		Signatures: []NativeSig{{Args: []*Type{TList}, BarrierPos: -1}},
	})
	plain := r.Lookup("probe-plain")
	if plain == nil {
		t.Fatal("probe-plain did not register")
	}
	for i := range plain.Signatures {
		if !plain.Signatures[i].Fallback && plain.Signatures[i].Callable != nil {
			t.Errorf("ordinary word sig %d unexpectedly carries a Callable spec", i)
		}
	}
}
