package eng

import (
	"testing"
)

// Moved from lang/go/native with the Ideal/Module type (ADR-012 stage 2).

func TestModuleBehaviorFormatFallback(t *testing.T) {
	b := moduleTypeBehavior{path: "Ideal/Module"}
	if got := b.Format(NewInteger(3)); got != "Ideal/Module" {
		t.Fatalf("expected path fallback, got %q", got)
	}
	// Positive pair: a Module value formats with its ref.
	if got := b.Format(NewModuleInstance(ModuleDesc{Ref: "boru:x"})); got != "Module(boru:x)" {
		t.Fatalf("expected Module(boru:x), got %q", got)
	}
}

func TestModuleBehaviorConvertFallbacks(t *testing.T) {
	b := moduleTypeBehavior{path: "Ideal/Module"}
	m, err := b.ToMap(NewInteger(1))
	if err != nil {
		t.Fatal(err)
	}
	om, err2 := AsMap(m)
	if err2 != nil || len(om.Keys()) != 0 {
		t.Fatalf("expected empty map fallback, got %v", m)
	}
	l, err := b.ToList(NewInteger(1))
	if err != nil {
		t.Fatal(err)
	}
	elems, err3 := AsList(l)
	if err3 != nil || elems.Len() != 0 {
		t.Fatalf("expected empty list fallback, got %v", l)
	}
	// Positive pair: a real descriptor projects its fields.
	inst := NewModuleInstance(ModuleDesc{Ref: "boru:x", Kind: "file", Exports: map[string]*OrderedMap{"E": nil}})
	m, err = b.ToMap(inst)
	if err != nil {
		t.Fatal(err)
	}
	om, err2 = AsMap(m)
	if err2 != nil || len(om.Keys()) != 5 {
		t.Fatalf("expected the 5 descriptor fields, got %v", m)
	}
	l, err = b.ToList(inst)
	if err != nil {
		t.Fatal(err)
	}
	elems, err3 = AsList(l)
	if err3 != nil || elems.Len() != 1 {
		t.Fatalf("expected the one export name, got %v", l)
	}
}
