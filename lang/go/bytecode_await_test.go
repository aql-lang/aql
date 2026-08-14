package lang

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// await branch bodies compile per element (CompileStoresBodyList — the spawn
// store-body pattern applied to each parallels element) and run via RunUnit
// on their per-branch forks. Pinned here: compiled parity across the value /
// def-body / raising-branch / full-mode shapes, plus the per-element
// interpreter fallback for a branch the store-body compile declines (a
// Module construction in the body) — the base program's zero-interp-entry
// end state is the p6/concurrent-fork-bodies-on-vm frontier pin.
func TestAwaitCompiledBranchParity(t *testing.T) {
	cases := []string{
		`import "boru:time-util" TimeUtil.await [[1 add 2] [3 mul 4]]`,
		`import "boru:time-util" TimeUtil.await [[def x 5 x add 1] [3 mul 4]]`,
		`import "boru:time-util" TimeUtil.await [[raise bad_input "boom"] [3 mul 4]]`,
		`import "boru:time-util" TimeUtil.await {mode:"full"} [[raise bad_input "boom"] [3 mul 4]]`,
		// The winner-takes-all modes (first/any) are NOT here: their result
		// is the winning branch's whole residual — a runtime-variable count —
		// so the compile pass refuses them wholesale (NUR067; the pinned
		// refusal is TestAwaitWinnerModesRefuseCompilation below).
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			gotC, compiled, err := a.RunCompiled(src)
			if err != nil {
				t.Fatalf("RunCompiled: %v", err)
			}
			if !compiled {
				t.Fatal("await program must run compiled")
			}
			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			gotI, err := b.RunInterp(src)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if fmt.Sprintf("%v", gotC) != fmt.Sprintf("%v", gotI) {
				t.Errorf("parity: compiled %v != interp %v", gotC, gotI)
			}
		})
	}
}

// TestAwaitWinnerModesRefuseCompilation — first/any hand back the winning
// branch's WHOLE residual (0-or-more values, a count that can EXCEED any
// static seat), so the compile pass refuses wholesale and the interpreter
// owns these modes (NUR067 — the pre-refusal 1-seat layout was a live
// miscompile: `size [(await {mode:'any'} [[7 8]])]` stranded a value).
// The refusal reason is pinned so a graduation (a runtime-variadic region
// representation) flips this test loudly.
func TestAwaitWinnerModesRefuseCompilation(t *testing.T) {
	for _, src := range []string{
		`import "boru:time-util" TimeUtil.await {mode:"any"} [[raise bad_input "boom"] [3 mul 4]]`,
		`import "boru:time-util" TimeUtil.await {mode:"first"} [[1 2 3]]`,
	} {
		t.Run(src, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			_, compiled, err := a.RunCompiled(src)
			if compiled {
				t.Fatal("a winner-mode await must refuse compilation — has the runtime-variadic region representation landed? Graduate the frontier-await-winner.tsv ledger rows with this pin")
			}
			if err == nil || !strings.Contains(err.Error(), "runtime-variadic (0-or-more values) with no static seat") {
				t.Fatalf("refusal reason drifted: %v", err)
			}
			// The interpreter owns the modes, byte-identically.
			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := b.RunInterp(src); err != nil {
				t.Fatalf("RunInterp: %v", err)
			}
		})
	}
}

// TestAwaitRefusedBranchInterpretsPerElement — an element the store-body
// compile declines (a Module construction has no operand provenance) keeps
// its raw list: THAT branch runs on the interpreter (an Engine.Run entry
// appears) while the program still runs compiled with interpreter-identical
// results. The sibling compiled branch is unaffected — per-element, sound.
func TestAwaitRefusedBranchInterpretsPerElement(t *testing.T) {
	const src = `import "boru:time-util" TimeUtil.await [[9 (module [export "X" {a:1}]) drop] [3 mul 4]]`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var (
		mu      sync.Mutex
		engRuns int
	)
	disarm := a.ArmInterpEntryHook(func(e InterpEntry) {
		if strings.Contains(e.Seam, "Engine.Run") {
			mu.Lock()
			engRuns++
			mu.Unlock()
		}
	})
	defer disarm()
	gotC, compiled, err := a.RunCompiled(src)
	if err != nil {
		t.Fatalf("RunCompiled: %v", err)
	}
	if !compiled {
		t.Fatal("the program itself must run compiled — only the refused ELEMENT interprets")
	}
	mu.Lock()
	runs := engRuns
	mu.Unlock()
	if runs == 0 {
		t.Error("the Module-bearing branch must fall back to an interpreter sub-engine")
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotI, err := b.RunInterp(src)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fmt.Sprintf("%v", gotC) != fmt.Sprintf("%v", gotI) {
		t.Errorf("parity: compiled %v != interp %v", gotC, gotI)
	}
}

// The C1 effect fence on a BRANCH unit's internal_error (the runParallelBranch
// twin of RunCompiled's runtime-bail arm): with no observable effect the
// branch re-runs its raw tokens on the interpreter and the program matches
// the interpreter exactly; after an effect the re-run is blocked — the output
// is emitted exactly once and the branch surfaces the internal error as its
// error value (the L-DUP doctrine: no-duplicate-effects beats parity).
func TestAwaitBranchBailBeforeEffectFallsBack(t *testing.T) {
	const src = `import "boru:time-util" TimeUtil.await [[def i (zz-inst) i.m 5 42] [3 mul 4]]`
	a := zzShapedInstance(t)
	a.SetOutput(&bytes.Buffer{})
	gotC, compiled, err := a.RunCompiled(src)
	if err != nil || !compiled {
		t.Fatalf("RunCompiled: compiled=%v err=%v", compiled, err)
	}
	b := zzShapedInstance(t)
	b.SetOutput(&bytes.Buffer{})
	gotI, err := b.RunInterp(src)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fmt.Sprintf("%v", gotC) != fmt.Sprintf("%v", gotI) {
		t.Errorf("effect-free branch bail must re-run on the interpreter: compiled %v != interp %v", gotC, gotI)
	}
}

func TestAwaitBranchBailAfterEffectSurfaces(t *testing.T) {
	const src = `import "boru:time-util" TimeUtil.await [[print "once" def i (zz-inst) i.m 5 42] [3 mul 4]]`
	a := zzShapedInstance(t)
	var out bytes.Buffer
	a.SetOutput(&out)
	gotC, compiled, err := a.RunCompiled(src)
	if err != nil || !compiled {
		t.Fatalf("RunCompiled: compiled=%v err=%v", compiled, err)
	}
	if out.String() != "once\n" {
		t.Errorf("output = %q, want exactly one %q (no duplicate from a branch re-run)", out.String(), "once\n")
	}
	if s := fmt.Sprintf("%v", gotC); !strings.Contains(s, "internal") {
		t.Errorf("fenced branch must surface the internal error as its value, got %v", gotC)
	}
}
