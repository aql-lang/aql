package eng

import "testing"

// TestValueCarriesCarrierMapArm pins the MapPayload arm of the
// interpolation bake probe's recursive walk: a plain map value whose
// ENTRY is a check-mode carrier must report true even though the map's
// own Carrier flag is false. The list arm and the top-level flag are
// exercised by the corpus lanes; a map hole reaches the walk through
// the KeyVal-list shape, so this arm needs a direct MapPayload value.
func TestValueCarriesCarrierMapArm(t *testing.T) {
	withCarrier := NewOrderedMap()
	withCarrier.Set("a", NewCarrier(TInteger))
	if !valueCarriesCarrier(NewValueRaw(TMap, MapPayload{M: withCarrier})) {
		t.Error("map holding a carrier entry must report true")
	}

	concrete := NewOrderedMap()
	concrete.Set("a", NewInteger(1))
	if valueCarriesCarrier(NewValueRaw(TMap, MapPayload{M: concrete})) {
		t.Error("map of concrete entries must report false")
	}
}
