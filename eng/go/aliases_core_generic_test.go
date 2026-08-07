package eng

import "testing"

// eng.Cap is the ONE facade entry that cannot be demoted to a `var X = core.X`
// re-export when no caller reaches it: Go has no way to alias or var-wrap a
// generic function without instantiating it, so the wrapper body is permanent.
// That makes it the one place where "nothing calls it" has to be answered with
// a test rather than by piecetool's cold set — and it is worth answering
// properly, because Cap is the documented host-integration seam
// (design/PERMISSIONS-PLAN.10.md, design/DEBUG-MODULE.0.md both reach for
// `eng.Cap[T]`), so today's zero in-repo callers is a fact about this repo,
// not about the API.
//
// The three outcomes below are the whole contract: the typed hit, the absent
// name, and the type mismatch. Each must come back through the facade exactly
// as core.Cap produces it.

type capProbe struct{ n int }

func TestCapReadsThroughTheFacade(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := r.Capabilities.Set("engine.probe", capProbe{n: 7}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Present and correctly typed: the value round-trips.
	got, ok, err := Cap[capProbe](r, "engine.probe")
	if err != nil {
		t.Fatalf("Cap: unexpected error %v", err)
	}
	if !ok {
		t.Fatal("Cap reported the installed capability as absent")
	}
	if got.n != 7 {
		t.Errorf("Cap returned %+v, want n=7", got)
	}

	// Absent name: not an error, just not found — a handler is expected to
	// branch on ok and report its own message.
	if _, ok, err := Cap[capProbe](r, "engine.nope"); err != nil || ok {
		t.Errorf("Cap(absent) = (ok=%v, err=%v), want (false, nil)", ok, err)
	}

	// Installed under a DIFFERENT type reads as ABSENT, not as an error: the
	// failed assertion collapses into ok=false with a nil error, so a caller
	// cannot distinguish "nobody installed it" from "the host installed it
	// under a type I did not expect". Pinned deliberately rather than left
	// implicit — it is the contract every `if !ok { return "no X capability" }`
	// site is written against, and a diagnostic that named the mismatch would
	// be a behaviour change, not a bug fix.
	if v, ok, err := Cap[string](r, "engine.probe"); err != nil || ok || v != "" {
		t.Errorf("Cap(type mismatch) = (%q, ok=%v, err=%v), want (\"\", false, nil)", v, ok, err)
	}

	// A nil Registry is the one genuine error arm: it means the Registry was
	// built without NewRegistry, which is a programming error rather than a
	// missing capability, so it must not masquerade as "absent".
	if _, ok, err := Cap[capProbe](nil, "engine.probe"); err == nil || ok {
		t.Errorf("Cap(nil registry) = (ok=%v, err=%v), want (false, non-nil)", ok, err)
	}
}
