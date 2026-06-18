// Compiled-coverage ratchet (design plan: "Make the AQL bytecode VM fully
// independent of the interpreter at runtime", phase P0). The runtime-
// independence work drives every supported program to compile to bytecode
// that runs entirely in the VM, so that BOTH interpreter dependencies — the
// OpFallback island and the whole-program fallback — can be deleted.
//
// This test is the objective measure of that goal. It runs EVERY spec value
// row through CompileCheck and counts the rows that REFUSE (no Program, no
// check error), bucketed by a normalised refusal reason. The total refusal
// count is a downward ratchet: it must never rise above the recorded ceiling,
// and each phase (P2..P6) lowers the ceiling as a refusal category is
// eliminated. P7 (delete the fallback) is gated on this reaching ZERO.
//
// Rows that error during the check pass itself (parse errors, type-error
// rows) are NOT refusals — the program is statically invalid in both engines
// — and are reported separately, not counted against the ceiling.
package langspec

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// refusalCeiling is the maximum number of spec value rows allowed to refuse
// compilation (CompileCheck returns a nil Program with no check error AND the
// row is not a statically-invalid check-diagnostics row). The runtime-
// independence phases lower it monotonically; it must reach 0 before the
// interpreter fallback can be deleted (plan P7). Never raise it.
const refusalCeiling = 71 // trailing fn-value auto-apply: a fn value that TRAILS its argument (`5 m.f` — the fn `m.f` produces applies to the 5 beneath it; `[10 20 30] r.one-of`) refused as "residual shape beyond Stage 1 (call result above a literal)". The recorder now rotates the trailing fn to the front of the residual so the reconciliation lays out [fn, arg] exactly like the LEADING boundary, and a new OpCallDynamicTrailing applies it — identical to OpCallDynamic on the callable path, but rotating the fn back ON TOP of its arg when the value is not callable so the residual matches the interpreter's trailing order. Bounded to one arg (a 2-arg mixed form like `3 m.f 2` would order the island's forward args opposite to the interpreter's top-down stack collection, so it stays refused). Cleared the 1-arg trailing fn-value rows 73 -> 71. N-event reverse operand layout: a call with N>=3 COMPUTED args evaluates them left-to-right so sig position 0 lands DEEPEST on the stack, but the call wants it on top — a full reversal the VM's SWAP-only lowerer could not do (it refused `TimeUtil.is-between (a) (b) (c)` as "operand shape beyond Stage 1"). New OpReverse stack-scheduling primitive reverses the top N in one op; layoutOperands recognises the exact-reverse all-events layout and emits it. Cleared the operand-shape scheduling row 74 -> 73. suppressed-runtime-error rows compile an OpTrap error path: a word that is lenient in check mode but raises at run time — an orphan `gen [T]` (gen_without_constructor), an `unpack` of a missing key (unpack_error) — used to set CheckState.SuppressedRuntimeError, which refused the WHOLE program. The suppression site now records a TERMINAL trap (EmitState.RecordTrap) carrying the byte-identical error taxonomy; Finalize ends the program at the trap (events before it run for their side effects, exactly as the interpreter does), and the new OpTrap raises the matching error in the VM. A nested trap (inside a branch/loop/fn fragment) is not yet modelled, so it keeps the blanket refusal and falls back. Cleared the top-level gen/unpack error rows 79 -> 74. user-fn return-count mismatch compiles the error path: a DECLARED user fn whose body leaves a different value count than its declared returns (`def r2 fn [[n][Integer][n n]] r2 1`, surplus user-typed returns) used to refuse as "body value count differs" and fall back. The VM's RET now enforces EXACTLY len(returns) (it tracked only the underflow before) measured from the frame's entry stack base, raising the byte-identical type_error the interpreter raises at __RC — so the body compiles and errors in place. A CLOSURE body (each/scan/…$body) keeps refusing the same shape: its mismatch is the higher-order word's OWN error (each_error "body produced no result"), so it must island for the interpreter to raise the matching taxonomy (fnUnitRec.closure gates this; the RET exact-count is scoped to user-fn frames, leaving the re-entrant closure RET unchanged) 82 -> 79. fn-storing words bake the fn as a const: a blanket refusal rejected ANY sig with a [Function]/[FnDef] arg type ("function-valued operand"), but minilang/parselang `register` STORE the fn in a module rule table for later INTERPRETER-side invocation (never the VM tape) — like an introspection read. Exempt fnStoreWords (minilang-register/-compiled, parselang-register) from both function-valued refusal loops; a pure fn literal then bakes as an inert const operand (a capturing / sub-registry fn still refuses at isInertConst). Cleared the register cluster 94 -> 82. aql:test describe-body closures: `Test.describe "g" [body]` (and nested describes) is the same 0-output side-effect body as a case; wired test-describe as a callable body word, the handler pushes the group name on the path around an InvokeBody run, and nested `Test.test`/`Test.describe` each compile to their own closure 95 -> 94. aql:test case-body closures: `Test.test "n" [body]` is a SIDE-EFFECT code body (assertions raise on failure, nets 0 values) that the closure path refused because callableWords/RecordClosureCall/compileClosureBody all assumed exactly 1 body output. Generalised the closure path to 0-or-1 outputs (callableWord.bodyOut; declared=nil for a side-effect body; RecordClosureCall nout=len(outs)), registered `test-test` as a 0-out callable body word, and the handler runs a compiled-closure body via InvokeBody (catching the assertion error to record pass/fail) — clearing the Test.test/it cluster 107 -> 95. OpMakeList of make-instance lists: RecordMakeList excluded a `make`-produced element ("keep instance lists on the const-bake path"), so an instance list `def xs [(make Box …) (make Box …)]` refused (isInertConst rejects the mutable members) and a downstream each/fold/get over it had no resolvable operand. An OpMakeList of make EVENTS rebuilds fresh instances per run (sound, like OpMakeMap), and the typed-def reparent linkage the old comment feared is no longer broken (carrier-identity/value-def work since); both typed and untyped instance lists now assemble natively 109 -> 107. OpMakeMap (computed make-construction body): a `make Outer {i:(make Inner …)}` whose field VALUE is a computed mutable instance const-folded to a concrete instance that isInertConst correctly rejected (baking would alias — make stores provided fields verbatim), so it refused "not statically materialisable at make". New OpMakeMap opcode assembles the map at RUNTIME from value operands (keys ride in Program.MakeMaps), so the inner make is a fresh per-run event; autoEvalMap skips the fold for a make data-map's shared-mutable values (a class/refine SCHEMA default still folds + bakes as a FreshenDefault template, unchanged) and records the assembly. Cleared nested/computed-make rows 112 -> 109. residual ordering via local promotion: the residual reconciliation seats event results in stack order with literals as a trailing tail, so it refused a residual with an event ABOVE a literal (`1 2 word [add] 10` → [literal, event], "call result above a literal"). When such a residual holds no fn-value / dynamic (the auto-apply boundary's territory, which must not be reordered), force every residual event to a frame local so the whole residual re-pushes in exact order 113 -> 112. N-event already-in-layout lowering: layoutOperands refused 3+ prior-result (event) operands outright (only 0/1/2 were handled), so a computed list `[gensym gensym gensym]` / `[(inc1 4) (inc1 4) (inc1 9)]` whose N elements are all adjacent event results on top refused at "operand shape at […] beyond Stage 1". Generalised to accept any N events already sitting in sig order on top (position-verified, emits nothing) 115 -> 113. fn-body return-count: a 0-output statement guard (`if cond [raise]`) registers a phantom None in the analyzer's residual but produces NO runtime value — now excluded from the fn return-count check and from outOps (mirroring the top-level residual reconciliation), so a declared fn whose body is a guard + a real result (`def f fn [[n][Integer][if (n eq 0) [raise…] def q (n add 1) q]]`) compiles instead of refusing on a phantom-inflated count; a GENUINE count mismatch still refuses 119 -> 115. fn-value field calls: a fn-value held in a baked map/object field (`{b:f/r}`) now bakes as an inert const member AND applies faithfully — get poly-folds the member, CALL_DYNAMIC applies it, and callPoly/callDynamic fast-path ONLY a trivial-DELEGATION method (isDelegationFnDef), routing a USER fn through the island that runs its body; cleared the get-opaque fn-value-call cluster (`m.b 2`, `ops.f 5`, `typeof m.a/r`) 129 -> 119. branch-fragment value-def locals: an ENCLOSING computed value read INSIDE a branch arm / clause-guard fragment is promoted to a frame local (STORE_LOCAL once, PUSH_LOCAL per cross-floor read) rather than refusing on the closed-fragment scopeFloor rule — which lets a computed-scrutinee `case (1 add 1) […]` desugar (its clauses re-test the scrutinee in every fragment) 135 -> 129. value-def-locals + class mutable-default bake: a `def name (expr)` binding promotes to a frame local (re-pushable in any order, so `def a (make…) def b (make…) a.x … b.x` compiles), and a class body with a flex/Array/Store/Object default bakes as a const TEMPLATE that make's FreshenDefault copies per instance (per-instance isolation holds) -5; merged with PR #151's concat-overload tightening that requires a String operand (+1 arithmetic.tsv: the single Scalar→String coercion row became two Boolean/Float fall-back rows): 140 -> 135. factory-apply: a fn that RETURNS an anonymous capture-free closure compiles to OpPushClosure, and a [Function]-typed CARRIER leading the residual applies via a stack OpCallDynamic (`(mk2 5) 10` -> 11) 140 -> 139. StructUtil.jsonify/items/clone result types let six struct rows lower 222->216; filter result typed (List/Map subset) 224->222; P0 651 -> P3 642 -> wide poly 618 -> P4 CALL_DYNAMIC (fn-value apply) 616 -> P5 multi/0-result calls 598 -> make carrier-identity (impure constructor fresh-id) 580 -> value-def locals (OpStoreLocal) 568 -> dup carrier-identity (distinct ids for repeated outputs) 565 -> predicate-type operand provenance (DepScalar const-bake) 555 -> if value-else arm 545 -> case clause compilation (desugar to if-chain) 542 -> multi-return / 0-return / anonymous-lambda fns (N-result fn units) 538 -> apply of a fn value (ReturnsIdentity makes the check engine re-step it) 527 -> unnamed-fn map/list members const-bakeable (method fields: m.f via poly get + CALL_DYNAMIC) 521 -> 0-value-then 2-arg if statement guard (if cond [raise]) 519 -> concrete-args dynamic-output core builtins bake CALL_NATIVE (unify et al) 514 -> module inner natives (StructUtil.* etc, dot-access) bake CALL_NATIVE via sub-registry sig verification 459 -> 3-arg operand shape: sig-0 result over const operands lowers via push+swap chain (setpath et al) 453 -> strict-disjunct straddle lowers OpCallNativePoly (5 is (tnot (Integer gt 0))) 445 -> object/class atom-keyed set (quoted-operand exemption, mutation-safe) 425 -> computed-else if (SWAP cond up, DROP else on taken path) 421 -> variadic-else statement-if (mismatched-arm merge marked variadic) 417 -> fn-value introspection (typeof/tcmp/teq/tand/tor/tnot read a baked fn-value const) 407 -> filter lambda args (list {key,value} + map KeyVal closures compiled, driven via InvokeBody) 402 -> each/fold/scan map lambda args (KeyVal-shaped closure, ClosureInShape flag) 399 -> with-decimal block (0-input body closure run within the scoped decimal context) 394 -> args.N (params projected as the args list; get-N-of-args folds to PUSH_LOCAL N) 390 -> word/splice (compile-time macro inline: __SP marker preserved + expanded at the use site) 363 -> macroexpand (Lisp-style compile-time expansion baked as a code-as-data const; Word admitted as a const member) 359 -> make computed container defaults (class field defaults / data-map values const-fold via deterministic concrete eval; instances bake as schema members) 349 -> usurp (the wrapper re-dispatch runs in check mode; the carrier compiler compiles the reversed original call) 312 -> case no-default (even) clauses (nested-variadic branch lowering propagates 0-or-1 to the residual) 310 -> query DSL (aql:query): select/where/order/group/having/limit/offset/distinct/on/using bake their inert NoEvalArgs clause-lists (noEvalBodiesInert, sentinel bodies still refused); from/join/innerjoin/leftjoin/crossjoin bake their inert quoted table atoms (quoteOperandInertOK module-inner exemption) 283 -> reach lenses (receiverless `$.name`/`$.a.b`/`$!.x`/`$.1` inert lens bakes as a const via isInertReach; unblocks the lens VALUE, typeof, apply, rebind, getpath, setpath forms) 268 -> scalar carrier-keep (toCarrier keeps concrete inert scalars, not just integers, so a string/bool/temporal-bearing data list or map bakes as an inert const: `size people`, `each $.name people`, `sortby $.age people`, faithful class-default type bodies) + carrier-identity de-collision (a repeated identical computed call like `(context get 'n') add (context get 'n')` mints a fresh result id so both stack values stay distinct) 258 -> module-synthetic const-fold (a pure read over an import-bound module value — `MathUtil.$name`, `X.$module.name`, `convert Map/List Foo`, `typeof`/`is` of a Module — re-evaluates concretely off the emit path and bakes the real value; module instances kept concrete in toCarrier) 227 -> OpMakeList (a top-level computed list literal `[1 add 2]` / `[(1 add 2) (3 add 4)]` assembles its evaluated CORE-builtin elements with a new VM opcode; fn-body / higher-order / module-instance / type-pattern lists stay refused) 213 -> +11 merged main minilang/parselang corpus rows (PR #142; my compiler falls back faithfully on them — 0 divergences — native minilang compile is future bytecode work) 224 -> +1 patrun.tsv: a Patrun dispatch row refuses (patrun add/find route a dynamic Patrun value — Stage 3); the earlier `add` 3-arg-overload regression was fixed upstream (chained numeric `add` compiles again) 217 -> structural type-pattern operands bake as const members (a bare type node is admitted as a const MEMBER of `{a:Integer}` / `[Integer String]` / `[Resource Entity]`, so static is/typeof/size over them compiles) 196 -> generic schema bakes as a const (immutable, registry-held instantiation memo) so make Box {…} / is / typeof / schema-residual compile 185 -> typed-list-field generic schemas (`{items:[:T]}`) compile once the schema body check admits a Word naming a type parameter 181 -> a function-signature value (`fnsig`, generic schema body, `typeof`/`teq` over an instantiated fnsig) bakes as pure descriptor data 178 -> a surface type (immutable contract descriptor riding its canonical node) bakes, unblocking the surface residual/is/typeof/teq/tand/tor/tnot/unify cluster 159

// islandCeiling is the maximum number of compiled programs allowed to embed an
// interpreter island (OpFallback). Islands re-enter the interpreter sub-engine
// at run time, so this is the second downward ratchet toward run-time
// independence (plan): each phase that compiles an island shape natively lowers
// it, and it must reach 0 before the OpFallback machinery can be deleted (P7).
const islandCeiling = 7 // atom-keyed gets 102->36; fold-no-init + filter (dynamic-output) closures 36->29; P5 multi/0-result calls 29->26; case clause compilation (islanded cases now compile natively) 26->15; map iteration (each/fold/filter over a map) compiles a value-body closure via InvokeBody 15->9; nested-variadic branch lowering compiles no-default case islands 9->7; lower as more island shapes compile natively

// normaliseReason buckets a refusal reason into a stable category by
// stripping the row-specific tail (word names, counts), so the histogram is
// comparable across rows.
func normaliseReason(reason string) string {
	switch {
	case strings.HasPrefix(reason, "code-body word"):
		return "code-body word (NoEvalArgs)"
	case strings.HasPrefix(reason, "quoted-operand word"):
		return "quoted-operand word"
	case strings.HasPrefix(reason, "compile-time word"):
		return "compile-time word (RunInCheckMode)"
	case strings.HasPrefix(reason, "full-stack word"):
		return "full-stack word (depth/pick/roll)"
	case strings.HasPrefix(reason, "context-dependent word"):
		return "context-dependent word (args/__pa)"
	case strings.HasPrefix(reason, "user fn call"):
		return "user fn call (Stage 3)"
	case strings.HasPrefix(reason, "function-valued operand"):
		return "function-valued operand (Stage 3)"
	case strings.HasPrefix(reason, "function value reaches"):
		return "function value reaches word (Stage 3)"
	case strings.HasPrefix(reason, "anonymous function dispatch"):
		return "anonymous fn dispatch (Stage 3)"
	case strings.HasPrefix(reason, "dynamic input at"):
		return "dynamic input"
	case strings.HasPrefix(reason, "unannotated or opaque word"):
		return "dynamic/opaque output"
	case strings.HasPrefix(reason, "polymorphic dispatch"):
		return "polymorphic dispatch"
	case strings.Contains(reason, "surface-shape typed dispatch"),
		strings.Contains(reason, "unmatched dispatch recovered"):
		return "dispatch recovery (best guess)"
	case strings.Contains(reason, "returns ") && strings.Contains(reason, "values"):
		return "multi-result call"
	case strings.HasPrefix(reason, "dynamic value precedes residual"):
		return "fn-value-call boundary"
	case strings.Contains(reason, "operand of unknown provenance"),
		strings.Contains(reason, "of unknown provenance"):
		return "operand provenance"
	case strings.Contains(reason, "suppressed a runtime error"):
		return "suppressed runtime error"
	case strings.Contains(reason, "without exactly one declared return"),
		strings.Contains(reason, "body value count differs"):
		return "multi-return fn"
	case strings.HasPrefix(reason, "residual shape beyond Stage 1"),
		strings.Contains(reason, "residual value not statically materialisable"):
		return "residual lowering (Stage 1 limit)"
	case strings.HasPrefix(reason, "if:"):
		return "if-branch lowering"
	case strings.HasPrefix(reason, "stack discipline"):
		return "stack discipline (lowering)"
	case strings.HasPrefix(reason, "operand shape at"):
		return "operand shape (Stage 1 limit)"
	default:
		return "other: " + reason
	}
}

// rootCause maps a normalised refusal bucket to its underlying axis, the second
// dimension of the ratchet (review §6 meta-improvement). It tells a future
// session WHICH kind of investment clears a bucket:
//
//   - correct-error: the row is KNOWN to error; it should compile an error
//     program (OpTrap / a RET count check), never refuse. Must stay 0 — this is
//     the proof of the Trap/Raise work.
//   - soundness:     compiling would (or might) diverge from the interpreter —
//     dynamic/opaque values, the fn-value-call boundary, lost provenance. Needs
//     a soundness story (e.g. richer runtime dispatch), not just more lowering.
//   - scheduling:    the analysis is fine but the lowerer can't arrange the
//     stack — the stack-scheduling / DDCG territory (review §4.1).
//   - opcode:        a missing VM primitive (DUP/ROT/TUCK for deep stack words).
//   - coverage:      a word class / feature the compiler does not model yet
//     (code-body DSL words, quoted-operand meta words, user-fn dispatch).
func rootCause(bucket string) string {
	switch bucket {
	case "suppressed runtime error", "multi-return fn":
		return "correct-error"
	case "dynamic input", "dynamic/opaque output", "fn-value-call boundary",
		"dispatch recovery (best guess)", "operand provenance",
		"function value reaches word (Stage 3)", "context-dependent word (args/__pa)":
		return "soundness"
	case "residual lowering (Stage 1 limit)", "stack discipline (lowering)",
		"operand shape (Stage 1 limit)", "if-branch lowering", "multi-result call":
		return "scheduling"
	case "full-stack word (depth/pick/roll)":
		return "opcode"
	default:
		return "coverage"
	}
}

func TestCompiledCoverage(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var rows, compiled, checkErr, refused, islanded int
	buckets := map[string]int{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		path := filepath.Join(specDir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			input := strings.TrimSpace(parts[0])
			rows++

			a := newDifferentialInstance(t)
			prog, reason, _, cerr := a.CompileCheck(input)
			switch {
			case cerr != nil, reason == "check diagnostics":
				// Statically invalid in both engines: a parse/check Go error,
				// or error-severity check diagnostics (a type-error row).
				// CompileCheck returns a nil Program for these with the row
				// genuinely erroring in the interpreter too — not a refusal.
				checkErr++
			case prog != nil:
				compiled++
				// An islanded program still re-enters the interpreter at run
				// time (OpFallback → sub-engine). Driving this to zero — by
				// compiling each island shape natively — is the run-time-
				// independence goal; track it as a downward ratchet.
				if strings.Contains(prog.Disassemble(), "FALLBACK") {
					islanded++
				}
			default:
				refused++
				buckets[normaliseReason(reason)]++
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	// Histogram, most-frequent first.
	type kv struct {
		reason string
		n      int
	}
	hist := make([]kv, 0, len(buckets))
	for r, n := range buckets {
		hist = append(hist, kv{r, n})
	}
	sort.Slice(hist, func(i, j int) bool {
		if hist[i].n != hist[j].n {
			return hist[i].n > hist[j].n
		}
		return hist[i].reason < hist[j].reason
	})

	t.Logf("compiled coverage: %d rows — %d compiled (%d islanded), %d check-errors, %d refused",
		rows, compiled, islanded, checkErr, refused)
	for _, h := range hist {
		t.Logf("  refusal %4d  %s  [%s]", h.n, h.reason, rootCause(h.reason))
	}

	// Second axis: bucket the refusals by ROOT CAUSE so a future session can see
	// which kind of investment moves the number (soundness vs lowering vs a
	// missing opcode vs a known-error path). The correct-error axis is the proof
	// of the Trap/Raise work — those rows now compile an error program, so it
	// must stay 0.
	byCause := map[string]int{}
	for _, h := range hist {
		byCause[rootCause(h.reason)] += h.n
	}
	for _, cause := range []string{"correct-error", "soundness", "scheduling", "opcode", "coverage"} {
		t.Logf("  root-cause %4d  %s", byCause[cause], cause)
	}
	if byCause["correct-error"] != 0 {
		t.Errorf("correct-error refusals %d (want 0): a known-to-error row must compile an OpTrap / RET error path, not refuse", byCause["correct-error"])
	}

	if refused > refusalCeiling {
		t.Errorf("compile refusals %d exceed ceiling %d — coverage regressed", refused, refusalCeiling)
	}
	if islanded > islandCeiling {
		t.Errorf("islanded programs %d exceed ceiling %d — interpreter-island use regressed", islanded, islandCeiling)
	}
}
