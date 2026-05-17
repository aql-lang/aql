package eng

import (
	"strings"
	"testing"
)

// TestEngineOptionsDefaults pins the contract that the zero value of
// EngineOptions is treated as "use the default": a NewWithOptions
// call with EngineOptions{} produces the same engine state as one
// with DefaultEngineOptions().
func TestEngineOptionsDefaults(t *testing.T) {
	r1, _ := NewRegistry()
	e1, err := NewWithOptions(r1, EngineOptions{})
	if err != nil {
		t.Fatalf("NewWithOptions(zero): %v", err)
	}
	r2, _ := NewRegistry()
	e2, err := NewWithOptions(r2, DefaultEngineOptions())
	if err != nil {
		t.Fatalf("NewWithOptions(defaults): %v", err)
	}
	if e1.stepLimit != e2.stepLimit {
		t.Errorf("stepLimit mismatch: zero=%d, defaults=%d", e1.stepLimit, e2.stepLimit)
	}
	// Both registries should have the same set of core word names.
	want := CoreWordNames()
	for _, name := range want {
		if !r1.Defs.Has(name) {
			t.Errorf("zero-options engine missing core word %q", name)
		}
		if !r2.Defs.Has(name) {
			t.Errorf("defaults-options engine missing core word %q", name)
		}
	}
}

// TestEngineOptionsPartial pins the partial-override semantics:
// fields the caller doesn't set fall through to the defaults; fields
// the caller does set override.
func TestEngineOptionsPartial(t *testing.T) {
	r, _ := NewRegistry()
	e, err := NewWithOptions(r, EngineOptions{MaxSteps: 5000})
	if err != nil {
		t.Fatalf("NewWithOptions: %v", err)
	}
	if e.stepLimit != 5000 {
		t.Errorf("MaxSteps override broken: got %d, want 5000", e.stepLimit)
	}
	// Words wasn't set, so it should default to "*" — all core
	// words installed.
	for _, name := range CoreWordNames() {
		if !r.Defs.Has(name) {
			t.Errorf("partial-options engine missing core word %q (default Words should install all)", name)
		}
	}
}

// TestEngineOptionsWordWhitelist exercises selective registration.
func TestEngineOptionsWordWhitelist(t *testing.T) {
	r, _ := NewRegistry()
	want := []string{"def", "fn", "dup", "drop"}
	e, err := NewWithOptions(r, EngineOptions{Words: want})
	if err != nil {
		t.Fatalf("NewWithOptions: %v", err)
	}
	if e.stepLimit != DefaultEngineOptions().MaxSteps {
		t.Errorf("MaxSteps default not applied: got %d", e.stepLimit)
	}
	// The four named words must be present.
	for _, name := range want {
		if !r.Defs.Has(name) {
			t.Errorf("whitelisted core word %q not registered", name)
		}
	}
	// Words NOT in the whitelist must NOT be present.
	for _, name := range CoreWordNames() {
		inList := false
		for _, w := range want {
			if w == name {
				inList = true
				break
			}
		}
		if !inList && r.Defs.Has(name) {
			t.Errorf("non-whitelisted core word %q should not be registered", name)
		}
	}
}

// TestEngineOptionsWildcardInList — "*" anywhere in the Words slice
// short-circuits to "install everything".
func TestEngineOptionsWildcardInList(t *testing.T) {
	r, _ := NewRegistry()
	_, err := NewWithOptions(r, EngineOptions{Words: []string{"def", "*", "this-name-does-not-exist"}})
	if err != nil {
		t.Fatalf("wildcard should short-circuit, but got: %v", err)
	}
	for _, name := range CoreWordNames() {
		if !r.Defs.Has(name) {
			t.Errorf("wildcard install should have registered %q", name)
		}
	}
}

// TestEngineOptionsEmptyWords — non-nil empty slice means "no core
// words at all"; the registry is left untouched (apart from whatever
// the caller put on it before calling).
func TestEngineOptionsEmptyWords(t *testing.T) {
	r, _ := NewRegistry()
	_, err := NewWithOptions(r, EngineOptions{Words: []string{}})
	if err != nil {
		t.Fatalf("NewWithOptions: %v", err)
	}
	for _, name := range CoreWordNames() {
		if r.Defs.Has(name) {
			t.Errorf("empty Words should leave registry untouched, but %q is registered", name)
		}
	}
}

// TestEngineOptionsUnknownWord — an unknown name returns an AqlError
// with code "unknown_core_word", and the registry is left untouched
// (validation happens before any registration).
func TestEngineOptionsUnknownWord(t *testing.T) {
	r, _ := NewRegistry()
	_, err := NewWithOptions(r, EngineOptions{Words: []string{"def", "frobnicate", "fn"}})
	if err == nil {
		t.Fatal("expected error for unknown core word, got nil")
	}
	aqlErr, ok := err.(*AqlError)
	if !ok {
		t.Fatalf("expected *AqlError, got %T: %v", err, err)
	}
	if aqlErr.Code != "unknown_core_word" {
		t.Errorf("wrong error code: got %q, want \"unknown_core_word\"", aqlErr.Code)
	}
	if !strings.Contains(aqlErr.Detail, "frobnicate") {
		t.Errorf("error detail should mention the bad name; got: %s", aqlErr.Detail)
	}
	// Validation precedes registration: even the names BEFORE the
	// bad one ("def" here) should not have been registered.
	if r.Defs.Has("def") {
		t.Errorf("registry should be untouched on validation failure, but %q is registered", "def")
	}
	if r.Defs.Has("fn") {
		t.Errorf("registry should be untouched on validation failure, but %q is registered", "fn")
	}
}

// TestCoreWordNames — the public helper returns a stable, sorted list.
func TestCoreWordNames(t *testing.T) {
	names := CoreWordNames()
	if len(names) == 0 {
		t.Fatal("CoreWordNames returned empty list")
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Errorf("CoreWordNames not sorted: %q before %q", names[i-1], names[i])
		}
	}
	// Spot-check a few words we know should be present. not/and/or
	// and tor/tand moved to lang/engine; break/continue moved to lang
	// too. Only kernel-resident registrations are checked here.
	want := []string{"def", "fn", "quote", "args", "dup", "swap", "drop", "over", "rot"}
	for _, w := range want {
		found := false
		for _, n := range names {
			if n == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("CoreWordNames missing expected word %q", w)
		}
	}
}
