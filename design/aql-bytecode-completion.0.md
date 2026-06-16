# AQL Bytecode — Completion Roadmap (toward P7)

Status: design. Companion to `aql-bytecode-plan.0.md` (P0–P4) and
`aql-bytecode-runtime-independence.0.md` (P5–P7 machinery). This doc
**reconciles the plan with the live state** after the P5/identity/predicate/
case/fn-value items landed and the A–F robustness hardening, then maps the
**remaining** work to the goal with a grounded cluster census and a
**re-scoped P7** that is actually reachable.

## 1. Status reconciliation

The runtime-independence doc's "Recommended sequencing" marks items 1–6 DONE,
but the ratchets stood at **459 refused / 15 islanded** (from 651 / 115 at P0)
when this doc was written. Those items each cleared their *named* cluster; what
remained is a **long tail** the sequencing section never enumerated, which this
doc enumerates. As of the latest landings (items 1–6, 8, and lambda higher-order
args) the ratchets stand at **399 refused / 9 islanded**.

Robustness (separate from coverage) was hardened since: RunCompiled falls back
on `internal_error`/foreign errors, the VM has a panic guard + concurrency
guard, loop fixed-point rounds no longer re-record, and `for` bounds agree
across engines. None of that moves the ratchets; it makes the *fallback* and
*VM* safe, which matters because P7 deletes one and leans entirely on the other.

## 2. The live refusal census (459) + islands (15)

Measured by bucketing every `.tsv` value row through `CompileCheck` and
tallying the offending word (June 2026). The drivers, not the buckets, are
what the roadmap targets:

| Cluster (driver words) | ~rows | Kind |
|---|---|---|
| `make` class with typed-instance field defaults (`class {x:(make Foo 1)}`) | 32 | **compute** |
| `get`/`is`/`typeof` over class/object instances & dynamic receivers | ~63 | **compute** |
| `set` on object/class instances (field mutation) | 24 | **compute** |
| `filter`/`fold`/`each` with lambda (`=>`) args + map iteration | ~22 (+7 islands) | **compute** |
| `select` query DSL bodies | 20 | compute/DSL |
| 3-arg operand shape (`StructUtil.setpath recv k v`) | 7 | **compute** (lowering) |
| computed-else / variadic-statement `if` | 13 | **compute** (lowering) |
| poly type-algebra over predicates (`tnot`/`tand`/`tor`) | ~12 | **compute** |
| introspection over fn-values (`typeof (f/r)`, `tcmp Function`) | 12 | compute (follow-on) |
| 0-return fn bound to `def` (`def r (f 1)`) | few | **compute** (lowering) |
| count-mismatch / statement-`if`-in-fn return rows | 7 | error rows |
| **`Vm.run`/`run-with`** (runs runtime source in a sub-engine) | ~3 | **META** |
| **Test harness** (`test-test`/`test-prop`/`test-check-prop`/`test-skip`) | ~16 | **META** |
| **`flex`/`canon`/`macroexpand`/`minilang-register`** (reflective) | ~12 | **META** |
| **`codequote`/`quote`** (code-as-data) | ~5 | **META** |
| **usurp-modified fns** (`f/u`, `f/ur`, `f/us` — re-step, tape-coupled) | ~15 | **META** |
| **`args.N`** inside a fn body (frame keeps locals, not an args stack) | 2 | **META** |
| **suppressed-runtime-error** rows (`unpack` missing key, orphan `gen [T]`) | 5 | error rows |
| `reach` `$.path` lenses | 6 | compute (niche) |

Islands (15): map iteration (`each`/`fold` over `{…}` → ~7), a few `case`
shapes whose value isn't re-pushable (~5), multi-result `do` (1), `each [drop]`
(1).

## 3. P7 reachability — what is truly irreducible (and what only looks it)

`aql-bytecode-runtime-independence.0.md` gates P7 (delete `OpFallback` + the
whole-program fallback) on **both ratchets at 0** — every row compiles. Exactly
ONE narrow thing makes that unreachable, and it is worth stating precisely,
because an earlier draft of this doc overstated it.

A bytecode compiler is an ahead-of-time function `compile : Program ->
Instructions`, run once before execution. The only genuinely irreducible words
are those that **execute code computed at RUNTIME** — `Vm.run`/`run-with` of a
runtime string (`lang/go/modules/vm.go`). For those there is no static program
to emit; "compiling" them is parse+compile+run *at runtime*, which is exactly
what the `OpFallback` island (the embedded interpreter) does. That, and only
that, is irreducible — and even then it is not magic, just the bytecode invoking
the parser/VM on data.

**Everything else the corpus refuses is REDUCIBLE** — refused by a specific,
nameable limitation of *this* compiler/VM, not by any law:

- `macroexpand` — ✅ NOW COMPILED (static cases). Lisp-style: the macro + args
  are static, so the expansion is a compile-time computation — carrierResults
  runs it and bakes the token list as a code-as-data const (a Word is admitted as
  a const MEMBER; a bare type node is NOT, to dodge the canonical-`*Type` hazard).
  `macroexpand (twice 5)` → `[5 word(add) 5]`.
- `word` (Forth splice) — ✅ NOW COMPILED. The __SP marker is preserved through
  check mode and the existing stepLiteral splice expands the body inline at each
  use site, so the instructions land in place. Late binding falls straight out of
  the normal `def c1 10 … def c1 20` sequence the VM already runs:
  `def c1 10 def x word [c1 2] def c1 20 x` → 20 2 (re-resolved at the USE site).
  Nothing was ever frozen — the compiler just used to treat `word` as opaque.
- `args.N` — ✅ NOW COMPILED. It was billed "context-dependent, needs a per-call
  args stack the VM frame doesn't keep"; in fact the params ARE the frame's
  leading locals, so `args.N` lowers to a plain `PUSH_LOCAL N`. AnalyseFnBody
  projects the params as the args list and `tryFoldStaticIndex` folds
  `get N args` to the local (carrier.go). The compiled body is byte-identical to
  the named-param form — the concrete proof that "tier 2" is reducible work, not
  irreducibility.
- `flex`/`canon`/`usurp`/`quote`/`Test`/`minilang` — reference cells, a pure
  canonicaliser, a VM dispatch model for usurp, constant token-list bakes,
  compiling candidate bodies. All named VM/compiler features.

So the honest re-scope keeps a fallback ONLY for runtime code-eval, and treats
the rest as work:

> **Native execution of all reducible code; an OpFallback island confined to the
> one irreducible category — words that execute runtime-computed code.**

### Re-scoped P7 gate — three tiers (replaces "both ratchets == 0")

**✅ LANDED** (`test/go/langspec/compiled_metafallback_test.go`,
`TestOnlyMetaFallsBack`). The gate partitions every refused/islanded spec value
row into three tiers plus error-rows, each on its own ratchet. Current partition
(June 2026): **2 interpreter-only (tier 1), 96 reducible (tier 2), 12
error-row, 258 compute gap**.

1. ✅ **Tier 1 — `interpreterOnlyWords` (permanent, capped at 3).** Executes
   runtime-constructed code: `Vm.run`/`Vm.run-with`. The legitimate, permanent
   home of the island. A NEW tier-1 entry is an *irreducibility claim* the gate
   forces you to justify.
2. ✅ **Tier 2 — `reducibleWords` (ratcheted, `reducibleCeiling = 96`).**
   Refused by a NAMED missing feature, each `why` recording what compiling it
   takes: usurp 43, Test/Assert 28, quote 10, flex 7, minilang 5, word 3 (residual
   splices). These are TODOs, not exclusions — they ratchet to 0 like any other
   work. (The earlier draft mislabeled this tier "irreducible meta"; that
   laundered unfinished work as impossibility. `args.N`, the 30-row `word` macro
   splice, `macroexpand`, and `with-decimal` disproved the framing concretely —
   each moved OUT of this tier to native code.)
3. ✅ **Compute frontier (ratcheted, `computeRefusalCeiling = 258`).** Cascades
   (operand-provenance 140), code-body DSL bodies (47), Stage-1 lowering (~24),
   dynamic in/out (~24), 9 islands, user-fn dispatch (5). Error rows (12) are
   allowlisted via `errorRowReason` (the checker refuses so the interpreter
   raises the taxonomy; making the VM raise them stays available).
4. **PENDING** (gated on tiers 2 + 3 reaching 0): the §P7 deletion of the
   *unbounded* whole-program fallback — `RunCompiled` stops calling `a.Run(src)`
   for arbitrary refusals and runs the compiled `Program`, whose `OpFallback`
   island is then provably confined to tier-1 spans. The island machinery stays.

This is "complete delivery": all reducible code native; the island confined to
runtime code-eval. The gate MEASURES the distance (96 reducible + 258 compute)
instead of asserting an impossible absolute — and keeps tier 2 honestly on the
work list rather than excused as "meta."

## 4. Sequenced roadmap (tractable clusters)

Ordered by leverage ÷ risk. Each lands gate-clean (the §6 discipline) and
lowers `refusalCeiling`/`islandCeiling` monotonically; commit each with its
before/after numbers.

1. **3-arg operand shape (lowering gap, LOW risk). ✅ LANDED (459 → 453).**
   `layoutOperands`' single-result case now seats a sig-0 computed result over
   const operands with a push+swap chain (`setpath (make…) k v`); n>2 chains the
   swap, no new opcode/local. The companion "0-return fn bound to def" was
   *dropped*: those rows are the empty-body fn case the runtime-independence doc
   deliberately leaves refused (check-mode `[Any]`), not a lowering gap.
   (`eng/go/lower.go`.)

2. **Object/class `set` field mutation. ✅ LANDED (445 → 425).** `set k v inst`
   on an Object/Class instance refused only because the atom-keyed overload
   carries a QuoteArgs key (`p set x 7`). Exempted `set` from the
   quoted-operand refusal alongside get/getr: the key is an inert Atom const,
   the receiver is a non-const instance (mutation-safe — instance types are
   absent from `isInertConst`, exactly as the integer-keyed `set 1 v arr` that
   already compiled relies on), and `set` cannot be shadowed (builtin), so the
   word-name match admits only the real mutator. Sealed-field / out-of-bounds
   error rows raise the same taxonomy in both engines. The integer-keyed array
   set (incl. aliasing `def b a set 0 9 a b`) already compiled. Cleared the 24
   set rows; the residual 13 quoted-operand refusals are meta
   (minilang/codequote/quote/timeout). (`eng/go/emit.go` RecordCall.)

3. **`make` class with typed-instance field defaults — DEFERRED (5 rows, HIGH
   risk, multi-blocker).** A class body `{x:(make Foo 1)}` whose default is a
   user-type instance. Scoped + attempted June 2026; reverted. Findings:
   - Only **5 spec rows** (`user-types.tsv` §, `class.tsv` lines 79–81 refine
     defaults + 114–115 class-instance defaults that pin per-instance COPY).
   - NOT a provenance gap: `ReturnsFreshInstance` returns a bare type CARRIER in
     check mode (`NewCarrier(t)`), so `make Foo 1` has no concrete value to
     bake/remember. The carrier-identity work deliberately made it so.
   - **Blocker 1 (bake) — solvable.** A `rememberConcreteMake` hook (run the
     pure-data constructor for real in check mode, remember the instance against
     the carrier id) + a `materialise` ObjectTypeInfo-rebuild makes the class
     body bake. Verified: the refusal moves PAST the make operand.
   - **Blocker 2 (recording) — high blast radius.** The inner `make Foo 1`,
     evaluated during the `def S` body, records an event whose result is buried
     in the baked schema (not on the runtime stack), so Finalize hits "residual
     shape … call results reordered." Fixing it means suppressing recording
     during class-body default evaluation — a change to the `def`/`autoEvalMap`
     recording path that also governs `def x (computed) x` and every other
     def-body dispatch. Too dangerous for 5 rows.
   - **Blocker 3 (mutation).** The class-instance rows additionally need
     `typeBodyConstOK` to admit a concrete instance as a type-body member
     (sound — type bodies are schemas `make` copies per instance, never mutated
     — but a fourth coordinated change).

   Verdict: revisit only as a dedicated multi-step effort once a clean
   "suppress recording inside a const-baked schema construction" primitive
   exists; the leverage (5 rows) does not justify it ahead of items 5/6.
   (`eng/go/emit.go`, `eng/go/carrier.go`.)

4. **Poly type-algebra over predicates. ✅ LANDED (453 → 445).** SCOPE
   CORRECTION: the billed "~75 get/is/typeof over instances" turned out to be
   mostly ALREADY COMPILING (simple `(make S {}) get x`, `is S`, `typeof` all
   lower today) plus CASCADES from items 3 (typed-field defaults) and 5
   (closures) — those `get`/`is`/`typeof` rows refuse because their *receiver*
   is unproducible, so they clear when 3/5 land, not as independent item-4 work.
   The genuine independent item-4 content was the ~8 "polymorphic dispatch"
   rows: a STRICT-disjunct straddle (`5 is (tnot (Integer gt 0))`,
   `Integer tand (tnot …)`) that `disjunctPartitionReturns` refused via
   `RecordPoly`. `tryRecordPoly` now takes a `disjunctStraddle` flag that
   bypasses only its dynamic-only gate (every other safety gate stays), so a
   safe-builtin straddle lowers to `OpCallNativePoly` — the VM re-matches the
   one concrete runtime alternative. Also flipped the obsolete
   `TestEmitRefusesPolySite` (the site now compiles) to assert the poly
   lowering + parity. (`eng/go/carrier.go`.)

5. **Lambda higher-order args + map iteration ✅ FULLY LANDED. Part A — map
   iteration (islands 15 → 9); Part B — lambda args (459-era ~22 → cleared the 8
   directly-attributable rows, 407 → 399; the rest were get/is/typeof cascades).**
   - Part A — `each`/`fold`/`filter` over a MAP islanded because the token-body
     map path ran the body via `runQuotationBody` (`New(reg).Run`), bypassing
     the InvokeBody seam the list path uses. `newMapBody` now detects a compiled
     CLOSURE (`IsCompiledClosure`) and routes it per VALUE through `InvokeBody`
     (token/lambda paths unchanged — interpreter untouched), and
     `DataListElemTypeFromValue` returns the map's common VALUE type so a
     value-body closure compiles. Map iteration is now native; the islanded rows
     were already `wasCompiled=true`, so this moves the ISLAND ceiling, not the
     differential count. (`eng/go/bytecode.go` IsCompiledClosure,
     `eng/go/carrier.go` DataListElemTypeFromValue, `native/native_map_iter.go`.)
   - Part B — a lambda VALUE arg (`filter ([p] => …) data`). The earlier attempt
     reverted because it sought a UNIFORM closure, but each higher-order word has
     its OWN callback convention. The fix is per-word callback unification:
     compile the afn body against the WORD'S callback input SHAPE, and route a
     compiled closure through that handler's existing shape:
       - `filter` (list) → a `{key, value}` pair Map (element via `.value`).
       - `filter`/`each` (map) → a KeyVal {k v i n} (value via `.v`).
       - `fold` (init) / `scan` (map) → (accumulator, KeyVal).
     `tryRecordClosure` detects an FnDefInfo body and routes to
     `tryRecordLambdaClosure`, which builds the representative-carrier inputs
     (`lambdaCallbackInputs`) and compiles the body with the lambda's NAMED
     params bound to them (AnalyseFnBody's def path) — so `p.value`/`kv.v`
     typechecks where the declared `Any`/`KeyVal` param alone could not. The
     filter handlers build their own fixed shape, so they need no metadata; the
     map-iteration handler is shared between the token (bare value) and lambda
     (KeyVal) forms, so the input shape is recorded on the unit
     (`ClosureInShape`, copied onto the ClosurePayload at OpPushClosure) and the
     handler reads it back (`ClosureWantsKeyVal`). Capturing lambdas, list
     each/fold, no-init map fold, and `for-each` (0-result/1-output mismatch)
     stay refused — conservative, gate-clean. (`eng/go/callable_words.go`,
     `eng/go/bytecode.go`, `eng/go/emit.go`, `eng/go/vm.go`,
     `native/filter.go`, `native/native_map_iter.go`. Two commits, each
     gate-clean: filter then each/fold/scan.)

6. **if-branch lowering ✅ FULLY LANDED (bucket 13 → 0). 6a computed-else
   (425 → 421); 6b variadic-else (421 → 417).**
   - 6a — `if cond [then] (expr)` where the else is an eagerly-evaluated paren
     result on the stack: a new `OpDrop` + a `lowerArmsComputed` path SWAPs the
     cond above the eager else value, `JMP_IF_FALSE`, and on the taken path DROPs
     the else before the then-body (the false path keeps it as the result). Both
     arms net one value → a single (non-variadic) merge.
   - 6b — a statement-`if` where exactly one arm nets a value and the other nets
     0 WITHOUT diverging (`if c [99] []`, `if c [] [99]`, `if c [raise] [99]`):
     `resolveArm` now allows a non-diverging 0-value arm, and `lowerArms` marks
     the merge VARIADIC (0-or-1, residual-only) when the arms' counts mismatch
     and the empty arm doesn't diverge (a diverging 0-arm leaves, so the
     surviving value is unconditional — non-variadic). The raise-guard errors on
     its taken path in both engines.
   (`eng/go/bytecode.go` OpDrop, `eng/go/vm.go`, `eng/go/emit.go` RecordBranch,
   `eng/go/lower.go` lowerArms/lowerArmsComputed.)

7. **`select` query DSL ✅ LANDED (310 → 283); `reach` lenses ✅ LANDED
   (283 → 268).**
   The `aql:query` words are trivial-delegation module wrappers over inner
   natives, so they compiled the moment their two operand shapes did:
   - **Clause words** (select/where/order/group/having/limit/offset/distinct/on/
     using) carry `NoEvalArgs` whose clause is an inert word-list (`[name age]`,
     `[age gt 1]`). `noEvalBodiesInert` now admits a `NoEvalArgs` body that is an
     `isInertConst` (a Word-member list bakes as a code-as-data const), so the
     wrapper records the inner native as a plain `CALL_NATIVE`. Flow-control
     sentinel bodies (`each [break]`) stay refused (`bodyHasSentinel`): baking
     `[break]` + a `CALL_NATIVE` cannot carry the break across the call boundary.
   - **Source words** (from/join/innerjoin/leftjoin/crossjoin) carry a
     `QuoteArgs` table-NAME atom. `quoteOperandInertOK` is the principled
     extension of the get/getr/set quoted-operand exemption: a MODULE INNER
     native (confirmed by `isModuleInnerSig` pointer identity) whose quoted
     operands are all inert Atom consts bakes a plain `CALL_NATIVE`. It is gated
     on module-inner so it never leaks to the core meta quoted words (usurp /
     force-arity / ref-family / inspect / has), which keep refusing.
   The interpreter reaches the same inner native through the wrapper's trivial
   delegation, so the lazy-query value is built identically — differential 0
   mismatches across the family.
   (`eng/go/emit.go` RecordCall `noEvalBodiesInert`, `eng/go/carrier.go`
   `quoteOperandInertOK`; `lang/go/bytecode_findings_test.go`
   `TestQueryDSLCompilesNative`.)

   **`reach` lenses** then split the same way. A RECEIVERLESS reach (`$.name`,
   `$.a.b`, `$!.x`, `$.1`) is an INERT first-class lens — it evaluates to itself,
   `typeof` reads `Reach`, and `apply`/`getpath`/`setpath` walk its segments
   against a FRESH receiver. `isInertReach` admits exactly that shape to
   `isInertConst` (Eval=false, no Receiver, all-literal-key segments), so the
   lens bakes into the const pool like an atom — the opposite of a dot-access
   EVAL reach (`m.a.b`), which `expandReach` lowers to a get-chain IN PLACE and
   which `isInertConst` rightly still excludes. That single const-bake unblocks
   the lens VALUE, `typeof`, `apply`, `rebind`, and StructUtil `getpath`/`setpath`
   forms as plain const-operand CALL_NATIVEs (the keys are Words/Atoms/scalars,
   no canonical-*Type hazard). The HIGHER-ORDER lens forms (`each $.name people`,
   `filter $.on data`, `ArrayUtil.sortby $.age people`) stay refused on an
   ORTHOGONAL limit — a data list's interior strings are carrier-stripped in
   check mode (the same reason `size people` over string data refuses), so the
   list is not a bakeable const; `tryRecordFallback` declines to island those
   (a reach is data, not a code body) so they refuse cleanly instead of
   regressing `islandCeiling`. (`eng/go/emit.go` `isInertReach`,
   `eng/go/carrier.go` `tryRecordFallback` lens guard;
   `lang/go/bytecode_findings_test.go` `TestReachLensCompilesNative`.)

8. **introspection over fn-values ✅ LANDED (417 → 407).** A type-READING word
   (`typeof`/`tcmp`/`teq`/`tand`/`tor`/`tnot`/`inspect`) over a fn VALUE bakes
   the immutable fn as a const operand the handler inspects (`typeof (f/r)`,
   `Positive tcmp Function`): `fnIntrospectionWords` exempts them from the
   "function value reaches word" refusal, and a concrete fn-value arg interns as
   a never-pooled const (CanonValue is not a reliable fn identity key).
   Fn-INVOKING words stay refused — apply, the higher-order forms, and crucially
   `is` over a predicate fn (whose handler APPLIES the predicate; the VM cannot
   re-step a fn body) — so they are deliberately OFF the allowlist.
   (`eng/go/emit.go` RecordCall + intern.)

9. **Error-row disposition + the re-scoped P7 gate ✅ GATE LANDED (three-tier);
   fallback narrowing PENDING.** `TestOnlyMetaFallsBack` partitions every
   refused/islanded row into tier 1 interpreter-only (2, capped), tier 2
   reducible (61, `reducibleCeiling`), error-row (12, allowlisted), and compute
   gap (200, `computeRefusalCeiling`). Only tier 1 (`Vm.run`) is a permanent
   island; tiers 2 and 3 both ratchet to 0. `args.N`, `with-decimal`, `word`,
   `macroexpand`, `usurp`, the query DSL and the reach lens-as-const forms are
   the tier-2/frontier reductions proving the ratchet bites. Error rows are
   allowlisted (the checker refuses so the interpreter surfaces the taxonomy);
   making the VM raise them stays available as future tightening. The §P7
   fallback narrowing (step 4) waits on the compute gap reaching 0 — its
   remaining drivers are the operand-provenance cascades (117, clear when their
   producers compile: item-3 class instances, user-fn results), the residual
   code-body DSL words (8: `with-decimal` / follow-ons — `select` / `where` /
   `group` / `case` now compile), the higher-order lens/data forms blocked on
   list-element carrier-stripping, Stage-1 lowering residuals (~19), dynamic
   in/out (~34), the 7 islands, and user-fn dispatch (5).
   (`test/go/langspec/compiled_metafallback_test.go`.)

10. **Scalar carrier-keep + carrier-identity de-collision ✅ LANDED
    (283 → 258).** Two coupled runtime-independence steps that clear a slab of
    the operand-provenance cascade:
    - **Scalar carrier-keep.** `toCarrier` kept only concrete INTEGERS concrete
      through check mode (for static index checking); every other scalar
      stripped to a type-only carrier. So a DATA list/map whose interior is a
      string/bool/temporal stripped to `[ProperString …]` and was no longer an
      inert const — `size people`, `each $.name people`, `ArrayUtil.sortby
      $.age people`, and a class type body with a `r:1.0` default all refused
      "operand of unknown provenance". Keeping every concrete inert SCALAR
      concrete (string/bool/float/atom/big-int/decimal/temporal, DepScalar
      CONSTRAINTS still excluded — their payload is a DepScalarInfo) makes the
      container bake. It also closed a latent hazard: the generic-over-class
      `r:Float`-vs-`r:1.0` type-body taint is now faithfully `r:1.0`.
    - **Carrier-identity de-collision.** Keeping the scalar key concrete made a
      repeated identical computed call deterministic: `(context get 'n') add
      (context get 'n')` issues two `get` events whose results now share an id.
      The generic RecordCall "already recorded → a structured hook owns it"
      skip then mis-fired on the SECOND get (orphaning its receiver push), and
      even past the skip the shared id let `add` resolve both operands to one
      event ("call results reordered"). Fixed in its targeted form: the skip is
      gated on the producer being a STRUCTURED hook (RecordBranch / user-fn /
      poly / closure / loop / fallback — tracked by `genericSeq`), so a prior
      GENERIC collision falls through; and the output loop mints a fresh id for
      an output that collides with a prior event (guarded against `dup`/`swap`
      identity pass-through: same-event and out-id-in-args are skipped). The
      all-rows diff confirmed 0 native→non-native regressions, +12 improvements.
    Ratchets: refused 283 → 258, minCompiledRows 1996 → 2007, compute gap 200 →
    190; interp dropped 2 → 1 (a former `Vm.run (canon …)` row compiled),
    rebalancing tier-2 to 62. (`eng/go/carrier.go` toCarrier scalar-keep,
    `eng/go/emit.go` RecordCall genericSeq gate + de-collision;
    `lang/go/bytecode_findings_test.go` `TestScalarKeepAndCarrierIdentity`.)

11. **Module-synthetic const-fold ✅ LANDED (258 → 227).** `import` binds an
    IMMUTABLE, deterministic Module / ModuleExport instance, so a pure read of
    one is a compile-time constant — `MathUtil.$name`, `X.$module.name` / `.kind`
    / `.exports`, `convert Map/List Foo`, `typeof` / `is` over a Module. They
    refused "operand of unknown provenance at get/convert": the module value is
    not an inert const, so it has no compiled operand home. `tryFoldModuleConst`
    folds them. The subtlety that makes it sound: the checker's RECORDED result
    is the declared TYPE (a `Map` / `Boolean` carrier), not the value — baking
    that renders `Map` where the interpreter rebuilds `{a:1 b:2}` (a real bug the
    first cut hit). So the fold RE-EVALUATES the dispatch concretely off the emit
    path (`concreteHandlerEval`: check mode off, the def stack snapshotted, run
    TWICE and only folded when both agree — the same determinism guard as the
    `make` default fold), then bakes the real value as an inert const / type
    operand. Gated to a known pure-reader set with a module-family operand and
    otherwise compile-time-constant args; module instances are kept concrete
    through `toCarrier` (like `FnDefInfo`) so the `$module` chain resolves. The
    all-rows diff confirmed 0 native→non-native regressions, +31 improvements
    (the cascade also cleared the `Test.*` synthetic-get rows, ratcheting
    reducible 62 → 54). Ratchets: refused 258 → 227, minCompiledRows 2007 →
    2038, compute gap 190 → 167. (`eng/go/carrier.go` `tryFoldModuleConst` /
    `concreteHandlerEval` / `isModuleFamilyValue` + the toCarrier module guard;
    `lang/go/bytecode_findings_test.go` `TestModuleSyntheticConstFold`.)

12. **OpMakeList — computed list literals ✅ LANDED (227 → 213).** A list
    literal whose elements are COMPUTED (`[1 add 2]` → `[3]`, `[(1 add 2)
    (3 add 4)]`, `[a.b c.d]`) cannot bake as an inert const (its elements are
    event results), so it refused "residual value of unknown provenance" — the
    compiler had NO list-assembly primitive, only `OpPushConst` for fully-literal
    lists. Added `OpMakeList N` (pop N, push a list, order preserved). `autoEval
    List` records the assembly: the elements lower onto the stack (their own
    events), then the opcode assembles them. The soundness gates, each found by
    the differential / regression diff:
    - **Top frame only** — a fn-body / closure / branch-arm list is re-evaluated
      per call with a different scope (`fn […[[c1]]]`), so freezing one assembly
      diverges.
    - **TOP engine only** (`e.isTop`) — a SUB-engine eval is a handler inferring
      a re-run NoEval body (`list-of [Rand.int 0 10] 3`), often non-deterministic.
    - **Core-builtin element producers** — a module/user word may be stateful
      (`rand-int` advances the seed); only a builtin value-yield re-computes
      identically.
    - **No type-pattern (`[Integer]`) or make-instance elements** — those are
      owned by the type machinery / schema-member const-bake, which a typed-def
      reparent then re-IDs.
    - **Not a `for` range** — `for` over a runtime-assembled range list diverges;
      keep it on the literal-const path (`makeListRange` refusal).
    Order matters: the ops reverse so element 0 lands DEEPEST (a list assembles
    bottom-up, unlike a sig's top-first operands). The all-rows diff confirmed
    0 native→non-native regressions, +14 improvements — and it cleared the
    corpus's last tier-1 row (`Vm.run (canon [1 {a:none} x/q])`, once the canon
    list could assemble), so interp is now 0. (`eng/go/bytecode.go` OpMakeList,
    `eng/go/vm.go` exec, `eng/go/lower.go` + `eng/go/emit.go` RecordMakeList /
    `makeListRange` / the for gate, `eng/go/engine.go` autoEvalList;
    `lang/go/bytecode_findings_test.go` `TestOpMakeListCompiles`.)

Projected trajectory: items 1–4 take 459 → ~150 (clearing make/get/set/is/
typeof/lowering); item 5 takes islands 15 → ~3 and refusals ~150 → ~80; items
6–8 → ~40, all META + error rows; item 9 closes the gate.

## 5. Why not delete the island after all

Even at the re-scoped target the `OpFallback` island earns its keep: it is the
seam that runs a tier-1 span (a `Vm.run` body — runtime-computed code) through
the interpreter mid-Program without abandoning the compiled code around it.
Deleting it (the runtime-independence doc's literal §P7) would force the *whole*
program to fall back the moment it touches one runtime-eval word — strictly worse
coverage than a confined island. Keep the island; delete only the *unbounded
whole-program* fallback path in `RunCompiled` once tiers 2 and 3 reach 0 (so the
island is provably confined to tier 1).

## 6. Verification discipline

Per item: `make fmt && make vet && make lint && make test`; the coverage
ratchets move only DOWN; `TestSpecCompiledDifferential` (raise `minCompiledRows`)
and `TestSpecCompiledOrFallback` at 0 divergences (value + taxonomy); `-race` on
both concurrency gates; alloc ceilings held; and the new `-tags aqldebug` gate
(args-aliasing) green. Gate-clean-or-revert; commit each landed item with its
ratchet delta.

**Property-based differential (`TestPropertyDifferential`).** The curated
corpus is a finite, hand-written oracle — it exercises rows, never COMBINATIONS,
and the carrier compiler's per-construct gates fail exactly on combinations the
author didn't foresee (the make-default fold inside a `for` body; a computed map
value referencing a def-local carrier). So a generator emits well-typed AQL from
the compilable subset (arithmetic / division / boolean logic / strings / `if` /
computed lists / `size` / `def`-locals / `for` / nested maps + `get` / in-place
mutation (`Array` + map `set`) / object & class field mutation / higher-order
`fold`/`each`/`scan` over literal lists, type-tracked so `if` branches and `get`
results stay well-typed; error TAXONOMY compared too) and asserts compiled ==
interpreted on each, with a shrinker that reduces a failure to a minimal program. It has found THREE real divergences the
corpus had missed for the life of the project: (1) `for 3 [{a: (3 mul i)} get a]`
— `constFoldContainerVal` froze the loop-iterator-dependent map value; (2) `def
v0 (0 add 3) … {a: (5 mul v0)}` — the fold ran against `v0`'s CARRIER binding,
which `AsInteger`-coerced to 0, baking `{a:0}` (both fixed by gating the fold to
the top frame AND off any expression referencing a carrier binding,
`exprRefsCarrier`; a user TYPE binding, `Carrier=false`, still folds); (3) `def
v0 3 def v1 (if c [v0] [4]) v0` -> 4 — `JoinCarriers` kept the then-arm's ID, so
the `if`-result reused `v0`'s identity and a later `v0` reference resolved to the
if-event (fixed by minting a fresh ID for the merged carrier — it is a NEW
value). 36 000 generated programs across 12 seeds are divergence-free after each
fix. Each subsequent widening round — in-place mutation, object/class field
mutation, and higher-order `fold`/`each`/`scan` — landed divergence-free on
first hunt (21 808 compiled paths across 12 seeds on the higher-order round
alone), so the generator now doubles as a standing soundness witness for the
constructs it already covers, not only a bug-finder. This is the durable answer
to "why is this so fiddly": fund the ORACLE, not the manual probing.
