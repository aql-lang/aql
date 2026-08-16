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
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/parser/go"
	"github.com/boru-lang/boru/test/specfix"
)

// The shared frontier TSV corpus (lang/spec/frontier/*.tsv — the flat corpus
// glob skips subdirectories, so these rows sit OUTSIDE the live refusal/
// island ratchets). Each row is an ordinary spec row — `input⇥expected` with
// the interpreter as the semantics oracle, exactly the format a TS port will
// run — whose COMPILE status is the frontier: the expected-red ledger below
// pins which rows the compiler refuses today and why, with the same
// stale/drift/bootstrap contract as the lang-package frontier ledger
// (lang/go/frontier_ledger_test.go) and knownRefusals. Graduation = the row
// compiles → delete its ledger entry and (usually) move the row into the
// main lang/spec corpus so the census owns it.
//
// TestFrontierRefusalRowsCompile is the sibling inventory for the 9
// knownRefusals rows: those already live in the MAIN corpus, so their
// frontier cases read the sources straight from the knownRefusals map (one
// source of truth) and assert the TARGET (compile + byte-identical
// error/value parity); graduation is coupled per-row with the knownRefusals
// deletion.

type frontierRow struct {
	file  string
	line  int
	input string
	want  string
}

func loadFrontierRows(t *testing.T) []frontierRow {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "lang", "spec", "frontier")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("frontier spec dir: %v", err)
	}
	var rows []frontierRow
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("open %s: %v", e.Name(), err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				t.Fatalf("%s:%d: malformed row (no tab)", e.Name(), lineNo)
			}
			rows = append(rows, frontierRow{
				file:  e.Name(),
				line:  lineNo,
				input: strings.TrimSpace(parts[0]),
				want:  strings.TrimSpace(parts[1]),
			})
		}
		f.Close()
	}
	if len(rows) == 0 {
		t.Fatal("no frontier rows loaded")
	}
	return rows
}

// runFrontierInterp evaluates a row on the interpreter with the SAME wiring
// as the production spec runner (langspec_test.go's closure) and reports the
// canonical outcome string (core.Canon of the stack, or ERROR:<text>).
func runFrontierInterp(input string) (string, error) {
	values, err := parser.Parse(input)
	if err != nil {
		return "ERROR:" + err.Error(), nil
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		return "", err
	}
	specfix.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)
	out, rerr := native.NewTop(reg).Run(values)
	if rerr != nil {
		return "ERROR:" + rerr.Error(), nil
	}
	return core.Canon(out), nil
}

// TestFrontierSpecInterp — the semantics oracle: every frontier row must PASS
// on the interpreter (rows are green semantics whose compile status is red).
// An expected of BOOTSTRAP fails printing the observed outcome verbatim, so
// populating a new row's expected column is enforced, exactly like the
// ledger's failsWith sentinel.
func TestFrontierSpecInterp(t *testing.T) {
	for _, row := range loadFrontierRows(t) {
		got, err := runFrontierInterp(row.input)
		if err != nil {
			t.Errorf("%s:%d: harness: %v", row.file, row.line, err)
			continue
		}
		switch {
		case row.want == "BOOTSTRAP":
			t.Errorf("%s:%d: BOOTSTRAP row — record the interpreter outcome into the expected column. Observed: %s", row.file, row.line, got)
		case strings.HasPrefix(row.want, "ERROR:"):
			if !strings.HasPrefix(got, "ERROR:") || !strings.Contains(got, strings.TrimPrefix(row.want, "ERROR:")) {
				t.Errorf("%s:%d: interpreter outcome %q, want error containing %q", row.file, row.line, got, row.want)
			}
		case got != row.want:
			t.Errorf("%s:%d: interpreter outcome %q, want %q", row.file, row.line, got, row.want)
		}
	}
}

// docMod is the shared module preamble of the do-catch rows (a value-
// dependently-raising fn and an always-raising one, reached as M.dec/M.boom).
// Must match the TSV rows byte-for-byte — the orphan arm catches drift.
const docMod = `import module [ def dec fn [[bad:Boolean x:Any] [Any] [ if bad [raise bad_input "boom"] [x] ]] def boom fn [[x:Any] [Any] [ raise bad_input "always" ]] export "M" {dec: dec/r, boom: boom/r} ] end `

// frontierCompileLedger pins the frontier rows the compiler REFUSES today,
// keyed by exact input (the knownRefusals convention). failsWith pins the
// refusal reason substring (stable core only); "" is the bootstrap sentinel.
// Signatures transcribed from the 2026-07-13 bootstrap run.
var frontierCompileLedger = map[string]frontierEntryLS{
	// NUR054 — a `context` read inside an inline-lowered body (here an
	// auto-evaluated def-list): the interpreter gives that body its own
	// context layer, the inline stream has no layer to hand out, and every
	// layer-distinguishing consumption of the handle would diverge — so the
	// mint refuses (recordDispatchOutcome) and the interpreter owns the
	// program. Moved from flex.tsv:304 (its point is flex gradual typing,
	// not context scoping — the green semantics stay on record here).
	// Graduation = an emitted context-frame opcode pair bracketing
	// inline-lowered regions.
	`context set 'k' 1 end context del 'k' end context set 'k' 2 end def l [(context get 'k')] (l get 0) add 1`: {why: "NUR054: a context read inside an auto-evaluated list has no compiled context layer", failsWith: "no layer to hand out"},

	// Conditional fn-shadow — a MISCOMPILE (variation sweep,
	// forward-barrier.tsv:73); now a SOUND REFUSAL: a user fn redefined
	// inside a conditionally-reached body overlap-removes the enclosing
	// overload in place, so the branch/loop def rollback cannot restore it and
	// compiled resolution bakes the shadow while the interpreter keeps the
	// outer fn on the not-taken / zero-iteration path. Refused CondBodyDepth-
	// gated (eng/go/core_helpers.go). Full graduation = a runtime dispatch
	// respecting the conditional binding compiles these rows.
	`def g fn [[x:Any] [Integer] [x add 100]] if false [def g fn [[x:Any] [Integer] [x add 1]]] g 1`: {why: "conditional fn redefinition shadows an outer overload; compiled bake would diverge from the interpreter on the not-taken branch", failsWith: "redefined inside a conditional body"},

	// L-DO — plan Phase 5: the body nets N values on no-raise but 1 Error on
	// raise; needs OpStackMark/OpDropToMark variable arity across the catch
	// merge. One entry per fallibility route (Reach raise, no-raise-at-input,
	// always-raise, value-diverging native, user fn body, bare module-export
	// value, branch-arm nesting).
	// L-DO PART 1 LANDED (2026-07-13): fallible multi-value do results now
	// record VARIADIC (the SetCatchVariadic latch) instead of refusing at the
	// ReturnsFn — the div row graduated to the main corpus, and the remaining
	// rows drifted to the DOWNSTREAM refusals below: `error` consuming the
	// variadic region's top needs the part-2 region-top lowering
	// (strip-input over a variadic region; see the L-DO implementation map
	// in the completion plan).
	// Re-diagnosed 2026-07-20 (PR #280 review): the def-bound variadic
	// region now refuses at lowerCall's store-prologue gate — a promoted
	// variadic result's stores pop success-arity values the raise path
	// never delivers — one stage before the "residual shape beyond Stage 1"
	// decline these rows used to surface. Same sound refusal, earlier and
	// truer diagnosis.
	// Re-diagnosed 2026-07-30 (design/FN-VALUE-DISPATCH.0.md): the region's
	// `M.dec` call fails dispatch, which is now an error-severity check
	// diagnostic in the model-undermining class (dispatch did not resolve, so
	// there is no call to compile) — the pipeline therefore refuses on the
	// diagnostics one stage before the promotion gate these rows used to
	// reach. The INTERPRETER runs them: the failure raises at the call, inside
	// the `do`, so the region's own handler catches it (see the note in
	// frontier-do-catch.tsv, and the check-vs-run divergence recorded in the
	// design note's §6). The L-DO promotion work below is still what would
	// graduate the SHAPE; these two rows can no longer witness it.
	docMod + `def msg (do [(true 5 M.dec) "no-raise"] error [dot code])  msg`:  {why: "plan Phase 5 (L-DO part 2): variadic region under a def binding — now check-rejected first (failed fn-value dispatch)", failsWith: "check diagnostics"},
	docMod + `def msg (do [(false 5 M.dec) "no-raise"] error [dot code])  msg`: {why: "plan Phase 5 (L-DO part 2): same shape, no raise at this input — likewise check-rejected first", failsWith: "check diagnostics"},
	// PR #280 review's promotion-gate representative (the variation
	// differential's prefix-stack find): a BRANCH-VARIANT multi-out do body
	// (0-or-2 values per arm) is variadic without any raise in sight, and a
	// dirty-stack prefix forces its promotion — the same exact-arity seat
	// hazard as the fallible regions.
	`7 def b true  do [1 2 (if b [] [9 9])]`: {why: "PR #280 review: branch-variant multi-out do region promoted under a dirty-stack prefix", failsWith: "variadic result promoted to frame slots"},

	// Chained forward application of Function params (frontier-chained-apply
	// .tsv) — the compose family, a live MISCOMPILE until 2026-08-02 (the
	// whole-frame replay's flat window lost the paren structure: compiled
	// RET count-error, interpreted 14), then a sound refusal
	// (noteDynFrameReplay declines a window with >1 applicable value).
	// GRADUATED 2026-08-03 in three coordinated steps: (1) the Stage-G
	// single-arg increment — a leading one-arg fn-carrier apply `(g x)`
	// records the trailing spelling's RecordDynApply event (compose/twice →
	// fn-value.tsv §8); (2) checkModeParenFnCollapse — the plain-surface
	// collapse twin — killed the def-split checker FP
	// (check_fn_param_apply_def_fp_test.go is the positive pin); (3) the
	// replayIsBodyTail windowReadsID widening — a dyn-bind of a value the
	// window reads is not a reorderable event — armed the def-split body
	// tail, so the stage row compiles natively too (fn-value.tsv §8). The
	// family's remaining refusal is the CHAINED MULTI-ARG apply
	// (`f (g x y)`), ledgered above in the emit-refusal families via
	// lang/go/bytecode_chained_apply_test.go's TestMultiArgChainedApplyRefuses
	// (no separate frontier entry: the two-applicable window refusal is the
	// §9.1 class, "unapplied fn-value in body residual").

	// Full-stack words GRADUATED 2026-08-03 (EmitState.FoldFullStack —
	// static fold over a provably-exact stack; rows moved to
	// corpus-core.tsv). The remaining sub-frontier
	// (frontier-full-stack.tsv): a roll permuting two EVENT results asks
	// the program residual to re-push call results in a non-production
	// order — beyond the Stage-1 residual discipline. The fold models it
	// correctly; the LOWERING declines. Graduation = program-residual
	// ordering beyond Stage 1.
	`(1 add 2) (3 add 4) 1 roll`: {why: "two event results permuted by roll: the residual re-push order exceeds Stage 1", failsWith: "residual shape beyond Stage 1"},
	// A full-stack word inside a wrapped code body compiles WITH an island
	// (the fold is gated to the top unit; the body's island machinery owns
	// the occurrence — sound interpreter re-entry, parity held). Graduation
	// = per-unit exactness for the fold. Same bucket pinned in
	// varyRefusalLedger ("islanded").
	`[10 20] each [drop 1 2 3 1 pick]`: {why: "full-stack word in a code body: the fold declines outside the top unit; the island seam owns it", failsWith: "islanded"},

	// Bare deref of a Function-typed PARAM where no argument is available
	// (design/FUNCTION-VALUE-SCOPE.0.md §11 rule 3, §12.4). The SLOT is fine —
	// stepWord's TFunction intercept binds the argument as a reference. Reading
	// the bound param in the body is what diverges: the interpreter treats a
	// bare name as a CALL (arity 0 applies, arity >=1 raises), the compiler
	// treats a param as a VALUE slot (RegisterLocal) and yields the Function.
	// Both are internally consistent under different models, and both are
	// check-clean, so this is a language decision rather than a defect with a
	// right answer. The middle case (arity >=1 WITH arguments) already agrees on
	// both engines and is pinned green five times in the main corpus.
	//
	// Graduation = a maintainer decision on one question: is a Function-bound
	// name read with no argument available a nullary call, or a value?
	`def nought fn [[] [Integer] [7]] def grab fn [[f:Function] [Any] [f]] typeof (grab nought)`:          {why: "arity-0 Function param read bare: interpreter applies it, compiler yields the Function", failsWith: "value parity"},
	`def dbl fn [[n:Integer] [Integer] [n mul 2]] def hold fn [[c:Function] [Any] [c]] typeof (hold dbl)`: {why: "arity-1 Function param read bare with no argument: interpreter raises, compiler yields the Function", failsWith: "parity"},

	// Cross-module fn value in a higher-order word's CLOSURE slot
	// (design/FUNCTION-VALUE-SCOPE.0.md §12.3). A fn value resolves its free
	// words in its DEFINING module; the closure lowering compiles the body
	// against the CALLING one, so compiling this baked the caller's `lim`
	// (100) and returned [] where the interpreter returns [3 4] — a
	// check-clean miscompile. foreignFnHome (compiler/go/callable_words.go)
	// declines the lowering, and the callback seam's island runs the body on
	// its own registry (core.CallBoruFn), so parity holds and the island is
	// merely slow.
	//
	// Graduation = compile the foreign body against fd.Registry. The RUNTIME
	// half already exists: CompiledFn.Reg (compiler/go/bytecode.go:970) plus
	// enterUnit's `if p.Fns[u].Reg != nil { curReg = p.Fns[u].Reg }`
	// (eng/go/vm.go:1403-1414) already give a closure unit its own dispatch
	// registry, and StartFnCompile's fnReg parameter (compiler/go/emit.go:3314)
	// already plumbs it. Three of the four compile-side roles are solved
	// patterns the NAMED foreign-fn path already uses — shareCheckStateFrom
	// (check/go/check_recovery.go:527) for the CheckState, and
	// fd.Registry.AnalysisScopeID() for the memo key, exactly as
	// check/go/check_fnbody.go:312,513 does. The one unsolved role is CAPTURE
	// OPERANDS: recordClosureDispatch resolves them in the CALLER's emit tables
	// (callable_words.go:474-481) and dynScopeRescue re-resolves them at run
	// time against the caller's curReg (eng/go/vm.go:1902), so a foreign
	// module-scope capture has no operand home. Closing it needs either a
	// refusal for foreign closures with non-lexical captures, or a
	// registry-tagged dyn-scope operand so OpLookupDynScope can name
	// fd.Registry. The context bracket is a second, smaller asymmetry:
	// enterBodyUnit pushes/pops on the CALLING registry (vm.go:294-300) while
	// curReg would be fd.Registry.
	`import module [def lim fn [[n:Integer] [Integer] [2]] def big fn [[e:Map] [Boolean] [(e dot value) gt (lim 0)]] export "A" {big: big/r}] end def lim fn [[n:Integer] [Integer] [100]] filter A.big [1 2 3 4]`: {why: "a fn value from another module reaches a higher-order word's closure slot; the lowering would resolve its free words in the CALLING module, so it declines and the callback-seam island owns it", failsWith: "islanded"},

	// Gradual-Any to a multi-overload user fn with DIFFERING arm returns —
	// the P1.3 target — GRADUATED 2026-08-03 (completeness-review §8.2(3)/
	// §9.11): tryCompileUserPolyArms records the position-wise JOIN of the
	// arms' returns (userPolyPlan.outs — a dynamic carrier at the arms'
	// common ancestor), userPolyArmShapeOK relaxed to count + nil-ness
	// agreement, and applyGradualContagion's first-match-partition widening
	// preserves the recorded identity (out[0].ID) so the poly event
	// survives to the elision. Both rows compile natively via
	// OpCallUserPoly (moved to lang/spec/fn-value.tsv §9; pinned in
	// lang/go/bytecode_poly_join_test.go with the count-mismatch negative).

	// `do … error` with a zero-netting handler (frontier-do-error-arity
	// .tsv) — the P1.6 target, GRADUATED 2026-08-03 for the PROVEN-raise
	// shape (completeness-review §9.13): a strict Error do-result fixes the
	// arity at zero, errorReturnsFn returns it truthfully, and the
	// strip-input shape screen's want-0 arm admits the empty residual —
	// the row compiles natively (moved to the main corpus; pinned in
	// lang/go/bytecode_do_error_arity_test.go). The MAYBE-raising twin
	// below keeps the refusal: a dynamic Error bound has variable arity
	// (pass-through 1 vs caught 0), the true remaining §8.2(6) target
	// (the variable-arity island via the mark machinery).
	`def xs [0] do [1 div (xs 0 getr)] error [drop] end 2 add 3`: {why: "maybe-raising body with a zero-netting handler: the pass-through nets one where the caught path nets zero — no fixed seat", failsWith: "handler nets no value"},

	// NUR038 statement-seal twin-call matrix (frontier-nur038-seal.tsv):
	// semantically green under the seal + arrival barrier; compile-refused
	// on the pre-existing residual-ordering limitation — two dynamic call
	// results in one program residual exceed the Stage-1 lowering. Sound
	// interpreter fallback. Graduation = multi-dynamic-result residual
	// lowering; the rows then move to lang/spec/fn-value.tsv §6.
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/r} end m.p 5 m.p 7`:                                {why: "NUR038 seal: twin value-call residual", failsWith: "fn-value-call boundary"},
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/r} end m.p 1 m.p 2 m.p 3`:                          {why: "NUR038 seal: triple value-call residual", failsWith: "fn-value-call boundary"},
	`def g fn [[a:Any b:Any] [Any] [(a mul 100) add b]] end def m {g: g/r} end m.g 1 2 m.g 3 4`:      {why: "NUR038 seal: two-arg twin windows", failsWith: "fn-value-call boundary"},
	`import module [def p fn [[x:Any] [Any] [x]] export "M" {p: p/r}] end M.p 5 M.p 7`:               {why: "NUR038 seal: module-export twins (the original shape)", failsWith: "fn-value-call boundary"},
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/r} end 5 m.p m.p 7`:                                {why: "NUR038 seal: stack form then forward form", failsWith: "fn-value-call boundary"},
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/r} end m.p (1 add 2) m.p 7`:                        {why: "NUR038 seal: computed first argument", failsWith: "fn-value-call boundary"},
	`def m {l: ([x:Any] => [x])} end m.l 5 m.l 7`:                                                    {why: "NUR038 seal: lambda twins", failsWith: "fn-value-call boundary"},
	`def f fn [[x:Any] [Any] [x]] end def h fn [[y:Any] [Any] [y]] end def m {p: f/r} end m.p 5 h 7`: {why: "NUR038 seal: value call then bare-word call", failsWith: "fn-value-call boundary"},
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/r} end (m.p 5) (m.p 7)`:                            {why: "NUR038 seal: explicit paren seals", failsWith: "fn-value-call boundary"},
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/r} end m.p 5 end m.p 7`:                            {why: "NUR038 seal: explicit end seals", failsWith: "fn-value-call boundary"},
	`def e fn [[] [Integer] [42] [x:Any] [Any] [x]] end def m {e: e/r} end m.e 5 m.e 7`:              {why: "NUR038 seal: mixed 0/1-arg overload twins (NUR035 guard)", failsWith: "fn value read from a container auto-dispatches"},

	// Namespace capture at a macro-expanded call site (the NUR038 wrapper
	// retirement's re-bucketed refusal — see frontier-capture-namespace.tsv):
	// an inner fn capturing a body-imported module namespace has no bakeable
	// operand home at the `parse` macro's expanded call site. Successor to
	// the graduated "closure captures a runtime-minted value" bucket (the
	// wrapper refused earlier, at capture-slot numbering). Full graduation =
	// a capture-slot lowering that materialises the namespace binding.
	`def zzvfn fn [[] [] [import "boru:parselang"  import "boru:string-util"  def calc (fn [[source:Any opts:Map] [List] [StringUtil.split ' ' (ParseLang.source source)]])  end  (parse calc {trace:true} 'x + y') get 1]] zzvfn`: {why: "inner fn captures the body-imported ParseLang namespace; no bakeable operand home at the macro-expanded call site", failsWith: "capture ParseLang of calc unreachable at a call site"},
	// GRADUATED 2026-07-17 (§9.1): the `do [M 3] error [dot code]` row
	// compiles — an identity-less dyn-body out (the module-export instance)
	// now mints a fresh ID at the record, restoring its tape placement and
	// event linkage, so the mark-window island owns the region as usual.
	// GRADUATED 2026-07-14 (the mark-window island, L-DO part 2b): the
	// error-over-the-variadic-region rows — do [(M.boom 5) "x"] / the [Any]
	// user-fn twin / the branch-arm nesting / the StructUtil.parse chained
	// leaf — compile natively: Finalize's markWindowShape opens an
	// OpStackMark before the region-starting do event and the residual
	// islands verbatim through OpCallDynMixedFromMark (rows moved to
	// lang/spec/bytecode-migrated.tsv; family pinned in
	// lang/go/bytecode_markwindow_test.go). The def-msg rows above and the
	// module-export row keep their sound refusals (a PROMOTED def read /
	// a non-event region entry decline the window).

	// NUR031 Module-descriptor equality (frontier-nur031-module-eq.tsv):
	// semantically green — the descriptor is an identity-equal opaque
	// handle since 2026-08-02 — but a `$module` synthetic read has no
	// bakeable operand home, so the operand reaching eq/deq carries no
	// static provenance. The PRE-fix binary refuses the identical shape:
	// the fix changed the answer, not the compile status. Graduation =
	// a static provenance representation for the module-namespace
	// synthetics (the capture-namespace family above); the rows then
	// move back into edge-modules-1.tsv and compare-restrict.tsv.
	`import module [export "M" {a:1}] M.$module eq M.$module`:                                                                                                             {why: "NUR031: descriptor reflexive eq", failsWith: "operand of unknown provenance"},
	`import module [export "M" {a:1}] M.$module deq M.$module`:                                                                                                            {why: "NUR031: descriptor reflexive deq", failsWith: "operand of unknown provenance"},
	`import module [export "A" {x:1}] import module [export "B" {y:2}] A.$module eq B.$module`:                                                                            {why: "NUR031: distinct descriptors are not eq", failsWith: "operand of unknown provenance"},
	`import module [export "A" {x:1}] import module [export "B" {y:2}] A.$module deq B.$module`:                                                                           {why: "NUR031: distinct descriptors are not deq", failsWith: "operand of unknown provenance"},
	`import "boru:array-util" import module [export "A" {x:1} export "B" {y:2}] import module [export "C" {z:3}] size (ArrayUtil.unique [A.$module B.$module C.$module])`: {why: "NUR031: the Module handle family in unique's DeqIndex", failsWith: "operand of unknown provenance"},
	`import module [export "A" {x:1} export "B" {y:2} export "C" {z:3}] A.$module eq C.$module`:                                                                           {why: "NUR031: one module, many namespaces — one shared descriptor", failsWith: "operand of unknown provenance"},
	`import module [export "A" {x:1} export "B" {y:2}] A.$module deq B.$module`:                                                                                           {why: "NUR031: sibling namespaces of ONE module share a descriptor (deq mirrors eq)", failsWith: "operand of unknown provenance"},
	`import "boru:test" Test.$module eq Assert.$module`:                                                                                                                   {why: "NUR031: a native module's sibling namespaces share one descriptor", failsWith: "operand of unknown provenance"},
	`import "boru:string-util" import "boru:string-util" StringUtil.$module eq StringUtil.$module`:                                                                        {why: "NUR031: repeat import is a cache no-op — same descriptor instance", failsWith: "operand of unknown provenance"},
	`import "boru:string-util" def a StringUtil.$module undef StringUtil import "boru:string-util" a eq StringUtil.$module`:                                               {why: "NUR031: re-import after undef mints a FRESH descriptor (identity is per-import-instance)", failsWith: "operand of unknown provenance"},

	// NUR031 Function-value equality (frontier-nur031-fn-eq.tsv): the
	// second half of the record — eq is the identity token, deq is canon
	// content. Semantically green; refused for a reason unrelated to
	// equality, and one the PRE-fix binary refuses identically: a function
	// VALUE reaching a non-inert word is declined outright by
	// EmitState.RecordCallOperands. Graduation = a compiled representation
	// for a function value as an operand (the Stage-3 fn-value work); the
	// rows then move to compare-restrict.tsv and fn-value.tsv.
	`def f fn x:Integer [Integer] [x add 1] f/r eq f/r`:                                         {why: "NUR031: a function is reflexively eq", failsWith: "function value reaches eq (Stage 3)"},
	`def f fn x:Integer [Integer] [x add 1] f/r deq f/r`:                                        {why: "NUR031: …and reflexively deq — ADR-015's prerequisite for the kind", failsWith: "function value reaches deq (Stage 3)"},
	`def f fn x:Integer [Integer] [x add 1] def a (f/r) def b (f/r) a/r eq b/r`:                 {why: "NUR031: two names for one function are eq — identity survives rebinding", failsWith: "function value reaches eq (Stage 3)"},
	`def f fn x:Integer [Integer] [x add 1] def a (f/r) a/r eq f/r`:                             {why: "NUR031: …and eq to the function they were reached from", failsWith: "function value reaches eq (Stage 3)"},
	`def f fn x:Integer [Integer] [x add 1] def g fn x:Integer [Integer] [x add 1] f/r eq g/r`:  {why: "NUR031: identical content is NOT eq — eq is identity", failsWith: "function value reaches eq (Stage 3)"},
	`def f fn x:Integer [Integer] [x add 1] def g fn x:Integer [Integer] [x add 1] f/r deq g/r`: {why: "NUR031: …but it IS deq — deq is content", failsWith: "function value reaches deq (Stage 3)"},
	`def f fn x:Integer [Integer] [x add 1] def h fn x:Integer [Integer] [x add 2] f/r deq h/r`: {why: "NUR031: a different body is a different value", failsWith: "function value reaches deq (Stage 3)"},
	`def f fn x:Integer [Integer] [x add 1] def s fn x:String [String] [x] f/r deq s/r`:         {why: "NUR031: a different signature likewise", failsWith: "function value reaches deq (Stage 3)"},
	`def f fn x:Integer [Integer] [x add 1] canon (f/r)`:                                        {why: "NUR031: canon renders the anonymous fn literal — no binding name", failsWith: "function value reaches canon (Stage 3)"},
	`def f fn x:Integer [Integer] [x add 1] def a (f/r) (canon (a/r)) eq (canon (f/r))`:         {why: "NUR031: one function under two names has ONE canon", failsWith: "function value reaches canon (Stage 3)"},
	`def f fn x:Integer [Integer] [x add 1] f/r eq 1`:                                           {why: "NUR031: cross-type eq is false, never an error", failsWith: "function value reaches eq (Stage 3)"},
	`def f fn x:Integer [Integer] [x add 1] f/r deq 1`:                                          {why: "NUR031: cross-type deq likewise", failsWith: "function value reaches deq (Stage 3)"},
	// The namespace rows refuse for the MODULE-synthetic reason above, not
	// the fn-value one: a namespace binding has no bakeable operand home.
	`import "boru:io" IO deq IO`:                          {why: "NUR031: a function-exporting namespace is deq-reflexive — the record's acceptance signal", failsWith: "operand of unknown provenance"},
	`import "boru:string-util" StringUtil deq StringUtil`: {why: "NUR031: …and so is every other native module's namespace", failsWith: "operand of unknown provenance"},

	// NUR067 — await's winner-takes-all modes (frontier-await-winner.tsv):
	// `first` / `any` hand back the winning branch's WHOLE residual, 0-or-more
	// values — a count that can EXCEED any static seat, the direction the L-DO
	// variadic mark cannot express. The 1-seat layout was a live MISCOMPILE
	// (`size [(await {mode:'any'} [[7 8]])]` — interpreter 2, compiled a
	// stranded 7 and a 1-element list), so awaitVariadicResult now refuses the
	// compile pass wholesale and the interpreter owns these modes. Graduation
	// = a runtime-variadic region representation (an OpStackMark-style collect
	// with no static count); the refusal arm then records the region and the
	// rows move to lang/spec/module-time.tsv.
	`import "boru:time-util" TimeUtil.await {mode:'first'} [[1 2 3]]`:      {why: "NUR067: the winner's 3-value residual has no static seat", failsWith: "runtime-variadic (0-or-more values) with no static seat"},
	`import "boru:time-util" size [(TimeUtil.await {mode:'any'} [[7 8]])]`: {why: "NUR067: the miscompile shape — both values must reach the collecting paren", failsWith: "runtime-variadic (0-or-more values) with no static seat"},
	`import "boru:time-util" 99 TimeUtil.await {mode:'first'} [[]]`:        {why: "NUR067: an empty winner contributes nothing — the zero-count direction", failsWith: "runtime-variadic (0-or-more values) with no static seat"},

	// Net drivers — plan Phase 5: per-iteration mark/collect in the for: lowering.

	// GRADUATED 2026-07-14 (L-EACH, plan Phase 5): the three forward-drift
	// rows compile natively — errorReturnsFn narrows the catch result to
	// dynamic(join(pass-through, handler-residual)), so the String catch-all
	// overload is disjoint and check mode selects the interpreter's forward
	// collection (rows moved to lang/spec/bytecode-migrated.tsv; family
	// pinned in lang/go/bytecode_edge_findings_test.go with the genuinely
	// wide-join negative keeping the drift refusal).

	// do-unit registry replay — was a MISCOMPILE (variation sweep,
	// 2026-07-13); now a SOUND REFUSAL (drift graduated the same day): the
	// bake decision declines a body carrying a capitalised def
	// (bodyHasReplayHazard), so the interpreter owns the shape with full
	// parity. Full graduation = the Phase 6 JIT detached-unit cache compiles
	// these bodies as units (the check-time install becomes the only
	// install), at which point the rows compile and this entry deletes.
	// GRADUATED 2026-07-14: the do-unit registry-replay rows — do-def LEAK
	// fidelity (RunCarrierBodyKeepDefs) lets the closure re-analysis
	// shadow-rebind instead of tripping the parts conflict, so the typed-def
	// bodies compile as closure units with byte-identical results (rows
	// moved to lang/spec/bytecode-migrated.tsv; leak-semantics edges pinned
	// in lang/go/bytecode_replayhazard_test.go).

	// GRADUATED 2026-07-14: the L-JOIN recursive branch-join row — the
	// refusal was the disjunct-distribution recording per-alternative
	// (disjunctPartitionReturns combos under the armed recording); the fix
	// suspends the combo probes and records ONE CALL_USER with the original
	// args (carrier.go, gated by disjunctCombosTakeSig). Row moved to
	// lang/spec/bytecode-migrated.tsv; the family is pinned in
	// lang/go/bytecode_ljoin_test.go.
}

type frontierEntryLS struct {
	why       string
	failsWith string
}

// TestFrontierSpecCompiled — the compile frontier: an unledgered row must
// compile NATIVELY (no island) and run with byte-identical parity; a
// ledgered row must refuse with the pinned reason (stale → graduate; drift →
// re-diagnose).
func TestFrontierSpecCompiled(t *testing.T) {
	for _, row := range loadFrontierRows(t) {
		err := frontierRowCompiles(row.input)
		key := row.input
		entry, ledgered := frontierCompileLedger[key]
		loc := fmt.Sprintf("%s:%d", row.file, row.line)
		switch {
		case !ledgered && err != nil:
			t.Errorf("%s: frontier row must COMPILE (not ledgered): %v\n  input: %s", loc, err, row.input)
		case ledgered && err == nil:
			t.Errorf("%s: stale compile-ledger entry — the row now compiles; graduate it (delete the entry; usually move the row into the main lang/spec corpus).\n  was red because: %s", loc, entry.why)
		case ledgered && entry.failsWith == "":
			t.Errorf("%s: unpinned compile-ledger row — record the failure mode. Observed: %v", loc, err)
		case ledgered && !strings.Contains(err.Error(), entry.failsWith):
			t.Errorf("%s: compile failure MODE drifted:\n  got:    %v\n  pinned: %q\nre-diagnose before editing the ledger", loc, err, entry.failsWith)
		}
	}
	for key := range frontierCompileLedger {
		found := false
		for _, row := range loadFrontierRows(t) {
			if row.input == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("orphan compile-ledger entry (no such frontier row): %.80s…", key)
		}
	}
}

// frontierRowCompiles asserts the TARGET compile behavior for one row:
// a native (island-free) Program plus byte-identical value/error parity.
func frontierRowCompiles(input string) error {
	a, err := lang.New()
	if err != nil {
		return err
	}
	a.SetClock(specClock)
	prog, reason, _, cerr := a.CompileCheck(input)
	if cerr != nil {
		return fmt.Errorf("check error: %v", cerr)
	}
	if prog == nil {
		return fmt.Errorf("refused: %s", reason)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		return fmt.Errorf("islanded: program embeds an OpFallback span")
	}
	b, err := lang.New()
	if err != nil {
		return err
	}
	b.SetClock(specClock)
	gotC, compiled, errC := b.RunCompiled(input)
	if !compiled {
		return fmt.Errorf("did not run compiled (err=%v)", errC)
	}
	c, err := lang.New()
	if err != nil {
		return err
	}
	c.SetClock(specClock)
	gotI, errI := c.RunInterp(input)
	if fmt.Sprint(errC) != fmt.Sprint(errI) {
		return fmt.Errorf("error parity: compiled %v vs interp %v", errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		return fmt.Errorf("value parity: compiled %v vs interp %v", gotC, gotI)
	}
	return nil
}

// refusalRowLedger pins the knownRefusals rows' TARGET failure modes: each
// must eventually compile via the sound runtime re-dispatch mechanism (plan
// Phase 3, OpDispatchRematch) and raise the interpreter-identical error.
// DERIVED from knownRefusals — the single source of truth for the row
// sources — so graduation is auto-coupled: deleting a knownRefusals entry
// drops its ledger row here, flipping this test's assertion for that row to
// the target (compile + byte-identical parity). The failure mode is the
// LEADING CLAUSE of the knownRefusals reason text ("branch leaves extra
// values (…)" pins "branch leaves extra values"): the remaining row's
// dispatch half already records an offset-form rematch, so its refusal
// signature is the branch-residual seat, not the dispatch recovery — a row
// developing a different failure mode trips the drift arm.
var refusalRowLedger = func() map[string]frontierEntryLS {
	m := make(map[string]frontierEntryLS, len(knownRefusals))
	for input, why := range knownRefusals {
		mode := why
		if i := strings.Index(mode, " ("); i > 0 {
			mode = mode[:i]
		}
		m[input] = frontierEntryLS{
			why:       "plan Phase 3/5 (OpDispatchRematch + variadic-region merge): " + why,
			failsWith: mode,
		}
	}
	return m
}()

// TestFrontierRefusalRowsCompile asserts the TARGET for every knownRefusals
// row: CompileCheck yields a Program and RunCompiled matches the
// interpreter's error byte-for-byte. All 9 are expected-red until Phase 3
// lands, ratcheting down row-by-row in lockstep with knownRefusals.
func TestFrontierRefusalRowsCompile(t *testing.T) {
	for input := range knownRefusals {
		err := frontierRowCompiles(input)
		entry, ledgered := refusalRowLedger[input]
		switch {
		case !ledgered && err != nil:
			t.Errorf("knownRefusals row must COMPILE (not ledgered — did Phase 3 graduate it?): %v\n  input: %.100s", err, input)
		case ledgered && err == nil:
			t.Errorf("stale refusal-ledger entry — the row now compiles; graduate BOTH ledgers (delete here AND in knownRefusals).\n  input: %.100s\n  was red because: %s", input, entry.why)
		case ledgered && entry.failsWith == "":
			t.Errorf("unpinned refusal-ledger row — record the failure mode. Observed: %v\n  input: %.100s", err, input)
		case ledgered && !strings.Contains(err.Error(), entry.failsWith):
			t.Errorf("refusal row failure MODE drifted:\n  got:    %v\n  pinned: %q\n  input: %.100s", err, entry.failsWith, input)
		}
	}
	for input := range refusalRowLedger {
		if _, ok := knownRefusals[input]; !ok {
			t.Errorf("orphan refusal-ledger entry (row left knownRefusals): %.80s…", input)
		}
	}
}
