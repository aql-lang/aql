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

**Status:** Accepted · **Date:** 2026-07-31 · with NUR050

There is exactly one function type, `Type/Function`. `Word/__FN`
(FixedID 23) is retired, never recycled; the TS twin moved in lockstep.

A function value is always the inert, referenceable thing — **calling is
an act of the use site**. The discriminators are the name/value
distinction and the transient `Quoted` flag, never the Parent type. A bare
name bound to a function calls; a value at the pointer dispatches; `/r`
takes the reference and is no collection barrier; a bare fn name before a
`Function`-typed slot resolves as a reference.

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

> **Amendment (2026-08-07).** "`eng` only" was measured false: it carried
> zero symbols. `compiler` went too, a seam gap closed by widening
> `core.EmitRecorder`. **`check` stays** — control words have an analysis
> half with no correct inactive default.

> **Amendment (2026-08-08).** **`check` goes too; the set is now `core` +
> `parser`.** The previous amendment's premise did not survive inspection.
> It read basic's 23 check symbols as the checker's vocabulary; 21 of them
> were pure functions over CORE types — Values, Types, `r.Defs`, and
> `CheckState`, all core-owned — that were merely filed in `check/go`. They
> were **moved down**, not forwarded: the join lattice
> (`core/go/carrier_join.go`), the body runners (`carrier_body.go`), guard
> narrowing (`guard_narrow.go`, `guard_predicate.go`), dead-overload
> detection (`deadsig.go`), the spread carrier (`carrier_spread.go`), the
> typed-def make record (`record_typed_def.go`), and the deduping
> diagnostic emitters (into `check_state.go`, beside the state they read).
> `core.JoinCarriersHook` and the `AnalysisImpl.AddUnique` slot were
> retired in the same move — with their subjects core-resident, there was
> nothing left to indirect.
>
> The real remainder is two symbols, `AnalyseFnBody` and `AnalyseLoopBody`
> — the analysis **pass** itself (memoised per call shape, recursion-
> bailing, quota-capped, Kleene-iterated). They stay in `check` behind S1
> slots reached as `core.RunFnBodyAnalysis` / `core.RunLoopBodyAnalysis`.
> That is a seam rather than a mailbox because a check-less build runs no
> analysis at all: `AnalysisImpl.ReturnsFn` returns nil, so no `ReturnsFunc`
> executes and neither accessor is reached. The nil defaults are the same
> no-analysis regime every other S1 slot defines, and nil is in-band
> anyway — `AnalyseFnBody` already documents an empty result as "the
> analyser aborted, treat as an Any carrier".
>
> The general rule this settles: **a word library defines language types
> and words, and the interpreter's own primitives are enough for that.**
> Where a word has an analysis half, that half belongs in core's carrier
> vocabulary; only a driver of the pass itself justifies a slot. Neither
> justifies a dependency edge.
