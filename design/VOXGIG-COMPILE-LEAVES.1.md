# voxgig-aql interpret/check/compile completion — current leaf map (post-merge)

Goal: every voxgig-aql repo (bloom-filter, decision, sort, stats, trie, **template**)
fully `aql run` (interpret), `aql check`, AND `aql run --force-compile` clean.

Baseline on the merged binary (HEAD `53719f21`, my 22 commits + main `cc3fcf63`):
- **interpret**: 44/48 OK (4 `*_prop_spec` are slow property GENERATORS that time out >60s — not errors; raise the timeout or treat as pass).
- **check**: 47/48 OK — only `template.aql` (24 false-positive errors; see L8).
- **compile** (`-force-compile`): 14 COMPILED, 34 REFUSE, **0 ERROR**. `--compile` == interpret for all 14 COMPILED (verified) and all 34 REFUSE fall back soundly. **Soundness is solid; the goal is COVERAGE.**

KEY: each refusing file is a CHAIN of distinct leaves; the refusal text names only
the FIRST surfacing one (and "code-body word X (Stage 2)" is a MASK — the body is
probe-compiled in a throwaway EmitState at `callable_words.go:249-255`, so the real
reason is swallowed). A file flips only when ALL its leaves clear. Gate every fix
with `verify-bytecode` (the differential) + a hand-written off-corpus
`RunCompiledStrict`-vs-`Run` regression (the differential is blind to these shapes —
this caught the param-guard/Options miscompiles) + the voxgig `--compile`==interpret
sweep.

## Real leaves (unmasked by a 9-agent workflow), highest leverage first

### L0 (FULLY TRACED w/ pointer identity) — multi-pass dispatch records AND refuses the same each
Pointer-identity trace of `each [def j (5 add 1) j]`:
- es A (`0x…f180`, **suspended=1, active=false**): a result-type probe pass —
  tryRecordClosure declines at `!es.active()` (callable_words.go:84).
- es B (`0x…f040`, **suspended=0, active=true**): the LIVE compile es —
  tryRecordClosure REACHES recordClosureDispatch and **records the closure**.
- **REFUSE on es B** (the SAME live es, suspended=0): the `execBodyRefsNames`
  refusal (emit.go:2020) fires and latches es B uncompilable.
So the live es BOTH records the each (closure) AND refuses it (RecordCall), across
the two analyses of g's body (ReturnsFn install + compile). A suspend-skip gate
does NOT fix it (the refusal is on the un-suspended es B; verified — no-op).
Discriminator: `each [def j 5 j]` compiles, `each [def j (5 add 1) j]` refuses —
`valueRefsName` (emit.go:2575) flags ANY Word, so the computed `(5 add 1)`'s `add`
trips it; but narrowing it to non-builtins fixes only `(5 add 1)`, NOT the real
voxgig `[def j (cur get 0) j]` (cur is a genuine frame-local capture the CLOSURE
path handles via capture, yet the const-bake-guard refusal still fires).
**The true fix is multi-pass dispatch CONSISTENCY**: the RecordCall refusal must not
latch the program when the same word records via the closure path in the recording
pass — e.g. defer code-body-word RecordCall refusals and reconcile at Finalize, or
have RecordCall skip the latch for a Callable word whose body is closure-compilable
(probe). Intricate + soundness-critical (a wrong skip lets a true-fallback frame-
local body const-bake → silent miscompile). Mandatory regressions: a captured-
frame-local each body that the closure records (must compile), and a non-closure-
able name-body (must still refuse).

### L0-PRIOR (superseded) — premature `execBodyRefsNames` refusal
**Exact mechanism** (traced with instrumentation): an each whose body contains a
value-def (`def j (5 add 1)`) — i.e. the body REFERENCES A NAME — dispatches TWICE:
1. once with `es.active()==FALSE` (a non-recording pre-pass / nested closure probe):
   `tryRecordClosure` declines at the `!es.active()` guard (callable_words.go:84),
   so carrier.go falls through to `RecordCall`, which hits the
   `(sig.Callable != nil && execBodyRefsNames(sig, args))` refusal (emit.go:2020) →
   `MarkUncompilable("code-body word each (Stage 2)")` → LATCHES the program;
2. once with `es.active()==TRUE`: `tryRecordClosure` REACHES recordClosureDispatch
   and RECORDS the closure successfully.
The body IS compilable via the closure path (pass 2 proves it); the refusal exists
only to guard the const-bake FALLBACK (re-running a name-referencing body in a
sub-engine is unsound), but it fires in pass 1 even though pass 2 records via
PUSH_CLOSURE (no fallback). **`each [def j 5 j]` (const value-def, no name-ref in
the computed sense) compiles; `each [def j (5 add 1) j]` refuses** — execBodyRefsNames
is the discriminator.

**Fix direction:** the `execBodyRefsNames` refusal must not LATCH the program when
the closure path will/did record the body — i.e. defer or skip it in the
non-recording pre-pass (the proven suspend-skip precedent, commit 6e3c089e, but the
state here is `!active` from a nested-probe es-swap rather than `suspended>0`, so the
gate must key on the right pass identity). The es-swap interaction (recordClosure
Dispatch swaps r.Check.Emit for the throwaway probe) is the subtlety to nail before
implementing — a wrong gate either fails to fix it or lets a genuinely-fallback
name-body const-bake (silent miscompile). Mandatory: a RunCompiledStrict regression
for a name-referencing each body that the closure path records, AND one for a body
that genuinely falls back (must still refuse). This unblocks the 14 each + 8 test-
test files.

### L0-OLDER (incomplete) — value-def of a COMPUTED expression inside an each-closure body
**`var` is NOT the leaf** — its sig declares `RunInCheckMode: true` and it compiles
(the splice records as value-def locals). The real dominant leaf, unmasked by direct
minimization: a **value-def whose value is a computed paren-expression, inside an
each-closure body**. Precise repro:
- `[1] each [def j 5 j]` → COMPILES (value-def of a const)
- `[1] each [def j (i mul 2) j]` → COMPILES (param-computed)
- `[1] each [def j cur (j get 0)]` → COMPILES (value-def of a capture directly)
- `[1] each [def j (5 add 1) j]` → **REFUSES** (value-def of a computed const)
- `[1] each [def j (cur get 0) j]` → **REFUSES** (value-def of a computed capture-get)

The SAME body `def j (5 add 1) j` compiles as a plain FN body (`def g fn
[[][Integer][def j (5 add 1) j]] (g)` → 6) but refuses as an each-closure body — so
it is the each$body promotion path (compileClosureBody, callable_words.go:31), not
value-defs generally. FURTHER NARROWED: the throwaway closure PROBE (callable_words.go:251)
SUCCEEDS; the refusal is in the post-probe REAL compile (:259) or RecordClosure
Call's collection-operand resolve — i.e. a state-difference between the probe and the
real emit. The next pass must instrument the REAL compileClosureBody call (:259) +
RecordClosureCall, not the probe, to find the exact MarkUncompilable site, then fix
the each$body value-def promotion (leaves-doc carrier-ID hazard applies; mandatory
per-frame-local-kind RunCompiledStrict regression). This is the 14-each + 8-test-test
blocker; `var` was a red herring.

### L0-OLD (WRONG) — `var` block hypothesis (kept for the record; var compiles)
**Status: this is what actually blocks the 14 each + 8 test-test files** (L1 below
cleared a different each-leaf but flipped 0 files — the chains hit `var`). The loop
idiom `iota n each [ var [[t] <body with def/if> ] ]` (sort.aql:98 etc.) refuses
because `var` is a `CompileExecutesBody` macro: its handler desugars to `def t end;
<body>; __varundef t` (native_definition.go:583-654; `__varundef` exists precisely
"to let a var-body compile to a closure unit"). The desugared form IS compilable,
but the compiler can't bake a `CALL_NATIVE(var)` — the VM would re-run the handler
and trip on the tape-coupled tokens (emit.go:2020-2041 refusal). **Fix: macro-inline
compilation** — recognise `var` (and similar splice macros) and compile its spliced
desugaring INLINE (the def-locals as scoped frame locals + the body + __varundef
cleanup) rather than as a native call. Substantial; the highest-leverage remaining
compiler feature (unblocks the 14 each + 8 test-test files once their other chain
leaves also clear). Gate hard: the property fuzzer caught var-block miscompiles
across all three frame-local kinds (param/capture/for-iterator), so an off-corpus
`RunCompiledStrict` regression per kind is mandatory.

### L1 — each over a list-literal with a COMPUTED element (14 files) — FIXED (609d502b)
`[(mk)] each [...]` refuses; `[1 2 3] each` (const) compiles. The collection is a
list-carrier with a non-const element → `resolveOperand` declines (emit.go:690,
not inert-const, not a recorded event). Even `[(mk) (mk)]` at TOP LEVEL refuses
("residual value of unknown provenance"). A list with a NATIVE element
(`[(m get "a")]`, `[1 (2 add 1) 3]`) DOES compile via `RecordMakeList`
(engine.go:3170/3684) — the gap is the **user-fn-call element**. Pattern in the
files: `def specs [ (Test.test …) … ]` then iterate. **Fix: compiler** — record
`OpMakeList` for a list whose elements are user-fn-call results.
Files: all 14 `each`-class.

### L2 — `.field`/`get` on an IMPORTED-module class instance types Dynamic (8 files)
The imported class's declared field types aren't materialised in the consuming
module, so `compiled.field` is Any → `dynamic input`/Stage-2 mask. Inlining the
class into the test file makes it COMPILE (so it's cross-module typing). **Fix:
compiler (materialise imported-class field types) or repo.** Surfaces as `test-test
(Stage 2)`. Files: all 8 `*_unit_test` (the `Test.test`/run-spec harness).

### L3 — `do {k:[expr]}` map-construction provenance (4 files)
A `do`-map whose value block is a branch-merged (`if`) result or an unresolvable
fn-result has no materialisable provenance (emit.go:2140). bloom's `encode` do-map;
stats/trie fold-body do-maps. **Fix: compiler** — materialise a branch-merged /
computed do-map value via `OpMakeMap`/`OpMakeList`. (A partial repo tweak
`set:[x]`→`set:x` preserves interpret but does NOT flip — the other computed values
keep the chain.) Files: bloom_smoke, bloom_unit_spec, stats_smoke, trie_smoke.

### L4 — single-overload USER fn over an Any receiver (3 files)
`(nd find-kid)` where `nd` is Any (from a user fn's `[Any]` return): no-signature
recovery, `tryRecordPoly` declines because `find-kid` is a USER fn not a builtin
(carrier.go:989) → MarkUncompilable (engine.go:6675). **Fix: compiler** — emit a
normal user-fn call (CALL_AQL) to the best-fit candidate; SOUND because the
CALL_USER param guard (added this session) re-checks the receiver at runtime, so a
mismatch raises the same signature_error the interpreter does. Files: trie_unit_spec,
trie_unit_test, tst_unit_test (find-kid/mk-tnode).

### L5 — `push`/`set` over a Dynamic receiver (2 files)
In-place mutators over an Any receiver refuse (emit.go:2063) before element
provenance. **Fix: compiler** — make push/set poly-safe over a Dynamic receiver
(OpCallNativePoly), or repo. Files: burst_unit_test (push), radix_unit_test (set).

### L6 — gradual-Any → multi-overload user fn `apply-op` (1 file)
My cluster-C refusal (sound). **Fix: compiler** — record a user-fn poly that
re-matches the overload at runtime; OR repo narrows the overloads. File:
decision_smoke_test.

### L7 (CHECK track) — `parse <kind>` over a runtime-registered grammar (template)
`Parse.register` is a runtime side effect invisible to the static check pass →
false `parse_unknown_lang` → `fn_body_error` → 24 of template.aql's check errors
(and its force-compile `check diagnostics` block + template_smoke `lex-mustache`).
**Fix: compiler** — give `parse-register` (parse.go:275) a `ReturnsFn` that installs
the kind at check time, mirroring `parselang-register`'s `parseRegisterReturns`
(parselang.go:478), with idempotent registration for the compiled double-run.
template.aql ALSO has residual `no_signature`/`unused_def` false positives (some
may resolve once `parse` resolves; the rest are separate checker-accuracy work).

## Recommended sequence
L4 (3 files, clean, sound via param guard) → L7 (unblocks the entire template check
track) → L1 (14 files, biggest) → L3 (4) → L2 (8) → L5 (2) → L6 (1). Re-run the
corpus retest after each; a file flips only when its whole chain is clear.

This is the compiler/checker-completion endgame — a sequenced multi-pass effort.
Soundness (`compile==interpret`) is already verified solid; this is purely about
turning sound REFUSALS into sound COMPILATIONS, plus the template checker accuracy.

### L0 — fix attempts RULED OUT (do not repeat)
1. **Suspend-skip the Stage-2 refusal** (gate MarkUncompilable on `es.suspended==0`):
   NO-OP. The latching refusal is on the LIVE active es (suspended=0), not the
   suspended probe es.
2. **Skip the refusal if the result is already recorded** (`es.producedBy[outs[0].ID]`):
   NO-OP. The refusing dispatch's result carrier has a DIFFERENT ID than the
   closure-recording dispatch's — they are separate re-analyses of the same source
   each, so an ID match never fires.
Open inconsistency for the next pass: `RecordCall` early-returns when `!es.active()`
(emit.go:1882), so the refusal only runs ACTIVE; yet the refusing dispatch did NOT
print a tryRecordClosure entry trace despite `sig.Callable=true` (so it did not
return at the `Callable==nil` guard either). The next pass must instrument the
carrier.go:603 dispatch chain ITSELF (which tryRecord* declined, and why
tryRecordClosure's entry wasn't reached) for the SECOND active dispatch of the same
each — likely a re-dispatch during the value-def's result-type analysis. The fix is
almost certainly to make that re-dispatch take the closure path consistently, or to
not run RecordCall's code-body refusal for a Callable word already lowered as a
closure at this site. Keep the two mandatory regressions (captured-frame-local each
body must compile; non-closureable name-body must still refuse).

## CORRECTED RETEST (post-L0, run from each repo ROOT — CRITICAL methodology)

Earlier "8 compiled / 23 check-diagnostics" was a MEASUREMENT ARTIFACT: the spec
files do `import "./bloom.aql"` resolved relative to CWD, so running from the wrong
dir failed every import → spurious `undefined_word` check errors. Run each file FROM
ITS REPO ROOT (`cd bloom-filter; aql check test/bloom_unit_spec.aql`). Correct state:
- **interpret 44/48** (4 `*_prop_spec` are slow property GENERATORS, >60s timeout — not errors).
- **check 47/48** — only `template.aql` (one `fn_body_error`). THE CHECK TRACK IS FINE.
- **force-compile 14/48**. Refusal breakdown (by leaf, files):
  - 14× `code-body word each` — but the each PATTERN now compiles (L0); the residual
    cause is a MODULE CODE-BODY WORD in the body (`Test.run-spec`).
  - 8× `code-body word test-test`; 2× `test-describe`; (run-spec → test-describe).
  - 5× `unmatched dispatch recovered at find-kid / mk-tnode / lex-mustache` (user-fn poly).
  - 2× `code-body word do`; 1× each `fold`, `set`(dynamic), `push`(dynamic), gradual-Any.
  - 2× real `check diagnostics` (template-family).

### L0 LANDED — commit 5f6561e4 (sound, gated, verify-bytecode + full suite green)
collectBodyLocalDefs excludes body-local `def`/`var` names from ComputeCaptures.
The each-body-local-value-def + var-loop patterns now compile (proven: `each [var
[[s] (s get "name") 0]]` → COMPILES). Flips 0 voxgig FILES (chains), but is the
foundation: the test-file eachs now refuse only on the MODULE code-body word inside.

### DOMINANT REMAINING LEAF — the aql:test framework's code-body words
`Test.run-spec` / `test-test` / `test-describe` (lang/go/modules/test.go:227,275,1411)
DECLARE a CallableSpec (BodyPos 1, BodyOut 0) — so they SHOULD compile via the
closure path — yet refuse "Stage 2". The recursive describe/spec body (run-spec calls
test-describe with a body that recursively calls run-spec) fails the closure compile.
This blocks ~all test files. NOTE: the repos' OWN gate (test/divergence/run.sh) is
INTERPRET + `aql --compile` (fallback-allowed) must SUCCEED AND AGREE — i.e.
compile==interpret, which HOLDS for all 48 (0 miscompiles). force-compile coverage of
the test framework is a STRETCH beyond the repos' gate. Next pass: unmask test-
describe's closure-body refusal (instrument recordClosureDispatch for word=="test-
describe") and compile the recursive spec body, OR accept fallback per the repos' gate.

### ROOT-CAUSED: the test-framework leaf IS the single-overload user-fn recovery leaf
test-describe's closure body fails with Reason `unmatched dispatch recovered at
run-cases` (instrumented compileClosureBody). `run-cases` (test.go:1403) is a
SINGLE-overload AQL user fn `fn [[| subject:Scalar cases:List][]…]`. Called with
Any-typed spec data, matchSignature can't statically commit → the engine.go:6664
recovery branch runs. tryRecordPoly DECLINES because its gate (carrier.go ~line 26)
is `if !matchReg.IsBuiltinWord(word) return false` — run-cases is a user `def fn`,
not a registered builtin (test-record IS a sub-registry builtin, so it passes; that's
the asymmetry the 6656 comment notes). So the recovery MarkUncompilable's.

THIS ONE LEAF blocks: the aql:test framework (run-spec → test-describe → run-cases →
~all test files) AND find-kid / mk-tnode / lex-mustache (the 5 direct recovery files).
Highest-leverage remaining leaf by far.

**Fix direction (soundness-critical, next focused pass):** in the engine.go:6664
recovery, when tryRecordPoly declines AND the word is a SINGLE-overload user fn,
record a user-fn dispatch (a plain CALL_AQL to its one sig — no poly re-match needed
since there's only one overload) GUARDED by checkParamContract (the param guard
landed this session). Soundness: at run time the concrete args either match the sole
sig (dispatch == interpreter) or fail the guard (raise == interpreter's no_signature).
A MULTI-overload user fn must STILL refuse (Cluster C) — the guard would raise where
the interpreter dispatches a sibling. Mandatory off-corpus regressions: a single-
overload user fn over Any args (must compile + RunCompiledStrict==interp), and a
multi-overload one (must still refuse). The repos' OWN gate is compile==interpret
(holds via fallback today); this fix converts fallback → native coverage.

### L4 LANDED — commit 913fe6a9 (Any + disjunct recovery, sound, gated)
tryRecordRecoveredUserFn records a guarded CALL_USER for a single-overload arg-bearing
user fn at BOTH the disjunct-straddle (6635) and Any-carrier (6675) recovery sites.
Clears find-kid / mk-tnode / lex-mustache (now at their next chain leaf: "operand of
unknown provenance at do/size"). Net voxgig flips: 0 (deep chains), same as L0/L1.

### test-framework run-cases reaches a THIRD recovery path (engine.go ~6700)
Instrumented: test-describe's body still fails Reason "unmatched dispatch recovered at
run-cases" AFTER L4 — run-cases there is dispatched over CONCRETE-but-statically-
mismatched args (NOT Any/disjunct carriers), so it skips both L4 sites and falls to
the 6700 concrete-mismatch fall-through ("genuine type error" path). Extending
tryRecordRecoveredUserFn to 6700 is plausibly sound (single-overload + the param
contract = dispatch-or-raise == interpreter) BUT is the RISKIEST site: concrete args
mean the param guard must match the interpreter's matchSignature coercion EXACTLY, and
a mismatch is a silent miscompile the differential is blind to. Deferred to a focused
pass with the mandatory off-corpus regression (a single-overload fn over concrete-
mismatched args that the interpreter coerces vs one it rejects). NOTE: even clearing
6700, test-describe likely chains further (test-test, recursive run-spec). The repos'
OWN gate is compile==interpret (holds via fallback); force-compiling the recursive
test framework is the stretch goal.

### CURRENT STATE (this session: L0 + L4 landed, sound/gated/committed)
interpret 44/48 (4 slow generators), check 47/48 (only template fn_body_error),
force-compile 14/48. Compile leaves remaining: each/test-test (test framework, deep
chain), do/provenance (2+1), dynamic set/push (1+1), fold (1), gradual-Any multi-
overload (1, correctly refused), template check (1). compile==interpret holds for all
48 (0 miscompiles) — the repos' own gate passes; the gap is native force-compile
COVERAGE, dominated by the recursive aql:test framework.
