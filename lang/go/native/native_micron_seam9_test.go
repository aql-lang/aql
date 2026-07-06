package native

import (
	"strings"
	"testing"
)

// Seam-9 coverage (W9_nativeC) for native_micron.go: the Micron
// property-access handlers (get/getr/has on type-literal receivers) and
// the check-mode narrowing ReturnsFn getMicronReturns, driven directly
// with crafted carrier/concrete receivers.

func TestW9MicronHandlerTypeLiteralReceiver(t *testing.T) {
	r := seam5Reg(t)
	lit := NewTypeLiteral(TMicron)

	// getMicronHandler: type-literal receiver rejected (24.23,26.3).
	if _, err := getMicronHandler([]Value{NewString("x"), lit}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "type literal") {
		t.Fatalf("getMicronHandler: expected type-literal error, got %v", err)
	}
	// getrMicronHandler: type-literal receiver rejected (39.23,41.3).
	if _, err := getrMicronHandler([]Value{NewString("x"), lit}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "type literal") {
		t.Fatalf("getrMicronHandler: expected type-literal error, got %v", err)
	}
	// hasMicronHandler: type-literal receiver → false (52.23,54.3).
	out, err := hasMicronHandler([]Value{NewString("x"), lit}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("hasMicronHandler: %v / %v", out, err)
	}
	if b, _ := AsBoolean(out[0]); b {
		t.Fatalf("hasMicronHandler type-literal: expected false, got true")
	}
}

func TestW9GetMicronReturnsBadArgs(t *testing.T) {
	r := seam5Reg(t)
	// Wrong arity → dynamic fallback (114.69,116.3).
	out := getMicronReturns([]Value{NewString("only-one")}, r)
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("getMicronReturns bad args: expected dynamic carrier, got %v", out)
	}
}

func TestW9GetMicronReturnsConcreteReceiver(t *testing.T) {
	r := seam5Reg(t)
	// A concrete Emailon instance.
	res, err := seam5Run(r, `make Emailon 'alice@example.com'`)
	if err != nil || len(res) == 0 {
		t.Fatalf("make Emailon: %v / %v", res, err)
	}
	email := res[len(res)-1]

	// Present property "user": narrows to the field's carrier type
	// (119.22 → 121.39 → NewCarrier(ft)).
	out := getMicronReturns([]Value{NewString("user"), email}, r)
	if len(out) != 1 || out[0].Dynamic {
		t.Fatalf("getMicronReturns user: expected concrete carrier, got %v", out)
	}

	// Absent property → NewCarrier(TNone) (126.3,126.36).
	out = getMicronReturns([]Value{NewString("nope"), email}, r)
	if len(out) != 1 || !out[0].Parent.Equal(TNone) {
		t.Fatalf("getMicronReturns absent: expected None carrier, got %v", out)
	}
}

func TestW9GetMicronReturnsCarrierDefaults(t *testing.T) {
	r := seam5Reg(t)

	// Pathon carrier, key that is neither "parts" nor "abs" → None (137.3,137.36).
	out := getMicronReturns([]Value{NewString("zzz"), NewCarrier(TPathon)}, r)
	if len(out) != 1 || !out[0].Parent.Equal(TNone) {
		t.Fatalf("pathon carrier default: expected None carrier, got %v", out)
	}

	// Urlon carrier, unknown key → None (152.3,152.36).
	out = getMicronReturns([]Value{NewString("zzz"), NewCarrier(TUrlon)}, r)
	if len(out) != 1 || !out[0].Parent.Equal(TNone) {
		t.Fatalf("urlon carrier default: expected None carrier, got %v", out)
	}
}

func TestW9GetMicronReturnsUserSchema(t *testing.T) {
	r := seam5Reg(t)
	// Define a user Micron kind carrying a schema, then obtain its type
	// literal so we can build a carrier receiver.
	res, err := seam5Run(r, `def Baron refine Micron {foo:String}
Baron`)
	if err != nil || len(res) == 0 {
		t.Fatalf("def Baron: %v / %v", res, err)
	}
	baronLit := res[len(res)-1]
	baronCarrier := NewCarrier(baronLit.Parent)

	// Key not in the schema → None (162.3,162.36).
	out := getMicronReturns([]Value{NewString("missing"), baronCarrier}, r)
	if len(out) != 1 || !out[0].Parent.Equal(TNone) {
		t.Fatalf("user schema key-miss: expected None carrier, got %v", out)
	}

	// A schema field typed Function narrows to dynamic (157.70,159.5):
	// callable fields can't be statically carried.
	res, err = seam5Run(r, `def Funcon refine Micron {doit:Function}
Funcon`)
	if err != nil || len(res) == 0 {
		t.Fatalf("def Funcon: %v / %v", res, err)
	}
	funCarrier := NewCarrier(res[len(res)-1].Parent)
	out = getMicronReturns([]Value{NewString("doit"), funCarrier}, r)
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("user schema function-field: expected dynamic carrier, got %v", out)
	}
}
