# Open Words — module-contributed signatures on existing words

Status: **PROPOSAL** (rev 0). Design only — nothing here is landed.
Discussion artifact per the ADR rule (design notes capture discovery;
no ADR entry without explicit maintainer instruction).

One sentence: let a module contribute **new signatures** to an
**existing word** — `add` gaining `[Matrix, Matrix] → Matrix` when
`aql:matrix-util` is imported — under a locked-signature rule that
makes core behaviour unoverridable, so user types participate in the
core vocabulary the way built-in types do.

## 1. Problem

AQL's dispatch is type-directed and openly polymorphic — `add` already
covers numeric addition, string concatenation, Bytes concatenation,
and Date/Duration arithmetic through one signature list. But the
*right to contribute to that list* is closed: only Go code in
`lang/go/native`, at registry build time, can append signatures
(`RegisterNativeFunc` — how `native_bytes.go` adds the Bytes `add`
overload and `native_math.go:130-158` carries the temporal ones).
Nothing at the AQL level can:

- `def add …` raises `[aql/reserved_word]` — built-in words cannot be
  redefined, and there is no separate append path.
- `aql:matrix-util` — a first-party module — could not give its own
  flagship type addition: `(matrix) add (matrix)` is a
  `signature_error`, and the module ships **`mat-add`** instead. The
  `mat-` prefix is the workaround made visible.
- `design/BEHAVIORS.10.md` §"Single dispatch on the LCA" already
  concedes the gap, naming this exact example: cross-type addition
  (Date + CalDuration) "would need either a multimethod-style
  extension or the user attaching the impl to the LCA themselves."
  The behavior system (`behave`) covers compare/format/match — a
  closed capability table — not arbitrary word signatures.

The consequence chain: module types are second-class in the core
vocabulary → modules mint mangled twins (`mat-add`, `MathUtil.add`
variants) → the language reads as if polymorphism stops at the
module boundary, when the dispatch machinery underneath has no such
limit.

The concrete forcing case: the temporal overloads on core `add`/`sub`
live in `native_math.go` today **because there is nowhere else to put
them**, not because core is their principled home. The types must be
global (ordering, equality, wire-stable FixedIDs, cross-module
producers like `aql:io` mtimes) — but the *signatures* are a
time-util concern stranded in core by the missing mechanism.

## 2. Goals and non-goals

Goals:

1. A module can contribute signatures to an existing word — core or
   another module's — visible to its importers.
2. Core semantics are unbreakable: no contribution can change what any
   existing call form means. A program that ran before an import means
   exactly the same thing after it; the import can only make
   previously-erroring calls work.
3. The mechanism is ordinary AQL surface (a word), not a Go
   privilege — while the *protection* (locking) stays host-only.
4. `describe`/`check`/dispatch pick contributions up with no separate
   registration, because they already read the live registry.

Non-goals:

- Replacing or wrapping existing behaviour (no `:around` methods, no
  shadowing). AQL stays minimal here deliberately — see
  BEHAVIORS.10.md §"Behavior name registry as contract".
- Un-importing / retracting contributions (modules cannot be unloaded
  today; contributions share the registry's lifetime).
- Changing `def` — `def <existing-word>` keeps its current
  reserved-word / shadowing rules. Extension is a distinct operation.

## 3. Mechanism

### 3.1 Locked signatures (host-only)

Every signature carries a `Locked bool`. All signatures registered
through the native Go paths (`RegisterNativeFunc`, module wrapper
construction, kernel registration) are locked. **Locking is not an
AQL language ability** — there is no word that sets it. It is a
property of the host registration layer, exactly like capability
flags: AQL programs live inside it, they don't wield it.

The invariant locking buys: a locked signature can never be removed,
replaced, reordered, or overlapped. `reserved_word` today protects
whole words; `Locked` refines the protection to the signature level so
the word itself can open up.

### 3.2 The `extend` word

```
extend add fn [[a:Matrix b:Matrix] [Matrix] [ …impl… ]]
```

`extend <word> <fn>` appends the fn's signatures to `<word>`'s
signature list in the **current registry**, subject to the checks in
§4. The fn is an ordinary `fn` value — multi-sig fns contribute each
sig; closures capture normally. Contributed signatures are unlocked
(a later `extend` cannot overlap them either — the non-overlap rule
is universal — but tooling may distinguish locked/contributed
provenance).

At the top level, `extend` acts immediately and registry-wide, like
`def`. Sub-engines (`do`, `each`, `await`) inherit it through the
registry chain, like every binding.

### 3.3 Module scoping and the contribution channel

**Confirmed behaviour today:** a module body runs in its own
sub-registry; its `def`s are module-private (`import module [def priv
42 export "M" {}] priv` → `undefined_word`), and exported fns execute
against the captured sub-registry (the FnDef wrapper carries
`Registry: subReg`). So a naive `extend` inside a module body would
land in the sub-registry and be invisible to importers.

Bytes never faces this because **Bytes is not a module**: its `add`
overload rides `RegisterNativeFunc` at *base-registry construction* in
`lang/go/native`, so every registry is *born* with the signature.
Nothing is exported; there is no channel to ride. That is exactly the
Go privilege this design removes the need for.

The design therefore gives contributions the same two-sided treatment
module exports already get:

1. `extend` inside a module body applies to the module's sub-registry
   immediately (code later in the body sees it), **and** records a
   `SignatureContribution{Word, FnDef}` on the `ModuleDesc`.
2. `import` installs each recorded contribution into the **importing
   registry**, alongside the namespace binding — running the same §4
   checks against the importer's live signature lists.
3. The contributed fn keeps `Registry: subReg`, exactly like an
   exported wrapper — its body resolves module-private helpers even
   though its *signature* is attached importer-side.

This mirrors the existing wrapper mechanism precisely; the only new
part is the attachment point (a word's signature list instead of the
namespace map). No `export` keyword is needed: an `extend` in a module
body is by nature for consumers — a module-private overload of a
public word would be a trap, so contributions always ride the
descriptor.

Transitivity: if module A imports module B (gaining B's
contributions in A's sub-registry), importing A does **not**
re-install B's contributions — they arrive only via importing B.
Modules that need their dependency's contributions visible to their
own callers re-export by importing B in the caller's context being
documented, or the contribution rides only its owning module. (Open
question §9.2 — mirror of how nested imports behave for names.)

## 4. Conflict rules

### 4.1 Non-overlap (hard error)

At install — `extend` at top level, or import-time for contributions —
each new signature's argument-type tuple must not **unify-overlap**
any existing signature of the word, locked or contributed. Overlap
means: some value tuple could match both (checked with the existing
unifier — subtype relations included, so `[Number Number]` overlaps
`[Integer Integer]`). Violation raises `[aql/extend_conflict]` naming
both signatures and both provenances, at the import site.

Non-overlap is what makes rules 2 and 3 in §2 hold: dispatch order
between contributions becomes semantically irrelevant (only
error-message-relevant), and no existing call can change meaning.

### 4.2 Piracy and per-module nominal identity — prerequisite bug (FIXED)

The classic multimethod hazard (Julia's type piracy, Haskell's orphan
instances): module X contributes `add [Foo Bar]` where it owns neither
Foo, Bar, nor `add`, colliding with module Y doing the same. AQL's
nominal doctrine *should* dissolve most of this: `Foo` minted by one
module is a different lattice node from `Foo` minted by another, so
two honest modules' contributions on their own types cannot overlap
even with identical spellings.

**Confirmed broken at rev 0; fixed alongside this note.** The
doctrine held within one registry
(`def A (refine Integer) def B (refine Integer) A teq B` → false) but
**fails across module boundaries**:

```
import module [def Foo (refine Integer) export "M1" {Foo: Foo}]
import module [def Bar (refine String)  export "M2" {Bar: Bar}]
M1.Foo teq M2.Bar                 # returned true — WRONG (refine of
                                  # Integer identical to refine of String)

import module [def Foo (refine Integer) export "M1" {Foo: Foo}]
def A (refine String)
A teq M1.Foo                      # returned true — WRONG (first
                                  # top-level mint after the import
                                  # collided too)
```

Cause: `TypeTable.mintID` derived IDs from a strictly **per-table**
counter, and every sub-registry forked for a module body started from
the parent's count — so the Nth mint in any two sibling registries got
the same ID, and identity (`teq`, the nominal `is` walk, dispatch) is
ID-based. This had to be fixed before open words is sound — the
non-overlap check in §4.1 compares types by identity, and colliding
identities would let one module's contribution silently capture
another module's types.

**The fix (landed with this note):** the mint counter is shared **per
registry tree** — module sub-registries adopt the importing tree's
counter (`TypeTable.AdoptSeqFrom`, called by `RunModuleBody` and
`BuildIOModule` for its StreamKind mint), concurrent forks share it
(`CloneDynamic`), while rollback sandboxes **copy** it (`Clone`) so
their discarded mints don't shift later IDs — which is what keeps a
check-mode pass and a plain run of one program minting identical IDs
(the type-soundness ratchet compares the two engines by identity).
Deliberately per-tree rather than process-global: dynamic IDs stay a
deterministic function of the program. Pinned in
`eng/go/mintid_test.go` and `lang/spec/module-instance.tsv` §7. Known
residual: two *unrelated* engines in one process can still mint
colliding IDs; hosts exchanging Values across engines is out of scope
(and was never sound).

### 4.3 Orphan rule (advisory)

With per-module identity fixed, overlap between honest modules can
only occur when a contributed tuple mentions **only shared (core)
types** — e.g. two modules both contributing `pretty [Atom]`. The
non-overlap check already turns that into a loud import-time error,
so a hard orphan rule ("at least one argument position must be a type
the contributing module minted") is not required for soundness — and
a hard rule would block the flagship migration, since the Time types
are globally registered rather than minted by `aql:time-util`.

Recommendation: **hard non-overlap, advisory orphan** — `aql check`
emits a non-gating `extend_orphan` advisory when a contribution
mentions no module-owned type, mirroring `forward_strands_operand`'s
tone. First-party modules contributing on the global types they
conceptually own (time-util on `Scalar/Time/*`) suppress it via a
declared-ownership list on the module descriptor, if wanted later.

## 5. Dispatch, ordering, forward collection

- Signature match order: locked signatures first (registration
  order — today's behaviour, byte-for-byte), then contributions in
  install order. Non-overlap makes the contribution order
  unobservable except in error messages.
- Forward collection is type-directed, so a contributed signature
  changes a word's reach **only where values of the contributed types
  appear** — which is the intended semantics, not a hazard. A program
  with no Matrix values parses and collects identically before and
  after `import "aql:matrix-util"`.
- `execFnDefLiteral`'s trivial-delegation short-circuit and
  `matchSignature` need no changes: contributions are ordinary
  signatures on the word's list.

## 6. Tooling

- **`describe`** already reads the live engine; contributed
  signatures appear automatically. Add provenance to the rendering:
  `[ [Matrix Matrix] Matrix ]  (via aql:matrix-util)`.
- **`check`** is registry-driven and follows imports, so
  contribution-aware checking works without new machinery; the
  `extend_conflict` / `extend_orphan` diagnostics are new.
- **Bytecode compiler**: fold sites for foldable words must consult
  the word's live signature list rather than any baked table, and
  must refuse to fold a call that matches a contributed sig whose fn
  isn't compilable — the existing interpreter-fallback covers it.
  This is the one real implementation cost outside the registry.

## 7. Migration candidates

Once landed, in order of payoff:

1. **Temporal `add`/`sub` overloads** (`native_math.go:130-158`) →
   contributions of `aql:time-util`. The Time *types* stay globally
   registered (ordering, equality, FixedIDs, io/log producers — see
   the type/constructor split already in force). `aql:io` imports
   time-util so mtime arithmetic keeps working out of the box.
2. **`MatrixUtil.mat-add` / `mat-mul` / `mat-emul`** → `extend add`
   / `extend mul` on `[Matrix Matrix]` (keep the `mat-*` names as
   deprecated aliases one release).
3. **Bytes `add`** could move to `aql:bin-util` for symmetry, or stay
   native — Bytes concatenation is arguably as core as String's.
   No forcing argument either way; decide by taste when 1–2 land.
4. **Future micro types** (`Scalar/Micro/*` — see the minilang
   lexer-sugar thread): `add [Money Money]`, `convert` overloads,
   etc., contributed by their defining modules from day one — the
   first consumer that never needs the Go privilege.

## 8. Spec obligations (when implemented)

Per the paired-negative discipline:

- `extend` on a fresh word / core word: contributed call works;
  pre-existing call forms byte-identical (pin a before/after pair).
- Overlap with a locked sig → `ERROR:extend_conflict` (e.g.
  `extend add fn [[a:Integer b:Integer] …]`).
- Overlap between two module contributions → error at second import.
- Module-body `extend` visible to importer; NOT visible without the
  import (negative row in a fresh engine).
- Contribution body using a module-private helper works (sub-registry
  closure semantics).
- Forward-collection row: a contributed-type value is collected, a
  same-shape program without the import errors identically to today.
- Cross-module nominal identity rows from §4.2 (land with the mintID
  fix, before this design).

## 9. Open questions

1. Should `extend` work on module-namespaced words
   (`extend MatrixUtil.mat-add …`) or only registry-visible bare
   words? (Lean: bare words only; namespaced words belong to their
   module.)
2. Transitive contributions (§3.3): does importing A also install
   what A imported from B? (Lean: no — contributions belong to their
   owning module; A re-exporting is a separate, explicit feature if
   ever needed.)
3. Does `extend` require the target word to exist? (Lean: yes —
   extending a missing word is `undefined_word`; creating words is
   `def`'s job.)
4. Ownership declarations for the orphan advisory (§4.3): module
   manifest field, or convention only?
5. The word's name. `extend` is unregistered today, but sits one
   character from the existing gen-bound word `extends`
   (`gen [T extends Comparable]`) — both type-system-adjacent, so a
   typo of one for the other yields confusing errors, and the type
   vocabulary already clusters on e- (`extends`, `exposes`,
   `exclude`, `extract`). Unregistered alternates: **`overload`**
   (names the semantics — appending an overload — and has no near
   neighbour; current lean), `augment`, `contribute`.
