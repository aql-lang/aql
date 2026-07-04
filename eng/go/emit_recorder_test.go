package eng

import "testing"

// countingRecorder wraps the inactive no-op recorder and counts every probe
// the check pass makes, proving the checker runs against the EmitRecorder
// INTERFACE — no code path requires the concrete *EmitState (G9 / completion
// plan 4.5). Embedding inactiveEmit supplies the full method set; the
// overridden methods tally the calls that every check pass is guaranteed to
// make.
type countingRecorder struct {
	inactiveEmit
	activeCalls int
	armedCalls  int
	recordCalls int
}

func (c *countingRecorder) active() bool { c.activeCalls++; return false }
func (c *countingRecorder) Active() bool { c.activeCalls++; return false }
func (c *countingRecorder) Armed() bool  { c.armedCalls++; return false }
func (c *countingRecorder) RecordCall(string, *Signature, []Value, []Value, SrcPos, bool, bool) {
	c.recordCalls++
}

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
	if r.Check.Recorder() != theInactiveEmit {
		t.Fatalf("fresh registry recorder = %T, want the inactive no-op", r.Check.Recorder())
	}
	done := r.Check.Begin()
	if r.Check.Recorder() != theInactiveEmit {
		t.Fatalf("post-Begin recorder = %T, want the inactive no-op", r.Check.Recorder())
	}
	eng := NewTop(r)
	if _, err := eng.Run([]Value{NewInteger(1), NewWord("padd"), NewInteger(2)}); err != nil {
		t.Fatalf("check run: %v", err)
	}
	if r.Check.Recorder() != theInactiveEmit {
		t.Fatalf("post-run recorder = %T, want the inactive no-op (a plain check must not arm an EmitState)", r.Check.Recorder())
	}
	done()
}

// TestPlainCheckRunsAgainstRecorderInterface swaps a counting NON-EmitState
// recorder into a plain check pass and asserts (a) the pass completes with
// the same diagnostics as the stock pass, and (b) the recorder interface was
// actually consulted — the checker's only coupling to the emit machinery is
// the interface.
func TestPlainCheckRunsAgainstRecorderInterface(t *testing.T) {
	runPass := func(rec EmitRecorder) []CheckDiagnostic {
		r := recorderTestRegistry(t)
		defer r.Check.Begin()()
		if rec != nil {
			r.Check.Emit = rec
		}
		eng := NewTop(r)
		// A dispatch plus a genuine diagnostic (undefined word).
		toks := []Value{
			NewInteger(1), NewWord("padd"), NewInteger(2),
			NewWord("nosuchword"),
		}
		_, _ = eng.Run(toks)
		return append([]CheckDiagnostic(nil), r.Check.Diagnostics...)
	}

	base := runPass(nil)
	fake := &countingRecorder{}
	got := runPass(fake)

	if len(base) != len(got) {
		t.Fatalf("diagnostics diverge under the fake recorder: stock %d, fake %d", len(base), len(got))
	}
	for i := range base {
		if base[i].Code != got[i].Code || base[i].Word != got[i].Word {
			t.Errorf("diagnostic %d diverges: stock %s/%s, fake %s/%s",
				i, base[i].Code, base[i].Word, got[i].Code, got[i].Word)
		}
	}
	if fake.activeCalls == 0 && fake.armedCalls == 0 && fake.recordCalls == 0 {
		t.Fatalf("the check pass never consulted the recorder interface (active=%d armed=%d record=%d)",
			fake.activeCalls, fake.armedCalls, fake.recordCalls)
	}
}
