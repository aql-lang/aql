# Non-Uniformity Register (NUR)

A running register of every place where boru — the language or its
implementation — deviates from one of its own uniform rules. A
non-uniformity is any special case: a type treated differently from its
siblings, one member of a word family with an exception, a path that
bypasses a single-source-of-truth mechanism. Uniformity is a core design
value of boru (one parser, one argument-positioning convention, one
binding store, one total order, one truthiness rule); this register is
where every deviation from that value is made visible, argued, and
either eliminated or explicitly accepted.

Records are short, numbered (`NUR000`, `NUR001`, …), and dated, in the
style of [ADR.md](ADR.md). Numbers are **never reused**. A **Resolved**
record is **deleted** from this file — the fix and its rationale live
in the resolving commit, which names the `NURnnn` it closes — and its
number is retired, never reassigned. A gap in the sequence is itself
the record that something was found and fixed, and any external
reference to a deleted `NURnnn` stays unambiguous forever.

> **Recording a non-uniformity is mandatory; a Pending record does NOT
> block the PR.** When a non-uniformity surfaces — in code review, in a
> design note, or during coding and debugging — it is recorded here
> immediately with status **Pending**. That recording is not optional and
> not subject to maintainer instruction (unlike an ADR entry); what
> requires the maintainer is the **Allowed** verdict — the same reviewed
> discipline as a `//covergate:allow` entry
> (`design/COVERAGE-ALLOWLIST.10.md`). A record is discharged by becoming
> **Resolved** (the divergence is removed) or **Allowed** (an explicit,
> argued acceptance), and it may stay Pending across many merges: the
> register's job is that a divergence is never lost or silently
> baselined, not that work stops until it is settled.

**Statuses:**

- **Pending** — not yet discharged. Either not yet argued to a verdict
  at all, or argued to a **verdict of "resolve by fix"** whose fix has
  not landed: a record directed at a fix stays Pending until the
  divergence is actually gone, because only Resolved and Allowed
  discharge it. Does not hold up a merge. Every Pending record must also
  appear in the open list below.
- **Allowed** — a deliberate divergence, kept. The record states the
  uniform rule, the divergence, the rationale, and the evidence that
  pins it (docs and tests), so the acceptance cannot silently rot.
- **Resolved** — the divergence was removed. The record is **deleted**
  and its number retired (see above), so a record only ever appears in
  this file as Pending or Allowed; `git log -S NURnnn` recovers a
  retired number's history.

---

## Pending non-uniformities (the open list)

The live list of records whose status is **Pending** — the standing
inventory of known, argued-or-unargued divergences. An entry leaves this
list only by becoming **Resolved** or **Allowed** in its record below —
keep the two in sync in the same commit.

| # | Title | Surfaced by / provenance |
|---|-------|--------------------------|
| [NUR009](#nur009) | Bytes excluded from the DepScalar refinement bases — VERDICT 2026-08-15: WAIT for the ADR-012 `types/go` consolidation to close this through the refinement-base capability; no narrow fix meanwhile | 2026-07-22 uniformity review |
| [NUR026](#nur026) | Escape sets diverge between quoted strings and templates — NARROWED 2026-08-15: the escape VOCABULARY is resolved by fix (templates take the quoted-string set: \b \f \v \xNN \uNNNN, and an unknown escape drops its backslash); what remains is the malformed-input REPORTING difference, which needs an error channel the template lexer seam does not have | 2026-07-22 uniformity review |
| [NUR056](#nur056) | `make`-constructibility is the one capability with no opt-in — VERDICT 2026-08-14: resolve by fix (a `Maker` capability + the eighth `behave` slot) | 2026-08-02 NUR register review |
| [NUR072](#nur072) | Three sugar kinds (mini, type-bound, lambda) still canon in DEBUG form after NUR059 — withdrawn there because the renders do not round-trip: SugarInfo does not retain the mini delimiter, and type-bound renders its Items rather than the bound's text; also carries the undecided bare-word question (`word(foo)` vs `foo`, 175 corpus rows) | NUR059's fix, 2026-08-15 |
| [NUR073](#nur073) | `deq` is extensible per type (`DeepEqualer`), `eq` is not — the one part of the retired NUR031's verdict its fix did not take: the divergences closed by adding kernel arms rather than by routing through `Behavior`, so a type can define its own deep equality but not its own identity | NUR031's fix, 2026-08-16 |
| [NUR060](#nur060) | The parser twins disagree on open-input sources beyond the corpus | PR #337 parity-probe sweep (flagged for NUR by Codex P1) |
| [NUR063](#nur063) | Seven self-knowledge words are proposed to dispatch from two module surfaces (`boru:debug` and `boru:scry`) — VERDICT 2026-08-15: `boru:scry` canonical, the `boru:debug` copies frozen behind shared handlers and deprecated on a stated timeline | design/BORU-SCRY.0.md §6 (flagged for NUR by PR #344 Codex P1) |
| [NUR064](#nur064) | Pattern clauses route-and-bind in `receive` but route-only in `add` — VERDICT 2026-08-15: defer to the processes/services design line, to be decided when those modules are built | `design/STATE-MACHINES.0.md` §8 (flagged for NUR by the PR #345 review, Codex P1) |
| [NUR065](#nur065) | Two spellings of the classifier role get different static guarantees: `classes:` is alphabet-closed and diagnosed, `classify:` is neither — VERDICT 2026-08-15: defer to the state-machine design line (its open question #7) | `design/STATE-MACHINES.0.md` §3.6 (flagged for NUR by the PR #352 review, Codex P1) |

Pending records normally use a compact form (rule / divergence /
evidence / documentation status, plus a proposed verdict where one is
obvious). A record argued to a **resolve-by-fix** verdict keeps the
compact form and appends the verdict, since the fix — not prose — is
what will close it; a record **re-opened from Allowed** keeps the
argued form it already had, with the superseded allowance retained as
data. Expansion to the full argued form is what an **Allowed** verdict
requires.

---

## NUR000 — Boolean arithmetic is a defined error {#nur000}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

The six arithmetic words (`add`/`sub`/`mul`/`div`/`mod`/`pow`) are
**total within every scalar type and every Micron kind**
(REFERENCE.md §"Within-type operations"): numbers compute, `String` and
`Atom` carry the occurrence package, `Bytes` mirrors it over byte
subsequences, Microns fall to the field-wise default.

### The divergence

`Boolean` is the single scalar family excluded: `add true false` raises
`[boru/type_error]: add: arithmetic is not defined on Boolean` — for all
six ops.

### Why allowed

Boolean deliberately carries the logical words (`and`/`or`/`xor`/`not`)
instead of arithmetic; every candidate arithmetic semantics (C-style
integer promotion, GF(2)) is arbitrary, and an arbitrary choice would
be silently accepted where a loud error teaches the logical vocabulary.
The exception is implemented as a **registered** `[Boolean Boolean]`
signature that raises with a pinned message (the `setMicron` precedent)
rather than by signature absence, so the failure is specific instead of
an opaque dispatch error; a check-mode mirror (`booleanArithReturns`)
flags concrete Boolean arithmetic statically; and the signatures are
CoreDefault, so a user's `refine Boolean` overload can still extend an
arithmetic word by specificity (the refinement escape).

### Evidence

- `lang/go/native/native_scalar_ops.go` — `booleanArithHandler` /
  `booleanArithError` / `booleanArithReturns`; the six erroring
  `[Boolean Boolean]` signatures.
- REFERENCE.md §"Within-type operations" — "**`Boolean`** arithmetic is
  a **defined error**".
- `lang/spec/scalar-micron-ops.tsv` (all six ops pinned as errors);
  `lang/spec/open-words.tsv` (the refine-extension escape and its
  negative twins).

---

## NUR001 — `convert Boolean` coerces by presence, not content {#nur001}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

`convert <ScalarType> <String>` parses the string's **content**:
`convert Integer "42"` → `42`, `convert Float "1.5"` → `1.5`.

### The divergence

`convert Boolean "false"` → `true`. Boolean conversion applies the
truthiness rule — `false`, numeric zero in any leaf
(`0`/`0.0`/`0d0`) and `""` are false; a String's characters are never
inspected.

One neighbouring defect is recorded separately and does not disturb
this allowance: the language's falsy set also contains `none`, `[]`
and `{}`, but `convert`'s source slot is Scalar-only and refuses all
three with a `signature_error` (NUR053).

### Why allowed

`convert Boolean` shares **one** coercion rule with `if`-condition
truthiness and `make Boolean` (presence, not content); making the
conversion path parse content would fork truthiness into two rules —
a worse non-uniformity than the one it fixes. The three consumers apply
the same rule but do not accept the same *domain*; that separate
divergence is NUR053 and does not disturb this allowance, which is
about content-vs-presence. Content parsing exists as
an explicit opt-in: `convert Boolean {truthy: true}` parses the YAML
tokens (`y`/`yes`/`true`/`on` and `n`/`no`/`false`/`off`,
case-insensitive) and
falls back to presence for anything else; the option is inert for
non-Boolean targets.

### Evidence

- `lang/go/native/native_type.go` — `coerceBooleanTruthy` and the
  `truthy` option plumbing on `convert`.
- REFERENCE.md — "**`convert Boolean` is presence coercion; `{truthy:
  true}` opts into YAML parsing**" and §`if` ("coerces its condition …
  the exact same rule as `convert Boolean` and `make Boolean`").
- `lang/go/native/native_type_convert_seam9_test.go` (both modes,
  positive and negative); `lang/go/native/integration_coverage_test.go`
  (`'false' convert Boolean` → `true` pinned explicitly).

**Review (2026-07-31):** re-affirmed by the maintainer
(`design/NUR-RESOLUTION-PLAN.0.md`). The single coercion rule this
record leans on is now specified once — with every consuming construct
enumerated — in `design/TRUTHINESS.0.md` (the One Truthiness Model);
an ADR stating the model as a language principle is a recorded
candidate there.

---

## NUR002 — Value enumeration exhausts finite domains; Boolean is the built-in instance {#nur002}

**Status:** Allowed · **Date:** 2026-07-22 · **Rewritten:** 2026-07-31
(maintainer — the pre-rewrite record framed this as a Boolean special
case; the rewrite states the general rule instead)

### The uniform rule (as rewritten)

**Exhaustive coverage of any finite domain does not require a default
branch.** For a scalar scrutinee, a default-less `case` proves
exhaustiveness through type clauses, `[is T]` predicates,
comparison-predicate / refinement interval unions, or — when the
scrutinee's domain is **finite** — by enumerating its values. An
infinite scalar can never be covered by literal enumeration
(`case n [1 … 2 …]` can not cover `Integer`).

### Where Boolean sits

Boolean is not a special case: it is the built-in two-value
pseudo-enum. `true` + `false` cover a `Boolean` scrutinee exactly as
enum members cover an enum: `case b [true 1 false 0]` is statically
exhaustive with no default, and `case b [true 1]` is a
`case_not_exhaustive` check error (`uncovered: false`).

### Why this is the rule, not a divergence

Coverage-by-enumeration follows from cardinality, not special
pleading: a domain is enumerable iff it is finite, so the checker's
coverage proof stays in the sound direction throughout. The mechanism
is the one value/type coverage channel enums use (`def Color (red/q
tor …)` covered member by member) — and enums are themselves
specialisations of disjunct types, so the general principle is
**finite disjunct exhaustiveness**, of which Boolean is the built-in
instance. Documentation should present it that way rather than
presenting Boolean as special.

### Follow-on design work (recorded 2026-07-31, not yet scheduled)

Finite **dependent scalar types** also define finite domains, and
should eventually enter the same coverage channel. Two items to
investigate (`design/NUR-RESOLUTION-PLAN.0.md`):

- **Ergonomics** — allow a finite dependent type to be declared by
  enumerating its values (a `{2,3,4}`-style literal domain) rather
  than forcing range predicates (`Integer >=2 and <=4`).
- **Implementation** — when a finite dependent set is statically
  known, avoid materialising large sets; prefer symbolic/range
  representations. Where free variables remain, a symbolic
  representation is necessary regardless.

### Evidence

- REFERENCE.md §"`case` — dispatch and exhaustiveness" — "**Boolean, by
  `true` and `false`** (or the `Boolean` literal)", including the
  negative example.
- `lang/spec/case.tsv` §6 ("true+false cover Boolean", alongside the
  union and enum coverage rows that show the shared mechanism).

---

## NUR003 — `and`/`or` select an operand; the rest of the boolean family returns strict Boolean {#nur003}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

The boolean word family returns strict `Boolean`: `not`, `xor`, `any`,
`all`, and the `boru:logic-util` gates (`nand`/`nor`/`xnor`/`iff`/
`implies`) all coerce their inputs by truthiness and yield `true` or
`false`.

### The divergence

`and` and `or` are value-selecting short-circuit connectives: they
return whichever **operand** decided the result, of whatever type —
`1 and 2` → `2`, `false 5 and` → `false`, `0 9 or` → `9`.

### Why allowed

Deliberate Lisp/Python semantics: the operand form composes directly
(`x or default`, with `otherwise` as the None-aware variant), and a
strict Boolean is one `not not` (or a comparison) away. The divergence
is loudly documented at the word table itself, and check mode types the
result precisely — `foldOrJoin` concrete-folds statically-decided
selections and otherwise narrows to the join of the operand types, so
the non-uniform return type never degrades static analysis to `Any`.

### Evidence

- `lang/go/native/native_boolean.go` — `andHandler`/`orHandler` (operand
  return) vs `notHandler`/`boolBinaryNative`/`anyHandler`/`allHandler`
  (strict Boolean); `foldOrJoin` for the check-mode typing.
- REFERENCE.md §Boolean — "**`and` / `or` return an operand, not a
  coerced boolean.**"

**Review (2026-07-31):** re-affirmed by the maintainer
(`design/NUR-RESOLUTION-PLAN.0.md`). The operand-return semantics —
short-circuit behaviour, evaluation order, which operand is returned,
and the interaction with static typing — are specified in
`design/TRUTHINESS.0.md` §"The connectives", which this record now
leans on.

---

## NUR004 — Boolean, Atom and Bytes have no lattice subtypes {#nur004}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

The scalar branch families carry structural leaves: `String` has
`EmptyString`/`ProperString`, `Number` has `Integer`/`Float`/
`BigInteger`/`BigDecimal`, `Micron` has its twelve kinds.

### The divergence

`Scalar/Boolean` and `Scalar/Atom` are leaf-less — direct children of
`Scalar` with no builtin subtypes (no `True`/`False` lattice nodes).
`Scalar/Bytes` is a third leaf-less child, registered from the language
layer (`native_bytes.go`) rather than declared in `builtinDecls`. The
same reasoning covers it with one caveat: of the two value-level
substitutes named below, `case` literal coverage does not reach Bytes
(the domain is infinite) and DepScalar refinement construction is not
available for it either (it is not a supported refinement base —
NUR009). The nominal-split route, `refine Bytes`, does work, and is
what stands in for subtypes here.

### Why allowed

Vacuous rather than divergent: no kernel mechanism requires a scalar
family to have leaves, and nothing dispatches on their presence. There
is no useful structural split of Boolean — `True`/`False` subtypes would
duplicate what value-level machinery already provides uniformly (`case`
literal coverage per NUR002, and DepScalar refinements: `(Boolean gte
true)` *is* the true-only subset, since Boolean is one of the supported
refinement bases — `canonicalBaseType` admits Integer, Float, Number,
String, Boolean and Atom). Users who want a
nominal split can mint it (`refine Boolean`), which participates in
dispatch by specificity like any refinement.

**Clarified (2026-07-31, maintainer):** the two layers this record
separates are the **lattice subtype hierarchy** (structural leaves the
kernel dispatches on — `EmptyString`/`ProperString`, the Number
leaves) and **value-level finite sets** (the inhabitants of a finite
domain — what `case` coverage and DepScalar refinements operate on).
`true`/`false` belong at the value layer: they are the two members of
a finite domain (NUR002, as rewritten), not structural variants of the
type, so minting `True`/`False` lattice leaves would put value
distinctions into the structural layer — the wrong home for them.

### Evidence

- `eng/go/typetable.go::builtinDecls` — the Scalar branch layout.
- `eng/go/depscalar.go::canonicalBaseType` — Boolean listed among the
  supported DepScalar bases (`Boolean gte true` constructs).
- `lang/spec/case.tsv:75` (true+false cover Boolean) and
  `lang/spec/edge-types-1.tsv:82-85` / `lang/spec/open-words.tsv:26-29`
  (`refine Boolean` mints a nominal split that dispatches) — the
  value-level machinery that stands in for subtypes.
- `lang/go/native/native_bytes.go:23` — the `Scalar/Bytes`
  registration, the third leaf-less child.

---

## NUR005 — String `add` is the sole cross-type exception to same-type arithmetic {#nur005}

**Status:** Allowed · **Date:** 2026-07-31 (recorded Pending
2026-07-22; verdict and rewritten wording: maintainer, via
`design/NUR-RESOLUTION-PLAN.0.md`)

### The uniform rule

Scalar arithmetic is **same-type arithmetic**: the six words are
"applied within a type, never across it" — REFERENCE.md §"Within-type
operations". A cross-type pair has no signature and raises
`[boru/signature_error]`. Where a signature is *deliberately registered
to refuse*, the failure is instead a coded error with a specific
message — `[boru/type_error]` for `Big`⊕`Float`, for Boolean
arithmetic (NUR000), for a cross-KIND Micron pair, and for several
within-kind Micron restrictions (`mul` on two Qions); `[boru/arith_error]`
for a Qion currency mismatch.

### The divergence

`add` carries `[String Scalar]` / `[Scalar String]` overloads that
stringify the non-String operand (`add "x" 5` → `'5x'`), while Atom
`add` is `[Atom Atom]`-only and Bytes `add` is `[Bytes Bytes]`-only.

### Why allowed

**String `add` is the sole language-level exception to same-type
arithmetic, and it is deliberate.** Concatenation-with-coercion is the
overwhelmingly common string operation, the coercion is total and
canonical (every Scalar has one string render), and the overloads
require **at least one** String operand, so they never manufacture a
concatenation of two NON-String operands: `add true 1` raises
`[boru/signature_error]` — no concat overload matches without a String,
and no within-type arm matches a Boolean/Integer pair either, so the
refusal is a dispatch miss rather than the registered `type_error`
NUR000 installs for Boolean arithmetic. ("String-or-bust" governs the
concat overloads only; two non-String scalars of the SAME type still
have their within-type arm — `add 1 2` → 3, `add a/q b/q` → 'ba'.)

The Pending record's framing — that Atom and Bytes "do not mirror it" —
treated the trio as an
architectural grouping obliged to move together; the verdict is that
the String/Atom/Bytes occurrence-package parallel is a **documentary
comparison, not an architectural grouping**. Nothing requires Atom or
Bytes to adopt a cross-type overload because their within-type
packages mirror String's, and neither has String's coercion case: an
Atom is a name and Bytes are raw octets, so a silent stringify would
manufacture bugs, not ergonomics.

### Evidence

- REFERENCE.md §"Within-type operations" — now states the exception
  **at the rule**: "The **sole language-level exception** is `String`
  `add` … no other word, and no other type — `Atom` and `Bytes`
  included — crosses scalar types" (doc fix landed with this verdict,
  closing the 60-lines-apart contradiction the Pending record flagged).
- `lang/go/native/native_math.go` — the `[TString TScalar]` /
  `[TScalar TString]` overloads and the "string-or-bust" comment;
  `native_scalar_ops.go` / `native_bytes.go` — the within-type
  `[Atom Atom]` / `[Bytes Bytes]` signatures.
- `lang/spec/arithmetic.tsv` §3 — the concat battery, including the
  `add true 1` and `add true false` negatives.

---

## NUR009 — Bytes excluded from the DepScalar refinement bases {#nur009}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** the comparison words double as refinement constructors over
the well-known ordered scalar bases (`Integer gte 0`, `String lt "z"`,
`Boolean gte true` — one shared resolver, `canonicalBaseType`).
**Divergence:** `canonicalBaseType` admits Integer/Float/Number/String/
Boolean/Atom and omits Bytes — the one ordered scalar leaf (full
byte-lexicographic Comparer, Sizer, complete occurrence package) denied
refinement construction.
**Evidence:** `eng/go/depscalar.go:157-175`;
`lang/go/native/native_bytes.go:13,138-163`.
**Documentation status:** not found in REFERENCE/ADR/design — either an
unstated deliberate scoping or an omission; needs a verdict.

**Verdict direction (maintainer, 2026-07-31 — architectural
remediation, `design/NUR-RESOLUTION-PLAN.0.md`):** the Bytes omission
exposes a deeper **ownership** problem in the kernel type hierarchy,
and the fix is architectural rather than a one-line addition to
`canonicalBaseType`. The proposed rule: **all globally visible
descendants of `Node` or `Scalar` belong in `eng`**; core modules
(`boru:*`) may define module-owned descendants; the `lang` layer must
not define additional *global* Node/Scalar descendants except through
an explicit NUR. Likely migrations into eng: `Bytes`, `Time`, `Date`,
`DateTime`, `Instant`; the remaining scalar descendants are to be
reviewed individually. A new ADR describing ownership of the kernel
type hierarchy is required before the migration lands (recorded as ADR
candidate 2 in the resolution plan). This record stays Pending until
that remediation (or a narrower argued verdict) closes it.
**ADR-012 (2026-08-03) retargets the remediation's destination:** the
ownership rule is recorded, but the migrations consolidate in the new
`types/go` component with capability opt-ins (the refinement-base
capability closes this record), not in eng — the kernel stays
content-free.

**Verdict (maintainer, 2026-08-15 — WAIT, no narrow fix):** do **not**
add `Bytes` to `canonicalBaseType` as a one-line patch. This record
closes through the **ADR-012 `types/go` consolidation** and its
refinement-base capability, as the retargeted direction describes.

The reasoning is that a special case added now is a special case the
consolidation would have to unwind: the point of the capability is that
a type DECLARES its refinement-base participation, and hand-listing one
more leaf in the resolver is the mechanism the remediation replaces.
The gap is real and stays visible in the meantime — that is what a
Pending record is for. Stays **Pending** until the consolidation lands.

---

## NUR011 — `eq` is identity for compounds, value for scalars {#nur011}

**Status:** Allowed · **Date:** 2026-07-23

### The uniform rule

One word, one equality principle.

### The divergence

`eq` compares scalars by value but lists/maps/XML/instances by
container identity (`["a"] eq ["a"]` → false); `deq` is deep value
equality throughout. Consequence: `eq` disagrees with `cmp`-equality
on compounds (structurally-equal lists are `cmp`-equal but not `eq`).

### Why allowed

The maintainer's rule (2026-07-23, resolving NUR015 in the same
stroke): **for Scalars, `eq` and `deq` are the same and based on
values; for Nodes and Ideals, `eq` is by reference, `deq` is by
value.**

The Ideal half carries argued carve-outs, settled by the equality work
of the retired NUR031 (`git log -S NUR031` for its reasoning) and
recorded here now that it owns them. Two kinds have no second level to
offer, so their `eq` and `deq` coincide from opposite directions:

- **By VALUE, both** — a *value-like* Ideal with no handle behind it.
  `Error` (two independently raised errors with equal code, message and
  payload are `eq`) and `Word` (a name plus its `/`-modifiers), joined
  by the declared **type values** — a `class` and refinements of one, a
  disjunction/`enum`, a `fnsig`/`surface`, an uninstantiated `gen`
  schema — which are immutable declarations compared nominally.
- **By REFERENCE, both** — an opaque handle whose identity IS its value:
  `Timeout`/`Interval`, the `Module` descriptor (2026-08-02), and a
  sealed host `ExtensionPayload` (an `IO.open` file handle, a lock, a
  watcher, an mmap), which the kernel compares as a box and never reads
  into.

`Store` and `Function` are the two that DO have both levels, and they
take the rule as written: `eq` is reference identity — a Store's
`*StoreInstanceInfo`, a function's identity token — and `deq` is deep
value: a Store's own entry projection, a function's content as canon.

All of these are the rule applied, not departures from it — but the rule
as quoted above does not say so, and this record is where a reader looks
first.

Two equality levels are deliberate — reference identity
answers "is this the same container?" (cheap, aliasing-aware), deep
equality answers "do these hold the same values?" — the Scheme
`eq?`/`equal?` trichotomy collapsed to two levels because scalar
value-identity makes the levels coincide there. Every value-oriented
word keys on `deq` (the collection words since the NUR015 fix); `eq`
remains the aliasing probe.

### Evidence

- `eng/go/compare.go` — `ExactEqual` (scalar arm shared with
  `DeepEqual` via `scalarFamilyEqual`, so eq and deq can never drift
  on a scalar; `sameContainer` identity arms for compounds).
- REFERENCE.md §Comparison ("**`eq` is identity for compounds; `deq`
  is structural — by design**"); EXPLANATION.md §"Type ordering", the "**Two equalities, one rule.**"
  lead-in (added with this verdict); `design/LISP-ANALYSIS.5.md` (the original
  argument).
- `core/go/compare.go` — the carve-out arms themselves
  (`opaqueIdealExactEqual` / `opaqueIdealDeepEqual`, `storeDeepEqual`,
  `errorInfoEqual`, `hostPayloadIdentity`, `sameFnIdentity` /
  `fnStructurallyEqual`), and `core/go/compare_nur031_test.go`, which
  pins each of them.
- `lang/spec/compare-restrict.tsv` — the per-kind rows, including the
  code and type values; `lang/spec/module-array.tsv` — the collection
  words' `deq`-basis battery pins the value side of the rule.

**Modification recorded (maintainer, 2026-07-31,
`design/NUR-RESOLUTION-PLAN.0.md`):** the two-level model is to grow
into a complete equality family with a third word — **`req`**,
reference equality (pointer identity only, uniformly for compounds
and scalars) — separating three notions many languages conflate:
convenience equality (`eq`), deep structural equality (`deq`), and
reference identity (`req`). Performance note: Bytes `deq` may be
O(n); `req` gives a constant-time identity probe. Documentation
should compare the model with JavaScript, Python, Ruby, and the
Lisp family. The `req` design travelled with the equality work of the
retired NUR031 and is now unowned: it is a third WORD, not a
non-uniformity, so no record tracks it. This record's allowance is
unchanged.

---

## NUR013 — Two ordering regimes: a lawful total order and IEEE relationals {#nur013}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22; the 2026-07-31 investigation verdict discharged below;
verdict: maintainer, accepting the recommendation in
`design/NUR-EFFORT-TRIAGE.0.md`)

### The uniform rule

One ordering answer per value pair within one word family.

### The divergence

`cmp`/`tcmp`/`sort` give NaN a defined slot (sorts greatest; two NaNs
tie) while `lt`/`lte`/`gt`/`gte` apply the IEEE unordered rule (always
false); `nan eq nan` is false while `nan cmp nan` is 0. Signed zeros
now add a mirror-image case in the other direction: `-0.0 cmp 0.0` is
-1 while `-0.0 eq 0.0` is true and `-0.0 lt 0.0` is false.

### The `totalOrder` comparison (the 2026-07-31 verdict, discharged)

IEEE-754 §5.10 `totalOrder` requires
`−qNaN < −inf < negative finite < −0 < +0 < positive finite < +inf <
+qNaN`, with NaNs further ordered by sign and payload. boru's order
was compared against it point by point:

- **NaN slotting — conforming, for boru's observable NaN.** boru
  exposes exactly one quiet NaN: there is a single `nan` literal, sign
  is not observable (`nan -1.0 mul` renders `nan`), and no payload is
  reachable. For a single positive qNaN, `totalOrder` demands exactly
  what boru does — greatest, above `+inf`, tying with itself.
- **NaN sign/payload ordering — impractical, and accepted.** Ordering
  negative NaNs below `−inf` and ordering by payload would require
  making NaN sign and payload observable values in the language, which
  nothing else in boru does and no boru program can produce. The
  divergence is therefore vacuous at the language level; per the
  verdict's own terms it is folded into this record's acceptance.
- **Signed zeros — was nonconforming, now FIXED.** `-0.0 tcmp 0.0`
  answered 0; `totalOrder` requires −0 before +0. The total order now
  slots negative zero first (`sort [0.0 -0.0]` → `[-0.0 0.0]`), with
  Integer `0` and BigDecimal `0d0` slotting as +0 so the cross-leaf
  triangle stays transitive.

### Why allowed

The two regimes are deliberate and are the standard resolution of an
unsatisfiable constraint set — the same architecture NUR024 records as
**semantic** vs **deterministic** ordering:

- **The relationals** (`lt`/`lte`/`gt`/`gte`) answer a *mathematical*
  question and therefore obey IEEE-754: NaN comparisons are false
  (§5.11), and ±0 compare equal. A language that silently ordered NaN
  in `lt` would be wrong by the numeric standard its floats implement.
- **The total order** (`cmp`/`tcmp`/`sort`) answers "give me a lawful,
  deterministic arrangement of these values". It must be total and
  antisymmetric or `sort` is not a function; that requires a slot for
  NaN and a decision on ±0, which is precisely what `totalOrder`
  specifies and what boru now implements.

Because the relationals must keep IEEE ±0 equality while the total
order separates the zeros, the relational path carries an explicit
signed-zero carve-out beside the NaN one. That carve-out is part of
this acceptance, not a new divergence: it is the same
semantic-vs-deterministic split applied to the other special value.

### Evidence

- `eng/go/compare_scalar_behaviors.go` — the NaN slot and the
  Signbit tiebreak (float projection and big-rat paths, keeping
  Integer/BigDecimal zeros at +0).
- `eng/go/compare.go` — the relational unordered/signed-zero guards
  that keep `lt`/`lte`/`gt`/`gte` IEEE-conforming.
- `eng/go/compare_nan_test.go`, `eng/go/compare_zero_test.go` — both
  regimes, positive and negative.
- `lang/spec/float-special.tsv` (signed-zero and NaN sections),
  `lang/spec/edge-scalars-2.tsv` (the cmp/sort rows).
- `design/IEEE-754-COMPLIANCE.8.md` §5.10 — the conformance record
  above; `design/TYPE-ORDERING.10.md` §"NaN in the total order".

---

## NUR014 — Cross-leaf numeric magnitude equality depends on the leaf pair AND the value {#nur014}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22; verdict: maintainer, accepting the recommendation in
`design/NUR-EFFORT-TRIAGE.0.md`)

### The uniform rule

Leaves of the same family compare by magnitude: `1 cmp 1.0` → 0,
`1 eq 1.0` → true.

### The divergence

Whether the collapse holds is decided by the leaf pair **and** by the
value, so it is not a family invariant either way.

- **Value-dependent within one pair.** Float↔BigDecimal collapses for
  every binary-exact (dyadic) magnitude — `0d0.5 eq 0.5` → true — and
  fails for every other — `0.1 eq 0d0.1` → false (an exact big.Rat
  compare of the float's true binary value against the exact decimal).
- **Pair-dependent at one value.** The same magnitudes answer
  differently purely because of the leaves: `9007199254740993 eq
  9007199254740992.0` → true (Integer↔Float compares through a float64
  projection) while `0d9007199254740993 eq 9007199254740992.0` → false
  (BigInteger↔Float compares exactly).

### Why allowed

The divergence is **mathematically honest**: the Float written `0.1`
IS NOT one-tenth — it is the nearest binary64 value,
0.1000000000000000055511151231257827…, and the exact big.Rat compare
reports that truthfully. Every collapse that *can* hold exactly does
hold (`1 eq 1.0`, `1 eq 0d1`, `0d0.5 eq 0.5` — dyadic values convert
exactly), so the family invariant fails only where the mathematics
itself fails. The alternative — rounding BigDecimal through float64 to
force the collapse — would silently equate distinct values, defeating
the reason BigDecimal exists; it would also contradict the
exactness-preserving design that already makes mixed Big⊕Float
arithmetic a defined error. The behaviour is Python's
(`Decimal('0.1') == 0.1` → False), for the same reason.

### Evidence

- `eng/go/compare_scalar_behaviors.go` — `numberCompareBehavior.
  Compare` and `toRatExact` (the in-code rationale comments cite the
  Python precedent).
- REFERENCE.md:195-200 — the user-facing statement of the honest
  result, with the exact-value explanation.
- `lang/spec/bignum.tsv:47-63` — pins both directions: the collapses
  that hold (`0d5 eq 5`, `1 cmp 0d1.0` → 0, `0d0.5 eq 0.5`) and the
  one that must not (`0.1 eq 0d0.1` → false).
- `lang/spec/edge-scalars-1.tsv:24-25` — both `cmp` directions of the
  non-collapse.

---

## NUR018 — Store and Error are excluded from `make` {#nur018}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22; verdict: maintainer, accepting the recommendation in
`design/NUR-EFFORT-TRIAGE.0.md`)

### The uniform rule

`make` instantiates the structural type-kinds; the kernel guide groups
Record, Options, Table, Class, Store, Error and the Micron family
together as the `make`/`record`/`class` structural set
(eng/go/CLAUDE.md §"Where a Type Lives" rule 4).

### The divergence

`make Store {}` and `make Error {message:"x"}` raise
`[boru/unsupported]: make: unsupported target type` while
Record/Options/Table/Class/Micron are `make` targets — Store and Error
construct only through their dedicated words.

### Why allowed

`make` targets are the **schema-bearing** structural kinds: a
Record/Options/Table/Class/Micron declares a shape, and `make`
instantiates a value against that shape. Store and Error carry no
user-declared schema and their constructors are semantically loaded in
ways a bare `make` cannot honour: a Store IS its position in the
context machinery (`StoreInstanceInfo` carries the parent-chain and
COW-layer state that `eng/go/registry.go`'s context words establish —
a detached `make Store {}` would have to invent an answer to "whose
child is it?"), and an Error's identity is its passage through
`raise`/`trap` (`describe raise`: "construct an Ideal/Error"), so
error construction always flows through the raising path that stamps
code and context. The kernel-guide grouping this record measured
against is about **kernel residence** (where the types live), not
about `make`-constructibility — clarified at the rule itself with this
verdict. The exclusion is loud (a coded `unsupported` error, not a
dispatch miss), and the dedicated constructors are the documented
route.

### Evidence

- `eng/go/core_make.go` — `registerKernelIdeals` (:788) is where the
  omission lives: it registers Ideals for Object, Resource, Record,
  Micron and Table and registers none for Store or Error, so
  `reg.Ideals.For`/`Match` return nil for those two and the target
  falls through to `MakeConvert` (:1088), whose default arm (:1128)
  raises the covered `unsupported target type`. (`isTypeLike` (:31) is
  NOT the gate — it short-circuits on `IsBareTypeNode` and answers true
  for Store and Error exactly as it does for Record.)
- `eng/spec/make.tsv` — negative rows pinning both exclusions
  (`make Store {}` and `make Error {message:'x'}` → ERROR).
- eng/go/CLAUDE.md §"Where a Type Lives" rule 4 — the
  kernel-residence clarification landed with this verdict.
- REFERENCE.md — the `make` documentation states the exclusion and
  names the dedicated constructors.

---

## NUR019 — `slice` is a core sequence word, not a String straggler {#nur019}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22 as "the String family's core straggler"; verdict:
maintainer, accepting the recommendation in
`design/NUR-EFFORT-TRIAGE.0.md`)

### The uniform rule

The string vocabulary moved to `boru:string-util`; moved words are not
available unqualified (lang/go/CLAUDE.md §"Package layout").

### The divergence (as recorded)

`slice` alone stayed core — REFERENCE's string table listed it
unqualified between two `StringUtil.*` rows, and `boru describe` files
it under `list`, not `string`, with the reason stated nowhere.

### Why allowed

The move rule does not apply because **`slice` is not a String-family
word**: it is a core *sequence* word, polymorphic over String, List,
and Bytes (nine unqualified signatures spanning all three), kin of
`size`/`take`/`reverse`, which also stayed core for the same reason.
Relocating it to `StringUtil` would force splitting one polymorphic
word — the List and Bytes overloads cannot live in a string namespace
— which is a semantically worse outcome than the filing confusion this
record flagged. What WAS wrong was the filing: REFERENCE's string
table presented `slice` as if it were an unqualified string word, and
the describe categories did not say where to find it. Both filings are
fixed with this verdict; the `list` category placement stands, because
that is the sequence home.

### Evidence

- `lang/go/native/natives.go:372-385` and
  `lang/go/native/native_bytes.go` — the String+List signature pairs
  plus the Bytes overloads: one polymorphic word.
- `boru describe slice` — all nine signatures, unqualified.
- REFERENCE.md:1160 — the string-table row now carries the "core
  *sequence* word, no import — also slices List and Bytes; filed under
  the `list` describe category, see NUR019" parenthetical (fixed with
  this verdict).
- `lang/go/native/help/help_categories.go` — the string category's
  description now points at core `slice` (fixed with this verdict).
- `lang/spec/edge-scalars-3.tsv:45-53`, `corpus-core.tsv:119`,
  `corpus-structures.tsv:14` — both string and list behaviour pinned;
  the two-argument negative-start form is pinned at
  `edge-scalars-3.tsv:47,52`. NUR039's actual divergence — a negative
  start in the THREE-argument form discarding `end` — is pinned by no
  spec row.

---

## NUR020 — `print` stays in core; every other IO word is namespaced {#nur020}

**Status:** Allowed · **Date:** 2026-07-31 (recorded Pending
2026-07-22; verdict: maintainer, via `design/NUR-RESOLUTION-PLAN.0.md`)

### The uniform rule

The IO vocabulary lives in `boru:io` (`IO.printstr`, `IO.read`,
`IO.write`, …); moved words are not available unqualified.

### The divergence

`print` alone stays in core, unqualified — one IO word outside the
namespace the rest of its family lives in.

### Why allowed

The argument, now written down rather than asserted: **`print` in core
is what makes the expected "Hello World" learning experience work.**
`print "Hello, World"` must be a complete first program — no `import`,
no namespace, no explanation of the module system before the first
line of output — and that matches the expectation practically every
mainstream language sets (`print`/`println`/`puts`/`console.log`
reachable from the first line). The pedagogical entry point outweighs
family symmetry for exactly one word; everything programmatic
(`printstr`, streams, `read`/`write`, `trace`) correctly demands the
`boru:io` import, so the capability surface of real programs is
unchanged. The boundary is one word wide and this record is its
argument; a second unqualified IO word would need its own NUR.

### Evidence

- `lang/go/native/native_print.go` and `register.go` — `print` is the
  single core IO registration; `io_module.go` — everything else.
- `lang/go/CLAUDE.md` §"Package layout" — "only `print` stays in core";
  ADR-004 §Consequences argues print's *forwardness* (a distinct
  question, deliberately not revisited here).
- Bare `print` works in a one-line program with no import
  (`boru -e 'print "Hello, World"'`) — the experience this record
  protects; HOWTO.md's recipes use it unqualified throughout.

---

## NUR022 — `del` covers a fraction of `set`'s containers {#nur022}

**Status:** Allowed · **Date:** 2026-08-14 (the container gap was
RESOLVED BY FIX 2026-08-02; the surviving slot asymmetry is allowed —
see the verdict at the end) · **Recorded:** 2026-07-22 ·
**Surfaced by:** full-repo uniformity review

**Rule (as restated by the 2026-08-14 verdict):** the storage-column
words cover the same **keys** — a key that `set` can write, `del` can
remove. **Slots are out of scope**: a declared Class field and a List
index are positions, not keys, and the inverse of writing a position is
writing a different value, not removing the position.
**Divergence (as recorded, now FIXED — see below):** `set` dispatched
over Class, Store, FlexXml, WeakFlexXml, FlexMap, WeakFlexMap, Map,
List, FlexList, WeakFlexList (and carried a registered `type_error`
refusal for the immutable Microns); `del` covered Map and FlexMap
only. The List exclusion was documented (pointing at
pop/shift/remove-at); the Store, Class, FlexList/WeakFlexList and
FlexXml/WeakFlexXml absences were not. `boru describe set` listed 19
signatures, `boru describe del` four.
**Documentation status:** documented — `lang/spec/flex.tsv` §12 now
states the per-container contract, and every refusal carries its own
message.

**Note on the rule (2026-08-02 review):** the rule above was
originally phrased "paired reader/writer words cover the same
containers", which mis-describes the pair: `set` and `del` are both
WRITERS. The reader, `get`,
covers a third and wider set again (Module, Class, Store, Error,
Resource, Xml, Node, Micron, None) — so container coverage is not
uniform across the storage column at all. That wider spread is
context for the verdict below, not a separate record: bringing `del`
into line with `set` is the step that was directed.

**Verdict (maintainer, 2026-07-31 — resolve by fix,
`design/NUR-RESOLUTION-PLAN.0.md`):** bring `del` into symmetry with
`set` across the container set. **First investigation step:** confirm
that boru distinguishes an *absent key* from a *present key bound to
`none`* — the deletion semantics hang on that distinction being real
and observable. Separately, a **sentinel-values design programme** is
opened (globally unique singletons, user- and system-defined
sentinels, their interaction with containers, equality, and
option-like APIs) — it needs its own design document because it
potentially touches many language facilities, but **NUR022 must not
wait on it**: the del/set symmetry fix proceeds independently. Stays
Pending until the fix lands.

### Investigation step (2026-08-02): the distinction is real, with one hole

An absent key and a key bound to `none` are distinguishable, so
deletion is not expressible as `set key none` and the word earns its
place:

| probe | `{a:1}` | `{a:1 b:none}` |
| --- | --- | --- |
| `has b/q` | `false` | `true` |
| `size` | `1` | `2` |
| `keys` | `["a"]` | `["a", "b"]` |

and the two containers are not `eq`: `{a:1} eq {a:1 b:none}` → `false`.

`del` and `set … none` therefore produce different containers:
`({a:1 b:2} del b) eq ({a:1 b:2} set b none)` → `false`.

The hole is **`get`**. Reading an absent key and reading a
present-none key both yield something whose `typeof` is `None` and
which answers `eq none` → true, `deq none` → true, `eq None` → true.
They *render* differently (`None` for the miss, `none` for the
binding — type literal vs value), but no comparison operator
separates them. So the distinction is observable through `has` /
`size` / `keys` / `eq`, and invisible through the reader. That is
the shape the **sentinel-values programme** the verdict opened has to
settle (a distinct miss sentinel would close it); it is recorded here
as context, not as a separate divergence.

### Fix (2026-08-02): the container sets are now identical

`del` dispatches over exactly the eleven containers `set` does, with
the same key shapes (String and Atom for keyed containers, Integer
for indexed) — 19 signatures each. Each container either removes the
slot or refuses with its own message:

| container | `del` |
| --- | --- |
| Map | copy-returning — a new map without the key |
| FlexMap, WeakFlexMap | in place, returns the node |
| FlexXml, WeakFlexXml | removes an **attribute** — the slot `set` writes |
| Store | copy-on-write, via a tombstone layer (`CowDel`) |
| Class | refused — a declared field is sealed |
| Micron | refused — immutable, mirroring `set`'s own refusal |
| List, FlexList, WeakFlexList | refused — names pop / shift / `ArrayUtil.remove-at` |

The refusals are **registered signatures**, not sig-absence, for the
reason `set`'s Micron form is: an absent signature raises an opaque
`signature_error`, a present one raises the specific message, and
negative spec rows can pin it.

The Store form needed new kernel machinery. `CowSet` layers a binding
over the old store because that store may be shared with an enclosing
scope; removal cannot work by subtraction, since there is nothing in
the new layer to leave out. So `CowDel` writes a **tombstone** and
`StoreInstanceInfo.Get` stops there — the key reads absent from the
deleting layer down while the layer that owns it is untouched. Own
`Data` beats a tombstone, so a `set` after a `del` re-binds; clones
carry tombstones, or a cloned prototype chain would resurrect every
deleted key.

Gate: `lang/go/native/native_del_symmetry_test.go` asserts the two
words carry the **same** container set and the same key shapes, and
fails in both directions — so a container added to `set` cannot
silently reopen the gap, and a `del`-only container is caught too.
Behaviour: `lang/spec/flex.tsv` §12; kernel:
`eng/go/store_tombstone_test.go`.

### Verdict (maintainer, 2026-08-14): Allowed — slots are not keys

One asymmetry survives on purpose: **`set` can write a declared Class
field and `del` cannot remove it.** Under the rule as originally
worded that was still a divergence; the verdict is that the WORDING
was wrong, not the behaviour.

A class field is a **slot**, not a key. The inverse of writing a value
to a slot is writing a different value, not deleting the slot — an
instance missing a declared field would no longer satisfy its own
type. The same reading is what makes the List refusal correct (`set`
replaces at an index; removal shifts the tail, which is a different
operation), so the line is not a special case for Class: it is the
same line drawn twice. The rule at the top of this record is
therefore restated as "a **key** that `set` can write, `del` can
remove", with slots explicitly out of scope, and the record is
**Allowed**.

What remains true and is deliberately NOT closed here: the `get` hole
recorded in the investigation step above — an absent key and a
present-`none` key are indistinguishable through the reader — belongs
to the **sentinel-values programme**, which the 2026-07-31 verdict
opened as its own design line. That programme decides whether a
distinct miss sentinel exists; it does not reopen this record.

---

## NUR024 — Two orderings by design: semantic (`cmp`) and deterministic (`tcmp`) {#nur024}

**Status:** Allowed · **Date:** 2026-07-31 (recorded Pending
2026-07-22; verdict: maintainer, via `design/NUR-RESOLUTION-PLAN.0.md`)

### The uniform rule

One comparison vocabulary, one totality regime.

### The divergence

`cmp`/`lt`/`lte`/`gt`/`gte` raise `[boru/incomparable]` across
families (`cmp true 1` errors) while `eq`/`neq`/`deq` are total
(`1 eq "1"` → false) and `tcmp` is an unrestricted total order — two
totality regimes inside one family, with `cmp` and `tcmp` answering
differently for the same pair.

### Why allowed

The language deliberately carries **two distinct orderings**, and the
divergence is that architecture made visible:

- **Semantic ordering** — `cmp`, `lt`, `lte`, `gt`, `gte`. These
  answer "which is greater, *as values in one domain*?" and therefore
  **reject meaningless comparisons**: `cmp true 1` has no semantic
  answer, and a silent cross-family verdict would hide a real type
  error at exactly the moment it is cheapest to catch.
- **Deterministic ordering** — `tcmp`. This answers "give me *some*
  stable, lawful total order over everything" and exists for
  implementation purposes: deterministic signature ordering,
  deterministic map-key walks, reproducible sorts of heterogeneous
  data. It never rejects, because its job is determinism, not meaning.

Equality (`eq`/`neq`/`deq`) is total in both regimes because "are
these the same value?" has an answer across families (no), while
"which is greater?" does not. The two-ordering separation should also
be stated at the architecture level — recorded as ADR candidate 5 in
the resolution plan (semantic vs deterministic ordering).

### Evidence

- REFERENCE.md §Comparison — both regimes documented with
  the rationale: the ordering words are "**family-restricted**" and
  raise `[boru/incomparable]` across families (:1202-1206), `tcmp` is
  "the **unrestricted** total order" (:1208), and the callout at
  :1214-1216 states "different types are simply *not equal* … Only the
  **ordering** words restrict".
- `eng/go/compare.go` (family restriction raising `incomparable`);
  `eng/go/compare_types.go` (tcmp's Rank-based total order);
  `lang/spec/compare.tsv` and `lang/spec/compare-restrict.tsv` — the
  positive/negative batteries pinning both regimes.

---

## NUR026 — Escape sets diverge between quoted strings and templates {#nur026}

**Status:** Pending (NARROWED — the escape VOCABULARY is resolved by
fix, 2026-08-15; the malformed-input REPORTING difference remains) ·
**Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** one escape vocabulary across string literal forms.
**Divergence:** quoted strings (`"…"`/`'…'`) accept jsonic's full
escape set (`\x41` → `A`, plus `\b`, `\f`, …); backtick templates
process only `\n \t \r \\ \` \$` — `size "z\x41z"` → 3 while the same
text in a template → 6 (the escape survives literally).
**Evidence:** `eng/go/parser/parse.go:1681-1714`
(`processTemplateEscapes`); `eng/go/parser/grammar.go:103-104,340-367`.
**Documentation status:** REFERENCE documents the restricted template
set but never states quoted strings accept a superset — the asymmetry
is undocumented.

**Root cause (source investigation, 2026-07-31):** the divergence is
an **implementation accident, not a design choice**. `setupBaseTokens`
(grammar.go:97-104) deletes the backtick from jsonic's `StringChars`
and `MultiChars` so jsonic's built-in string matcher never consumes
templates — necessary because templates need `${…}` interpolation,
which the plain string matcher cannot provide. That forced a
hand-rolled template scanner (grammar.go:340-367), whose
`processTemplateEscapes` reimplements escapes from scratch as a
minimal six-case switch (`\n \t \r \\ \` \$`) with everything else
falling through to "keep literally" — while quoted strings still ride
jsonic's native escape handling and get the full set. Templates were
severed from jsonic purely to bolt on interpolation, and the
replacement escape handler was never brought to parity.

**Verdict (maintainer, 2026-07-31 — resolve by fix,
`design/NUR-RESOLUTION-PLAN.0.md`):** boru shall **not** use the
jsonic JSON string lexer as-is for strings. Instead: a **custom
unified string lexer** — a vendored copy of jsonic's string lexer,
extended to also handle backtick templates (i.e. `${…}`
interpolation). One lexer then (1) preserves the full escape set
across every string-literal form (the rule this record seeks), (2)
makes string processing uniform — one escape vocabulary in exactly one
place — and (3) parses templates, interpolation included, correctly.
This retires the hand-rolled `processTemplateEscapes` path and its
minimal escape set. Stays Pending until the unified lexer lands.

**Verdict (maintainer, 2026-08-15 — resolve by fix):** **templates take
the full quoted-string escape set**, so one string syntax means one
escape vocabulary and `size "z\x41z"` and its template spelling agree.

Documenting the asymmetry was the cheaper option and was not taken: a
reader should not have to know which quoting form they are in to know
what `\x41` means. The narrowing direction (cutting quoted strings down
to the template set) was rejected outright — it removes working
spellings. The cost to watch and pin: a template containing a backslash
sequence that is inert today would start escaping, so the fix wants
rows for each newly-live escape and for the sequences that must stay
literal. Stays **Pending** until it lands.

### Resolved: the vocabulary (2026-08-15)

`writeStringEscape` (Go) / `readStringEscape` (TS) is now the single
escape vocabulary, and it is the quoted-string one, measured against
jsonic rather than assumed:

```
\n \t \r \b \f \v   the control characters
\xNN                one byte, two hex digits
\uNNNN              one rune, four hex digits
anything else       the character itself, backslash DROPPED
```

`size "z\x41z"` and its template spelling now agree. The last rule is
the behaviour change the verdict asked to pin: `\z` is `z` and `\0` is
`0` in a template exactly as in a quoted string, where both were
previously literal. The template-only spellings need no case of their
own — `\``` and `\$` fall into the default arm and yield the bare
character, which is what they always meant.

Pinned in `parser/spec/parse.tsv` (both ports, no shared code) for the
newly-live escapes and the flipped unknown-escape row, and in the two
ports' unit tests for `\b` / `\f` / `\v`, whose canon carries a raw
control byte no single TSV line can hold.

**The migration cost is real, and REGEX is where it lands.** A regex
written in a template is the common case of "a backslash sequence that
was inert": `\s`, `\[`, `\(`, `\?`, `\]` all used to survive to the
regex engine and now lose their backslash at parse time. Every
backslash a regex needs must be written DOUBLED in a template — `\\s`,
not `\s` — exactly as it already had to be in a quoted string.

The tree's own sources were the proof: a repo-wide scan for templates
whose meaning changes found **two lines, both in
`lang/go/modules/sift.boru`** — the size-suffix matcher (`\s`) and the
pattern tokenizer (`\[`, `\(`, `\?`, `\]`) — and both broke loudly
(`TestSiftBoruCoverage`: 13 failures, `error parsing regexp`) rather
than silently. Both are migrated in the same commit. The loudness is
the mitigating fact: a mangled regex fails to compile, so this is not
the class of change that quietly returns wrong answers.

### What REMAINS open — malformed input

A malformed `\x` / `\u` (too few digits, or a non-hex digit) is
reported differently by the two forms:

```
"a\xZZb"    ERROR: the escape sequence … does not encode a valid ASCII character
`a\xZZb`    'axZZb'   — the literal reading, no error
```

The VOCABULARY is uniform; what a well-formed escape means no longer
depends on the quoting form, which is the divergence this record was
opened for. What is left is error REPORTING, and closing it needs an
error channel the call site does not have: the template path is a
jsonic `LexMatcher` returning a `*Token`, so raising means changing the
lexer seam — the unified-lexer work the 2026-07-31 verdict sketched and
the 2026-08-15 verdict did not ask for. Recorded rather than silently
accepted, with a spec row pinning the residual.

---

## NUR039 — `slice` with a negative start silently ignores its end argument {#nur039}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** C3 `boru:cli`
scouting

### The uniform rule

An argument is honoured or refused, never ignored. Out-of-domain
indices elsewhere in the String family clamp predictably
(`slice 5 6 "abc"` → `''`, `slice 0 5 "-"` → `'-'`).

### The divergence

A NEGATIVE start silently collapses `slice start end s` to the
two-argument "drop N from the end" form, discarding `end` entirely:

```
slice -3 -1 'abcde'   →  ab
slice -3  2 'abcde'   →  ab
slice -3  5 'abcde'   →  ab
slice  1  3 'abcde'   →  bc     (the positive form honours end)
```

Three different `end` values, one answer. The negative-index
convention is documented as "count from the end"; that an `end`
argument is then dropped is not.

### Why allowed

The affected spelling is a negative start, which every caller in this
repository can avoid by clamping — and clamping is what a caller wants
anyway, since a negative index is a bug at the call site more often
than an intent to count from the end. The alternative fixes (honour
`end` for a negative start, or refuse the combination) are both
behavioural changes to a core sequence word, which is a larger edit
than the confusion it removes.

The acceptance rests on callers not reaching the spelling, so the
guard that matters is an *upstream* one:

- `utils/cut.boru`'s `cut-span` (:180) and `cut-point` (:165) reject
  `lo < 1` before any range reaches the slicing helpers
  (`cut-err-rng "fields and characters are numbered from 1"`), so no
  negative start is constructed in the first place.
- `utils/tests/cut_test.boru` pins that rejection and the clamped
  behaviour at both ends.

**Correction (2026-08-02 review).** This record previously claimed the
pin was that `cut-chars-rng` "clamps the start explicitly". It does
not: `cut-chars-rng` (utils/cut.boru:329-335) computes
`def a ((rg get 0) sub 1)` with no start clamp and clamps only the
END (`def b (if (hi gt n) [n] [hi])`); its `if (a gte b)` guard is an
empty-range test that a negative `a` against a positive `b` passes
straight through. Replaying its body with `lo = 0` reproduces this
record's own divergence inside the function that was cited as its pin.
The register inherited the error from the source comment above that
function, which mis-described its own body until this review corrected
it (the comment now runs utils/cut.boru:322-328 and says the opposite).
The acceptance survives — the real guard is the upstream `lo < 1`
rejection above, so `cut` is correct today — but the local fragility
is now recorded rather than mis-pinned.

**Correction (2026-08-02 review).** The motivating example previously
given here — `slice (ep add 1) (size tok) tok` where `ep` is `-1` from
a failed `indexof` — does not exhibit this divergence: `(ep add 1)` is
`0`, a NON-NEGATIVE start, and `end` is honoured normally. It illustrates
an off-by-one, not the negative-start collapse. The spelling that does
trigger it is the same call without the `add 1`.

### Evidence

- The four `slice` calls above, verified on the current binary.
- `utils/cut.boru:165,180` (the upstream `lo < 1` rejection) and
  `utils/tests/cut_test.boru`.
- NUR019 records the separate question of where `slice` belongs, and
  its 2026-08-02 verdict is that `slice` is a core **sequence** word,
  not a String-family straggler; this record is an independent defect
  in the same word and takes no position on filing.

---

## NUR040 — `set` quotes a bare computed key where `get` refuses it {#nur040}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** C3 `boru:cli`
scouting

### The uniform rule

Sibling accessors treat their key argument the same way, and a program
that means a variable's VALUE does not silently get its NAME.
lang/go/CLAUDE.md states the split it intends: `dot`/`dotr` quote a
bare word as a literal field name, `get`/`getr` evaluate it.

### The divergence

`set` carries the quoting `Atom/q` slot that `get` does not, so the
same bare-word spelling means opposite things:

```
def k "aa"   {} set k 1     →  {k:1}      # the NAME was stored
def k "aa"   {} set (k) 1   →  {aa:1}     # the VALUE
def k "aa"   {aa:1} get k   →  1          # get EVALUATES k
```

`boru check` reports no ERROR for the first line. It does emit
`[warning] unused_def: def k is never used` — which is the tell, since
that warning appears for neither alternative spelling — but nothing
names the actual hazard. The failure mode in real code is a map built
entirely under one literal key: every iteration of a loop overwrites
`{k:…}`, and only an unused-binding warning hints at it.

### Why allowed

The asymmetry leaks from a distinction that is deliberate and
load-bearing elsewhere — `dot`/`dotr` quote a bare key, `get`/`getr`
evaluate one (lang/go/CLAUDE.md, "dot / dotr vs get / getr"). Making
`set` match `get` is a behavioural change to a core word, which is a
larger and riskier edit than the confusion it removes. The quoting slot
has a real purpose (`set name value store` reads well).

The standing improvement, not required by this allowance: a check-mode
advisory when a bare word passed to a quoting slot is ALSO a live
binding — the one case where the two readings differ and the author
almost certainly meant the value. The `unused_def` warning above is an
accidental partial signal of exactly that condition.

### Evidence

- The three calls above, and the three `boru check` runs behind the
  warning claim.
- **The two files this record was scouted from never pass a bare word
  to `set`'s key slot**, so neither depends on which way the ambiguity
  resolves: `utils/` spells every LITERAL key `(quote k)` (117 sites, 0
  exceptions), and `lang/go/modules/cli.boru` uses `(quote …)` at its
  75 literal-key sites (42 distinct names) and the parenthesised value
  form (`set (nm) …`) at its 8 computed ones. Its house rule at
  cli.boru:53-54 states the convention: "a computed map key is always
  parenthesised (`m set (k) v`) — a bare `k` stores the literal name
  \"k\", with no diagnostic at all."
- **Elsewhere the repo does rely on the quoting reading**, which is the
  real reason the fix is riskier than the confusion:
  `lang/go/modules/vault_tui.boru` — shipped, `//go:embed`-ed — has 77 bare-word key
  sites (`grep -oE '\bset +[a-z][a-zA-Z0-9_-]*'`; 75 excluding the two
  that follow a `-`-suffixed word) (`state set screens …`, `state set status …`),
  and `kg/report.boru:334`, `design/examples/apps/todo-tui.boru:52` and
  the linguist samples do the same. Making `set` evaluate its key would
  change all of them.
- lang/go/CLAUDE.md:303-316 — the "**`dot` / `dotr` vs `get` / `getr`
  (CRITICAL)**" bullet inside §"Parser Customization" (a bolded
  lead-in, not a section) — the deliberate split this record's
  divergence leaks from.

---

## NUR046 — `boru fmt` is not idempotent: one pass is not a fixed point {#nur046}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** the C3 utils
suite (`utils/`)

### The uniform rule

A formatter is idempotent. `fmt(fmt(x)) == fmt(x)`, so "formatted"
is a property a file either has or does not, a `make fmt` target converges,
and a formatting check can be a single-pass diff. `make fmt-docs` and
`kg/Makefile`'s restored `fmt` target both rely on this.

### The divergence

On a `def name fn [[params] [Returns] [body]]` whose header
does not fit the width, the FIRST pass and the SECOND pass produce different
layouts. It converges at pass 2 — passes 2..n are identical — so the fixed
point exists; one application simply does not reach it.

```boru
# m.boru, as hand-written:
def cat-format fn [[line:String k:Integer numbered:Boolean ends:Boolean] [String] [
  def body (if ends [(join "" [line "$"])] [line])
  join "" [body "\n"]
]]
```

```
$ boru fmt m.boru && cat m.boru          # pass 1
def cat-format fn
  [[line:String k:Integer numbered:Boolean ends:Boolean] [String] [
  def body (if ends [(join "" [line "$"])] [line]) join "" [body "\n"]
]]

$ boru fmt m.boru && cat m.boru          # pass 2 — different, and stable
def cat-format fn
[[line:String k:Integer numbered:Boolean ends:Boolean] [String]
      [def body (if ends [(join "" [line "$"])] [line]) join ""
          [body "\n"]
      ]
  ]
```

The blast radius is in §Evidence below; program output is unchanged in
every affected file, and every one still passes `boru check`.

**Why it matters:** three ways.

1. A `fmt` target inside an `all:` target never converges in one run, so
   `make all` always leaves a dirty tree — which is why `utils/Makefile`
   deliberately keeps `fmt` OUT of `all` and says so, the same posture
   `kg/Makefile` held while NUR028 was open.
2. Pass 1 joins two statements onto one line (`… [line]) join "" [body …`)
   and pass 2 re-indents a statement as though it continued the previous
   one. Both are legal — boru is whitespace-insensitive — but a reader
   cannot tell statement boundaries by eye any more, which is most of what
   a formatter is for.
3. It is a fixed-point bug in the same component as the resolved
   superlinear blow-up, in a shape that blow-up's gate would not have
   caught: that gate compared old-binary and new-binary output on the
   *repo's already-canonical* corpus, where pass 1 is already the fixed
   point. Non-canonical input is the untested axis.

### Documentation status

`kg/Makefile:25-28` claims idempotence in so
many words — "the formatter is idempotent, so once they are canonical
this is a no-op on the tree and `make all` leaves nothing to commit" —
with `fmt` inside its `all` target (kg/Makefile:11). kg's own sources
happen to sit at their fixed point, so no dirty tree results today, but
the written claim is false in general. `kg/README.md` and `make
fmt-docs` likewise treat a single `fmt` run as producing canonical
form.

**The mechanism (corrected 2026-08-02).** This record originally
proposed that "the first pass measures widths against a pre-wrap layout
decision it then invalidates". `design/NUR-EFFORT-TRIAGE.0.md:139-148`
(the NUR046 bullet; the cause statement at :140-141) investigated and
found otherwise: the true cause is **re-parse
statement-segmentation drift** (root-level newlines emitted by pass 1
change how pass 2 segments statements). The width-memoisation framing
is retired.

**The standing fix, when scheduled:** a regression guard belongs with
it — format every `.boru` in the repo TWICE and require the second pass
to be a no-op, with at least one deliberately non-canonical fixture,
since the already-canonical corpus cannot detect this.

### Why allowed

Formatting does not change behaviour — all 995 cases in `utils/` pass either
way, verified — so what the non-idempotence costs is a clean tree and
readable sources, not correctness. It converges at the second pass, so a `fmt` target
that ran twice would be stable; the reason not to paper over it that way is
that the intermediate layout runs statements together on one line, which is
most of what a formatter is for.

### Evidence

- The repro above, and the **repo-wide sweep** (re-run 2026-08-02 on
  the current binary): of the 122 tracked `.boru` files, **19 are
  non-idempotent** — all 12 `utils/*.boru` programs, three SHIPPED
  library modules (`lang/go/modules/cli.boru`, `sift.boru`,
  `vault_tui.boru`), two `design/examples` programs and two
  `editors/linguist/samples`. All 11 `utils/tests/*_test.boru` suites
  ARE at their fixed point after one pass, which is the qualitative
  split that makes the divergence easy to miss.
- `utils/Makefile` keeps `fmt` OUT of its `all` target and its comment
  names this record and explains why — the same posture `kg/Makefile`
  held while its own formatter blocker was open — so the tree cannot
  silently start churning on every build.
- `kg/Makefile:11,25-28` — the idempotence claim named above, the one
  place the property is asserted rather than assumed.

**Correction (2026-08-02 review).** This record previously said the
non-idempotence hits "all six programs" in `utils/` with "the five
`tests/*.boru` suites" already at their fixed point. Those counts were
accurate on 2026-07-30 when the record was written (the tree then held
six programs and five suites) and have since drifted: it is 12 and 11,
and the blast radius reaches shipped `lang/go/modules/*.boru`, a scope
the record never mentioned. An **Allowed** record carries "the evidence
that pins it … so the acceptance cannot silently rot"; this evidence
had rotted by a factor of two.

---



## NUR056 — `make`-constructibility is the one capability with no opt-in {#nur056}

**Status:** Pending · **Recorded:** 2026-08-02 · **Surfaced by:** NUR
register review (auditing the TypeBehavior capability surface)

> Numbered 056, not 055: NUR055 was opened and resolved earlier in this
> same review (`cde2d3f` then `b5051be` — Big numeric values reading as
> uniformly falsy), and a resolved record's number is retired forever.

**Rule:** a type says how it participates in a kernel operation
through its Behavior — one mechanism, reachable from Go via a
capability interface and from boru via a `behave` slot.

**Divergence:** every kernel operation follows that rule except
construction. Ordering, rendering, membership, unification, hashing,
walking, projection, const-baking, truthiness, deep equality and size
are all capability-dispatched, and seven of them have `behave` slots
(`compare`, `canon`, `nodify`, `unify`, `truthy`, `deq`, `size`).
`make` has neither: its scalar arm is a closed switch over the kernel
leaves, and its Ideal arm dispatches through a *registry* of Ideal
kinds rather than through the type's Behavior.

```
$ boru do 'def C (refine Float) behave make/q (fn [[String] [C] [1.0]])'
  error: behave make: unknown behavior name;
         known: canon, compare, deq, nodify, size, truthy, unify
```

So a user type can define what it MEANS in every operation that
consumes it, and nothing about how it is BUILT. A Go-side Ideal can
(via `Ideal.Instantiate`), which makes this also a Go-vs-boru
asymmetry, not only a missing capability.

**Evidence** (paths relative to the repo root):
`eng/go/core_make.go` — `MakeConvert`'s `default:` arm raising
"make: unsupported target type" is the closed scalar switch;
`MakeObjHandler`'s `reg.Ideals.For(targetVal).Instantiate` is the
Go-only Ideal hook. `lang/go/native/native_behave.go` — the
`behaviors` table, seven entries, no construction slot.
`eng/go/typebehavior.go` — the capability interfaces that do exist.

**Documentation status:** undocumented. `boru describe behave` now
lists the seven installable slots, and nothing states that
construction is not among them or why.

**Verdict (maintainer, 2026-08-14 — resolve by fix):** add the
`Maker` capability (`fn [[Any] [T]]`), dispatched by `MakeConvert`
before its scalar switch and by `MakeObjHandler` before the Ideals
registry, plus the eighth `behave make/q` slot — completing the
capability surface.

The counter-argument was considered and not taken: construction has no
receiver to dispatch on, only a target type and an arbitrary source,
which is a real disanalogy with the seven value-Behavior slots. What
it does not answer is the **Go-vs-boru asymmetry** — a Go-side Ideal
already customises construction through `Ideal.Instantiate`, so the
capability exists; only its boru-side spelling is missing. Closing
that is the fix, and it is what makes the documented capability list
honest rather than requiring a paragraph explaining an omission.

Scope, unchanged by this verdict: it does NOT block on the
sentinel-values programme or on NUR018 (`Store`/`Error` deliberately
not being `make` targets) — those decide WHICH types construct, this
decides WHO gets to say how. Stays **Pending** until the capability,
the slot, and their negative rows (a declined `Maker` falls through to
today's behaviour; a wrong-typed return is refused) land.

---

## NUR072 — Three sugar kinds still canon in debug form, and their source spelling is not recoverable {#nur072}

**Status:** Pending · **Recorded:** 2026-08-15 · **Surfaced by:** NUR059's
fix — the residue its per-row fixpoint check refused

**Rule:** `CanonValue` renders canonical boru **source** — the string it
produces parses back as the same value (ADR-015).

**Divergence:** NUR059 gave the angle sugar, the paren group and the word
`/`-modifiers a source form. Three sugar kinds were attempted in the same
change and **withdrawn**, because a fixpoint check — does the new render
re-parse to itself? — proved the spellings wrong:

```
+m'src'   rendered  +m<src>    re-parses with a stray `>`
w/t       rendered  [w/q]/t    does not parse at all
```

The mini case is not a coding slip: **`SugarInfo` does not retain the
source delimiter.** `+m'src'`, `+m<src>` and `+m|src|` all lex to the same
payload, so no renderer can reproduce what the user wrote without the
parser keeping it. The type-bound case renders its `Items`, which hold the
bound as a list containing an atom rather than the bound's own text.

The lambda marker is a third shape: `=>` renders fine on its own, but every
row containing one ALSO contains a bare word, which canon spells
`word(x)` — so the row cannot reach a fixpoint whatever the lambda does.

**Verdict:** none yet. A spelling that does not round-trip is worse than
the debug form, because it looks like source; that is why these were
withdrawn rather than shipped. Fixing them needs either a parser change
(retain the mini delimiter) or the bare-word decision below.

**A THIRD kind, found by Codex review on PR #372:** the `/N` arity is an
exact int64 in Go and a JS `number` in TS, so a magnitude above 2^53 is
ROUNDED at parse — `x/9223372036854775807` canons as
`x/9223372036854776000` in TS, outside the accepted int64 range, so it
cannot be re-parsed as a modifier at all. The loss is in the TS
**payload**, not the renderer, which only made it visible; fixing it
means carrying the arity exactly in `parser/ts`. Pinned meanwhile as a
row in `parser/spec/divergent.tsv`, the shrink-toward-zero ledger.

**The bare-word question, deliberately not decided:** canon spells a Word
value `word(foo)`. Rendering it bare would change **175 of
`parser/spec/parse.tsv`'s 724 rows**, and it is a real question rather
than a formatting one — `word(foo)` denotes a word VALUE, while bare
`foo` re-parses as a word that will be DISPATCHED. NUR059 scoped its word
arm to modifier-bearing words for exactly this reason. Whoever takes this
record decides that first.

## NUR073 — `deq` is extensible per type, `eq` is not {#nur073}

**Status:** Pending · **Recorded:** 2026-08-16 · **Surfaced by:** the
retired NUR031's fix — the one part of its verdict the fix did not take

**Rule:** a per-type operation is reached through the type's capability
seam, so a type can answer for its own values. `Truther`, `Sizer`,
`Comparer`, `DeepEqualer` and `Formatter` all work this way, exposed to
boru code through `behave`.

**Divergence:** `deq` has `DeepEqualer` — a type installs one and
`DeepEqual` consults it (`core/go/deepequal_capability.go`, reached at the
bottom of the walk so it can only turn the terminal `false` into a real
answer). `eq` has no counterpart. `ExactEqual` is entirely hardcoded
arms, so a host or boru type can define what "same value" means for its
values but not what "same thing" means, and the two halves of one word
family are extensible on different terms.

**How it got here.** NUR031's verdict proposed routing `eq`/`deq` through
the type's `Behavior` for all Ideals, replacing the kernel's hardcoded
arms wholesale. Its fix did not do that: it closed every divergence the
record measured — reflexivity for every kind, identity that survives
rebinding, a name-independent function canon — by adding arms rather than
removing them, and the proposed mechanism turned out to be a refactor of
the most-exercised path in the kernel with no observable change to show
for it. What the mechanism WOULD have bought, and the arms do not, is
this one asymmetry. It is recorded here so the deviation from that
verdict is visible rather than lost with the retired record.

**Cost of the divergence:** narrow today. A type wanting reference
identity for its values gets it already — every kernel-known payload
kind has an arm, and a sealed host payload gets box identity by default
— so nothing is currently *unable* to answer `eq`. The asymmetry bites
when a type wants an `eq` that is neither box identity nor the kernel's
guess: an interned value that should be `eq` across two constructions,
or a handle whose identity lives one level below its payload box.

**Proposed verdict:** none yet. The candidates are an `ExactEqualer`
capability mirroring `DeepEqualer` (small, symmetric, and paid for only
by types that install it); the full `Behavior` routing NUR031 proposed
(uniform, but a large refactor of a hot path); or Allowed, on the
argument that reference identity is a KERNEL property — what "the same
thing" means is the runtime's answer, not a type's — in which case
`DeepEqualer` is the sound asymmetry and the register should say so.
The third reading is the strongest and is why this is not filed as a
fix-by-default.

**Evidence:** `core/go/deepequal_capability.go` (the `deq` capability and
its LCA walk); `core/go/compare.go` — `ExactEqual`'s arms, which no
capability can reach, against `DeepEqual`'s terminal
`deepEqualCapability` call; `lang/go/native/native_behave.go` (the
`behave` slots, `DeepEqualer` among them and no `eq` twin);
`core/go/capability_gaps_test.go`.

## NUR060 — The parser twins disagree on open-input sources beyond the corpus {#nur060}

**Status:** Pending · **Recorded:** 2026-08-09 · **Surfaced by:** PR #337
parity-probe sweep; flagged for this register by the PR #337 review
(Codex P1)

**Rule:** one language contract, two implementations: `parser/go` and
`parser/ts` must render every source identically — the uniformity the
`parser/spec` corpus exists to enforce.
**Divergence:** a 2,587-source probe sweep measured 55 sources (~2.1%)
where the twins disagree, and follow-up probing added a class the sweep's
seed missed — nine classes so far: trailing-`=>` fold loss (TS drops the
paren group Go folds — also inside dotchains), two accept/reject splits
(trailing bare `:` — Go accepts, TS refuses; `=> ,` — Go refuses, TS
accepts and silently drops tokens), post-`]` recovery-token detail (TS
reports an empty token where Go names the offender), two error-precedence
splits (receiverless-`.` vs unmatched-`(`, and bare-`/s` vs
unmatched-`(`), an internal type-name leak in one message on both sides,
and an empty-`${}` fold split in an unterminated template (Go folds to
`interp('')`, TS keeps the hole).
**Evidence:** `parser/spec/divergent.tsv` — the live ledger, one measured
row per class; both spec runners re-render every row against their own
column on every run, so a row can neither rot nor survive its fix.
`scripts/parity-probe.sh` reproduces the sweep.
**Documentation status:** parser/spec/README.md §"The current debt" and
design/GO-TS-PARITY.0.md carry the honest scope: corpus parity exact,
open-input parity not.
**Proposed verdict:** resolve by fix, class by class — each fix moves its
ledger row to `parse.tsv` (the runners force the move: a fixed divergence
fails the ledger loudly). The behavioral classes (fold loss, the two
accept/reject splits) should go first; the diagnostic-detail classes
follow. The record discharges when the ledger is empty again.

> **Update (2026-08-10).** Two DATA-seam asymmetries this record also
> carried — the TS seam's inability to express map insertion order for
> integer-like keys, and Go alone reading `+_1` as a number — were
> dependency defects, not boru's. Both are fixed upstream in
> `tabnas/jsonic v0.6.0` / `tabnas/parser v0.8.0` (ADR-014) and are now
> pinned by ten `data.tsv` rows rather than described in prose. The nine
> GRAMMAR-level classes above are untouched by the upgrade: a fresh
> 2,587-source sweep on the new dependency measures the byte-identical 55
> divergences, which is what distinguishes the two categories. This record
> stays Pending on those nine.

---

## NUR062 — Numeric marker letters are lowercase-only while every other letter in a literal is case-flexible {#nur062}

**Status:** Allowed · **Date:** 2026-08-14 · **Recorded:** 2026-08-11 ·
**Surfaced by:** the maintainer's decision on PR #339 ("only lowercase should
be valid for numeric syntax prefixes"); flagged for this register by the
PR #339 review (Codex P1)

**Rule:** one lexical convention per kind of thing. Letters inside a numeric
literal are either case-significant or they are not.

**Divergence:** they are now both. The four MARKER letters are lowercase-only
— `0x`, `0o`, `0b` and the big-number `0d`, so `0XFF` raises
`[boru/syntax_error]: numeric prefix must be lowercase: 0XFF` — while every
other letter a numeric literal can contain stays case-flexible:

```
0xff  ==  0xFF        hex DIGITS take either case
1e3   ==  1E3         the exponent marker takes either case
0XFF  ->  syntax_error    but the base marker does not
```

So `0XFF` is refused and `0xFF` accepted, yet `1E3` and `1e3` are equally
valid, and `0xAB` and `0xab` are the same value. A reader cannot derive one
from the other; each has to be learned.

**Scope, measured.** The rule governs numeric LITERALS only. A run in a NAME
position is not a literal and behaves identically in both cases — a quoted
atom (`0XFF/q`), a `/r` word reference (`0XFF/r` -> `word(0XFF)`), a
type-bound (`0XFF/t`), and a bare map key (`{0XFF: 1}`) are all names, never
numbers, exactly as their lowercase spellings are. The `0d` family is not a
DATA numeric in either case, so `0d12` and `0D12` both decode as lenient text
through `StructUtil.parse`. Both boundaries are pinned by rows rather than
left to prose: `parser/spec/parse.tsv` §"the lowercase-only rule governs
numeric LITERALS" and `parser/spec/data.tsv` §"the 0d big-number prefix is not
a DATA numeric".

**Evidence:** `parser/spec/parse.tsv` (8 refusal rows + 5 name-position rows),
`parser/spec/data.tsv` (4 refusal rows + 3 `0d` rows), `parser/spec/lex.tsv`
(2 token rows), each re-rendered independently by both port runners.
`REFERENCE.md` §"Numeric literals" states the rule and its scope.

**Documentation status:** stated in REFERENCE.md; both editor grammars
(tree-sitter, pygments) reject uppercase markers so highlighting cannot
advertise a literal the language refuses.

**Verdict (maintainer, 2026-08-14): Allowed** — as proposed. The asymmetry
is deliberate: a marker is a *spelling of syntax* while digits and exponents
are *content*. `0XFF` is a typo for `0xFF` far more often than it is anything
a user meant, whereas `0xAB` vs `0xab` and `1E3` vs `1e3` carry no such
signal, so refusing the first while accepting the others is a diagnostic, not
an inconsistency. The rule is already stated in REFERENCE.md §"Numeric
literals" with its scope, pinned by refusal and name-position rows in
`parser/spec/` that both port runners re-render independently, and both
editor grammars reject uppercase markers so highlighting cannot advertise a
literal the language refuses. No code or documentation change follows from
this verdict — the record closes as it stands.

---

## NUR063 — Seven self-knowledge words are proposed to dispatch from two module surfaces (`boru:debug` and `boru:scry`) {#nur063}

**Status:** Pending · **Recorded:** 2026-08-12 · **Surfaced by:**
design/BORU-SCRY.0.md §6 (the boru:scry proposal); flagged for this
register by the PR #344 review (Codex P1)

**Rule:** one capability, one home: a word lives in exactly one module
surface and `boru describe` names it there — the module taxonomy
`lang/go/CLAUDE.md` documents, and the same single-source value behind
ADR-001's no-shadowing rule.

**Divergence:** `boru:debug` shipped with seven data-returning
self-knowledge words (`words`, `defs`, `modules`, `sig`, `body`,
`deps`, `shape`). The accepted-in-discussion direction (2026-08-12)
splits introspection into `boru:scry`, which adopts those seven — so if
the proposal ships as designed, the same seven capabilities dispatch
from two module surfaces backed by shared Go handlers. No code exists
yet; the divergence begins the day `BuildScryModule` registers them.

**Evidence:** design/BORU-SCRY.0.md §2 (the overlap inventory) and §6
(the containment plan: shared handlers so behaviour cannot fork, scry
canonical, the debug copies frozen at today's seven, `boru describe`
marking the debug variants' canonical home).

**Documentation status:** the plan is stated in design/BORU-SCRY.0.md
§6 and its open question §9 Q1; nothing user-facing exists to update
yet.

**Proposed verdict:** none yet — deliberately. Keeping both surfaces
indefinitely is an **Allowed** argument the maintainer may make;
deprecating and later removing the debug copies is a resolve-by-fix
path. design/BORU-SCRY.0.md §9 Q1 puts that choice to the maintainer
with a lean (keep through one release, then decide with usage
evidence). Recorded now so the dual surface cannot ship as an
unexamined default.

**Verdict (maintainer, 2026-08-15):** **`boru:scry` is canonical.** The
`boru:debug` copies stay, frozen at today's seven and backed by the
SAME handlers so behaviour cannot fork, with `boru describe` naming
scry as the canonical home — and they are **deprecated on a stated
timeline** rather than kept indefinitely.

That is the one-capability-one-home rule applied with a migration path
instead of a break: keeping both forever would need an argument for why
this capability is exempt, and none was offered beyond convenience.
Recorded before `BuildScryModule` exists, so the dual surface never
ships as an unexamined default — which is what this record was opened
to prevent. Stays **Pending** until scry ships with the deprecation
notice in place.

---

## NUR064 — Pattern clauses route-and-bind in `receive` but route-only in `add` {#nur064}

**Status:** Pending · **Recorded:** 2026-08-12 · **Surfaced by:**
`design/STATE-MACHINES.0.md` §8 (which names the asymmetry while declining to
solve it there); flagged for this register by the PR #345 review (Codex P1).
The split itself was designed deliberately in `PROCESSES.0.md` §3 and
`SERVICES.0.md` §1 but never recorded here.

**Rule:** one pattern-clause semantics per matcher. The service `add` and the
process `receive` route through the same patrun matcher, and "match a message"
is supposed to mean one thing everywhere (`SERVICES.0.md`: "the *same* matcher
`receive` uses … 'Match a message' means one thing everywhere").

**Divergence:** the two consumers give the same clause surface different
powers. A `receive` clause pattern is two layers — scalar-tag *routing* via
patrun plus `name:Type` *binding* slots destructured by the fn-param machinery
(`PROCESSES.0.md` §3 "clause matching: routing vs. binding") — while an `add`
pattern is routing only: "An `add` pattern routes; it does not bind"
(`SERVICES.0.md` §1). So `{op:"create" text:String}` binds `text` into the
body as a `receive` clause but silently binds nothing as an `add` pattern,
where the handler must destructure `req.text` by hand. A reader cannot derive
one behaviour from the other; each has to be learned.

**Evidence:** `PROCESSES.0.md` §3 (the two-layer clause spec, including the
explicit callout "The binding layer is specific to `receive`"); `SERVICES.0.md`
§1 (the route-only rule for `add`); `design/STATE-MACHINES.0.md` §8 item 5
(the asymmetry surfacing as a cost for any facility built over both).

**Documentation status:** both design docs state their own side explicitly;
no user-facing doc contrasts them.

**Proposed verdict:** none yet — genuinely open. The candidate resolutions
pull opposite ways: generalize binding slots into a facility `add` (and other
patrun consumers) can opt into, or declare the split Allowed on the argument
that `add` patterns are routing *tables* (inspectable, whole-request handlers)
while `receive` clauses are *destructuring* sites. Deciding belongs to the
processes/services design line; this record exists so the divergence is not
silently baselined meanwhile.

**Verdict (maintainer, 2026-08-15 — defer, deliberately):** decided in
the **processes/services design line**, when those modules are actually
built, not here and not now. Both candidate resolutions (generalise
binding slots into a facility `add` can opt into, or declare the split
Allowed on the routing-table-vs-destructuring-site argument) depend on
implementation experience this record does not have.

The record keeps doing its job meanwhile: the divergence is written
down, so building either module against the other's assumption is a
choice rather than an accident. Stays **Pending** by design.

## NUR065 — Two spellings of the classifier role get different static guarantees {#nur065}

**Status:** Pending · **Recorded:** 2026-08-14 · **Surfaced by:**
`design/STATE-MACHINES.0.md` §3.6 (which introduces both spellings and states
the asymmetry as a preference rather than resolving it); flagged for this
register by the PR #352 review (Codex P1).

**Rule:** one role, one set of guarantees. A spec key's static checking should
follow from *what it means*, not from which of two spellings the author
happened to pick — the same uniformity principle behind one parser, one
argument-positioning convention, one total order.

**Divergence:** `boru:state`'s classification edge has two spellings of a
single role — "turn a raw input into an alphabet member" — and they are
checked to different standards:

- **Alphabet closure.** Every `classes:` key must be a declared `events:`
  member, checked at define time (`state_unknown_name`, §6.3). A `classify:`
  fn's output domain is not statically knowable, so an event atom it invents
  that `events:` never declared is only a step-time `state_bad_event`. The
  §3.3.11 closure guarantee therefore holds up to the machine's edge for one
  spelling and through it for the other.
- **Payload shape.** `classes:` produces a frozen `{event: <class> raw: <v>}`
  (§3.6.2), so a reducer always reaches the input as `ev.raw`. A `classify:`
  fn returns the whole event map and owns its payload shape, so nothing about
  a classified event's contents is derivable from the spec.
- **Diagnostics.** `state_bad_class` (Error) and `state_class_gap` (Info)
  exist only for the table form. The fn form has no analogue of either, so a
  classifier with an unreachable branch or an uncovered input domain is
  invisible to `boru check` and to `State.lint`.

**Evidence:** `design/STATE-MACHINES.0.md` §3.6.1 (the fn form's stated cost),
§3.6.2 (the four frozen table-form properties and the payload freeze), §6.3
(the `state_*` table, where the two class diagnostics have no fn-form rows),
and open question #7 (whether the fn form should ship in v1 at all).

**Documentation status:** the design document states the asymmetry openly and
names `classes:` the preferred form; no user-facing doc exists yet, since the
module is unimplemented.

**Proposed verdict:** none yet — it is open question #7, and the candidate
resolutions are the obvious three. Drop the fn form, making the role uniform
by having one spelling (cleanest, but leaves inputs no partition describes
with no answer). Keep both and declare the split Allowed on the argument that
a *declarative partition* and an *arbitrary function* are honestly different
things whose guarantees cannot be equal — in which case `State.lint` should
report fn-form classification so the weaker guarantee is visible at the use
site, not just in this document. Or narrow the gap by requiring a fn-form
classifier to declare its output alphabet in the spec, recovering closure
while leaving the mapping opaque. This record exists so the divergence is not
silently baselined while that question is decided.

**Verdict (maintainer, 2026-08-15 — defer, deliberately):** decided in
the **state-machine design line** as its open question #7, when
`boru:state` is built. The three candidates (drop the fn form, keep
both with `State.lint` reporting the weaker guarantee, or require a
fn-form classifier to declare its output alphabet) all turn on how the
module actually gets used, and the document is still in flux.

Stays **Pending** by design, so the asymmetry cannot be silently
baselined while that question is open.

---

## NUR070 — `if` reads a List condition as CODE while every other truthiness consumer coerces it {#nur070}

**Status:** Allowed · **Date:** 2026-08-15 · **Recorded:** 2026-08-14 ·
**Surfaced by:** implementing NUR053's fix (measuring whether the three
consumers really share a domain once `convert Boolean`'s slot was
widened)

**Verdict (maintainer, 2026-08-15): Allowed — a List in a condition
position is CODE.** That is what a concatenative language should mean by
a bracketed body there, and the One Truthiness Model governs *values*,
not code positions: `if [ … ]` running its condition is the language's
way of spelling a computed condition, not an accident to be coerced
away. The two readings genuinely differ, and the code reading is the
intended one.

What the allowance costs, stated so it cannot rot: `design/TRUTHINESS.0.md`
§2 must say the model has one shape-shaped hole — a List reaching `if`
is executed, so the "every consumer agrees" claim holds for Map, None,
String and the numeric leaves and NOT for List — and `if xs` where `xs`
holds a list stays a sharp edge for anyone who expected presence
coercion. The §2 amendment landed with NUR053's fix and already states
both the domain and this split, with the measured opposite-answer table.
The spec rows in `lang/spec/edge-scalars-3.tsv` pin it in both
directions, so the accepted behaviour is executable rather than merely
described.

**Rule:** one truthiness model, applied by every construct that coerces
a value to a Boolean — `design/TRUTHINESS.0.md`, the One Truthiness
Model. NUR053's premise, and the sentence this record corrects, was that
"`if` and `make Boolean` accept any value".

**Divergence:** they do not agree on a **List**. `make Boolean` and
`convert Boolean` coerce a list by PRESENCE (non-empty → true); `if`
does not coerce it at all — it runs it as a **code body** and takes the
truthiness of what the body leaves. The two readings give opposite
answers on the same bound value, and the disagreement is not confined to
an edge case:

```
def xs [0]   if xs ['T'] ['F']        # → 'F'     the body runs, yields 0, falsy
def xs [0]   convert Boolean xs       # → true    non-empty list is present
def xs [0]   make Boolean xs          # → true

def xs []    if xs ['T'] ['F']        # → [boru/runtime_error]: if: condition
                                      #   produced no value
def xs []    convert Boolean xs       # → false
def xs []    make Boolean xs          # → false
```

Every other source shape agrees. Measured across the three consumers:
Map (`{}` → false, `{a:1}` → true), `none` → false, empty String →
false, and the numeric leaves including the Big ones (NUR055's rows)
all give the same answer through `if`, `convert Boolean` and
`make Boolean`. The List is the sole shape where the consumers split.

**Why it is not simply a defect in `if`:** the code-body condition is a
deliberate, documented form — `if [ … ] [then] [else]` runs its
condition, which is how a computed condition is spelled, and the
compiled path models it (the "if code-body condition" row in
`lang/go/context_boundary_differential_test.go`). The non-uniformity is
not that the form exists; it is that **the same value gets two different
readings depending on how it reaches `if`**, with nothing at the call
site to distinguish them: a literal `[ … ]` is unambiguously code, but a
bound name holding a List is read as code too, where the value reading
is at least as plausible.

**Evidence:** `lang/spec/edge-scalars-3.tsv` (the NUR053 domain block
now pins both sides — the Map/None agreement rows and the three List
rows showing the split); `core/go/core_helpers.go` `CoerceBoolean` (the
presence rule the two constructors share); `design/TRUTHINESS.0.md` §2
(amended by NUR053's fix to state the domain, and to name this split).

**Documentation status:** newly documented by this record and the
TRUTHINESS.0.md §2 amendment; before them, nothing stated that `if`'s
domain differs from the constructors' for one shape, and NUR053's own
text asserted the opposite.

**Proposed verdict:** argue or fix, and the options are genuinely
balanced.

- **Allowed** — a List in a condition position is code, full stop; that
  is what a concatenative language should mean by it, and the truthiness
  model governs values, not code positions. Cost: `design/TRUTHINESS.0.md`
  must say the model has one shape-shaped hole, and the `if xs` case
  stays a trap for anyone holding a list in a variable.
- **Fix by distinguishing the spellings** — a LITERAL list condition
  stays code (unchanged), while a condition that arrives as an already-
  evaluated VALUE coerces. This is the reading that makes `if xs` agree
  with both constructors, and it is what a user who wrote `def xs []`
  almost certainly meant. Cost: the two spellings stop being
  interchangeable, and the distinction has to survive the compiled path
  as well as the interpreter.
- **Fix by widening the error** — keep the code-body reading but make
  the empty case a diagnostic that names the ambiguity rather than the
  bare "condition produced no value".

This record does NOT block NUR053, which is resolved: the constructor
pair now shares a domain exactly, and this is the residue that pairing
them revealed.
