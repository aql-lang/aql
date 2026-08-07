package modules

import (
	"errors"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// w9RandListOfHandler returns the rand-list-of native handler bound to a fresh
// seeded state.
func w9RandListOfHandler(t *testing.T) native.Handler {
	t.Helper()
	for _, nf := range randNativesForState(newRandState(1)) {
		if nf.Name == "rand-list-of" {
			return nf.Signatures[0].DispatchHandler()
		}
	}
	t.Fatal("rand-list-of native not found")
	return nil
}

// TestW9RandListOfClosureArms drives rand-list-of's compiled-closure body arms:
// the InvokeBody error (rand.go:376) and the empty-result guard (379). The
// body is a compiled closure and r.Invoker is seamed to control InvokeBody.
func TestW9RandListOfClosureArms(t *testing.T) {
	h := w9RandListOfHandler(t)
	closure := compiler.NewClosure(0, nil) // ClosurePayload → IsCompiledClosure true
	twoArgs := func() []native.Value { return []native.Value{closure, native.NewInteger(1)} }

	// 376: the body invocation errors.
	rErr := mcovReg(t)
	rErr.Invoker = func(_ *native.Registry, _ native.Value, _ []native.Value) ([]native.Value, error) {
		return nil, errors.New("w9 invoke boom")
	}
	if _, err := h(twoArgs(), nil, nil, rErr); err == nil {
		t.Error("rand-list-of: an erroring closure body should propagate")
	}

	// 379: the body produces no value.
	rEmpty := mcovReg(t)
	rEmpty.Invoker = func(_ *native.Registry, _ native.Value, _ []native.Value) ([]native.Value, error) {
		return nil, nil
	}
	if _, err := h(twoArgs(), nil, nil, rEmpty); err == nil {
		t.Error("rand-list-of: a value-less closure body should error")
	}
}

// TestW9CallParseFnCompiledClosure drives callParseFn's compiled-closure arm
// (parse.go:924): a ClosurePayload fn routes through InvokeBody.
func TestW9CallParseFnCompiledClosure(t *testing.T) {
	r := mcovReg(t)
	want := native.NewInteger(42)
	r.Invoker = func(_ *native.Registry, _ native.Value, _ []native.Value) ([]native.Value, error) {
		return []native.Value{want}, nil
	}
	closure := compiler.NewClosure(0, nil)
	out, err := callParseFn(r, closure, []native.Value{native.NewString("x")})
	if err != nil {
		t.Fatalf("callParseFn(compiled closure): %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("callParseFn: got %d results, want 1", len(out))
	}
	if i, _ := out[0].AsConcreteInteger(); i != 42 {
		t.Errorf("callParseFn compiled-closure result = %v, want 42", out[0])
	}
}
