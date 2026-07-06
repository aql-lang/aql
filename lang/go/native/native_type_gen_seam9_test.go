package native

import (
	"testing"
)

// W9_nativeB — coverage for native_type_gen.go ofHandler's check-mode
// Never-instantiation warning. See design/TEST-SEAMS.10.md.

func TestW9OfNeverInstantiationWarns(t *testing.T) {
	r := seam5Reg(t)
	if _, err := seam5Run(r, `def Box gen [T] refine Record [value:T]`); err != nil {
		t.Fatalf("def Box: %v", err)
	}
	// Instantiating a schema with Never is value-uninhabited: in check mode
	// `of` emits a static_warning diagnostic.
	before := len(r.Check.Diagnostics)
	if _, err := seam5Check(r, `Box of [Never]`); err != nil {
		t.Fatalf("Box of [Never]: %v", err)
	}
	if len(r.Check.Diagnostics) <= before {
		t.Fatal("Box of [Never] should emit a value-uninhabited static_warning")
	}
	found := false
	for _, d := range r.Check.Diagnostics {
		if d.Code == "static_warning" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a static_warning diagnostic for the Never instantiation")
	}
}
