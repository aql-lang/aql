package native

import (
	"testing"
)

// W9_nativeB — coverage for native_macro.go handlers and the namespace
// resolution helpers (mini / parse / emit). The namespace helpers each do
// an independent Defs.Top + asModuleExportInfo lookup; the guard arms are
// driven directly by binding the namespace to a plain value or leaving it
// unbound. See design/TEST-SEAMS.10.md.

// --- macroHandler arg guards ---

func TestW9MacroHandlerNonConcreteList(t *testing.T) {
	r := seam5Reg(t)
	if _, err := macroHandler([]Value{NewTypeLiteral(TList)}, nil, nil, r); err == nil {
		t.Fatal("macro: non-concrete list must error")
	}
}

func TestW9MacroHandlerNonWordParam(t *testing.T) {
	r := seam5Reg(t)
	// [[5] [body]] — the single param is a literal, not a bare name.
	spec := NewList([]Value{
		NewList([]Value{NewInteger(5)}),
		NewList([]Value{NewWord("body")}),
	})
	if _, err := macroHandler([]Value{spec}, nil, nil, r); err == nil {
		t.Fatal("macro: non-word param must error")
	}
}

func TestW9MacroHandlerBareValueBody(t *testing.T) {
	r := seam5Reg(t)
	// [[a] 5] — a bare (non-list) body is wrapped into a single-element body.
	spec := NewList([]Value{
		NewList([]Value{NewWord("a")}),
		NewInteger(5),
	})
	out, err := macroHandler([]Value{spec}, nil, nil, r)
	if err != nil {
		t.Fatalf("macro bare body: %v", err)
	}
	if len(out) != 1 || !out[0].Parent.Equal(TFunction) {
		t.Fatalf("macro should yield a Function, got %v", Canon(out))
	}
}

// --- miniHandler / parseHandler: kind must be a literal ---

func TestW9MiniHandlerKindNotLiteral(t *testing.T) {
	r := seam5Reg(t)
	if _, err := miniHandler([]Value{NewInteger(5), NewString("x"), NewMap(NewOrderedMap())}, nil, nil, r); err == nil {
		t.Fatal("mini: non-literal kind must error")
	}
}

func TestW9ParseHandlerKindNotLiteral(t *testing.T) {
	r := seam5Reg(t)
	if _, err := parseHandler([]Value{NewInteger(5), NewString("x")}, nil, nil, r); err == nil {
		t.Fatal("parse: non-literal kind must error")
	}
}

// --- mini namespace helpers ---

func TestW9MiniKindExportUnbound(t *testing.T) {
	r := seam5Reg(t) // MiniLang not imported
	if _, ok := miniKindExport(r, "lang_re"); ok {
		t.Fatal("miniKindExport must fail when MiniLang is unbound")
	}
}

func TestW9MiniKindExportNonModuleExport(t *testing.T) {
	r := seam5Reg(t)
	r.Defs.Push("MiniLang", NewInteger(5)) // bound, but not a ModuleExport
	if _, ok := miniKindExport(r, "lang_re"); ok {
		t.Fatal("miniKindExport must fail when MiniLang is not a ModuleExport")
	}
}

func TestW9MiniPartialFnWrapperMissing(t *testing.T) {
	r := seam5Reg(t) // MiniLang unbound → miniKindExport fails
	if _, ok := miniPartialFn(r, "re", "lang_re", []Value{NewEnd()}); ok {
		t.Fatal("miniPartialFn must decline when the wrapper is missing")
	}
}

func TestW9MiniPartialFnWrapperNotFnDef(t *testing.T) {
	r := seam5Reg(t)
	fields := NewOrderedMap()
	fields.Set("lang_re", NewInteger(5)) // export is not an FnDef
	r.Defs.Push("MiniLang", NewModuleExport("MiniLang", fields, Value{}))
	if _, ok := miniPartialFn(r, "re", "lang_re", []Value{NewEnd()}); ok {
		t.Fatal("miniPartialFn must decline when the export is not an FnDef")
	}
}

func TestW9MiniCompileExportUnbound(t *testing.T) {
	r := seam5Reg(t)
	if _, ok := miniCompileExport(r, "re"); ok {
		t.Fatal("miniCompileExport must fail when MiniLang is unbound")
	}
}

func TestW9MiniCompileExportNonModuleExport(t *testing.T) {
	r := seam5Reg(t)
	r.Defs.Push("MiniLang", NewInteger(5))
	if _, ok := miniCompileExport(r, "re"); ok {
		t.Fatal("miniCompileExport must fail when MiniLang is not a ModuleExport")
	}
}

func TestW9MiniInvokeAQLCompileNotMacro(t *testing.T) {
	r := seam5Reg(t)
	// A non-macro (here a plain non-Function) compile hook is rejected.
	if _, err := miniInvokeAQLCompile(r, "re", NewInteger(5), "src", NewMap(NewOrderedMap())); err == nil {
		t.Fatal("miniInvokeAQLCompile must reject a non-macro hook")
	}
}

func TestW9MiniKindRegisteredNonModuleExport(t *testing.T) {
	r := seam5Reg(t)
	r.Defs.Push("MiniLang", NewInteger(5))
	if miniKindRegistered(r, "lang_re") {
		t.Fatal("miniKindRegistered must be false when MiniLang is not a ModuleExport")
	}
}

// --- parse namespace helpers + deferred-kind tracking ---

func TestW9ParseKindRegisteredNonModuleExport(t *testing.T) {
	r := seam5Reg(t)
	r.Defs.Push("ParseLang", NewInteger(5))
	if parseKindRegistered(r, "parse_calc") {
		t.Fatal("parseKindRegistered must be false when ParseLang is not a ModuleExport")
	}
}

func TestW9EmitKindRegisteredNonModuleExport(t *testing.T) {
	r := seam5Reg(t)
	r.Defs.Push("EmitLang", NewInteger(5))
	if emitKindRegistered(r, "emit_json") {
		t.Fatal("emitKindRegistered must be false when EmitLang is not a ModuleExport")
	}
}

func TestW9MarkParseKindDeferredGuards(t *testing.T) {
	// Nil registry and empty kind are no-ops.
	MarkParseKindDeferred(nil, "x")
	r := seam5Reg(t)
	MarkParseKindDeferred(r, "")
	if parseKindDeferred(r, "") {
		t.Fatal("empty kind must not be recorded")
	}
	// First call creates the set; second call reuses the existing map.
	MarkParseKindDeferred(r, "k1")
	MarkParseKindDeferred(r, "k2")
	if !parseKindDeferred(r, "k1") || !parseKindDeferred(r, "k2") {
		t.Fatal("both deferred kinds must be recorded")
	}
}

func TestW9ResetParseDeferredKindsGuard(t *testing.T) {
	// Nil registry is a no-op (no panic).
	ResetParseDeferredKinds(nil)
}

func TestW9InstallParseDeferredDispatchGuard(t *testing.T) {
	// Nil registry / nil sig are no-ops (no panic).
	InstallParseDeferredDispatch(nil, nil)
	r := seam5Reg(t)
	InstallParseDeferredDispatch(r, nil)
}

// --- recordDeferredParseDispatch (compile-recorder arms) ---

func TestW9RecordDeferredParseDispatchNoDispatcher(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	// No dispatcher installed → the sig lookup fails and recording declines.
	if _, ok := recordDeferredParseDispatch(r, []Value{NewAtom("calc"), NewString("x")}); ok {
		t.Fatal("recordDeferredParseDispatch must decline without an installed dispatcher")
	}
}

func TestW9RecordDeferredParseDispatchThreeArgs(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	InstallParseDeferredDispatch(r, &Signature{
		Args:       []*Type{TAtom, TAny, TAny},
		Returns:    []*Type{TAny},
		BarrierPos: -1,
	})
	// Three args → the opts-middle / source-last split branch.
	out, ok := recordDeferredParseDispatch(r,
		[]Value{NewAtom("calc"), NewMap(NewOrderedMap()), NewString("x + y")})
	if !ok {
		t.Fatal("recordDeferredParseDispatch should record with a dispatcher installed")
	}
	if !out.Dynamic {
		t.Fatalf("deferred parse result should be a dynamic carrier, got %s", out.String())
	}
}
