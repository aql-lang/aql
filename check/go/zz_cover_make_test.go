package check

// Complements zz_cut_residual_test.go's TestCheckMakeConstruction: that
// test owns the class-type happy path and its three diagnostics; this one
// covers the declines and the Resource path.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestCheckMakeConstructionDeclines(t *testing.T) {
	r := newTestRegistry(t)

	// No-op outside check mode, and on a nil registry: nothing to assert
	// beyond "does not panic and adds nothing".
	CheckMakeConstruction(nil, core.NewInteger(1), core.NewInteger(1), core.SrcPos{})
	CheckMakeConstruction(r, core.NewInteger(1), core.NewInteger(1), core.SrcPos{})

	done := r.Check.Begin()
	defer done()

	fields := core.NewOrderedMap()
	fields.Set("n", core.NewTypeLiteral(core.TInteger))
	ct := r.Types.MintType("Class/Dcl", core.TClass)
	cls := core.NewClassType(ct, core.ClassTypeInfo{Fields: fields, Name: "Dcl", Type: ct})

	// A non-concrete construction map is value-dependent: the runtime
	// constructor owns it, the mirror stays silent.
	CheckMakeConstruction(r, cls, core.NewCarrier(core.TMap), core.SrcPos{})
	// A non-map source likewise.
	CheckMakeConstruction(r, cls, core.NewInteger(3), core.SrcPos{})
	// A target that is neither class nor Resource declines.
	CheckMakeConstruction(r, core.NewInteger(1), mapOf("n", core.NewInteger(1)), core.SrcPos{})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("declines must add no diagnostics, got %+v", r.Check.Diagnostics)
	}
}

func TestCheckMakeConstructionResource(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	fields := core.NewOrderedMap()
	fields.Set("host", core.NewTypeLiteral(core.TString))
	rt := r.Types.MintType("Ideal/Resource/Res", core.TResource)
	res := core.NewResourceType(rt, core.ResourceTypeInfo{Fields: fields, Name: "Res", Type: rt})

	// Clean construction: silent.
	CheckMakeConstruction(r, res, mapOf("host", core.NewString("h")), core.SrcPos{Row: 1, Col: 1})
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("clean resource construction flagged: %+v", r.Check.Diagnostics)
	}

	// An unknown field on the Resource path flags with the resource label.
	CheckMakeConstruction(r, res, mapOf("bogus", core.NewString("h")), core.SrcPos{Row: 2, Col: 1})
	if len(r.Check.Diagnostics) == 0 {
		t.Fatal("unknown resource field must flag")
	}

	// The dedup: the same detail at the same position reports once even
	// when the ReturnsFn runs again for another analysed call shape.
	n := len(r.Check.Diagnostics)
	CheckMakeConstruction(r, res, mapOf("bogus", core.NewString("h")), core.SrcPos{Row: 2, Col: 1})
	if len(r.Check.Diagnostics) != n {
		t.Fatalf("duplicate detail+position must dedup: %d -> %d", n, len(r.Check.Diagnostics))
	}
}
