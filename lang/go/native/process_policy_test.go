package native

import (
	"testing"

	"github.com/aql-lang/aql/lang/go/policy"
)

// `spawn` is gated by the `process` capability scope (PROCESSES.0.md §7):
// the restrictive built-in profiles hard-deny it, trusted/full allow it,
// and no policy at all keeps the historical allow-everything default.

func TestSpawnPolicyAllowsWithFull(t *testing.T) {
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "full"))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkProcessPolicy(r); err != nil {
		t.Errorf("full should permit spawn: %v", err)
	}
}

func TestSpawnPolicyDeniedBySandbox(t *testing.T) {
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "sandbox"))
	if err != nil {
		t.Fatal(err)
	}
	err = checkProcessPolicy(r)
	if err == nil {
		t.Fatal("expected sandbox to deny spawn")
	}
	d, ok := err.(*policy.Denied)
	if !ok {
		t.Fatalf("expected *policy.Denied, got %T (%v)", err, err)
	}
	if d.Code != policy.CodeCapabilityNotInstalled {
		t.Errorf("Code = %q, want %q", d.Code, policy.CodeCapabilityNotInstalled)
	}
}

func TestSpawnPolicyNoPolicyAllows(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkProcessPolicy(r); err != nil {
		t.Errorf("no policy must allow spawn: %v", err)
	}
}
