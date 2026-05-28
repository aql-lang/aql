package shrink

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/stackform"
)

func TestDefaultPolicy_KernelArithmeticIsTransparent(t *testing.T) {
	p := DefaultPolicy()
	for _, w := range []string{"add", "sub", "mul", "div", "mod", "min", "max"} {
		if got := p.Classify(w); got != Transparent {
			t.Errorf("Classify(%q) = %s, want Transparent", w, got)
		}
	}
}

func TestDefaultPolicy_RandPrefixIsGenerator(t *testing.T) {
	p := DefaultPolicy()
	for _, w := range []string{"rand-int", "rand-bool", "rand-float", "rand-string", "rand-one-of"} {
		if got := p.Classify(w); got != Generator {
			t.Errorf("Classify(%q) = %s, want Generator", w, got)
		}
	}
}

func TestDefaultPolicy_TimePrefixIsFrozen(t *testing.T) {
	p := DefaultPolicy()
	for _, w := range []string{"time-tz", "time-unix", "time-now-local"} {
		if got := p.Classify(w); got != Frozen {
			t.Errorf("Classify(%q) = %s, want Frozen", w, got)
		}
	}
}

func TestDefaultPolicy_FetchPrefixIsFrozen(t *testing.T) {
	p := DefaultPolicy()
	for _, w := range []string{"fetch-get", "fetch-post"} {
		if got := p.Classify(w); got != Frozen {
			t.Errorf("Classify(%q) = %s, want Frozen", w, got)
		}
	}
}

func TestDefaultPolicy_UnknownIsOpaque(t *testing.T) {
	p := DefaultPolicy()
	for _, w := range []string{"some-user-word", "domain.calculate", "frobnicate"} {
		if got := p.Classify(w); got != Opaque {
			t.Errorf("Classify(%q) = %s, want Opaque", w, got)
		}
	}
}

func TestDefaultPolicy_ExactBeatsPrefix(t *testing.T) {
	// An exact word in `Words` overrides any prefix match.
	p := DefaultPolicy()
	p.Words["rand-special-pure"] = Transparent
	if got := p.Classify("rand-special-pure"); got != Transparent {
		t.Errorf("exact Words entry should beat prefix; got %s", got)
	}
	// Other rand-* still match the Generator prefix.
	if got := p.Classify("rand-int"); got != Generator {
		t.Errorf("non-overridden rand-* should stay Generator; got %s", got)
	}
}

func TestDefaultPolicy_WeightsAreCalibrated(t *testing.T) {
	p := DefaultPolicy()
	if p.Weight(Transparent) != 0 {
		t.Errorf("Transparent weight = %d, want 0", p.Weight(Transparent))
	}
	if p.Weight(Generator) != 2 {
		t.Errorf("Generator weight = %d, want 2", p.Weight(Generator))
	}
	if p.Weight(Frozen) != 10 {
		t.Errorf("Frozen weight = %d, want 10", p.Weight(Frozen))
	}
	if p.Weight(Opaque) != 5 {
		t.Errorf("Opaque weight = %d, want 5", p.Weight(Opaque))
	}
}

func TestShrinkCost_PureTransparentAddsNoPolicyBias(t *testing.T) {
	// 1 2 add — entirely Transparent. The Call op's Transparent
	// weight is 0, so ShrinkCost should equal stackform.Cost PLUS
	// the literal-complexity contributions (not zero — the cost
	// model knows PushLit(1) is cheaper than PushLit(1000000)).
	form := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(1)}, // 1 base + 1 mag = 2
		stackform.PushLit{V: eng.NewInteger(2)}, // 1 base + 2 mag = 3
		stackform.Call{Name: "add", Arity: 2},   // 2 base + 0 weight = 2
	}}
	p := DefaultPolicy()
	got := ShrinkCost(form, p)
	want := 2 + 3 + 2
	if got != want {
		t.Errorf("ShrinkCost = %d, want %d", got, want)
	}
	// And confirm there's NO policy bias from the Transparent Call.
	// Cost(Call("add")) == 2 + 0 == 2; cost(Call("opaque-word")) would
	// be 2 + Opaque-weight (5) = 7. So swapping the word changes
	// only the Call contribution.
	formOpaque := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(1)},
		stackform.PushLit{V: eng.NewInteger(2)},
		stackform.Call{Name: "unknown-word", Arity: 2},
	}}
	gotOpaque := ShrinkCost(formOpaque, p)
	if gotOpaque-got != p.Weight(Opaque) {
		t.Errorf("opaque vs transparent diff = %d, want %d (Opaque weight)",
			gotOpaque-got, p.Weight(Opaque))
	}
}

func TestShrinkCost_GeneratorAddsBias(t *testing.T) {
	// rand-int call should cost more than an equivalent add.
	gen := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(0)},
		stackform.PushLit{V: eng.NewInteger(100)},
		stackform.Call{Name: "rand-int", Arity: 2},
	}}
	add := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: eng.NewInteger(0)},
		stackform.PushLit{V: eng.NewInteger(100)},
		stackform.Call{Name: "add", Arity: 2},
	}}
	p := DefaultPolicy()
	cGen := ShrinkCost(gen, p)
	cAdd := ShrinkCost(add, p)
	if cGen <= cAdd {
		t.Errorf("Generator should cost more than Transparent: rand=%d add=%d", cGen, cAdd)
	}
	// Diff should match the Generator weight.
	if cGen-cAdd != p.Weight(Generator) {
		t.Errorf("diff = %d, want %d (Generator weight)", cGen-cAdd, p.Weight(Generator))
	}
}

func TestShrinkCost_FrozenAddsLargerBias(t *testing.T) {
	frozen := &stackform.StackForm{Ops: []stackform.Op{
		stackform.Call{Name: "time-now-local", Arity: 0},
	}}
	transp := &stackform.StackForm{Ops: []stackform.Op{
		stackform.Call{Name: "dup", Arity: 1},
	}}
	p := DefaultPolicy()
	if ShrinkCost(frozen, p) <= ShrinkCost(transp, p) {
		t.Errorf("Frozen should cost more than Transparent")
	}
	// Frozen weight (10) > Generator weight (2) > Opaque (5)... wait
	// Opaque is 5 < Frozen 10. Frozen > Opaque > Generator > Transparent.
	if p.Weight(Frozen) <= p.Weight(Opaque) {
		t.Errorf("Frozen weight must exceed Opaque (semantics: never rewrite Frozen)")
	}
	if p.Weight(Opaque) <= p.Weight(Generator) {
		t.Errorf("Opaque weight must exceed Generator (semantics: prefer known generators over unknown words)")
	}
}

func TestShrinkCost_QuoteRecursesIntoBody(t *testing.T) {
	inner := &stackform.StackForm{Ops: []stackform.Op{
		stackform.Call{Name: "rand-int", Arity: 2},
	}}
	form := &stackform.StackForm{Ops: []stackform.Op{
		stackform.Quote{Body: inner},
	}}
	p := DefaultPolicy()
	innerCost := ShrinkCost(inner, p) // 2 (Call base) + 2 (Generator weight) = 4
	// Quote adds 1 + body cost = 5.
	got := ShrinkCost(form, p)
	want := 1 + innerCost
	if got != want {
		t.Errorf("Quote cost = %d, want %d (1 + inner %d)", got, want, innerCost)
	}
}

func TestShrinkCost_NilFormIsZero(t *testing.T) {
	if got := ShrinkCost(nil, DefaultPolicy()); got != 0 {
		t.Errorf("ShrinkCost(nil) = %d, want 0", got)
	}
}

func TestPolicy_NilSafeClassify(t *testing.T) {
	var p *Policy
	if got := p.Classify("anything"); got != Opaque {
		t.Errorf("nil policy Classify = %s, want Opaque", got)
	}
	if got := p.Weight(Transparent); got != 0 {
		t.Errorf("nil policy Weight = %d, want 0", got)
	}
}
