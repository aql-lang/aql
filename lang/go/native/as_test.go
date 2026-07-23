package native

import (
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
)

// TestAsValidateDynamicArms pins asValidate's gradual admission over
// check-mode DYNAMIC carriers — the arms spec rows cannot isolate
// directly (a dynamic value with a NARROW bound feeding `as` needs a
// hand-built carrier): a bound that overlaps the target on the tree
// lattice (either conformance direction) passes for the runtime step to
// prove; a provably disjoint bound refuses statically with as_error.
func TestAsValidateDynamicArms(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Bound ⊑ target: proven upcast, passes.
	down := eng.NewDynamicCarrier(TString)
	if verr := asValidate(r, eng.TScalar, down); verr != nil {
		t.Errorf("dynamic(String) as Scalar must pass (bound conforms): %v", verr)
	}
	// Target ⊑ bound: optimistic gradual pass (runtime validates).
	up := eng.NewDynamicCarrier(TAny)
	if verr := asValidate(r, TMap, up); verr != nil {
		t.Errorf("dynamic(Any) as Map must pass gradually: %v", verr)
	}
	// Disjoint bound: the tree lattice refutes it — refuse.
	dis := eng.NewDynamicCarrier(TString)
	verr := asValidate(r, TMap, dis)
	if verr == nil {
		t.Fatal("dynamic(String) as Map must refuse (provably disjoint)")
	}
	if !strings.Contains(verr.Error(), "as_error") {
		t.Errorf("disjoint dynamic refusal should be as_error, got: %v", verr)
	}
}
