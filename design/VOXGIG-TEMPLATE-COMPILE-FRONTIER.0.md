# voxgig `Template` library — bytecode-compile frontier

Diagnosis of every bytecode-compilation refusal the **voxgig `Template`**
library (`voxgig-aql/template`) and its test suites trigger against `aql`
`main` (`203ea2f`), and the one fix landed here. Method: build `aql`, run each
suite under `-force-compile` / `-compile-report`, reduce every refusal to a
minimal reproducer, and map it to its emit/lower site. This is the Template
corpus's analogue of `VOXGIG-COMPILE-COMPLETION-PLAN.0.md` (which covers the
bloom/stats/decision/trie/sort corpus).

## The one hard rule held

`compile == interpret` (byte-identical stdout). Confirmed for every Template
suite: `diff <(aql -compile f) <(aql -no-compile f)` is identical on stdout for
all eight files, and `make verify-bytecode` stays green with the fix below.

## Refusal frontier (whole-program, after the do-map fix)

| Refusal | Where it fires | Shape in the library | Status |
|---|---|---|---|
| `code-body word each (Stage 2)` | `template_prop_test` top level | `Test.results each [ var … ]` (the test file's own result loop) | open — higher-order body reads an enclosing binding (leaf-1 class) |
| `code-body word test-test (Stage 2)` | every `*_unit_test` | `Test.test "name" [ … ]` framework word | open — the check-prop/test-body class (leaf-6 class) |
| `operand … at size` / `branch reads enclosing computation` | `lex-mustache`, `compile-tagged-seq`, … | `mustache-acc size` where `mustache-acc` is a module-level `def x (flex [])` | open — a mutable **module reference** read through dynamic scope; see below |
| `fn gen-program: consumes loop results` | `gen-program` | `(compile-tagged-seq … ) get "code"` — a recursive fn returning a `do {…}` map, result consumed | **FIXED here** |

`vm-run-with` (the sandbox runner) and the `size`/`each`/`test-test` rows share
the chain: `-force-compile` aborts at the first, so the frontier is a *chain*
per file — the `flex` read gates the large majority. Because the `flex` read
gates first, the `do {…}` fix below flips no Template file on its own until it is
resolved; it is landed as a langspec-gated correctness step (it also fixes the
shape for any non-Template program), the same discipline
`VOXGIG-COMPILE-COMPLETION-PLAN.0.md` uses for its flips-0 leaves.

## Fix landed: `do {…}` value-eval map is a single (non-variadic) result

`tryRecordDynBody` (`carrier.go`) flagged **every** `do`-body event
`variadicResult`. That is correct for the code-body (`do [list]`) overload — a
sub-program residual is runtime-variable — but wrong for the value-eval
(`do {map}`) overload, which produces **exactly one** value (the map) whatever
its members compute. As a whole fn body the over-mark was rescued by
`RecordUserCall`'s declared-return exception; **inside an `if` arm** it was not,
so the branch merge (and any fn whose body is that `if`) became variadic and a
fixed-arity consumer of the fn result — `(mk …) get "code"` — refused
`consumes loop results`. The fix marks the concrete-map value-eval overload
non-variadic at the source (`!body.Dynamic && len(outs)==1 && body Is Map`); the
List overload and the dynamic-body poly case stay variadic. Regression:
`TestDoMapVariadicArmCompiles` (`RunCompiledStrict == Run`, byte-identical, which
is also the no-FALLBACK-island proof since force mode does not fall back).
`make test` + `make verify-bytecode` green.

**Note on `make cover-gate`:** main (`203ea2f`) already leaves two rare
interpreter-only branches uncovered (`engine.go`'s
`isPendingResidualContainer` non-container fall-through and
`unwindLiveFrames`'s `PopFrameArgs`-error arm), so the 100% gate is red before
this change. This change is cover-gate-**neutral** — the uncovered set is
byte-for-byte identical with and without it (verified by running the gate on
both trees). It adds no uncovered statement; the pre-existing gap is unrelated.

## The `flex` module-reference read (open — do NOT paper over)

`def mustache-acc (flex [])` is a **mutable module-level reference**; the lexer
and `lex-*` fns read it (`mustache-acc size`, `mustache-acc push …`). A concrete
`flex` value cannot bake as a const (baking would capture the compile-time
instance and diverge from the runtime one), so the sound lowering is a runtime
dynamic-scope read (`OpLookupDynScope`, `curReg.Defs.Top(name)`, identical to the
interpreter's substitution). `dynScopeRescue` already emits exactly that — but
declines a module-level name because the call-graph reachability gate
(`dynamicScopeReachable`, `FnBinders`) only admits **fn-bound** names, and a
module def is bound by no fn.

**A naive widening is UNSOUND and was reverted.** Admitting module-scope names
by any of the tried signals — `Depth(name) <= baseline[name]`, the
`ModuleBinders`-at-`FnBodyDepth==0` set, `FnBinders[name] empty`, `!v.Dynamic`,
`suspended==0` — each mis-classified a **`fold`/`each` body-local** (`liquid`'s
`split-args` binds `q` inside a `fold` body) as module scope during the
higher-order-body **sub-run compile**, where the fn baseline, `FnNameStack`, and
`FnBinders` are all detached from the enclosing fn. That emitted an
`OpLookupDynScope` for `q`, which **misses at run time inside the fold callback**
and corrupts the fold result (`interpreter fail-count 0`, compiled `1` — a
silent miscompile the langspec differential is blind to, caught only by running
the Template suites compiled).

The sound fix needs a module-scope signal that survives the higher-order-body
sub-run. **This was found and works** (see next section) — but it unmasks a
deeper, separate cross-registry defect, so the `flex` read still **refuses and
falls back** (sound, slower) pending that Stage-C work.

## Update — the read-site classification is the sound module-scope signal

The mis-classification above is entirely an artifact of asking the question in
the **emit recorder** (a higher-order-body sub-run, where the baseline is
detached). Asking it at the **READ site** (`engine.go` stepWord, where the live
baseline is the reading fn's own) is exact: instrumented over the whole Template
suite, `ModuleScopeBinding` returns `true` for every `*-acc` flex-cell read and
`false` for every `fold` body-local `q` read — 100% reliable, no exceptions.

The mechanism (prototyped, then reverted — see below):

- `CheckState.ModuleScopeReads` — a per-pass set. `engine.go`'s def-read site
  records `w.Name` when `ModuleScopeBinding(e.registry, w.Name)` (the *reading
  fn's* baseline). A concrete module reference is also `SetDynFrom`-tagged so
  `dynScopeRescue` can recover its name (a `flex` value carries no `Value.ID`).
- `dynScopeRescue` admits `name` when `c.ModuleScopeReads[name]` **or** the
  existing `dynamicScopeReachable` gate — the module case needs no call-graph
  proof (a module def is unconditionally live in the registry). A fold/each
  body-local is never in `ModuleScopeReads`, so it keeps the reachability gate
  and is never mis-admitted. This flipped **7 of 8** Template files to fully
  compiled with byte-identical output.

## Why it is NOT landed: a separate cross-registry `dynEnv` miscompile (Stage C)

Landing the read-site fix flips `liquid_unit_test` to a miscompile — but **not**
through the flex read. A `do {…}` value-eval arms program-wide `dynEnv` mode
(`tryRecordDynBody`), under which every `def` and param dyn-binds
(`OpBindDynScope`) and dynamic body reads lower to `OpLookupDynScope`. In the
**imported** `template.aql`, `split-args`'s `do {… q:[q]}` (q a `fold`
body-local) emits an `OpLookupDynScope q` whose `OpBindDynScope` twin is not live
when the value-eval sub-run reads it → a run-time miss that corrupts the fold
(interp `fail-count 0`, compiled `1`).

Crucially this is **independent of the flex fix and of the module-scope read**:

- `split-args` has NO module-scope read; the read-site fix does not touch it.
- The identical `split-args` shape compiles byte-identically **standalone**
  (`RunCompiledStrict == Run`) AND **through a 2-file import** — the defect only
  surfaces inside the *full* Template module, a context-dependent cross-registry
  `dynEnv`/`OpBindDynScope` coordination gap.

That is exactly Stage C in `aql-bytecode-next-stages.0.md` — "sound module-body
compilation (cross-registry EmitState) … the one stage that is a *project*, not a
commit," gated on a corpus re-baseline. The read-site module-scope mechanism is
the correct Stage-E/F piece and should land **together with** the Stage-C
cross-registry `dynEnv` fix, so it never exposes the latent miss. Until then the
flex read refuses and falls back (sound).

## Langspec Stage D/E/F status (for reference)

At the internal langspec surface these stages are **already cleared** (prior
work): `reach.tsv:38` (Stage D, `getpath ∘ setpath` over a dynamic receiver) →
`7`; `flex` push/drop alias reference semantics (Stage E) → `[1 2 3]`;
`recursion.tsv:72` (Stage F, a callee reading the caller's `n` by dynamic scope)
→ `42` — all compile natively under `-force-compile`. The 9 residual langspec
refusals are all documented **Stage-H** "unmatched dispatch recovered" ERROR
rows (`knownRefusals`), where the program raises and a static guess would
diverge — correct-by-design soundness refusals, not D/E/F gaps.
