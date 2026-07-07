package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
)

// s7aEmitHandler is a trivial value→string emit handler used to exercise the
// host-registration and install-loop arms without depending on real encoders.
func s7aEmitHandler(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	return []native.Value{native.NewString("x")}, nil
}

// TestS7A_EmitBuildInstallError drives BuildEmitLangModule's install-loop error
// arm (and installHostEmitter's duplicate-key arm) by pre-seeding the host
// state with a spec whose key collides with a built-in kind — bypassing
// RegisterHostEmitter's up-front guards.
func TestS7A_EmitBuildInstallError(t *testing.T) {
	parent := s7aVMReg(t)
	state := emitLangHostStateFor(parent, true)
	state.specs = append(state.specs, EmitLangSpec{Name: "json", Handler: s7aEmitHandler})
	if _, err := BuildEmitLangModule(parent); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected an install collision on the json key, got %v", err)
	}
}

// TestS7A_EmitHostStateNoCreate drives emitLangHostStateFor's create=false arm.
func TestS7A_EmitHostStateNoCreate(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if s := emitLangHostStateFor(r, false); s != nil {
		t.Errorf("expected nil host state when create=false and none exists, got %v", s)
	}
}

// TestS7A_EmitRegisterHostValidation drives RegisterHostEmitter's three
// up-front refusal arms: bad name, nil handler, built-in collision.
func TestS7A_EmitRegisterHostValidation(t *testing.T) {
	r := s7aVMReg(t)
	if err := RegisterHostEmitter(r, EmitLangSpec{Name: "", Handler: s7aEmitHandler}); err == nil ||
		!strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("empty name should be rejected, got %v", err)
	}
	if err := RegisterHostEmitter(r, EmitLangSpec{Name: "okname"}); err == nil ||
		!strings.Contains(err.Error(), "handler must not be nil") {
		t.Errorf("nil handler should be rejected, got %v", err)
	}
	if err := RegisterHostEmitter(r, EmitLangSpec{Name: "json", Handler: s7aEmitHandler}); err == nil ||
		!strings.Contains(err.Error(), "already registered (built-in)") {
		t.Errorf("built-in collision should be rejected, got %v", err)
	}
}

// TestS7A_EmitRegisterHostDuplicate drives the state.specs duplicate arm: two
// host registrations of the same (non-built-in) name.
func TestS7A_EmitRegisterHostDuplicate(t *testing.T) {
	r := s7aVMReg(t)
	if err := RegisterHostEmitter(r, EmitLangSpec{Name: "dupfmt", Handler: s7aEmitHandler}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := RegisterHostEmitter(r, EmitLangSpec{Name: "dupfmt", Handler: s7aEmitHandler}); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Errorf("duplicate host spec should be rejected, got %v", err)
	}
}

// TestS7A_EmitRegisterHostLiveCollision drives RegisterHostEmitter's
// state.live-install arm (and its error propagation) by seeding a live export
// map that already carries the emit_<name> key.
func TestS7A_EmitRegisterHostLiveCollision(t *testing.T) {
	r := s7aVMReg(t)
	state := emitLangHostStateFor(r, true)
	liveExports := native.NewOrderedMap()
	liveExports.Set("emit_zzz", native.NewString("placeholder"))
	sub, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	state.live = &builtEmitLang{exports: liveExports, subReg: sub}
	if err := RegisterHostEmitter(r, EmitLangSpec{Name: "zzz", Handler: s7aEmitHandler}); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Errorf("a live-exports collision should surface, got %v", err)
	}
}

// TestS7A_EmitKindHandlerCodelessError drives emitKindHandler's fallback arm:
// an encode error whose text has no recognised EmitErrorCode maps to the
// generic emit_error code.
func TestS7A_EmitKindHandlerCodelessError(t *testing.T) {
	r := s7aVMReg(t)
	h := emitKindHandler("fake", func(native.Value, map[string]any) (string, error) {
		return "", fmt.Errorf("plain codeless failure")
	})
	_, err := h([]native.Value{native.NewInteger(1), native.NewMap(native.NewOrderedMap())}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "emit_error") {
		t.Errorf("a codeless encode error should map to emit_error, got %v", err)
	}
}

// TestS7A_EmitAutoNoNatural drives emitAutoHandler's no-natural-format arm: a
// Function value has no natural emit kind.
func TestS7A_EmitAutoNoNatural(t *testing.T) {
	r := s7aVMReg(t)
	fn := native.NewFunction(native.FnDefInfo{Name: "f"})
	_, err := emitAutoHandler([]native.Value{fn, native.NewMap(native.NewOrderedMap())}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "emit_no_natural") {
		t.Errorf("a Function has no natural emit kind, got %v", err)
	}
}

// TestS7A_EmitRegisterValidate drives emitRegisterValidate's name / function /
// opts-type refusal arms.
func TestS7A_EmitRegisterValidate(t *testing.T) {
	r := s7aVMReg(t)
	fnGood := native.NewFnDef(native.FnDefInfo{
		Name: "e",
		Signatures: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TAny}, {Type: native.TMap}},
			Returns: []*native.Type{native.TString},
		}},
	})
	// non-atom name → AsConcreteAtom fails
	if _, err := emitRegisterValidate([]native.Value{native.NewTypeLiteral(native.TAtom), fnGood}, r); err == nil ||
		!strings.Contains(err.Error(), "emit_bad_name") {
		t.Errorf("a non-atom name should fail, got %v", err)
	}
	// args[1] not a function value
	if _, err := emitRegisterValidate([]native.Value{native.NewAtom("e"), native.NewInteger(5)}, r); err == nil ||
		!strings.Contains(err.Error(), "expected a function value") {
		t.Errorf("a non-function arg should fail, got %v", err)
	}
	// opts param (Params[1]) not a Map — reached through the register word.
	if _, err := runEmit(t, emitImp+`EmitLang.register wrongopts (fn [[value:Any opts:Integer] [String] ["x"]])`); err == nil ||
		!strings.Contains(err.Error(), "emit_bad_signature") {
		t.Errorf("a non-Map opts param should fail, got %v", err)
	}
}

// TestS7A_EmitRegisterReturnsGuards drives emitRegisterReturns' two early-exit
// guard arms: too few args and a non-function second arg.
func TestS7A_EmitRegisterReturnsGuards(t *testing.T) {
	r := s7aVMReg(t)
	f := emitRegisterReturns(native.NewOrderedMap(), map[string]registerIdent{})
	if out := f([]native.Value{native.NewAtom("x")}, r); out != nil {
		t.Errorf("expected nil for <2 args, got %v", out)
	}
	if out := f([]native.Value{native.NewAtom("x"), native.NewInteger(1)}, r); out != nil {
		t.Errorf("expected nil for a non-function arg, got %v", out)
	}
}
