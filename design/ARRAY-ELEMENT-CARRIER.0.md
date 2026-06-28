# Parameterised mutable-Array carriers (`Array<T>`) — design

Goal: let the byte compiler type the ELEMENT of a mutable `Array` so that
`arr get i` yields the element type instead of a gradual `Any`. This is the
layer-1 root fix for the radix-msd compile leaf (the last of the 30 sort
algorithms) and a class of `make Array … get … arithmetic` shapes across the
voxgig corpus. It is a type-system feature, NOT a lowering leaf — scoped here
because it spans the lattice, `make`, `get`, and `set`, and because the
mutable-aliasing soundness model needs deciding before any code.

Soundness invariant (non-negotiable, as everywhere in this compiler):
`compile == interpret`, 0 miscompiles. The langspec differential is BLIND to
off-corpus shapes, so every stage ships `make verify-bytecode` GREEN +
`make crossdiff` GREEN + a hand-pinned off-corpus `RunCompiledStrict == Run`
regression. A wrong element type is a SILENT MISCOMPILE (the body compiles a
numeric op the runtime value can't take), so this feature is gated harder than
a lowering refusal — a refusal falls back soundly; a wrong carrier does not.

## 1. The motivating cascade (radix-msd, fully root-caused)

`radix-msd-sort` / `msd-go` recurse over Integer-only arrays. The compile
refusal "code-body word each" is the TAIL of a 4-layer cascade, all rooted in
one fact — **Array `get` returns gradual `Any`**:

1. **`counts get 0` → gradual `Any`.** `counts = make Array [0 0 0]` is a
   non-concrete carrier in check mode; the Array-get sig hardcodes
   `Returns: [TAny]` (`native_storage.go:191`), and Array carriers carry NO
   element type. (Confirmed: `[ADDCONCAT] args=[Any(dyn=true) Integer]`.)
2. **`lo add (gradual Any)` → gradual `Scalar`.** The gradual Any fills the
   String slot of `add`'s concat overload `[Scalar String]` optimistically, so
   `ReturnsAddConcat` returns `NewDynamicCarrier(TScalar)` (`carrier.go`). So
   `blo`/`bhi` widen to `Scalar`.
3. **The recursive `… go` call passes the `Scalar` into `hi:Integer`.** The
   body-UNIT-compile path generalises args with `NewCarrier(a.Parent)` and does
   NOT narrow to declared param types (`core_helpers.go` ~532; the carrier-
   RESULT path DOES, via `narrowArgsToParams` at ~577). So go's body recompiles
   with `hi:Scalar` → `(hi sub lo)` = "unmatched dispatch recovered at sub".
4. **Even narrowing layer 3 only exposes layer 4** — the narrowed recursive arg
   matches the in-flight key, so `AnalyseFnBody` BAILS returning
   `NewCarrier(declared)` (no provenance); the enclosing outer-`if` Array merge
   is then unrecorded → "body result of unknown provenance".

If `counts get 0` yielded `Integer`, NONE of this starts: `lo add Integer` →
`Integer` (numeric, unambiguous), params stay `Integer`, recursion is the clean
Integer shape that already compiles. **Layer 1 collapses all four.** That is
the entire justification for this feature.

## 2. Why it is hard: mutable + aliased + forward-pass

Lists already track their element type — `NewCarrierTypedList(elem)` is a
`TList` carrier with `Data = ChildTypeInfo{Child: NewCarrier(elem)}`, read back
via `DataListElemTypeFromValue` (`carrier.go:60`). `make Array` could mint the
analogous `Array<T>` trivially. Three things make Array genuinely harder than
List:

- **Mutability.** Lists are immutable (every op returns a new list), so the
  element type is fixed at construction. `Array.set` MUTATES in place, so the
  element type must account for EVERY write, not just `make`.
- **Aliasing.** Arrays are pointer-backed (`ArrayInstanceInfo{Elems}`) and a
  binding copy SHARES the backing (the capture-semantics note in
  `lang/go/CLAUDE.md`). A `set` through one alias must re-type EVERY alias's
  carrier — `def a (make Array …); def b a; b set 0 "x"` widens `a`'s element
  type too.
- **Forward-pass incompleteness.** Check-mode analysis is a single
  left-to-right walk. A `get` that precedes a later widening `set` would read a
  too-narrow type and commit a numeric op the run will break. The element type a
  `get` returns must be the FINAL upper bound, not the writes-seen-so-far bound.

The element type T must therefore be a sound **upper bound of every value the
array ever holds** — at every `get`, across all aliases, regardless of source
order. Getting that wrong is a miscompile, not a refusal.

## 3. Representation: `Array<T>` carrier

Mirror the typed-list carrier exactly, on the Array side:

- `NewCarrierTypedArray(elem *Type) Value` → a `TArray` carrier with
  `Data = ChildTypeInfo{Child: NewCarrier(elem)}` and `Carrier = true`. (Reuse
  `ChildTypeInfo` — it is already the kernel's "child element type" payload and
  is `Parent`-agnostic; `AsMap`/`AsList` guards already tolerate it on `TMap`.)
- `DataArrayElemTypeFromValue(v) *Type` → reads the `ChildTypeInfo.Child`
  parent, falling back to `TAny` when absent (an untyped Array carrier — e.g. a
  param `a:Array` with no element annotation).
- A concrete Array value (`ArrayInstanceInfo`) is unaffected at RUN time; this
  is a CHECK-MODE carrier shape only, like `NewCarrierTypedList`.

`T = TAny` (the untyped case) must behave EXACTLY as today: `get` → gradual
`Any` → the existing sound refusal path. The feature only ever NARROWS the
untyped-Any baseline to a known T; it never widens past Any.

## 4. The three touch points

### 4a. `make Array (src)` — mint `Array<T>`
`MakeArrayHandler`'s sig gets a `ReturnsFn` (today it is a flat
`Returns: [TArray]`, `native_make.go:40`). The ReturnsFn reads the SOURCE list's
element type: `T = DataListElemTypeFromValue(args[1])` and returns
`NewCarrierTypedArray(T)`. `make Array [0 0 0]` → `Array<Integer>`;
`make Array (iota n each [drop 0])` → `Array<Integer>` (the each-closure result
element); `make Array someUntypedList` → `Array<Any>` (unchanged behaviour).

### 4b. `arr get i` — return the element type
Array-get's sig gets a `ReturnsFn` (today flat `Returns: [TAny]`,
`native_storage.go:191`) that returns `DataArrayElemTypeFromValue(args[1])`.
Open decision (§6): strict `NewCarrier(T)` vs gradual `NewDynamicCarrier(T)`.

### 4c. `arr set i v` — keep T a sound upper bound
The crux. `set`'s carrier effect must guarantee that, AT EVERY `get`, the
array's element carrier is an upper bound of the value stored here. Options in
§5. Whatever the rule, `set` is where soundness is won or lost.

## 5. The element-bound strategy — three options

### Option A — flow-sensitive set-widening with alias propagation
At each `arr set i v`, join `v`'s type into `arr`'s element type and REBIND
every alias's carrier to `Array<join>`. Most precise; requires (1) a
`set`-typer that mutates the binding, and (2) alias resolution (which carriers
share this Array's identity). Defeated by forward-pass order: a `get` BEFORE a
widening `set` already returned the narrow type. Needs a fixed-point or a
two-pass walk over the fn body. **Highest precision, highest risk and cost** —
the aliasing + fixed-point is the part most likely to harbour a silent
miscompile.

### Option B — conservative whole-fn element join (RECOMMENDED first cut)
Before typing any `get`, compute T as the join (least upper bound) of the
`make` source element AND every value written by ANY `set` whose receiver
resolves to this array, anywhere in the fn body — order-independent, so every
`get` sees the FINAL bound. A `get`/`set` whose receiver can't be statically
resolved to a single tracked array (escapes, returned, stored in a map, passed
to another fn that may alias) collapses T to `Any` (today's behaviour) — sound
by construction. radix-msd's arrays are fn-local with conforming Integer sets,
so T stays `Integer`. **Soundest simple cut: an upper bound by construction, no
fixed-point, no flow-sensitivity.** The cost is the receiver-resolution /
escape analysis (which `def`-bound array each `get`/`set` targets), but that is
a bounded static query, not a lattice fixed-point.

### Option C — escape-gated strict, else Any
Even simpler: an Array gets a tracked T ONLY if it (1) is a `def`-bound local,
(2) never escapes the fn (not returned, not stored, not passed where it could
alias), and (3) every `set` on it writes a value conforming to the `make`
source element. If all three hold → `get` returns the strict `make` element
type; otherwise → `Any`. No join, no widening — a pure admissibility gate. This
is Option B with "join" replaced by "prove monomorphic-or-bail". Cheapest;
types radix-msd; declines anything it can't prove (sound). **Best risk/reward
for a FIRST landing** — it converts the feature from "track a widening lattice
through mutation+aliasing" into "admit only the provably-invariant arrays",
which is a refusal (sound) when the proof fails.

Recommendation: **ship Option C first** (it clears radix-msd and is a pure
admissibility gate — a wrong proof declines, it never miscompiles), then
generalise to B if more corpus shapes need a widening element type. Do NOT
start with A.

## 6. Open decision: strict vs gradual element carrier from `get`

Whether `arr get i` returns `NewCarrier(T)` (strict) or `NewDynamicCarrier(T)`
(gradual) changes the downstream dispatch and must be settled with an
experiment, because it determines whether the cascade actually collapses:

- **Strict `Integer`**: `lo add Integer` → numeric overload only → `Integer`.
  Clean. But a strict `Integer` get-result flowing into a position that expects
  gradual (an `Any` param) may be over-strict elsewhere.
- **Gradual `Integer`**: matches optimistically. MUST verify the actual kernel
  matching semantics — does `NewDynamicCarrier(TInteger)` match `add`'s concat
  `[Scalar String]` String slot? If a gradual Integer matches ONLY
  Integer-compatible slots (bound-respecting), `lo add (gradual Integer)` stays
  numeric and the cascade collapses. If it matches String optimistically
  (fully-open), it STAYS ambiguous → Scalar → no improvement. **This is the
  first thing to test** (a one-line probe: type `counts get` as gradual Integer
  and observe whether `ADDCONCAT` still fires). Mirror whichever the typed-list
  element path (`NewElementCarrier`, `carrier.go:124`) already does for a known
  element type — that path is the precedent and is already gate-green.

Under Option C the array is proven monomorphic, so a STRICT element carrier is
sound (the run value provably conforms). Start strict; fall back to gradual only
if a strict get-result over-refuses elsewhere in the corpus.

## 7. What collapses, and the secondary fixes that become unnecessary

With `counts get → Integer`:
- Layer 2 (add→Scalar) never fires.
- Layer 3 (genArgs param-narrowing) becomes UNNECESSARY for this shape — but it
  is independently a real cross-pass inconsistency (the compile-path and
  result-path analyse the same body with different arg carriers). It can be
  landed SEPARATELY as a consistency fix IF a concrete win is found; this design
  does not depend on it. (See §16 of MODULE-FN-PARAM-SLOT-COMPILATION.0.md — it
  was reverted twice for lack of a standalone win.)
- Layer 4 (recursion-bail provenance) never triggers — params stay Integer, the
  recursion is the clean Integer shape that already compiles via the §14
  `forkForProbe` recursion-through-closure fix.

So this ONE feature flips radix-msd → 30/30, retiring the sort chain.

## 8. Staged implementation plan (each stage independently gate-green)

- **Stage 0 — probe the §6 decision.** Temporarily hard-code Array-get to
  `NewDynamicCarrier(TInteger)` and confirm `ADDCONCAT` stops firing for
  `lo add (counts get)`; decide strict vs gradual. No commit — throwaway.
- **Stage 1 — representation.** `NewCarrierTypedArray` /
  `DataArrayElemTypeFromValue` + unit tests. No behaviour change yet (nothing
  produces a typed-array carrier). Gate: build + the new unit tests.
- **Stage 2 — `make Array` ReturnsFn.** Mint `Array<T>` from the source list.
  Still inert: `get` ignores it. Gate: verify-bytecode + crossdiff GREEN (a
  typed-array carrier that nothing reads must not change any result).
- **Stage 3 — `get` ReturnsFn under Option C admissibility.** Add the
  escape+monomorphic gate; a get on an admitted array returns `T`, else `Any`.
  This is the stage that changes results. Gate: verify-bytecode + crossdiff
  GREEN + radix-msd compiles natively + `RunCompiledStrict == Run` on the sort
  smoke corpus + an off-corpus regression where a NON-conforming set correctly
  DECLINES (get stays Any, program still compiles==interprets via fallback).
- **Stage 4 — flip `sort_smoke`.** Confirm all 30 algorithms compile and the
  voxgig sort suite's interpreter==compiler divergence run is clean. Update the
  pinned-corpus expectations.

The admissibility gate (Stage 3) is the soundness keystone — its NEGATIVE tests
matter more than the positive: an array that escapes, that a non-conforming set
writes, that a get/set can't resolve to a single tracked array, MUST fall back
to `Any`. Pair every "types as Integer" assertion with a "declines to Any"
sibling (per `lang/go/CLAUDE.md` test discipline).

## 9. Risks, alternatives, scope

- **Top risk: a silent miscompile from an unsound element bound.** Mitigated by
  Option C (admit only provably-invariant arrays) + the negative-test battery +
  the off-corpus `RunCompiledStrict` gate. NEVER let an unresolved-receiver
  `get`/`set` keep a narrow T.
- **Aliasing escape hatch.** Any array reachable through a capture, a returned
  value, a map/store field, or an argument to a fn that might retain it is
  NON-admissible in Option C. Be conservative: when in doubt, `Any`.
- **`convert List arr` / mixed reads.** radix-msd ends `convert List cur`; the
  array still flows out as a List there — fine, that consumes the array, it does
  not need the element type. Only the in-loop arithmetic `get`s need T.
- **Alternative considered & rejected for now:** a get-time runtime type GUARD
  (raise on mismatch) to license a strict T — rejected because it would raise
  where the interpreter does not (`compile != interpret`).
- **Out of scope:** typed-array SURFACE SYNTAX (`[:Integer]`-style array
  literals), element types for arrays built by non-`make` paths, and the
  general widening lattice (Option A). This design tracks ONLY what `make Array`
  establishes and what a local, non-escaping, monomorphic mutation pattern
  preserves.

## 10. Validation checklist (every result-changing stage)

1. `make verify-bytecode` GREEN — 0 miscompiles, incl. -race + aqldebug.
2. `make crossdiff` GREEN — 0 Go-vs-TS interpret divergences.
3. `make test` — only the pre-existing `TestCheckAccuracyRatchet` may fail.
4. Hand-pinned off-corpus `RunCompiledStrict == Run`: (a) a monomorphic-Integer
   local array compiles its arithmetic gets; (b) a NON-conforming-set array
   declines to `Any` and still compiles==interprets; (c) an ESCAPING array
   declines; (d) radix-msd / radix-msd-sort compile natively and sort correctly.
5. The voxgig `sort` repo's `test/divergence/run.sh` (interpreter vs
   `aql --compile`) is clean for all 30 algorithms.
