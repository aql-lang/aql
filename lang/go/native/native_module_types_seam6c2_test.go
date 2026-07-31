package native

import (
	"errors"
	"strings"
	"testing"
)

// Seam-6 coverage for native_module_types.go, typeinit.go, and setup.go
// (design/TEST-SEAMS.10.md).

func TestSeam6C2RegisterModuleTypeDuplicateRecorded(t *testing.T) {
	saved := typeInitErrs
	t.Cleanup(func() { typeInitErrs = saved })
	typeInitErrs = nil

	// The nil arm: recording no error records nothing.
	recordTypeInitErr(nil)
	if TypeInitError() != nil {
		t.Fatal("nil record must not create an init error")
	}

	// Re-registering an existing path records the failure instead of
	// panicking (ADR-005).
	if tt := registerModuleType("Ideal/Module", 5000); tt != nil {
		t.Fatalf("duplicate registration must return nil, got %v", tt)
	}
	err := TypeInitError()
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected already-registered init error, got %v", err)
	}

	// DefaultRegistry surfaces the recorded init error at construction.
	if _, err := DefaultRegistry(); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("DefaultRegistry must surface the init error, got %v", err)
	}
}

func TestSeam6C2DefaultRegistryEngRegistryError(t *testing.T) {
	old := newEngRegistry
	newEngRegistry = func() (*Registry, error) { return nil, errors.New("kernel boom") }
	t.Cleanup(func() { newEngRegistry = old })
	if _, err := DefaultRegistry(); err == nil || !strings.Contains(err.Error(), "kernel boom") {
		t.Fatalf("expected kernel init error, got %v", err)
	}
}

func TestSeam6C2DefaultRegistrySQLiteError(t *testing.T) {
	old := newSQLiteStoreFn
	newSQLiteStoreFn = func() (*SQLiteStore, error) { return nil, errors.New("store boom") }
	t.Cleanup(func() { newSQLiteStoreFn = old })
	if _, err := DefaultRegistry(); err == nil || !strings.Contains(err.Error(), "store boom") {
		t.Fatalf("expected sqlite init error, got %v", err)
	}
}

func TestSeam6C2DefaultRegistryProviderAndErr(t *testing.T) {
	// A provider runs; a bad registration inside it surfaces via r.Err().
	called := false
	_, err := DefaultRegistry(func(r *Registry) {
		called = true
		r.RegisterNativeFunc(NativeFunc{Name: "BadName"})
	})
	if !called {
		t.Fatal("provider must be invoked")
	}
	if err == nil {
		t.Fatal("expected registration error to surface from DefaultRegistry")
	}
	// Positive pair: a well-behaved provider yields a working registry.
	r, err := DefaultRegistry(func(r *Registry) {})
	if err != nil || r == nil {
		t.Fatalf("expected clean registry, got %v", err)
	}
}

func TestSeam6C2NewModuleExportNilFields(t *testing.T) {
	v := NewModuleExport("E", nil, Value{})
	me, ok := asModuleExportInfo(v)
	if !ok || me.Fields == nil {
		t.Fatal("nil fields must be defaulted to an empty map")
	}
}

func TestSeam6C2AsModuleDescNonModule(t *testing.T) {
	if _, ok := AsModuleDesc(NewInteger(1)); ok {
		t.Fatal("integer must not unwrap as ModuleDesc")
	}
	// Positive pair.
	if _, ok := AsModuleDesc(NewModuleInstance(ModuleDesc{ID: "m"})); !ok {
		t.Fatal("module instance must unwrap")
	}
}

func TestSeam6C2ModuleExportGetNilFieldsInfo(t *testing.T) {
	v := Value{Parent: TModuleExport, Data: ExtensionPayload{Body: &moduleExportInfo{Name: "E"}}}
	if _, ok := moduleExportGet(v, "missing"); ok {
		t.Fatal("nil fields must answer no keys")
	}
	// The synthetic keys still resolve.
	if name, ok := moduleExportGet(v, "$name"); !ok {
		t.Fatal("$name must resolve")
	} else if s, _ := AsString(name); s != "E" {
		t.Fatalf("expected export name E, got %v", name)
	}
}

func TestSeam6C2ModuleGetArms(t *testing.T) {
	// Not a module value at all.
	if _, ok := moduleGet(NewInteger(1), "kind"); ok {
		t.Fatal("integer must not answer module fields")
	}
	inst := NewModuleInstance(ModuleDesc{})
	// Kind defaults to inline.
	kind, ok := moduleGet(inst, "kind")
	if !ok {
		t.Fatal("kind must resolve")
	}
	if s, _ := AsString(kind); s != "inline" {
		t.Fatalf("empty kind must default to inline, got %v", kind)
	}
	// Unknown key.
	if _, ok := moduleGet(inst, "nope"); ok {
		t.Fatal("unknown key must not resolve")
	}
}

func TestSeam6C2ModuleInstGetReturnsArms(t *testing.T) {
	// Wrong arity: dynamic Any.
	out := moduleInstGetReturns([]Value{NewString("kind")}, nil)
	if len(out) != 1 || !out[0].Parent.ConformsTo(TAny) {
		t.Fatalf("expected dynamic Any for wrong arity, got %v", out)
	}
	// Unknown key on a concrete pair: dynamic Any.
	inst := NewModuleInstance(ModuleDesc{Kind: "file"})
	out = moduleInstGetReturns([]Value{NewString("nope"), inst}, nil)
	if len(out) != 1 || !out[0].Parent.ConformsTo(TAny) {
		t.Fatalf("expected dynamic Any for unknown key, got %v", out)
	}
	// Positive pairs: closed fields keep their types.
	out = moduleInstGetReturns([]Value{NewString("kind"), inst}, nil)
	if len(out) != 1 || !out[0].Parent.ConformsTo(TString) {
		t.Fatalf("expected String carrier for kind, got %v", out)
	}
	out = moduleInstGetReturns([]Value{NewString("exports"), inst}, nil)
	if len(out) != 1 || !out[0].Parent.ConformsTo(TList) {
		t.Fatalf("expected List carrier for exports, got %v", out)
	}
}

func TestSeam6C2ModuleBehaviorFormatFallback(t *testing.T) {
	b := moduleTypeBehavior{path: "Ideal/Module"}
	if got := b.Format(NewInteger(3)); got != "Ideal/Module" {
		t.Fatalf("expected path fallback, got %q", got)
	}
	// Positive pair: a Module value formats with its ref.
	if got := b.Format(NewModuleInstance(ModuleDesc{Ref: "boru:x"})); got != "Module(boru:x)" {
		t.Fatalf("expected Module(boru:x), got %q", got)
	}
}

func TestSeam6C2ModuleExportHandlersTypeLiteral(t *testing.T) {
	r := seam5Reg(t)
	lit := NewTypeLiteral(TModuleExport)
	if _, err := getModuleExportHandler([]Value{NewString("k"), lit}, nil, nil, r); err == nil {
		t.Fatal("get on a type literal must error")
	}
	if _, err := getrModuleExportHandler([]Value{NewString("k"), lit}, nil, nil, r); err == nil {
		t.Fatal("getr on a type literal must error")
	}
}

func TestSeam6C2ModuleInstHandlers(t *testing.T) {
	r := seam5Reg(t)
	lit := NewTypeLiteral(TModuleInst)
	if _, err := getModuleInstHandler([]Value{NewString("kind"), lit}, nil, nil, r); err == nil {
		t.Fatal("get on a Module type literal must error")
	}
	if _, err := getrModuleInstHandler([]Value{NewString("kind"), lit}, nil, nil, r); err == nil {
		t.Fatal("getr on a Module type literal must error")
	}

	inst := NewModuleInstance(ModuleDesc{Kind: "file", Ref: "m"})
	// get: unknown key answers None.
	out, err := getModuleInstHandler([]Value{NewString("nope"), inst}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsNoneShape(out[0]) {
		t.Fatalf("expected None for unknown key, got %v / %v", out, err)
	}
	// getr: known key answers the field.
	out, err = getrModuleInstHandler([]Value{NewString("kind"), inst}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("expected kind value, got %v / %v", out, err)
	}
	if s, _ := AsString(out[0]); s != "file" {
		t.Fatalf("expected file, got %v", out[0])
	}
	// getr: unknown key raises not_found.
	_, err = getrModuleInstHandler([]Value{NewString("nope"), inst}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "not found in Module") {
		t.Fatalf("expected not_found, got %v", err)
	}
}

func TestSeam6C2ModuleBehaviorConvertFallbacks(t *testing.T) {
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
}
