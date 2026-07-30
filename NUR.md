# Non-Uniformity Register (NUR)

A running register of every place where AQL — the language or its
implementation — deviates from one of its own uniform rules. A
non-uniformity is any special case: a type treated differently from its
siblings, one member of a word family with an exception, a path that
bypasses a single-source-of-truth mechanism. Uniformity is a core design
value of AQL (one parser, one argument-positioning convention, one
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
| [NUR005](#nur005) | String `add` crosses scalar types; Atom/Bytes do not mirror it | 2026-07-22 uniformity review |
| [NUR009](#nur009) | Bytes excluded from the DepScalar refinement bases | 2026-07-22 uniformity review |
| [NUR010](#nur010) | Integer `pow` negative-exponent error carries no `[aql/…]` code | 2026-07-22 uniformity review |
| [NUR012](#nur012) | Pathon orders segments in reverse lexical order | 2026-07-22 uniformity review |
| [NUR013](#nur013) | NaN: total-order slot in cmp/sort, IEEE-unordered in lt/gt | 2026-07-22 uniformity review |
| [NUR014](#nur014) | Cross-leaf numeric magnitude equality is leaf-pair-dependent | 2026-07-22 uniformity review |
| [NUR018](#nur018) | Store and Error are excluded from `make` | 2026-07-22 uniformity review |
| [NUR019](#nur019) | `slice` is the String family's core straggler | 2026-07-22 uniformity review |
| [NUR020](#nur020) | The `print` core exception is asserted, never argued | 2026-07-22 uniformity review |
| [NUR022](#nur022) | `del` covers a fraction of `set`'s containers | 2026-07-22 uniformity review |
| [NUR023](#nur023) | Stack-only registrations outside ADR-004's closed list | 2026-07-22 uniformity review |
| [NUR024](#nur024) | Ordering words are family-restricted; equality is total | 2026-07-22 uniformity review |
| [NUR025](#nur025) | Comment forms: documented `## ##` does not exist; `//` and `/* */` do, undocumented | 2026-07-22 uniformity review |
| [NUR026](#nur026) | Escape sets diverge between quoted strings and templates | 2026-07-22 uniformity review |
| [NUR029](#nur029) | Design-note-tracked sibling-form divergences (SHARP-EDGES G8–G13b) | 2026-07-22 uniformity review |
| [NUR037](#nur037) | A fn-local fn used as a higher-order body word breaks in compiled mode only | 2026-07-30 C3 `aql:cli` scouting |
| [NUR038](#nur038) | Two consecutive statements headed by a 1-arg Any module export misfire silently | 2026-07-30 C3 `aql:cli` scouting |
| [NUR039](#nur039) | `slice` with a negative start silently ignores its end argument | 2026-07-30 C3 `aql:cli` scouting |
| [NUR040](#nur040) | `set` quotes a bare computed key where `get` refuses it | 2026-07-30 C3 `aql:cli` scouting |
| [NUR041](#nur041) | The `read-only` profile denies file READS | 2026-07-30 C3 perms scouting |
| [NUR042](#nur042) | `-policy-dry-run` is documented and does nothing | 2026-07-30 C3 perms scouting |
| [NUR044](#nur044) | `aql build` skips the static check `aql run` performs | 2026-07-30 C3 perms scouting |
| [NUR045](#nur045) | Per-export module gating is dead schema: `sandbox`'s `deny: ["sleep"]` does not deny | 2026-07-30 C3 perms scouting |

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
`[aql/type_error]: add: arithmetic is not defined on Boolean` — for all
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

---

## NUR002 — Boolean alone is case-exhaustible by value enumeration {#nur002}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

For a scalar scrutinee, a default-less `case` proves exhaustiveness
through type clauses, `[is T]` predicates, or comparison-predicate /
refinement interval unions — never by listing scalar literals
(`case n [1 … 2 …]` can not cover `Integer`).

### The divergence

`true` + `false` cover a `Boolean` scrutinee like enum members: `case b
[true 1 false 0]` is statically exhaustive with no default, and `case b
[true 1]` is a `case_not_exhaustive` check error (`uncovered: false`).
No other builtin scalar gets literal-enumeration coverage.

### Why allowed

The divergence follows from cardinality, not special pleading: Boolean
is the only **finite** builtin scalar, so value enumeration is actually
sound for it — the checker's coverage proof stays in the sound
direction. The mechanism is the same value/type coverage channel enums
use (`def Color (red/q tor …)` covered member by member), so Boolean is
being admitted to an existing uniform mechanism that other scalars
cannot soundly enter, rather than getting a private code path.

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
`all`, and the `aql:logic-util` gates (`nand`/`nor`/`xnor`/`iff`/
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

### Evidence

- `eng/go/typetable.go::builtinDecls` — the Scalar branch layout.
- `eng/go/depscalar.go::canonicalBaseType` — Boolean listed among the
  supported DepScalar bases; `lang/spec/compare.tsv` and
  `lang/spec/case.tsv` exercise the value-level machinery that stands in
  for subtypes.

---

## NUR005 — String `add` crosses scalar types; Atom/Bytes do not mirror it {#nur005}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** arithmetic is "applied within a type, never across it (a
cross-type pair is a `[aql/type_error]`)" — REFERENCE.md §"Within-type
operations".
**Divergence:** `add` carries `[String Scalar]` / `[Scalar String]`
overloads that stringify the non-String operand (`add "x" 5` → `'5x'`,
verified live), while its occurrence-package siblings stay within-type:
Atom `add` is `[Atom Atom]`-only (`add red/q 5` is a signature error,
verified) and Bytes `add` is `[Bytes Bytes]`-only. One member of the
mirrored String/Atom/Bytes trio crosses types; two do not.
**Evidence:** `lang/go/native/native_math.go:311-312,434-436`;
`native_scalar_ops.go:538-547,661-662`; `native_bytes.go:249`.
**Documentation status:** the String concat rule itself is documented
(REFERENCE.md:981-984, with the `add true 1` negative), but it sits 60
lines from the "never across it" rule it contradicts, and the
Atom/Bytes asymmetry is noted only in a code comment.

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

---

## NUR010 — Integer `pow` negative-exponent error carries no `[aql/…]` code {#nur010}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** every arithmetic fault carries a coded AQL error —
div/mod-by-zero raise `[aql/arith_error]`, range faults
`[aql/integer_overflow]`.
**Divergence:** `2 pow -1` fails with the bare `error: pow: negative
exponent -1` (verified live) — a `fmt.Errorf`, no `[aql/…]` code, so it
is invisible to code-based error handling that every sibling fault
supports. (The partiality itself — Integer pow rejecting negative
exponents while Float pow computes `0.5` — is documented in
design/LANGREF.10.md:780.)
**Evidence:** `native_math.go:252-254` and mirror `:161` vs `:216,235`
(coded siblings).
**Documentation status:** partiality documented; the code-less error is
not. **Proposed verdict:** resolve by fix (give it an `arith_error`
code); the partiality itself needs its own verdict.

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

---

## NUR012 — Pathon orders segments in reverse lexical order {#nur012}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** name-like scalars order forward-lexicographically (String,
Atom, Word).
**Divergence:** Pathon orders fewer-segments-first, then segment-by-
segment in REVERSE lexical order (`sort [b/a a/z]` keeps `b/a` first —
verified live), then relative-before-absolute; the same function orders
DepScalar pairs forward-lex, so one comparator houses both directions.
**Evidence:** `eng/go/compare_scalar_behaviors.go:359-413` (reverse at
`:398`, forward DepScalar tail at `:366`).
**Documentation status:** stated as fact (design/TYPE-ORDERING.10.md:74,
REFERENCE.md:728 — which calls it "historical"); the rationale for
reverse is argued nowhere.

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

---

## NUR014 — Cross-leaf numeric magnitude equality is leaf-pair-dependent {#nur014}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** "leaves of the same family compare by magnitude"
(`1 cmp 1.0` → 0, `1 eq 1.0` → true).
**Divergence:** the collapse holds for Integer↔Float and Integer↔Big
but NOT Float↔BigDecimal: `Float 0.1 ≠ 0d0.1` (exact big.Rat compare of
the float's true binary value). Magnitude equality is thus a per-pair
property, not a family invariant.
**Evidence:** `compare_scalar_behaviors.go:111-179,195-226`.
**Documentation status:** deliberate (in-code rationale cites Python's
`Decimal('0.1') == 0.1 → False`); user-facing at REFERENCE.md:197,501.
**Proposed verdict:** allow — mathematically honest; the alternative
(rounding Big to float64) silently collapses distinct values.

---

## NUR018 — Store and Error are excluded from `make` {#nur018}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** TYPE-UNIFORM — `make` instantiates the structural
type-kinds; the kernel guide groups Record, Options, Table, Class,
Store, Error and the Micron family together as the `make`/`record`/
`class` structural set (eng/go/CLAUDE.md §"Where a Type Lives" rule 4).
**Divergence:** `make Store {}` and `make Error {message:"x"}` raise
`[aql/unsupported]: make: unsupported target type` (verified live)
while Record/Options/Table/Class/Micron are `make` targets — Store and
Error construct only through their dedicated words.
**Evidence:** `eng/go/core_make.go:31-37` (`isTypeLike`).
**Documentation status:** likely deliberate but stated nowhere; needs a
one-line verdict.

---

## NUR019 — `slice` is the String family's core straggler {#nur019}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** the string vocabulary moved to `aql:string-util`; moved
words are not available unqualified (lang/go/CLAUDE.md §"Package
layout").
**Divergence:** `slice` alone stays core — REFERENCE's string table
lists it unqualified between two `StringUtil.*` rows — and `aql
describe` files it under `list`, not `string`. The likely reason
(it is polymorphic over String and List, i.e. a sequence word) is
stated nowhere.
**Evidence:** `lang/go/native/natives.go:372-382`; REFERENCE.md:1117;
`help_categories.go:40-49`.
**Documentation status:** undocumented rationale.

---

## NUR020 — The `print` core exception is asserted, never argued {#nur020}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** the IO vocabulary moved to `aql:io`; "only `print` stays in
core".
**Divergence:** the exception is repeated in four places with only a
one-line "so basic output needs no import" rationale; ADR-004 argues
print's *forwardness*, not its placement. One IO word unqualified, the
rest namespaced, with the boundary un-argued.
**Evidence:** `lang/go/CLAUDE.md:82`; `native_print.go:6-7`;
`io_module.go:21`; `register.go:123-127`; ADR-004 §Consequences.
**Documentation status:** deliberate but asserted, not argued.
**Proposed verdict:** allow with the argument written down (or resolve
by moving print — a breaking change).

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

---

## NUR024 — Ordering words are family-restricted; equality is total {#nur024}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** one comparison vocabulary, one totality regime.
**Divergence:** `cmp`/`lt`/`lte`/`gt`/`gte` raise `[aql/incomparable]`
across families (`cmp true 1` errors — verified live) while
`eq`/`neq`/`deq` are total (`1 eq "1"` → false) and `tcmp` is the
unrestricted total order — two totality regimes inside one family,
with `cmp` and `tcmp` answering differently for the same pair.
**Evidence:** REFERENCE.md:1156-1179; `compare.go` (family
restriction), `compare_types.go` (tcmp's Rank order).
**Documentation status:** fully documented with rationale ("different
types are simply not equal; only the ordering words restrict").
**Proposed verdict:** allow — the restriction catches real type errors
where a silent cross-family order would hide them, and `tcmp` provides
the escape.

---

## NUR025 — Comment forms: documented `## ##` does not exist; `//` and `/* */` do, undocumented {#nur025}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** REFERENCE §Comments defines the comment vocabulary: `# text`
(line) and `## text ##` (block).
**Divergence:** `##` does not open a block — `1 ## hi ## add 2` → `1`
(rest of line ignored; the second `##` closes nothing). The actual
block form is `/* */` (`1 /* hi */ add 2` → `3`) and `//` is a second
working line form — both inherited jsonic defaults, absent from
REFERENCE.
**Evidence:** REFERENCE.md:243-248; `parse_error.go:67`
(`unterminated_comment: /*`); `parse_test.go:563`.
**Documentation status:** the documented form is wrong and the working
forms are undocumented. **Proposed verdict:** resolve by decision —
either implement `## ##` and hide the jsonic defaults, or document
what the parser actually accepts.

---

## NUR026 — Escape sets diverge between quoted strings and templates {#nur026}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** one escape vocabulary across string literal forms.
**Divergence:** quoted strings (`"…"`/`'…'`) accept jsonic's full
escape set (`\x41`, `A`, `\b`, `\f`, …); backtick templates
process only `\n \t \r \\ \` \$` — `size "z\x41z"` → 3 while the same
text in a template → 6 (the escape survives literally).
**Evidence:** `parse.go:1604-1637` (`processTemplateEscapes`);
`grammar.go:103,355-367`.
**Documentation status:** REFERENCE documents the restricted template
set but never states quoted strings accept a superset — the asymmetry
is undocumented.

---

## NUR029 — Design-note-tracked sibling-form divergences (SHARP-EDGES G8–G13b) {#nur029}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** the same construct behaves the same across sibling forms
(parked vs named, parenthesized vs bare, matched vs default arm,
returned vs bound, `none` vs `None`, single- vs multi-token).
**Divergence:** `design/AQL-SHARP-EDGES.0.md` documents seven
sibling-form divergences, none previously registered: G8 (recovered
`raise` tears down enclosing params), G9 (a `case` DEFAULT arm
forward-collects the scrutinee stack; matched arms are isolated), G10
(parenthesized `(dot message)` finds no receiver where bare `dot
message` works), G11 (a returned bare list evaluates lazily after
frame teardown; a `def`-bound one snapshots eagerly), G12 (a
`/r`-parked fn does not satisfy a `Function` param, yet the same value
dot-invoked from a map dispatches), G13a (single-token bare-map body
refuses to compile; multi-token compiles), G13b (`{r: None}` refuses
to bytecode-compile while `{r: none}` compiles).
**Evidence:** `design/AQL-SHARP-EDGES.0.md` (minimal repros and triage
table per item; two engine-bug candidates, two compiler limits, three
sharp edges).
**Documentation status:** tracked in the design note with per-item
status; registered here so each resolution or allowance is recorded.
This umbrella entry splits into per-item records if any single item is
allowed rather than fixed.

**Re-verified 2026-07-30** (every item re-run against the binary, during
C3 scouting): **G8, G11 and G13a no longer reproduce** — a recovered
`raise` no longer tears down enclosing params, a returned bare list no
longer evaluates lazily after teardown, and a single-token bare-map body
compiles. They were fixed by unrelated work and this register did not
notice, which is the cost of an umbrella entry. **G9, G10, G12 and G13b
still reproduce exactly as described.** The design note must be corrected
to match, and the umbrella should be split so a per-item fix can retire a
per-item record.

---

## NUR030 — `group` co-groups deq-distinct keys that render identically {#nur030}

**Status:** Allowed · **Date:** 2026-07-24

### The uniform rule

The collection words operate on `deq` classes (NUR011 / NUR015): one
group per value, membership by deep value equality.

### The divergence

`group` returns a Map, and a Map key is a rendered string. Two keys
that are `deq`-distinct but render identically therefore share one Map
entry: `group [Integer Integer/q]` (a type literal and a same-named
atom, `deq`-unequal) yields the single group `{Integer:[0 1]}`.

### Why allowed

The fold is forced by `group`'s Map return shape, and it is **benign**:
no index is lost — both occurrences are retained under the shared key.
Crucially, the same fold is what makes `group` total over the common
**non-reflexive** keys: `nan` is `deq`-unequal to itself, and the bare
container literals are too (`List deq List` → false), so each `nan` (or
each `List`) key fails its own `deq` probe and reaches the render path.
Folding gives `group [nan nan]` → `{nan:[0 1]}`; the rejected
alternative (raising on a render collision) would turn grouping over
NaN-bearing numeric data into a hard error — a far worse divergence
than co-grouping the rare type-literal/atom pair. The lossless
alternative that preserves both classes — returning `[[rep group] …]`
pairs — would break `group`'s established Map shape and every caller.

### Evidence

- `lang/go/native/native_array.go` — `deqGrouper.add` (the render-key
  fold, commented).
- `lang/spec/module-array.tsv` §3 — `group [Integer Integer/q]` →
  `{Integer:[0 1]}` and `group [nan nan] [1 2]` → `{nan:[1 2]}` pin
  both the collision fold and the non-reflexive fold.

---

## NUR031 — Code/opaque values have no value equality {#nur031}

**Status:** Allowed · **Date:** 2026-07-24

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

### Why the remainder is allowed

The **code / opaque values** — `Function`/`FnDef`, `Module`/
`ModuleExport`, `Word` — keep the equal-to-nothing behaviour, and it is
accepted:

- A function's or module's "value" is opaque code and binding state,
  not user data; there is no principled structural equality to give it,
  and no stable reference the kernel may compare — module handles are
  backed by `ExtensionPayload`, the escape hatch the kernel
  **deliberately does not inspect** (eng/go/CLAUDE.md "Sealed Payload").
- `Function`/`FnDef` and `Word` values are in practice **rejected at
  dispatch** — `eq`/`deq` have no signature admitting them, so a
  comparison is a loud `signature_error`, not a silent wrong answer.
- `Module`/`ModuleExport` values do reach `eq`/`deq` and return `false`
  (including self). Giving them reference identity would require routing
  `eq`/`deq` through the type's `Behavior` for Ideals rather than the
  kernel's hardcoded arms — an architectural change deferred to a future
  ADR. Recorded here so the remainder is visible rather than silent.

### Evidence

- `eng/go/compare.go` — resolved handle arms + the terminal `false` the
  code/opaque values still reach.
- `lang/go/native/native_module_types.go` — `ExtensionPayload`-backed
  module handles.
- `lang/spec/compare-restrict.tsv`, `lang/spec/edge-containers-1.tsv`
  §8, `lang/spec/edge-containers-2.tsv`, `lang/spec/edge-errors-2.tsv`.


---


## NUR037 — A fn-local fn used as a higher-order body word breaks in compiled mode only {#nur037}

**Status:** Pending · **Recorded:** 2026-07-30 · **Surfaced by:** C3 `aql:cli`
scouting (design/CLI-PROGRAMS.0.md §8)

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

- `aql check` → `0 error(s), 0 warning(s), 0 info`
- `aql run -no-compile` → `{x:true y:true}`
- `aql run` (the DEFAULT) → `error: for-each: element 0: [aql/undefined_word]:
  undefined word: step`, caret on `[step]`

Hoisting the same `def step fn` to module scope makes all three agree.

**Evidence:** the three commands above, on the current binary. The shape is
not exotic: a helper local to the function that uses it is the obvious way
to write a callback, and `sift.aql`'s inline comment about "a fold body will
not compile" is this defect seen from a different angle (its stated form —
that `fold` bodies as such refuse — does NOT reproduce; module-level body
fns compile fine).

**Documentation status:** undocumented. `design/AQL-SHARP-EDGES.0.md` does
not list it, and nothing warns that the default mode has a smaller name
resolution scope than the interpreter.

**Proposed verdict:** fix. A compiled fn unit must capture the enclosing
frame's local fn bindings, or the compiler must refuse the shape (a refusal
is merely slow; an `undefined_word` on a working program is wrong). The
check pass reporting clean while the default runtime fails is the part that
makes this a trap rather than a limitation.


---


## NUR038 — Two consecutive statements headed by a 1-arg Any module export misfire silently {#nur038}

**Status:** Pending · **Recorded:** 2026-07-30 · **Surfaced by:** C3 `aql:cli`
scouting

**Rule:** a statement boundary separates statements. Two statements in
sequence run in order, each consuming its own arguments.

**Divergence:** when the head of each statement is a module export whose
single parameter is typed `Any`, the second statement's argument is
collected by the FIRST statement's word, the order inverts, and one call is
lost — with no diagnostic:

```
import "aql:io"
IO.printstr "A\n"
IO.printstr "B\n"
```

prints `B`, then `A`, then leaves ` fn printstr(Any)` on the stack (the
residual the driver then prints). `aql check` reports `0 error(s)` and a
residual of `ProperString __FN` — the `__FN` in the residual is the only
trace, and no diagnostic names it. Terminating either statement with `end`
or wrapping it in parens gives the expected `A`, `B`.

**Evidence:** the file above, run with `aql run -no-compile` and `aql run`;
`aql check` on the same source. An `Any` parameter is what makes it happen:
the same shape with a `String`-typed export behaves.

**Documentation status:** undocumented. `design/ERRORS.8.md` §6 covers
chained forward calls and `mixed_form_call` advises on mixed splits, but
neither covers a same-statement-boundary inversion that drops a call.

**Proposed verdict:** fix, or diagnose. This is the silent-wrong-answer
class: the program printed the wrong thing in the wrong order and exited 0.
If the collection rule genuinely requires `end` here, `aql check` must say
so — a `forward_strands_operand`-style advisory at minimum. Until then the
house rule for AQL-authored modules and programs is to terminate every
statement whose head is a module export with `end`.


---


## NUR039 — `slice` with a negative start silently ignores its end argument {#nur039}

**Status:** Pending · **Recorded:** 2026-07-30 · **Surfaced by:** C3 `aql:cli`
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


---


## NUR040 — `set` quotes a bare computed key where `get` refuses it {#nur040}

**Status:** Pending · **Recorded:** 2026-07-30 · **Surfaced by:** C3 `aql:cli`
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

`aql check` reports nothing for the first line.

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


---


## NUR041 — The `read-only` profile denies file READS {#nur041}

**Status:** Pending · **Recorded:** 2026-07-30 · **Surfaced by:** C3 baked-perms
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
$ aql policy explain read-only fileops.read path=ro.txt
decision: DENY   blame: fileops.words default=deny
$ aql run -perms read-only -e 'import "aql:io" print (IO.read (make Pathon "ro.txt") {fmt:"text"})'
error: [aql/read_error]: read: permission denied: fileops.read
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


---


## NUR042 — `-policy-dry-run` is documented and does nothing {#nur042}

**Status:** Pending · **Recorded:** 2026-07-30 · **Surfaced by:** C3 baked-perms
scouting

**Rule:** a flag the CLI advertises does what it says, or does not exist.

**Divergence:** `-policy-dry-run` is advertised on `aql run` and `aql build`
as "observe-only: log what the policy would do but allow every call". It is
parsed, read at exactly one site (to stop the resolver returning nil), and
never wraps the policy in an observe-only decorator. Nothing is logged and
nothing is allowed:

```
$ aql run -perms read-only -policy-dry-run -e 'import "aql:io"  IO.write (make Pathon "dry.txt") "x" {fmt:"text"}'
error: [aql/write_error]: write: permission denied: fileops.write …
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


---


## NUR044 — `aql build` skips the static check `aql run` performs {#nur044}

**Status:** Pending · **Recorded:** 2026-07-30 · **Surfaced by:** C3 baked-perms
scouting

**Rule:** the CLI's entry points agree about whether a program is valid.
`aql run` performs a preflight check and refuses to execute a program with a
check error.

**Divergence:** `aql build` performs no check at all, so a program `aql run`
refuses to run builds successfully and ships:

```
$ echo 'nosuchword 1 2' > bad.aql
$ aql build bad.aql -o badbin
wrote badbin              # exit 0
$ ./badbin
error: [aql/undefined_word]: undefined word: nosuchword    # exit 1
$ aql run bad.aql
check: [error] undefined_word: …  →  refuses to run
```

**Evidence:** the session above.

**Documentation status:** `CLI.md` describes the preflight for `run` and
does not say `build` omits it.

**Proposed verdict:** fix — `aql build` should run the same preflight and
refuse by default (with a `-no-check` escape hatch mirroring `run`'s). The
asymmetry is worst exactly where it matters: the artefact that outlives the
session is the one nothing validated.


---


## NUR045 — Per-export module gating is dead schema: `sandbox`'s `deny: ["sleep"]` does not deny {#nur045}

**Status:** Pending · **Recorded:** 2026-07-30 · **Surfaced by:** C3 baked-perms
scouting

**Rule:** a policy rule a shipped profile declares is enforced. The policy
engine has one evaluation path and every scope reaches it through
`Policy.Check`.

**Divergence:** the `modules` scope has a second, per-export half —
`modules.scopes."aql:x".words`, keyed by export name — with a full
implementation (`checkModuleCall`, `evaluate.go`) and unit tests. **Nothing
in production ever calls it.** `Check("modules", "call", …)` appears only in
`lang/go/policy/*_test.go`; the sole production `modules` checks are the
import gate and the per-module `Installed()` flag
(`lang/go/modules/modules.go`). So every per-export rule in every shipped
profile is inert:

```
$ time aql do -perms sandbox 'import "aql:time-util"  TimeUtil.sleep 1500'
real 0m1.531s          # it slept; sandbox.jsonic declares deny: ["sleep"]
$ time aql do -perms full 'import "aql:time-util"  TimeUtil.sleep 1500'
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
