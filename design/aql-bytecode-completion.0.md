# AQL Bytecode — Completion Roadmap (toward P7)

Status: design. Companion to `aql-bytecode-plan.0.md` (P0–P4) and
`aql-bytecode-runtime-independence.0.md` (P5–P7 machinery). This doc
**reconciles the plan with the live state** after the P5/identity/predicate/
case/fn-value items landed and the A–F robustness hardening, then maps the
**remaining** work to the goal with a grounded cluster census and a
**re-scoped P7** that is actually reachable.

## 1. Status reconciliation

The runtime-independence doc's "Recommended sequencing" marks items 1–6 DONE,
but the ratchets stand at **459 refused / 15 islanded** (from 651 / 115 at P0).
Those items each cleared their *named* cluster; what remains is a **long tail**
the sequencing section never enumerated. This doc enumerates it.

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

1. Define `metaFallbackWords` — the curated allowlist of inherently-dynamic
   words (`vm-run`, `vm-run-with`, the `test-*` family, `flex`, `canon`,
   `macroexpand`, `minilang-register`, `codequote`, the usurp synthetics,
   `args`). Each entry carries a one-line *why it cannot compile*.
2. `TestOnlyMetaFallsBack` (replaces `TestEveryRowCompiles`): every `.tsv`
   value row either compiles to a fallback-free `Program`, OR its refusal/island
   is attributable to a `metaFallbackWords` member. A refusal/island from any
   word NOT on the list fails the gate. This keeps the downward ratchet honest
   without demanding the impossible.
3. Error rows (count-mismatch returns, `unpack` missing key, orphan `gen`) get
   their own small disposition: either the VM raises the matching taxonomy
   (preferred — they then compile), or they join the allowlist as
   "deliberately interpreted to surface the runtime error."
4. THEN the deletions in `aql-bytecode-runtime-independence.0.md` §P7 apply to
   the WHOLE-PROGRAM fallback's *unbounded* form: `RunCompiled` stops calling
   `a.Run(src)` for arbitrary refusals and instead runs the compiled `Program`
   (which now contains a NARROW `OpFallback` island only for allowlisted meta
   spans). The island machinery **stays** — it is the hybrid's interpreter seam,
   now provably confined to meta.

This is "complete delivery": real compute is 100% native and runtime-
independent; meta is explicitly, auditably interpreted.

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

5. **Lambda higher-order args + map iteration (~22 refusals + 7 islands, HIGH
   risk).** `filter ([p:Any] => …)`, `fold` over maps, `each {…}`. Two parts:
   (a) a lambda VALUE arg compiles its body to a closure (the closure machinery
   exists; the afn-value path needs the fn-VALUE-on-stack handling the apply
   shortcut sketched); (b) map iteration emits ordered KeyVal traversal natively
   instead of islanding. The highest-leverage island reducer. (`eng/go/carrier.go`,
   `emit.go`, possibly a `FOR_EACH_MAP` lowering.)

6. **computed-else `if` ✅ LANDED (425 → 421); variadic-statement `if` still
   open (~9 rows).** The computed-else half — `if cond [then] (expr)` where the
   else is an eagerly-evaluated paren result on the stack — now lowers: a new
   `OpDrop` plus a `lowerArmsComputed` path SWAPs the cond above the
   eager else value, `JMP_IF_FALSE`, and on the taken path DROPs the else before
   the then-body (the false path keeps it as the result). Both arms net one
   value, so the result is a single (non-variadic) merge slot — handled only
   when the cond is itself a plain stack event. The remaining if-branch bucket
   (9) is the variadic/empty-else statement-if (`if c [99] []`, `if c [raise]`
   as a discarded statement), which needs true 0-or-1 residual modelling and is
   a separate follow-on. (`eng/go/bytecode.go` OpDrop, `eng/go/vm.go`,
   `eng/go/emit.go` RecordBranch, `eng/go/lower.go` lowerArmsComputed.)

7. **`select` query DSL + `reach` lenses (~26, MED, OPTIONAL for P7).** Compile
   the query/lens bodies, or — if the cost outweighs the benefit — add them to
   `metaFallbackWords` as a deliberate DSL-interpreted boundary. Decide with the
   island/refusal numbers in hand.

8. **introspection over fn-values (12, LOW-MED).** `typeof (f/r)`, `tcmp
   Function` want the fn value as a plain CALL_NATIVE operand (not a closure) —
   the "smaller follow-on" the runtime-independence doc named.

9. **Error-row disposition + the re-scoped P7 gate.** Make the VM raise the
   count-mismatch / unpack-missing / orphan-gen taxonomies (so they compile), or
   allowlist them; then land `metaFallbackWords` + `TestOnlyMetaFallsBack` and
   perform the §P7 fallback narrowing.

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
