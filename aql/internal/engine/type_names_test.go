package engine

import "testing"

// TestTypeNameTableConsistency verifies that typeNameEntries and TypeNameTable
// are consistent — every entry produces a table entry with the correct type.
func TestTypeNameTableConsistency(t *testing.T) {
	full := TypeNameTable()

	if len(full) == 0 {
		t.Fatal("TypeNameTable is empty")
	}
	if len(full) != len(typeNameEntries) {
		t.Fatalf("TypeNameTable has %d entries but typeNameEntries has %d",
			len(full), len(typeNameEntries))
	}

	// Every entry in the canonical list must appear in the full table.
	for _, e := range typeNameEntries {
		ft, ok := full[e.Name]
		if !ok {
			t.Errorf("typeNameEntries has %q but TypeNameTable does not", e.Name)
			continue
		}
		if !ft.Equal(e.Type) {
			t.Errorf("type mismatch for %q: entry=%v, table=%v", e.Name, e.Type, ft)
		}
	}

	// Reverse map must also be consistent.
	for _, e := range typeNameEntries {
		if e.Type.ID == "" {
			continue // runtime types have no fixed ID
		}
		rev, ok := typeNamesByTypeID[e.Type.ID]
		if !ok {
			t.Errorf("typeNamesByTypeID missing reverse entry for %q (ID=%s)", e.Name, e.Type.ID)
			continue
		}
		if rev != e.Name {
			t.Errorf("typeNamesByTypeID[%s] = %q, want %q", e.Type.ID, rev, e.Name)
		}
	}
}

// TestTypeNameTableNoDuplicates ensures no name appears twice in typeNameEntries.
func TestTypeNameTableNoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, e := range typeNameEntries {
		if seen[e.Name] {
			t.Errorf("duplicate entry in typeNameEntries: %q", e.Name)
		}
		seen[e.Name] = true
	}
}
