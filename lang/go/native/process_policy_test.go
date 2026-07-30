package native

import (
	"errors"
	"testing"
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
	// Coded by the gate, so `do [spawn ...] error [dot code]` can dispatch —
	// see policy_error.go. The raw *policy.Denied no longer escapes.
	var ae *AqlError
	if !errors.As(err, &ae) {
		t.Fatalf("expected a coded AqlError, got %T (%v)", err, err)
	}
	if ae.Code != "capability_not_installed" {
		t.Errorf("Code = %q, want capability_not_installed", ae.Code)
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
