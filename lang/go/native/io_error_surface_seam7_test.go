package native

import (
	"fmt"
	"strings"
	"testing"
)

// Seam-7 coverage for io_stream.go, native_error_raise.go and
// native_surface.go edge arms (design/TEST-SEAMS.10.md).

func TestB3IsStreamAtomNonAtom(t *testing.T) {
	// Non-atom value -> not a stream (AsConcreteAtom declines).
	if isStreamAtom(NewInteger(1)) {
		t.Fatal("integer must not be a stream atom")
	}
	// Positive pair: a stream-named atom is recognised.
	if !isStreamAtom(NewAtom("stdin")) {
		t.Fatal("stdin atom must be a stream atom")
	}
	if isStreamAtom(NewAtom("notastream")) {
		t.Fatal("unknown atom name must not be a stream atom")
	}
}

func TestB3GetErrorFieldNotError(t *testing.T) {
	r := seam5Reg(t)
	// args[1] is not an error value -> AsError error.
	if _, err := getErrorFieldHandler([]Value{NewString("code"), NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "not an error value") {
		t.Fatalf("getErrorField: expected not-an-error error, got %v", err)
	}
	// Positive pair: reading "message" off a real error value works.
	ev := NewError(fmt.Errorf("boom"))
	out, err := getErrorFieldHandler([]Value{NewString("message"), ev}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("getErrorField(message): unexpected %v / %v", out, err)
	}
	// A BARE (code-less) error reads `code` as the None literal — the
	// branch used to be covered by div-by-zero rows, which now carry
	// arith_error (phase-5 taxonomy normalisation).
	out, err = getErrorFieldHandler([]Value{NewString("code"), ev}, nil, nil, r)
	if err != nil || len(out) != 1 || !ValueType(out[0]).Equal(TNone) {
		t.Fatalf("getErrorField(code) on a bare error: want the None literal, got %v / %v", out, err)
	}
	// And a CODED error reads its code as an atom.
	cv := NewError(MakeAqlError("arith_error", "division by zero", "div", "", ""))
	out, err = getErrorFieldHandler([]Value{NewString("code"), cv}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("getErrorField(code) on a coded error: unexpected %v / %v", out, err)
	}
	if a, aerr := AsAtom(out[0]); aerr != nil || a != "arith_error" {
		t.Fatalf("coded error code = %v, want arith_error atom", out[0])
	}
}

func TestB3SurfaceHandlerNonMap(t *testing.T) {
	r := seam5Reg(t)
	// surface with a non-concrete-map argument -> error.
	if _, err := surfaceHandler([]Value{NewTypeLiteral(TMap)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "concrete map") {
		t.Fatalf("surface: expected concrete-map error, got %v", err)
	}
}

func TestB3ExposesShapeNotFnUndefInfo(t *testing.T) {
	r := seam5Reg(t)
	// A surface whose required shape lacks an FnUndefInfo payload makes
	// exposesHandler report an unknown-shape gap. surfaceHandler itself
	// enforces fnsig shapes, so this defended-against state is built
	// directly (white-box) via SurfaceInfo.
	required := NewOrderedMap()
	required.Set("area", NewInteger(1))
	surf := NewSurfaceType(TSurface, &SurfaceInfo{Name: "S", Required: required, Conform: map[string]bool{}})
	_, err := exposesHandler([]Value{surf, NewTypeLiteral(TInteger)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "not statically known") {
		t.Fatalf("exposes: expected unknown-shape gap, got %v", err)
	}
}
