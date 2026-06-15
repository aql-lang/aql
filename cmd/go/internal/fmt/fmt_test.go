package fmt

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestFmtPositionalTildeExpanded guards that a leading ~ in a positional
// file path that the shell left verbatim (e.g. a quoted "~/x.aql")
// resolves under the home folder rather than being read as a literal
// "~/x.aql" relative to the working directory.
func TestFmtPositionalTildeExpanded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home) // os.UserHomeDir honors $HOME on Unix.
	src := filepath.Join(home, "x.aql")
	if err := os.WriteFile(src, []byte("foo:1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"~/x.aql"}, &out, &errOut); code != 0 {
		t.Fatalf("fmt code=%d, stderr=%q", code, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("unexpected stderr (a literal ~ would fail to open): %q", errOut.String())
	}
}
