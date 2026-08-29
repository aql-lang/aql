// interp_entry_census_test.go measures the island the OpFallback ceiling
// cannot see.
//
// `TestCompiledCoverage` reports 0 islanded, and that number is real but
// narrow: it counts programs whose DISASSEMBLY embeds an OpFallback span. It
// cannot see a `CallBoru` made INSIDE a native handler, because no opcode
// records one — and that is where interpretation actually survives. Every
// predicate dispatch takes `InvokeCallback:callboru` today with no ledger row
// and no island flag (design/FULL-COMPILATION.0.md §2.3, §6.3).
//
// So "0 islanded" is a claim about the metric, not about the runtime. This
// census is the claim about the runtime: RUN each corpus row compiled with the
// InterpEntry hook armed — Stage 1 built it for exactly this — and count the
// entries that are UNATTRIBUTED, i.e. interpreter execution the end-state
// invariant does not permit. Check-mode entries are excluded: the compiler
// front end running RunInCheckMode words is attributed by construction.
//
// T2 ("No islands") is not satisfiable while this number is non-zero, whatever
// the OpFallback ceiling says. The ceiling below is a DOWNWARD ratchet, like
// refusalSiteCeiling: it only falls.
package langspec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// interpEntryRowCeiling is the number of corpus rows that, when RUN compiled,
// re-enter the interpreter through an unattributed seam.
//
// First measured 2026-08-28 at 184 of 7180 rows that run compiled (959
// entries). Now 100, after nine changes. The seam spread — the
// shape of the debt, not a second ceiling — and what each moved:
//
//	                         first    (a)    (b)    (c)    (d)    (e)    (f)    (g)    (h)    (i)
//	Engine.Run                 501    477    453    443    433    439    425    400    391    383
//	CallBoru                   275    251    251    251    251    251    238    238    238    238
//	vm:island                   66     66     48     48     39     39     39     39     39     39
//	runPooledSub                37     37     35     35     35     37     36     11     11     11
//	RunResolved                 31     31     31     31     31     31     31     31     31     31
//	vm:island-resolved          21     21     21     11     10     10     10     10     10     10
//	InvokeCallback:callboru     28      4      4      4      4      4      6      6      6      6
//	                          ----   ----   ----   ----   ----   ----   ----   ----   ----   ----
//	rows                       184    163    151    141    131    131    130    114    107    100
//
// (a) Foreign detached units became hostable mid-run
// (eng/go/vm_foreign_unit.go). The InvokeCallback column is that one: those 24
// entries were predicate bodies that HAD compiled to units and ran on the
// interpreter anyway, because the mid-run nested path declined every ref whose
// Program was not the running one — which a detached ref never is.
//
// (b) The Apply kernel (eng/go/vm_dyn_apply.go + compiler stampFnConst): a
// fn-value CONST carries its compiled unit, and a dynamic apply ENTERS that
// unit as a frame instead of islanding the callee. The `vm:island` column is
// that one, and it is the column the OpFallback ceiling can never see — those
// programs disassemble with `fallbacks=0` and the island lives inside the
// dynamic-apply opcode.
//
// (c) The Apply kernel reached CALL_DYN_FRAME's replay window in its simplest
// shape — an empty resolved prefix and a token region that is a fn followed by
// plain data. That is the `vm:island-resolved` column. A non-empty prefix keeps
// the island by contract: the fn stack-collects from the prefix as well as
// forward-collecting the region, so the frame would bind a different arg set
// than the interpreter assembles.
//
// (d) stampFnConst descends into list and map CONSTS, so a fn read out of a
// container carries its unit too. Measured before it did: of the island rows
// the top-level stamp left, roughly four in five were exactly that shape —
// `def m {f: (fn …)}  m.f 5`, `def ops {f: inc/v}  ops.f 5`, a class field
// method.
//
// (e) Emit-state isolation for the stamp — the one column that goes UP, and it
// should be read as the price of a correctness fix rather than a regression.
// stampFnConst now declines a stamp that would register a dynamic-scope name in
// the enclosing program, so a handful of bodies that used to carry a unit take
// the apply island again: Engine.Run +6, runPooledSub +2, no row moved. The
// alternative was keeping a stamp that MASKED a miscompile (see
// design/FULL-COMPILATION.0.md, "Container descent, and the mask it nearly
// shipped"). A seam count is not a score.
//
// (f) The seven native-callback sites that used CallBoruFn — `filter`'s
// Function form, the map-lambda each/fold bodies, core walk / StructUtil.walk,
// IO.mount's fileops handlers, boru:parse's matcher and action callbacks, and
// the fn-util words — now call InvokeCallbackFn, so a stamped body runs on the
// VM instead of never being offered to it. CallBoruFn is deleted: it existed to
// keep those bodies OFF the VM, which is the fence full compilation removes.
//
// (g) The Apply kernel reached LENSES (core/go/reach_unit.go). A Reach is a
// callable value exactly as a fn value is, and every consumer — `apply`, the
// lens forms of each/filter/sortby, getpath/setpath — funnels through one
// primitive, ApplyReach, which ran the lowered `[recv dot key …]` chain on a
// pooled sub-engine per application. It now compiles that chain once, caching
// the unit on the Reach payload, and runs it on the VM. This is the column the
// per-file attribution picked out: 16 rows, one mechanism, found by looking at
// the table rather than by guessing.
//
// Every drop carried Engine.Run with it, because an island is a nested Run.
//
// WHAT THE CallBoru COLUMN ACTUALLY IS, measured by probe at (f) — because it
// had not moved through four fixes and read as the largest block of debt:
//
//	Test.property's generator/property calls   224
//	core_helpers foreign-registry fn dispatch    8
//	InvokeCallback's own interpreter fallback    6
//
// So it is not broad interpretation debt. It is two or three `module-test.tsv`
// rows amplified by an iteration count — Test.property runs its bodies ~100
// times per row, and the seam counts INVOCATIONS while the ceiling counts ROWS.
// Those bodies are raw QUOTATIONS, not fn values: the module already routes a
// compiled sig through InvokeCallback and only falls back when the property was
// written as `[body]` tokens, which carry no unit to offer. Compiling them is
// Stage 7 (runtime compilation everywhere), not a seam flip, and it is worth
// perhaps three rows. Read the seam counts as a shape, never as a priority
// order: a big number here can be one row in a loop.
//
// WHERE THE ROWS ARE, by file AND the seams those rows touch (the per-file
// lines the run logs; ROW counts, so they add up against the ceiling). The
// table below is the state at the current ceiling — every cluster the columns
// after it took has left it. Rewrite it, do not append to it:
//
//	path-modifier.tsv        13 rows   Engine.Run 13, vm:island 13
//	bytecode-migrated.tsv     7 rows   RunResolved 6, vm:island 5, CallBoru 2, …
//	module-test.tsv           5 rows   Engine.Run 5, CallBoru 3   (2 assertion rows compiled at (j))
//
// Engine.Run appears everywhere because every other seam runs a nested engine;
// a row with ONLY Engine.Run is the interesting case — nothing islanded, the
// whole body simply interpreted. Two of the three module clusters that shape
// described are gone (columns (i)-(l) took boru:log, boru:test's assertions and
// the whole of boru:parse); what is left of it is module-test.tsv's five, which
// are Stage 6/7 work (module bodies compiling), not a seam flip.
//
// reach.tsv led this table at 14 rows, every one through runPooledSub, and it
// is gone: that is column (g).
//
// path-modifier.tsv is the cluster column (h) went after, and it is the honest
// counter-example to "one mechanism, one fix". All thirteen of its rows still
// island, because the dispatch-modifier wrapper is built at RUN time out of a
// fn the check pass only sees as a dynamic carrier (a dot-access read), so
// there is nothing to compile at compile time. The BY-NAME forms already
// compile clean — `usurp sub 10 3` has no seams at all.
//
// This note used to read "the blocker is the dot-access hiding the callee, not
// the modifier", and column (k) is the correction: the dot-access alone was
// never the blocker. `m.f 5` over a boru fn value compiles now, thirteen other
// rows with it, and every path-modifier row stayed put. What those rows have in
// common is not the dot — it is the CALLEE. `def m {a:add/v}` stores a NATIVE,
// which carries no unit for the Apply kernel to enter, and the modifier
// wrappers on top of it return TOKENS for the engine to re-step (see the
// rejected increment above). So the remaining cluster is the native-callee half
// of the question, and its cost was already measured: one row, bought with a
// worse error message.
//
// (h) boru:debug's body runs are ATTRIBUTED, not compiled — and the distinction
// is the point, because it is the one place "compile everything" is the wrong
// goal. Every one of those sites installs a TRACE HOOK and the word's ANSWER is
// what the hook saw: Debug.steps returns the engine-step count, the profiler
// returns a tally of observed dispatches, the stepper pauses at each step and
// breakpoint. Compiling those bodies would not speed them up — it would empty
// the tally, change the count, and leave the debugger nothing to step through.
//
// So this is interpretation the end state must PERMIT. It gets the same C4
// attribution `module-load` already has ("debug-observe"), which is what this
// census means by attributed: interpretation that is specified, named, and
// therefore not debt. It is NOT an escape hatch — the test is whether COMPILING
// the body would change the word's answer. For a step counter it plainly does;
// for `filter`'s callback it plainly does not, which is why that one compiled.
//
// A number this census cannot reach by compiling alone is worth knowing early:
// some of the remaining rows are of this kind, and the honest end state is
// "every entry attributed", not "no entries".
//
// (i) `Log.with-span NAME [body]` compiles its body. It DECLARES the body slot
// callable (a CallableSpec with 0 inputs and BodyOutResidual, the shape
// Test.describe already uses) and the handler drives the compiled closure
// through InvokeBody, falling back to the token run when the body did not
// compile. All 7 module-log rows, and the file left the cluster table.
//
// It is the same question (h) answers the other way: nothing about a span
// depends on WHICH engine runs its body, so compiling changes no answer. The
// two increments together are the rule — attribute where the engine IS the
// answer, compile where it is not.
//
// AN INCREMENT THAT WAS BUILT, MEASURED AND REJECTED — recorded because the
// measurement is worth more than the code was. Dispatching a callee whose
// matched sig carries its OWN Go handler (a native fn value read out of a
// container, `def m {a:add/v}  m.a 1 2`) directly instead of islanding it:
//
//  1. It first looked like TWELVE rows, 114 -> 102, and was FLATTERING ITSELF.
//     It also took the dispatch-modifier wrappers, and those handlers do not
//     COMPUTE a result — usurp / stack-args / forward-args / force-arity return
//     TOKENS for the engine to re-step (execMatch re-steps a handler's result by
//     default; only Park() opts out). Pushed onto the operand stack as data they
//     tripped screenResults, and ELEVEN corpus rows went from
//     compiled-with-an-island to not compiled at all. A row that stops compiling
//     leaves this census's DENOMINATOR, so the count fell for the worst possible
//     reason. An A/B walk comparing wasCompiled per row found them.
//
//  2. Handing the rewrite's tokens to the island instead fixed that, with zero
//     regressions — and the honest gain was ONE row.
//
//  3. That one row cost error fidelity. A handler that raises through the direct
//     path loses its source position ("source position unknown" where the
//     interpreter points a caret), because the dyn-apply opcodes are lowered
//     with SrcPos.Row == 0 and stampAt has nothing to stamp. The island never
//     needed it: its sub-engine re-ran the tokens and carried their positions.
//     Closing that means giving those opcodes positions in the lowerer, which
//     ripples through error rendering corpus-wide, where content parity is
//     gated.
//
// One row, bought with a worse error message, is not the trade this mission
// makes — the same judgement that un-masked the flex-map miscompile at (e).
//
// (j) `Assert.throws [body]` compiles its body — the same CallableSpec + drive-
// through-InvokeBody shape as (i), at BodyPos 0 with the residual DISCARDED.
// Every corpus row that runs an assertion body left the census: five of them,
// two in module-test.tsv and three in the edge-errors files, which is three
// more than the cluster table showed. The table only lists files at 5+ rows, so
// a word used two-or-three-at-a-time across several files is INVISIBLE in it.
// Worth remembering when reading the table as a work queue: it ranks
// concentrations, not mechanisms, and a mechanism can be spread thin.
//
// It is worth naming why a word whose whole purpose is to OBSERVE AN ERROR is a
// compile and not an attribution, since (h) attributes on exactly that kind of
// reasoning. The distinction is what the word observes. boru:debug observes the
// ENGINE — steps taken, dispatches seen — so the engine is the answer and
// compiling erases it. Assert.throws observes the PROGRAM: whether the body
// raised. A raise is a raise on either engine (the VM traps and returns the same
// boru error the sub-engine run would have), so the answer is engine-independent
// and the body compiles.
//
// (k) The Apply kernel now takes the SHAPED-METHOD apply (OpCallDynMethod) —
// the `m.f 5` shape where a dot-access reads a fn value out of a container and
// applies it. That is the cluster (h) went after and could not close, and this
// is the part of it that was never about the modifier: fourteen rows, all of
// fn-value.tsv and valof.tsv plus one of bytecode-migrated's, 95 -> 81.
//
// callDynMethod had every other apply path — a compiled closure, a
// trivial-delegation native — and fell through to the island for a plain boru
// fn value, even though its unit was compiled and sitting in the same program.
// It now asks dynApplyEnter first, exactly as callDynamic and callDynFrame do.
//
// WHAT MADE IT UNSOUND UNTIL NOW, and this is the increment's real content. A
// stamped fn-value unit compiles COUNT-AGNOSTIC — compileStoredFnUnit goes
// through compileClosureBody with bodyOut 0, whose `declared = nil` leaves
// CompiledFn.Returns empty — so its RET enforces nothing. Entering such a unit
// therefore SKIPPED the fn's declared return contract, which the island path
// applies (the interpreter's __RC runs inside the CallBoru the nested Run
// reaches). Measured on the EXISTING kernel, before this increment touched
// anything:
//
//	def bad fn [[n:Any][Integer][n]] end
//	def mk  fn [[][Function][bad/v]] end
//	((mk) 'str')
//	  interpreted  [boru/type_error] bad: return value 1: expected Integer, …
//	  compiled     'str'
//
// A silent wrong answer on main, not a refusal — found by asking what the
// entry would skip, not by a failing test. The fix is that the entry CARRIES
// the applied value's contract (dynEnter.retFn, applyRetContract) and the
// frame's RET applies it; lang/spec/fn-value.tsv §12 pins both halves. The
// census rows are downstream of that: they are only takeable BECAUSE the entry
// became contract-faithful.
//
// The shape claim's result half is discharged statically against that carried
// contract (dynMethodClaimOK) — a frame push has no results to count, and the
// RET now guarantees the count the sig declares.
//
// (l) boru:parselang's runtime `parse <fn>` dispatch stops stepping its
// expansion tail in a sub-engine. That tail — the parser value followed by
// `source opts end` — was a whole interpreter run inside a compiled program,
// one per `parse <parser> …` row: all eleven of module-parse.tsv and four more
// elsewhere. 81 -> 66.
//
// It replaces one lane with two, and the question that picks between them is
// not "is this a fn value" but "WHAT KIND of fn value is this":
//
//   - a matched overload carrying a GO handler dispatches directly, the way the
//     interpreter's execFnDefLiteral wrapper branch does (this mirrors the VM's
//     own tryNativeFnApply arm for arm);
//   - a real boru body goes through the callback seam, where it is offered to
//     its compiled unit before CallBoru.
//
// Getting that question wrong is what an earlier attempt did, and the two ways
// it goes wrong are worth keeping, because they look nothing alike:
//
//  1. `Parse.parser` mints a TRIVIAL-DELEGATION wrapper — unnamed params, a body
//     of one Word naming the inner Go native. Through the callback seam,
//     CallBoru splices that body and re-dispatches its word over a frame whose
//     unnamed args sit stack-order, so the inner native's sig positions come out
//     REVERSED and nothing matches:
//     `signature_error: cannot call parse-parser-1 — no signature matches`.
//     Loud, and it had been recorded as an ARG-ORDER mistake. It is not one:
//     [source, opts] is the sig order on every lane, and permuting it would
//     have made this row pass and every other one wrong.
//
//  2. `def myp (Parse.parser g)` REBINDS that wrapper, and InstallDef's
//     module-wrapper branch binds the inner native's Signatures verbatim under
//     the new name — so the value carries GO sigs outright, and the callback
//     seam ran them as though they had a body. SILENT:
//
//     def acc (flex []) … Parse.matcher g lex 5 ([s:String] => [acc push {v:s} …])
//     interpreted  [{v:'hello'}]
//     compiled     []                  the matcher never ran
//
//     Caught by lang/go's TestCompileParseOverEnclosingParserDef, which is a
//     reminder that the census is not the gate — it cannot see a wrong answer,
//     only an interpreter entry, and this change LOWERED it while breaking a
//     program.
//
// The lesson generalises past this word. A fn VALUE is not one kind of thing,
// and a seam that "runs a fn value" has to ask which kind before it picks a
// mechanism. Two plausible-looking argument orders is the symptom of having
// skipped that question — the order was never the variable.
//
// (m) The whole-frame replay window (OpCallDynFrame) admits the Apply kernel
// over a NON-EMPTY resolved prefix, when the callee is all-forward. Two rows,
// 66 -> 64 — small, and worth recording for where the boundary sits rather than
// for the count.
//
// The prefix is the frame-bottom unnamed-param re-push; the token region is
// what the interpreter's pointer would step. The window refused any prefix at
// all because a BARRIER'd callee stack-collects from it as well as
// forward-collecting the tokens, so a frame push would bind a different arg set.
// That is true of a barrier'd callee and only of one: an all-forward callee
// whose params the token args exactly fill cannot reach the prefix, so the
// prefix survives underneath and the unit's result lands on top of it — which
// is precisely the residual the island returns.
//
// What did NOT move says more than what did. `def looper fn [[Function]
// [Integer] [def acc 0 for 5 [(args.0 1) …] acc]]` still islands, because the
// apply sits inside a LOOP body and the window is not the frame's; and `def
// keep fn [[Function] [Function] [args.0]]` still islands because it does not
// apply its argument at all. Neither is a barrier question, so neither is
// reachable from here.
//
// (n) `Debug.trace` and `IO.trace` are ATTRIBUTED, on column (h)'s rule and by
// the same reading of it. Both route through core.RunTrace, whose entire job is
// to print what the interpreter did step by step; compiling the body would not
// speed it up, it would leave nothing to print. Two rows, 64 -> 62, and the
// attribution goes on RunTrace itself so the two words cannot drift apart.
//
// (h) attributed boru:debug's step counter, profiler and stepper and MISSED
// these two, which is worth noting as a property of that kind of fix: an
// attribution applied word-by-word leaves siblings behind. Putting it on the
// shared entry point is what makes it exhaustive.
//
// (o) `do {key:[body]}` stops starting an engine to step values it has already
// computed. Six bytecode-migrated rows, 62 -> 56, and the interesting part is
// that NOTHING needed compiling — the compiler was already doing the work.
//
// The disassembly settles it. `def f fn [[a:Integer] [Map] [ do {n:[a add 1]} ]]`
// lowers to
//
//	PUSH_LOCAL l0 / PUSH_CONST 1 / CALL_NATIVE add / MAKE_LIST / MAKE_MAP / CALL_NATIVE do
//
// so the map reaching DoEvalMapValue is `{n:[6]}` — the addition happened at
// MAKE_LIST. The handler then ran `[6]` in a sub-engine to discover that
// stepping the literal 6 yields 6. That is an interpreter entry inside a
// compiled program buying precisely nothing, and doEvalDataList now returns a
// STEPLESS list as its own residual instead.
//
// The interpreter's lane is untouched, which is what makes this safe rather
// than clever: the same source arrives there as [Word(a) Word(add) 1], which is
// not stepless, so it still runs. The two engines' answers are unchanged; only
// the compiled lane's redundant engine is gone.
//
// Worth generalising from: a census row is not automatically a COMPILER
// problem. The cluster table said "do {map} computed-map bodies" and read like
// Stage 6 work on code bodies. Reading the actual disassembly said the bodies
// were already gone, and the fix was six lines in a handler. Read the
// bytecode before believing a cluster's name.
//
// (p) `ArrayUtil.foldaxis` DECLARES its body callable. Two rows, 56 -> 54, and
// no handler change at all: foldaxisHandler already reduces each lane through
// the same doFold that `fold` uses, and doFold already drives the body through
// InvokeBody. The word was seam-ready and simply never said so.
//
// The spec is fold's, with the inputs taken one rank deeper — a lane is a row
// (or a transposed column), so each step sees two elements of an INNER list,
// never a row. `rank2ElemType` reads the first row for that, exact against the
// rectangular shape the handler enforces at run time, and answers Any for a
// data argument with no element to take a type from. That arm is the new
// `foldaxis 0 [add] []` corpus row, which is also the shape the handler
// short-circuits to `[]`.
//
// Its sibling `eachrank` is NOT here, and the reason is the one that decides
// whether a word is a declaration away or a change away: eachrankHandler slices
// its body into raw TOKENS and walks them itself (eachrankWalk), so it has no
// InvokeBody seam to declare over. Handler first, declaration second — never
// the reverse.
//
// TWO LESSONS. A census whose denominator is "rows that ran compiled" REWARDS a
// change that stops rows compiling: read it against the corpus gate and the
// compile-or-fallback walk, never alone. And the remaining path-modifier rows
// do not need this seam at all — see the cluster note below.
//
// Lower it whenever it falls. Raising it means a change put interpretation
// back into compiled programs, which is the one thing the compilation mission
// rules out — so a rise wants a design note, not a bigger number.
const interpEntryRowCeiling = 54

func TestInterpEntryCensus(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatal(err)
	}
	seams := map[string]int{}
	perFile := map[string]int{}
	// fileSeamRows counts ROWS, not entries: file → seam → how many of that
	// file's dirty rows touch that seam at all. Entry counts mislead (see the
	// CallBoru note above — 224 of them were one word in a loop), and the
	// ceiling is a row ratchet, so the attribution that picks the next fix has
	// to be in the same unit the ceiling is.
	fileSeamRows := map[string]map[string]int{}
	rows, dirty, ran := 0, 0, 0

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
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			rows++
			n, seen, ok := runWithEntryHook(t, strings.TrimSpace(parts[0]))
			if !ok {
				continue // refused or check-error: the interpreter owns it by design
			}
			ran++
			if n == 0 {
				continue
			}
			dirty++
			perFile[e.Name()]++
			if fileSeamRows[e.Name()] == nil {
				fileSeamRows[e.Name()] = map[string]int{}
			}
			for s, c := range seen {
				seams[s] += c
				fileSeamRows[e.Name()][s]++ // once per ROW, whatever c is
			}
		}
		f.Close()
	}

	t.Logf("interp-entry census: %d rows, %d ran compiled, %d with UNATTRIBUTED interpreter entries (ceiling %d)",
		rows, ran, dirty, interpEntryRowCeiling)
	for _, s := range sortedKeys(seams) {
		t.Logf("   seam %-28s %d", s, seams[s])
	}
	for _, f := range sortedKeys(perFile) {
		if perFile[f] >= 5 {
			t.Logf("   file %-34s %d rows   via %s", f, perFile[f], seamBreakdown(fileSeamRows[f]))
		}
	}

	if dirty > interpEntryRowCeiling {
		t.Errorf("interp-entry census %d exceeds ceiling %d — a change put interpretation BACK "+
			"into compiled programs. The OpFallback island ceiling cannot see this (it counts "+
			"disassembly spans, not a CallBoru inside a handler), so raising the number here is "+
			"not a bookkeeping fix: it is the invariant T2 forbids", dirty, interpEntryRowCeiling)
	}
	if dirty < interpEntryRowCeiling {
		t.Errorf("interp-entry census %d is BELOW the ceiling %d — the ratchet tightened, "+
			"lower interpEntryRowCeiling to %d", dirty, interpEntryRowCeiling, dirty)
	}
}

// runWithEntryHook runs one row compiled with the interpreter-entry hook armed
// and returns the unattributed entry count. ok is false when the row did not
// run compiled at all — a refusal or a static check error, where the
// interpreter owning the program is the designed behaviour and not an island.
func runWithEntryHook(t *testing.T, src string) (int, map[string]int, bool) {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	a.SetClock(specClock)
	var mu sync.Mutex
	seen := map[string]int{}
	total := 0
	disarm := a.ArmInterpEntryHook(func(ev lang.InterpEntry) {
		if ev.CheckMode || ev.Attribution != "" {
			return
		}
		mu.Lock()
		seen[ev.Seam]++
		total++
		mu.Unlock()
	})
	_, compiled, _ := a.RunCompiled(src)
	disarm()
	mu.Lock()
	defer mu.Unlock()
	return total, seen, compiled
}

// seamBreakdown renders one file's seam→rows map as a stable, compact line —
// "Engine.Run 14, CallBoru 9" — highest first, ties broken by name so a diff of
// two census runs is readable. Empty maps cannot reach here (a file only lands
// in perFile once a row recorded at least one seam).
func seamBreakdown(bySeam map[string]int) string {
	names := sortedKeys(bySeam)
	sort.SliceStable(names, func(i, j int) bool { return bySeam[names[i]] > bySeam[names[j]] })
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s %d", n, bySeam[n]))
	}
	return strings.Join(parts, ", ")
}
