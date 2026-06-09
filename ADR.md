# Architecture Design Record (ADR)

A running list of the key architectural decisions behind AQL — the ones
that shape the language and its implementation, with the reasoning that
led to them. Each record is short, numbered, and dated. Newer decisions
may supersede older ones; superseded records are kept (struck through in
status) rather than deleted, so the history of *why* stays legible.

When you make a decision that future contributors would otherwise have
to reverse-engineer from the code, add a record here.

---

## ADR-001 — Native modules must not shadow core words {#adr-001}

**Status:** Accepted · **Date:** 2026-05-30

### Decision

A native module (`aql:math`, `aql:array-util`, `aql:matrix-util`, …) must **never
export a name that collides with a core (built-in) word**. If an
operation would naturally share a core word's name, do one of the
following instead:

1. **Extend the core word** with an additional type-dispatched signature,
   when the operation is a genuine variant of it; or
2. **Choose a different export name** for the module word.

### Context

AQL resolves words by signature and has no implicit `Word → Atom`
fallback. When a module exports a name that also exists as a core word,
two different operations end up wearing the "same" name, distinguished
only by an `aql:array-util`-style prefix. That is confusing in exactly the
case it matters most: when both apply to the *same* value type but mean
different things.

The motivating case was the array vocabulary. Three array operations had
been given `arr-`-prefixed built-in names (`arr-flatten`,
`arr-transpose`, `arr-indexof`) purely to dodge collisions with the core
`flatten` and `indexof`, and the first cut of the `aql:array-util` module
re-exported them as `ArrayUtil.flatten`/`ArrayUtil.indexof`. That meant
`flatten` (core, one level) and `ArrayUtil.flatten` (deep) did *different
things to the same list* — a foot-gun, and a symptom that the boundary
was drawn in the wrong place.

### Consequences

For `aql:array-util` specifically:

- **Deep flatten** is now `flatten -1` — a negative depth on the core
  `flatten` word (which removes one level by default, or `N` levels with
  `flatten N`). There is no `ArrayUtil.flatten`.
- **List lookup** is `ArrayUtil.indices` — a distinctly-named array word
  (for each needle, its index in the haystack, or `-1` when absent). There
  is no `ArrayUtil.indexof`.

  > **Amendment (2026-06-07).** This was originally folded into the core
  > `indexof` word as a `[List, List]` overload. Two later changes undid
  > that: `indexof` itself moved out of core into `aql:string-util`
  > (`StringUtil.indexof`, string-only), and overloading one word across
  > two unrelated domains proved a smell — the string form returns a
  > scalar with `-1`-when-absent, while the list form returns a vector
  > with a *different* absent sentinel. The list form is now its own word,
  > `ArrayUtil.indices`, in `aql:array-util`, with `-1` for an absent
  > needle (consistent with the string form's not-found value). This still
  > honours the ADR: `indices` shadows no core word.
- **`transpose`** has no core counterpart, so it keeps its plain name and
  remains `ArrayUtil.transpose`. The `arr-` workaround names are gone.

After this, the `aql:array-util` export set shares no name with any core word.

### Applied to `aql:matrix-util`

The `aql:matrix-util` module predated this record and exported `size`,
`flatten`, and `transpose`. These have been reconciled:

- **`size`** — dropped. The core `size` word already reports a tensor's
  entry count via the Sizer behavior (`TensorData`), so a `MatrixUtil.size`
  export only shadowed it.
- **`flatten`** — renamed to **`MatrixUtil.values`** (the row-major list of
  entries). The core `flatten` word remains the only `flatten`.
- **`transpose`** — kept. `transpose` is *not* a core word; it lives in
  the `aql:array-util` module. `MatrixUtil.transpose` and `ArrayUtil.transpose` are
  two namespaced module words, which this rule permits — the rule is
  about shadowing *core* words, not other module words.

After this, no module export shadows a core word.

---

## ADR-002 — No implicit broadcasting {#adr-002}

**Status:** Accepted · **Date:** 2026-05-30

### Decision

AQL will **not** implement broadcasting — the implicit lifting of a
scalar word over an array. Applying an operation across an array is
always **explicit**, via a combinator (`each`, `eachrank`, `fold`, …).
A scalar word applied to a list where it expects a scalar is a **type
error**, not a silent element-wise map.

```
add 10 [1,2,3]            # type error — no matching signature
each [add 10] [1,2,3]     #  # returns [11,12,13]   (the supported form)
```

### Context

An earlier draft of `design/ARRAYIFICATION.6.md` proposed broadcasting:
`add 10 [1,2,3]` returns `[11,12,13]`, with rules for scalar+list, equal-length
list+list zip, and nested alignment. It is attractive (it reads like
NumPy/APL) but a poor fit for AQL:

1. **It cannot be a word.** It would have to be a fallback wedged into
   the signature matcher (`eng/go/match.go`) — the most load-bearing
   code in the kernel — affecting *every* scalar word at once. A subtle
   bug there regresses the whole language, not one word.
2. **It defeats the static checker.** Result rank depends on the runtime
   shape of the operands, so `Check` mode could no longer infer result
   types without modelling unknown-depth lifting — undermining the
   typed-list carrier inference the codebase already relies on.
3. **It is ambiguous.** Words that legitimately take list arguments
   (`reshape`, `at`, the `group`/`fold` overloads, …) collide with the
   "scalar op lifted over a list" reading. The matcher would need a
   fragile precedence rule between "a real `[List, …]` signature exists"
   and "no scalar match → broadcast".
4. **It buys ergonomics, not power.** `add 10 [1,2,3]` is already
   `each [add 10] [1,2,3]`. The implicit form saves keystrokes at the
   cost of making dispatch — and reading — less predictable.

### Consequences

- Design principle 3 is "explicit iteration", not "implicit iteration".
- The `## Broadcasting` section of the arrayification design is marked
  rejected; Phase 5 is "rank polymorphism" (`eachrank`, `foldaxis`),
  which is explicit depth-targeting, not broadcasting.
- `eachrank`/`foldaxis` bodies must themselves iterate (e.g.
  `eachrank 1 [each [add 10]] …`); there is no implicit lift at the cell.
- This is a decision about the *language*. Type-specific element-wise
  behaviour can still be offered by a word with an explicit `[List, …]`
  signature (as `add` does for string concatenation, or `indexof` for
  lists) — that is normal signature dispatch, not broadcasting.

---

## ADR-003 — Every native-module export must be spec-covered {#adr-003}

**Status:** Accepted · **Date:** 2026-06-07

### Decision

Every word exported by a native module under `lang/go/modules/` —
i.e. every name reachable as `Namespace.word` after `import` — **must be
exercised by at least one row in the `lang/spec/*.tsv` suite**. A
content-based guard enforces this: a new export that ships without a
spec row fails the build.

The coverage unit is the **qualified name** `Namespace.word`
(`ArrayUtil.indices`, `MatrixUtil.transpose`), not the bare word. The
qualified form is what a user actually types after `import`, and it
disambiguates the legitimate cross-module name reuse the language allows
(`ArrayUtil.transpose` vs `MatrixUtil.transpose` — see ADR-001).

### Context

Of the seventeen native modules, four (`array-util`, `matrix-util`,
`string-util` in part, and the AQL-implemented `decision`/`report`/`test`
/`vm`/`query` modules) had grown export sets with **zero** rows in the
formal spec suite, and even the modules *with* a spec file
(`math-util`, `type-util`, `time-util`, …) covered only a fraction of
their exports. Nothing flagged the gap, so a newly-added module word
could ship completely untested by the language-level specs — the same
class of silent hole the user-type return-annotation bug exploited
(see `lang/go/CLAUDE.md` "Test discipline").

Per-word Go unit tests exist for many of these, but they test the Go
implementation, not the *imported, dot-accessed surface* a user calls.
The `.tsv` suite is the contract for that surface; it should be
exhaustive over the public export set, not a sample of it.

### Consequences

- A guard test, `TestModuleExportCoverage`
  (`test/go/langspec/coverage_test.go`), enumerates the live export set
  straight from the module registry (`modules.Names()` → `Resolve` →
  `ModuleDesc.Exports`), forms each `Namespace.word`, and asserts the
  literal string appears in at least one `lang/spec/*.tsv` input. It
  fails with the concrete list of uncovered names. Because it reads the
  registry rather than a hard-coded list, a new export is covered by the
  guard automatically — there is no second place to update.
- The companion `TestSpecProd` actually *runs* every row; this guard
  only asserts the rows exist. Together they make "exported" imply
  "imported, called, and checked at least once" in the formal suite.
- The initial backfill added the missing rows across
  `lang/spec/module-*.tsv` (new files for array/matrix/query/decision/
  vm/report/test/string; appended sections for the partially-covered
  modules) so the guard passes at adoption.
- Adding a native-module export now means adding at least one
  `Namespace.word` spec row in the same change. This is the module-export
  analogue of the "always pair positive with negative" test discipline.
- A **narrow hermetic-exemption escape hatch** exists for the rare export
  that cannot be exercised by a hermetic, deterministic spec row — currently
  only `IO.folder`, a host-filesystem `mkdir` whose in-memory-FS toggle does
  not engage through a spec row's context layering and whose `mem://` scheme
  is mangled by `make Path`. Such words are listed in `hermeticExempt`
  (`coverage_test.go`) with a justification and remain covered by Go tests
  (e.g. `lang/go/native/folder_test.go`). The list is asserted to contain
  only live exports so it cannot rot, and is meant to stay tiny: prefer the
  `mem://` scheme or a deterministic validation-error row (as the `aql:net`
  `prepare`/`direct` rows do) before reaching for an exemption.

---

## ADR-004 — All words are forward by default {#adr-004}

**Status:** Accepted · **Date:** 2026-06-09

### Decision

Every AQL word is **forward-collecting by default**: a word looks ahead
for its arguments first, so the canonical call form is
`word arg1 arg2 …` — written argument order matches declared parameter
order, and code reads like a function call. The only standing exception
is the **traditional Forth stack-manipulation vocabulary** — `dup`,
`swap`, `drop`, `over`, `rot`, `dup2`, `swap2`, `drop2`, `over2`,
`depth`, … — which is stack-only (`/s`) by nature: its entire meaning
*is* the stack, so there is nothing sensible to collect forward.

This is a **language cultural default**, not merely an implementation
detail. New words — core, module, and user `fn` definitions alike —
ship forward-eligible (`BarrierPos: -1`) unless their semantics are
intrinsically about the stack. Proposals to flip an individual word to
stack-first for local ergonomic relief are rejected; the per-call
modifiers (`/s`, `/f`, `/N`) and grouping (`(…)`, `end`, `;`) are the
sanctioned levers when a particular call site wants different
collection behaviour.

### Context

AQL is a concatenative language with a stack, but it deliberately does
not *read* like Forth. The §1.4 sig-order unification (see lang/go/
CLAUDE.md "Argument Ordering") made one rule govern every word, with
the forward phase first and the stack as fallback; the mirror
equivalence `f a b ≡ b f a ≡ b a f` means pipeline code and call-style
code are the same word. Both DX field reports
(`design/VOXGIG-DX-REPORT.5.md`, `design/AQL-DX-REPORT.5.md`)
confirmed the direction: the issues users hit were *edges* of forward
collection (grouping, mixed forms), and the fixes that landed
(structure-first lazy forward-argument resolution, FnDef forward
collection for module words) all moved the language *toward* uniform
forward behaviour, not away from it.

The cultural framing matters because the alternative — deciding
forward-vs-stack per word on ergonomic grounds — re-creates exactly
the "two calling conventions" complaint the DX reports documented for
module words. One memorable default beats a per-word lookup table.

The boundary is drawn at the traditional Forth words because they are
the vocabulary a stack-language user already holds in their head as
stack operations; making `dup` or `swap` forward-collect would be
gibberish. The full list is pinned in REFERENCE.md ("All stack words
are stack-only").

### Consequences

- **`print` stays forward.** The bloom-filter report suggested making
  `print` stack-first to fix the chained-print reversal
  (`(1 add 1) print (2 add 2) print` printing 4 then 2 — VOXGIG B2a).
  Under this ADR that fix is rejected: `print` is not a Forth stack
  word, and a one-off flip would be the first per-word cultural
  exception. The sanctioned forms are statement separation
  (`(1 add 1) print end (2 add 2) print`), the explicit modifier
  (`(1 add 1) print/s`), or the forward form (`print (1 add 1)`).
  The *residual* problem — chained un-separated forward calls
  evaluating right-to-left — is an evaluation-order question to fix
  (or diagnose via `aql check`) without changing any word's default;
  see `design/ERRORS.0.md` §"Chained forward calls".
- **Mixed-form calls are user error territory, diagnosed not blessed.**
  Forms that split one call's args across both sides without grouping
  (`(x 3 gt) if [a] [b]` — VOXGIG T9.4) are not given bespoke per-word
  semantics; the investment goes into `check`-mode advisories
  (`forward_strands_operand`, `uncalled_function`) that catch the
  stranded shapes.
- **Module wrappers must keep `BarrierPos: -1`** (already a hard rule —
  see lang/go/CLAUDE.md "Module FnDef Wrappers"). A stack-only inner
  sig silently breaks the swap form, which is a forward-culture
  violation *and* a silent-failure bug.
- **Docs and examples lead with the forward form.** REFERENCE.md,
  TUTORIAL.md, help entries, and spec rows show `word args` first and
  present the pipeline/stack forms as derived equivalents.
- The traditional-Forth exception list is closed by default: a new
  stack-only word needs the same justification weight as a new
  init-time panic (lang/go/CLAUDE.md "Panic Prevention") — i.e. its
  semantics must be *about* the stack itself.

---

## ADR-005 — Container symmetry: Object is the mutable keyed core type; classes get the `class` word {#adr-005}

**Status:** Accepted (implementation pending) · **Date:** 2026-06-09 · **Revised:** 2026-06-09 (no aliases; paren-free forms; flat instances)

### Decision

Linked decisions, accepted together (design, surface code, and phased
plan in `design/CLASS-OBJECT.0.md`):

1. **The container vocabulary is a 2×2:** `List : Array :: Map :
   Object` — immutable value vs mutable container, indexed vs keyed.
   `Object` becomes a core, constructible, fully-enumerable, mutable
   keyed container, with `object {…}` / `array […]` as sugars for
   `make Object {…}` / `make Array […]`. Dot access (`.` = `get`)
   is guaranteed across every receiver — Map, List, Object, Array,
   class instance, Store, module — read-only, literal-key semantics.
   **Store remains a separate surface type**: it is the language's
   *delegating* keyed container (chained copy-on-write lookup), which
   is why plain Object stays flat — see decision 4.
2. **`class` defines, `refine` extends:** `def Foo class {…}` defines
   a root class (paren-free, the same nested-collection shape as
   `def name fn […]`); `def Bar refine Foo {…}` defines a subclass —
   `refine` keeps its general "refine an existing type" meaning, and
   bare/predicate `refine` on scalars is untouched. **No deprecated
   aliases:** `refine Object {…}` is removed outright and raises a
   loud error with a hint pointing at `class {…}`.
3. **Class instances are sealed:** writing an undeclared field raises
   `[aql/sealed_field]` loudly at the `set`. Open dynamic data belongs
   on plain Object.
4. **No prototypes on classes or Objects — delegation is Store's
   job.** A class is a schema (fields + defaults + parent class);
   `make` resolves the full default set eagerly, **flat into the
   instance**. Instances and plain Objects carry one flat field map —
   no `Prototype` link, no delegation at `get` (the instance-side
   `buildBasePrototype`/`GetField` walk is deleted). A delegating
   Object would reintroduce the reads-see-what-enumeration-doesn't
   bug class this design removes; the delegation use cases are owned
   elsewhere — defaults by class schemas, data layering by
   `StructUtil.merge`/`setpath`, chained lookup by **Store** (whose
   copy-on-write parent chain is its identity and the reason it stays
   separate). There is no surface `proto` and no prototype *method*
   dispatch — polymorphism stays with signature dispatch through the
   type lattice. One dispatch mechanism, the same principle ADR-004
   applies to argument collection.

### Context

`Object` was playing three roles at once: class machinery
(`refine Object` + `make` + nominal dispatch), an accidental mutable
bag (undeclared dynamic fields that `set` accepts but enumeration
cannot see), and — internally — the Store/context scope chain. The
voxgig DX reports hit the seams: `make Object {}` rejected with an
unactionable error (B5), computed-key maps forced into workarounds
(T9.1), dynamic fields enumerating as `[]`. The 2026-06-09
copy-returning `set` on Map fixed the immutable column and made the
mutability rule explicit — *mutable containers mutate in place and
return nothing; immutable values return the updated copy* — which is
the rule this symmetry completes.

### Consequences

- `make Object {}` becomes valid (an empty open Object), retiring the
  B5 error-hint proposal in `design/ERRORS.0.md` §4 — resolved by
  design, not by message.
- One clean break instead of a deprecation wave: Phase A lands
  `class` + `refine`-subclassing + sealing + the `refine Object`
  removal together, since every call site must be rewritten anyway.
  README upgrade-note entry in the same change.
- Flat eager-default instances make instance reads a single map
  lookup, and the "dynamic fields invisible to enumeration" bug class
  ceases to exist structurally.
- Methods remain free functions over instances; any future implicit
  receiver must arrive as a signature parameter
  (`design/OBJECT-METHODS.0.md` Option A), never as prototype lookup.
- Open questions tracked in the design doc: List copy-returning `set`
  for column consistency, `make Array` constructibility, the
  `convert` freeze/thaw pair, class introspection rendering.

---

## ADR-006 — Traits: explicit, checkable contracts; never a dispatch mechanism {#adr-006}

**Status:** Accepted (implementation scheduled with generics) · **Date:** 2026-06-09

### Decision

AQL adopts **traits** as named contracts over the existing open
multimethods (design in `design/TRAITS.0.md`):

1. `def Shape trait { area: fn [[Self] [Float]] … }` declares a
   contract — full signatures with a `Self` placeholder, not bare
   word names.
2. Conformance is **explicit and checked immediately**:
   `Circle implements Shape` verifies at the declaration that every
   required overload exists with conforming types (Self substituted),
   erroring loudly with the missing list (`[aql/trait_unsatisfied]`);
   on success it registers membership. No Go-style implicit
   structural conformance.
3. A trait is a **lattice type** — the set of conforming types —
   usable in signature slots, `is`, the `tor`/`tand`/`tnot` algebra,
   and as the referent of generics' `T extends C` constraint
   (`design/GENERICS.0.md`, which used `Comparable` as an example
   constraint without defining it; traits are that definition).
4. **Traits constrain and check; they never dispatch.** Calling a
   required word on a trait-typed value is ordinary signature-matched
   multimethod dispatch — one dispatch mechanism, the same principle
   ADR-004 applies to argument collection and ADR-005 to prototypes.
5. **No default method bodies, no state in v1** — pure contracts;
   mixin defaults could be added compatibly later, not removed.

### Context

The dispatch half of typeclasses already exists: separate `def`s of
one fn name merge into an open multimethod table, and the
`tor`/`tand`/`tnot` algebra makes types-as-sets native. What was
missing is the contract half — a named operation bundle, a
completeness check at the site where conformance is intended (rather
than `no matching signature` at a distant call site), and a type
meaning "anything satisfying this" for parameter slots and generics
constraints. `behave` proved the explicit-implementation pattern for
the four kernel capabilities; traits generalise the *shape* of that
idea to user-defined vocabularies without touching kernel hooks.

### Consequences

- Implementation rides the generics phase; until then the design
  costs nothing and `extends` has a defined referent.
- Check mode gains real types from trait-typed carriers (trait sigs
  with Self substituted), instead of degrading to Any.
- Open questions parked in the design doc: super-traits, orphan
  declarations, blanket conformance (generics-dependent), and which
  standard traits ship seeded (`Comparable`, …) with the eventual
  behave bridge.
