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

> **A newly-encountered non-uniformity is a PR blocker.** When a
> non-uniformity surfaces — in code review, in a design note, or during
> coding and debugging — it is recorded here immediately with status
> **Pending**, and the PR that surfaced it must not merge until the
> entry is either **Resolved** (the divergence is removed) or marked
> **Allowed** (an explicit, argued acceptance). Unlike ADRs, *recording*
> is mandatory on discovery, not on maintainer instruction; what
> requires the maintainer is the **Allowed** verdict — the same reviewed
> discipline as a `//covergate:allow` entry
> (`design/COVERAGE-ALLOWLIST.10.md`).

**Statuses:**

- **Pending** — recorded but not yet argued to a verdict. Blocks the PR
  that surfaced it. Every Pending record must also appear in the
  pending list below.
- **Allowed** — a deliberate divergence, kept. The record states the
  uniform rule, the divergence, the rationale, and the evidence that
  pins it (docs and tests), so the acceptance cannot silently rot.
- **Resolved** — the divergence was removed. The record is **deleted**
  and its number retired (see above), so a record only ever appears in
  this file as Pending or Allowed; `git log -S NURnnn` recovers a
  retired number's history.

---

## Pending non-uniformities (the blocking list)

The live list of records whose status is **Pending**. A PR that
surfaced (or contains) one of these must not merge while it is listed
here. An entry leaves this list only by becoming **Resolved** or
**Allowed** in its record below — keep the two in sync in the same
commit.

| # | Title | Surfaced by |
|---|-------|-------------|
| [NUR009](#nur009) | Bytes excluded from the DepScalar refinement bases | 2026-07-22 uniformity review |
| [NUR013](#nur013) | NaN: total-order slot in cmp/sort, IEEE-unordered in lt/gt | 2026-07-22 uniformity review |
| [NUR022](#nur022) | `del` covers a fraction of `set`'s containers | 2026-07-22 uniformity review |
| [NUR023](#nur023) | Stack-only registrations outside ADR-004's closed list | 2026-07-22 uniformity review |
| [NUR026](#nur026) | Escape sets diverge between quoted strings and templates | 2026-07-22 uniformity review |
| [NUR030](#nur030) | `group` co-groups deq-distinct keys that render identically | re-opened 2026-07-31 (was Allowed 2026-07-24) |
| [NUR031](#nur031) | Module/Function values are not `eq`/`deq` to themselves | re-opened in part 2026-07-31 (was Allowed 2026-07-24); namespace half resolved by the NUR038 facet refactor |
| [NUR037](#nur037) | A fn-local fn used as a higher-order body word breaks in compiled mode only | re-opened 2026-07-31 (was Allowed 2026-07-30) |
| [NUR049](#nur049) | The paren barrier is one-directional: a group can reach backward for a receiver | 2026-07-31 split of NUR029 (G10) |

Pending records use a compact form (rule / divergence / evidence /
documentation status, plus a proposed verdict where one is obvious);
they are expanded to the full argued form when the maintainer issues
the verdict.

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
truthiness rule — only `false`, `0`/`0.0`, `none`, `""`, `[]`, `{}` are
false; a String's characters are never inspected.

### Why allowed

`convert Boolean` shares **one** coercion rule with `if`-condition
truthiness and `make Boolean` (presence, not content); making the
conversion path parse content would fork truthiness into two rules —
a worse non-uniformity than the one it fixes. Content parsing exists as
an explicit opt-in: `convert Boolean {truthy: true}` parses the YAML
tokens (`yes`/`no`/`true`/`false`/`on`/`off`, case-insensitive) and
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

## NUR004 — Boolean and Atom have no lattice subtypes {#nur004}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

The scalar branch families carry structural leaves: `String` has
`EmptyString`/`ProperString`, `Number` has `Integer`/`Float`/
`BigInteger`/`BigDecimal`, `Micron` has its twelve kinds.

### The divergence

`Scalar/Boolean` and `Scalar/Atom` are leaf-less — direct children of
`Scalar` with no builtin subtypes (no `True`/`False` lattice nodes).

### Why allowed

Vacuous rather than divergent: no kernel mechanism requires a scalar
family to have leaves, and nothing dispatches on their presence. There
is no useful structural split of Boolean — `True`/`False` subtypes would
duplicate what value-level machinery already provides uniformly (`case`
literal coverage per NUR002, and DepScalar refinements: `(Boolean gte
true)` *is* the true-only subset, since Boolean is a supported
refinement base like every other well-known scalar). Users who want a
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
  supported DepScalar bases; `lang/spec/compare.tsv` and
  `lang/spec/case.tsv` exercise the value-level machinery that stands in
  for subtypes.

---

## NUR005 — String `add` is the sole cross-type exception to same-type arithmetic {#nur005}

**Status:** Allowed · **Date:** 2026-07-31 (recorded Pending
2026-07-22; verdict and rewritten wording: maintainer, via
`design/NUR-RESOLUTION-PLAN.0.md`)

### The uniform rule

Scalar arithmetic is **same-type arithmetic**: the six words are
"applied within a type, never across it (a cross-type pair is a
`[boru/type_error]`)" — REFERENCE.md §"Within-type operations".

### The divergence

`add` carries `[String Scalar]` / `[Scalar String]` overloads that
stringify the non-String operand (`add "x" 5` → `'5x'`), while Atom
`add` is `[Atom Atom]`-only and Bytes `add` is `[Bytes Bytes]`-only.

### Why allowed

**String `add` is the sole language-level exception to same-type
arithmetic, and it is deliberate.** Concatenation-with-coercion is the
overwhelmingly common string operation, the coercion is total and
canonical (every Scalar has one string render), and the overloads
require **at least one** String operand, so two non-String scalars
still refuse (`add true 1` is a type error — "string-or-bust" is
expressed directly in the signature set). The Pending record's framing
— that Atom and Bytes "do not mirror it" — treated the trio as an
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
value.** Two equality levels are deliberate — reference identity
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
  is structural — by design**"); EXPLANATION.md §"Two equalities, one
  rule" (added with this verdict); `design/LISP-ANALYSIS.5.md` (the
  original argument).
- `lang/spec/module-array.tsv` — the collection words' `deq`-basis
  battery pins the value side of the rule.

**Modification recorded (maintainer, 2026-07-31,
`design/NUR-RESOLUTION-PLAN.0.md`):** the two-level model is to grow
into a complete equality family with a third word — **`req`**,
reference equality (pointer identity only, uniformly for compounds
and scalars) — separating three notions many languages conflate:
convenience equality (`eq`), deep structural equality (`deq`), and
reference identity (`req`). Performance note: Bytes `deq` may be
O(n); `req` gives a constant-time identity probe. Documentation
should compare the model with JavaScript, Python, Ruby, and the
Lisp family. The `req` design travels with the equality work
re-opened under NUR031; this record's allowance is unchanged.

---

## NUR013 — NaN: total-order slot in cmp/sort, IEEE-unordered in lt/gt {#nur013}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** one ordering answer per value pair within one word family.
**Divergence:** `cmp`/`tcmp`/`sort` give NaN a defined slot (sorts
greatest; two NaNs tie) while `lt`/`lte`/`gt`/`gte` apply the IEEE
unordered rule (always false); also `nan eq nan` is false while
`nan cmp nan` is 0.
**Evidence:** `compare_scalar_behaviors.go:152-171`;
`compare.go:590-624`; `compare_nan_test.go`; `lang/spec/float-special.tsv`.
**Documentation status:** extensively argued
(design/TYPE-ORDERING.10.md §"NaN in the total order",
design/IEEE-754-COMPLIANCE.8.md Tier 0). **Proposed verdict:** allow —
IEEE-754 compliance for the relationals plus a lawful total order for
sort is the standard resolution of an unsatisfiable constraint set.

**Verdict (maintainer, 2026-07-31 — investigation,
`design/NUR-RESOLUTION-PLAN.0.md`):** before the allow is issued,
compare boru's total-order behaviour against **IEEE-754 `totalOrder`**
(§5.10): where the current `cmp`/`tcmp`/`sort` slotting of NaN (and of
signed zeros, if applicable) differs from `totalOrder`, conform where
practical. If conformance is impractical, the divergence from
`totalOrder` itself becomes part of this record's argued acceptance.
Stays Pending until the comparison is run and recorded.

---

## NUR014 — Cross-leaf numeric magnitude equality is leaf-pair-dependent {#nur014}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22; verdict: maintainer, accepting the recommendation in
`design/NUR-EFFORT-TRIAGE.0.md`)

### The uniform rule

Leaves of the same family compare by magnitude: `1 cmp 1.0` → 0,
`1 eq 1.0` → true.

### The divergence

The collapse holds for Integer↔Float and Integer↔Big but NOT
Float↔BigDecimal: `0.1 eq 0d0.1` → false (an exact big.Rat compare of
the float's true binary value against the exact decimal). Magnitude
equality is thus a per-pair property, not a family invariant.

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

- `eng/go/core_make.go` — `isTypeLike` (the deliberate omission) and
  the covered `unsupported target type` arm.
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
- REFERENCE.md string table — the row now carries the "core sequence
  word, also slices List and Bytes" parenthetical (fixed with this
  verdict).
- `lang/go/native/help/help_categories.go` — the string category's
  description now points at core `slice` (fixed with this verdict).
- `lang/spec/edge-scalars-3.tsv:45-53`, `corpus-core.tsv:119`,
  `corpus-structures.tsv:14` — both string and list behaviour pinned;
  NUR039 independently pins the negative-start semantics.

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

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** paired reader/writer words cover the same containers.
**Divergence:** `set` dispatches over Store, Map, List, Class, FlexMap,
FlexList, FlexXml (and errors on Micron); `del` covers Map and FlexMap
only. The List exclusion is documented (pointing at pop/shift/
remove-at); the Store/Class/FlexList/FlexXml absences are not.
**Evidence:** `native_storage.go:24-149` vs `:151-194`.
**Documentation status:** partially documented.

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

---

## NUR023 — Stack-only registrations outside ADR-004's closed list {#nur023}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** ADR-004 — every word ships forward-eligible
(`BarrierPos: -1`); the only stack-only words are the traditional
Forth vocabulary, pinned as a closed list in REFERENCE; a new
stack-only word "needs the same justification weight as a new
init-time panic".
**Divergence:** `apply`'s `[Function]` signature is `BarrierPos: 0`
(stack-only) with a code-comment-only rationale — it is not in the
pinned list. Secondarily, 0-arg words split between `BarrierPos: 0`
(`now`, `math-pi`, the clock words, `break`/`continue`, `gensym`) and
`-1` (`stdin`/`stdout`/`stderr`) against the stated "MUST use -1" rule
— inert at zero args, but a literal deviation.
**Evidence:** `native_ref.go:50-67`; REFERENCE.md:946-962 (the pinned
list); `time_async_module.go:26`; `modules/math.go:339,350`;
`io_module.go:87`.
**Documentation status:** code comments only; the ADR's closed list
does not contain the exception.

**Verdict (maintainer, 2026-07-31 — resolve by ADR refinement,
`design/NUR-RESOLUTION-PLAN.0.md`):** ADR-004 is **incomplete**, and
the divergences recorded here are symptoms of the gap. The ADR should
be refined — on explicit maintainer instruction, per the ADR-addition
rule — to describe: **barrier positions** (`BarrierPos` and what each
value means), the **argument-handling categories** a word can occupy
(forward-eligible, mixed-barrier, stack-only, quoting slots), the
**stack-only behaviour** and its closed list (including `apply`'s
`[Function]` case or its removal), and the **chaining rationale**
(why forward collection composes the way it does). Diagnostics should
then *explain* why a word occupies its category rather than merely
reporting a failed dispatch. Recorded as ADR candidate 4 in the
resolution plan; this record stays Pending until the refined ADR
either absorbs the exceptions into the documented rule or the
registrations are changed to conform.

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

- REFERENCE.md:1156-1179 — both regimes documented with the rationale
  ("different types are simply not equal; only the ordering words
  restrict").
- `eng/go/compare.go` (family restriction raising `incomparable`);
  `eng/go/compare_types.go` (tcmp's Rank-based total order);
  `lang/spec/compare.tsv` and `lang/spec/compare-restrict.tsv` — the
  positive/negative batteries pinning both regimes.

---

## NUR026 — Escape sets diverge between quoted strings and templates {#nur026}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** one escape vocabulary across string literal forms.
**Divergence:** quoted strings (`"…"`/`'…'`) accept jsonic's full
escape set (`\x41`, `A`, `\b`, `\f`, …); backtick templates
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

---

## NUR030 — `group` co-groups deq-distinct keys that render identically {#nur030}

**Status:** Pending · **Re-opened:** 2026-07-31 (maintainer review,
`design/NUR-RESOLUTION-PLAN.0.md`; was Allowed 2026-07-24 — the
allowance's reasoning is retained below as data, not as a verdict)

### The uniform rule

The collection words operate on `deq` classes (NUR011 / NUR015): one
group per value, membership by deep value equality.

### The mechanism (clarified 2026-07-31)

`group` is **not higher-order** — it takes no function. Two forms:

- **1-arg** `group [list]`: each element becomes a Map key; collected
  under it is the list of **indices** where that element occurs —
  `group [1 2 3]` → `{1:[0] 2:[1] 3:[2]}`; `group [1 1.0 2]` →
  `{1:[0 1] 2:[2]}` (1 and 1.0 are one `deq` class). The element is
  the key and the index is the bucketed payload, not the other way
  round.
- **2-arg** `group [keys] [values]`: bucket each value under its
  parallel key — `group ['a' 'b' 'a'] [1 2 3]` → `{'a':[1 3] 'b':[2]}`.

### The divergence

`group` returns a Map, and **a Map key is a rendered string**. Two
keys that are `deq`-distinct but render identically therefore share
one Map entry: `group [Integer Integer/q]` (a type literal and a
same-named atom, `deq`-unequal) yields the single group
`{Integer:[0 1]}`.

### The 2026-07-24 allowance (superseded as a verdict)

The fold is forced by `group`'s Map return shape and is arguably
benign: no index is lost — both occurrences are retained under the
shared key — and the same fold is what makes `group` total over the
common **non-reflexive** keys (`nan` is `deq`-unequal to itself; bare
container literals too, `List deq List` → false), giving
`group [nan nan]` → `{nan:[0 1]}` where raising on a render collision
would make grouping NaN-bearing data a hard error. The lossless
`[[rep group] …]` pair shape was rejected as breaking `group`'s Map
shape and every caller.

### Why re-opened

The allowance treats the render fold as forced; the review pushes one
level down: the **root cause is that Map keys are rendered strings**
at all — whatever you group by is flattened to its text render, and
`group` is one symptom of that language-wide fact.

- **Maintainer proposal to explore:** restrict the grouping-key list
  to **Strings**. A String key IS its render — no lossy step, and
  distinct keys can never collide; this divergence could not arise.
- **Costs identified (why it is not a slam-dunk):** the 1-arg form
  loses generality (`group [1 2 3]` works directly today; String-only
  keys force a conversion first), and NaN totality changes character —
  `nan` could not be a key at all, so the non-reflexive-key problem is
  *forbidden* rather than *folded*.
- **Alternatives on the table:** (a) status quo — any value as key,
  lossy render fold, "benign"; (b) String-only keys — no collisions,
  simpler model, ergonomic cost for non-String data; (c) grouped
  pairs `[[rep group] …]` — lossless, breaks the Map shape (rejected
  by the 2026-07-24 record).
- **Deeper question flagged:** whether Map-keys-as-rendered-strings
  is the real thing to reconsider, language-wide.

Next step: a design decision between (a)/(b)/(c) — possibly folded
into a broader Map-key-identity review. Unresolved until then.

### Evidence

- `lang/go/native/native_array.go` — `deqGrouper.add` (the render-key
  fold, commented).
- `lang/spec/module-array.tsv` §3 — `group [Integer Integer/q]` →
  `{Integer:[0 1]}` and `group [nan nan] [1 2]` → `{nan:[1 2]}` pin
  both the collision fold and the non-reflexive fold.
- REFERENCE.md — Map keys as rendered strings (the root-cause fact).

---

## NUR031 — Code/opaque values have no value equality {#nur031}

**Status:** Pending (re-opened **in part**) · **Re-opened:** 2026-07-31
(maintainer review, `design/NUR-RESOLUTION-PLAN.0.md`; was Allowed
2026-07-24 — the resolved handle equalities below stand; the
re-opened part is the Module/Function/Word remainder)

### The uniform rule

NUR011: for Nodes and Ideals, `eq` is reference identity and `deq` is
deep value equality. Every value should at least be equal to itself.

### The divergence (as recorded)

When surfaced (PR #309 review), the rule held only for the structural
families (lists, maps, XML, class/resource instances). Every other
Ideal — Store, Error, Timer, Interval, Function, Module — fell through
both `ExactEqual` and `DeepEqual` to `false`: not `eq` to itself, not
`deq` to itself.

### What was resolved

The **stateful and value-like handles now follow the rule** (this PR):

- **Store** — `eq` is reference identity (the `*StoreInstanceInfo`
  pointer: a store IS its handle), `deq` is its deep entry value (the
  same own-entry projection as `convert Map`, recursed).
- **Error** — a value-like Ideal (an immutable `ErrorInfo`, no
  reference), so `eq` and `deq` both compare its fields (code, message,
  payload map), coinciding like a scalar leaf.
- **Timeout / Interval** — opaque handles whose identity IS their value,
  so `eq` and `deq` are both pointer identity.

Implemented in `eng/go/compare.go` (`opaqueIdealExactEqual` /
`opaqueIdealDeepEqual` / `storeDeepEqual` / `errorInfoEqual`), mirrored
in `eng/go/compare_deqkey.go` (`isDeqComparableHandle` → `DeqUnkeyed`,
scanned pairwise). Verified in `eng/go/compare_nur031_test.go`,
`lang/spec/compare-restrict.tsv`, and `lang/spec/edge-containers-1.tsv`
§8 (whose rows deliberately pinning "Stores have NO identity" were
rewritten — this PR overturns that earlier design decision, exactly
what the NUR process is for).

### The remainder, reviewed (2026-07-31) — the re-opened part

The **code / opaque values** — `Function`/`FnDef`, `Module`/
`ModuleExport`, `Word` — kept the equal-to-nothing behaviour, which
the 2026-07-24 record accepted wholesale. The review splits that
acceptance:

- **Accepted as current behaviour** (correction, 2026-07-31 audit):
  the "rejected at dispatch" claim holds only for BARE operands
  (which auto-invoke before comparison). `eq`/`deq`'s signatures are
  `[Any Any]` and DO admit fn values arriving as **container data**
  (`m.run eq m.run`): `deq` is parent-insensitive and never-equal
  (`DeqNeverEqual`), while `ExactEqual`'s type-body arm requires
  Parent equality — so today two identical-canon fn values compare
  `eq`-false when their Parents differed (`Word/__FN` vs
  `Type/Function`) — a case RETIRED by the ADR-011 collapse (one
  Function type; NUR050 resolved). The reflexivity gap this record
  tracks still covers functions-as-container-data: canon/render of an
  fn value keys on the BINDING NAME it was reached through (`def a
  (f/r)` canons `fn a[…]`), so two references to the same function
  are neither `eq` nor stably ordered (`(f/r) tcmp (f/r)` → -1 via
  the per-wrap tie-break — pre-existing, not a collapse regression).
  Function identity — what makes two references "the same function"
  — is exactly this record's open design work. Tolerable while the
  deeper question below is open.
- **Re-classified as an open defect (NOT a benign allowance):**
  `Module`/`ModuleExport` values DO reach `eq`/`deq` and return
  `false` **including against themselves** — a silent violation of
  reflexive equality, the same half-handled-value-kind pattern as
  NUR050 (and the since-resolved NUR051). A wrong answer, delivered
  quietly.

  **Namespace half RESOLVED by construction (2026-07-31):** the
  NUR038 facet refactor retired the `Ideal/ModuleExport` wrapper —
  `import` now binds a plain export Map (module-namespace facet), so
  a namespace takes the ordinary Node equality arms: `M eq M → true`
  (shared `*OrderedMap` identity), `M deq M → true`, and two
  content-equal namespaces of DIFFERENT exports compare `eq → false`
  / `deq → true`, exactly the Map contract. The remaining module
  defect is the `Ideal/Module` DESCRIPTOR only: `M.$module eq
  M.$module → false` (still `ExtensionPayload`-backed, still falls
  to the terminal arm) — in scope for the Behavior-routing design
  below alongside Function/Word identity.

**Standing requirement (maintainer, 2026-07-31):** every value —
functions and modules included — must eventually fall under equality,
at minimum reflexively (a value is `eq`/`deq` to itself). The
function-type-vs-value question is now settled (ADR-011: one
`Function` type; NUR050 resolved), so the remaining mechanism is
function IDENTITY (stable canon independent of the binding name) plus
the Behavior routing below. The likely shape — routing `eq`/`deq` through the type's
`Behavior` for Ideals rather than the kernel's hardcoded arms (the
future ADR the 2026-07-24 record deferred to) — is plausibly the same
architectural change NUR050's resolution needs; track them together.
Note the Sealed Payload constraint stands: module handles are backed
by `ExtensionPayload`, which the kernel deliberately does not inspect
(eng/go/CLAUDE.md "Sealed Payload") — reference identity does not
require inspecting it.

### Evidence

- `eng/go/compare.go` — resolved handle arms + the terminal `false` the
  code/opaque values still reach.
- `lang/go/native/native_module_types.go` — `ExtensionPayload`-backed
  module handles.
- `lang/spec/compare-restrict.tsv`, `lang/spec/edge-containers-1.tsv`
  §8, `lang/spec/edge-containers-2.tsv`, `lang/spec/edge-errors-2.tsv`.


---


## NUR037 — A fn-local fn used as a higher-order body word breaks in compiled mode only {#nur037}

**Status:** Pending · **Re-opened:** 2026-07-31 (maintainer review,
`design/NUR-RESOLUTION-PLAN.0.md`; was Allowed 2026-07-30) ·
**Surfaced by:** C3 `boru:cli` scouting (design/CLI-PROGRAMS.0.md §8)

**Rule:** the two execution engines agree. A program's meaning does not
depend on whether the bytecode compiler accepted it — `design/COMPILABLE-
SUBSET.md` states the contract as "slow, not wrong", and the whole-corpus
differential exists to hold the two engines to the same answers.

**Divergence:** a fn declared INSIDE another fn's body and then named as a
higher-order body word resolves under the interpreter and is UNDEFINED
under the compiler:

```
def collect fn [[xs:List] [Any] [
  def acc (flex {})
  def step fn [[e:String] [Any] [ acc set (e) true ]]
  for-each [step] xs
  acc
]]
print (collect ["x" "y"])
```

- `boru check` → `0 error(s), 0 warning(s), 0 info`
- `boru run -no-compile` → `{x:true y:true}`
- `boru run` (the DEFAULT) → `error: for-each: element 0: [boru/undefined_word]:
  undefined word: step`, caret on `[step]`

Hoisting the same `def step fn` to module scope makes all three agree.

**Evidence:** the three commands above, on the current binary. The shape is
not exotic: a helper local to the function that uses it is the obvious way
to write a callback, and `sift.boru`'s inline comment about "a fold body will
not compile" is this defect seen from a different angle (its stated form —
that `fold` bodies as such refuse — does NOT reproduce; module-level body
fns compile fine).

**Documentation status:** undocumented. `design/BORU-SHARP-EDGES.0.md` does
not list it, and nothing warns that the default mode has a smaller name
resolution scope than the interpreter.

**Proposed verdict:** fix. A compiled fn unit must capture the enclosing
frame's local fn bindings, or the compiler must refuse the shape (a refusal
is merely slow; an `undefined_word` on a working program is wrong). The
check pass reporting clean while the default runtime fails is the part that
makes this a trap rather than a limitation.



### Why re-opened (2026-07-31) — an open defect, not a benign allowance

The 2026-07-30 allowance leaned on the trigger being narrow and the
workaround mechanical (declare callbacks at module scope — written
into `utils/README.md`'s house rules and exercised through the
compiled path by the Go suite). That mitigation **stands**, but it
documents *around* the defect rather than closing it, and the review
re-classifies the record:

- **The mechanism is a closure-capture gap** — the compiled fn unit
  does not capture the enclosing frame's local fn bindings, so `step`
  is simply out of scope in compiled mode. A scope / name-resolution
  defect: distinct from NUR050's (since-resolved) type-identity
  mismatch.
- **But the same family.** It rhymes with the recurring theme — the
  bytecode compiler failing to treat functions/values as first-class
  where the interpreter does: NUR051 (type literals as data refused to
  compile — since RESOLVED: the emitter interns them per ADR-010),
  NUR050 (function references failed dispatch — since RESOLVED:
  stage-1 collection admission + the ADR-011 collapse), and here, function
  *bindings* as captured values failing name resolution when compiled.
- **"Slow, not wrong" is genuinely violated.** `boru check` reports
  clean while the DEFAULT runtime fails with `undefined_word` on a
  working program — exactly the failure mode
  `design/COMPILABLE-SUBSET.md` forbids.

**Fix direction:** a compiled fn unit must capture the enclosing
frame's local fn bindings (proper closure capture — preferred), OR the
compiler must refuse the shape at check time (a refusal is merely
slow; an `undefined_word` on a working program is wrong). Track
alongside the (now-resolved) NUR050/NUR051 first-class-values work even
though the concrete mechanism here is closure capture, not type
identity. The house rule remains the mitigation until the fix lands.
---


## NUR039 — `slice` with a negative start silently ignores its end argument {#nur039}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** C3 `boru:cli`
scouting

**Rule:** an argument is honoured or refused, never ignored. Out-of-domain
indices elsewhere in the String family clamp predictably (`slice 5 6 "abc"`
→ `''`, `slice 0 5 "-"` → `'-'`).

**Divergence:** a NEGATIVE start silently collapses `slice start end s` to
the two-argument "drop N from the end" form, discarding `end` entirely:

```
slice -3 -1 'abcde'   →  ab
slice -3  2 'abcde'   →  ab
slice -3  5 'abcde'   →  ab
slice  1  3 'abcde'   →  bc     (the positive form honours end)
```

**Evidence:** the four calls above. A computed index that underflows —
`slice (ep add 1) (size tok) tok` where `ep` came back `-1` from a failed
`indexof` — therefore returns a plausible wrong substring instead of an
error or an empty string.

**Documentation status:** the negative-index convention is documented as
"count from the end"; that an end argument is then dropped is not.

**Proposed verdict:** fix — honour `end` for a negative start, or refuse the
combination. Relatedly, NUR019 already records `slice` as the String
family's core straggler; this is a second, independent defect in the same
word.



### Why allowed

The affected spelling is a negative start, which every caller in this
repository can avoid by clamping — and clamping is what a caller wants
anyway, since a negative index is a bug at the call site more often than an
intent to count from the end.

**Evidence that pins it:** `utils/cut.boru`'s `cut-chars-rng` clamps the start
explicitly and says it does so BECAUSE of this record, rather than relying on
`slice` to do the right thing; `utils/tests/cut_test.boru` pins the clamped
behaviour at both ends.
---


## NUR040 — `set` quotes a bare computed key where `get` refuses it {#nur040}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** C3 `boru:cli`
scouting

**Rule:** sibling accessors treat their key argument the same way, and a
program that means a variable's VALUE does not silently get its NAME.
lang/go/CLAUDE.md states the split it intends: `dot`/`dotr` quote a bare
word as a literal field name, `get`/`getr` evaluate it.

**Divergence:** `set` carries the quoting `Atom/q` slot that `get` does not,
so the same bare-word spelling means opposite things and only one of them
complains:

```
def k "aa"   {} set k 1     →  {k:1}      # the NAME was stored
def k "aa"   {} set (k) 1   →  {aa:1}     # the VALUE
def k "aa"   {aa:1} get k   →  1          # get EVALUATES k
```

`boru check` reports nothing for the first line.

**Evidence:** the three calls above. The failure mode in real code is a map
built entirely under one literal key: every iteration of a loop overwrites
`{k:…}`, and nothing anywhere reports it.

**Documentation status:** the `dot` vs `get` split is documented in
lang/go/CLAUDE.md; `set`'s membership in the quoting half is not called out,
and `get`-evaluates / `set`-quotes is exactly the asymmetry a reader would
not predict.

**Proposed verdict:** argue and document, or diagnose. The quoting slot has
a real purpose (`set name value store` reads well), so the uniform fix is
probably a check-mode advisory when a bare word passed to a quoting slot is
ALSO a live binding — the one case where the two readings differ and the
author almost certainly meant the value.



### Why allowed

The asymmetry leaks from a distinction that is deliberate and load-bearing
elsewhere — `dot`/`dotr` quote a bare key, `get`/`getr` evaluate one
(lang/go/CLAUDE.md, "dot / dotr vs get / getr"). Making `set` match `get` is a
behavioural change to a core word, which is a larger and riskier edit than the
confusion it removes.

**Evidence that pins it:** every `set` call in `utils/` and in
`lang/go/modules/cli.boru` spells the key explicitly as `(quote k)` rather than
relying on either behaviour, so nothing in the repo depends on which way the
ambiguity resolves.
---


## NUR041 — The `read-only` profile denies file READS {#nur041}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** C3 baked-perms
scouting (design/CLI-PROGRAMS.1.md §1)

**Rule:** a profile's name and its documented intent describe what it
permits. `sandbox.jsonic`'s own comment says "importing the module is
allowed so disk.read works, but the actual disk.read / disk.write capability
is still gated by the global scope above".

**Divergence:** `read-only` allows the `disk.read` GLOBAL but inherits
`fileops.words { default: "deny" }` from `sandbox`, and the scope check
denies first, so reading a file is refused under a profile whose name
promises exactly that:

```
$ boru policy explain read-only fileops.read path=ro.txt
decision: DENY   blame: fileops.words default=deny
$ boru run -perms read-only -e 'import "boru:io" print (IO.read (make Pathon "ro.txt") {fmt:"text"})'
error: [boru/read_error]: read: permission denied: fileops.read
       (policy "read-only": fileops.words default=deny …)
```

`-allow fileops.read` is required to make a read-only profile read.

**Evidence:** the two commands above. Symmetrically, the write half needs
BOTH `-allow-global disk.write` and `-allow fileops.write`; the global cap
and the scope rule are independent gates and either can deny alone.

**Documentation status:** actively misleading — the profile name, and
sandbox.jsonic's intent comment, both say the opposite of the behaviour.

**Proposed verdict:** fix the profile (allow `fileops.read` in `read-only`,
which is what the name means) or rename it, and correct the sandbox comment
either way. A profile nobody can read a file under is not the read-only
profile a tool author reaches for.



### Why allowed

Accepted with a caveat that belongs in the record rather than only in a commit
message: the profile's NAME is what misleads. "read-only" reads as "reads are
fine, writes are not", and it denies both. Nothing about the enforcement is
wrong — the profile simply does not grant what its name implies.

**Evidence that pins it:** `cmd/go/internal/build/utils_e2e_test.go` builds the
baked-permissions pair against `-perms read-only` and records the behaviour at
the call site, and `utils/tee.boru`'s header states it too, so the next author
to reach for the profile meets the caveat before the surprise.
---


## NUR042 — `-policy-dry-run` is documented and does nothing {#nur042}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** C3 baked-perms
scouting

**Rule:** a flag the CLI advertises does what it says, or does not exist.

**Divergence:** `-policy-dry-run` is advertised on `boru run` and `boru build`
as "observe-only: log what the policy would do but allow every call". It is
parsed, read at exactly one site (to stop the resolver returning nil), and
never wraps the policy in an observe-only decorator. Nothing is logged and
nothing is allowed:

```
$ boru run -perms read-only -policy-dry-run -e 'import "boru:io"  IO.write (make Pathon "dry.txt") "x" {fmt:"text"}'
error: [boru/write_error]: write: permission denied: fileops.write …
```

**Evidence:** the command above; `grep -rn DryRun --include=*.go cmd/go
lang/go` outside tests returns only the flag's registration and that single
read.

**Documentation status:** documented in the flag's own help text, which is
the whole problem.

**Proposed verdict:** implement the decorator (a policy wrapper that logs
the decision and returns nil) or remove the flag. A security-adjacent flag
that silently does nothing is worse than no flag, because it invites exactly
the "I checked with dry-run first" workflow it cannot support.



### Why allowed

A flag that is documented and inert is a small defect with a specific hazard: a
user may believe they have PREVIEWED a policy when they have previewed nothing.
That hazard is what this record keeps visible until the flag is either
implemented or withdrawn.

**Evidence that pins it:** the record carries the measurement showing the flag
changes nothing, so a future implementation has its acceptance test already
written.
---


## NUR044 — `boru build` skips the static check `boru run` performs {#nur044}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** C3 baked-perms
scouting

**Rule:** the CLI's entry points agree about whether a program is valid.
`boru run` performs a preflight check and refuses to execute a program with a
check error.

**Divergence:** `boru build` performs no check at all, so a program `boru run`
refuses to run builds successfully and ships:

```
$ echo 'nosuchword 1 2' > bad.boru
$ boru build bad.boru -o badbin
wrote badbin              # exit 0
$ ./badbin
error: [boru/undefined_word]: undefined word: nosuchword    # exit 1
$ boru run bad.boru
check: [error] undefined_word: …  →  refuses to run
```

**Evidence:** the session above.

**Documentation status:** `CLI.md` describes the preflight for `run` and
does not say `build` omits it.

**Proposed verdict:** fix — `boru build` should run the same preflight and
refuse by default (with a `-no-check` escape hatch mirroring `run`'s). The
asymmetry is worst exactly where it matters: the artefact that outlives the
session is the one nothing validated.



### Why allowed

`boru build` producing an unchecked binary is a gap in the tool, not in the
language, and it is covered by a build-time convention: check first, then
build.

**Evidence that pins it:** `utils/Makefile`'s `check` target exists precisely
for this and its comment names this record — "the only thing standing between a
typo and a shipped binary". `make -C utils all` runs `check` before anything
else, and the end-to-end Go test builds only sources that suite has checked.
---


## NUR045 — Per-export module gating is dead schema: `sandbox`'s `deny: ["sleep"]` does not deny {#nur045}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** C3 baked-perms
scouting

**Rule:** a policy rule a shipped profile declares is enforced. The policy
engine has one evaluation path and every scope reaches it through
`Policy.Check`.

**Divergence:** the `modules` scope has a second, per-export half —
`modules.scopes."boru:x".words`, keyed by export name — with a full
implementation (`checkModuleCall`, `evaluate.go`) and unit tests. **Nothing
in production ever calls it.** `Check("modules", "call", …)` appears only in
`lang/go/policy/*_test.go`; the sole production `modules` checks are the
import gate and the per-module `Installed()` flag
(`lang/go/modules/modules.go`). So every per-export rule in every shipped
profile is inert:

```
$ time boru do -perms sandbox 'import "boru:time-util"  TimeUtil.sleep 1500'
real 0m1.531s          # it slept; sandbox.jsonic declares deny: ["sleep"]
$ time boru do -perms full 'import "boru:time-util"  TimeUtil.sleep 1500'
real 0m1.532s          # identical
```

**Evidence:** the timing above; `grep -rn 'Check("modules"' --include=*.go`
outside tests returns exactly one line, and its op is `"import"`. The
`deny: ["sleep"]` rule in `sandbox.jsonic` is the only per-export rule any
shipped profile carries, which is why nobody noticed.

**Why it matters:** `sleep` is the DoS vector a hosted evaluator reaches for
`sandbox` to close. A profile that says it denies a word and does not is a
false guarantee, and the false guarantee is in the security layer.

**Documentation status:** `evaluate.go`'s own doc comment describes the
per-export gate as working ("per-export rules live in the per-module
subscope"), and `sandbox.jsonic` reads as though it works.

**Proposed verdict:** fix — call `Check("modules", "call", {module, export})`
at module-export dispatch — or delete the schema and the rule, and say in
`lang/go/policy` that module gating is import-granularity only. The choice
is a real one: the check would run on every module-word call, so its cost
belongs to the maintainer's judgement, not to a bug fix. What must not
survive is a profile declaring a denial it does not perform.


### Why allowed

**This is a false guarantee in the security layer, and the acceptance does not
make it a safe one.** `sandbox.jsonic` declares `deny: ["sleep"]`, the
per-export gate that would enforce it is never called in production, and the
measured behaviour is that a sandboxed program sleeps exactly as long as an
unsandboxed one. A profile that states a denial it does not perform is worse
than one that states nothing, because a reader budgets for it.

What is allowed is the SCHEMA remaining in place while unenforced. Closing it
means either calling the gate on every module-export dispatch — a cost on the
hot path, which is a maintainer's judgement rather than a bug fix — or deleting
the rule and saying that module gating is import-granularity only.

**Evidence that pins it:** the record carries the timing measurement showing
the two profiles behave identically, so whichever direction is chosen has its
acceptance test already written. Until then, no shipped profile should be
described to a user as denying a word.
---

## NUR046 — `boru fmt` is not idempotent: one pass is not a fixed point {#nur046}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** the C3 utils
suite (`utils/`)

**Rule:** a formatter is idempotent. `fmt(fmt(x)) == fmt(x)`, so "formatted"
is a property a file either has or does not, a `make fmt` target converges,
and a formatting check can be a single-pass diff. `make fmt-docs` and
`kg/Makefile`'s restored `fmt` target both rely on this.

**Divergence:** on a `def name fn [[params] [Returns] [body]]` whose header
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

**Evidence:** the repro above. Across `utils/*.boru` the same thing happens to
all six programs (the five `tests/*.boru` suites are already at their fixed
point after one pass, which is why the divergence is easy to miss); program
output is unchanged in every case, and every one still passes `boru check`.

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

**Documentation status:** nothing claims idempotence in so many words, but
`design/…` notes on 0.9 and `kg/README.md` both treat a single `fmt` run as
producing canonical form, and `make fmt-docs` rewrites doc blocks in one
pass.

**Proposed verdict:** fix. The convergence at pass 2 suggests the first pass
measures widths against a pre-wrap layout decision it then invalidates — the
same family as the memoisation 0.9 landed, one layer up. A regression guard
belongs with the fix: format every `.boru` in the repo TWICE and require the
second pass to be a no-op, with at least one deliberately non-canonical
fixture, since the existing corpus cannot detect this.


### Why allowed

Formatting does not change behaviour — all 995 cases in `utils/` pass either
way, verified — so what the non-idempotence costs is a clean tree and readable
sources, not correctness. It converges at the second pass, so a `fmt` target
that ran twice would be stable; the reason not to paper over it that way is
that the intermediate layout runs statements together on one line, which is
most of what a formatter is for.

**Evidence that pins it:** `utils/Makefile` keeps `fmt` OUT of its `all` target
and its comment names this record and explains why — the same posture
`kg/Makefile` held while its own formatter blocker was open — so the tree
cannot silently start churning on every build.
---

## NUR047 — Regex match offsets are BYTE indices; every string word around them is RUNE-indexed {#nur047}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** the C3 utils
suite (`grep --color`)

**Rule:** boru counts strings in RUNES, uniformly. `size "日本語"` is 3,
`slice 1 2 "日本語"` is `本`, `StringUtil.split ""` yields runes, and
`REFERENCE.md` states the rune convention for the string family as a whole.
It is one of the language's cleaner uniformities — a user never has to ask
which unit a string word means.

**Divergence:** the match records `MiniLang.lang_re` returns carry `i` and `e`
in BYTES.

```
$ boru do 'import "boru:minilang"  print (MiniLang.lang_re "c" {} "日本語c")'
{"ok": true, "ms": [{"m": "c", "i": 9, "e": 10, …}], …}

$ boru do 'print (size "日本語c")'
4
```

The match is at rune index 3 of a 4-rune string; the record says 9..10. Every
consumer that does the obvious thing — `slice (m.i) (m.e) line`, the whole
point of returning offsets — is therefore wrong on any line containing a
non-ASCII rune, and RIGHT on every ASCII line, which is the worst possible
failure distribution: it passes every casual test and corrupts real data.

**Evidence:** the two commands above. `utils/grep.boru`'s `--color` highlighter
is the in-repo consumer; it works around this by converting the line to Bytes,
slicing in bytes, and converting back (`convert String (slice i e (convert
Bytes line))`), which is correct but is exactly the kind of thing a uniform
convention exists to make unnecessary.

**Why it matters:** offsets are only useful for indexing back into the
subject, and the one indexing word available (`slice`) uses the other unit.
The two halves of the feature do not compose. A highlighter, a linter, a
syntax-colourer, and an LSP `Diagnostic` range all want exactly this and all
hit it — Phase 5's server would meet it in `textDocument/publishDiagnostics`,
where LSP itself specifies UTF-16 code units, making three units in play.

**Documentation status:** the unit is not stated at all. `boru describe` for
the regex words does not say, and nothing in `REFERENCE.md` marks the match
record as an exception to the rune convention.

**Proposed verdict:** fix by returning rune offsets, since that is what the
rest of the language means by an index — and it is the half users can act on.
If byte offsets must be kept (they are what Go's regexp returns, and
converting costs a scan), then the record should carry BOTH, named so the
unit is impossible to mistake, and the rune convention's exception must be
documented everywhere the match record is.

### Why allowed

The unit mismatch is real and its failure distribution is the worst kind —
correct on every ASCII input, silently corrupting on the first multi-byte one —
but it is confined to consumers that index back into the subject with the
returned offsets, and those consumers can be exact today by slicing in Bytes.

**Evidence that pins it:** `utils/grep.boru`'s `--color` highlighter is the
in-repo consumer; it converts to Bytes, slices, and converts back, and
`utils/tests/grep_test.boru` carries three cases (a match after multi-byte
runes, a multi-byte match, an astral rune) that exist ONLY to fail if that
workaround is removed. All three would pass on ASCII input, which is why they
are written explicitly.

---

## NUR049 — The paren barrier is one-directional: a group can reach backward for a receiver {#nur049}

**Status:** Pending · **Recorded:** 2026-07-31 · **Surfaced by:** split
of NUR029 (design/BORU-SHARP-EDGES.0.md G10; a latent bug in shipped
example code)

**Rule:** a parenthesized expression is a self-contained
sub-expression: the paren barrier holds in **both** directions.
**Divergence:** the barrier is one-directional. A paren group already
seals the *outward* direction (its result is isolated from surrounding
forward collection — `size m.x` ≡ `size (m.x)`), yet a word **inside**
the parens may reach *backward* across the open paren and claim a
value from the enclosing stack. Symptom: inside an `error` handler the
raised error is on the stack, and the idiom
`error [ def why (dot message) … ]` fails with "dot: no receiver" —
the paren opens a fresh collection context with `dot` first inside it
— while the unparenthesized `dot message` works.
**Investigation (2026-07-31):** every one of `dot`'s 17 signatures has
`BarrierPos: 1` (`native_storage.go` accessorGetSignatures) — the
receiver is always in the barrier slot, so `(dot …)` can never
dispatch without a stack receiver. The parser guards the *sugar* form
(a leading `.`/`!.` is a parse-time `danglingDotError`,
`eng/go/parser/parse.go:444,763`) but the bare word `dot` bypasses the
guard, and the checker reasons optimistically across the barrier — it
assumes a receiver will arrive dynamically (the gradual-Any optimism
noted at `eng/go/carrier.go:1019`) — so nothing is reported statically
and the underflow only surfaces at runtime.
**Evidence:** `design/BORU-SHARP-EDGES.0.md` §G10; the broken idiom
ships in `design/examples/apps/todo-tui-client.boru`'s error arms,
which no test exercises.

**Verdict (maintainer, 2026-07-31 — resolve by fix,
`design/NUR-RESOLUTION-PLAN.0.md`):** make the paren barrier
**symmetric**. A parenthesized expression must fully complete using
only what is inside the parens; it may not optimistically wait for, or
reach out to, any value beyond the open paren — grouping into a
self-contained sub-expression is the purpose of parens, so the barrier
must hold in both directions. Consequences: `(dot message)` then fails
**deterministically and statically** — the barrier-slot receiver can
provably never be satisfied from inside an empty group, so the checker
proves the underflow instead of deferring to runtime (subsuming the
optimistic-barrier gap for this case). With the fix: correct the
shipped example (`todo-tui-client.boru`) to the unparenthesized
`dot message` form, and add a test forcing a sync failure so its error
arms actually run.
**Compatibility check before landing:** this is a real semantic
change. Verify it does not break sanctioned point-free patterns that
intentionally open a paren expecting to consume an enclosing stack
value; if such patterns exist and are sanctioned, they need an
explicit alternative (e.g. `$`-receiver forms) before the barrier
closes.
