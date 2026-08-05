package core

import (
	"errors"
	"testing"
)

// RunUnit starts a fresh VM run entered at a specific compiled fn unit, binding
// per-call args to the leading param slots and the ref's captures to the
// trailing slots — the durable-callback entry point (serve-raw handlers,
// spawned processes) that fires AFTER the enclosing RunProgram returned. These
// tests drive it with hand-built units, mirroring the vm_steplimit / vm_seam
// style, since eng has no def/fn words to compile a real body.

func runUnitReg(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

// isInternalErr classifies exactly the internal_error BoruError (and nothing
// else): a foreign error and a genuine boru runtime error both report false, so
// InvokeCallback returns them straight through rather than masking with a
// fallback.
func TestIsInternalErr(t *testing.T) {
	if !IsInternalErr(makeBoruError("internal_error", "boom", "", "", "")) {
		t.Error("internal_error BoruError must classify true")
	}
	if IsInternalErr(makeBoruError("signature_error", "no match", "", "", "")) {
		t.Error("a genuine boru runtime error must classify false")
	}
	if IsInternalErr(errors.New("foreign")) {
		t.Error("a non-BoruError must classify false")
	}
	if IsInternalErr(nil) {
		t.Error("nil must classify false")
	}
}
