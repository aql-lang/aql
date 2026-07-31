package permsflags

import (
	"errors"
	"testing"

	"github.com/boru-lang/boru/lang/go/policy"
)

// TestS7n_ResolveModsWithoutBaseLoadFullFailure drives Resolve's
// "mods supplied, no base, policy.Load(\"full\") fails" arm by
// swapping the policyLoadFull seam.
func TestS7n_ResolveModsWithoutBaseLoadFullFailure(t *testing.T) {
	clearPolicyEnv(t)
	orig := policyLoadFull
	t.Cleanup(func() { policyLoadFull = orig })
	policyLoadFull = func() (policy.Policy, error) { return nil, errors.New("boom-load-full") }

	_, err := parseFlags(t, "--deny", "fileops.write").Resolve()
	if err == nil || err.Error() != "boom-load-full" {
		t.Errorf("Resolve() err = %v, want boom-load-full", err)
	}
}

// TestS7n_AddRuleInitializesNilScopes drives addRule's nil-Scopes
// guard directly: the only production call site (applyMods) always
// pre-initializes Scopes, so a bare *policy.Profile is needed to
// reach the guard.
func TestS7n_AddRuleInitializesNilScopes(t *testing.T) {
	p := &policy.Profile{}
	addRule(p, "fileops", policy.Rule{Allow: []string{"read"}})
	if p.Scopes == nil {
		t.Fatal("addRule should initialize a nil Scopes map")
	}
	s, ok := p.Scopes["fileops"]
	if !ok || s == nil {
		t.Fatal("addRule should create the named scope")
	}
	if len(s.Words.Rules) != 1 {
		t.Errorf("Words.Rules = %+v, want 1 rule", s.Words.Rules)
	}
}

// TestS7n_SetInstallInitializesNilScopes is the setInstall analogue.
func TestS7n_SetInstallInitializesNilScopes(t *testing.T) {
	p := &policy.Profile{}
	setInstall(p, "fileops", true)
	if p.Scopes == nil {
		t.Fatal("setInstall should initialize a nil Scopes map")
	}
	s, ok := p.Scopes["fileops"]
	if !ok || s == nil {
		t.Fatal("setInstall should create the named scope")
	}
	if s.Install == nil || !*s.Install {
		t.Errorf("Install = %v, want true", s.Install)
	}
}
