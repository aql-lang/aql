package policy

import (
	"strings"
	"testing"
)

// CompileProfile is the seam a BAKED policy comes back through: `aql build
// -perms …` flattens a Policy to a *Profile, JSON-encodes it into the
// binary's trailer, and the built binary compiles it back on startup. The
// trailer has no integrity check, so a corrupt or absent profile is an
// ordinary input, not an impossible state — it must return an error rather
// than dereference nil.
func TestCompileProfileNil(t *testing.T) {
	pol, err := CompileProfile(nil)
	if err == nil {
		t.Fatal("a nil profile compiled without error")
	}
	if pol != nil {
		t.Errorf("a failed compile returned a policy: %T", pol)
	}
	if !strings.Contains(err.Error(), "nil profile") {
		t.Errorf("error = %v, want it to name the nil profile", err)
	}
}

// A well-formed profile round-trips to a working policy — the path a built
// binary actually takes on startup.
func TestCompileProfileRoundTrip(t *testing.T) {
	prof := &Profile{Version: 1, Name: "baked"}
	pol, err := CompileProfile(prof)
	if err != nil {
		t.Fatalf("compiling a minimal profile: %v", err)
	}
	if pol == nil {
		t.Fatal("CompileProfile returned no policy and no error")
	}
}
