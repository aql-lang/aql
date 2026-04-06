package engine

import "testing"

// TestTypeNameTableConsistency verifies that TypeNameTable and
// ParseTimeTypeNames are consistent subsets of the canonical registry
// and that no entry is missing from the full table.
func TestTypeNameTableConsistency(t *testing.T) {
	full := TypeNameTable()
	parseTime := ParseTimeTypeNames()

	if len(full) == 0 {
		t.Fatal("TypeNameTable is empty")
	}
	if len(parseTime) == 0 {
		t.Fatal("ParseTimeTypeNames is empty")
	}
	if len(parseTime) >= len(full) {
		t.Fatalf("ParseTimeTypeNames (%d) should be a strict subset of TypeNameTable (%d)",
			len(parseTime), len(full))
	}

	// Every parse-time entry must exist in the full table with the same type.
	for name, pt := range parseTime {
		ft, ok := full[name]
		if !ok {
			t.Errorf("ParseTimeTypeNames has %q but TypeNameTable does not", name)
			continue
		}
		if !ft.Equal(pt) {
			t.Errorf("type mismatch for %q: ParseTimeTypeNames=%v, TypeNameTable=%v", name, pt, ft)
		}
	}

	// Every entry in the canonical list must appear in the full table.
	for _, e := range typeNameEntries {
		if _, ok := full[e.Name]; !ok {
			t.Errorf("typeNameEntries has %q but TypeNameTable does not", e.Name)
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
