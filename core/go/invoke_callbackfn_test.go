package core

import "testing"

// InvokeCallbackFn is the native-callback seam: it answers FnHome's question
// — which registry resolves this fn's free words — and dispatches there,
// offering the body to the VM first and falling back to CallBoru. This test
// drives the FALLBACK arm (an unstamped interpreter fn), which is what its
// retired sibling CallBoruFn used to do unconditionally.
//
// Covered here because core/go's standalone gate is by its OWN suite, and
// this function's other callers live in lang. A seam whose only exercise is
// a module above it is exactly what that gate exists to catch.
func TestInvokeCallbackFnRunsInTheDefiningRegistry(t *testing.T) {
	home, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	caller, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	sig := &Signature{
		Params:  []FnParam{{Name: "n", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl:    Boru([]Value{NewWord("n")}),
	}
	// A fn carrying a DEFINING registry runs there, not in the caller's.
	fnDef := &FnDefInfo{Name: "f", Registry: home, Signatures: []Signature{*sig}}
	out, err := InvokeCallbackFn(caller, fnDef, sig, []Value{NewInteger(7)})
	if err != nil {
		t.Fatalf("InvokeCallbackFn: %v", err)
	}
	if n, _ := AsInteger(out[len(out)-1]); n != 7 {
		t.Fatalf("result = %v, want 7", out)
	}
	// A fn with NO defining registry falls back to the caller's — the
	// common case, and byte-identical to the pre-seam behaviour.
	local := &FnDefInfo{Name: "g", Signatures: []Signature{*sig}}
	out, err = InvokeCallbackFn(caller, local, sig, []Value{NewInteger(3)})
	if err != nil {
		t.Fatalf("InvokeCallbackFn (local): %v", err)
	}
	if n, _ := AsInteger(out[len(out)-1]); n != 3 {
		t.Fatalf("result = %v, want 3", out)
	}
}
