package native

import (
	"testing"
)

// Seam-9 coverage (W9_nativeC) for items.go: the non-string-key
// stringify fallback (21.10,23.4). voxgigstruct.Items always keys pairs
// with strings, so the arm is only reachable by seaming Items to return
// a non-string key.
func TestW9ItemsNonStringKey(t *testing.T) {
	saved := w9ItemsFn
	t.Cleanup(func() { w9ItemsFn = saved })
	w9ItemsFn = func(any) [][2]any {
		return [][2]any{{42, "v"}} // a non-string key forces the fallback
	}

	r := seam5Reg(t)
	om := NewOrderedMap()
	om.Set("x", NewInteger(1))
	out, err := itemsHandler([]Value{NewMap(om)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("itemsHandler: %v / %v", out, err)
	}
	lst, _ := AsList(out[0])
	if lst.Len() != 1 {
		t.Fatalf("expected one pair, got %d", lst.Len())
	}
	pair, _ := AsList(lst.Get(0))
	if k, _ := AsString(pair.Get(0)); k != "42" {
		t.Fatalf("non-string key must stringify to \"42\", got %q", k)
	}
}
