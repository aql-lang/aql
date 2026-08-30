package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestGeneralisedResidualModelsReturn pins the soundness gate that decides
// whether a GENERALISED body analysis — one run against the declared params'
// carriers, as the end-of-pass pending-body drain does — may be held to the
// body's declared return (NUR111).
//
// Each case is one shape MEASURED off a real corpus row; the comment names
// the row, because the whole point of the gate is that three correct spec
// rows were falsely rejected without it.
func TestGeneralisedResidualModelsReturn(t *testing.T) {
	carrier := func(ty *core.Type) core.Value { return core.NewCarrier(ty) }

	cases := []struct {
		name     string
		stk      []core.Value
		declared []*core.Type
		want     bool
		why      string
	}{
		{
			name:     "carrier residual matching the declaration is modelled",
			stk:      []core.Value{carrier(core.TInteger)},
			declared: []*core.Type{core.TBoolean},
			want:     true,
			why:      "fn-value.tsv §13 `def cbad fn [[n:Integer][Boolean][n]]` — the body returns its param, so the residual IS the model and Integer-vs-Boolean is a real finding",
		},
		{
			name:     "conforming carrier residual is still modelled",
			stk:      []core.Value{carrier(core.TBoolean)},
			declared: []*core.Type{core.TBoolean},
			want:     true,
			why:      "the gate decides whether to LOOK, not what the answer is; conformance is checkBodyReturnConformance's call",
		},
		{
			name:     "a residual longer than the declaration is not modelled",
			stk:      []core.Value{carrier(core.TAny), core.NewInteger(5)},
			declared: []*core.Type{core.TString},
			want:     false,
			why:      "fnsig.tsv:L49 `def h fn g:(fnsig Integer String) String [(g 5)]` — g is a carrier, the call is not modelled, and the literal 5 is stranded",
		},
		{
			name:     "stranded literals under an unapplied fn param are not modelled",
			stk:      []core.Value{carrier(core.TAny), core.NewInteger(2), core.NewInteger(3)},
			declared: []*core.Type{core.TInteger},
			want:     false,
			why:      "fnsig.tsv:L50 `def h fn g:T Integer [(g 2 3)]` — residual [T 2 3], three slots for one declared return",
		},
		{
			name:     "a concrete slot at the declared length is not modelled",
			stk:      []core.Value{core.NewMap(core.NewOrderedMap())},
			declared: []*core.Type{core.TString},
			want:     false,
			why:      "module-emitlang.tsv:L93 — a Map param generalises to a concrete {}, which can survive to the residual as a fake return value; length alone would not catch it",
		},
		{
			name:     "a stranded CALLEE disqualifies an all-carrier residual",
			stk:      []core.Value{carrier(core.TFunction), carrier(core.TInteger), carrier(core.TInteger)},
			declared: []*core.Type{core.TBoolean, core.TBoolean, core.TBoolean},
			want:     false,
			why:      "an unmodelled application can strand ONLY carriers at exactly the declared arity (`def h fn [[g:T a:Integer b:Integer] [Boolean Boolean Boolean] [(g a b)]]`); the fn-typed slot is the callee left behind, and without this arm the gate admits the shape and reports three type_errors",
		},
		{
			name:     "an undeclared (anonymous) fn is never held to a return",
			stk:      []core.Value{carrier(core.TInteger)},
			declared: nil,
			want:     false,
			why:      "an anonymous fn declares no return, so there is no obligation to check",
		},
		{
			name:     "a residual shorter than the declaration is not modelled",
			stk:      nil,
			declared: []*core.Type{core.TInteger},
			want:     false,
			why:      "the short side is the runtime's arity error; the generalised analysis cannot tell under-return from a bail",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := generalisedResidualModelsReturn(tc.stk, tc.declared); got != tc.want {
				t.Fatalf("generalisedResidualModelsReturn = %v, want %v\n%s", got, tc.want, tc.why)
			}
		})
	}
}

// TestBodySpanHelpers covers the two helpers the return-contract call sites
// share. Both were inlined at the dispatch site before NUR111 gave the drain
// a second caller; extracting them made each branch a statement of its own,
// and the empty-body branch is not reachable from either caller (the drain
// skips a signature whose body is empty, and the dispatch path only builds a
// returns-fn for a body it analysed), so it is proved here directly.
func TestBodySpanHelpers(t *testing.T) {
	if got := bodyStartPos(nil); got != (core.SrcPos{}) {
		t.Fatalf("bodyStartPos(nil) = %v, want zero", got)
	}
	tok := core.WithPosAt(core.NewInteger(1), core.SrcPos{Row: 3, Col: 7})
	if got := bodyStartPos([]core.Value{tok, core.NewInteger(2)}); got.Row != 3 || got.Col != 7 {
		t.Fatalf("bodyStartPos = %v, want 3:7 (the FIRST token)", got)
	}

	if got := unnamedParamCount(nil); got != 0 {
		t.Fatalf("unnamedParamCount(nil) = %d, want 0", got)
	}
	params := []core.FnParam{{Name: "a"}, {Name: ""}, {Name: "c"}, {Name: ""}}
	if got := unnamedParamCount(params); got != 2 {
		t.Fatalf("unnamedParamCount = %d, want 2", got)
	}
}
