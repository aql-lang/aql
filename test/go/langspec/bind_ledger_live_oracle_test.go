// bind_ledger_live_oracle_test.go is the STRONG oracle §6.5's staging calls
// for: the ledger's final depth per name compared against the depth the check
// pass ACTUALLY left in the live registry.
//
// TestBindLedgerDepthsCompose is self-contained by construction — it replays
// each name's transitions symbolically and can only catch internal
// incoherence. This one cannot be fooled the same way: Boru.NativeRegistry()
// exposes the registry the pass ran against, so a transition the ledger
// MISSED entirely (an install with no note, a teardown with no undef) shows
// up as a final-depth disagreement even when the recorded entries compose
// perfectly among themselves.
//
// This is the assertion the inert-twin step needs (§6.5 "assert that the
// replayed binding state matches what the pass left") — a twin replaying the
// ledger onto a rolled-back registry reproduces exactly the ledger's final
// depths, so any name where ledger and live registry disagree is a name the
// replay would get wrong.
//
// NO ALLOWANCE. A gate that reports a number it tolerates teaches the next
// author to tolerate it. When this fails, fix the ledger (or, where the
// mismatch is the ORACLE's own blind spot, fix the comparison) — never the
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

// ledgerFinalDepths reduces a ledger to the last recorded depth per name, in
// first-touch order (order only matters for stable reporting).
func ledgerFinalDepths(led []core.BindTransition) (map[string]int, []string) {
	final := map[string]int{}
	var order []string
	for _, tr := range led {
		if _, seen := final[tr.Name]; !seen {
			order = append(order, tr.Name)
		}
		final[tr.Name] = tr.Depth
	}
	return final, order
}

// liveOracleRow runs one row's compile pass and reports every name whose
// final ledger depth disagrees with the live registry's post-pass depth.
func liveOracleRow(src, where string) []string {
	a, err := lang.New()
	if err != nil {
		return []string{fmt.Sprintf("%s lang.New: %v", where, err)}
	}
	_, _, res, _ := a.CompileCheck(src)
	if len(res.BindLedger) == 0 {
		return nil
	}
	final, order := ledgerFinalDepths(res.BindLedger)
	defs := a.NativeRegistry().Defs
	var bad []string
	for _, name := range order {
		live := defs.Depth(name)
		if live != final[name] {
			s := src
			if len(s) > 90 {
				s = s[:90] + "…"
			}
			bad = append(bad, fmt.Sprintf("%s name=%q ledger=%d live=%d  %s",
				where, name, final[name], live, s))
		}
	}
	return bad
}

// TestBindLedgerLiveDepths runs the strong oracle over the whole corpus plus
// the synthetic branch-arm rows the corpus lacks.
func TestBindLedgerLiveDepths(t *testing.T) {
	for _, src := range syntheticBranchArmSources {
		for _, bad := range liveOracleRow(src, "synthetic") {
			t.Errorf("%s", bad)
		}
	}

	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatal(err)
	}
	checked, mismatched := 0, 0
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
			bad := liveOracleRow(src, fmt.Sprintf("%s:%d", e.Name(), lineNo))
			if bad == nil {
				continue
			}
			checked++
			mismatched += len(bad)
			for _, b := range bad {
				if len(worst) < 20 {
					worst = append(worst, b)
				}
			}
		}
		_ = f.Close()
	}

	t.Logf("bind-ledger live-depth oracle: %d rows mismatched, %d name mismatches", checked, mismatched)
	for _, w := range worst {
		t.Logf("    %s", w)
	}
	if mismatched > 0 {
		t.Errorf("%d ledger-vs-live depth mismatches: the ledger disagrees with the state "+
			"the check pass left, so a twin replay would reproduce the wrong registry (§6.5)", mismatched)
	}
}
