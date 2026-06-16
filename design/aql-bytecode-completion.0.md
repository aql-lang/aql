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

## 3. P7 reachability — the literal goal is not reachable; re-scope it

`aql-bytecode-runtime-independence.0.md` gates P7 (delete `OpFallback` + the
whole-program fallback) on **both ratchets at 0** — every row compiles. That is
**not achievable** while the corpus exercises the META cluster:

- `Vm.run`/`run-with` parse and execute **runtime-constructed source** in a
  sub-engine (`lang/go/modules/vm.go`). The program isn't known until run time,
  so there is nothing to AOT-compile — this IS the interpreter, by definition.
- The **Test harness** generates inputs and runs candidate programs (property
  testing); `macroexpand`/`minilang-register` rewrite/register code at run
  time; `flex`/`canon` are reflective; `codequote`/`quote` are code-as-data;
  usurp re-steps tape-coupled values the VM cannot push; `args.N` needs a
  per-call args stack the VM frame deliberately doesn't keep.

The original outline (`aql-bytecode-outline.0.md` §5, §8) was right: *"Not as a
replacement for the interpreter — dynamic features need a fallback. A hybrid
design keeps the interpreter for these."* The runtime-independence doc's P7 over-
reached. **Re-scope P7** from *"delete the fallback"* to:

> **Native execution of all REAL COMPUTE; an explicit, enumerated, allowlisted
> meta-fallback for the irreducible dynamic/reflective words.**

### Re-scoped P7 gate (replaces "both ratchets == 0")

**Steps 1–3 ✅ LANDED** (`test/go/langspec/compiled_metafallback_test.go`,
`TestOnlyMetaFallsBack`). The gate partitions every refused/islanded spec value
row into meta / error-row / **compute gap** and ratchets the compute gap toward
0. Current partition (June 2026): **131 meta-attributable, 12 error-row, 265
compute gap** (`computeRefusalCeiling`). Meta breakdown: usurp 43, word (the
Forth-style macro splice) 30, Test/Assert harness 28, quote/codequote 14, flex
7, minilang 5, args 2, Vm.run 1, canon 1.

1. ✅ `metaFallbackWords` — the curated allowlist (`usurp` + `/u`/`/ur`/`/us`
   synthetics, `quote`/`codequote`, `macroexpand`, `minilang`, `flex`, `canon`,
   `args.` accessor, `Vm.run`/`Vm.run-with`, the `Test.`/`Assert.` harness).
   Each entry carries a one-line *why it cannot compile*. Attribution is by a
   NARROW source pattern (the accessor `args.`, not `forward-args`; `Vm.run`,
   not every `run`) so a real compute gap is never masked.
2. ✅ `TestOnlyMetaFallsBack`: every `.tsv` value row either compiles to a
   fallback-free `Program`, OR its refusal/island is attributed to a
   `metaFallbackWords` member or an error-row. A non-meta, non-error
   refusal/island is a COMPUTE GAP, ratcheted by `computeRefusalCeiling` — at 0
   this becomes the strict "only meta falls back" gate. Keeps the ratchet honest
   without demanding the impossible.
3. ✅ Error rows (count-mismatch returns, `unpack` missing key, orphan `gen`)
   are dispositioned as **allowlisted** (`errorRowReason`): the checker
   deliberately refuses so the interpreter surfaces the matching taxonomy. The
   alternative (make the VM raise the taxonomy so they compile) stays available
   as future tightening.
4. **PENDING** (gated on `computeRefusalCeiling` reaching 0): THEN the deletions
   in `aql-bytecode-runtime-independence.0.md` §P7 apply to the WHOLE-PROGRAM
   fallback's *unbounded* form: `RunCompiled` stops calling `a.Run(src)` for
   arbitrary refusals and instead runs the compiled `Program` (which now contains
   a NARROW `OpFallback` island only for allowlisted meta spans). The island
   machinery **stays** — it is the hybrid's interpreter seam, now provably
   confined to meta.

This is "complete delivery": real compute is 100% native and runtime-
independent; meta is explicitly, auditably interpreted. The gate now MEASURES
the gap to that delivery (295 compute rows) instead of asserting an impossible
absolute.

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

7. **`select` query DSL + `reach` lenses (~26, MED, OPTIONAL for P7).** Compile
   the query/lens bodies, or — if the cost outweighs the benefit — add them to
   `metaFallbackWords` as a deliberate DSL-interpreted boundary. Decide with the
   island/refusal numbers in hand.

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

9. **Error-row disposition + the re-scoped P7 gate ✅ GATE LANDED (steps 1–3);
   fallback narrowing PENDING (step 4).** `metaFallbackWords` +
   `TestOnlyMetaFallsBack` partition every refused/islanded row into meta (131) /
   error-row (12) / compute gap (265) and ratchet the compute gap toward 0
   (`computeRefusalCeiling = 265`). Error rows are allowlisted (the checker
   refuses so the interpreter surfaces the taxonomy); making the VM raise them
   stays available as future tightening. The §P7 fallback narrowing (step 4)
   waits on the compute gap reaching 0 — its remaining drivers are the
   operand-provenance cascades (140, clear when their producers compile: item-3
   class instances, user-fn results), the code-body DSL words (82: `select` /
   `case` / `reach` / `with-decimal` / `where` / `group` / … — item 7 + follow-
   ons), Stage-1 lowering residuals (~24), dynamic in/out (~24), the 9 islands,
   and user-fn dispatch (5). (`test/go/langspec/compiled_metafallback_test.go`.)

Projected trajectory: items 1–4 take 459 → ~150 (clearing make/get/set/is/
typeof/lowering); item 5 takes islands 15 → ~3 and refusals ~150 → ~80; items
6–8 → ~40, all META + error rows; item 9 closes the gate.

## 5. Why not delete the island after all

Even at the re-scoped target the `OpFallback` island earns its keep: it is the
seam that runs an allowlisted meta span (e.g. a `Vm.run` body) through the
interpreter mid-Program without abandoning the compiled code around it. Deleting
it (the runtime-independence doc's literal §P7) would force the *whole* program
to fall back the moment it touches one meta word — strictly worse coverage than
a confined island. Keep the island; delete only the *unbounded whole-program*
fallback path in `RunCompiled` once `TestOnlyMetaFallsBack` is green.

## 6. Verification discipline (unchanged)

Per item: `make fmt && make vet && make lint && make test`; the coverage
ratchets move only DOWN; `TestSpecCompiledDifferential` (raise `minCompiledRows`)
and `TestSpecCompiledOrFallback` at 0 divergences (value + taxonomy); `-race` on
both concurrency gates; alloc ceilings held; and the new `-tags aqldebug` gate
(args-aliasing) green. Gate-clean-or-revert; commit each landed item with its
ratchet delta.
