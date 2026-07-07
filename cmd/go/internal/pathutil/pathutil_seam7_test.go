package pathutil

import (
	"errors"
	"testing"
)

// TestS7n_ExpandNoHomeDirReturnsPathUnchanged drives Expand's
// no-home-dir fallback arm by swapping the userHomeDir seam.
func TestS7n_ExpandNoHomeDirReturnsPathUnchanged(t *testing.T) {
	orig := userHomeDir
	t.Cleanup(func() { userHomeDir = orig })
	userHomeDir = func() (string, error) { return "", errors.New("boom-no-home") }

	if got := Expand("~/sub"); got != "~/sub" {
		t.Errorf("Expand(~/sub) = %q, want unchanged when the home dir is unresolvable", got)
	}
}
