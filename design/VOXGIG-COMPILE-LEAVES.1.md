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

### L1 — each over a list-literal with a COMPUTED element (14 files)
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
