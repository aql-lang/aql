package repl

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// The REPL builds its registry by hand and runs it through
// lang.NewFromRegistry, which — unlike lang.New — installs no capabilities.
// So the terminal probe has to be wired explicitly here, and this is the one
// place a real terminal genuinely exists: a prompt that answered IO.is-tty
// false would be the worst possible gap.
func TestReplRegistryInstallsStreamProbe(t *testing.T) {
	reg, err := newRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if native.HostStreamProbe(reg) == nil {
		t.Error("the REPL registry has no StreamProbe: IO.is-tty would answer false at the prompt")
	}
}
