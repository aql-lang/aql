package native

import (
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
)

// Direct arm coverage for superHandler — the spec rows (super.tsv)
// pin the surface; these pin the guards.
func TestSuperHandlerArms(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Non-atom operand (a sig mismatch reaching the handler directly).
	if _, err := superHandler([]Value{eng.NewInteger(1)}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "super_error") {
		t.Fatalf("non-atom operand: %v", err)
	}
	// Unknown word.
	if _, err := superHandler([]Value{eng.NewAtom("zz-nope")}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "super_error") {
		t.Fatalf("unknown word: %v", err)
	}
	// Happy path: the returned FnDef carries quote-cleared copies and
	// the kernel's own registration is untouched.
	out, err := superHandler([]Value{eng.NewAtom("set")}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("super set: %v", err)
	}
	fd, ok := out[0].Data.(eng.FnDefInfo)
	if !ok {
		t.Fatalf("super set returned %T", out[0].Data)
	}
	for i := range fd.Signatures {
		if len(fd.Signatures[i].QuoteArgs) != 0 {
			t.Fatalf("sig %d kept QuoteArgs", i)
		}
		for _, p := range fd.Signatures[i].Params {
			if p.Quote {
				t.Fatalf("sig %d kept a Quote param", i)
			}
		}
	}
	base := r.BuiltinBase("set")
	quoted := false
	for i := range base.Signatures {
		if len(base.Signatures[i].QuoteArgs) > 0 {
			quoted = true
		}
	}
	if !quoted {
		t.Fatal("the kernel set registration lost its QuoteArgs — super mutated shared state")
	}
}
