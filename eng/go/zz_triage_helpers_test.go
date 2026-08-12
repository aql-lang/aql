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

func parenBody(tokens ...core.Value) []core.Value {
	out := []core.Value{core.NewOpenParen()}
	out = append(out, tokens...)
	out = append(out, core.NewCloseParen())
	return out
}

// --- anonymous fn values ----------------------------------------------------

func mapOf(pairs ...any) core.Value {
	m := core.NewOrderedMap()
	for i := 0; i < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1].(core.Value))
	}
	return core.NewMap(m)
}

// --- MakeRecord ------------------------------------------------------------

func runUnitReg(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

func newTestRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

func intStrRecord() core.RecordTypeInfo {
	fields := core.NewOrderedMap()
	fields.Set("a", core.NewTypeLiteral(core.TInteger))
	fields.Set("b", core.NewTypeLiteral(core.TString))
	return core.RecordTypeInfo{Fields: fields}
}

func testClassType() core.ClassTypeInfo {
	fields := core.NewOrderedMap()
	fields.Set("x", core.NewTypeLiteral(core.TInteger))
	fields.Set("y", core.NewTypeLiteral(core.TString))
	return core.ClassTypeInfo{Fields: fields, Name: "Class/Cov", ID: "T_cov000000001"}
}

// registerIslandWord registers a word with the given compile effect and
// arg count and returns the REGISTERED sig pointer (identity matters).
func registerIslandWord(t *testing.T, r *core.Registry, name string, effect core.CompileEffect, argc int, barrier int) *core.Signature {
	t.Helper()
	args := make([]*core.Type, argc)
	for i := range args {
		args[i] = core.TAny
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: name,
		Signatures: []core.Signature{{
			Args:          args,
			CompileEffect: effect,
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewInteger(0)}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: barrier,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration of %s: %v", name, err)
	}
	return &r.Lookup(name).Signatures[0]
}

func moduleGateReg(t *testing.T, module, export string) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	if err := r.Capabilities.Set(core.CapPolicy, denyExportChecker{module: module, export: export}); err != nil {
		t.Fatalf("install checker: %v", err)
	}
	return r
}

// denyExportChecker denies exactly one (module, export) pair and
// implements ModuleCallChecker only.
type denyExportChecker struct{ module, export string }

func pnmRegistry(t *testing.T, sigs []core.Signature) *core.Registry {
	t.Helper()
	r := covRegistry(t, nil)
	r.RegisterNativeFunc(core.NativeFunc{Name: "pnmw", Signatures: sigs})
	if err := r.Err(); err != nil {
		t.Fatalf("register pnmw: %v", err)
	}
	return r
}

func registerUserPolyList(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "clany",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)})}, nil
			}),
			Returns: []*core.Type{core.TAny}, BarrierPos: -1,
		}},
	})
	core.InstallFnDef(r, "upolyl", core.FnDefInfo{
		Signatures: []core.Signature{
			{
				Params:     []core.FnParam{{Name: "l", Type: core.TList}},
				Returns:    []*core.Type{core.TAny},
				Impl:       core.Boru(parenBody(core.NewWord("l"))),
				BarrierPos: core.BarrierAllForward,
			},
			{
				Params:     []core.FnParam{{Name: "s", Type: core.TString}},
				Returns:    []*core.Type{core.TAny},
				Impl:       core.Boru(parenBody(core.NewInteger(-1))),
				BarrierPos: core.BarrierAllForward,
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
