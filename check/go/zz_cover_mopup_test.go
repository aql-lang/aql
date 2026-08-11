package check

// The final mop-up: branches the corpus and the family-focused test
// files cannot reach — value-shape arms of the const-fold carrier scan,
// the def-read tagging's dynamic and flex arms, the make mirror's
// typed-map decline, and CheckAtIndices' unknown-length silence.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestValueCarriesCarrierShapes(t *testing.T) {
	// A carrier is a carrier wherever it hides.
	if !valueCarriesCarrier(core.NewCarrier(core.TInteger)) {
		t.Error("a bare carrier carries")
	}
	if !valueCarriesCarrier(core.NewList([]core.Value{core.NewInteger(1), core.NewCarrier(core.TInteger)})) {
		t.Error("a carrier inside a list carries")
	}
	if valueCarriesCarrier(core.NewList([]core.Value{core.NewInteger(1)})) {
		t.Error("a concrete list of concretes does not carry")
	}
	if valueCarriesCarrier(core.NewInteger(1)) {
		t.Error("a concrete scalar does not carry")
	}
}

func TestExprRefsCarrierShapes(t *testing.T) {
	r := newTestRegistry(t)
	e := core.NewTop(r)
	done := r.Check.Begin()
	defer done()

	// A word bound to a carrier on the def stack refs a carrier.
	r.Defs.Push("cw", core.NewCarrier(core.TInteger))
	if !exprRefsCarrier(e, []core.Value{core.NewWord("cw")}) {
		t.Error("a def-bound carrier word refs a carrier")
	}
	// A word bound to a concrete does not.
	r.Defs.Push("kw", core.NewInteger(4))
	if exprRefsCarrier(e, []core.Value{core.NewWord("kw")}) {
		t.Error("a def-bound concrete word does not ref a carrier")
	}
	// The scan recurses into nested lists.
	if !exprRefsCarrier(e, []core.Value{core.NewList([]core.Value{core.NewWord("cw")})}) {
		t.Error("a carrier word nested in a list refs a carrier")
	}
	// An unbound word and a plain literal do not.
	if exprRefsCarrier(e, []core.Value{core.NewWord("nope"), core.NewInteger(1)}) {
		t.Error("unbound words and literals do not ref carriers")
	}
}

func TestTagCheckModeDefRead(t *testing.T) {
	r := newTestRegistry(t)
	e := core.NewTop(r)
	done := r.Check.Begin()
	defer done()

	// A dynamic bound value tags the def it came from, so a later
	// diagnostic can name its origin.
	dyn := core.NewDynamicCarrier(core.TAny)
	tagCheckModeDefRead(e, &dyn, "srcdef")
	if dyn.DynFrom() != "srcdef" {
		t.Errorf("a dynamic def read must tag its origin, got %q", dyn.DynFrom())
	}
	// A concrete read is left untagged.
	conc := core.NewInteger(3)
	tagCheckModeDefRead(e, &conc, "srcdef")
	if conc.DynFrom() != "" {
		t.Error("a concrete def read must stay untagged")
	}
}

func TestCheckMakeConstructionTypedMapDeclines(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	fields := core.NewOrderedMap()
	fields.Set("n", core.NewTypeLiteral(core.TInteger))
	ct := r.Types.MintType("Class/TM", core.TClass)
	cls := core.NewClassType(ct, core.ClassTypeInfo{Fields: fields, Name: "TM", Type: ct})

	// A typed-map literal conforms to TMap but AsMap declines it — the
	// provided==nil arm. Value-dependent, so the mirror stays silent.
	tm := core.NewTypedMap(core.NewTypeLiteral(core.TInteger))
	CheckMakeConstruction(r, cls, tm, core.SrcPos{})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("typed-map source must decline silently, got %+v", r.Check.Diagnostics)
	}

	// A carrier field value is value-dependent and skipped, while a
	// concrete wrong-typed sibling still flags.
	src := mapOf("n", core.NewCarrier(core.TString))
	CheckMakeConstruction(r, cls, src, core.SrcPos{Row: 3, Col: 1})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("carrier field value must be skipped, got %+v", r.Check.Diagnostics)
	}
}

func TestCheckAtIndicesUnknownLength(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	idxs := core.NewList([]core.Value{core.NewInteger(99)})
	// Unknown data length: silent regardless of the index.
	CheckAtIndices(r, idxs, core.NewCarrier(core.TList), "atq")
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("unknown length must stay silent, got %+v", r.Check.Diagnostics)
	}
	// Non-concrete indices over a known length: silent too.
	data := core.NewList([]core.Value{core.NewInteger(1)})
	CheckAtIndices(r, core.NewCarrier(core.TList), data, "atq")
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("non-concrete indices must stay silent, got %+v", r.Check.Diagnostics)
	}
}
