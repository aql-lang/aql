package native

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
)

// The three kernel type-kinds are registered as Ideals on every
// DefaultRegistry, and `type`'s dispatch resolves through them.
func TestIdeals_KernelKindsRegistered(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Object", "Record", "Table"} {
		if r.Ideals.Get(name) == nil {
			t.Errorf("kernel Ideal %q is not registered", name)
		}
	}
	if id := r.Ideals.For(NewTypeLiteral(TObject)); id == nil || id.Name != "Object" {
		t.Errorf("For(Object literal) = %v, want Object", id)
	}
	if id := r.Ideals.For(NewTypeLiteral(TRecord)); id == nil || id.Name != "Record" {
		t.Errorf("For(Record literal) = %v, want Record", id)
	}
}

// A host-registered Ideal is dispatched by the real `type` word — the
// dynamic extensibility the Ideal registry exists to provide.
func TestIdeals_CustomKindDispatchesThroughType(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	called := false
	r.Ideals.Register(&eng.Ideal{
		Name:    "Stringy",
		Enabled: true,
		Accepts: func(base Value) bool {
			return base.Data == nil && base.VType.Equal(TString)
		},
		Construct: func(base, arg Value, r *Registry) ([]Value, error) {
			called = true
			return []Value{NewTypeLiteral(TString)}, nil
		},
	})
	// `type String {}` — no kernel kind claims a String base, so the
	// custom Ideal handles it.
	runAQL(t, r, []Value{
		NewWord("type"), NewTypeLiteral(TString), NewMap(NewOrderedMap()),
	})
	if !called {
		t.Error("custom Ideal's Construct was not invoked by `type`")
	}
}

// A host-registered Ideal's Instantiate is dispatched by the real
// `make` word.
func TestIdeals_CustomKindInstantiatesThroughMake(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	called := false
	r.Ideals.Register(&eng.Ideal{
		Name:    "Listy",
		Enabled: true,
		Accepts: func(v Value) bool {
			return v.Data == nil && v.VType.Equal(TList)
		},
		Instantiate: func(typ, data Value, r *Registry) ([]Value, error) {
			called = true
			return []Value{NewInteger(0)}, nil
		},
	})
	// `make List 7` — no kernel kind claims a bare List type literal,
	// so the custom Ideal's Instantiate handles it.
	runAQL(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TList), NewInteger(7),
	})
	if !called {
		t.Error("custom Ideal's Instantiate was not invoked by `make`")
	}
}
