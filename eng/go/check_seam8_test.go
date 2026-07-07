package eng

// Seam-8 (cluster W8_eng_rest): in-package unit test for the nil-receiver
// guard on CheckState.Clone (check.go). Per design/TEST-SEAMS.10.md.

import "testing"

func TestW8CheckStateCloneNil(t *testing.T) {
	var c *CheckState
	if got := c.Clone(); got != nil {
		t.Fatalf("nil CheckState clones to nil, got %v", got)
	}
}
