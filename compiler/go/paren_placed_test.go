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
