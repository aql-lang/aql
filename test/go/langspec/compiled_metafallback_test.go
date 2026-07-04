// The re-scoped P7 gate (design/aql-bytecode-completion.0.md §3).
//
// The literal P7 — every spec value row compiles to a fallback-free Program —
// is not reachable for ONE narrow reason: a bytecode compiler is an
// ahead-of-time function `compile : Program -> Instructions`, and a handful of
// words EXECUTE CODE THAT IS COMPUTED AT RUNTIME (`Vm.run` of a runtime string).
// For those there is no static instruction sequence — "compiling" them is
// parse+compile+run at runtime, which is exactly what the OpFallback island
// (the embedded interpreter) does. That, and only that, is irreducible.
//
// EVERYTHING ELSE that refuses today is REDUCIBLE — it is refused by a specific,
// nameable limitation of THIS compiler/VM, not by any law. So the partition is
// three tiers, not "meta vs compute":
//
//   - tier 1, interpreter-only (interpreterOnlyWords): executes runtime-computed
//     code. The island is the correct, permanent home. Capped, not ratcheted.
//   - tier 2, reducible-not-yet-compiled (reducibleWords): refused by a named
//     missing compiler/VM feature (macroexpand = compile-time expand + bake;
//     word = splice inline; args.N = frame-local read; flex = reference cells;
//     …). These are TODOs, ratcheted toward 0 like any other work.
//   - the compute frontier: cascades, lowering residuals, DSL bodies, error
//     rows. Ratcheted by computeRefusalCeiling.
//
// (An earlier version of this file called tier 2 "irreducible meta." That was
// wrong — it laundered unfinished compiler work as impossibility. `args.N`
// proved it concretely: once called "context-dependent, needs an args stack the
// VM frame doesn't keep," it now compiles to a plain PUSH_LOCAL N — the params
// ARE the frame locals. `with-decimal` was the same move. The remaining tier-2
// words are more of the same, just deeper.)
//
// When BOTH ratchets reach 0, only tier 1 falls back, and the unbounded
// whole-program fallback in RunCompiled can be narrowed to the tier-1 island
// spans (§3 step 4).
//
// This is the SOLE boundary gate. An earlier parallel gate (meta_fallback_test.go,
// TestMetaFallbackBoundary) curated a permanent-META allowlist and re-walked the
// corpus independently — both the "permanent meta" framing (repudiated here and
// in design §3) and the second walk (the shared census in compiled_census_test.go
// is the one walk) were stale, so it was retired. Its disposition is subsumed:
// total refusals are ratcheted by refusalCeiling, and the per-tier breakdown by
// the three ceilings below.
package langspec

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

type allowEntry struct {
	name    string
	pattern *regexp.Regexp
	why     string
}

// interpreterOnlyWords (tier 1) execute code that is constructed at RUNTIME, so
// there is no ahead-of-time instruction sequence to emit: "compiling" them is a
// runtime parse+compile+run, i.e. the interpreter. This is the one genuinely
// irreducible category and the legitimate permanent home of the OpFallback
// island. Kept deliberately tiny — a NEW entry here is a claim of irreducibility
// that must be justified.
var interpreterOnlyWords = []allowEntry{
	{"Vm.run", regexp.MustCompile(`\bVm\.run`),
		"executes runtime-constructed source in a sub-engine — there is no static program to compile; this IS the interpreter"},
}

// reducibleWords (tier 2) refuse today because of a specific, NAMED limitation
// of this compiler/VM — not because they cannot be compiled. The `why` records
// exactly what compiling each would take. They are ratcheted toward 0
// (reducibleCeiling); none is a permanent exclusion.
var reducibleWords = []allowEntry{
	{"usurp", regexp.MustCompile(`\busurp\b|/u[rs]?(\b|$)`),
		"REDUCIBLE (mostly COMPILED): usurp is a static arg permutation; its wrapper re-dispatch runs in check mode so the carrier compiler compiles the reversed original call (core_ref.go). Residual: usurp combined with quote/codequote or a non-fn target"},
	{"quote", regexp.MustCompile(`\b(code)?quote\b`),
		"REDUCIBLE: code-as-data; needs the compiler to bake the quoted form as a constant token-list value"},
	{"word", regexp.MustCompile(`\bword\b`),
		"REDUCIBLE (mostly COMPILED): the __SP splice now expands inline at the use site (carrier.go). Residual: a few splices whose inlined body reaches an un-compilable word"},
	{"macroexpand", regexp.MustCompile(`\bmacroexpand\b`),
		"REDUCIBLE (static cases COMPILED): the macro expands at compile time and the token list bakes as a code-as-data const (carrier.go). Residual: expansions with a non-Word/un-bakeable member (parens, type nodes)"},
	{"minilang", regexp.MustCompile(`\bminilang`),
		"REDUCIBLE: registers a sublanguage; static registrations could compile, only runtime-input parsing is interpreter-bound"},
	{"flex", regexp.MustCompile(`\bflex\b`),
		"REDUCIBLE (now COMPILED): the pointer-backed flex node rides as a frame local / closure capture, so compiled mutations hit the SAME cell (corpus-core.tsv:50 landed when core `walk` gained a CallableSpec: the descend hook compiles to a closure with the module-scope flex as a capture, and the empty-flex ascend operand is admitted by provenance — eng/go/callable_words.go emptyFlexHookOperand). Kept for classification: a future flex row refusing on a NEW gap would re-surface here"},
	{"canon", regexp.MustCompile(`\bcanon\b`),
		"REDUCIBLE: canonicalisation is a pure function; just unimplemented in the VM"},
	// `args.N` was here ("REDUCIBLE: a frame-local read"); it is now COMPILED —
	// AnalyseFnBody exposes the params as the args projection and
	// tryFoldStaticIndex folds `get N args` to PUSH_LOCAL N (carrier.go). It is
	// the concrete proof that the tier-2 words are reducible, not irreducible.
	{"Test/Assert", regexp.MustCompile(`\b(Test|Assert)\.`),
		"REDUCIBLE: the test/property harness; the candidate bodies compile, the harness accumulates — only random input generation is runtime"},
}

// classify returns the tier a refused/islanded row's source is attributable to:
// "" (compute gap), or the matched tier-1/tier-2 word. Tier 1 is checked first
// so a `Vm.run (canon …)` row counts as interpreter-only, not reducible-canon.
func classify(src string) (tier int, name string) {
	for _, e := range interpreterOnlyWords {
		if e.pattern.MatchString(src) {
			return 1, e.name
		}
	}
	for _, e := range reducibleWords {
		if e.pattern.MatchString(src) {
			return 2, e.name
		}
	}
	return 0, ""
}

// errorRowReason reports whether a refusal is a deliberately-interpreted
// runtime-error surface: the checker refuses so the interpreter raises the
// matching taxonomy (a return-count mismatch, an unpack of a missing key, an
// orphan gen). §3 step 3 dispositions these as allowlisted.
func errorRowReason(reason string) bool {
	return strings.Contains(reason, "suppressed a runtime error") ||
		strings.Contains(reason, "body value count differs") ||
		strings.Contains(reason, "without exactly one declared return")
}

// Three downward ratchets. interpreterOnlyCeiling caps the ONE permanent island
// category (a new tier-1 word is an irreducibility claim that must be argued);
// reducibleCeiling and computeRefusalCeiling both ratchet toward 0 — when they
// reach it, only tier 1 falls back and the unbounded fallback can be narrowed.
const (
	interpreterOnlyCeiling = 3  // Vm.run / Vm.run-with — execute runtime-computed code. NOW 0: the corpus's last tier-1 row, Vm.run(canon [1 {a:none} x/q]), compiled once OpMakeList let the canon list assemble. Kept at 3 for headroom — a Vm.run of a genuinely RUNTIME string would re-populate it
	reducibleCeiling       = 0  // was 1 — 1 -> 0: corpus-core.tsv:50 (`flex`) COMPILED, the tier-2 P7 floor. Core `walk` gained a CallableSpec (BodyPos 2, BodyOut 0 — visit-only, results discarded; LambdaSharesTokenShape — quotation and lambda hooks both receive the one `{key value path parent depth}` payload carrier), so the descend hook compiles to a closure unit walkClassifyHook drives via InvokeBody; the lambda body's module-scope flex accumulator rides as a closure capture (tryRecordLambdaClosure now applies moduleScopeMutableCaptures, same identity argument as token bodies) so the compiled body mutates the SAME pointer-backed FlexList cell the frame local holds; and the row's trailing `acc` — forward-collected as the 4-arg sig's ASCEND hook — is admitted as a value operand only by the empty-flex provenance proof (eng/go/callable_words.go emptyFlexHookOperand: constructed by the main-registry `flex` native over empty const containers, top-level straight-line, no event recorded since construction — so its classify-time token snapshot is provably a zero-length list on every invocation). Every other ascend shape (non-empty flex, mutated-before-walk, loop-nested, lambda ascend) keeps its refusal — pinned by lang/go bytecode_findings_test.go TestWalkHookClosureCompiles. PRIOR 3 -> 1: module-test.tsv:48 (`Test.prop`/`Test.run-property`) FLIPPED — Test.check-prop declared CompileRunsBodyIsolated (its gen/property bodies run in isolated CallAQL frames, sound to bake as CALL_NATIVE) and run-property's param retyped `p:Map` -> `p:PropertySpec` (record-schema-typed gets, proven-dynamic operands admitted by dynInputsProven); the whole `_prop_spec` corpus cluster compiles. Only corpus-core.tsv:50 (`flex`) remains tier-2. PRIOR 2 -> 3: the fn-dispatch UNIFICATION (all fns compile their body as a unit via execMatch — design/MODULE-FN-PARAM-SLOT-COMPILATION.0.md §8) moved a second aql:test harness row to fallback. The recursive code-body word `test-describe` (run-spec -> test-describe -> run-spec) hits the recursive-closure-compilation limit the old inline CallAQL path masked via data-driven recursion termination; the rows fall back SOUNDLY (verify-bytecode + crossdiff green). Restoring native compilation is the recursive-code-body-closure follow-up; the ceiling reflects the new reducible debt, NOT a soundness change. PRIOR 1 -> 2: the two genuine WORD-CLASS tier-2 rows are corpus-core.tsv:50 (`flex` — a reference-cell container; its `walk` body is a code-body higher-order word the compiler does not model) and module-test.tsv:48 (`Test.prop`/`Test.run-property` — the test-harness body needs the module-preamble fns to compile as cross-registry closure units). Both rootCause "coverage". The 7 path-modifier.tsv `m.a/u` rows that ALSO surfaced here (checker fix 15b9fb1 cleared their flex/stored-fn-ref false positives, moving them from check-error to refused value rows) are NOT counted as tier-2: they refuse on "operand provenance" (rootCause "soundness") — usurp of a MAP-STORED fn, the dynamic-fn-application frontier the compute ceiling already tracks, which usurp's own residual definition excludes ("quote/codequote or a non-fn target"); the census now attributes a soundness-frontier row to the compute frontier even when its source mentions a tier-2 word (compiled_census_test.go). HISTORY: prior comment kept for the audit trail — earlier "back to 1" (the help-eval decouple transiently raised it to 2 then IsolateEmit cleared it): macro.tsv:26 (`def defconst (macro …) defconst answer 42 answer` -> 42) was a check-error pre-decouple, briefly surfaced as a tier-2 "quote" refusal (the help-eval const-pool contamination made its macro-def dispatch recover), and now COMPILES once the eval is fully hermetic (CheckState.IsolateEmit) — so the lone remaining tier-2 row is again module-test.tsv:38 (Test.run-spec). PRIOR 2 -> 1 when macro.tsv:45 (`def loopy (macro [[a] [quote [loopy unquote a]]]) macroexpand (loopy 1)`, expect ERROR:expansion too deep) was recognised as a correct-error row rather than a reducible-quote TODO: it is a DIVERGENT recursive macro whose non-termination is not statically decidable, so refuse-and-let-the-interpreter-raise is the only sound behaviour (design §Stage I). The census now checks the error-row disposition (the spec's authoritative ERROR: marker) BEFORE the tier-2 word bucket, so a correct-error row is never laundered as reducible compiler debt merely because its source mentions `quote`. The lone remaining tier-2 row is module-test.tsv:38 (`Test.run-spec`), whose harness body needs the module-preamble fns (run-spec/run-cases) to compile as closure units — the cross-registry module-body work, not a quick win. 3 -> 2 when the word-splice row (`def vs word [2,3] … 1 add2 vs`, residual [1, 5]) compiled: user-fn-call results now seat to a frame local for an out-of-order residual (lower.go planValueDefLocals/lowerUserCall), so the splice's `add2` result above the literal lays out in order; 4 -> 3 when `def x 5  x/u` (a `/u` usurp of a non-fn binding) compiled to an illegal_ref OpTrap (engine.go stepWordUsurp), leaving the tier-2 usurp bucket for the error-raising path; each remaining entry a named, reducible compiler/VM TODO; 6 -> 4 when codequote compiled (a Quoted ParenExpr bakes as an inert code-as-data const, isInertConst ParenExpr case — the 2 codequote.tsv rows left the quote bucket); 54 -> 52 when structural type-pattern operands started baking (two rows that refused at a reducible word past the now-compiled pattern dropped out); 52 -> 51 when dead value-defs stopped refusing (a reducible-word row past a now-dropped dead binding fell out); 51 -> 48 when arity/param/return introspection over a baked fn value compiled (those rows left the reducible bucket); 48 -> 6: re-tightened to the live actual (the ratchet had stalled at 48 while usurp / quote / word / macroexpand / Test-Assert mostly moved to native, draining the bucket); a fixed, small word set with deterministic classification, so kept tight like refusalCeiling
	computeRefusalCeiling  = 56 // was 66 — 66 -> 56: Stage M1 (merged-word seam, design/STAGE3-INLINING-DESIGN-ROUND.0.md §6): buildFnBodyReturnsFn now shares the check state caller→owner at the ReturnsFn boundary itself (shareCheckStateFrom), so a TRANSPLANTED word-extension sig (open words — a module-defined `add` merged into the importer's bare word) unit-compiles into the MAIN program exactly like every other user-fn dispatch; the entire "user fn call (Stage 3)" bucket (open-words.tsv:72,77,78,82,85,86,87,98,99 + micron.tsv:200) compiled to native CALL_USER units, 10 -> 0, with every other bucket byte-identical (no migration). PRIOR — 86 -> 66: typed-def store-with-reparent (OpBindTyped) compiled the "dynamic refinement reparent/validate is interpreter-only" AND "DepScalar predicate validation is interpreter-only" buckets to zero (RecordTypedBind / RunTypedBind — the interpreter's own RunPredicate / Unify-vs-builtin-ancestor / DepScalar Unify re-run over the runtime value, byte-identical errors, ReparentValue where defTypedHandler reparents; miscompile B's refusal closed as a compiled bind). Of the 12 open-words.tsv rows in the bucket, 3 compiled outright and 9 progressed to their NEXT frontier (the open-words merged-word dispatch: 6 "user fn call", 3 "dispatch recovery"), and the ratchet re-tightens to the live actual per the refusalCeiling convention. PRIOR: operand-provenance cascades, code-body DSL words, Stage-1 lowering residuals, dynamic in/out, 5 islands, user-fn dispatch (+8 from merged PR #142 minilang/parselang corpus rows that fall back faithfully; +1 patrun.tsv — a mutable Patrun dispatch / `add` 3-arg-overload row falls back, the bytecode fix is tracked on a separate branch); 166 -> 137: structural type-pattern operands (`{a:Integer}`, `[Integer String]`, `[Resource Entity]`) now bake as const members, so static is/typeof/size over them compiles; 137 -> 126: generic schema bakes as a const, unblocking make/is/typeof/residual over top-level generic types; 126 -> 122: schema body check admits a type-parameter Word, so typed-list-field generic schemas (`{items:[:T]}`) bake too; 122 -> 119: a function-signature value bakes as pure descriptor data, unblocking fnsig residual/typeof and fnsig-bodied generic schemas; 119 -> 100: a surface type bakes as a const (shared canonical descriptor; the compiled path keeps check-pass mints so it never goes stale), unblocking the surface type-algebra cluster; 100 -> 97: typed-def object construction records its skipped make event, giving the bound instance make-equivalent provenance; 97 -> 90: a single-result value-def referenced zero times drops its dead result (OpDrop) rather than refusing as an unconsumed call result; 90 -> 85: a no-capture fn value bakes as a const so it stands as data (residual, map member, introspection operand), with an auto-dispatch-boundary guard keeping a fn-ahead-of-args residual on the fallback path; 85 -> 86: +1 arithmetic.tsv — `add`'s String-concat overloads (`[String Scalar]`/`[Scalar String]`) gained a second Scalar→String coercion row (Boolean and Float operands, both positions) when the old `[Scalar Scalar]` overload was tightened to require a String; the coercion concat still falls back faithfully
)

func TestOnlyMetaFallsBack(t *testing.T) {
	c := gatherCensus(t)
	interp, reducible, errorRows, computeGap := c.interp, c.reducible, c.errorRows, c.computeGap
	tier1By, tier2By, computeByReason := c.tier1By, c.tier2By, c.computeBy

	t.Logf("re-scoped P7 partition: %d interpreter-only (tier 1, permanent), %d reducible (tier 2, TODO), %d error-row, %d COMPUTE GAP",
		interp, reducible, errorRows, computeGap)
	t.Logf("tier 1 — interpreter-only (ceiling %d):", interpreterOnlyCeiling)
	for _, w := range sortedKeys(tier1By) {
		t.Logf("  interp %4d  %s", tier1By[w], w)
	}
	t.Logf("tier 2 — reducible, not yet compiled (ceiling %d):", reducibleCeiling)
	for _, w := range sortedKeys(tier2By) {
		t.Logf("  reduce %4d  %s", tier2By[w], w)
	}
	t.Logf("compute gaps by reason (ceiling %d):", computeRefusalCeiling)
	for _, r := range sortedKeys(computeByReason) {
		t.Logf("  gap    %4d  %s", computeByReason[r], r)
	}

	if interp > interpreterOnlyCeiling {
		t.Errorf("interpreter-only rows %d exceed cap %d — a NEW word claims irreducibility; justify it or compile it",
			interp, interpreterOnlyCeiling)
	}
	if reducible > reducibleCeiling {
		t.Errorf("reducible (tier-2) rows %d exceed ceiling %d — a reducible-but-unimplemented row regressed",
			reducible, reducibleCeiling)
	}
	if computeGap > computeRefusalCeiling {
		t.Errorf("compute gaps %d exceed ceiling %d — a real-compute row regressed to refusing",
			computeGap, computeRefusalCeiling)
	}
}

// sortedKeys returns a map's keys sorted by descending value then name, for a
// stable most-frequent-first histogram.
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}
