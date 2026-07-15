package native

import (
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
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
	_, reason, _ := r.Check.Recorder().Finalize(nil)
	if !strings.Contains(reason, "fn-predicate bind is runtime-evaluated") {
		t.Fatalf("declined record must refuse; Finalize reason = %q", reason)
	}
}
