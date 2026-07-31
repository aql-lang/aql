// Wave-4 coverage for the top-level dispatcher helpers: versionString
// (published and dev-build branches) and the runEmbedded no-payload
// path.
package boru

import (
	"strings"
	"testing"
)

func TestW4VersionStringPublishedPassthrough(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()

	Version = "3.2.1"
	if got := versionString(); got != "3.2.1" {
		t.Errorf("versionString() = %q, want the ldflags-stamped value verbatim", got)
	}
}

func TestW4VersionStringDevBuild(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()

	Version = "0.1.0-dev"
	got := versionString()
	// Test binaries carry build info without a VCS stamp, so the dev
	// default comes back unchanged; a stamped build appends the
	// revision. Either way the dev prefix must survive.
	if !strings.HasPrefix(got, "0.1.0-dev") {
		t.Errorf("versionString() = %q, want a 0.1.0-dev prefix", got)
	}
	if strings.Contains(got, "git ") {
		if !strings.Contains(got, "(git ") {
			t.Errorf("versionString() = %q, malformed VCS suffix", got)
		}
	}
}

func TestW4RunEmbeddedNoPayload(t *testing.T) {
	// The test binary carries no appended payload, so runEmbedded must
	// report embedded=false and defer to the normal CLI.
	code, embedded := runEmbedded()
	if embedded {
		t.Fatalf("runEmbedded() = (%d, true) on a plain binary, want embedded=false", code)
	}
	if code != 0 {
		t.Errorf("runEmbedded() code = %d, want 0", code)
	}
}
