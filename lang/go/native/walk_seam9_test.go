package native

import (
	"errors"
	"testing"
)

// Seam-9 coverage (W9_nativeC) for walk.go (StructUtil.walk): the
// convert-back failure arms, forced via the w9WalkAnyToValue seam.

func w9WalkCb(t *testing.T, r *Registry) Value {
	t.Helper()
	// The callback returns a SCALAR (not the node map) so the traversal
	// terminates — a before-callback that returns the {key value path} map
	// would re-expand every leaf into a new map forever.
	res, err := seam5Run(r, `fn [[m:Any] [Any] [7]]`)
	if err != nil || len(res) == 0 {
		t.Fatalf("build walk callback: %v / %v", res, err)
	}
	return res[len(res)-1]
}

func TestW9WalkConvertBackFailures(t *testing.T) {
	r := seam5Reg(t)
	cb := w9WalkCb(t, r)

	saved := w9WalkAnyToValue
	t.Cleanup(func() { w9WalkAnyToValue = saved })
	w9WalkAnyToValue = func(any) (Value, error) { return Value{}, errors.New("boom-any") }

	data := NewList([]Value{NewInteger(1)})

	// walkHandler: a leaf whose convert-back fails uses the stringified
	// fallback (31.18,33.5).
	if _, err := walkHandler([]Value{data}, nil, nil, r); err != nil {
		t.Fatalf("walkHandler must tolerate a convert-back failure, got %v", err)
	}

	// walkBeforeHandler: the callback's leaf fallback (68.17,70.4) and the
	// result convert-back failure (112.16,114.3).
	if _, err := walkBeforeHandler([]Value{cb, data}, nil, nil, r); err == nil {
		t.Fatalf("walkBeforeHandler result convert-back failure must error")
	}

	// walkBeforeAfterHandler: the result convert-back failure (137.16,139.3).
	if _, err := walkBeforeAfterHandler([]Value{cb, cb, data}, nil, nil, r); err == nil {
		t.Fatalf("walkBeforeAfterHandler result convert-back failure must error")
	}
}
