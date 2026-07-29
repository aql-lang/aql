package build

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aql-lang/aql/cmd/go/internal/buildrt"
)

// build_perms_test.go — `aql build -perms …` bakes the resolved policy into
// the produced binary (design/CLI-PROGRAMS.1.md). The two arms here are the
// resolution boundary: an unresolvable spec fails the BUILD rather than
// producing a binary with no constraints, and a resolved one is flattened
// to the *policy.Profile that rides in the payload.

// An unknown profile name fails the build with a message, rather than
// silently producing an unconstrained binary — the failure mode that makes
// a baked policy worth having in the first place.
func TestRunRejectsUnresolvablePerms(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "p.aql")
	if err := os.WriteFile(src, []byte("add 1 2"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := New().Run([]string{src, "-perms", "no-such-profile-xyz",
		"-o", filepath.Join(dir, "p")}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 {
		t.Fatal("an unresolvable -perms produced a binary")
	}
	if stderr.Len() == 0 {
		t.Error("the resolution failure was not reported")
	}
	if _, err := os.Stat(filepath.Join(dir, "p")); err == nil {
		t.Error("a binary was written despite the policy failing to resolve")
	}
}

// A resolvable profile is flattened onto the Config the payload carries.
// Profile is a *policy.Profile and not a policy.Policy because Policy is an
// interface and Config is JSON-marshalled into the executable's trailer.
func TestBuildConfigCarriesResolvedProfile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "p.aql")
	if err := os.WriteFile(src, []byte("add 1 2"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	out := filepath.Join(dir, "p")
	if code := New().Run([]string{src, "-perms", "sandbox", "-o", out},
		strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("build failed: %s", stderr.String())
	}
	cfg, ok, err := buildrt.ReadEmbeddedPayload(out)
	if err != nil || !ok {
		t.Fatalf("no payload in the produced binary (ok=%v err=%v)", ok, err)
	}
	if cfg.Profile == nil {
		t.Fatal("the resolved policy was not baked into the payload")
	}
	if cfg.Profile.Name == "" {
		t.Error("the baked profile carries no name")
	}
}
