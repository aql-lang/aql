package eng

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// These tests pin the no-panic policy (ADR-005) for the type-
// registration paths: a bad registration must return an error, never
// panic, and a healthy build must report no init error.

func TestBuiltinInitErrorNilOnHealthyBuild(t *testing.T) {
	// The shipped builtinDecls and T* constants must build cleanly.
	if err := core.BuiltinInitError(); err != nil {
		t.Fatalf("BuiltinInitError() = %v, want nil on a healthy build", err)
	}
}

func TestRegisterTypeDuplicateFixedIDErrors(t *testing.T) {
	// A FixedID collision must return an error (it used to panic at the
	// call sites that wrapped this). Use a fresh dynamic table so we do
	// not perturb the shared Builtin table.
	tt := core.NewDynamicTypeTable()
	// Seed a parent the external paths can hang off.
	if _, err := tt.RegisterType("Zeta", 90001, "plugin:test", nil); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, err := tt.RegisterType("Zeta", 90001, "plugin:test", nil)
	if err == nil {
		t.Fatal("duplicate path/FixedID registration returned nil error, want an error")
	}
	if !strings.Contains(err.Error(), "already registered") &&
		!strings.Contains(err.Error(), "collides") {
		t.Errorf("unexpected error %q", err)
	}
}

func TestRegisterTypeBadPathErrors(t *testing.T) {
	tt := core.NewDynamicTypeTable()
	for _, bad := range []string{"", "lower", "Foo//Bar"} {
		if _, err := tt.RegisterType(bad, 90010, "plugin:test", nil); err == nil {
			t.Errorf("RegisterType(%q) = nil error, want rejection", bad)
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
	tt := core.NewDynamicTypeTable()
	_, _ = tt.RegisterType("Quux", 90020, "plugin:test", nil)
	_, _ = tt.RegisterType("Quux", 90020, "plugin:test", nil) // duplicate
	_, _ = tt.RegisterType("", 0, "plugin:test", nil)         // malformed
}
