package compiler

// NUR057 — unit pins for setDelKernelSig, the binding-identity key that
// replaced the bare `word != "set" && word != "del"` tests in the two
// quote-arg exemptions. The admitted set: a LOCKED sig (a Go registration —
// Locked is stamped only by the registration path, so no runtime
// construction can counterfeit it) or a BORU-BODIED sig (an open-words
// extension, an ordinary CALL_USER capture). A runtime-minted handler sig
// under the name — the usurp-wrapper class the exemptions' old no-shadow
// comment feared — has neither and declines.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestSetDelKernelSigIdentity(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	noop := func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return nil, nil
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "set",
		Signatures: []core.Signature{{
			Args:      []*core.Type{core.TAtom, core.TAny, core.TMap},
			QuoteArgs: map[int]bool{0: true},
			Impl:      core.Go(noop),
			Returns:   []*core.Type{core.TMap},
		}},
	})
	fn := r.Lookup("set")
	if fn == nil || len(fn.Signatures) == 0 {
		t.Fatal("registered set not found")
	}
	kernel := &fn.Signatures[0]
	if !kernel.Locked {
		t.Fatal("a Go registration must arrive Locked — the discriminator depends on it")
	}
	if !setDelKernelSig(r, "set", kernel) {
		t.Error("the kernel registration's own (Locked) sig must be admitted")
	}

	// A boru-BODIED sig under the name (an open-words extension shape):
	// admitted — its /q param is an ordinary CALL_USER capture.
	ext := core.Signature{
		Args:      []*core.Type{core.TAtom, core.TAny, core.TMap},
		QuoteArgs: map[int]bool{0: true},
		Impl:      core.Boru([]core.Value{core.NewInteger(1)}),
	}
	if !setDelKernelSig(r, "set", &ext) {
		t.Error("a boru-bodied extension sig must be admitted")
	}

	// A RUNTIME-MINTED handler sig under the name — not Locked, no boru
	// body (the usurp-wrapper class) — must decline.
	wrapper := core.Signature{
		Args:      []*core.Type{core.TAtom, core.TAny, core.TMap},
		QuoteArgs: map[int]bool{0: true},
		Impl:      core.Go(noop),
	}
	if setDelKernelSig(r, "set", &wrapper) {
		t.Error("a runtime-minted (unlocked, bodiless) sig must decline")
	}

	// Any other word name declines regardless of identity; nil declines.
	if setDelKernelSig(r, "get", kernel) {
		t.Error("the key is scoped to set/del by name")
	}
	if setDelKernelSig(r, "set", nil) {
		t.Error("nil sig must decline")
	}
	if !setDelKernelSig(nil, "del", kernel) {
		t.Error("the registry is not consulted — a Locked del-named sig admits")
	}
}
