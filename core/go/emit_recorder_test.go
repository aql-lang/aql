package core

import "testing"

// recorderTestRegistry builds an eng-only registry with one probe native
// (`padd`), enough for a check pass to exercise dispatch through
// recordDispatchOutcome and emit a genuine diagnostic on an unknown word.
func recorderTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.RegisterNativeFunc(NativeFunc{
		Name: "padd",
		Signatures: []Signature{{
			Args: []*Type{TInteger, TInteger},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := AsInteger(args[0])
				b, _ := AsInteger(args[1])
				return []Value{NewInteger(a + b)}, nil
			}),
			Returns:    []*Type{TInteger},
			BarrierPos: -1,
		}},
	})
	return r
}

// TestPlainCheckUsesInactiveRecorder pins the 4.5 lifecycle: a plain
// (compile-free) check pass runs entirely against the inactive no-op
// recorder — Begin installs it, nothing swaps in an *EmitState, and it is
// still installed when the pass ends.
func TestPlainCheckUsesInactiveRecorder(t *testing.T) {
	r := recorderTestRegistry(t)
	if r.Check.Recorder() != TheInactiveEmit {
		t.Fatalf("fresh registry recorder = %T, want the inactive no-op", r.Check.Recorder())
	}
	done := r.Check.Begin()
	if r.Check.Recorder() != TheInactiveEmit {
		t.Fatalf("post-Begin recorder = %T, want the inactive no-op", r.Check.Recorder())
	}
	eng := NewTop(r)
	if _, err := eng.Run([]Value{NewInteger(1), NewWord("padd"), NewInteger(2)}); err != nil {
		t.Fatalf("check run: %v", err)
	}
	if r.Check.Recorder() != TheInactiveEmit {
		t.Fatalf("post-run recorder = %T, want the inactive no-op (a plain check must not arm an EmitState)", r.Check.Recorder())
	}
	done()
}
