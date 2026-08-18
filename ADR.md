# Architecture Design Record (ADR)

A running list of the key architectural decisions behind boru — the ones
that shape the language and its implementation. Each record is short,
numbered, and dated. Newer decisions may supersede older ones; superseded
records are kept (struck through in status) rather than deleted, so the
history of *why* stays legible.

**Each record is capped at 111 words**, heading and status included. The
cap buys scannability at the cost of the reasoning: the argument, the
measurements and the rejected alternatives live in the `design/*.md` note
each record cites, and the long form as it stood before 2026-08-08 is in
this file's git history.

When you make a decision that future contributors would otherwise have
to reverse-engineer from the code, add a record here.

> **ADRs are added only on explicit maintainer instruction.** Design
> conversations, reviews, and accepted-in-discussion directions live in
> `design/*.md` discovery notes; they do not become ADR entries until
> the maintainer explicitly says to add one.

---

## ADR-001 — Modules must not shadow core words {#adr-001}

**Status:** Accepted · **Date:** 2026-05-30

A native module must never export a name that collides with a core word.
Either extend the core word with a type-dispatched signature, or choose
another export name. One name meaning two things is a foot-gun.

> **Amendment (2026-07-18).** A new signature on a core word — from
> anywhere, core included — must contain at least one **user type**.
> Core-types-only overloads are prohibited: they change the language for
> every program — this record's hazard, arriving through dispatch.
> Earlier reconciliations are grandfathered.

---

## ADR-002 — No implicit broadcasting {#adr-002}

**Status:** Accepted · **Date:** 2026-05-30

boru will not lift a scalar word over an array implicitly. A scalar word
applied to a list where it expects a scalar is a type error, not a silent
element-wise map: write `each [add 10] [1,2,3]`.

Broadcasting cannot be a word — it would wedge into the signature matcher
and change every scalar word at once. It defeats the static checker, since
result rank would then depend on runtime shape. It collides with words
that legitimately take lists. And it buys keystrokes, not power.

Design principle 3 is explicit iteration.

---

## ADR-003 — Every module export must be spec-covered {#adr-003}

**Status:** Accepted · **Date:** 2026-06-07

Every word reachable as `Namespace.word` after `import` must be exercised
by at least one `lang/spec/*.tsv` row. `TestModuleExportCoverage`
enumerates the live export set from the module registry — never a
hard-coded list — and fails naming the uncovered.

The unit is the **qualified** name: it is what a user types, and it
disambiguates the cross-module reuse ADR-001 allows. Go tests check the
implementation; the `.tsv` suite is the contract for the imported,
dot-accessed surface.

`hermeticExempt` covers the few exports no deterministic row can drive.

---

## ADR-004 — All words are forward by default {#adr-004}

**Status:** Accepted · **Date:** 2026-06-09

Every word is forward-collecting, so the canonical form is
`word arg1 arg2 …` and written order matches declared order. The sole
exception is the Forth stack vocabulary (`dup`, `swap`, `drop`, `rot`),
whose entire meaning *is* the stack.

This is a cultural default. New words ship forward-eligible and per-word
flips are rejected; the levers are the call-site modifiers and grouping.

> **Amendment (2026-07-30).** `uncalled_function` raises at the call
> site at runtime, not as a check-mode advisory.

> **Amendment (2026-08-15).** Four categories, not one exception: `|`
> splits signatures into forward-eligible, mixed-barrier, stack-only,
> quoting slots. [ADR-004-REFINEMENT.0.md](design/ADR-004-REFINEMENT.0.md)

---

## ADR-005 — No deliberate panics {#adr-005}

**Status:** Accepted · **Date:** 2026-06-11

The implementation never panics deliberately. Every error condition —
including build-time programmer errors such as a malformed type path or a
duplicate `FixedID` — is a returned `error` surfaced at a checkable
boundary. The init-time carve-out is withdrawn: registration errors
accumulate and surface at the first registry construction.

The engine is embedded in long-lived hosts, where a panic takes the host
down. "Allowed only at init" was a comment convention no compiler checked,
and it concealed a real nil-map panic.

Remaining: stdlib `Must*` on constants, and `recover()` in tests.

---

## ADR-006 — Vault slots use a backend-agnostic envelope {#adr-006}

**Status:** Accepted · **Date:** 2026-06-15

The vault supports named, scoped passwords ("slots"). Each namespace has
a random data key (NDK); every secret is sealed under its NDK before
reaching any backend, so backends hold only ciphertext. Each slot owns an
X25519 keypair, and granted NDKs are sealed to its public key.

Namespace isolation is cryptographic. `scope` is bound into the slot's
verifier, so editing it fails authentication rather than escalating; the
namespace list is deliberately unbound, letting an admin reassign
namespaces without the holder's passphrase.

Legacy vaults migrate rather than reformat; Feature B needs slots.

---

## ADR-007 — No secondary parsing {#adr-007}

**Status:** Accepted · **Date:** 2026-06-29

A word must not define a custom sub-language it parses out of a token
stream or a value's text. Any structure it consumes must be ordinary boru
**Node** data — `List`/`Map` of plain scalars — that it only reads,
never re-lexes or string-splits.

The contract: every accepted structure is macro-constructable and
JSON-representable. A terse surface is earned in the one parser, or a
macro expanding to Node data. A second grammar cannot be built or
inspected as data.

Switching on a discriminator field is not parsing.

---

## ADR-008 — 100% Go coverage of reachable code, always {#adr-008}

**Status:** Accepted · **Date:** 2026-07-07

`make cover-gate` fails below 100% of reachable statements. It runs every
module with `-coverpkg` spanning the repo and merges the profiles
block-by-block, so a statement counts when **any** suite reaches it. To
make statements reachable the codebase admits mocking seams
(`design/TEST-SEAMS.10.md`).

Partial floors invite ratchet erosion; the 2026-07 program found shipped
bugs living only in uncovered code.

One exclusion: an inline `//covergate:allow <reason>` on a genuinely
unreachable **defensive** guard. Dead code is removed instead. The gate
fails on a stale or now-covered pragma.

---

## ADR-009 — Vault formats are versioned, incompatible by refusal {#adr-009}

**Status:** Accepted · **Date:** 2026-06-10 · **Renumbered** 2026-07-18

Every persistent vault artifact carries an explicit format version.

Older than the binary → **migrate forward** through an ordered, pure
migration. Newer than the binary → **refuse**, never parse leniently:
`encoding/json` drops unknown fields, so a load-then-save would erase data
the binary does not understand.

Bumping a version is a three-part commit — constant, migration, golden
fixture plus test — with `TestMigrationRegistryLength` keeping them in
lockstep. Live wire protocols are versioned the same way.

---

## ADR-010 — Types are values {#adr-010}

**Status:** Proposed · **Date:** 2026-07-31 · `design/NUR-RESOLUTION-PLAN.0.md`

A type literal is a **value**, usable anywhere a value may appear. No
layer may treat it as declaration-only syntax.

It is the **singleton** inhabiting that type position, so
`Integer eq Integer` is true while `0 eq Integer` is false: the operands
are a scalar and a type, a value-vs-value mismatch, not evidence that
types aren't values. Cross-type comparison is `teq`.

Interpreter, checker and compiler honour this identically; a compiler
refusal is a bug. The emitter interns bare type nodes, nested included.

---

## ADR-011 — One Function type {#adr-011}

**Status:** Accepted · **Date:** 2026-07-31 · with NUR050 ·
**Amended:** 2026-08-17 (maintainer): the final sentence's fourth clause
("a bare fn name before a `Function`-typed slot resolves as a reference")
is struck, per clause 2 of the 2026-08-16 `/r` ruling
(`design/FUNCTION-VALUE-SCOPE.0.md` §12.4). Passing a function as an
argument requires `/r`; the bare-name-calls rule now applies universally,
with no slot-typed exception.

There is exactly one function type, `Type/Function`. `Word/__FN`
(FixedID 23) is retired, never recycled; the TS twin moved in lockstep.

A function value is always the inert, referenceable thing — **calling is
an act of the use site**. The discriminators are the name/value
distinction and the transient `Quoted` flag, never the Parent type. A bare
name bound to a function calls; a value at the pointer dispatches; `/r`
takes the reference and is no collection barrier.

---

## ADR-012 — The kernel is mechanism, never content {#adr-012}

**Status:** Accepted · **Date:** 2026-08-03

`eng/` holds mechanics only: no registrations, no domain types.

1. **Type residence is three-way:** mechanism types in eng, global types
   in a middle component, module-scoped in `boru:*`.
2. **Algorithms are mechanism,** even serving one word.
3. **The kernel is name-blind:** word facts come from structural markers,
   declared properties, role bindings, never string literals.
   > **Amendment (2026-08-04).** The parser emits no names, only markers
   > lowered by the sugar-role table.
4. **The parser is type-name-opaque:** capitalised names are ordinary
   Words; the engine resolves them.
5. **Capability over enumeration:** facilities are granted by property,
   never name lists.

---

## ADR-013 — `basic` is a component below `lang` {#adr-013}

**Status:** Accepted · **Date:** 2026-08-04

`basic/` is the base language layer: the fundamental words (stack,
definition, control-flow, type-generics) and the predefined global content
types, moved out of `lang`. It registers against the kernel, never being
kernel machinery. It depends on the pieces it uses and nothing else, and
`basic/go/depsgate_test.go` pins the set.

> **Amendment (2026-08-07).** "`eng` only" was measured false: zero
> symbols. `compiler` went too, its seam gap closed by widening
> `core.EmitRecorder`. `check` kept.

> **Amendment (2026-08-08).** `check` removed too; the set is `core` +
> `parser`. Core-typed primitives moved down, two pass drivers became S1
> slots. [BASIC-CHECK-CUT.0.md](design/BASIC-CHECK-CUT.0.md)

---

## ADR-014 — Tabnas parser defects are fixed upstream, never worked around {#adr-014}

**Status:** Accepted · **Date:** 2026-08-10

A defect in the `tabnas/parser` + `tabnas/jsonic` twins — lexing, token
boundaries, values, concurrency — is reproduced against the BARE
dependency, reported, fixed upstream, consumed as a version bump. Never a
boru-side shim: a shim doubles the code paths, hides the bug from the other
port and every other consumer, and rots into dead weight the moment
upstream moves. `scripts/parity-probe.sh` is the instrument. Divergences
from boru's OWN grammar layer stay boru's, recorded in `NUR.md`. A
temporary shim needs its upstream issue linked in the comment and a NUR
record holding it open.
[TABNAS-UPSTREAM-FIRST.0.md](design/TABNAS-UPSTREAM-FIRST.0.md)

---

## ADR-015 — Canon always round-trips {#adr-015}

**Status:** Accepted · **Date:** 2026-08-15

`canon v` renders boru source that re-parses to a value `deq` to `v`.
Every value; no exempt kinds. A rendering no parser accepts — a debug
spelling, a struct dump, a pointer address — is a defect, not a display
choice: canon is the serialisation boundary, so a value that cannot be
written cannot be moved, stored, or compared across ports. Exempting
the awkward kinds would exempt exactly the interesting values.
A property gate over the spec corpus enforces it in both
ports, landing with a shrinking ledger of failing kinds. NUR031
(fn/host `deq` reflexivity, name-independent fn canon) is a
prerequisite.
[CANON-ROUNDTRIP.0.md](design/CANON-ROUNDTRIP.0.md)

---

## ADR-016 — Arity and origin never change function behaviour {#adr-016}

**Status:** Accepted · **Date:** 2026-08-15 · sharpens ADR-011

Every function behaves the same way whatever its arity and wherever it
came from — named or anonymous, module or local, boru or native.
ADR-011 fixes the rules; this record forbids *exceptions* keyed on arity
or origin. Two exist today, and are defects rather than design: `/r`
fails to park a 0-arg fn bound to a param, yielding the call's result
where the author asked for the function; and `execFnDefLiteral` treats a
0-arg **anonymous** value as data where a named one dispatches.
[FUNCTION-VALUE-SCOPE.0.md](design/FUNCTION-VALUE-SCOPE.0.md)

---

## ADR-017 — Diagnostics show the values {#adr-017}

**Status:** Accepted · **Date:** 2026-08-18

An error names the values it is about. `f: expected 1 return value(s),
got 2` reports arithmetic and withholds the one thing that identifies
the fault — which second value appeared. Show them:
`got 2 — [1 {i:Integer}]`. Abbreviate long runs with an explicit
elision; never drop the values to keep the line short. Where a message
reports a count too, the rendered list carries exactly that many
entries, and every engine builds the text once, so interpreter, VM and
checker agree.
[DIAGNOSTIC-VALUES.0.md](design/DIAGNOSTIC-VALUES.0.md)
