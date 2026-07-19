package eng

import "testing"

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
