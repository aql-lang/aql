// fmtidempotent_test.go ratchets `boru fmt`'s fixed point.
//
// A formatter is idempotent: `fmt(fmt(x)) == fmt(x)`. boru's is not — one
// pass is not a fixed point, it converges only at pass 2, and the
// intermediate layout runs statements together on one line, which is most
// of what a formatter is for. That divergence is recorded and ALLOWED
// (NUR046), so this gate does not fail the tree for it.
//
// What it does is stop the blast radius growing. NUR046's own proposed
// verdict asks for exactly this guard — "format every `.boru` in the repo
// TWICE and require the second pass to be a no-op" — and the reason it
// could not simply be switched on is that 19 files fail it today. So the
// 19 are listed, and the gate fails in BOTH directions:
//
//   - a file that becomes non-idempotent and is not listed → fail (the
//     regression this exists to catch);
//   - a listed file that becomes idempotent → fail, asking for the entry
//     to be dropped, so the list can only shrink.
//
// The second direction is the important one. A known-bad list that is
// allowed to keep stale entries silently stops being a measure of anything;
// this one is forced to track reality, and reaches empty exactly when
// NUR046 is resolved and its record can be deleted.
package docexamples

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/formatter"
)

// fmtNonIdempotent are the tracked .boru files for which pass 2 differs
// from pass 1 — the NUR046 blast radius, measured 2026-08-02. Repo-relative.
//
// This list may only SHRINK. Do not add to it to make a build pass: a new
// entry means a newly non-idempotent file, which is the regression.
var fmtNonIdempotent = map[string]bool{
	"design/examples/sift/ifconfig.boru":      true,
	"design/examples/todo/app.boru":           true,
	"editors/linguist/samples/app.boru":       true,
	"editors/linguist/samples/echo_boru.boru": true,
	"lang/go/modules/cli.boru":                true,
	"lang/go/modules/sift.boru":               true,
	"lang/go/modules/vault_tui.boru":          true,
	"utils/cat.boru":                          true,
	"utils/cut.boru":                          true,
	"utils/false.boru":                        true,
	"utils/grep.boru":                         true,
	"utils/head.boru":                         true,
	"utils/printenv.boru":                     true,
	"utils/seq.boru":                          true,
	"utils/sort.boru":                         true,
	"utils/tee.boru":                          true,
	"utils/true.boru":                         true,
	"utils/uniq.boru":                         true,
	"utils/wc.boru":                           true,
}

// TestFmtIdempotenceRatchet is the gate. See the file comment.
func TestFmtIdempotenceRatchet(t *testing.T) {
	root := docRoot()
	files := trackedBoruFiles(t, root)
	if len(files) == 0 {
		t.Skip("no tracked .boru files resolved (not a git checkout?)")
	}

	seen := map[string]bool{}
	for _, rel := range files {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Errorf("read %s: %v", rel, err)
			continue
		}
		pass1 := formatter.Format(string(body))
		pass2 := formatter.Format(pass1)
		idempotent := pass1 == pass2

		switch {
		case !idempotent && !fmtNonIdempotent[rel]:
			t.Errorf("%s: `boru fmt` is not idempotent on this file, and it is not a "+
				"recorded NUR046 case.\nA second pass must be a no-op. Either fix the "+
				"formatting change that introduced this, or — if the file genuinely "+
				"exposes NUR046 — add it to fmtNonIdempotent WITH a note on the record.",
				rel)
		case idempotent && fmtNonIdempotent[rel]:
			t.Errorf("%s: listed as NUR046-affected, but `boru fmt` is now idempotent "+
				"on it.\nThat is progress: remove it from fmtNonIdempotent so the list "+
				"keeps measuring the real blast radius (NUR046 records 19).", rel)
		}
		if fmtNonIdempotent[rel] {
			seen[rel] = true
		}
	}

	// An entry naming a file that no longer exists is the same rot in a
	// different shape — it makes the count look larger than reality.
	var stale []string
	for rel := range fmtNonIdempotent {
		if !seen[rel] {
			stale = append(stale, rel)
		}
	}
	sort.Strings(stale)
	for _, rel := range stale {
		t.Errorf("fmtNonIdempotent lists %s, which is not a tracked .boru file "+
			"any more — drop the entry", rel)
	}

	t.Logf("checked %d tracked .boru files; %d recorded non-idempotent (NUR046)",
		len(files), len(fmtNonIdempotent))
}

// trackedBoruFiles lists the repo's tracked .boru files, repo-relative.
// git is the source of truth so scratch and build output never enter the
// measurement.
func trackedBoruFiles(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "*.boru")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	sort.Strings(files)
	return files
}
