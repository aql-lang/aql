// bind_replay_sandbox_test.go gives core/go/binding_sandbox.go its FIRST
// CLIENT, and proves §6.5's central mechanism at corpus scale before any VM
// semantics change: SNAPSHOT the runtime-visible bindings, run the compile
// pass (installs kept, as today), ROLL BACK with RestoreBindings, then REPLAY
// the pass's ledger — re-installing at each transition the IDENTICAL entry
// the pass left at that depth — and land exactly the registry the pass left.
//
// Two assertions carry the weight, per transition and per name:
//
//   - after each replayed transition, the name's live depth equals the
//     ledger's recorded depth — the ledger's ABSOLUTE depths are exactly
//     reproducible over the rolled-back registry, which is what lets a twin
//     op trust its recorded depth at VM time;
//   - after the full replay, each ledgered name's entry stack equals the
//     pass-left stack in length and entry identity (TypeDef pointer, Minted
//     flag) — the "replay, never re-execution" contract: the same FnDefInfo,
//     the same module instance, the same minted node, reconnected rather
//     than reproduced.
//
// SCOPE: rows whose ledger holds only PUSH transitions (def, type-install) —
// 99.5% of the corpus population. An undef or def-replace transition needs
// state the pass-left registry no longer holds (the popped entry), which is
// exactly what the twin OP will carry; those rows are counted and skipped
// here, not silently ignored.
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

// replaySandboxRow runs one row's snapshot → pass → rollback → replay cycle
// and returns a failure description, or "" on success / not-applicable.
func replaySandboxRow(src, where string) string {
	a, err := lang.New()
	if err != nil {
		return fmt.Sprintf("%s lang.New: %v", where, err)
	}
	reg := a.NativeRegistry()
	snap := reg.SnapshotBindings()
	_, _, res, _ := a.CompileCheck(src)
	if len(res.BindLedger) == 0 {
		return ""
	}
	for _, tr := range res.BindLedger {
		if tr.Kind == core.BindUndef || tr.Kind == core.BindDefReplace {
			return "skip"
		}
	}
	s := src
	if len(s) > 70 {
		s = s[:70] + "…"
	}

	// Capture the pass-left entry stacks for every ledgered name.
	left := map[string][]core.DefEntry{}
	preDepth := map[string]int{}
	for _, tr := range res.BindLedger {
		if _, ok := left[tr.Name]; !ok {
			left[tr.Name] = reg.Defs.Entries(tr.Name)
			// The depth BEFORE the pass touched the name: its first recorded
			// transition is a push landing at preDepth+1.
			preDepth[tr.Name] = tr.Depth - 1
		}
	}

	reg.RestoreBindings(snap)
	for name, pre := range preDepth {
		if d := reg.Defs.Depth(name); d != pre {
			return fmt.Sprintf("%s rollback left %q at depth %d, want the pre-pass %d  %s",
				where, name, d, pre, s)
		}
	}

	// Replay the ledger in order, re-installing the pass-left entry at each
	// transition's depth.
	for i, tr := range res.BindLedger {
		entries := left[tr.Name]
		if tr.Depth < 1 || tr.Depth > len(entries) {
			return fmt.Sprintf("%s transition %d: recorded depth %d outside the pass-left stack (%d entries) for %q  %s",
				where, i, tr.Depth, len(entries), tr.Name, s)
		}
		e := entries[tr.Depth-1]
		switch {
		case e.TypeDef == nil:
			reg.Defs.Push(tr.Name, e.Body)
		case e.Minted:
			reg.Defs.PushType(tr.Name, e.TypeDef, e.Body)
		default:
			reg.Defs.PushTypeAdopted(tr.Name, e.TypeDef, e.Body)
		}
		if d := reg.Defs.Depth(tr.Name); d != tr.Depth {
			return fmt.Sprintf("%s transition %d (%s %q): replay landed at depth %d, ledger says %d  %s",
				where, i, tr.Kind, tr.Name, d, tr.Depth, s)
		}
	}

	// The replayed stacks equal the pass-left stacks — length and the
	// type-binding identity per entry.
	for name, want := range left {
		got := reg.Defs.Entries(name)
		if len(got) != len(want) {
			return fmt.Sprintf("%s %q replayed to %d entries, pass left %d  %s",
				where, name, len(got), len(want), s)
		}
		for i := range want {
			if got[i].TypeDef != want[i].TypeDef || got[i].Minted != want[i].Minted {
				return fmt.Sprintf("%s %q entry %d: replayed {TypeDef %p Minted %v}, pass left {TypeDef %p Minted %v}  %s",
					where, name, i, got[i].TypeDef, got[i].Minted, want[i].TypeDef, want[i].Minted, s)
			}
		}
	}
	return ""
}

// TestBindingSandboxRollbackAndReplay runs the cycle over the corpus and the
// synthetic rows. No allowance.
func TestBindingSandboxRollbackAndReplay(t *testing.T) {
	for _, src := range syntheticBranchArmSources {
		if bad := replaySandboxRow(src, "synthetic"); bad != "" && bad != "skip" {
			t.Errorf("%s", bad)
		}
	}

	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatal(err)
	}
	replayed, skipped, failed := 0, 0, 0
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
			switch bad := replaySandboxRow(src, fmt.Sprintf("%s:%d", e.Name(), lineNo)); bad {
			case "":
			case "skip":
				skipped++
			default:
				failed++
				if len(worst) < 12 {
					worst = append(worst, bad)
				}
			}
			replayed++
		}
		_ = f.Close()
	}

	t.Logf("binding-sandbox rollback+replay: %d rows cycled, %d skipped (undef/def-replace), %d failed",
		replayed, skipped, failed)
	for _, w := range worst {
		t.Logf("    %s", w)
	}
	if failed > 0 {
		t.Errorf("%d rows where rollback+replay does not reproduce the pass-left registry: "+
			"the flip cannot trust the sandbox or the ledger's depths (§6.5)", failed)
	}
}
