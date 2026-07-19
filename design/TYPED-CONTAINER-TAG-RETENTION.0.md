# Typed-Container Tag Retention — the enabler for `{:T}` write enforcement

**Status:** Proposed. Design only; no compiler code changed by this document.
The maintainer has chosen **full enforcement** of `{:T}` writes (reject a
non-conforming write at BOTH check and runtime), which is impossible under the
current value model. This document specifies the value-model change that makes it
possible.

Companion reading: `TYPED-CONTAINER-ELEMENT-PRECISION.0.md` (D2 — reads/writes,
the doc this one unblocks), `REFINE-NEWTYPE-VS-SUBSET.10.md` (the reparent
machinery a subtype representation would reuse), `INTERPRETER-SPEED-PLAN.10.md`
(why `Value` is size-optimized — constrains the representation).

---

## Why the existing D2 doc is blocked (discovered this cycle)

`TYPED-CONTAINER-ELEMENT-PRECISION.0.md` assumed a `{:T}` value **retains** its
element type after construction ("the result stays statically `{:Integer}`"). It
does not. Three measured facts:

1. **Construction drops the tag.** `def m:{:Integer} {a:1}` runs
   `Unify({a:1}, {:Integer})`. The typed-vs-concrete branch
   (`unify_map.go:86` → `unifyTypedMapWithConcrete` → `unifyMapValues`,
   `unify_fold.go:65`) returns `NewMap(result)` — a **plain** concrete map. Probe:
   `IsConcrete=true, IsTypedMap=false, child=none`. The list twin
   (`unify_list.go:109 unifyTypedListWithConcrete`) is identical.
2. **The runtime never carries `{:T}`.** Immutable `set` (`setMapHandler`) returns
   `NewMap(out)` — untagged. A `{:T}` fn PARAM is bound to whatever concrete map is
   passed (a plain map). Flex-`{:T}` is separately blocked (`flex {:Integer}` →
   `flex_error`, and `def m:{:Integer} (flex …)` currently **panics** — a
   pre-existing latent bug, see Appendix A).
3. **So writes cannot be rejected at runtime, and the concrete top-level example
   cannot be rejected at all.** `def m:{:Integer} {a:1} (m set "b" "wrong")` has no
   element type at the `set` site. The only place a `{:T}` element type is live is
   **inside a `{:T}` param body**, and only in check mode (Part A of D2 threads it
   into the body carrier).

The maintainer's requirement — reject `(m set "b" "wrong")` at check AND runtime —
therefore needs the value itself to **carry `{:T}` as a concrete value**. That is
this document.

## What "tag retention" must deliver

A concrete `{:T}` container is a value that is simultaneously:

- **Concrete and readable** — `IsConcrete(v)` is true and `AsMap(v)`/`AsList(v)`
  return the payload, so every existing read/op (`get`, `size`, `for`, `.k`, dot-
  chains, comparison, rendering) is unchanged. This rules out the
  `ChildTypeInfo`-backed carrier form (`Data` would be `ChildTypeInfo`, `AsMap`
  returns nil — it is not concrete). See "Representation" for why the `Entries`
  field is not the vehicle.
- **Element-typed** — the element constraint `T` is recoverable at the value from a
  cheap, non-payload channel, so `set`/`setpath`/`merge`/`inject` can check a write
  and reads can narrow to strict `T`.

The invariant this buys (the whole point): **construction AND every write are
enforced ⇒ a `{:T}` value provably holds only `T` ⇒ reads are soundly strict `T`,
and a provably-disjoint dispatch is a real static error.**

## Representation — the linchpin decision

Three candidates, judged against the two facts above (`Value` is size-optimized;
concrete reads need `AsMap`/`AsList`) and against census/dispatch ripple.

### Option 1 — minted lattice subtype per element type (reparent)

Mint a `*Type` for each distinct `{:T}` (`Parent = TMap`, element constraint
carried by a `typedMapBehavior{elem}` via `MintTypeWithBehavior`,
`typetable.go:542`). `def m:{:T} body` **reparents** the concrete map to it
(`ReparentValue`, the refine/newtype path). `set`/reads recover `elem` from
`v.Parent`'s behavior.

- **Pros:** values stay concrete (`AsMap` works); reuses the reparent + custom-
  Behavior machinery; typed containers become *real types* — fits AQL's "def binds,
  make instantiates" model and makes `typeof` honest.
- **Cons:** **broad census ripple.** Every `{:T}` value's `Parent` changes from
  `Map` to a minted subtype → touches dispatch admission, `typeof`, comparison
  ordering (Rank of a minted node), and the serialised `Value.ID`. Mint/dedup/
  retire/canonicalize per element type (incl. disjunct/nested children) is real
  surface. Highest blast radius; most "correct" long-term.

### Option 2 — one pointer field on `Value` (RECOMMENDED)

Add `elem *Type` to `Value`, **behind a pointer** exactly like the existing
`pos`/`tmeta`/`dynFrom` fields (nil for the overwhelming majority of values). A
concrete `{:T}` map is a normal `MapPayload` value with `elem` set; `Parent` stays
`TMap`.

- **Pros:** **lowest census ripple** — `Parent` stays `Map`, so dispatch, `typeof`,
  comparison, and `Value.ID` are all unchanged *except where we deliberately consult
  `elem`* (the write-check and, later, strict reads). Values stay concrete (`AsMap`
  works). Consistent with the established "one nil-cheap pointer" pattern the perf
  work already uses; a single extra 8-byte pointer, nil on the hot path.
- **Cons:** must thread `elem` through `CloneValue`, `ReparentValue`, `WithPos`, and
  the value constructors that should propagate it (`set`'s result). Must **not** let
  `elem` leak into equality or rendering (two maps with equal entries stay `Equal`
  and `String()`-identical regardless of `elem`) — otherwise `compare.tsv`/assert
  differentials churn. A disjunct/nested child needs `elem` to point at a real
  constraint type (mint a small carrier type or store a `*Type` whose behavior holds
  the disjunct — reuse the `{:T}` type-literal's existing child Value).

### Option 3 — `ChildTypeInfo.Entries` as the standard concrete form — REJECTED

Make `{a:1 :Integer}`-style `ChildTypeInfo{Child, Entries}` the representation for
all concrete `{:T}` values. Rejected: such a value is **not** `IsConcrete` and
`AsMap` returns nil, so every read/op would need a parallel entries-reading path
(enormous blast radius); the surface syntax is barely-supported and parses
confusingly (`{a:1 :Integer}` lexes as `{a:{1:Integer}}`).

### Recommendation

**Option 2** (pointer field). It reaches full enforcement with the **smallest
census surface** because `Parent` stays `Map` — the change is *additive and opt-in
at the consult sites*, not a lattice-wide reparent. Option 1 is the cleaner
long-term model but pays a whole-corpus reparent ripple up front; defer it unless
`typeof`-honesty or type-level dispatch on `{:T}` becomes a requirement.

## Enforcement points (Option 2)

1. **Construction (already the gate; now also SETS `elem`).**
   `unifyTypedMapWithConcrete` / `unifyTypedListWithConcrete` currently validate
   every element and return `NewMap`/`NewList`. Change: return the concrete
   container **with `elem = childType`**. This is the single origin of the tag.
   (The def-handler and `typed_bind.go` construction checks already run this Unify.)
2. **`set` (Map + List) — check + retain.** In `setMapHandler`/`setListHandler`:
   when the receiver's `elem` is set and the written value does not `Unify` with it
   → `type_error` at runtime; on success the returned copy **keeps `elem`**. Mirror
   in a check-mode `ReturnsFn` (the diagnostic surfaces inside a called `{:T}` param
   body — verified: an ungated `set` ReturnsFn diagnostic *does* surface at
   `FnBodyDepth=1`). This is the class-instance precedent (`setClassInstanceReturns`
   + `setClassInstanceHandler`) applied to `elem`.
3. **`setpath` / `merge` / `inject` — deep writes.** These delegate to
   voxgigstruct with no element check. Phase them AFTER `set` (documented, not
   silently skipped): a deep write into a `{:T}` container must re-validate the
   touched leaves against `elem`. `merge`/`inject` need a walk of the merged result.
4. **Reads — tighten to strict `T`.** Once the invariant holds, D2 Part B's read can
   move from `dynamic(T)` to strict `(T tor None)`. Land this LAST, behind the
   differential.

## Phasing (each phase validates against the census before the next)

- **Phase R1 — the field + construction origin.** Add `elem *Type` (pointer,
  nil-default), thread through `CloneValue`/`ReparentValue`/`WithPos`, set it in the
  two `unifyTyped*WithConcrete` origins. NO behavior change yet (nothing consults
  `elem`). Gate: census/differentials **byte-identical** (the field is inert). This
  de-risks the core-model touch in isolation.
- **Phase R2 — `set` write-check + result retention (runtime + check).** The
  maintainer's example now errors at check AND `aql run`. Gate: census green (valid
  corpus has no non-conforming writes); new pins for reject + conforming-keeps-tag.
- **Phase R3 — `setpath`/`merge`/`inject`** deep-write enforcement.
- **Phase R4 — strict reads** (D2 Part B `dynamic(T)` → `(T tor None)`). NOTE a
  pre-existing precision bug to fix here: `d2TypedContainerBound`'s single-type
  branch binds `child.Parent` (the element's lattice SUPERTYPE — a `{:Integer}`
  read narrows to `dynamic(Number)`, `{:String}` to `dynamic(Scalar)`) because a
  type literal's `.Parent` is its supertype. Use the child value's own denoted
  type (as the disjunct branch already does via `NewDynamicCarrierValue`). The
  census tolerates the widening today (gradual, non-divergent); R4 makes reads
  exact. R2's write-check is already exact — it unifies against the child value.

Phase R1 is the make-or-break core-model step; if its census is not byte-identical,
the representation is leaking (into equality, rendering, `typeof`, or dispatch) and
must be corrected before any enforcement lands.

## Interaction with the shipped D2 A+B

D2 Part A (`typedContainerCarrier`, `core_helpers.go`) and Part B
(`d2TypedContainerBound`, `native_storage.go`) are in the tree, census-green. They
thread/narrow the **carrier** element type for `{:T}` PARAM bodies in check mode —
independent of tag retention (a param carrier already holds `ChildTypeInfo`). Tag
retention is what extends precision + enforcement to **concrete** `{:T}` values and
to **runtime**. A+B stay; R4 tightens B's reads once R2 makes them sound.

## Risks

- **Representation leak (Phase R1).** If `elem` reaches equality/rendering/`typeof`/
  dispatch, the census churns corpus-wide. Mitigation: R1 lands the field INERT and
  asserts byte-identical census before anything consults it.
- **Result-retention gaps.** A write path that builds a fresh container and forgets
  to carry `elem` silently drops enforcement downstream. Mitigation: centralise the
  "copy map/list preserving `elem`" in one helper; audit every `NewMap`/`NewList`
  that copies a receiver.
- **`elem` provenance for disjunct/nested children.** `{:(A tor B)}` / `{:[:Integer]}`
  need `elem` to reference a real constraint. Mitigation: reuse the child Value the
  `{:T}` type literal already carries (mint a member type if a bare `*Type` is
  insufficient).
- **Flex.** Mutable-by-reference `{:T}` flex would need in-place write enforcement;
  it is currently blocked/panics (Appendix A). Keep flex out of scope until its
  construction is fixed, or continue to reject `{:T}` flex at construction.

## Validation surface (every phase)

- `test/go/langspec`: `TestCompiledCoverage` (byte-identical census — R1 must not
  move it at all; R2+ only on buggy programs, which the corpus lacks),
  `TestSpecCompiledDifferential`, `TestVariationDifferential`, `TestOnlyMetaFallsBack`.
- `make fmt && make vet && make lint && make test && make cover-gate`.
- The 6-library voxgig four-surface harness.
- New pins (`lang/go/bytecode_typed_container_test.go`): a concrete `{:Integer}`
  retains its tag through a conforming `set`; a non-conforming `set` is a
  `type_error` at check AND raises at runtime; an untyped `Map` is unchanged;
  `typeof`/`String()`/equality of a tagged map are identical to the untagged map.

## Critical files

- `eng/go/value.go` — the `Value` struct + `elem` field; `CloneValue`; the
  `AsMap`/`IsConcrete` invariants (must stay true for a tagged value).
- `eng/go/unify_map.go` (`unifyTypedMapWithConcrete`:160) / `eng/go/unify_list.go`
  (`unifyTypedListWithConcrete`:109) / `eng/go/unify_fold.go`
  (`unifyMapValues`:65) — the construction origins that must SET `elem`.
- `eng/go/carrier.go`, `eng/go/core_boundedtype.go`, `typed_bind.go` — reparent /
  typed-bind paths that must thread `elem`.
- `lang/go/native/native_storage.go` — `setMapHandler`/`setListHandler` +
  `ReturnsFn`s (R2); `getNodeReturns`/`getIntKeyReturns` (R4).
- `lang/go/native/setpath.go`, `merge.go`, `inject.go` — R3.

## Appendix A — pre-existing flex-`{:T}` panic (out of scope, file separately)

`def m:{:Integer} (flex {a:1}) …` → `internal_error: invalid memory address or nil
pointer dereference` (reproduced on a clean tree without the D2 changes). A
`{:T}`-typed def with a flex body nil-derefs. Unrelated to D2; a panic is an
ADR-005 violation and should be fixed independently.
