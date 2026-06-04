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
		if !strings.HasPrefix(name, "tmp$g") {
			t.Errorf("gensym name %q should start with tmp$g", name)
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

// Phase 1c+1d: a macro definer + expander. unquote inserts operand forms;
// splice flattens; manual hygiene via gensym keeps an introduced binder from
// capturing a same-named user var.
func TestMacroExpandAndHygiene(t *testing.T) {
	// unless: cond false → runs the body; cond true → the then branch.
	res, err := runNativeSteps(t, nil, []string{
		`def unless (macro [[c body] [ quote [ if unquote c [0] unquote body ] ]])`,
		`unless false [42]`,
	})
	if err != nil {
		t.Fatalf("unless: %v", err)
	}
	if n, _ := eng.AsInteger(res[0]); n != 42 {
		t.Errorf("unless false [42] = %v, want 42", res[0])
	}

	// splice flattens a list operand into the call.
	res, err = runNativeSteps(t, nil, []string{
		`def callit (macro [[f xs] [ quote [ unquote f splice xs ] ]])`,
		`def add3 fn [[a:Integer b:Integer c:Integer][Integer][a add b add c]]`,
		`callit add3 [1 2 3]`,
	})
	if err != nil {
		t.Fatalf("splice: %v", err)
	}
	if n, _ := eng.AsInteger(res[0]); n != 6 {
		t.Errorf("callit add3 [1 2 3] = %v, want 6", res[0])
	}

	// Manual hygiene: the gensym'd temp does NOT capture the user's `tmp`.
	res, err = runNativeSteps(t, nil, []string{
		`def myor (macro [[a b] [ def g (gensym)  quote [ def unquote g unquote a  if unquote g [unquote g] [unquote b] ] ]])`,
		`def tmp 42`,
		`myor false tmp`,
	})
	if err != nil {
		t.Fatalf("myor: %v", err)
	}
	if n, _ := eng.AsInteger(res[0]); n != 42 {
		t.Errorf("hygienic myor false tmp = %v, want 42 (user tmp untouched)", res[0])
	}

	// Negative: unquote outside a template errors.
	if _, err := runNativeSteps(t, nil, []string{`unquote 5`}); err == nil {
		t.Error("unquote outside a macro should error, got nil")
	}
	// Negative: too few operands.
	if _, err := runNativeSteps(t, nil, []string{
		`def m (macro [[a b] [quote [unquote a]]])`, `m 1`,
	}); err == nil {
		t.Error("macro with too few operands should error, got nil")
	}
}
