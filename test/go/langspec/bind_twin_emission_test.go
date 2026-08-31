// bind_twin_emission_test.go gates §6.5's inert-emission stage: every
// compiled program's bind-twin table equals the check pass's own bind ledger,
// entry for entry.
//
// The twins are fired from NoteBindTransition itself — the ledger's single
// funnel, after its suppressions — so equality here is not a tautology: what
// this catches is a RECORDER-LIFECYCLE hole, an entry noted while a swapped
// or isolated recorder was installed (the dynamic-help eval's IsolateEmit, a
// module sub-pass, a fork), which would silently drop a twin the ledger
// keeps. A twin table missing one transition is a registry the
// rollback-and-replay flip would reproduce wrongly — the same class of
// late-surfacing divergence the depth-composition and live-depth oracles
// exist to catch early.
//
// NO ALLOWANCE, same as the oracles: fix the recorder lifecycle, never the
// number.
package langspec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// twinRowMismatch compiles one row and reports a mismatch line, or "" when
// the row does not compile or the tables agree.
func twinRowMismatch(src, where string) string {
	a, err := lang.New()
	if err != nil {
		return fmt.Sprintf("%s lang.New: %v", where, err)
	}
	prog, _, res, _ := a.CompileCheck(src)
	if prog == nil {
		return ""
	}
	s := src
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	if len(prog.BindTwins) != len(res.BindLedger) {
		return fmt.Sprintf("%s twins=%d ledger=%d  %s", where, len(prog.BindTwins), len(res.BindLedger), s)
	}
	for i, tw := range prog.BindTwins {
		ld := res.BindLedger[i]
		if tw.Kind != ld.Kind || tw.Name != ld.Name || tw.Depth != ld.Depth || tw.Pos != ld.Pos {
			return fmt.Sprintf("%s entry %d: twin=%+v ledger=%+v  %s", where, i, tw, ld, s)
		}
	}
	return ""
}

// TestBindTwinsEqualLedger runs the emission gate over the corpus and the
// synthetic rows.
func TestBindTwinsEqualLedger(t *testing.T) {
	for _, src := range syntheticBranchArmSources {
		if bad := twinRowMismatch(src, "synthetic"); bad != "" {
			t.Errorf("%s", bad)
		}
	}

	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatal(err)
	}
	compiled, withTwins, mismatched := 0, 0, 0
	var worst []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, ferr := os.Open(filepath.Join(specDir, e.Name()))
		if ferr != nil {
			t.Fatal(ferr)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			line := strings.TrimRight(sc.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			src := strings.TrimSpace(parts[0])
			a, aerr := lang.New()
			if aerr != nil {
				continue
			}
			prog, _, res, _ := a.CompileCheck(src)
			if prog == nil {
				continue
			}
			compiled++
			if len(prog.BindTwins) > 0 {
				withTwins++
			}
			where := fmt.Sprintf("%s:%d", e.Name(), lineNo)
			bad := ""
			if len(prog.BindTwins) != len(res.BindLedger) {
				bad = fmt.Sprintf("%s twins=%d ledger=%d  %s", where, len(prog.BindTwins), len(res.BindLedger), src)
			} else {
				for i, tw := range prog.BindTwins {
					ld := res.BindLedger[i]
					if tw.Kind != ld.Kind || tw.Name != ld.Name || tw.Depth != ld.Depth || tw.Pos != ld.Pos {
						bad = fmt.Sprintf("%s entry %d: twin=%+v ledger=%+v", where, i, tw, ld)
						break
					}
				}
			}
			if bad != "" {
				mismatched++
				if len(worst) < 15 {
					worst = append(worst, bad)
				}
			}
		}
		_ = f.Close()
	}

	t.Logf("bind-twin emission: %d compiled rows, %d with twins, %d mismatched", compiled, withTwins, mismatched)
	for _, w := range worst {
		t.Logf("    %s", w)
	}
	if compiled == 0 || withTwins == 0 {
		t.Fatal("no compiled row carried a twin table — the gate is measuring its own wiring")
	}
	if mismatched > 0 {
		t.Errorf("%d rows where the program's twin table diverges from the pass's ledger: "+
			"a recorder-lifecycle hole ate or duplicated a twin (§6.5)", mismatched)
	}
	_ = core.BindDef // keep the core import for the shared helpers' package
}
