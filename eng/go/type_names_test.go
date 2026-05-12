package eng

import "testing"

// TestTypeNameTableConsistency verifies that TypeNameTable and TypeNameByID
// are mutually consistent against the Builtin TypeTable.
func TestTypeNameTableConsistency(t *testing.T) {
	full := TypeNameTable()

	if len(full) == 0 {
		t.Fatal("TypeNameTable is empty")
	}

	// Every entry must round-trip name → *Type → ID → name.
	for name, ft := range full {
		if ft == nil {
			t.Errorf("TypeNameTable[%q] has nil Def", name)
			continue
		}
		if ft.ID == "" {
			continue // tolerate transitional entries
		}
		got := TypeNameByID(ft.ID)
		if got != name {
			t.Errorf("TypeNameByID(%s) = %q, want %q", ft.ID, got, name)
		}
	}
}

// TestTypeNameTableNoDuplicates ensures every entry in the Builtin byName
// table is non-ambiguous (single stack entry at init, no duplicates).
func TestTypeNameTableNoDuplicates(t *testing.T) {
	for name, stack := range Builtin.byName {
		if len(stack) != 1 {
			t.Errorf("Builtin.byName[%q] has %d entries at init; expected 1", name, len(stack))
		}
	}
}
