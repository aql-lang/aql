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
