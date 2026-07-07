package vault

// location_seam7_test.go — the uppercase-letter arm of validSuffix
// (location.go:71), otherwise unexercised by the existing suffix table.

import "testing"

func TestS7_ValidSuffixUppercase(t *testing.T) {
	for _, in := range []string{"A", "Work", "aB9", "Z"} {
		if !validSuffix(in) {
			t.Errorf("validSuffix(%q) = false, want true", in)
		}
	}
	// Negative: an uppercase string with a forbidden character still fails.
	if validSuffix("A/B") {
		t.Error(`validSuffix("A/B") = true, want false`)
	}
}
