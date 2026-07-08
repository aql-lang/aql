package eng

import "testing"

// StoreKey's scalar branches: the Integer projection (a numeric map/store key
// formats via FormatInt, not the String/Atom accessors above it).
func TestStoreKeyIntegerBranch(t *testing.T) {
	if got := StoreKey(NewInteger(42)); got != "42" {
		t.Errorf("StoreKey(42) = %q, want \"42\"", got)
	}
}
