package core

// The dispatch-agreement probe seam (dispatch_probe.go), pinned at both
// commit sites: the word-dispatch site (stepWord's match + retry) and the
// fn-value site (execFnDefLiteral's). Each pin also demonstrates the
// contract a census client relies on — positions are TAPE-ABSOLUTE in
// signature order, so Tape.At(positions[i]) is the operand for sig
// position i, and handing that window to the kernel matcher with the
// VM's exact-arity WordInfo selects the same overload here (the
// agreement the corpus census then measures at scale).

import "testing"

type probeEvent struct {
	word   string
	sigs   []Signature
	sig    *Signature
	window []Value
}

func installRecordingProbe(t *testing.T) *[]probeEvent {
	t.Helper()
	events := &[]probeEvent{}
	uninstall := InstallDispatchProbe(func(e *Engine, fn *FnDefInfo, w WordInfo, sig *Signature, positions []int, specAt int) {
		ev := probeEvent{word: fn.Name, sigs: fn.Signatures, sig: sig}
		if sig != nil {
			for _, p := range positions {
				ev.window = append(ev.window, e.Tape.At(p))
			}
		}
		*events = append(*events, ev)
	})
	t.Cleanup(uninstall)
	return events
}

// The word-dispatch site: a successful native dispatch fires the probe
// with the committed sig and tape-absolute positions, and the rebuilt
// sig-order window re-selects the same overload through the kernel
// matcher — the agreement contract, demonstrated on one dispatch.
func TestDispatchProbeWordSite(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	events := installRecordingProbe(t)

	if _, err := e.Run([]Value{NewInteger(2), NewInteger(3), NewWord("cadd")}); err != nil {
		t.Fatalf("run: %v", err)
	}
	var hit *probeEvent
	for i := range *events {
		if (*events)[i].word == "cadd" && (*events)[i].sig != nil {
			hit = &(*events)[i]
			break
		}
	}
	if hit == nil {
		t.Fatalf("no committed cadd dispatch reached the probe; events = %+v", *events)
	}
	if len(hit.window) != 2 {
		t.Fatalf("cadd window = %v, want the two operands", hit.window)
	}
	mr := MatchSignature(hit.sigs, hit.window, WordInfo{ArgCount: len(hit.window)})
	if mr == nil || mr.Sig != hit.sig {
		t.Errorf("kernel matcher over the rebuilt window did not re-select the committed sig (mr=%+v)", mr)
	}
}

// The fn-value site: an anonymous fn value applied to a stack operand
// fires the probe from execFnDefLiteral's commit.
func TestDispatchProbeFnValueSite(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	events := installRecordingProbe(t)

	fnv := anonFnVal(
		[]FnParam{{Name: "n", Type: TInteger}}, []*Type{TInteger},
		parenBody(NewWord("cadd"), NewWord("n"), NewWord("n")))
	e.Tape = NewTape([]Value{NewInteger(4), fnv}, StackHeadroom)
	e.Pointer = 1
	if err := e.execFnDefLiteral(1); err != nil {
		t.Fatalf("execFnDefLiteral: %v", err)
	}
	found := false
	for _, ev := range *events {
		if ev.sig != nil && len(ev.window) == 1 {
			if n, err := AsInteger(ev.window[0]); err == nil && n == 4 {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("the fn-value dispatch commit did not reach the probe; events = %+v", *events)
	}
}

// A 0-arg anonymous lambda alone at the pointer is DATA — the parking
// gate leaves it inert (ADR-016's `def f ([] => [body])` contract) — so
// no dispatch commits and the probe must see NOTHING from the fn-value
// site (Codex P2, PR #420: probing before the gate counted
// non-dispatches as agreements).
func TestDispatchProbeInertZeroArgLambdaSilent(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	events := installRecordingProbe(t)

	fnv := anonFnVal(nil, []*Type{TInteger}, parenBody(NewInteger(42)))
	e.Tape = NewTape([]Value{fnv}, StackHeadroom)
	e.Pointer = 0
	if err := e.execFnDefLiteral(0); err != nil {
		t.Fatalf("execFnDefLiteral: %v", err)
	}
	if e.Pointer != 1 {
		t.Fatalf("the inert lambda must park (pointer advanced), got pointer %d", e.Pointer)
	}
	if len(*events) != 0 {
		t.Errorf("an inert (parked) fn value is not a dispatch commit; probe saw %+v", *events)
	}
}

// A failed dispatch fires the probe with a nil sig (the census counts
// these rather than comparing them), and the uninstaller disarms the
// seam cleanly.
func TestDispatchProbeNoMatchAndUninstall(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)

	events := &[]probeEvent{}
	uninstall := InstallDispatchProbe(func(pe *Engine, fn *FnDefInfo, w WordInfo, sig *Signature, positions []int, specAt int) {
		*events = append(*events, probeEvent{word: fn.Name, sig: sig})
	})

	_, _ = e.Run([]Value{NewWord("cadd")}) // no operands: no signature matches
	sawNil := false
	for _, ev := range *events {
		if ev.word == "cadd" && ev.sig == nil {
			sawNil = true
		}
	}
	if !sawNil {
		t.Error("a failed dispatch must still reach the probe with a nil sig")
	}

	uninstall()
	n := len(*events)
	e2 := NewTop(covRegistry(t, nil))
	if _, err := e2.Run([]Value{NewInteger(1), NewInteger(1), NewWord("cadd")}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(*events) != n {
		t.Error("an uninstalled probe must not fire")
	}
}
