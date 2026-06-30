package eng

import "testing"

// Stage 1 (representation) for parameterised mutable-Array carriers.
// See design/ARRAY-ELEMENT-CARRIER{,-ARCH}.0.md. No behaviour change yet —
// nothing mints these carriers in production; these tests pin the shape.

func TestTypedArrayCarrierConstruct(t *testing.T) {
	id := SrcPos{Row: 7, Col: 3, Src: "make"}
	v := NewCarrierTypedArray(TInteger, id)

	if !v.Parent.Equal(TArray) {
		t.Errorf("Parent = %s, want Array", v.Parent.String())
	}
	if !v.Carrier {
		t.Error("typed-array carrier must have Carrier=true so the engine treats it as abstract")
	}
	if got := DataArrayElemTypeFromValue(v); !got.Equal(TInteger) {
		t.Errorf("element = %s, want Integer", got.String())
	}
	if got := DataArrayIDFromValue(v); got != id {
		t.Errorf("ArrayID = %+v, want %+v", got, id)
	}
}

func TestTypedArrayElemDefaultsToAny(t *testing.T) {
	// NEGATIVE: a value that is NOT a typed-array carrier reads back as Any /
	// zero-id, so an untyped `a:Array` behaves exactly as today (gradual Any).
	for _, tc := range []struct {
		name string
		v    Value
	}{
		{"bare-array-literal", NewTypeLiteral(TArray)},
		{"integer", NewInteger(1)},
		{"plain-carrier", NewCarrier(TArray)},
	} {
		if got := DataArrayElemTypeFromValue(tc.v); !got.Equal(TAny) {
			t.Errorf("%s: element = %s, want Any", tc.name, got.String())
		}
		if got := DataArrayIDFromValue(tc.v); (got != SrcPos{}) {
			t.Errorf("%s: ArrayID = %+v, want zero", tc.name, got)
		}
	}
}

func TestTypedArrayCarrierSurvivesCopyAndToCarrier(t *testing.T) {
	// The element type + identity must ride the VALUE through binding/capture
	// (a struct copy) and through toCarrier (which preserves a Carrier=true
	// value's Data via its `if v.Carrier { return v }` guard). This is the
	// channel Spike A proved; pin it as a regression.
	id := SrcPos{Row: 12, Col: 9, Src: "make"}
	orig := NewCarrierTypedArray(TString, id)

	cp := orig // value copy — what def-binding / capture do
	if got := DataArrayElemTypeFromValue(cp); !got.Equal(TString) {
		t.Errorf("after copy: element = %s, want String", got.String())
	}
	if got := DataArrayIDFromValue(cp); got != id {
		t.Errorf("after copy: ArrayID = %+v, want %+v", got, id)
	}

	tc := toCarrier(orig)
	if got := DataArrayElemTypeFromValue(tc); !got.Equal(TString) {
		t.Errorf("after toCarrier: element = %s, want String (Data stripped)", got.String())
	}
	if got := DataArrayIDFromValue(tc); got != id {
		t.Errorf("after toCarrier: ArrayID = %+v, want %+v", got, id)
	}
}

func TestTypedArrayValueElement(t *testing.T) {
	// NewCarrierTypedArrayValue takes an arbitrary carrier element.
	id := SrcPos{Row: 1, Col: 1, Src: "make"}
	inner := NewCarrierTypedArray(TInteger, SrcPos{Row: 2}) // Array<Array<Integer>>
	v := NewCarrierTypedArrayValue(inner, id)
	if !v.Parent.Equal(TArray) || !v.Carrier {
		t.Fatalf("want Array carrier, got Parent=%s carrier=%v", v.Parent.String(), v.Carrier)
	}
	elem := DataArrayElemTypeFromValue(v)
	if !elem.Equal(TArray) {
		t.Errorf("outer element = %s, want Array (nested)", elem.String())
	}
}
