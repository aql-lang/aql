package compiler

// NUR057 — unit pins for setDelKernelSig, the binding-identity key that
// replaced the bare `word != "set" && word != "del"` tests in the two
// quote-arg exemptions. The exemptions' argument covers only the kernel
// mutator, so a sig must prove it IS the kernel registration's own Locked
// signature in the live registry — never merely carry the name.

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
		t.Error("the kernel registration's own sig must be admitted")
	}

	// The same word NAME with a sig that is NOT the registry's binding — the
	// shape of an open-words extension dispatch — must decline.
	stray := *kernel
	if setDelKernelSig(r, "set", &stray) {
		t.Error("a copy outside the live binding must decline (pointer identity)")
	}
	unlocked := *kernel
	unlocked.Locked = false
	if setDelKernelSig(r, "set", &unlocked) {
		t.Error("an unlocked sig (a boru extension is never Locked) must decline")
	}

	// Any other word name declines regardless of identity.
	if setDelKernelSig(r, "get", kernel) {
		t.Error("the key is scoped to set/del by name")
	}
	// Nil-safety and an unregistered word.
	if setDelKernelSig(nil, "set", kernel) || setDelKernelSig(r, "set", nil) {
		t.Error("nil registry / sig must decline")
	}
	if setDelKernelSig(r, "del", kernel) {
		t.Error("del is not registered in this registry — no binding, no admission")
	}
}
