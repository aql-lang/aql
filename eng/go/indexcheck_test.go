package eng

import (
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestStaticListLen pins the length-recovery rule: a concrete list
// reports its exact element count, a length-refined carrier reports its
// refinement, and everything else (unrefined carrier, non-list, type
// literal) reports "unknown" so the index checker stays silent.
func TestStaticListLen(t *testing.T) {
	cases := []struct {
		name    string
		v       core.Value
		wantLen int
		wantOK  bool
	}{
		{"concrete two-element list", core.NewList([]core.Value{core.NewInteger(10), core.NewInteger(20)}), 2, true},
		{"concrete empty list", core.NewList([]core.Value{}), 0, true},
		{"length-refined carrier", check.NewCarrierTypedListLen(core.TInteger, 3), 3, true},
		{"unrefined list carrier", core.NewCarrierTypedList(core.TInteger), 0, false},
		{"plain list carrier", core.NewCarrier(core.TList), 0, false},
		{"non-list value", core.NewInteger(5), 0, false},
		{"bare list type literal", core.NewTypeLiteral(core.TList), 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, ok := check.StaticListLen(tc.v)
			if ok != tc.wantOK || (ok && n != tc.wantLen) {
				t.Errorf("StaticListLen = (%d, %v), want (%d, %v)", n, ok, tc.wantLen, tc.wantOK)
			}
		})
	}
}
