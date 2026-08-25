package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestParenPlacedMemberFnGuards covers parenPlacedMemberFn's defensive
// declines. resolveDynamicApply asks it for every residual lead, so a nil
// emit state, a stateless registry and an id-less value all reach it.
func TestParenPlacedMemberFnGuards(t *testing.T) {
	var es *EmitState
	if es.parenPlacedMemberFn(core.NewInteger(1)) {
		t.Error("a nil EmitState must decline")
	}
	live := &EmitState{}
	if live.parenPlacedMemberFn(core.NewInteger(1)) {
		t.Error("an EmitState with no registry must decline")
	}
}

// TestSmallerArityOverloadUnknownWord covers smallerArityOverload's lookup
// miss. tryRecordPoly asks it with the DISPATCHING word's name, and that
// name is not always a registered builtin — a user fn, a module wrapper and
// a def-bound name all reach the same question — so "no such native" is a
// live answer, not a formality. No overload means no SMALLER overload, so
// the mixed-arity decline does not fire.
func TestSmallerArityOverloadUnknownWord(t *testing.T) {
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if smallerArityOverload(reg, "no-such-native-xyz", 2) {
		t.Error("an unregistered word has no overloads at all, so none is smaller")
	}
	// The positive and negative halves over a REGISTERED word, so the
	// lookup-miss arm is pinned against the real answers either side of it:
	// a 2-arg overload is smaller than a 3-operand window and not smaller
	// than a 2-operand one.
	noop := func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return nil, nil
	}
	reg.RegisterNativeFunc(core.NativeFunc{
		Name: "pair-xyz",
		Signatures: []core.Signature{{
			Args:    []*core.Type{core.TAny, core.TAny},
			Impl:    core.Go(noop),
			Returns: []*core.Type{core.TAny},
		}},
	})
	if !smallerArityOverload(reg, "pair-xyz", 3) {
		t.Error("a 2-arg overload is smaller than a 3-operand window")
	}
	if smallerArityOverload(reg, "pair-xyz", 2) {
		t.Error("a 2-arg overload is not SMALLER than a 2-operand window")
	}
}
