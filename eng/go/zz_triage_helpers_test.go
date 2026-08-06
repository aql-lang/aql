package eng

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

import (
	"errors"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

func parenBody(tokens ...Value) []Value {
	out := []Value{NewOpenParen()}
	out = append(out, tokens...)
	out = append(out, NewCloseParen())
	return out
}

// --- anonymous fn values ----------------------------------------------------

func mapOf(pairs ...any) Value {
	m := NewOrderedMap()
	for i := 0; i < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1].(Value))
	}
	return NewMap(m)
}

// --- MakeRecord ------------------------------------------------------------

func runUnitReg(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

func intStrRecord() RecordTypeInfo {
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	fields.Set("b", NewTypeLiteral(TString))
	return RecordTypeInfo{Fields: fields}
}

func testClassType() ClassTypeInfo {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	fields.Set("y", NewTypeLiteral(TString))
	return ClassTypeInfo{Fields: fields, Name: "Class/Cov", ID: "T_cov000000001"}
}

// registerIslandWord registers a word with the given compile effect and
// arg count and returns the REGISTERED sig pointer (identity matters).
func registerIslandWord(t *testing.T, r *core.Registry, name string, effect core.CompileEffect, argc int, barrier int) *core.Signature {
	t.Helper()
	args := make([]*Type, argc)
	for i := range args {
		args[i] = TAny
	}
	r.RegisterNativeFunc(NativeFunc{
		Name: name,
		Signatures: []Signature{{
			Args:          args,
			CompileEffect: effect,
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewInteger(0)}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: barrier,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration of %s: %v", name, err)
	}
	return &r.Lookup(name).Signatures[0]
}

func moduleGateReg(t *testing.T, module, export string) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	if err := r.Capabilities.Set(CapPolicy, denyExportChecker{module: module, export: export}); err != nil {
		t.Fatalf("install checker: %v", err)
	}
	return r
}

// denyExportChecker denies exactly one (module, export) pair and
// implements ModuleCallChecker only.
type denyExportChecker struct{ module, export string }

func pnmRegistry(t *testing.T, sigs []Signature) *Registry {
	t.Helper()
	r := covRegistry(t, nil)
	r.RegisterNativeFunc(NativeFunc{Name: "pnmw", Signatures: sigs})
	if err := r.Err(); err != nil {
		t.Fatalf("register pnmw: %v", err)
	}
	return r
}

func registerUserPolyList(r *core.Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "clany",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewList([]Value{NewInteger(1), NewInteger(2)})}, nil
			}),
			Returns: []*Type{TAny}, BarrierPos: -1,
		}},
	})
	InstallFnDef(r, "upolyl", FnDefInfo{
		Signatures: []Signature{
			{
				Params:     []FnParam{{Name: "l", Type: TList}},
				Returns:    []*Type{TAny},
				Impl:       Boru(parenBody(NewWord("l"))),
				BarrierPos: BarrierAllForward,
			},
			{
				Params:     []FnParam{{Name: "s", Type: TString}},
				Returns:    []*Type{TAny},
				Impl:       Boru(parenBody(NewInteger(-1))),
				BarrierPos: BarrierAllForward,
			},
		},
	})
}

// z9Member builds a named trivial-delegation fn VALUE over inner natives
// registered in r (the shaped-instance-method class).
func z9Member(name string, r *core.Registry, sigs ...core.Signature) core.Value {
	return core.NewFunction(core.FnDefInfo{Name: name, Registry: r, Signatures: sigs})
}

func (d denyExportChecker) CheckModuleCall(module, export string) error {
	if module == d.module && export == d.export {
		return errModDenied
	}
	return nil
}

var errModDenied = errors.New("zz-policy: module export denied")
