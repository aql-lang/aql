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

// TestIndexProvablyOOB pins the soundness boundary: an index is flagged
// only when every value it could take is outside [0, n). Concrete
// in-bounds indices and value-less carriers must never be flagged.
func TestIndexProvablyOOB(t *testing.T) {
	const n = 2
	cases := []struct {
		name    string
		idx     Value
		n       int
		wantOOB bool
	}{
		// Concrete integers.
		{"past end", NewInteger(5), n, true},
		{"at length boundary", NewInteger(2), n, true},
		{"last valid", NewInteger(1), n, false},
		{"first", NewInteger(0), n, false},
		{"negative", NewInteger(-1), n, true},
		{"empty list, index 0", NewInteger(0), 0, true},
		// DepScalar lower bound → too-high detection.
		{"gte past end", NewDepScalar(DepGTE, NewInteger(5)), n, true},
		{"gt to boundary", NewDepScalar(DepGT, NewInteger(1)), n, true}, // min 2 >= 2
		{"gte in range", NewDepScalar(DepGTE, NewInteger(1)), n, false}, // min 1 < 2
		// DepScalar upper bound → negative detection.
		{"lt zero is negative", NewDepScalar(DepLT, NewInteger(0)), 5, true}, // max -1 < 0
		{"lte zero not negative", NewDepScalar(DepLTE, NewInteger(0)), 5, false},
		// Unknown index (value-less carrier) is never provably OOB.
		{"value-less carrier", NewCarrier(TInteger), n, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			detail, oob := indexProvablyOOB(tc.idx, tc.n)
			if oob != tc.wantOOB {
				t.Errorf("indexProvablyOOB(%v, %d) = %v (%q), want %v", tc.idx, tc.n, oob, detail, tc.wantOOB)
			}
			if oob && detail == "" {
				t.Errorf("flagged index produced an empty detail message")
			}
		})
	}
}
