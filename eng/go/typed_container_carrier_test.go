package eng

import "testing"

// unifyTyped{List,Map}WithConcrete's !IsConcrete guard (typed flex layer) tags
// an abstract CARRIER gradually. Ordinary dispatch routes flex carriers through
// unifyCarrierVsTyped first (and only the shape-tracked MAP carrier reaches the
// map twin), so the LIST arm is dispatch-unreachable — pin both by direct call.
// paramBodyCarrier (poly-arm + construction-check body-input builder) makes a
// {:T}/[:T] param's body reads narrow. Its typed arms are pinned by direct call
// (the census exercises the compile paths; the branch coverage is here).
func TestParamBodyCarrier(t *testing.T) {
	mapPat := NewTypedMap(NewTypeLiteral(TInteger))
	// A {:T} param carrier must be DYNAMIC — the arg may be flex, so an in-place
	// mutation in the body must runtime-rematch rather than preselect the
	// immutable handler (the compile-vs-interpret divergence fix).
	if v := paramBodyCarrier(FnParam{Pattern: &mapPat, Type: TMap}); !IsTypedMap(v) || !v.Dynamic {
		t.Errorf("{:Integer} param → %s (dynamic=%v), want a dynamic typed-map carrier", v.Parent.String(), v.Dynamic)
	}
	listPat := NewCarrierTypedListValue(NewTypeLiteral(TInteger))
	if v := paramBodyCarrier(FnParam{Pattern: &listPat, Type: TList}); !IsTypedList(v) || !v.Dynamic {
		t.Errorf("[:Integer] param → %s (dynamic=%v), want a dynamic typed-list carrier", v.Parent.String(), v.Dynamic)
	}
	// A non-typed-container param falls back to ParamInputCarrier (no pattern).
	if v := paramBodyCarrier(FnParam{Type: TInteger}); v.Parent == nil || !v.Parent.ConformsTo(TInteger) {
		t.Errorf("scalar param → %s, want an Integer carrier", v.Parent.String())
	}
	// A {:Any} pattern (child Any) falls through to ParamInputCarrier(TMap).
	anyPat := NewTypedMap(NewTypeLiteral(TAny))
	if v := paramBodyCarrier(FnParam{Pattern: &anyPat, Type: TMap}); v.Parent == nil || !v.Parent.ConformsTo(TMap) {
		t.Errorf("{:Any} param → %s, want a Map carrier", v.Parent.String())
	}
}

func TestUnifyTypedContainerCarrierGuard(t *testing.T) {
	child := NewTypeLiteral(TInteger)
	lc := NewCarrier(TFlexList)
	out, uerr := unifyTypedListWithConcrete(lc, child)
	if uerr != nil {
		t.Fatalf("list carrier: %v", uerr)
	}
	if c, ok := out.ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("list carrier not tagged with [:Integer]")
	}
	mc := NewCarrier(TFlexMap)
	outm, merr := unifyTypedMapWithConcrete(mc, child)
	if merr != nil {
		t.Fatalf("map carrier: %v", merr)
	}
	if c, ok := outm.ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("map carrier not tagged with {:Integer}")
	}
}

// typedContainerCarrier (D2 Part A) threads a {:T} param's element type into
// the body carrier. Its guard arms are exercised through the compile path by
// the langspec census; the arms that ordinary dispatch cannot reach (a typed
// pattern whose arg is not the matching container family — matchSignature
// rejects that before genArgs) are pinned by direct call.
func TestTypedContainerCarrier(t *testing.T) {
	mapPat := NewTypedMap(NewTypeLiteral(TInteger))
	listPat := NewCarrierTypedListValue(NewTypeLiteral(TInteger))

	// Typed-map pattern + a conforming Map arg → a typed carrier.
	if v, ok := typedContainerCarrier(FnParam{Pattern: &mapPat}, NewCarrier(TMap)); !ok || !IsTypedMap(v) {
		t.Errorf("map pattern + Map arg: got (%v, %v), want a typed-map carrier", v.Parent, ok)
	}
	// Typed-list pattern + a conforming List arg → a typed carrier.
	if v, ok := typedContainerCarrier(FnParam{Pattern: &listPat}, NewCarrier(TList)); !ok || !IsTypedList(v) {
		t.Errorf("list pattern + List arg: got (%v, %v), want a typed-list carrier", v.Parent, ok)
	}
	// Nil pattern → no carrier.
	if _, ok := typedContainerCarrier(FnParam{}, NewCarrier(TMap)); ok {
		t.Errorf("nil pattern should not produce a carrier")
	}
	// {:Any} pattern (child Any) → falls through, no tag.
	anyPat := NewTypedMap(NewTypeLiteral(TAny))
	if _, ok := typedContainerCarrier(FnParam{Pattern: &anyPat}, NewCarrier(TMap)); ok {
		t.Errorf("{:Any} pattern should fall through")
	}
	// Typed-map pattern + a NON-container arg (Integer) → the final
	// fallthrough: neither the map nor the list branch admits it.
	if _, ok := typedContainerCarrier(FnParam{Pattern: &mapPat}, NewInteger(5)); ok {
		t.Errorf("map pattern + Integer arg should fall through")
	}
	// Typed-list pattern + a Map arg (wrong family) → fallthrough too.
	if _, ok := typedContainerCarrier(FnParam{Pattern: &listPat}, NewCarrier(TMap)); ok {
		t.Errorf("list pattern + Map arg should fall through")
	}
}
