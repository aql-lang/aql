package native

import (
	"strings"
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
)

// The fn-predicate bind's DECLINED-record refusal (the concrete-permitting
// twin of recordTypedBindOrRefuse): with an ACTIVE recorder and a body whose
// operand has no resolvable provenance, RecordTypedBind declines and the
// refuse closure marks the program uncompilable — never the silent bake the
// 2026-07-15 flip attempt caught.
func TestRecordTypedBindOrRefuseConcreteDecline(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Check.Begin()()
	defer r.Check.BeginCompilePass()()
	dyn := eng.NewCarrier(TAny)
	dyn.Dynamic = true
	dyn.ID = ""
	cons := eng.NewCarrier(TAny)
	out := recordTypedBindOrRefuseConcrete(r, func() eng.TypedBindSpec {
		return eng.TypedBindSpec{Kind: eng.TypedBindPredicate, Name: "zz", Describe: "Zz", Cons: &cons}
	}, dyn, dyn, eng.SrcPos{}, func() { markFnPredicateBindUncompilable(r, "zz") })
	if !out.Dynamic {
		t.Fatal("the declined record must return the bound value unchanged")
	}
	_, reason, _ := r.Check.Recorder().(*eng.EmitState).Finalize(nil)
	if !strings.Contains(reason, "fn-predicate bind is runtime-evaluated") {
		t.Fatalf("declined record must refuse; Finalize reason = %q", reason)
	}
}

// miniHookToksMaterialisable's decline arms: a carrier/dynamic token and a
// non-inert marker token (a Forward) both divert the compile pass onto the
// standard transducer call.
func TestMiniHookToksMaterialisableArms(t *testing.T) {
	if !miniHookToksMaterialisable([]eng.Value{NewString("x"), NewWord("w"), NewEnd()}) {
		t.Fatal("inert tokens must be materialisable")
	}
	if miniHookToksMaterialisable([]eng.Value{NewCarrier(TAny)}) {
		t.Fatal("a carrier token must not be materialisable")
	}
	dyn := NewCarrier(TAny)
	dyn.Dynamic = true
	if miniHookToksMaterialisable([]eng.Value{dyn}) {
		t.Fatal("a dynamic token must not be materialisable")
	}
	if miniHookToksMaterialisable([]eng.Value{eng.NewExtension(TAny, 1)}) {
		t.Fatal("an extension-payload token must not be materialisable")
	}
}
