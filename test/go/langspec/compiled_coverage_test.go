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
	"sort"
	"strings"
	"testing"
)

// refusalCeiling is the maximum number of spec value rows allowed to refuse
// compilation (CompileCheck returns a nil Program with no check error AND the
// row is not a statically-invalid check-diagnostics row). The runtime-
// independence phases lower it monotonically; it must reach 0 before the
// interpreter fallback can be deleted (plan P7). Never raise it.
const refusalCeiling = 28 // make with a computed construction body inside a fn body: `make (Box of [T]) {value:x}` (x a param) refused because RecordMakeMap was top-frame-only. But make's body is a CONSUMED arg (dataMap, match.Name=="make") it evaluates in the current scope — never a deferred residual — so the OpMakeMap re-assembles per call soundly; dropped the top-frame gate for the dataMap case (engine.go autoEvalMap + RecordMakeMap). Cleared 3 generics-fn.tsv rows (boxit/boxfirst/pairup) 31 -> 28. dispatch-modifier word forms: forward-args / stack-args / force-arity now bake like usurp — their re-dispatch wrappers (rebarrierFunction / ForceArityFunction, core_ref.go) gained RunInCheckMode, so the carrier compiler steps the re-dispatch and compiles the underlying call directly (`forward-args sub 10 3` lowers like `sub`), instead of refusing the opaque wrapper; cleared 4 path-modifier.tsv rows 35 -> 31. await: a code-body native with a dynamic (declared-Any) result now bakes a plain CALL_NATIVE — dynOutNativeOK admits an inert NoEvalArgs body (the same noEvalBodiesInert screen RecordCall uses) instead of a blanket NoEvalArgs exclusion, so the handler runs its parallels faithfully and the dynamic result bakes; cleared the module-time.tsv await row 36 -> 35. cross-fn break/continue: a break/continue in a FN body with no enclosing loop in its own unit now compiles — the recorder emits OpFlowBreak/OpFlowContinue (lower.go) instead of refusing, and the VM unwinds to the nearest open loop in an ANCESTOR frame (vmLoop's exitPC/nextPC/unit/iterBase, derived from the instruction stream), discarding the current iteration's partial like the interpreter's mark→move splice. Two enablers landed alongside: a 0-value `if cond [break] []` (both arms net 0) now records zeroOut instead of refusing "branches produce no value" (native_control.go if3 + eng RecordBranch), and AnalyseFnBody strips the zeroOut phantom None from a fn's CALL residual so a 0-return fn (`def b fn [[n][][if … [break] []]]`) nets 0 to a multi-value caller (`b i i`). Cleared the 2 recursion.tsv §9 cross-fn break/continue rows, 0 divergences 38 -> 36. empty-body user fn returning ZERO values (`def v fn [[x:Integer] [] []]`): the check-mode ReturnsFn returned a bogus single Any approximation and skipped recording, so `v 1 99` (→ [99]) and `v 1` (→ []) refused "user fn call (Stage 3)". When the body unit compiles, it now records a 0-output CALL_USER and returns zero carriers — matching runtime exactly (the call runs for effects, the next token is the residual). recordCallElided gained the 0-output user-fn case (unambiguous: a DECLINED unit still returns the 1-value Any approximation, so only the compiled call reaches with an empty residual). The two error consumers (`def r (f 1)` → def_error, `add 5 (f 1)` → no_value_error) now surface their diagnostic at check time and fall back, faithfully 40 -> 38. reach computed-segment + TimeUtil timeout/interval body: (a) isInertConstMember now admits a deferred ParenExpr (`reach 5 [a (add 1 2) c]`) riding inside a never-evaluated compound — like the Word/Reach member cases it is pure data the reach handler re-runs at apply time, so the key list bakes as a const + CALL_NATIVE; cleared the 2 reach computed-segment code-body rows. (b) The inner timeout/interval LIST sigs gained NoEvalArgs (alongside QuoteArgs): the execMatch auto-eval gate keys ONLY off NoEvalArgs, and the module wrapper's trivial delegation runs execMatch on the inner sig, so without it `TimeUtil.timeout 1000 [body]` sub-Ran the body eagerly (a latent runtime bug — `[print 9]` printed at construction) AND reached the recorder as the evaluated type `[Integer]`, unbakeable. With NoEvalArgs the body stays raw code, noEvalBodiesInert bakes it, and the QuoteArgs refusal (hasUncoveredQuoteArg) no longer double-refuses a position already covered by NoEvalArgs; cleared the 3 TimeUtil quoted-operand rows + 1 operand-provenance row 46 -> 40. dynamic input to a closure-dispatch word: RecordClosureCall pre-declined ANY dynamic non-body operand, but the loop below already declines via resolveOperand when one genuinely cannot resolve. A caught `do` error reaching `error [handler]` is a resolvable event whose closure input is a FIXED TError carrier (independent of the dynamic value — the handler runs faithfully over whatever it turns out to be), so the pre-decline was overly conservative. Dropping it (relying on resolveOperand) compiles `do [{x:1} !. y] error [get code]` island-free, differential-clean. The other 4 dynamic-input rows keep their own blockers (flex mutation, IO-context set receiver, struct module sub-registry getpath) 47 -> 46. value-def-locals in fn units: planValueDefLocals (the computed-result → frame-slot promotion) now runs for EVERY compiled unit, not just the top-level program. A computed scrutinee read across a desugared `case` if-chain (`get code case […]`) is a cross-fragment read (fragRef) that must seat as a frame local; previously CanSeatAcrossFragment bailed in any fn unit (`len(units) != 1`), so the case islanded and the enclosing `error` handler closure refused. Running the promotion per-unit (in StartFnCompile's finish, while the unit is live, with the unit's OUTPUT operands as the residual-equivalent extra refs) + dropping the guard clears the 4 `error [get code case [value-clauses]]` rows — completing the error cluster (all 6 `error [handler]` rows now compile; the remaining dynamic-output rows are await + path-modifier, correct refusals) 51 -> 47. catch frame (do/error closure bodies): a closure body that ALWAYS raises (`raise`, now CompileDiverges) is DIVERGENT — it never returns normally, so it compiles with NO RET and at run time the error propagates out of the VM and the catching word turns it into an Error value via InvokeBody (the bytecode side of structured exception handling; `do [raise …]` was an island). `error` was collapsed to ONE (List, Any) sig with a runtime IsError branch — a static TError-vs-TAny pick was unsound (the checker types `do [raise …]` as Any and baked the pass-through even when the runtime value is an Error) — so the handler body compiles as a closure (BodyPos 0, error pushed as its one input). Cleared `error [get message]` + the predicate-clause `error [case …]` 53 -> 51. The other 4 `error [get code case [value-clauses]]` rows refuse on a COMPUTED scrutinee (`get code`) the fn-unit value-def-locals promotion cannot seat (CanSeatAcrossFragment bails when len(units) != 1) — they fall back faithfully; clearing them needs value-def-locals extended to fn units (the branch-result-modeling refactor). dot-access reach in an inert code body: the property-test words Test.prop / Test.check-prop / Test.skip take TWO code bodies (`[r.int 0 100]` + `[0 gte]`) that are inert at the call (prop stores them in a PropertySpec map, skip discards them, check-prop CallAQLs them in its native handler), so noEvalBodiesInert should bake them as const operands (a plain CALL_NATIVE) — but a dot-access reach (`r.int`, an Eval=true receiver-bearing Reach) inside a body was not an inert const MEMBER, so the body list failed isInertConst and the word refused "code-body word X (Stage 2)". Unlike isInertReach (the STANDALONE detached lens — receiverless, Eval=false, never expanded at the pointer), a reach as a MEMBER of a never-evaluated compound is pure DATA (the VM pushes the baked compound verbatim and never expands a reach), so inertReachMember admits a receiver reach whose receiver/literal-key tokens are themselves inert (a computed paren segment still refuses). Cleared all 3 code-body rows + Rand.map-from (its schema map `{a:[Rand.int 0 10]}` holds the same reach), all island-free 57 -> 53. module-exported Table TYPE get fold: a Table type literal (`Test.TestSet`) carries a TableTypeInfo payload, so tryFoldModuleConst's ride switch (isInertConst | bare-type-node) rejected it and the get over the immutable module export refused "unknown provenance ... at get" — unlike a BARE type node (`Test.TestCase`, Data==nil) which already folds. TableTypeInfo is a thin wrapper over its row RecordType, so it joins the structural-type-body const family (Record/Options/Object/Disjunct) in isInertConst + typeBodyConstOK, const-safe whenever that record is (same fieldsOK interior check, carrier interior still refuses). The get then bakes the immutable type and the downstream make/is/istype compile natively, clearing both Test.TestSet rows 59 -> 57. returnsof static return type: TypeUtil.returnsof read a fn's declared return but reported its static output as the opaque TAny (dynamic), so the dispatch refused as a dynamic output — unlike arityof ([TInteger]) / typeof ([TType]) whose concrete output bakes a CALL_NATIVE. Added a ReturnsFn that computes the precise return type for a concrete fn (the same value the handler produces), making the output a concrete type node, so it bakes like arityof 60 -> 59. The other 16 dynamic rows are the hard soundness frontier: error-handler-over-dynamic-error (×7, the `get` handler body islands → islandCeiling risk), path-modifier re-dispatch fns (forward-args/stack-args/force-arity, ×4, meta), await (async), and dynamic-input rows needing poly-extension to non-core words (drop/set/getpath). value-arm if: `if cond v1 v2` with VALUE arms (not `[body]` code) refused "if: then-branch not captured" because the then was run as a body (nil fragment) while only the else handled a value. Made the then arm symmetric with the else (a value-then pushes its value in the then arm, like value-else), clearing the direct value-arm if AND the usurp-if shape (`usurp if` dispatches if with value arms) 63 -> 60. The 3 remaining if-branch rows are NOT this: 1 computed-else/variadic-statement-if (needs the variadic-merge refactor) + 2 cross-fn break/continue (break inside a fn breaking the CALLER's loop — a cross-unit soundness boundary, correctly refused). rand-list-of generator body: `Rand.list-of [body] n` runs a 0-input body n times — the same closure shape as `do`, so it now declares a CallableSpec and its handler runs the compiled closure via InvokeBody instead of refusing the NoEvalArgs code body (the body's RNG draws advance the same module generator, so compiled == interpreted) 64 -> 63. quoted-operand inert words: quote / codequote / raise / timeout / interval declare CompileQuoteInert, so the recorder bakes their inert quoted operand (a quoted symbol, or a code body held as data) + CALL_NATIVE instead of refusing — the get/getr/set exemption made declarable. Cleared 7 of the 10 quoted-operand rows; the 3 timeout/interval rows whose code BODY reaches the check as a carrier without recoverable inert provenance stay refused (a §4.3 / materialise gap, not the flag) 71 -> 64. trailing fn-value auto-apply (`5 m.f`, `[..] r.one-of`) compiles via OpCallDynamicTrailing 73 -> 71; OpReverse N-event reverse 74 -> 73; suppressed-runtime OpTrap 79 -> 74; exact-count OpRet 82 -> 79. The full decrement history (94 -> ... -> 159) is in design/checker-compiler-architecture-review.0.md §11. Lower this monotonically with a one-line rationale; never raise (the §11 entries are the template).

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
	c := gatherCensus(t)
	rows, compiled, checkErr, refused, islanded := c.rows, c.compiled, c.checkErr, c.refused, c.islanded
	buckets := c.refusalBuckets

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
