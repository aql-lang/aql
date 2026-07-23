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
| [NUR011](#nur011) | `eq` is identity for compounds, value for scalars | 2026-07-22 uniformity review |
| [NUR012](#nur012) | Pathon orders segments in reverse lexical order | 2026-07-22 uniformity review |
| [NUR013](#nur013) | NaN: total-order slot in cmp/sort, IEEE-unordered in lt/gt | 2026-07-22 uniformity review |
| [NUR014](#nur014) | Cross-leaf numeric magnitude equality is leaf-pair-dependent | 2026-07-22 uniformity review |
| [NUR015](#nur015) | Collection words dedup/group/member on rendered form — a third equality | 2026-07-22 uniformity review |
| [NUR018](#nur018) | Store and Error are excluded from `make` | 2026-07-22 uniformity review |
| [NUR019](#nur019) | `slice` is the String family's core straggler | 2026-07-22 uniformity review |
| [NUR020](#nur020) | The `print` core exception is asserted, never argued | 2026-07-22 uniformity review |
| [NUR022](#nur022) | `del` covers a fraction of `set`'s containers | 2026-07-22 uniformity review |
| [NUR023](#nur023) | Stack-only registrations outside ADR-004's closed list | 2026-07-22 uniformity review |
| [NUR024](#nur024) | Ordering words are family-restricted; equality is total | 2026-07-22 uniformity review |
| [NUR025](#nur025) | Comment forms: documented `## ##` does not exist; `//` and `/* */` do, undocumented | 2026-07-22 uniformity review |
| [NUR026](#nur026) | Escape sets diverge between quoted strings and templates | 2026-07-22 uniformity review |
| [NUR028](#nur028) | `aql fmt` re-parses template holes as map literals | 2026-07-22 uniformity review |
| [NUR029](#nur029) | Design-note-tracked sibling-form divergences (SHARP-EDGES G8–G13b) | 2026-07-22 uniformity review |

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

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** one word, one equality principle.
**Divergence:** `eq` compares scalars by value but lists/maps/XML by
container identity (`["a"] eq ["a"]` → false); `deq` is structural
throughout. Consequence: `eq` disagrees with `cmp`-equality on
compounds (structurally-equal lists are `cmp`-equal but not `eq`).
**Evidence:** `eng/go/compare.go:326-384`; REFERENCE.md:1174-1181.
**Documentation status:** argued in `design/LISP-ANALYSIS.5.md` (the
Scheme eq?/equal? trichotomy) and accepted in the DX reports; stated in
REFERENCE; absent from EXPLANATION's equality discussion.
**Proposed verdict:** allow (argued design), plus a doc task to surface
the argument in EXPLANATION.

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

## NUR015 — Collection words dedup/group/member on rendered form — a third equality {#nur015}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** the language defines two equality notions (`eq` identity/
value, `deq` structural) and one order (`cmp`/`tcmp`); collection
words should use one of them.
**Divergence:** `ArrayUtil.unique`/`member`/`indices`/`group` key on
the rendered `Value.String()` — a third notion agreeing with none:
`unique [1 1.0]` keeps both (though `1 eq 1.0`, `deq [1] [1.0]`, and
`cmp 1 1.0 = 0` all say equal — verified live) while
`unique [["a"] ["a"]]` dedups lists that `eq` calls distinct
(verified).
**Evidence:** `native_array.go:935,1129,1156,1163,1191,1212`.
**Documentation status:** undocumented — REFERENCE and
design/ARRAYIFICATION.6.md give examples but never state the equality
basis.

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

## NUR028 — `aql fmt` re-parses template holes as map literals {#nur028}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** one parser (ADR-007's spirit) — the formatter must agree with
the parser about what source means.
**Divergence:** the formatter has no InterpString handling, so
`` `hi ${name}` `` round-trips to `` `hi $ {name:name} ` `` — the
`${expr}` hole re-parsed as a map literal, changing program semantics.
The kg pipeline hand-formats its sources and guards `make fmt` into a
no-op because of this.
**Evidence:** `lang/go/formatter/` (no interp path);
kg/README.md:238-245 (the only place it is tracked);
design/go-modules/FMT.10.md is silent.
**Documentation status:** tracked only in a component README.
**Proposed verdict:** resolve by fix (formatter learns InterpString);
until then the register entry is the tracker.

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
