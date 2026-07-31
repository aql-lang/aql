# Architecture Design Record (ADR)

A running list of the key architectural decisions behind boru — the ones
that shape the language and its implementation, with the reasoning that
led to them. Each record is short, numbered, and dated. Newer decisions
may supersede older ones; superseded records are kept (struck through in
status) rather than deleted, so the history of *why* stays legible.

When you make a decision that future contributors would otherwise have
to reverse-engineer from the code, add a record here.

> **ADRs are added only on explicit maintainer instruction.** Design
> conversations, reviews, and accepted-in-discussion directions live in
> `design/*.md` discovery notes; they do not become ADR entries until
> the maintainer explicitly says to add one.

---

## ADR-001 — Native modules must not shadow core words {#adr-001}

**Status:** Accepted · **Date:** 2026-05-30

### Decision

A native module (`boru:math`, `boru:array-util`, `boru:matrix-util`, …) must **never
export a name that collides with a core (built-in) word**. If an
operation would naturally share a core word's name, do one of the
following instead:

1. **Extend the core word** with an additional type-dispatched signature,
   when the operation is a genuine variant of it (subject to the
   2026-07-18 amendment below); or
2. **Choose a different export name** for the module word.

> **Amendment (2026-07-18) — new core-word signatures require a user
> type.** Option 1 is now constrained: a **new signature added to a
> core word — from anywhere, core included — must contain at least one
> user type** (a non-core type: module-registered such as `Tensor`, or
> user-defined via `refine`/`class`). Adding a new signature built
> **only from core types is prohibited** — the core-types-only dispatch
> surface of an existing core word is frozen. Rationale: a
> core-types-only overload changes the core language for *every*
> program — the same silent-shadowing hazard this record exists to
> prevent, arriving through dispatch instead of names — whereas a
> signature anchored on a user type fires only when that type is in
> play. The pre-amendment reconciliations recorded below (e.g. the
> `flatten -1` / `flatten N` depth overloads) predate this rule and are
> grandfathered.

### Context

boru resolves words by signature and has no implicit `Word → Atom`
fallback. When a module exports a name that also exists as a core word,
two different operations end up wearing the "same" name, distinguished
only by a `boru:array-util`-style prefix. That is confusing in exactly the
case it matters most: when both apply to the *same* value type but mean
different things.

The motivating case was the array vocabulary. Three array operations had
been given `arr-`-prefixed built-in names (`arr-flatten`,
`arr-transpose`, `arr-indexof`) purely to dodge collisions with the core
`flatten` and `indexof`, and the first cut of the `boru:array-util` module
re-exported them as `ArrayUtil.flatten`/`ArrayUtil.indexof`. That meant
`flatten` (core, one level) and `ArrayUtil.flatten` (deep) did *different
things to the same list* — a foot-gun, and a symptom that the boundary
was drawn in the wrong place.

### Consequences

For `boru:array-util` specifically:

- **Deep flatten** is now `flatten -1` — a negative depth on the core
  `flatten` word (which removes one level by default, or `N` levels with
  `flatten N`). There is no `ArrayUtil.flatten`.
- **List lookup** is `ArrayUtil.indices` — a distinctly-named array word
  (for each needle, its index in the haystack, or `-1` when absent). There
  is no `ArrayUtil.indexof`.

  > **Amendment (2026-06-07).** This was originally folded into the core
  > `indexof` word as a `[List, List]` overload. Two later changes undid
  > that: `indexof` itself moved out of core into `boru:string-util`
  > (`StringUtil.indexof`, string-only), and overloading one word across
  > two unrelated domains proved a smell — the string form returns a
  > scalar with `-1`-when-absent, while the list form returns a vector
  > with a *different* absent sentinel. The list form is now its own word,
  > `ArrayUtil.indices`, in `boru:array-util`, with `-1` for an absent
  > needle (consistent with the string form's not-found value). This still
  > honours the ADR: `indices` shadows no core word.
- **`transpose`** has no core counterpart, so it keeps its plain name and
  remains `ArrayUtil.transpose`. The `arr-` workaround names are gone.

After this, the `boru:array-util` export set shares no name with any core word.

### Applied to `boru:matrix-util`

The `boru:matrix-util` module predated this record and exported `size`,
`flatten`, and `transpose`. These have been reconciled:

- **`size`** — dropped. The core `size` word already reports a tensor's
  entry count via the Sizer behavior (`TensorData`), so a `MatrixUtil.size`
  export only shadowed it.
- **`flatten`** — renamed to **`MatrixUtil.values`** (the row-major list of
  entries). The core `flatten` word remains the only `flatten`.
- **`transpose`** — kept. `transpose` is *not* a core word; it lives in
  the `boru:array-util` module. `MatrixUtil.transpose` and `ArrayUtil.transpose` are
  two namespaced module words, which this rule permits — the rule is
  about shadowing *core* words, not other module words.

After this, no module export shadows a core word.

---

## ADR-002 — No implicit broadcasting {#adr-002}

**Status:** Accepted · **Date:** 2026-05-30

### Decision

boru will **not** implement broadcasting — the implicit lifting of a
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
NumPy/APL) but a poor fit for boru:

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
`string-util` in part, and the boru-implemented `decision`/`report`/`test`
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
  `mem://` scheme or a deterministic validation-error row (as the `boru:net`
  `prepare`/`direct` rows do) before reaching for an exemption.

---

## ADR-004 — All words are forward by default {#adr-004}

**Status:** Accepted · **Date:** 2026-06-09

### Decision

Every boru word is **forward-collecting by default**: a word looks ahead
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

boru is a concatenative language with a stack, but it deliberately does
not *read* like Forth. The §1.4 sig-order unification (see lang/go/
CLAUDE.md "Argument Ordering") made one rule govern every word, with
the forward phase first and the stack as fallback; the mirror
equivalence `f a b ≡ b f a ≡ b a f` means pipeline code and call-style
code are the same word. Both DX field reports
(`design/VOXGIG-DX-REPORT.5.md`, `design/BORU-DX-REPORT.5.md`)
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
  (or diagnose via `boru check`) without changing any word's default;
  see `design/ERRORS.8.md` §"Chained forward calls".
- **Mixed-form calls are user error territory, diagnosed not blessed.**
  Forms that split one call's args across both sides without grouping
  (`(x 3 gt) if [a] [b]` — VOXGIG T9.4) are not given bespoke per-word
  semantics; the investment goes into `check`-mode advisories
  (`forward_strands_operand`, `uncalled_function`) that catch the
  stranded shapes.

  > **Amendment (2026-07-30) — `uncalled_function` is an error in both
  > surfaces, not a check-mode advisory.** A call on a function VALUE (a
  > module export reached through `dot`, a `usurp`ed value, a `def`ed fn)
  > whose arguments match no signature now RAISES at the call site at
  > runtime, exactly as a plain word's `no_signature` does; check mode
  > reports the identical finding at the identical place. The previous
  > design left the value on the stack and judged it as end-of-run
  > residue, which made the stranded shape *silent* whenever anything
  > consumed it — `print (IO.read "/nonexistent")` printed the function
  > and exited 0. Passing a function as data is now said explicitly
  > (`name/r`). This narrows the bullet above rather than reversing it:
  > `forward_strands_operand` remains an advisory, and no word's
  > collection default changed. See `design/FN-VALUE-DISPATCH.0.md`,
  > which supersedes `design/ERRORS.8.md` §5 option 2.
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

## ADR-005 — No deliberate panics; infrastructure code always returns errors {#adr-005}

**Status:** Accepted · **Date:** 2026-06-11

### Decision

The boru implementation **never panics deliberately**. Every error
condition — including build-time programmer errors such as a malformed
type path or a duplicate `FixedID` — is reported as a returned `error`,
surfaced at a checkable boundary, not raised as a `panic`. This is
infrastructure code: a panic crashes the host process that embeds the
engine (a server, an editor's LSP, a supervisor), which is never an
acceptable outcome for a library.

Concretely:

- Runtime handlers return errors (the existing "Panic Prevention" rule).
- **Init-time type registration also returns errors.** The previous
  carve-out — `// lint:allow-panic` on the hardcoded type-registration
  paths — is withdrawn. Those helpers now record the error into a
  package-level accumulator and the engine surfaces it the first time a
  registry is constructed:
  - `eng`: `BuiltinInitError()` is checked by `NewRegistry()`.
  - `native`: `TypeInitError()` is checked by `DefaultRegistryWithPolicy`.
  - `modules/matrix`: `tensorTypeInitErr` is checked by
    `BuildMatrixModule`.
- The well-known `T*` constant resolver (`mustType`) records its error
  and returns a degenerate placeholder `*Type` so other package-level
  `var` initialisers stay non-nil; the recorded error is then returned by
  `NewRegistry` before the registry is ever used.

The only remaining `panic`/`recover` in the codebase are: Go standard-
library `Must*` calls on compile-time-constant inputs (e.g.
`regexp.MustCompile` of a literal pattern), and `recover()` guards in
tests that *assert* no panic occurs. Neither is a deliberate failure
signal in our own logic.

### Context

The kernel historically permitted init-time panics on the type-
registration paths, reasoning that a `FixedID` collision or a bad path is
a build-time programmer error caught by tests, not a runtime condition.
That reasoning has two holes:

1. **The engine is embedded.** `lang.New()` is called from long-lived
   host processes. A panic in a package `init()` or in `NewRegistry`
   takes the whole host down with a stack trace instead of a handled
   error the host can log and refuse cleanly.
2. **The carve-out leaked.** "Allowed only at init time" is not a
   property the compiler checks; it relied on a comment convention.
   Auditing the tree turned up a genuine latent nil-map panic in
   `TypeTable.RegisterExternalBuiltin` (writing the builtin-only indexes
   on a dynamic table), reachable by any caller — exactly the class of
   bug a blanket no-panic rule prevents.

A single rule ("return errors, always") is simpler to follow and to
audit than a rule with an init-time exception, and it composes with the
error-returning constructors the codebase already has.

### Consequences

- `NewRegistry`, `DefaultRegistry[WithPolicy]`, and `BuildMatrixModule`
  already return `error`; the registration errors ride those existing
  channels, so no new failure surface is introduced for callers.
- A malformed `builtinDecls`, a duplicate external `FixedID`, or a bad
  registered path now fails the *first registry construction* with a
  descriptive error, instead of panicking during package import. Tests
  assert this (`eng/go/registration_error_test.go`), including a
  `recover()`-based guard that the conversion holds.
- New externally-registered types must still pick a unique `FixedID`
  from their documented range; the difference is only in how a mistake
  is reported.
- The `// lint:allow-panic` annotations are removed. Any new `panic` in
  non-test, non-`Must*`-constant code is a defect.

---

## ADR-006 — Vault scoped passwords use a backend-agnostic envelope; Feature B activates only with slots {#adr-006}

**Status:** Accepted · **Date:** 2026-06-15

### Decision

The vault supports multiple **named, scoped passwords** ("slots") over a
backend-agnostic **envelope**, and the content-versioning/integrity layer
(Feature B) activates only once a vault has slots.

1. **Envelope key hierarchy.** Each namespace has a random
   **namespace data key (NDK)**, identified by an 8-byte id. Every secret
   *value* is sealed (`"BORUE" | format | ndkID | nonce | ct`, AES-256-GCM,
   AAD binding `ndkID|namespace|alias`) under its namespace NDK *before*
   it reaches any storage backend — so backends become opaque ciphertext
   stores. Each **slot** owns an X25519 keypair: the private key is
   AES-GCM-sealed under a per-slot KEK = `HKDF(scrypt(passphrase,
   Store.VaultSalt), slot.Salt)` (one scrypt per authentication regardless
   of slot count), and each granted NDK is `nacl/box`-sealed to the slot's
   public key. The store-integrity HMAC key is the NDK of a reserved
   `@integrity` namespace, sealed to every slot.

2. **What is cryptographic vs policy.** Namespace isolation is
   cryptographic: a slot can only decrypt namespaces whose NDK is sealed
   to it. The `scope` tier (`read|write|move|admin`) is bound into the
   slot's **verifier and private-key AAD**, so editing the plaintext
   `scope` in `vault.jsonic` fails authentication rather than escalating.
   The namespace allow-list is **not** bound into the verifier — it is
   gated by NDK possession (editing the field grants nothing without the
   key), which is what lets an admin reassign namespaces (re-wrapping NDKs
   to a slot's authenticated public key) **without the holder's
   passphrase**. read-vs-write-vs-move within a held namespace is policy +
   audit (symmetric possession can't separate them). A per-slot `PubMAC`
   (keyed by the integrity key) authenticates a slot's public key before
   any admin re-seal targets it.

3. **Migration, not reformat-in-place.** The first `vault password add`
   on a legacy single-passphrase vault migrates it: it generates the
   `VaultSalt` and NDKs, turns the current passphrase into a seed `admin`
   slot, re-seals every secret under its namespace NDK, and adds the
   requested slot — committing the store (the only durable copy of the
   NDKs, via the admin slot) **before** overwriting the keyring. New
   on-disk versions follow ADR-009: `storeVersion` 3 → 4 (with a no-op
   `migrateStoreV3ToV4` and a golden fixture), and the envelope keyring is
   a self-describing `"BORUK" | format=2 | json` container distinct from
   the legacy format-1 encrypted blob.

4. **Feature B (versioning/integrity/history/restore) activates only with
   slots.** `SaveStore` bumps a monotonic `Generation` (only on real
   content change, so a no-op re-save stays byte-identical), writes a
   keyless signature sidecar (`vault.jsonic.sig`), and appends a redacted
   record to an event-sourced journal (`vault.jsonic.log`) — **only** when
   the vault has password slots. A legacy single-passphrase vault is
   written exactly as before: no generation counter, sidecar, or journal.

### Context

The vault was all-or-nothing: hold the one passphrase, get everything.
Delegating a credential to a collaborator or agent — scoped to some
namespaces and to read-only — required a second vault. The asymmetric
slot design is what makes scoped delegation manageable: an admin can
grant or reassign a namespace by sealing its NDK to a slot's *public*
key, so no passphrase-sharing or holder-coordination is needed. The
binding of `scope` (but not namespaces) into the verifier resolves a real
tension surfaced in review: binding namespaces too would lock a holder
out whenever an admin reassigned them, while leaving scope unbound would
let a holder self-escalate by editing a plaintext flag.

Gating Feature B on slot presence resolves two regressions an
unconditional implementation caused: legacy vaults stopped being
byte-identical (breaking the no-op-save guarantee), and the broker's
per-request quota-counter save turned into a multi-fsync + journal-append
hot path. Both vanish when the side artifacts only exist for envelope
vaults, where the richer metadata (slots, wrapped keys) is what makes
versioning and restore worth having.

### Consequences

- The at-rest guarantee is preserved and scoped: no valid passphrase →
  every NDK is unreachable and all values are ciphertext, exactly as
  before; a non-admin password recovers exactly its granted namespaces.
- The keyed integrity HMAC + anti-rollback anchor defend against an
  attacker with no valid passphrase. An authenticated insider is
  constrained instead by the per-slot scope/pubkey bindings, so the
  store-level HMAC was never the control that stops them; the integrity
  key is therefore reachable by every slot (not admin-only), which is
  required for a non-admin write to maintain the keyed layer at all.
- `restore` reinstates the *metadata* layer (aliases, agents, lock state,
  a monotonic capability merge) from a past generation while preserving
  the live password slots, namespace keys, and keyring values — it is a
  last-known-good recovery for `vault.jsonic`, not a secret-value rewind.
- The scoped-password feature requires the file backend's envelope; a
  keychain-backed vault that gains slots double-wraps (OS store of
  envelope ciphertext) and now requires a boru passphrase to decrypt.

---

## ADR-007 — No secondary parsing; every boru structure is macro-constructable Node data {#adr-007}

**Status:** Accepted · **Date:** 2026-06-29

### Decision

A word **must not** define a custom sub-language that it parses out of a
captured token stream or out of the text of a value. Any structure a word
consumes must be an ordinary boru **Node** (a `List`/`Map` of plain scalars)
that the word only **reads** — never re-lexes, re-parses, or string-splits.

Concretely, the contract is: every structure accepted by a word must be

1. **constructable by a macro** — a macro emits boru data (`quote`/`unquote`),
   so any structure a macro can produce is, by definition, plain Node data;
   and
2. **JSON-representable** — expressible with maps, lists, strings, integers,
   booleans, so it round-trips through serialisation unchanged.

If a feature wants a terse surface syntax, it earns it through the **one**
parser (a grammar rule / lexer matcher in `eng/go/parser`) or through a
**macro** that expands to Node data — not through a private parser hidden
inside a word's handler.

### Context

The motivating case is the `Bytes` bit-syntax. `make Bytes [spec]` accepted
a spec like `[ver:u8 len:u16 body:bytes(len)]` and a hand-rolled parser
inside the handler interpreted it: it `strings.Split`- on `/` to separate a
type from its `be`/`le`/`signed` modifiers, positionally attached a trailing
`(len)` ParenExpr to the preceding segment, and `strconv.ParseInt`-ed each
map key to decide literal-vs-name. That is a second parser — a sub-language
with its own grammar — living below the real one.

Secondary parsing is corrosive: the structure it accepts can't be built or
inspected as data (so no macro can emit it, and it can't be serialised),
its grammar drifts from the host language's, and its rules (here, "a numeric
`/N` is the arity modifier so sizes must use parens") are accidents of the
token stream rather than deliberate design. boru already has exactly one
parser and a uniform type/`make`/`refine` model; a per-word DSL undercuts
both.

The fix for `Bytes` is to make the spec a `List` of segment `Map`s —
`[{name:'ver' type:'u8'} {name:'body' type:'bytes' size:'len'}]` — that the
handler reads field by field (enum-dispatch on the `type` string is data
interpretation, not parsing). A follow-on question (whether such a layout
should be a named **type**, instantiated by plain `make`/`unpack`, rather
than an anonymous argument) is tracked separately in
`design/go-modules/BYTES.10.md`; this ADR governs only the no-secondary-
parsing principle, which holds either way.

### Consequences

- The `Bytes` spec is now a JSON-representable `List<Map>` read by
  `readBitSegments` (`lang/go/native/native_bytes.go`); the old
  `parseBitSegments`/`parseOneSeg`/`attachSize` token parser is gone, along
  with the `0x"…"`-era reliance on raw-token capture for the spec.
- New words may not ship a bespoke string/token grammar. When a compact
  surface is wanted, add it to the single parser (a grammar rule or a
  `mini`/`+name/src/` minilang kind) or provide a macro that builds the
  Node data — both keep "one parser" and "structures are data" intact.
- Reading a discriminator field and switching on its value (e.g. a `type`
  enum String) is explicitly **not** secondary parsing; it is the normal way
  a handler interprets data.

---

## ADR-008 — 100% Go unit test coverage (of reachable code) at all times {#adr-008}

**Status:** Accepted · **Date:** 2026-07-07

### Decision

The Go implementation maintains **100% unit-test statement coverage of
every reachable statement, at all times**. The gate is:

```bash
make cover-gate     # fails below 100% of reachable statements
```

which runs every module's tests with `-coverpkg` spanning the whole repo,
merges the per-module profiles block-by-block (a statement counts as
covered when **any** suite reaches it — lang's tests legitimately exercise
the eng kernel, the spec corpus exercises both), and fails the build
below the floor (`test/go/covergate`).

To make every statement reachable, the codebase **admits mocking seams**,
modeled on the virtual FileOps capability:

- **Capabilities / narrow interfaces** for subsystems (file I/O,
  keyrings, clocks, SDK hosts).
- **Unexported function-variable seams** for single OS/stdlib edges whose
  failure or platform arm is otherwise unreachable — `osExit`,
  `readPassword`, `goosName`, `randRead`, marshal and constructor seams.
  The default is always the real implementation; tests swap and restore.
- **Extract-run mains**: `func main() { osExit(run(...)) }`, so even
  `main` bodies are covered.

The conventions, naming rules, and restore discipline are in
`design/TEST-SEAMS.10.md`.

**The one exclusion — a reviewed allowlist.** A block that no test can
reach falls into exactly one of two dispositions:

1. **Truly dead** (a tautological comparison, a shadowed branch, a
   `return` after a loop that always returns) — **removed** at source
   under ADR-005, never allowlisted.
2. **A defensive error-propagation or safety arm** whose guarded call
   cannot fail today but defends against a future change, an external
   library, or data corruption — **kept and allowlisted**. Deleting it to
   win a coverage number would silently drop a real error if that call
   ever started failing.

Exclusions are inline `//covergate:allow <reason>` comments on the guard's
opening line — the exclusion travels WITH the code, so a refactor that
shifts lines never invalidates it. `covergate` keeps them honest — the gate
**fails** if a marked guard becomes covered (graduate it: cover it and drop
the pragma) or the pragma goes stale (no block opens on its line). Adding
one is a reviewed act requiring a proof of unreachability. The policy and
the category breakdown are in `design/COVERAGE-ALLOWLIST.10.md`.

This applies to the Go implementation only, for now. The TypeScript
engine port (`eng/ts`) keeps its row-for-row spec-parity gate instead.

### Context

The 2026-07 quality program took repo-wide coverage from 78.5% to 100% of
reachable statements, and found shipped bugs in the *uncovered* slices of
the tree — a `deq`/`eq` large-integer divergence, a broken generator
tool, a TUI mutation that could never succeed, a trace renderer printing
sealed payload structs, a doc-example evaluator writing to the real
filesystem, and a `valueToAny` nil-dereference on a typed-map descriptor.
Every one lived in code no test reached. The remaining uncovered tail was
not "unimportant code" — it was precisely the OS edges, error arms, and
platform branches where bugs hide longest, unreachable only because the
code offered no seam to fake the OS, the terminal, or a constructor
failure.

Partial floors (90%, 95%) invite ratchet erosion: every new uncovered
statement is individually defensible and collectively regressive. A 100%
floor removes the judgment call — if a statement can't be tested, the
code must be reshaped until it can (a design pressure toward smaller,
injectable units, the same pressure that produced FileOps) — with the
single, audited escape valve of the allowlist for guards that are
genuinely unreachable yet genuinely defensive.

### Consequences

- `make cover-gate` joins the pre-commit checklist alongside
  `make fmt && make vet && make lint && make test`. A change that adds an
  uncovered, reachable statement fails the gate; the fix is a test, a
  seam, or a simpler shape.
- New code is written seam-first: OS calls, process exits, platform
  switches, and fallible constructors go behind the documented seam
  shapes from the start.
- Truly-dead branches are removed (ADR-005), not tolerated as "safety".
  Genuinely-unreachable *defensive* guards (error propagation, crypto
  defense-in-depth) may be allowlisted with a proof — but the list can
  only shrink or move deliberately, because the gate rejects a stale or
  now-covered entry.
- The gate measures the merged cross-suite profile, not per-package
  self-coverage, so integration-style suites (the TSV spec corpus, the
  compiled/interpreted differential) count toward the floor — testing
  effort goes where it verifies behaviour, not where a per-package ratio
  demands duplication.

---

## ADR-009 — Vault on-disk formats are versioned and forward-incompatible by refusal {#adr-009}

**Status:** Accepted · **Date:** 2026-06-10 · **Renumbered** 2026-07-18 (was a duplicate ADR-004)

### Decision

Every persistent vault artifact carries an explicit format version, and
the rules for reading one are fixed:

1. **Older than the running binary → migrate forward.** Each on-disk
   version has an ordered, pure migration to the next. The store
   (`vault.jsonic`) uses the `storeMigrations` registry keyed by
   `Store.Version`; the encrypted keyring (`vault.keyring`) uses a
   `"BORUK" | format-byte | …` header and dispatches on the byte.
2. **Newer than the running binary → refuse, never parse leniently.**
   Go's `encoding/json` silently drops unknown fields, so loading then
   saving a future-version store with an old binary would *erase* data
   it doesn't understand. A higher version than the binary supports is a
   hard error that says "upgrade boru", for the store, the keyring, and
   `vault policy` files alike.
3. **Bumping a format version is a three-part commit:** raise the
   constant (`storeVersion` / `keyringFormat` / `policyVersion`), add the
   migration (where applicable), and check in a golden fixture plus a
   load-and-migrate test. `TestMigrationRegistryLength` asserts the
   registry stays in lockstep with `storeVersion` so the three cannot
   drift.

The live wire protocols are versioned too: the credential broker
advertises `X-Boru-Vault-Protocol` on every response and refuses a client
that declares a newer one; the MCP server reports its protocol and
server version from `initialize`.

### Context

`boru` is under active development while the vault must keep working
across binary upgrades and, increasingly, across machines. Three failure
modes motivated formalizing this:

- **Silent field-stripping.** The store already wrote a `version` field
  that nothing read. The first additive schema change (hashing
  capability tokens) proved the hazard: an older binary doing any
  load-modify-save would drop the new field. A *read-time* version gate
  is the only thing that converts that silent loss into a clear error.
- **Unversionable ciphertext.** The keyring blob was a headerless
  `salt | nonce | ciphertext`, so the KDF, cipher, or layout could never
  change — old files were indistinguishable from new. A magic + format
  byte (authenticated as AEAD additional data, so it can't be downgraded
  by tampering) makes the format evolvable; legacy headerless files stay
  readable and are rewritten with a header on the next save.
- **Mysterious breakage as the "migration."** When token hashing landed,
  pre-existing capabilities just started returning 401. The v1→v2 store
  migration now revokes those legacy capabilities explicitly, so the
  transition is visible in `vault status` and the audit trail instead of
  being debugged from first principles.

### Consequences

- A newer-format vault on an older binary fails loudly and actionably
  rather than corrupting data — the property that lets the vault "keep
  working" across in-flight changes.
- The cost of a breaking on-disk change is now a fixed, testable recipe
  (constant + migration + golden file), which is cheap enough that there
  is no excuse to make an unversioned breaking change.
- Backups and cross-machine moves are well-defined: a `file`-backend
  vault is two self-describing files (`vault.jsonic` + `vault.keyring`)
  plus the passphrase, portable to any OS; keychain-backed vaults are
  not, because the secret values live in the host OS store rather than
  under `~/.boru`.
- For the keychain case — and for any cross-backend or cross-OS move —
  `vault export` writes a passphrase-encrypted bundle (its own `"BORUX"`
  magic + format byte, same envelope discipline) carrying the aliases
  and their values, which `vault import` restores into any backend.
  Because the bundle is versioned the same way, an older `boru` refuses a
  newer bundle rather than mishandling it. `import` auto-detects whether
  its input is a bundle or a `.env` file.

---

## ADR-010 — Types are values: type literals are first-class, singleton values everywhere {#adr-010}

**Status:** Proposed · **Date:** 2026-07-31 · Recorded on maintainer
instruction via `design/NUR-RESOLUTION-PLAN.0.md` (motivated by
NUR051/G13b; reinforced by NUR050/G12)

### Decision (proposed)

1. **A type literal is a value** — a first-class value that may appear
   anywhere a value may appear: bound with `def`, stored as a map/list
   member, returned from a function, passed as an argument, matched in
   `case`, etc. No layer may treat a type literal as declaration-only
   syntax valid solely in signatures and schemas.

2. **A type literal is a singleton value**: the single value inhabiting
   that type position. Therefore `Integer eq Integer` is **true**
   (structural type equality — `ExactEqual`'s type-body arm,
   `eng/go/compare.go:358-361`), while `0 eq Integer` is **false**
   because the operands are a scalar value and a type value — a genuine
   value-vs-value mismatch, NOT evidence that the type "isn't a value".
   Cross-type comparison is `teq`.

3. **Uniform obligation across all layers.** The interpreter, the type
   checker, and the bytecode compiler honour (1) and (2) identically.
   Any construct that runs interpreted with a type literal in value
   position MUST also compile; a compiler refusal on a
   type-literal-as-value is a **bug**, not an accepted limit.

4. **Provenance requirement (compiler).** The bytecode emitter assigns
   a bare type node a first-class, interned type-operand identity
   wherever it occurs — nested in map/list members included (the
   `OpPushType`/`internType` path, `eng/go/emit.go:1655`) — and never
   falls through to "body result of unknown provenance" for a type
   literal used as data.

### Context

boru already states the doctrine informally — "types are values"
(EXPLANATION.md, "Generics as memoised type construction": a schema is
a value holding a type body; `typeof` names an instantiation because it
IS a type). But because it was never encoded as a strict cross-layer
invariant, individual layers drifted: the bytecode compiler modelled
type literals as type-declaration machinery only and refuses
`{r: None}` in a fn body (NUR051/G13b), and the function-vs-reference
confusion (NUR050/G12) is the same class of error — a value kind one
layer honours and another flattens. An ADR makes "types are values" a
checkable contract so future features cannot silently regress it.

### Consequences (proposed)

- Fixes NUR051/G13b **by mandate**: the compiler must intern nested
  type literals. (Implemented 2026-07-31 — `RecordMakeMap` /
  `recordMakeListInner` intern bare type nodes; NUR051 resolved.)
- Adds a standing test obligation: for every value-position construct,
  a type literal must behave identically interpreted vs compiled.
- Clarifies the equality model in one place — `eq` is value equality;
  `Integer eq Integer` → true; `0 eq Integer` → false; `teq` for
  cross-type — preventing the "type literals aren't values"
  misreading.
- Relates to (does not supersede) the pending NUR050 decision: a
  single, principled `Function` value type is consistent with, and
  reinforced by, this record.
