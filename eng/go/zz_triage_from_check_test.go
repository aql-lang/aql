package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Test blocks re-homed by compiler-driven triage at the carve.

// A delegation member whose inner overload is BORU-BODIED in a FOREIGN
// sub-registry declines tryNativeFnApply's fast path (the body's words
// resolve in the module's own scope, which only the interpreter's
// foreign-wrapper branch provides) — the caller then islands the apply.
func TestTryNativeFnApplyForeignBoruBodyDeclines(t *testing.T) {
	main := seam7Reg(t)
	foreign := seam7Reg(t)
	// A module-preamble boru fn: InstallFnDef registers the body-splicing
	// dispatch handler, so the match succeeds and the boru-impl probe is what
	// declines.
	core.InstallFnDef(foreign, "z9borubody", core.FnDefInfo{Signatures: []core.Signature{
		{Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: core.Boru([]core.Value{core.NewInteger(42)})},
	}})
	member := z9Member("z9borubody", foreign, core.Signature{
		Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: core.Boru([]core.Value{core.NewWord("z9borubody")}),
	})
	vc := seam7VC(main)
	res, done, err := vc.tryNativeFnApply(member, nil)
	if done || err != nil || res != nil {
		t.Errorf("foreign boru-bodied inner must decline the fast path (island territory): res=%v done=%v err=%v",
			res, done, err)
	}
}
