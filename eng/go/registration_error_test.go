package eng

import (
	"strings"
	"testing"
)

// These tests pin the no-panic policy (ADR-005) for the type-
// registration paths: a bad registration must return an error, never
// panic, and a healthy build must report no init error.

func TestBuiltinInitErrorNilOnHealthyBuild(t *testing.T) {
	// The shipped builtinDecls and T* constants must build cleanly.
	if err := BuiltinInitError(); err != nil {
		t.Fatalf("BuiltinInitError() = %v, want nil on a healthy build", err)
	}
}

func TestRegisterExternalBuiltinDuplicateFixedIDErrors(t *testing.T) {
	// A FixedID collision must return an error (it used to panic at the
	// call sites that wrapped this). Use a fresh dynamic table so we do
	// not perturb the shared Builtin table.
	tt := NewDynamicTypeTable()
	// Seed a parent the external paths can hang off.
	if _, err := tt.RegisterExternalBuiltin("Zeta", 90001, nil); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, err := tt.RegisterExternalBuiltin("Zeta", 90001, nil)
	if err == nil {
		t.Fatal("duplicate path/FixedID registration returned nil error, want an error")
	}
	if !strings.Contains(err.Error(), "already registered") &&
		!strings.Contains(err.Error(), "collides") {
		t.Errorf("unexpected error %q", err)
	}
}

func TestRegisterExternalBuiltinBadPathErrors(t *testing.T) {
	tt := NewDynamicTypeTable()
	for _, bad := range []string{"", "lower", "Foo//Bar"} {
		if _, err := tt.RegisterExternalBuiltin(bad, 90010, nil); err == nil {
			t.Errorf("RegisterExternalBuiltin(%q) = nil error, want rejection", bad)
		}
	}
}

// TestNoPanicOnRepeatedRegistration is the recover()-based guard the
// project requires for the no-panic policy: the conversion must hold
// even when a caller deliberately triggers the failure path.
func TestNoPanicOnRepeatedRegistration(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registration panicked instead of returning an error: %v", r)
		}
	}()
	tt := NewDynamicTypeTable()
	_, _ = tt.RegisterExternalBuiltin("Quux", 90020, nil)
	_, _ = tt.RegisterExternalBuiltin("Quux", 90020, nil) // duplicate
	_, _ = tt.RegisterExternalBuiltin("", 0, nil)         // malformed
}
