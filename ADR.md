# Architecture Design Record (ADR)

A running list of the key architectural decisions behind AQL — the ones
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
  see `design/ERRORS.8.md` §"Chained forward calls".
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
## ADR-004 — Vault on-disk formats are versioned and forward-incompatible by refusal {#adr-004}

**Status:** Accepted · **Date:** 2026-06-10

### Decision

Every persistent vault artifact carries an explicit format version, and
the rules for reading one are fixed:

1. **Older than the running binary → migrate forward.** Each on-disk
   version has an ordered, pure migration to the next. The store
   (`vault.jsonic`) uses the `storeMigrations` registry keyed by
   `Store.Version`; the encrypted keyring (`vault.keyring`) uses a
   `"AQLK" | format-byte | …` header and dispatches on the byte.
2. **Newer than the running binary → refuse, never parse leniently.**
   Go's `encoding/json` silently drops unknown fields, so loading then
   saving a future-version store with an old binary would *erase* data
   it doesn't understand. A higher version than the binary supports is a
   hard error that says "upgrade aql", for the store, the keyring, and
   `vault policy` files alike.
3. **Bumping a format version is a three-part commit:** raise the
   constant (`storeVersion` / `keyringFormat` / `policyVersion`), add the
   migration (where applicable), and check in a golden fixture plus a
   load-and-migrate test. `TestMigrationRegistryLength` asserts the
   registry stays in lockstep with `storeVersion` so the three cannot
   drift.

The live wire protocols are versioned too: the credential broker
advertises `X-AQL-Vault-Protocol` on every response and refuses a client
that declares a newer one; the MCP server reports its protocol and
server version from `initialize`.

### Context

`aql` is under active development while the vault must keep working
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
  under `~/.aql`.
- For the keychain case — and for any cross-backend or cross-OS move —
  `vault export` writes a passphrase-encrypted bundle (its own `"AQLX"`
  magic + format byte, same envelope discipline) carrying the aliases
  and their values, which `vault import` restores into any backend.
  Because the bundle is versioned the same way, an older `aql` refuses a
  newer bundle rather than mishandling it. `import` auto-detects whether
  its input is a bundle or a `.env` file.

---

## ADR-005 — No deliberate panics; infrastructure code always returns errors {#adr-005}

**Status:** Accepted · **Date:** 2026-06-11

### Decision

The AQL implementation **never panics deliberately**. Every error
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
   *value* is sealed (`"AQLE" | format | ndkID | nonce | ct`, AES-256-GCM,
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
   on-disk versions follow ADR-004: `storeVersion` 3 → 4 (with a no-op
   `migrateStoreV3ToV4` and a golden fixture), and the envelope keyring is
   a self-describing `"AQLK" | format=2 | json` container distinct from
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
  envelope ciphertext) and now requires an AQL passphrase to decrypt.
