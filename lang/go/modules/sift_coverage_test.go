package modules

import (
	_ "embed"
	"testing"
)

//go:embed sift_test.boru
var siftTestBORU string

// siftCoverAllow lists sift.boru rows that are STRUCTURALLY UNREACHABLE through
// the public Sift API — shadowed defense-in-depth guards a caller's earlier
// validation always intercepts (the BORU analog of //covergate:allow). Each
// entry is asserted to actually be uncovered, so it cannot rot.
//
// Both are terminal `else` arms of an exhaustive dispatch over a value that is
// validated against a FIXED SET one call earlier — the validator raises first,
// so the arm is dead through every public entry point:
//   - 136 sift-coerce-val's "unknown type": vtype is checked by sift-known-type
//     (against sift-type-set) at spec build — sift-w-parse-opts, sift-kv-types,
//     sift-fields-types — before sift-coerce reaches it.
//   - 809 sift-run-family's "unknown family": family is checked by
//     sift-known-family in sift-spec-from before sift-run-spec runs it.
var siftCoverAllow = map[int]string{
	136: "shadowed: sift-known-type (sift-type-set) validates vtype at spec build before sift-coerce-val",
	809: "shadowed: sift-known-family validates family in sift-spec-from before sift-run-spec calls sift-run-family",
}

// TestSiftBORUCoverage runs the boru:test suite for sift under the coverage hook
// and asserts (1) every case passes and (2) every executable row of sift.boru is
// covered, save the allowlisted unreachable guards.
func TestSiftBORUCoverage(t *testing.T) {
	assertBORUCoverage(t, "boru:sift", siftSource, siftTestBORU, siftCoverAllow)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
