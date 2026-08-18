package main

import (
	"errors"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestErrorCode pins that a coded engine failure yields its code and an
// uncoded one yields "" — the distinction that decides whether an
// example documents a refusal or is left to the fallback.
func TestErrorCode(t *testing.T) {
	coded := &core.BoruError{Code: "type_error", Detail: "boom"}
	if got := errorCode(coded); got != "type_error" {
		t.Fatalf("errorCode(coded) = %q, want %q", got, "type_error")
	}
	// Wrapped, as the engine returns it.
	if got := errorCode(errors.Join(coded)); got != "type_error" {
		t.Fatalf("errorCode(wrapped) = %q, want %q", got, "type_error")
	}
	if got := errorCode(errors.New("plain")); got != "" {
		t.Fatalf("errorCode(plain) = %q, want \"\"", got)
	}
	if got := errorCode(&core.BoruError{}); got != "" {
		t.Fatalf("errorCode(empty code) = %q, want \"\"", got)
	}
}

// TestNondeterministic pins the identity-render detector. A false
// negative bakes a per-run id into shipped docs and makes this generator
// irreproducible; a false positive silently drops a good example.
func TestNondeterministic(t *testing.T) {
	yes := []string{
		"object<Ideal/Class/T_4f5a79d3674a>{a:1 b:2}",
		"Pid<P_fd4ec55d00a0>",
		"[Pid<P_887edab44508> 1]",
	}
	for _, r := range yes {
		if !nondeterministic(r) {
			t.Errorf("nondeterministic(%q) = false, want true", r)
		}
	}
	no := []string{
		"3", "'ab'", "true", "[1 2 3]", "{a:1 b:2}",
		"error [boru/type_error]",
		noValueMarker,
		// Near-misses: wrong prefix, wrong length, wrong alphabet.
		"X_4f5a79d3674a", "T_4f5a79d367", "T_4f5a79d3674g",
	}
	for _, r := range no {
		if nondeterministic(r) {
			t.Errorf("nondeterministic(%q) = true, want false", r)
		}
	}
}
