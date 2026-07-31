package native

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// Wave-6 coverage for capabilities.go: the nil-registry / nil-arg guard
// arms of the SetHostX installers, the policy-uninstall arms of
// SetHostFormats / SetHostExtensions, and every fall-back arm of
// EffectiveFileOps' __sys.fs.mem probe chain.

func TestSetHostDebugOpsNilGuards(t *testing.T) {
	// r == nil arm: must not panic, must be a no-op.
	SetHostDebugOps(nil, scriptedDebugOps{})
	// ops == nil arm on a real registry: slot must stay empty.
	r := seam5Reg(t)
	SetHostDebugOps(r, nil)
	if _, ok := EffectiveDebugOps(r); ok {
		t.Fatal("nil DebugOps must not install anything")
	}
	// Positive pair: a real install is visible.
	SetHostDebugOps(r, scriptedDebugOps{})
	if _, ok := EffectiveDebugOps(r); !ok {
		t.Fatal("expected DebugOps to be installed")
	}
}

// scriptedDebugOps is a minimal DebugOps stand-in for the install test.
type scriptedDebugOps struct{ capabilities.DebugOps }

func TestSetHostClockNilRegistry(t *testing.T) {
	SetHostClock(nil, capabilities.WallClock{}) // must not panic
	// Positive pair: install on a real registry round-trips.
	r := seam5Reg(t)
	SetHostClock(r, capabilities.WallClock{})
	if EffectiveClock(r) == nil {
		t.Fatal("expected a clock")
	}
}

func TestSetHostLogSinksNilRegistry(t *testing.T) {
	SetHostLogSinks(nil, nil) // must not panic
}

func TestSetHostPolicyNilPolicy(t *testing.T) {
	r := seam5Reg(t)
	SetHostPolicy(r, nil)
	if HostPolicy(r) != nil {
		t.Fatal("nil policy must not be installed")
	}
}

func TestHostFileOpsNilRegistry(t *testing.T) {
	if ops := HostFileOps(nil); ops != nil {
		t.Fatalf("HostFileOps(nil) = %T, want nil", ops)
	}
}

// formatsOffPolicy compiles a policy whose formats scope is
// structurally uninstalled (install: false).
func formatsOffPolicy(t *testing.T) policy.Policy {
	t.Helper()
	p, err := policy.LoadInline(`{
		name: "formats-off"
		scopes: { formats: { install: false } }
	}`)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSetHostFormatsPolicyUninstall(t *testing.T) {
	r, err := DefaultRegistryWithPolicy(formatsOffPolicy(t))
	if err != nil {
		t.Fatal(err)
	}
	SetHostFormats(r, map[string]Format{})
	if HostFormats(r) != nil {
		t.Fatal("formats install:false must leave the slot empty")
	}
}

func TestSetHostExtensionsPolicyUninstall(t *testing.T) {
	r, err := DefaultRegistryWithPolicy(formatsOffPolicy(t))
	if err != nil {
		t.Fatal(err)
	}
	SetHostExtensions(r, map[string]string{"txt": "text"})
	if HostExtensions(r) != nil {
		t.Fatal("formats install:false must leave the extensions slot empty")
	}
}

// pushCtx installs si as the top context store for the test's duration.
func pushCtx(t *testing.T, r *Registry, si *StoreInstanceInfo) {
	t.Helper()
	r.Contexts.PushExisting(si)
	t.Cleanup(func() { r.Contexts.Pop() })
}

func TestEffectiveFileOpsFallbackArms(t *testing.T) {
	// nil registry → nil.
	if ops := EffectiveFileOps(nil); ops != nil {
		t.Fatalf("EffectiveFileOps(nil) = %T, want nil", ops)
	}

	r := seam5Reg(t)
	host := HostFileOps(r)
	if host == nil {
		t.Fatal("expected default host fileops")
	}

	// Empty context stack → host fileops.
	snap := r.Contexts.Snapshot()
	r.Contexts.Restore(nil)
	if got := EffectiveFileOps(r); got == nil {
		t.Fatal("empty context stack must fall back to host fileops")
	}
	r.Contexts.Restore(snap)

	newStore := func(entries map[string]Value) *StoreInstanceInfo {
		si := &StoreInstanceInfo{TypeName: "Ideal/Store", Data: map[string]Value{}}
		for k, v := range entries {
			si.Set(k, v)
		}
		return si
	}
	storeVal := func(si *StoreInstanceInfo) Value { return NewStoreValue(nil, si) }

	cases := []struct {
		name string
		top  *StoreInstanceInfo
	}{
		{"no __sys", newStore(nil)},
		{"__sys not a store", newStore(map[string]Value{"__sys": NewInteger(1)})},
		{"no fs", newStore(map[string]Value{"__sys": storeVal(newStore(nil))})},
		{"fs not a store", newStore(map[string]Value{
			"__sys": storeVal(newStore(map[string]Value{"fs": NewString("x")})),
		})},
		{"no mem", newStore(map[string]Value{
			"__sys": storeVal(newStore(map[string]Value{"fs": storeVal(newStore(nil))})),
		})},
		{"mem false", newStore(map[string]Value{
			"__sys": storeVal(newStore(map[string]Value{
				"fs": storeVal(newStore(map[string]Value{"mem": NewBoolean(false)})),
			})),
		})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pushCtx(t, r, tc.top)
			if got := EffectiveFileOps(r); got == nil {
				t.Fatalf("%s: expected host fileops fallback, got nil", tc.name)
			}
		})
	}

	// Positive pair: __sys.fs.mem=true yields the in-memory fileops.
	memTop := newStore(map[string]Value{
		"__sys": storeVal(newStore(map[string]Value{
			"fs": storeVal(newStore(map[string]Value{"mem": NewBoolean(true)})),
		})),
	})
	pushCtx(t, r, memTop)
	got := EffectiveFileOps(r)
	if _, ok := got.(*capabilities.MemFileOps); !ok {
		t.Fatalf("mem=true: got %T, want *capabilities.MemFileOps", got)
	}
	// Second call returns the cached instance.
	if again := EffectiveFileOps(r); again != got {
		t.Fatal("mem fileops must be cached after first use")
	}
}
