package test

import (
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
)

// Phase 0 (design/MACROS-PHASE1.0.md §7): gensym mints fresh, never-colliding
// atoms for capture-free temporaries.
func TestGensymUniqueAndMonotonic(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{`[gensym gensym gensym]`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	l, err := eng.AsList(res[0])
	if err != nil || l.Len() != 3 {
		t.Fatalf("expected 3 gensyms, got %v", res[0])
	}
	seen := map[string]bool{}
	for i := 0; i < l.Len(); i++ {
		v := l.Get(i)
		if !eng.IsAtom(v) {
			t.Fatalf("gensym[%d] should be an Atom, got %s", i, v.Parent)
		}
		name, _ := eng.AsAtom(v)
		if !strings.HasPrefix(name, "tmp$G") {
			t.Errorf("gensym name %q should start with tmp$G", name)
		}
		if seen[name] {
			t.Errorf("gensym produced a duplicate name %q", name)
		}
		seen[name] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected 3 distinct gensym names, got %d", len(seen))
	}

	// Negative: two gensyms never compare equal.
	res, err = runNativeSteps(t, nil, []string{`gensym eq gensym`})
	if err != nil {
		t.Fatalf("eq run: %v", err)
	}
	if b, _ := eng.AsBoolean(res[0]); b {
		t.Error("gensym eq gensym should be false (always distinct)")
	}
}
