package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Regression for the optional-Options dispatch loop: ExpandOptionalSigs
// used to `continue` past an omitted param whose default could not be
// synthesized — DROPPING the argument from the generated body — and to
// accept defaults that the param's own pattern rejects. Either way the
// generated 0-arg overload re-dispatched the word in a way that matched
// the same overload again: an infinite self-recursion that ground the
// tape to exhaustion (the lang/go/test/options_params.tsv row
// "Optional Options with type-only fields ... omitted triggers error").
//
// The rule now: an omission combination is expanded ONLY when every
// omitted param has a default that passes the same Unify predicate
// dispatch applies. Otherwise no overload is generated and the omission
// fails dispatch honestly.

func optSig(pattern core.Value) core.FnSig {
	return core.FnSig{
		Params: []core.FnParam{{
			Name:     "m",
			Type:     core.TMap,
			Pattern:  &pattern,
			Optional: true,
		}},
		Returns: []*core.Type{core.TMap},
		Impl:    core.Boru([]core.Value{core.NewWord("m")}),
	}
}

func TestExpandOptionalSkipsUnsatisfiableDefault(t *testing.T) {
	// Options{x:Integer} — type-only field, no default. The synthesized
	// omitted value would be {} which the pattern rejects, so NO 0-arg
	// overload may be generated.
	fields := core.NewOrderedMap()
	fields.Set("x", core.NewTypeLiteral(core.TInteger))
	sigs := core.ExpandOptionalSigs("f", []core.FnSig{optSig(core.NewOptionsType(fields))})

	if len(sigs) != 1 {
		t.Fatalf("got %d sigs, want only the original (no omission overload): %+v", len(sigs), sigs)
	}
	if len(sigs[0].Params) != 1 {
		t.Errorf("surviving sig should be the 1-param original, got %d params", len(sigs[0].Params))
	}
}

func TestExpandOptionalKeepsSatisfiableDefault(t *testing.T) {
	// Options{x:1} — concrete default. The omission overload must exist
	// and its body must carry the synthesized {x:1} argument.
	fields := core.NewOrderedMap()
	fields.Set("x", core.NewInteger(1))
	sigs := core.ExpandOptionalSigs("f", []core.FnSig{optSig(core.NewOptionsType(fields))})

	if len(sigs) != 2 {
		t.Fatalf("got %d sigs, want original + 0-arg omission overload", len(sigs))
	}
	zero := sigs[1]
	if len(zero.Params) != 0 {
		t.Fatalf("expansion has %d params, want 0", len(zero.Params))
	}
	// Body must be [Word(f), {x:1}] — the argument must NOT be dropped.
	if len(zero.Body()) != 2 {
		t.Fatalf("expansion body has %d tokens, want 2 (word + default): %v", len(zero.Body()), zero.Body())
	}
	m, err := core.AsMap(zero.Body()[1])
	if err != nil || m == nil {
		t.Fatalf("expansion body[1] is not the default map: %v", zero.Body()[1])
	}
	if v, ok := m.Get("x"); !ok {
		t.Error("default map lost field x")
	} else if n, _ := core.AsInteger(v); n != 1 {
		t.Errorf("default x = %d, want 1", n)
	}
}
