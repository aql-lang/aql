package eng

// barrier_receiver_hint_test.go pins the NUR049 help-text repair: a starved
// pending forward's "group the call in parens" suggestion must not name ONLY
// a form that can never fire. barrierReceiverWord classifies the boundary
// word — any registered signature reading a slot from the enclosing stack
// (BarrierPos < TotalArgs) means a paren group can seal off that slot, so
// strandedForwardError appends the sequential spelling.

import (
	"strings"
	"testing"
)

func TestBarrierReceiverWord(t *testing.T) {
	r := newTestRegistry(t)
	e := NewTop(r)

	// A word with a stack-barrier slot (BarrierPos 1 of 2 — the accessor
	// family's shape): reported.
	r.Register("brw-acc", Signature{
		Args: []*Type{TString, TMap}, BarrierPos: 1,
		Returns: []*Type{TAny},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{args[0]}, nil
		}),
	})
	if !e.barrierReceiverWord("brw-acc") {
		t.Error("a BarrierPos<TotalArgs sig must report a barrier receiver")
	}

	// An all-forward word (BarrierPos == TotalArgs after normalization): not
	// reported — the paren suggestion alone stays correct for it.
	r.Register("brw-fwd", Signature{
		Args: []*Type{TInteger, TInteger}, BarrierPos: -1,
		Returns: []*Type{TInteger},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{args[0]}, nil
		}),
	})
	if e.barrierReceiverWord("brw-fwd") {
		t.Error("an all-forward word must not report a barrier receiver")
	}

	// An unknown word: not reported.
	if e.barrierReceiverWord("brw-none") {
		t.Error("an unregistered word must not report a barrier receiver")
	}
}

func TestForwardParensSuggestionCaveat(t *testing.T) {
	// The shared suggestion must carry the sealed-stack caveat so it never
	// recommends only the dead form (NUR049's `def why (dot message)`).
	s := forwardParensSuggestion("dot")
	if !strings.Contains(s, "group the call in parens") {
		t.Error("the paren fix must stay first")
	}
	if !strings.Contains(s, "run it first") {
		t.Errorf("the sequential fallback must be offered, got %q", s)
	}
}
