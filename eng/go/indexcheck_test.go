package eng

import "testing"

// TestStaticListLen pins the length-recovery rule: a concrete list
// reports its exact element count, a length-refined carrier reports its
// refinement, and everything else (unrefined carrier, non-list, type
// literal) reports "unknown" so the index checker stays silent.
func TestStaticListLen(t *testing.T) {
	cases := []struct {
		name    string
		v       Value
		wantLen int
		wantOK  bool
	}{
		{"concrete two-element list", NewList([]Value{NewInteger(10), NewInteger(20)}), 2, true},
		{"concrete empty list", NewList([]Value{}), 0, true},
		{"length-refined carrier", NewCarrierTypedListLen(TInteger, 3), 3, true},
		{"unrefined list carrier", NewCarrierTypedList(TInteger), 0, false},
		{"plain list carrier", NewCarrier(TList), 0, false},
		{"non-list value", NewInteger(5), 0, false},
		{"bare list type literal", NewTypeLiteral(TList), 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, ok := StaticListLen(tc.v)
			if ok != tc.wantOK || (ok && n != tc.wantLen) {
				t.Errorf("StaticListLen = (%d, %v), want (%d, %v)", n, ok, tc.wantLen, tc.wantOK)
			}
		})
	}
}
