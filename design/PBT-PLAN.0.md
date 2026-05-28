# Property-Based Testing for AQL

> **STATUS: STAGES 0-5 COMPLETE — READY FOR STAGE 6 (2026-05-28)**
>
> Remaining work: decision PBT spec (Stage 6 — end-to-end demo).
>
> What landed:
> - Stage 0d (7d46b68): expandDottedWord doc cleanup.
> - Stage 1 (7d46b68 + 4ca8ffc): aql:rand module with
>   `rand.with-seed N` for deterministic instances; top-level is
>   non-deterministic by default; half-open `[lo, hi)` range.
> - Stage 2 (7d46b68): `gen` policy profile.
> - Stage 0a/0b/0c (0ffe9ea): kernel-level `eng/go/stackform/`
>   package — `StackForm`, `Compile`, `Eval`, `Pretty`, `Cost`,
>   `Equal`, `Walk`. Engine carries an opt-in `Recorder` hook fired
>   from execMatch + the forward-completion site; stepCloseParen
>   notifies via `RecorderSkipper.Skip` to dedupe paren-rewind
>   re-visits. Equivalence tests in
>   `lang/go/test/stackform_equivalence_test.go` confirm
>   `Eval(Compile(reg, src)) ≡ direct Run(src)` for arithmetic,
>   comparisons, strings, stack ops, lists, paren grouping, and
>   forward/stack/swap surface forms.
> - Stage 4 (this commit): `lang/go/modules/test/shrink/` package
>   with Transparency annotations (`Transparent`/`Generator`/
>   `Frozen`/`Opaque`), DefaultPolicy (covers arithmetic/comparison/
>   stack ops as Transparent, `rand-*` as Generator, `time-*` /
>   `fetch-*` as Frozen, unknown words as Opaque), and `ShrinkCost`
>   that adds policy weights + literal-complexity (intMagnitude,
>   string length, list size) on top of `stackform.Cost`.
> - Stage 5 (this commit): the greedy `Reduce` loop in
>   `shrink/reduce.go` + rewrite families in `shrink/candidates.go`
>   (drop-op, integer/decimal/string/boolean/list shrinking,
>   recursive Quote-body shrinking). Integration with `aql:test`
>   in `runCheckProp`: on failure, calls `shrinkFailingInput` which
>   wraps the value in a single-PushLit StackForm and lets the
>   reducer try smaller alternatives, re-running the property body
>   against each candidate. Populates `shrunk-input`,
>   `shrunk-source`, `shrunk-cost` in the PropertyResult.
>   `max-shrinks=0` disables shrinking (returns failing input
>   verbatim); default is 200.
> - Stage 3 (f37849a): `PropertySpec` + `PropertyResult`
>   Records, `test.check-prop` Go native (6-arg: name, gen,
>   property, runs, seed, max-shrinks), `test.prop` Go constructor
>   (preserves gen/property bodies via NoEvalArgs + Quoted=true),
>   AQL `run-property` fn that destructures a PropertySpec map.
>   Generator bodies bind `r` to a fresh seeded rand instance per
>   iteration (seed+i). Property bodies receive the generated value
>   on the stack and must return Boolean. Failure recorded with
>   `failing-input`; results flow into the active `testRun`.
>   shrunk-* fields populated as placeholders (mirror failing-input)
>   until Stage 5.
>
> The argument-ordering refactor (`design/SIG-ORDER-REFACTOR.0.md`)
> that was blocking Stage 0 has landed too — every dispatch path
> agrees on top-first sig order.
>
> **API update post-refactor**: the rand module has been reshaped so
> the **top-level** `rand.*` is non-deterministic by default (clock-
> seeded at module-build time). Deterministic / reproducible
> sequences come from `rand.with-seed N`, which returns an **isolated
> OrderedMap instance** carrying the same methods (`int`, `bool`,
> `float`, `string`, `one-of`). Each instance has its own PRNG. The
> old `rand.seed N` (top-level mutation) and `rand.fresh-seed` words
> were removed. Range semantics: **`rand.int LO HI` is half-open**
> `[LO, HI)` — inclusive lower, exclusive upper.
>
> The table in §"Stage 1" below should be read with these changes
> applied: replace `rand.seed` / `rand.fresh-seed` with
> `rand.with-seed N → Map`. The `rand.list-of` and `rand.map-from`
> rows still apply for future work — they remain deferred (FnSig has
> no NoEvalArgs equivalent yet; see `SIG-ORDER-REFACTOR.0.md`'s
> "Out-of-scope follow-ups").
>
> For PBT determinism, generators MUST use `rand.with-seed N`
> instances; the clock-seeded top-level would break replay. The gen
> policy profile currently allows both — Stage 3 should add a runtime
> check or a stricter sub-profile that denies top-level rand words,
> forcing PBT runs to pass through seeded instances.
>
> Resume with Stage 6 — author 3 properties against the decision
> module (hit-policy stability, collect-order preservation, De
> Morgan negation), plus a negative-control test that proves the
> shrinker collapses a failing input to the minimum violator.

## Context

`aql:test` currently supports declarative table-driven specs only — every case has fixed `in`/`out`. The next capability is **property-based testing (PBT)**: assert that a predicate holds for many randomly-generated inputs, and when it fails, automatically reduce the failing input to a minimal counterexample.

### Terminology

| Abbrev. | Stands for | Meaning here |
|---------|------------|--------------|
| **PBT** | Property-Based Testing | Run a predicate against N randomly-generated inputs; on failure, shrink. |
| **PRNG** | Pseudo-Random Number Generator | Seeded deterministic source of randomness (Go's `math/rand`). |
| **Stack form** | (no abbreviation) | A canonical, surface-syntax-free representation of an AQL program — every word call is in strict-stack order, no forward collection, no dotted access, no paren regrouping. The form the shrinker operates on. This plan uses **"stack form"** (not "IR") for two reasons: (a) it matches the terminology in `design/aql-bytecode-report.0.md` (the parent design doc this aligns with — see below) and `design/aql_property_based_reduction_report.md:229`; (b) "IR" is overloaded compiler jargon implying SSA-like passes that don't apply here. |
| **MDL** | Minimum Description Length | Cost-model principle: prefer the program with the smallest description (under a fixed cost table) that still triggers the failure. |
| **AQL-G** | AQL **G**enerator profile | A restricted policy profile (the report's name) that permits math + `aql:rand` + `aql:decision` and denies side-effecting words. The reducer re-evaluates candidates under this profile. |

### Why the 4-layer design

The design in `design/aql_property_based_reduction_report.md` argues that the most interesting PBT formulation for AQL is **counterexample shrinking as failure-preserving program compression**: don't shrink the JSON value, shrink the *generator program* that produced it, using an MDL cost model over the program's stack form. The four layers are:

```
1  Surface AQL authoring                              (existing)
2  Pure embedded AQL-G profile                        (build)
3  Canonical typed strict-stack form                  (build — see Stack form alignment below)
4  Failure-preserving reducer over stack form         (build)
```

### Stack form alignment with the bytecode design

`design/aql-bytecode-report.0.md` already proposes a recording mode on top of the existing check pass that emits a flat strict-stack instruction stream — its central thesis is "the compiler is the checker with a recording side effect" (lines 61-74). The check pass at `eng/go/check.go:35-49` and `eng/go/engine.go:213-272` already runs `matchSignature` at every call site and resolves every dispatch decision; the bytecode report's plan is to add a recording buffer so those decisions become an emitted instruction stream.

This plan adopts that architecture directly. Concretely:

- **Stack form** here is the higher-fidelity tier the bytecode report calls the "flat linear sequence of fixed-arity operations" (line 119) **before** instruction selection. Word names are preserved (not interned to `sig_id`), no `SWAP`/`ROLL` synthesis, no jump-label resolution — those are encoding choices for a future runtime bytecode. PBT shrinking needs the higher tier (readable, rewritable, pretty-printable).
- The pass that emits stack form is the check pass with an optional `RecordForm` flag. When the flag is on, every successful `matchSignature` writes a `Call{Name, Arity}` op; every literal push writes `PushLit{V}`; every quoted body recurses. When the flag is off, check mode behaves exactly as today.
- This work is therefore also concrete groundwork for the bytecode VM described in `aql-bytecode-report.0.md`. The future bytecode emitter becomes a downstream consumer of stack form (or an alternative recording fidelity in the same pass), rather than a parallel implementation.

Decision tables and predicates (`aql:decision`) are the chosen experimental subject: hit-policy invariants and De Morgan-style predicate rewrites have non-trivial properties that real-world generators can shake out.

### Locked-in scope choices

- **Full 4-layer architecture** (not just value-level shrinking).
- **`aql:rand` as a standalone module, `test.gen-*` re-exports** for ergonomics.
- **`PropertySpec` Record** in `aql:test`, mirroring `TestSpec`.
- **Shrink the generator program** via lowering to IR.

### Critical design constraint (one matcher, one truth)

Emitting stack form requires knowing, **per call site**, which signature was chosen and how its args were filled (forward vs stack). This is *exactly* what `matchSignature` already computes during the check pass. Re-deriving it via a simplified imitation would diverge from the engine's actual behaviour (see report §17 Gotchas 1+2, and `lang/go/CLAUDE.md` "Argument Ordering (CRITICAL)").

**Important distinction**: signature matching decides "which signature applies and which surface tokens fill which positions". Dispatch is the *separate* downstream step that mutates the stack, runs `rearrangeForForward`/`insertForward`, and calls the handler. The check pass already performs the former (without the side effects of the latter) at every call site — we hook in there.

Investigation result (Phase 1 exploration):
- `matchSignature` (engine.go:2828, *not* match.go — match.go contains `patternsOk` at line 22) is **pure**: no stack mutation, no token consumption, no registry writes.
- The check pass (`eng/go/check.go` + `engine.go::Engine.Run` with `r.Check.Mode = true`) calls `matchSignature` at every word in source order and resolves every dispatch decision via carriers. It does **not** today emit a stack-form trace — `CheckState` at `eng/go/registry.go:108-168` only accumulates diagnostics, fn summaries, and def-usage info.
- Adding a recording buffer to `CheckState` (`CheckState.Form *StackForm`, gated by `CheckState.RecordForm bool`) gives stack-form emission as a check-pass side effect at ~5% overhead when enabled and zero when not. This matches the architecture proposed by `aql-bytecode-report.0.md` and means there is exactly *one* matcher driving both checking and form emission.

This is a multi-stage build (~2300 LOC + tests). The plan breaks it into stages that can each be reviewed and committed independently.

---

## Stage 0 — Reusable foundation: check-pass stack-form recording + doc cleanup (~600 LOC)

This stage is independently valuable beyond PBT — it delivers half of what `design/aql-bytecode-report.0.md` proposes (the recording side effect; the bytecode encoder/VM remain future work). Any future analyser, formatter, optimiser, or runtime back-end benefits from a canonical stack form produced by the existing check pass. Stage 0 is shipped as its own commit and tested in isolation; the later PBT stages build on top of it without needing changes here.

### 0a — Stack form types + recording buffer (~150 LOC)

**Package location**: `eng/go/stackform/` (sibling of `eng/go/parser/`). Kernel-level, no language-layer dependencies, so language-layer consumers (PBT, formatter, future bytecode encoder, alternative back-ends) all import it.

**New file**: `eng/go/stackform/stackform.go`
```go
// StackForm is a canonical strict-stack representation of an AQL program.
// Word calls are in stack order, forward collection has been resolved,
// dotted access has been normalised to get-calls, paren grouping is gone.
// Produced by the check pass when RecordForm is enabled (see eng/check.go).
type StackForm struct {
    Ops []Op
}
type Op interface { opMarker() }
type PushLit struct { V eng.Value }              // literal, atom, type literal
type Call    struct { Name string; Arity int }   // word call (Arity from sig)
type Quote   struct { Body *StackForm }          // quoted body (def/fn/if/etc.)
type DoEval  struct { }                          // explicit do of top quotation
```

Also: `Walk`, `Equal`, and a `Cost` placeholder (flat per-Op weights; PBT-specific cost adjustments live in Stage 4).

### 0b — Hook stack-form recording into the check pass (~200 LOC)

**Goal**: when `CheckState.RecordForm` is true, the existing check-mode walk emits a `StackForm` as a side effect of every successful `matchSignature` and every literal push.

**Files modified**
- `eng/go/registry.go` — add to `CheckState` (around line 108):
  ```go
  RecordForm bool                 // opt-in: record stack form as we walk
  Form       *stackform.StackForm // populated when RecordForm == true
  ```
- `eng/go/engine.go` — emit Ops at the existing dispatch points:
  - Literal-push path (`stepLiteral` / wherever non-word tokens enter the stack): when `c.RecordForm`, append `PushLit{V}` to `c.Form.Ops`.
  - `execMatch` (engine.go:807-946) post-match: when `c.RecordForm`, append `Call{Name: sig.Word, Arity: len(sig.Args)}`. Forward args were already evaluated/pushed before the call resolves (per the bytecode-report architecture), so their `PushLit` Ops are already in `c.Form.Ops` in sig order.
  - Quoted bodies: when a `NoEvalArgs` position consumes a list literal, recursively run the check pass on its body with the same `RecordForm` flag and wrap the result as `Quote{Body: subForm}`.

**Public entry point** (in `eng/go/stackform/compile.go`):
```go
// Compile runs the check pass over tokens with stack-form recording
// enabled and returns the resulting form plus any diagnostics. It
// does not mutate the supplied registry (clones, like CheckMode does
// today).
func Compile(reg *eng.Registry, tokens []eng.Value) (*StackForm, []eng.CheckDiagnostic, error)
```

`Compile` is the canonical way for any consumer to produce stack form. The PBT shrinker (Stage 4-5), the formatter, the future bytecode encoder all call this. There is no separate "lowering pass" — there is only one check pass with optional recording.

**Why this is clean**:
- **One matcher.** `matchSignature` is unchanged; it is the single source of truth for wiring decisions. Both dispatch and form-recording observe its output.
- **One walker.** Adding a separate lowering pass would re-walk the source and risk divergence from the check pass's behaviour (eg quoted-body resolution, type-driven dispatch, paren handling). Folding into check mode eliminates that risk.
- **Zero overhead when off.** `RecordForm == false` branches around every emit; runtime cost matches today's check pass.
- **Re-uses existing carrier-mode forward-arg resolution.** Per `aql-bytecode-report.0.md:101-117`: by the time `execMatch` fires for a call site, forward args have already been pushed as carriers in sig order via `rearrangeForForward`. So `Call{Name, Arity}` correctly references the most recent N pushes regardless of surface form.

**Tests**: `eng/go/stackform/stackform_test.go` exercises forward-arg / swap-form / stack-only / `/s` / `/f` / `BarrierPos`-mid cases (mirroring `lang/go/CLAUDE.md` "The unified algorithm" table) and asserts the recorded ops match expected golden forms.

### 0c — Eval, pretty-print, and output-equivalence tests (~220 LOC)

**Files** (under `eng/go/stackform/`):
- `serialise.go` — `Flatten(*StackForm) []eng.Value` re-serialises stack form to a flat token sequence that runs in pure strict-stack order. Because the form is already strict-stack, this is a direct walk: `PushLit` → emit the value; `Call` → emit the word name (which the engine will then run in stack-only mode); `Quote` → emit a list-literal containing the recursively flattened body.
- `print.go` — `Pretty(*StackForm) string` produces readable AQL. The printer is purely cosmetic; the stack form itself stays strict-stack.
- `eval.go` — `Eval(form *StackForm, reg *eng.Registry) ([]eng.Value, error)` is a thin convenience wrapper: `Flatten` → `native.New(reg).Run`. Useful for callers like the shrinker that want "run this form and tell me the result" without manual token splicing.

**Output-equivalence tests** — the hard correctness gate (`stackform/equivalence_test.go`):

For a broad fixture corpus (drawn from `lang/go/CLAUDE.md` ordering table, dot-access sweeps, quoted/lambda patterns, and a sample of `lang/spec/*.tsv` lines), assert:

1. **Round-trip equivalence**: running `src` directly through `native.New(reg).Run(src)` produces a final stack `S1`; running `Eval(Compile(reg, src).Form, reg)` produces `S2`; `eng.DeepEqual(S1, S2)` holds.
2. **Recording idempotence**: `Compile(reg, Pretty(Compile(reg, src).Form))` yields an equal stack form (same Op sequence) and an equal eval result.
3. **No surface-form leakage**: no `Forward`/`Move`/`Mark`-flavoured markers survive into `StackForm.Ops`.
4. **Check-pass behavioural equivalence**: running check mode with `RecordForm = false` produces the same diagnostics and fn summaries as today. Recording is purely additive.

This corpus is the foundation everything else in this PR (and the future bytecode VM) relies on. If equivalence holds, downstream stages compose with confidence.

### 0d — Documentation cleanup: stale `expandDottedWord` references (~30 LOC)

**The problem** (exploration confirmed):
- A function named `expandDottedWord` does not exist in the codebase. Dot tokens (`#DT`, registered in `eng/go/parser/grammar.go:37`) are converted to `Word("get")` during top-level value conversion at `eng/go/parser/parse.go:173`. Chained access `m.a.b` becomes the token sequence `m get a get b` and is executed as two sequential `get` calls — no parser-time "expansion" pass exists. The removal is documented at `parse_test.go:1069`.
- Two artifacts still claim otherwise:

**Files modified**:
- `lang/go/CLAUDE.md:178` — currently reads `"expandDottedWord() — transforms foo.a.b into ( foo get a get b )"`. Replace with accurate text: dot tokens (`#DT`) are lexed by jsonic and converted to `Word("get")` during top-level value conversion in `parse.go::convertTopLevelValue`; chained access composes naturally because each `get` call produces the receiver for the next.
- `design/AQL-DX-REPORT.5.md` (~lines 48, 182) — replace "parser's dot expansion emits it as a word" / "change the dot expansion to emit string literal keys" with precise wording: the dot-to-word conversion in `parse.go::convertTopLevelValue` emits `Word("get")` followed by the next token as-is (a Word, hence the shadowing concern); the proposed fix is to emit the key as a `String` literal rather than a `Word`, at that conversion site (not via any imagined expansion pass).

This is a small textual cleanup but worth doing in Stage 0 so the PBT design documents that follow can cite an accurate kernel reference. It also removes a future trip-hazard: anyone reading `CLAUDE.md` and searching for `expandDottedWord` will currently get zero results and waste time figuring out what changed.

---

## Stage 1 — `aql:rand` (foundation, ~400 LOC)

Seeded deterministic randomness. Standalone module — useful beyond testing (demo data, sampling, Monte Carlo).

**New files**
- `lang/go/modules/rand.go` — module builder + Go natives.
- `lang/go/modules/rand_test.go`.

**State**: `*rand.Rand` keyed under `capRandRng` on the parent registry, lazily initialised (default seed = 1 so runs are reproducible). Pattern mirrors `testRun` in `modules/test.go:18-25` and `activeRun()` at lines 183-190.

**Words** (all in the `rand.` namespace):
| Word              | Sig                              | Notes                                |
|-------------------|----------------------------------|--------------------------------------|
| `rand.seed`       | `Integer →`                      | Re-seed the PRNG.                    |
| `rand.int`        | `min max → Integer`              | Uniform inclusive.                   |
| `rand.bool`       | `→ Boolean`                      |                                      |
| `rand.string`     | `charset:String len:Integer → String` | Pick chars from charset.         |
| `rand.one-of`     | `List → Any`                     | Uniform element.                     |
| `rand.frequency`  | `[[w1 v1] [w2 v2] …] → Any`      | Weighted choice.                     |
| `rand.list-of`    | `gen:List len:Integer → List`    | Run the quoted generator `len` times. |
| `rand.map-from`   | `schema:Map → Map`               | For each key, eval its quoted gen.   |

All sigs use `BarrierPos: -1` per the module-wrapper-dispatch rule (`lang/go/CLAUDE.md` "Module FnDef Wrappers"). Generator bodies are passed as quoted lists and executed via `native.New(r).Run(...)` (the same pattern as `eachHandler` in `native/native_array.go`).

Add `"rand": BuildRandModule` to the modules map in `lang/go/modules/modules.go:23-33`.

---

## Stage 2 — AQL-G policy profile (~60 LOC)

The pure profile from the report §7. Used to evaluate generator programs and shrink candidates safely.

**New file**: `lang/go/policy/profiles/gen.jsonic`

Starting from `compute.jsonic` (allows math only): also allow `aql:rand` and `aql:decision` (for testing decision properties), keep `mutate` allowed (for `def` local bindings inside generator bodies), tighten `maxStepBudget` to `200000`, leave `clock` denied.

**Rationale for limits**: the reducer re-evaluates a generator program ~1000 times during candidate exploration; each must terminate quickly. Step-budget is the natural fuel mechanism — already wired through `r.Check.StepBudget` in eng.

Documented in `design/NATIVE-MODULES.10.md` under a new "Profiles" subsection.

---

## Stage 3 — `PropertySpec` + `test.check-prop` (~300 LOC, no shrinking yet)

Property API in `aql:test`, value-loop only. Confirms generators and properties wire up correctly before we build the reducer.

**Files modified**
- `lang/go/modules/test.go` — extend the AQL preamble + add Go natives.

**Preamble additions** (Record types alongside existing TestCase/TestSet):
```
def PropertySpec refine Record [
  name:String
  gen:List          # quoted generator body — leaves one value on the stack
  property:List     # quoted predicate body — takes value, leaves Boolean
  runs:Integer      # default 100
  seed:Integer      # default 1
  max-shrinks:Integer  # default 200 (used in Stage 5)
]
def PropertyResult refine Record [
  name:String
  ok:Boolean
  runs:Integer
  failing-input:Any
  shrunk-input:Any
  shrunk-source:String
  shrunk-cost:Integer
  error:Any
]
```

**New Go natives** (in `testNatives()` at `modules/test.go:195`):
- `test-check-prop` — sig `[String List List Integer Integer Integer]` (name, gen-body, prop-body, runs, seed, max-shrinks). Loops `runs` times: seed RNG (via the rand module's capability), run gen-body in a sub-engine under `aql:gen` policy (reuse the `runInSubEngine` pattern from `modules/vm.go:167-202`), take top of stack as the generated value, push it and run prop-body, expect a Boolean on top. On failure, record `failing-input` and (in this stage) leave shrunk fields as None. Record into the active `testRun` via the existing `runCase` path (lines 462-490).

**AQL spec runner extension** (in the preamble): add a `run-property` fn that takes a `PropertySpec` Map and calls `test-check-prop` with its fields. Extend `run-spec` so a TestSpec whose `subs` contain PropertySpec values dispatches to `run-property` (discriminate by checking for the `gen` key — or add a tag field). For schema cleanliness, prefer renaming TestSpec's `subs:List` semantically to "child specs of any shape (TestSpec or PropertySpec)".

**Exports** (extending lines 706-738):
- `PropertySpec`, `PropertyResult`, `check-prop`, `prop` (constructor), `run-property`.

Add inline imperative form too: `test.check "name" [gen] [property]` with sensible defaults.

---

## Stage 4 — PBT-specific stack-form extensions (~200 LOC)

With the stack form, `Compile`, `Eval`, and `Pretty` all shipped in Stage 0, this stage adds only what is PBT-specific: word transparency annotations for the shrinker and PBT-tuned cost weights.

**Files**
- `lang/go/modules/test/shrink/policy.go` — per-word transparency annotations (Transparent / Opaque / Generator / Frozen, report §8). Initial table: arithmetic/literal/list-construction = Transparent; `rand.*` = Generator; `time.*`/`fetch.*` = Frozen; unknown user words default to Opaque.
- `lang/go/modules/test/shrink/cost.go` — `ShrinkCost(form *stackform.StackForm, policy *Policy) int`. Wraps `stackform.Cost` with policy adjustments per report §9 (eg Frozen words priced higher to discourage their rewriting).

`Compile`, `Eval`, `Pretty`, and base `Cost` are all consumed from `eng/go/stackform/` — Stage 4 does not re-implement them.

**Tests**: `shrink/policy_test.go` verifies the annotation table and cost weighting against the report §8/§9 reference values.

---

## Stage 5 — Reducer (~500 LOC)

The failure-preserving rewrite engine. Implements report §11-§14.

**Files** (under `lang/go/modules/test/shrink/`, alongside the policy/cost from Stage 4):
- `shrink/reduce.go` — main reducer loop (greedy descent).
- `shrink/candidates.go` — candidate generators (one function per rewrite family).
- `shrink/reduce_test.go`.

**Algorithm** (close to report §11):
```go
func Reduce(initial Program, eval func(Program) Outcome, profile *Profile) Program {
    current := Normalize(initial, profile)
    cost   := Cost(current, profile)
    seen   := map[Fingerprint]bool{Fingerprint(current): true}

    for step := 0; step < profile.MaxSteps; step++ {
        cands := generateCandidates(current, profile)
        cands = sortByCost(cands)
        accepted := false
        for _, c := range cands {
            fp := Fingerprint(c)
            if seen[fp] { continue }
            seen[fp] = true
            if Cost(c, profile) >= cost { continue }
            if !validShape(c) { continue }
            if eval(c) == Fail {
                current, cost, accepted = c, Cost(c, profile), true
                break
            }
        }
        if !accepted { break }
    }
    return current
}
```

**Rewrite families** (report §14, ordered by priority):
1. Structural deletion (remove map keys, drop list elements, prune dead code) — biggest cost wins first.
2. Literal shrinking (`42→0`, `42→21`, `"abc"→"a"`, `true→false`).
3. List shrinking (slice halves, drop one element).
4. Map/Record shrinking (remove optional fields, shrink values).
5. Stack simplification (`dup drop→identity`, `quote x do→x` when safe).
6. Quotation shrinking (recurse into quoted bodies).
7. Generator-semantic rewrites (`rand.int min max → rand.int 0 1`, `rand.list-of g 10 → rand.list-of g 1`) — uses the word-policy table from Stage 4.

Phase 4 stretch (report §15-§16, **deferred**): bounded best-first search, exact small-program search. Add `TODO`-pinned hooks but don't implement.

**Integration with Stage 3**: extend `test-check-prop` to, on failure, call `stackform.Compile(reg, gen-body) → shrink.Reduce(form, evalFn, policy) → stackform.Pretty(reduced)`. The `evalFn` closure runs the reduced form under the AQL-G profile (`stackform.Eval` + sub-engine) and returns `Fail` iff the property predicate returns `false` on the resulting value (and `Invalid` on eval errors). `PropertyResult.shrunk-input`, `shrunk-source`, `shrunk-cost` populated.

---

## Stage 6 — Decision PBT spec (experimental subject, ~300 LOC)

The proof-of-life. Three properties that exercise non-trivial invariants:

**New files**
- `lang/go/modules/decision_pbt_spec.aql`.
- `lang/go/modules/decision_pbt_spec_test.go`.

**Properties**:
1. **`appending-non-matching-rule-is-stable`**: for hit-policy "first", appending a rule whose `when` is unreachable does not change `eval-table`'s output. Generator builds an `{input, table}` pair plus a known-unreachable extra rule. Expected to PASS — shakes out future regressions.
2. **`collect-policy-preserves-rule-order`**: for "collect", the result list's order equals the order of matching rules in the table. Generator builds tables where multiple rules match.
3. **`not-not-cond-equivalent-to-cond`** (De Morgan): `eval-pred(not-of (not-of c), input) == eval-pred(c, input)`. Predicate-level identity. Almost certainly will pass; included to demonstrate the shape.

Each property is a `PropertySpec` Record using `rand.*` generators. Expected output documented in the test: properties pass under 100 runs at seed 1. The Go test (`decision_pbt_spec_test.go`) loads the file, runs `spec test.run-spec`, and asserts no failures.

A negative-control test deliberately introduces a buggy property (e.g. asserts `not-of x = x`), runs it, and asserts that the shrinker reduces the failing input to a minimal counterexample (e.g. `{x:false}` rather than the original 5-key generated map). This is what proves end-to-end shrinking works.

---

## Verification

After each stage:
```bash
make fmt && make vet && make lint && make test
```

End-to-end smoke (after Stage 5):
```bash
cd lang/go && go test ./modules/ -run TestDecisionPBT -v
```

Expected output: every property in `decision_pbt_spec.aql` passes 100 runs at seed 1; the negative-control test shows shrinking reduces input cost by ≥80% before reporting.

REPL smoke check:
```bash
cd cmd/go/aql && go run . repl
> "aql:test" import
> "aql:rand" import
> "aql:report" import
> def my-prop {name:"reverse-twice" gen:[[1 10 rand.int] 5 rand.list-of] property:[reverse reverse args.0 deq] runs:50 seed:1 max-shrinks:50}
> my-prop test.run-property
> test.results report.table print
```

---

## File summary

**New**
- `eng/go/stackform/{stackform,compile,serialise,print,eval,cost,walk}.go` — Stage 0a/0b/0c kernel stack-form package.
- `eng/go/stackform/{stackform_test,equivalence_test}.go` + `eng/go/stackform/testdata/` — unit + output-equivalence tests.
- `lang/go/modules/rand.go`, `rand_test.go`.
- `lang/go/policy/profiles/gen.jsonic`.
- `lang/go/modules/test/shrink/{policy,cost,reduce,candidates}.go` + tests.
- `lang/go/modules/decision_pbt_spec.aql`, `decision_pbt_spec_test.go`.

**Modified**
- `eng/go/registry.go` (Stage 0b) — add `RecordForm bool` and `Form *stackform.StackForm` fields to `CheckState`. No behavioural change when `RecordForm == false`.
- `eng/go/engine.go` (Stage 0b) — add Op-emission at the literal-push site and at `execMatch` (around line 807-946); recursive recording for `NoEvalArgs` quoted bodies. Guarded by `c.RecordForm`.
- `lang/go/native/aliases.go` (Stage 0) — re-export the `stackform` package's public types (`StackForm`, `Op`, `Compile`, `Eval`, `Pretty`, `Cost`).
- `lang/go/CLAUDE.md:178` (Stage 0d) — replace the stale `expandDottedWord()` description with accurate dot-token-to-`Word("get")` conversion text.
- `design/AQL-DX-REPORT.5.md` (Stage 0d) — fix the "parser's dot expansion" language at the two affected sections.
- `design/aql-bytecode-report.0.md` (Stage 0 — small addition) — cross-reference note that the recording side-effect described in §1.2 is now implemented in `eng/go/stackform/` at the stack-form fidelity tier; the bytecode-encoder tier remains future work.
- `lang/go/modules/modules.go` — register `"rand"`.
- `lang/go/modules/test.go` — extend preamble + add `test-check-prop`, `test-run-property` natives + new exports.
- `design/NATIVE-MODULES.10.md` — document `aql:rand`, the `gen` profile, the PropertySpec API, and the new `eng/go/stackform/` package.

**Reused (do not duplicate)**
- `eng/go/engine.go::matchSignature` (line 2828) — the **only** source of signature-matching truth. Stage 0 hooks stack-form recording into the check pass at this call site; nothing in the plan re-implements matching. Re-implementing this logic is explicitly forbidden by the design.
- `eng/go/match.go::patternsOk` (line 22) — predicate sig-pattern checker, already used by `matchSignature`.
- `eng/go/check.go::CheckState.Begin` and `eng/go/engine.go::Engine.Run` (check-mode path) — Stage 0 reuses this entire walker as the stack-form emitter, adding only the `RecordForm` switch and Op-append calls at existing dispatch points. The architecture is exactly the one outlined in `design/aql-bytecode-report.0.md:61-74`.
- `lang/go/modules/vm.go:167-202` `runInSubEngine` pattern for evaluating generator programs under the AQL-G profile.
- `lang/go/modules/test.go:183-190` `activeRun` capability pattern.
- `lang/go/modules/test.go:544-560` `invokeSubject` for property predicate dispatch.
- `eng/go/registry.go::CallAQL` (already used by the test runner) for invoking fn-shaped generators.
- `eng/go/compare.go::DeepEqual` / `ExactEqual` for fingerprinting candidates.

---

## Out of scope (this PR)

- Best-first search and exact small-program search (report §15-§16).
- Compression rewrites that synthesise `iota`/`repeat`/`range` from literal lists (report §14.9).
- Test-case minimisation across multiple failing runs (delta-debugging across properties).
- Shrinking that crosses module boundaries (eg shrinking calls into user-defined fns).

These should be follow-ups once the Stage-1-through-5 core proves itself on the decision spec.
