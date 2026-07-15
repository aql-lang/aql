package lang

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// The lang-package frontier inventory (see frontier_ledger_test.go for the
// contract): cases whose TARGET assertions need Go-level observability — the
// stamp report, the interp-entry hook, the runtime-bail hook. Pure
// source→value/error frontier repros live as shared TSV rows instead
// (lang/spec/frontier/, WS2 of the plan) so the TS port runs them too.

// --- error-returning assertion helpers --------------------------------------

// fcNew builds a fresh instance or reports why it could not.
func fcNew() (*AQL, error) {
	a, err := New()
	if err != nil {
		return nil, fmt.Errorf("New: %w", err)
	}
	return a, nil
}

// fcParityCompiledZeroBails asserts the FULL target for a source-level
// frontier gap: the program compiles (no refusal), runs on the VM with zero
// designed runtime bails, and its values, error taxonomy, AND printed output
// are byte-identical to the interpreter's.
func fcParityCompiledZeroBails(src string) error {
	a, err := fcNew()
	if err != nil {
		return err
	}
	var outC bytes.Buffer
	a.SetOutput(&outC)
	var bails []BailEvent
	defer a.ArmRuntimeBailHook(func(e BailEvent) { bails = append(bails, e) })()
	gotC, compiled, reason, errC := a.RunCompiledReason(src)
	if !compiled {
		if reason != "" {
			return fmt.Errorf("refused: %s", reason)
		}
		return fmt.Errorf("did not run compiled (err=%v)", errC)
	}
	if len(bails) > 0 {
		return fmt.Errorf("runtime bails: %s", bailCensus(bails))
	}

	b, err := fcNew()
	if err != nil {
		return err
	}
	var outI bytes.Buffer
	b.SetOutput(&outI)
	gotI, errI := b.Run(src)
	if codeOf(errC) != codeOf(errI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		return fmt.Errorf("error parity: compiled [%s] %v vs interp [%s] %v", codeOf(errC), errC, codeOf(errI), errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		return fmt.Errorf("value parity: compiled %v vs interp %v", gotC, gotI)
	}
	if outC.String() != outI.String() {
		return fmt.Errorf("output parity: compiled %q vs interp %q", outC.String(), outI.String())
	}
	return nil
}

// fcStampedRun runs src through RunCompiled and asserts the stamp report
// carries a SUCCESSFUL stamp for a binding whose name contains name.
func fcStampedRun(src, name string) error {
	a, err := fcNew()
	if err != nil {
		return err
	}
	a.SetOutput(&bytes.Buffer{})
	if _, _, err := a.RunCompiled(src); err != nil {
		return fmt.Errorf("run failed before the stamp assertion: %w", err)
	}
	var attempted *StampEvent
	for i, ev := range a.StampReport() {
		if strings.Contains(ev.Name, name) {
			if ev.Stamped {
				return nil
			}
			attempted = &a.StampReport()[i]
		}
	}
	if attempted != nil {
		return fmt.Errorf("stamp refused for %q: %s", name, attempted.Reason)
	}
	return fmt.Errorf("no stamp attempt recorded for %q", name)
}

// fcNoUnattributedInterp arms the interp-entry hook, runs drive, and fails
// when any interpreter entry lacks a C4 attribution — the end-state invariant
// ("no interpreter execution of an accepted program on a default path").
func fcNoUnattributedInterp(drive func(a *AQL) error) error {
	a, err := fcNew()
	if err != nil {
		return err
	}
	a.SetOutput(&bytes.Buffer{})
	var (
		mu      sync.Mutex
		entries []InterpEntry
	)
	disarm := a.ArmInterpEntryHook(func(e InterpEntry) {
		mu.Lock()
		defer mu.Unlock()
		entries = append(entries, e)
	})
	defer disarm()
	if err := drive(a); err != nil {
		return fmt.Errorf("drive failed before the entry assertion: %w", err)
	}
	mu.Lock()
	defer mu.Unlock()
	counts := map[string]int{}
	for _, e := range entries {
		if e.Attribution == "" {
			counts[e.Seam]++
		}
	}
	if len(counts) > 0 {
		return fmt.Errorf("unattributed interpreter entries: %s", seamCensus(counts))
	}
	return nil
}

func seamCensus(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s×%d", k, counts[k])
	}
	return strings.Join(parts, " ")
}

func bailCensus(bails []BailEvent) string {
	counts := map[string]int{}
	for _, b := range bails {
		counts[b.Site]++
	}
	return seamCensus(counts)
}

// --- shared frontier sources -------------------------------------------------

// lJoinRepro is the L-JOIN minimal repro verbatim from
// design/VOXGIG-COMPILE-LEAVES.2.md — the recursive branch-join accumulator
// whose self-call operand loses provenance across fixpoint iterations. The
// only library-code blocker (tst.aql / radix.aql).
const lJoinRepro = `def rec fn [
  [nd:Any key:Any consumed:Any best:Any] [Any] [
    if (nd eq none) [best] [
      def pc (consumed "x" add)
      def best2 (if (nd "end" get) [pc] [best])
      best2 consumed key (nd "mid" get) rec
    ]
  ]
]
(rec none "hi" "" none) print end`

// --- the inventory ------------------------------------------------------------

var frontierCases = []frontierCase{
	// Phase 4 — GRADUATED 2026-07-14 (permanent pin): the L-JOIN repro
	// compiles (the disjunct-distribution recording fix, carrier.go
	// disjunctPartitionReturns/carrierResults) and runs compiled with ZERO
	// runtime bails and full parity — the staged L-NP handoff never fired.
	{"p4/l-np-no-runtime-bail-after-join", func() error {
		return fcParityCompiledZeroBails(lJoinRepro)
	}},

	// Phase 6 — stamping extensions.
	{"p6/predicate-stamps-and-runs-vm", func() error {
		return fcStampedRun(`def Pos fn [[n:Integer] [Boolean] [n gt 0]] def x:Pos 5 x`, "Pos")
	}},
	{"p6/model-action-stamps", func() error {
		return fcStampedRun(`import "aql:model" def m (Model.new {src:'a: 1 b: 2', actions:{gen:([mod:Any] => [true])}}) (Model.run m) get 'ok'`, "gen")
	}},
	{"p6/capturing-handler-stamps", func() error {
		// The capture-decline shape from run_compile_report_test.go (verbatim
		// — the leading map-lambda statement is load-bearing for the parse):
		// the service handler closes over n, so StampDetachedFn declines.
		return fcStampedRun(`def m {f: ([y:Integer] => [y add 1])} add 1 ((m get "f") 5) drop def mk (fn [[n:Integer] [Any] [ def svc (service {}) add {cmd:"N"} ([req:Map state:Any] => [ n ]) svc svc ]]) def s (mk 7) (call {cmd:"N"} s)`, "anonymous fn")
	}},
	{"p6/check-prop-body-on-vm", func() error {
		// Module-scope check-prop (the compiling half of
		// bytecode_checkprop_interp_test.go). Target: the per-iteration
		// gen/property bodies run as JIT'd units — zero unattributed
		// interpreter entries. The fn-scope-refuses miscompile guard
		// (TestCheckPropInterpStringFnScopeRefuses) stays green forever and
		// is never ledgered.
		src := "import \"aql:test\" end\ndef res (Test.check-prop \"x\" [r.int 1 9] [ var [[k] (`v${k}`) eq `v${k}` ] ] 5 1 0)\nres get \"ok\""
		return fcNoUnattributedInterp(func(a *AQL) error {
			_, err := a.RunCompiledStrict(src)
			return err
		})
	}},
	{"p6/concurrent-fork-bodies-on-vm", func() error {
		return fcNoUnattributedInterp(func(a *AQL) error {
			_, _, err := a.RunCompiled(`import "aql:time-util" TimeUtil.await [[1 add 2] [3 add 4]]`)
			return err
		})
	}},
	{"p6/vm-run-on-vm", func() error {
		return fcNoUnattributedInterp(func(a *AQL) error {
			_, _, err := a.RunCompiled(`import "aql:vm" Vm.run "1 add 2"`)
			return err
		})
	}},

	// Phase 10 — the executed-census seeds.
	{"p10/no-unattributed-interp-on-islanded-program", func() error {
		// A genuine whole-program refusal (the each variadic-if knownRefusals
		// row): today the silent fallback re-runs the source unattributed.
		// Target: every residual interpreter entry belongs to a named C4 seam.
		return fcNoUnattributedInterp(func(a *AQL) error {
			_, _, _ = a.RunCompiled(zzRefusingRow) // the row raises; the entries are the assertion
			return nil
		})
	}},
	{"p10/runtime-bail-census-canary", func() error {
		// The zz-inst shape-claim violation is a real, reachable runtime bail;
		// the executed census must reach zero before Stage J.
		a := zzShapedInstanceE()
		if a == nil {
			return fmt.Errorf("zz-inst fixture unavailable")
		}
		a.SetOutput(&bytes.Buffer{})
		var bails []BailEvent
		defer a.ArmRuntimeBailHook(func(e BailEvent) { bails = append(bails, e) })()
		if _, _, err := a.RunCompiled(`def i (zz-inst) ; i.m 5 ; 42`); err != nil {
			return fmt.Errorf("run failed before the bail assertion: %w", err)
		}
		if len(bails) > 0 {
			return fmt.Errorf("runtime bails: %s", bailCensus(bails))
		}
		return nil
	}},

	// Phase 11 — Stage J.
	{"p11/public-run-is-compiled", func() error {
		return fcNoUnattributedInterp(func(a *AQL) error {
			_, err := a.Run(`1 add 2`)
			return err
		})
	}},
	{"p11/no-unbounded-fallback", func() error {
		// Post-Stage-J a refusal returns an error instead of silently
		// re-running the whole source on the interpreter; graduation is
		// coupled with rewriting the mustRefuseWithParity-family contracts.
		// (The former no-unattributed-entries assertion graduated early —
		// the pre-Stage-J bounded fallback is now ATTRIBUTED
		// (fallback:refusal), which is C4's target, so this case pins the
		// REMAINING contract: the fallback's absence itself.)
		// The probe must be a refusing program that SUCCEEDS interpreted (a
		// raising one cannot distinguish the fallback's error from a
		// returned refusal): the paren-bounded fn-value application refuses
		// and runs to bad_input/q on the interpreter.
		const refusingButSucceeds = `def zf fn [[x:Any] [Any] [raise bad_input 'no']]  def msg (do [(zf 5) 2] error [dot code])  msg`
		a, err := fcNew()
		if err != nil {
			return err
		}
		a.SetOutput(&bytes.Buffer{})
		out, compiled, rerr := a.RunCompiled(refusingButSucceeds)
		if !compiled && rerr == nil && len(out) > 0 {
			return fmt.Errorf("refusal resolved by the silent interpreter fallback (post-Stage-J it returns the refusal error)")
		}
		return nil
	}},
}

// zzShapedInstanceE is zzShapedInstance without the *testing.T (cases are
// data): it returns nil if the fixture cannot be built.
func zzShapedInstanceE() *AQL {
	a, err := New()
	if err != nil {
		return nil
	}
	zzInstallShapedInstance(a)
	return a
}

var frontierLedger = map[string]frontierEntry{
	// GRADUATED 2026-07-14: p4/l-np-no-runtime-bail-after-join — BOTH stages
	// at once: the L-JOIN refusal was a per-alternative recording leak (the
	// disjunct-distribution combos ran a user fn's ReturnsFn under the armed
	// recording with fresh-ID alternative copies — RecordUserCall then refused
	// "unknown provenance"); the fix suspends the combo probes and records ONE
	// CALL_USER with the original args (partition-joined carriers re-IDed onto
	// the recorded results, gated by disjunctCombosTakeSig). The anticipated
	// L-NP vm:dyn-scope-miss bail never materialised — the repro runs compiled
	// with ZERO runtime bails and full parity (bytecode_ljoin_test.go pins the
	// family). The case above stays as a permanent pin.
	// GRADUATED 2026-07-14: p6/predicate-stamps-and-runs-vm — predicates
	// stamp at construction (InstallType stamps the body in place, naming
	// the stamp event with the type name; RunPredicate's InvokeCallback then
	// runs the unit on the VM). The case above stays as a permanent pin.
	// GRADUATED 2026-07-14: p6/model-action-stamps — buildActions stamps each
	// action at model build (stampActionFn: the model's private copy takes the
	// action name and a detached unit; InvokeCallback runs it on the VM). The
	// case above stays as a permanent pin.
	"p6/capturing-handler-stamps": {
		why:       "plan Phase 6.3: StampDetachedFn declines lexical captures (eng stamp_runtime.go); capturing bodies need closure units with capture slots",
		failsWith: "no stamp attempt recorded for \"anonymous fn\"",
	},
	"p6/check-prop-body-on-vm": {
		// DRIFTED 2026-07-14 (the body half LANDED): the gen/property bodies
		// compile as same-program stored-param-body units
		// (Signature.StoredBodies → compileStoredParamBody) and run nested on
		// the VM via InvokeCallback — the per-iteration CallAQL entries are
		// GONE, and iteration count adds ZERO entries (pinned by
		// TestCheckPropIterationsAddNoInterpEntries). The residual entries
		// are `import "aql:test"` MODULE-LOAD AQL (BuildTestModule preamble
		// runs — identical counts for an import-only program), which needs
		// the Phase 10 module-load attribution seam, not body compilation.
		why:       "plan Phase 10: aql:test module-load AQL runs unattributed (Engine.Run/runPooledSub at BuildTestModule) — the module-load C4 seam is unbuilt",
		failsWith: "unattributed interpreter entries: Engine.Run",
	},
	// GRADUATED 2026-07-14: p6/concurrent-fork-bodies-on-vm — await's
	// parallels compile per element (CompileStoresBodyList → compileStoredBody
	// carriers) and runParallelBranch runs each via RunUnit on its fork.
	// The case above stays as a permanent pin.
	"p6/vm-run-on-vm": {
		why:       "plan Phase 6.6: Vm.run executes runtime-constructed source in a sub-engine; the fork-isolated runtime compile is unbuilt (tier-1 island)",
		failsWith: "unattributed interpreter entries: Engine.Run",
	},
	"p11/public-run-is-compiled": {
		why:       "plan Phase 11: the public (*AQL).Run is the tree-walker until Stage J flips it to the compiled path (RunInterp retained as the oracle)",
		failsWith: "unattributed interpreter entries: Engine.Run",
	},
	"p11/no-unbounded-fallback": {
		why:       "plan Phase 11 (C2): the nil-Program branch silently re-runs the whole source; post-Stage-J a refusal returns an error and only the bounded static-error oracle touches the interpreter",
		failsWith: "refusal resolved by the silent interpreter fallback",
	},
}
