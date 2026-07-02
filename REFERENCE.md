# AQL Reference

Information-oriented reference for AQL syntax, types, and the
built-in word library. For learning AQL, start with the
**[Tutorial](TUTORIAL.md)**. For task-oriented recipes, see the
**[How-To Guides](HOWTO.md)**. For *why* AQL is shaped the way it
is, see the **[Explanation](EXPLANATION.md)**.

> **Notation.** Throughout, a trailing `# returns …` comment shows what
> an expression evaluates to (`mul 2 3  # returns 6`); in prose we say
> "`mul 2 3` returns `6`". The comment is ordinary documentation (`#`
> begins a line comment), not special syntax. AQL has no result arrow:
> `=>` is itself a word — the anonymous-function arrow, sugar for `afn`
> — so results are written as comments rather than with `=>`.

## Contents

* [Syntax](#syntax)
* [Evaluation model](#evaluation-model)
* [Type system](#type-system)
* [Word reference](#word-reference)
  * [Stack manipulation](#stack-manipulation)
  * [Arithmetic](#arithmetic)
  * [Rounding](#rounding)
  * [Roots, exponentials, logarithms](#roots-exponentials-logarithms)
  * [Trigonometry](#trigonometry)
  * [Constants](#constants)
  * [Strings](#strings)
  * [Boolean](#boolean)
  * [Comparison](#comparison)
  * [Definition and scoping](#definition-and-scoping)
  * [Macros](#macros)
  * [Control flow](#control-flow)
  * [List and array words](#list-and-array-words)
  * [Higher-order array words](#higher-order-array-words)
  * [Maps and access](#maps-and-access)
  * [Flex nodes — FlexMap and FlexList](#flex-nodes--flexmap-and-flexlist)
  * [Type words](#type-words)
  * [Inspection](#inspection)
  * [I/O](#io)
  * [Networking — `fetch`](#networking--fetch)
  * [SQLite](#sqlite)
  * [Modules](#modules)
  * [Concurrency](#concurrency)
  * [Unification](#unification)
  * [Help](#help)
* [Built-in modules](#built-in-modules)
* [Error codes](#error-codes)
* [Capabilities](#capabilities)
* [CLI reference](#cli-reference)
  * [Subcommands](#subcommands)
  * [Vault modes](#vault-modes)
  * [Permission flags](#permission-flags)
  * [Exit codes](#exit-codes)


## Syntax

### Literals

| Syntax | Type | Example |
|--------|------|---------|
| Decimal digits, optional `-`/`+`, `_` separators | `Integer` | `42`, `-5`, `+7`, `0`, `1_000` |
| `0x` / `0o` / `0b` prefix | `Integer` | `0xFF`, `0o17`, `0b101` |
| Digits with `.` | `Float` | `3.14`, `-0.5`, `5.` |
| Scientific (`e`/`E`) | `Float` (or `Integer` if whole) | `1.5e3`, `2e-2`, `1e3` |
| `inf`, `-inf`, `nan` | `Float` (IEEE special) | `inf` |
| `0d` prefix, digits only | `BigInteger` (unbounded, exact) | `0d123`, `-0d5`, `0d1_000` |
| `0d` prefix with `.` or `e`/`E` | `BigDecimal` (exact base-10) | `0d12.5`, `0d0.1`, `0d1.5e3`, `0d1e3` |
| Double or single quotes | `String` | `"hello"`, `'world'` |
| Backticks with `${...}` | `String` (template) | `` `x = ${x}` `` |
| `true`, `false` | `Boolean` | `true` |
| `none` | `None` | `none` |
| Bare unquoted word | atom, only inside a `/q`-quoted slot | `foo` |
| `quote foo` | `Atom` | `foo` |

Type literals: `Number`, `Integer`, `Float`, `BigInteger`, `BigDecimal`,
`String`, `Boolean`, `Atom`, `Scalar`, `Any`, `None`, `List`, `Map`, plus
every named type you define with `def`.

#### Numeric literals in detail

**Integer** is a signed 64-bit value:
`-9223372036854775808 .. 9223372036854775807`.

- Decimal (`42`, `-5`, `+7`). An optional leading sign (`-`/`+`) is part
  of the literal; `+` is a no-op. Leading zeros are decimal, **not**
  octal (`010` is 10).
- Hex `0x…`, octal `0o…`, binary `0b…` (case-insensitive prefix), with an
  optional sign (`0xFF`, `-0o17`).
- `_` may be used as a **single** digit-separator **between** digits
  (`1_000_000`, `0xFF_FF`). Leading, trailing, or repeated underscores
  (`_1`, `1_`, `1__0`) are a syntax error.
- All integer literals — decimal **and** base-prefixed — are parsed
  exactly at every magnitude in range, and a value outside the int64
  range raises `[aql/integer_overflow]`. It never silently wraps or loses
  precision. (This includes the hex int64 minimum `-0x8000000000000000`.)

**Float** is IEEE-754 `binary64` (see
[design/IEEE-754-COMPLIANCE.8.md](design/IEEE-754-COMPLIANCE.8.md)):

- Any literal with a `.` is a `Float`, parsed correctly-rounded
  (round-ties-to-even): `3.14`, `-0.5`, trailing-dot `5.` → `5.0`.
  A **leading** `.` is **not** valid — `.5`, **and the signed `-.5` /
  `+.5`**, are all syntax errors. A number needs a digit before the dot;
  write `0.5` / `-0.5`.
- Scientific notation: `1.5e3`, `2e-2`. A whole-valued exponent form
  with no `.` and within int64 (`1e3`) is an `Integer`; otherwise it is a
  `Float`.
- Special values are the lowercase literals `inf`, `-inf`, `nan` (they
  render back to those same tokens).

**Recommended practice for the boundary cases:**

- **Exact large integers:** write them in **decimal** (or hex/oct/bin),
  which are exact and range-checked. Do **not** use scientific notation
  for a value you need to be an exact `Integer` — `1e19` exceeds the
  int64 range and silently becomes an (inexact) `Float`, whereas the
  decimal `10000000000000000000` is a clean `[aql/integer_overflow]`.
- **Infinity:** write the `inf` / `-inf` literal. An *overflowing*
  float literal such as `1e309` raises `[aql/float_overflow]` (you cannot
  spell ±∞ by overflowing a literal); use `inf`, or compute it
  (`mul 1e308 10`).
- **Tiny values:** the smallest positive subnormal is `5e-324`; a literal
  that underflows below that (e.g. `1e-400`) rounds to `0`. If you mean
  zero, write `0.0`.
- **Casing:** the special literals are lowercase only — `Inf`, `NaN`,
  `Infinity`, `+inf` are not recognised.
- **A name cannot start with a digit.** A digit-first token is always
  read as a number, so `2dup` is an `invalid numeric literal`, never a
  word. Word names begin with `[a-z_-$]` and may contain digits after the
  first character — so the paired-stack words put the digit **last**:
  `dup2`, `swap2`, `drop2`, `over2`.

#### Arbitrary-precision numbers: `BigInteger` and `BigDecimal`

The `0d` (or `0D`) prefix opts a literal into one of two **exact**,
arbitrary-precision numeric leaves under `Number` — siblings of `Integer`
and `Float`. They exist for the cases where the fixed-width leaves lose
information: integers beyond the int64 range, and decimals (money,
fractions) that binary `Float` cannot represent exactly (`0.1 + 0.2`).

- **`BigInteger`** (`math/big`): a `0d` literal with **digits only** —
  `0d123`, `-0d5`, `0d1_000_000`, `0d99999999999999999999999999999999`.
  Unbounded; never overflows.
- **`BigDecimal`** (`cockroachdb/apd`): a `0d` literal carrying a `.`
  **or** an exponent — `0d12.5`, `0d0.1`, `0d1.5e3`, **and** `0d1e3`
  (the exponent alone makes it a BigDecimal). Exact base-10; the scale is
  preserved on round-trip (`0d0.10` renders `0d0.10`).

A leading sign (`-0d5`, `+0d12.5`) and single `_` digit-separators are
allowed, exactly as for the fixed-width literals. Both render back with
the `0d` prefix so they re-parse to the same value.

**Type infection — widest exact leaf wins.** Among the exact leaves the
ladder is `Integer < BigInteger < BigDecimal`, so a mixed-leaf operation
promotes to the widest operand: `add 1 0d2` → `0d3` (BigInteger),
`add 0d2 0d0.5` → `0d2.5` (BigDecimal), `add 1 0d0.5` → `0d1.5`. The
existing `Integer ⊕ Float` rule is **unchanged** (`add 1 2.0` → `3.0`).

**A Big type never silently becomes a `Float`.** Mixing an exact Big
type with a binary `Float` in arithmetic is an `[aql/type_error]`
(`add 0d2 1.0`, `add 0d0.1 0.2`) — degrading to `Float` would throw away
the exactness the Big types exist to provide. Convert one operand
explicitly first. For the same reason `convert BigInteger 3.14` and
`convert BigDecimal 3.14` are **refused** (build a BigDecimal from a
`String` or the `0d` literal instead); `convert` between the exact leaves
and to/from `Integer`/`String` is exact, and `convert Float 0d2.5` is
allowed but documented as a lossy projection.

**Division.** `BigInteger div BigInteger` truncates toward zero and
returns a BigInteger (with `mod`), like `Integer div`. `BigDecimal div
BigDecimal` returns a BigDecimal rounded to the active decimal context —
by default IEEE **decimal128** (34 significant digits, round-half-even),
so `0d1.0 div 0d3.0` → `0d0.3333333333333333333333333333333333`.

**Comparison is honest across leaves.** `0d5 eq 5`, `1 cmp 0d1.0` → `0`,
and `0d0.5 eq 0.5` are all true (those magnitudes are exact in every
leaf). But a `Float` that is **not** an exact decimal does not equal its
`0d` lookalike: `0.1 eq 0d0.1` → `false`, because the Float's true value
is `0.1000000000000000055…`, not exactly one tenth (the same result
Python's `Decimal('0.1') == 0.1` gives).

**Transcendentals.** With an imported `MathUtil`, the apd-backed
functions return a `BigDecimal` for a Big argument computed to the active
context (`MathUtil.sqrt 0d2`, `cbrt`, `exp`, `log` (natural), `log10`),
and fractional `pow` (`0d2 pow 0d0.5`) likewise. Functions apd cannot
compute exactly — the trig family (`sin`/`cos`/`tan`/…), `log2`, `logb`
— accept a Big argument but return an (approximate) `Float`.

**Scoped precision — `with-decimal`.** Wrap a body in
`with-decimal {precision: N, rounding: "…"} [ … ]` to override the
BigDecimal context for every BigDecimal op inside it (arithmetic and the
apd-backed transcendentals). The override unwinds at the end of the
block and nests:

```
with-decimal {precision: 5} [0d1.0 div 0d3.0]                  # returns 0d0.33333
with-decimal {precision: 4 rounding: "down"} [0d2.0 div 0d3.0]  # returns 0d0.6666
```

Rounding names: `half-even` (default), `half-up`, `half-down`, `down`,
`up`, `ceiling`, `floor` (apd's `half_even` spellings are also accepted).

### Compound data

| Syntax | Meaning |
|--------|---------|
| `[a, b, c]` | List literal |
| `[:Type]` | Typed list (every element must match `Type`) |
| `{k:v, ...}` | Map literal |
| `{foo}` | Field shorthand — `{foo}` ≡ `{foo: foo}` (see [Map field shorthand](#map-field-shorthand)) |
| `{:Type}` | Typed map (every value must match `Type`) |

Commas are optional inside list and map literals — `[1 2 3]` and
`[1, 2, 3]` are equivalent. Two things to know:

* An **empty element** — a leading or repeated comma (`[,1]`, `[1,,2]`)
  — is a **syntax error** (`[aql/syntax_error]`), not a fabricated
  `null`. AQL has no implicit hole value; write `none` for an explicit
  empty value. (A trailing comma, `[1,]`, is fine.)
* A **duplicate key** in a map literal is accepted silently and the
  last value wins: `{a: 1, a: 2}` returns `{a:2}`.

### Comments

| Syntax | Scope |
|--------|-------|
| `# text` | Line — to end of line |
| `## text ##` | Block — delimited |

### Grouping

`(expr)` evaluates a sub-expression eagerly, regardless of forward
collection:

```
mul 2 (add 3 4)               # returns 14
```

### Template-string escapes

`\\`, `` \` ``, `\$`, `\n`, `\t`, `\r`. Use `\$` for a literal `${`.

### Word modifiers

A trailing `/...` suffix overrides a word's default argument shape:

| Modifier | Meaning |
|----------|---------|
| `/s` | Stack-only — never forward-collect |
| `/f` | Forward-only — never read the stack |
| `/N` | Force exactly N arguments |
| `/Nf` | N arguments, forward only |
| `/Ns` | N arguments, stack only |
| `/q` | The name as an atom — `foo/q` ≡ `(quote foo)` |
| `/r` | A reference — the function as inert data, not a call |
| `/u` | Usurp — `f/u` ≡ `usurp f` |
| `/t` | A type bound — `Map/t` ≡ `Type<Map>` ≡ `(Type of [Map])`; combines with no other modifier |

<!-- aql-test: skip -->
```
lower/f "ABC"                 # returns 'abc' — (lower is in aql:string-util — StringUtil.lower)
"DEF" lower/s                 # returns 'def'
lower/1 "GHI"                 # returns 'ghi'
```

### Map field shorthand

A map entry written as a bare name — with no `: value` — is shorthand
for binding that name to itself, mirroring JavaScript's `{ foo }`:

| Shorthand | Expands to | Notes |
|-----------|------------|-------|
| `{foo}` | `{foo: foo}` | value is the same auto-evaluated word |
| `{foo/r}` | `{foo: foo/r}` | a word modifier stays on the **value**; the key is the base name |
| `{foo?}` | `{foo?: foo}` | a trailing `?` keeps the field **optional**; the value is the bare word |

The rule in one line: **the key is the base name and the value is the
whole token.** So `{foo}` looks up the binding `foo` and stores it under
key `foo`; `{foo/r}` stores it under key `foo` but keeps the `/r` on the
value.

```
def x 1
{x}                           # returns {x:1}
def a 10  def b 20
{a b}                         # returns {a:10 b:20} — keys sort
{a c:3 b}                     # returns {a:10 b:20 c:3} — mixes with explicit pairs
{outer: {a}}                  # returns {outer:{a:10}} — nests
```

Because a shorthand value is auto-evaluated exactly like any bare map
value, the same rules apply (see
[Maps and access](#maps-and-access)): a plain binding resolves, a 0-arg
function dispatches, and a function that needs arguments must be held as
data with `/r` (or stored as an atom with `/q`):

```
def inc fn [[n:Integer] [Integer] [add n 1]]
{inc}                         # returns build error — inc dispatched 0-arg, fails its signature
{inc/r} . inc 5               # returns 6 — /r holds the function as data
{inc/q} . inc is Atom         # returns true — /q stores the bare name as an atom
```

The optional form composes with the `?:` optional-field rule: `{foo?}`
desugars to `{foo?: foo}`, i.e. the value becomes
`disjunct(foo, None, Absent)` — present, explicitly `none`, or absent.

**Only unquoted identifiers trigger the shorthand.** A quoted key
(`{'foo'}`, `{"foo"}`) or a non-identifier (`{123}`) is a parse error —
write the explicit `key: value` form for those. The pretty-printer
(`aql fmt`) normalises every shorthand back to its explicit form
(`{foo}` → `{foo:foo}`, `{foo/r}` → `{foo:foo/r}`, `{foo?}` →
`{foo?:foo}`).

**A word modifier belongs on a value, never on a bare key.** It is
legal on a shorthand entry (`{foo/r}` — the token is the value) but an
error on an explicit pair: `{foo/r: 1}` raises `[aql/illegal_key]`,
because the `/r` could only attach to the key `foo`, which is just a
name. If you genuinely need a `/` in a key, make it a literal with a
quoted key (`{'a/b': 1}`) or a computed key (`{[a/b]: 1}`).


## Evaluation model

* **Stack machine.** Each token either pushes a value or invokes a
  word. The final stack is the result.
* **Argument-order rule.** When a word runs, its parameter slots
  are filled **forward-first, then stack**. Tokens after the word
  are taken in source order into `args[0]`, `args[1]`, … until a
  barrier (`end`, `)`, another function word, type mismatch). Any
  remaining slots are filled from the stack, top of stack into the
  next-to-fill slot first. See
  **[Tutorial §3](TUTORIAL.md#the-argument-order-rule)**.
* **Type-directed collection.** A forward token is only consumed if
  it matches the next expected type; mismatches stop collection and
  the word executes with what it has (or fails if it doesn't have
  enough).
* **Structure-first.** A word forward-collects a following token only
  when one of its signatures could actually take it; a parenthesised
  expression or a value of an incompatible type is left to run on its
  own. So `import "mod"` takes its path and stops — no `end` needed
  before using the namespace (`import "aql:math-util"` then
  `5 MathUtil.log`).
* **Empty parens.** `()` is the empty expression: it yields no value
  (`5 () add 3` returns `8`) and nests freely (`(())`, `(add 1 ())`).
* **Left-to-right.** Words that are still waiting evaluate strictly
  in source order. Use `(...)` to override.
* **Quotation.** A `[ … ]` literal **evaluates its contents** as a
  sub-program and collects the resulting stack into the list — so
  `[add 1 2]` returns `[3]`, not `[add 1 2]`, and a bare `[dup mul]` errors
  (it runs `dup` on an empty stack). To hold code *unevaluated*, use
  `quote` (`quote [add 1 2]` returns `[word(add) 1 2]`). `do` runs a list as
  a program and leaves its result stack (`do [add 1 2]` returns `3`). The one
  subtlety: when a `[ … ]` is written **directly as the block
  argument** of a word that expects code — `do`, `each`, `if`/`for`
  branches, `fn`/`macro` bodies — it is held deferred and run by that
  word, which is why `each [dup mul]` works even though the same
  bracket evaluated on its own would not.
* **Referents.** A quoted atom can remember what its name referred to.
  `quote foo` (and `(quote foo)`) snapshots `foo`'s current binding onto the
  atom as its **referent**; `referent` reads it back:
  ```
  def x 5  def q (quote x)  def x 9
  q referent              # returns 5 — the value x had WHEN quoted (a frozen snapshot)
  ```
  The snapshot is shallow (the same copy semantics as closure capture). A bare
  `name/q` atom carries no snapshot, so `referent` falls back to the name's
  **current** binding (`def x 9  x/q referent` returns `9`); an unbound name is
  an error. The referent is metadata only — it never affects atom identity:
  same-named atoms stay equal and canonicalise to `name/q` regardless of what
  each referred to. (At load time a resolution pass also stamps referents for
  names already bound when the program starts; names bound only during
  execution are captured by `quote`.)
* **Macros.** A `macro` runs at expansion time on its operands *as
  code* and splices the result into the call site — new syntax in
  AQL itself. See **[Macros](#macros)**.
* **`end`.** Forces the nearest waiting word to stop forward
  collection — needed only when the next token would otherwise be a
  valid argument (e.g. `import "aql:math-util" "foo" print`). `;`
  is a synonym.
* **`aql check` advisories.** The checker raises non-gating advisories
  (info level) for likely mistakes that still run — notably the
  forward-greediness gotcha `1 2 add 3 mul` (returns `5`, not `9`;
  group as `(1 2 add) 3 mul`). See
  **[Explanation §Forward greediness](EXPLANATION.md#forward-greediness-and-stranded-operands)**.

See **[Explanation §The stack model](EXPLANATION.md#the-stack-model)**
for a longer treatment.


## Type system

### Hierarchy

```
Any
├── None                            -- unit; sole inhabitant: `none`
├── Never                           -- empty / bottom
├── Scalar
│   ├── Atom
│   ├── Boolean                     -- false | true
│   ├── Bytes                       -- byte string (`0x…` literals)
│   ├── Number
│   │   ├── Integer                  -- signed int64 (overflow → error)
│   │   ├── Float                    -- IEEE-754 binary64
│   │   ├── BigInteger               -- unbounded exact int   (`0d123`)
│   │   └── BigDecimal               -- exact base-10 decimal (`0d12.5`)
│   ├── String
│   │   ├── EmptyString
│   │   └── ProperString
│   ├── Path
│   └── Time
│       ├── Date, DateTime, Instant
│       └── (aql:time-util owns TimeOfDay, Duration
│            with CalendarDuration | ClockDuration, and Timezone —
│            module-minted per import, e.g. TimeUtil.CalendarDuration)
├── Node
│   ├── List
│   │   └── FlexList                 -- mutable list (see Flex nodes)
│   └── Map
│       └── FlexMap                  -- mutable map (see Flex nodes)
├── Ideal
│   ├── Class                       -- user classes root here
│   ├── Resource (Entity)           -- SDK object-type hierarchy
│   ├── Surface                     -- user surfaces (operation contracts)
│   ├── Record, Options, Error
│   ├── Store, Table
│   └── (module-minted per import — the owning module exports
│        the literals: aql:net owns Fetch with Request | Response
│        (Net.Response); aql:time-util owns Timeout and Interval
│        (TimeUtil.Timeout); aql:matrix-util owns Tensor with
│        Matrix | Vector (MatrixUtil.Matrix))
├── Word
│   └── (internal control words)
└── Type
    ├── Function, FunctionSignature
    ├── Disjunct (Enum)
    └── Negation
```

A child matches its parent (`Integer` is a `Number` is a `Scalar`
is an `Any`); the converse is false. Types are written with
slash-separated paths in `pathof`; short names like `Number` or
`Integer` are accepted in signatures.

### Short names

| Short | Full path |
|-------|-----------|
| `String` | `Scalar/String` |
| `Number` | `Scalar/Number` |
| `Integer` | `Scalar/Number/Integer` |
| `Float` | `Scalar/Number/Float` |
| `Boolean` | `Scalar/Boolean` |
| `Atom` | `Scalar/Atom` |
| `List` | `Node/List` |
| `Map` | `Node/Map` |
| `Store` | `Ideal/Store` |
| `Table` | `Ideal/Table` |
| `Record` | `Ideal/Record` |
| `Options` | `Ideal/Options` |
| `Timeout` | `Ideal/Timeout` |
| `Interval` | `Ideal/Interval` |
| `Function` | `Word/Function` |

> **`Float` is IEEE-754 `float64`.** A fractional literal like `3.14`
> is a `Float`, so expect binary-floating-point behaviour, not exact
> base-10: `add 0.1 0.2` returns `0.30000000000000004`. `Integer` and
> `Float` are distinct nodes but compare equal by magnitude (`1 eq 1.0`
> returns `true`, `1 cmp 1.0` returns `0`); they are **not**
> interchangeable, because integer `div` truncates while float `div`
> does not. `convert` does not move a value between the two numeric
> nodes — see [Type words](#type-words). (The name `Decimal` is
> reserved for a future exact base-10 type; it is not yet defined.)

### Disjunctions

`A tor B` produces a disjunctive type — values match if they match
either side:

```
def OptInt (Integer tor none)
OptInt unify 5                # returns 5 true
OptInt unify none             # returns none true
OptInt unify "x"              # returns '~unify-fail' false
```

### Absence — `none` and `None`

One absence concept, two spellings with distinct roles:

* **`none`** (lowercase) is the **value** — the sole inhabitant of
  the `None` type. It is what you write in source: the literal
  itself, optional-field constraints (`(String tor none)`, "string
  or absent"), and tests (`x eq none`).
* **`None`** (capital) is the **type** — a type literal usable
  anywhere a type goes (`x is None`, signature slots), and also the
  form the engine **returns** at absence sites.

```
typeof none                   # returns None
none eq None                  # returns true — value and type compare equal
none is None                  # returns true
5 eq none                     # returns false — absence equals nothing else
if none [1] [2]               # returns 2 — none is falsy
```

Words that *produce* absence — `get`/`.` on a missing key, an
out-of-range index, an omitted optional record field — return the
`None` type literal. So in canonical rendering, capital `None` marks
absence the **engine** produced, and lowercase `none` marks the
value your **source** wrote:

```
{a:1} dot b                   # returns None — engine-produced absence
{a:none}                      # returns {a:none} — source-written value
def P refine Record [name:String nick:(String tor none)]
make P {name:"Bob"}           # returns {name:'Bob' nick:None}
(make P {name:"Bob"}) . nick eq none      # returns true
```

To distinguish *absent* from *present-but-`none`*, ask `has` — the
Boolean presence predicate (`get` returns `None` on a miss, `getr`
raises, `has` answers whether the key is **bound** at all):

```
{a:None} has a                # returns true  — present, value is none
{a:None} dot a                # returns None  — indistinguishable from…
{a:1}    dot b                # returns None  — …an absent key
{a:1}    has b                # returns false
none     has a                # returns false — total: composes in conditions
```

Because the two spellings compare equal under `eq` and both satisfy
`is None`, the distinction never changes what a test answers — it is
visible only in rendering.

### Negation

`tnot T` produces the complement type — a value matches it if it does
**not** match `T`. With `tor` (union) and `tand` (intersection) it
closes the type algebra under Boolean operations:

```
def NotStr (tnot String)
5 is NotStr                              # returns true
"x" is NotStr                            # returns false
5 is (tnot (String tor Boolean))         # returns true — neither
Integer tand (tnot String)               # returns Integer — disjoint — no-op
String tand (tnot String)                # returns Never — self-complement is empty
(Integer tor String) tand (tnot String)  # returns Integer — drops the excluded alternative
```

The identities hold: `tnot Never` is `Any`, `tnot Any` is `Never`, and
`tnot (tnot T)` is `T`. A guard narrows the else branch by the
complement — after `if (x is T) […] […]`, `x` is `cur tand (tnot T)`
in the else branch.

De Morgan's laws fold conjunctions and disjunctions of negations:

```
(tnot Integer) tand (tnot String)  # returns tnot (Integer|String) — tnot A tand tnot B = tnot (A tor B)
(tnot Integer) tor (tnot String)   # returns Any — tnot A tor tnot B = tnot (A tand B) = tnot Never
```

Negating a refinement (`DepScalar`) takes its closed-form complement —
the bound flips within the base, so intersecting with the base reduces
to a positive refinement:

```
Integer tand (tnot (Integer gt 0))        # returns (Integer lte 0)
Integer tand (tnot (between 5 10 Integer))  # returns (Integer lt 5)|(Integer gt 10)
```

### Type ordering

Every type has a unified integer rank. `tcmp` and `sort` expose a
single LCA-Comparer-then-Rank cascade, so cross-type comparisons are
well-defined and total (`cmp` / `lt` / `gt` run the same cascade but
are restricted to same-family pairs). Type literals sort strictly
below their concrete inhabitants of the same family:

```
Integer tcmp 0                # returns -1
0 gt Integer                  # returns true
sort [Integer 0 5 -3]         # returns [Integer -3 0 5]
[1,2] cmp [1,3]               # returns -1
```

> **`lt`/`gt`/`lte`/`gte` with a type-literal *left* operand do not
> compare — they construct.** `Integer lt 0` builds the predicate
> refinement `(Integer lt 0)` (see
> [`fn` type semantics](#fn-type-semantics)). To ask
> the ordering question, put the literal on the right (`0 gt Integer`)
> or use `tcmp` (`(Integer tcmp 0) lt 0` returns `true`).

### Classes

`class {schema}` mints a sealed nominal record type under
`Ideal/Class`. The schema map declares each field once: a **type**
value declares a required field, a **concrete** value declares a
default (and the default's own type becomes the field's type).
`make` constructs flat instances — every field resolved eagerly,
no prototypes. A **mutable** default — a flex node, `Store`, or
instance — is copied fresh for each `make`, so
instances never share one underlying container (no Python-style
mutable-default trap); a mutable value you **pass in** is taken
as-is, so deliberate sharing stays available:

```
def Point class {x:Float y:0.0}     # x required, y defaults to 0.0
def p (make Point {x:1.0})
p.x                                  # returns 1.0 — dot access reads fields
p dot y                              # returns 0.0
describe Point                       # prints the schema view
```

Field typing is **strict**, at `make` and at `set` alike — no silent
conversion. Predicate types run their predicate, refined types
enforce the refinement, and `const v` pins a field to exactly one
value:

```
def Radius (Float gte 0.0)
def Circle class {r:Radius}
make Circle {r:-1.0}                # error — predicate fields run their predicate
def Tagged class {kind:(const 'point')}   # kind can only ever be 'point'
def Foo refine Integer
def S class {x:(make Foo 1)}        # a Foo-typed field defaulting to Foo(1)
```

Instances are **sealed**: `set` re-types an existing field, and a
new key is a loud `sealed_field` error. Subclass with
`refine <Class> {…}` — child fields must unify with the parent's,
and instances resolve the whole chain flat:

```
def Point3 refine Point {z:Float}   # Class/Point/Point3
```

Equality: `deq` is structural within the same exact class (a
subclass instance never equals a parent instance); `eq` is identity
(two names alias the same instance only if they share its fields).
`undef <Class>` removes the *name* (construction errors), but live
instances keep their identity, reads, and typed writes.

Serialization: `StructUtil.jsonify` emits a `$class` marker on
instances (user keys starting with `$` are escaped to `$$`), and
`StructUtil.reify Target json-or-node` hydrates back through `make`
— defaults fill, required fields and predicates enforce, unknown
keys error. The target is an explicit class or a `tor` union the
`$class` selects within. `StructUtil.clone` copies an instance
type-preservingly; `StructUtil.setpath` returns a *new* instance
with the edit applied, schema-checked.

Classes sit in a 2×2 container table:

|  | immutable (`set` returns a copy) | mutable (`set` writes in place) |
|---|---|---|
| **open keys** | `Map` `{…}` | `FlexMap` — `flex {…}` |
| **fixed shape** | `List` `[…]` | `FlexList` — `flex […]`; class instances (typed, sealed) |

The mutable column is the flex nodes (`flex {…}` / `flex […]`; see
[Flex nodes](#flex-nodes--flexmap-and-flexlist)) plus class instances.
`node <flex>` freezes a flex node back to an immutable Map/List. Flex
nodes and class instances are shared mutable state: writes are visible
through every alias, and concurrent writers (e.g. inside `parallel`
branches) must coordinate — prefer the immutable column for data that
crosses branch boundaries.

> The former open-keyed `Object` container (`object {…}` / `make
> Object`) and fixed-shape `Array` (`array […]` / `make Array`) were
> removed. Use `flex {…}` / `flex […]` for open mutable containers,
> and `class` for a typed, sealed record.

### Surfaces

A surface is a pure operation contract: a named set of required
signatures with no bodies and no state, minted under
`Ideal/Surface`. `Self` marks the positions the conforming type
occupies. Conformance is **explicit** — a type joins a surface only
by declaring `exposes`, which checks the word's overload table
(with `Self` substituted; contravariant parameters, covariant
returns) and raises `surface_unsatisfied` listing every gap:

```
def Shape surface {area: (fnsig [[Self] [Float]])}
def Circle class {r:1.0}
def area fn [[c:Circle] [Float] [(c dot r) mul 6.28]]
Circle exposes Shape
def total fn [[s:Shape] [Float] [area s]]
total (make Circle {r:2.0})   # 12.56
(make Circle {}) is Shape     # true
5 is Shape                    # false — no exposes, no membership
```

A surface is a normal type after that: surface-typed fn parameters
dispatch on membership, subclass instances of an exposer conform,
and the type algebra applies (`Shape tor none`, `tnot Shape`,
`Circle tand Shape` → `Circle` for an exposer). The conformance
check runs at declaration time; `describe Shape` prints the
contract.

### Generic types

A generic type is a **schema**: a class, record, or fn-shape
declared over one or more type parameters, instantiated with
concrete type arguments. The angle-bracket form is the usual
spelling — a capitalised name directly before `<` opens the
parameter or argument list (commas between entries are optional,
like list elements):

```
def Box<T> class {value:T}
def b:Box<Integer> {value:42}
typeof b                                   # returns Box of [Integer]
b is Box                                   # returns true
b is Box<Integer>                          # returns true
b is Box<String>                           # returns false
```

Each distinct instantiation is minted **once** (`Box<Integer> teq
(Box of [Integer])` returns `true`) as a nominal child of the
schema, so instances get real type identity: `typeof` names the
instantiation, a bare `Box` in any type position means "any
instantiation of Box", and sibling instantiations are distinct
types — v1 generics are **invariant**, so `Box<Integer>` is not a
`Box<Number>`. Class instantiations keep the full class contract
(strict field typing, sealing, `deq` per-instantiation,
jsonify/reify serialization).

The angle form is pure sugar over four ordinary words, desugared at
parse time — `def Name<…>` is `def Name gen [...]`, and a use-site
`Name<…>` is `( Name of [...] )` — so macros, `quote`, and
`Vm.parse` only ever see the canonical paren form. (`<` and `>` are
general-purpose parser tokens; the generics rule is their only
current consumer, and comparisons stay on `lt`/`gt`.) The canonical
form is needed for generic **functions**, whose names are lowercase:

```
def Pair<K, V> refine Record [key:K value:V]
Pair of [String Integer]                   # returns record{key:String value:Integer}
def first gen [T] fn [[xs:[:T]] [T] [xs get 0]]
first [10 20 30]                           # returns 10
```

A generic fn binds its parameters per call from the arguments —
`x:T` binds `typeof(arg)`, a `[:T]`/`{:T}` pattern binds the union
of the element types — and the bindings are in scope inside the
body, so `make (Box of [T]) {…}` constructs the caller's
instantiation. Bounds constrain parameters with `extends`
(`T extends C` inside angles); membership is the ordinary `is`
test, so lattice types, predicate refinements, disjunctions, and
**surfaces** all work as bounds, and violations reject at dispatch
and instantiation (`constraint_violation`). Defaults are declared
with `=` in angle form (`(T default D)` canonically) and may
reference earlier parameters:

```
def Sorted<T extends Number> class {items:[:T]}
def Result<T, E = Error> refine Record [ok:T err:E]
(Result of [Integer])                      # returns record{ok:Integer err:Error}
```

A schema's own name is unbound while its body builds; recursion is
written `Self of [...]`:

```
def Tree<T> refine Record [value:T left:(Self of [T])]
Tree of [Integer]                          # returns record{value:Integer left:Tree of [Integer]}
```

A bare schema used as a construction target **infers** its
arguments from the body (an uninferable, undefaulted parameter is a
loud `unbound_param`, never a silent `Any`; explicit instantiation
always wins):

```
def Box<T> class {value:T}
typeof (make Box {value:42})               # returns Box of [Integer]
```

In `aql check`, a generic fn's body is checked once at the
definition against its parameter bounds (operations on a bare `T`
must be justified by the bound), and each call site refines the
declared return through the inferred bindings.

## Word reference

Each entry lists the word, its signature(s), and one or more
examples. Where multiple signatures exist, they're tried in
declaration order — first match wins.

### Stack manipulation

All stack words are stack-only (modifier `/s`).

| Word | Effect | Description |
|------|--------|-------------|
| `dup` | `a → a a` | Duplicate top |
| `drop` | `a →` | Remove top |
| `swap` | `a b → b a` | Exchange top two |
| `over` | `a b → a b a` | Copy second to top |
| `rot` | `a b c → b c a` | Rotate top three |
| `nip` | `a b → b` | Remove second |
| `tuck` | `a b → b a b` | Copy top below second |
| `dup2` | `a b → a b a b` | Duplicate top pair |
| `drop2` | `a b →` | Remove top pair |
| `swap2` | `a b c d → c d a b` | Swap top two pairs |
| `over2` | `a b c d → a b c d a b` | Copy third pair to top |
| `pick` | `n → v` | Copy value at depth n |
| `roll` | `n → v` | Move value at depth n to top |
| `depth` | `→ n` | Current stack size |
| `stack` | `→ [...]` | Entire stack as a list |

### Arithmetic

Forward-collecting, Integer/Float with auto-promotion. The
asymmetric ops (`sub`, `div`, `mod`, `pow`) follow the
**argument-order rule** — see
[Tutorial §3](TUTORIAL.md#the-argument-order-rule). All three call
forms `a b sub`, `a sub b`, and `sub b a` compute `a - b`.

| Word | Operation | Example |
|------|-----------|---------|
| `add` | `a + b` (commutative) | `add 1 2` returns `3` |
| `sub` | `a - b` | `10 sub 3` returns `7` |
| `mul` | `a * b` (commutative) | `mul 4 5` returns `20` |
| `div` | `a / b` | `10 div 2` returns `5` |
| `mod` | `a % b` | `10 mod 3` returns `1` |
| `pow` | `a ^ b` | `2 pow 10` returns `1024` |

`add` on non-numeric scalars performs string concatenation:
`"a" add "b"` returns `'ab'`. This wins whenever **either** operand is
non-numeric: the other operand is rendered to text and the result is
a `String`, so `1 add "x"` returns `'1x'` and `true add 1` returns `'true1'` (no
type error — `add` simply concatenates). Use it deliberately, not as
a guard against mixed-type mistakes.

Two further sharp edges on numbers:

* **Integer division truncates** toward zero and never produces a
  remainder or a `Float`: `7 div 2` returns `3`, `1 div 2` returns `0`. Use a
  `Float` operand to get real division — `7.0 div 2` returns `3.5`.
* **Division/modulo by zero splits by type.** Integer `div`/`mod` by zero
  is a hard error (there is no integer infinity). Float `div`/`mod` by
  zero follows IEEE-754: `1.0 div 0.0` returns `inf`, `-1.0 div 0.0`
  returns `-inf`, and `0.0 div 0.0` (and any `mod` by zero) returns `nan`.
  A `Float` operand routes the whole operation through the Float path, so
  `7 div 0.0` is `inf`. Detect the results with `MathUtil.is-inf` /
  `MathUtil.is-nan`.
* **Integer is a 64-bit signed integer; overflow is an error, not a
  wrap.** An `Integer` holds any whole number in
  `-9223372036854775808..9223372036854775807` (int64). A literal outside
  that range, or an `add`/`sub`/`mul`/`pow` whose result would leave it,
  raises `[aql/integer_overflow]` rather than silently wrapping or
  degrading to a `Float`: `2 pow 63` and `add 9223372036854775807 1` both
  error. Make an operand a `Float` (e.g. `add 9223372036854775807 1.0`)
  for an approximate IEEE-754 result. (Arbitrary-precision integers are a
  planned future change — see `design/INTEGER-OVERFLOW-STRATEGY.5.md`.)
* `Float` is an IEEE-754 binary `float64`, **not** a base-10
  decimal — `add 0.1 0.2` returns `0.30000000000000004` and `1 eq 1.0`
  returns `true` even though the two divide differently. See
  [Type system](#type-system).
* **Special Float values** are written with the lowercase literals
  `inf`, `-inf`, and `nan` (reserved, like `true`/`false`/`none`); they
  render back to those same tokens. NaN follows IEEE rules for the
  ordering words: `nan lt 5.0`, `nan lte 5.0`, `nan gt 5.0`, `nan gte
  5.0`, and `nan eq nan` are **all `false`** (a comparison involving NaN
  is *unordered*), while `nan neq nan` is `true`. For a **total** order —
  what `cmp`, `tcmp`, and `sort` use — NaN is treated as the greatest
  value (`-inf < finite < inf < nan`), so sorting a list with a NaN is
  deterministic (NaN sorts last) rather than leaving it unordered.

Additional numeric words (`abs`, `negate`, `sign`, `min`, `max`,
`floor`, `ceil`, `round`, `trunc`, `sqrt`, `cbrt`, `exp`, `log`,
`log2`, `log10`, `sin`, `cos`, `tan`, `asin`, `acos`, `atan`,
`atan2`, `hypot`, constants `MathUtil.pi`, `MathUtil.e`) live in the
**`aql:math`** native module. Import to use:

```
import "aql:math-util"
MathUtil.abs -5                   # returns 5
MathUtil.floor 3.7                # returns 3
MathUtil.sqrt 16                  # returns 4.0
```

### Strings

All forward-collecting. The "options" form takes a trailing map
with named flags (see each word's docs in
`design/LANGREF.10.md` for the full set).

**Argument-order note:** for binary/ternary string words like
`contains`, `indexof`, `slice`, `replace`, `split`, the
all-forward form `WORD input arg…` is the clearest reading per the
[argument-order rule](TUTORIAL.md#the-argument-order-rule). Infix
forms work too but require placing the search/needle on the *left*
of the word, with the haystack as the forward arg.

| Word | Description | Example |
|------|-------------|---------|
import "aql:string-util" | `StringUtil.upper` | Uppercase | `StringUtil.upper "hello"` returns `'HELLO'` |
import "aql:string-util" | `StringUtil.lower` | Lowercase | `StringUtil.lower "ABC"` returns `'abc'` |
import "aql:string-util" | `StringUtil.concat` | Join list elements into a string | `StringUtil.concat ["a","b"]` returns `'ab'` |
import "aql:string-util" | `StringUtil.split` | Split string by separator (subject last) | `StringUtil.split "," "a,b"` returns `['a','b']` |
import "aql:string-util" | `StringUtil.contains` | Substring test (haystack last) | `StringUtil.contains "ell" "hello"` returns `true` |
import "aql:string-util" | `StringUtil.indexof` | Index of a needle in a haystack — **haystack last**: `indexof needle haystack` (string only; for the list form see `ArrayUtil.indices` under [List and array words](#list-and-array-words)) | `StringUtil.indexof "ll" "hello"` returns `2` |
| `slice` | Substring; negative indices ok | `"hello" slice 1 3` returns `'el'` |
import "aql:string-util" | `StringUtil.replace` | Replace pattern (subject last) | `StringUtil.replace "l" "r" "hello"` returns `'herlo'` |
import "aql:string-util" | `StringUtil.repeat` | Repeat string (subject last) | `StringUtil.repeat 3 "ab"` returns `'ababab'` |
import "aql:string-util" | `StringUtil.trim` | Trim whitespace or chars | `StringUtil.trim "  hi  "` returns `'hi'` |
import "aql:string-util" | `StringUtil.pad` | Pad to width | `"hi" StringUtil.pad 5` returns `'hi   '` |
import "aql:string-util" | `StringUtil.match` | Substring match, returns a struct (subject last) | `StringUtil.match "b" "abc"` |

#### Options examples

The subject string is the **last** string operand; an Options map trails it:

```
import "aql:string-util" StringUtil.split   ","    "a,,b"  {keepEmpty: true}            # returns ['a' '' 'b']
import "aql:string-util" StringUtil.contains "Ell"  "hello" {cs: "insensitive"}          # returns true
import "aql:string-util" StringUtil.replace "a" "b" "aaa"   {scope: "all"}               # returns 'bbb'
```

### Boolean

The built-in boolean words are `and`, `or`, `not`, and `xor`. The
remaining gates — `nand`, `nor`, `xnor`, `implies`, `iff` — live in
the `aql:logic-util` module and are called qualified after importing
it.

| Word | Description | Example |
|------|-------------|---------|
| `and` | Logical AND (short-circuit) | `true and false` returns `false` |
| `or` | Logical OR (short-circuit) | `true or false` returns `true` |
| `not` | Logical NOT | `not true` returns `false` |
| `xor` | Exclusive OR | `true xor true` returns `false` |
| `LogicUtil.nand` | NOT AND (needs `aql:logic-util`) | `LogicUtil.nand true true` returns `false` |
| `LogicUtil.implies` | Implication (needs `aql:logic-util`) | `true LogicUtil.implies false` returns `false` |

> **`and` / `or` return an operand, not a coerced boolean.** They
> short-circuit and yield the value that decided the result, of
> whatever type: `1 2 and` returns `2`, `false 5 and` returns `false`,
> `0 9 or` returns `9`. Wrap with `not not` (or compare) if you need a
> strict `Boolean`.

### Comparison

The **ordering** words (`cmp`, `lt`, `lte`, `gt`, `gte`) are
**family-restricted**: they compare only same-type values, or values a
shared same-family comparer can handle (`Integer`↔`Float` via `Number`,
two `Date`s, `EmptyString`↔`ProperString` via `String`, the
instant-bearing `Time` leaves chronologically). A cross-family pair
(`Integer`↔`String`, `List`↔`Map`) raises `[aql/incomparable]`.

`tcmp` is the **unrestricted** total order — it compares *any* two
values (the same order `sort` and the collection words use), returning
`-1` / `0` / `1`. Reach for it when you want the cross-type ordering the
restricted words refuse. See
**[Explanation §Type ordering](EXPLANATION.md#type-ordering)**.

> **Equality is not restricted.** `eq`/`neq`/`deq` compare across types
> safely — different types are simply *not equal* (`1 eq "1"` returns
> `false`, never an error). Only the **ordering** words restrict.
>
> **`eq` is identity for compounds; `deq` is structural — by design.**
> `eq` compares scalars (numbers, strings, booleans, atoms) by value, but
> lists and maps by *identity*: two distinct equal-looking lists are not
> `eq` (`["a" "b"] eq ["a" "b"]` → `false`). To compare compound *contents*,
> use `deq`, the deep/structural form (`["a" "b"] deq ["a" "b"]` → `true`).
> `assert.equal` (the test word) is deep, so a property body written with
> `eq` over lists can pass vacuously — reach for `deq` there.

```
1 lt 2.0                      # returns true        — Integer vs Float (shared Number)
1 lt "a"                      # returns error       — [aql/incomparable]; use tcmp
1 tcmp "a"                    # returns -1          — Integer ranks below String
[3 "a" 1 true] sort           # returns [true 1 3 'a']   — sort uses the total order
```

| Word | Description | Example |
|------|-------------|---------|
| `eq` | Equal — scalars by value, **compounds by identity** | `1 eq 1.0` returns `true`; `[1,2] eq [1,2]` returns `false` |
| `neq` | Not equal (negation of `eq`) | `1 neq 2` returns `true` |
| `deq` | **Deep / structural** equality (compares contents) | `[1,2] deq [1,2]` returns `true` |
| `lt` | Less than (same-family) | `1 lt 2` returns `true` |
| `gt` | Greater than (same-family) | `2 gt 1` returns `true` |
| `lte` | Less or equal (same-family) | `1 lte 1` returns `true` |
| `gte` | Greater or equal (same-family) | `2 gte 1` returns `true` |
| `cmp` | Three-way (same-family): `-1` / `0` / `1` | `5 cmp 10` returns `-1` |
| `tcmp` | Three-way across **any** types (total order) | `1 tcmp "a"` returns `-1` |
| `between` | Build closed-interval refinement | `Integer between 10 20` |

### Definition and scoping

| Word | Description | Example |
|------|-------------|---------|
| `def` | Define a word | `def x 42` |
| `undef` | Remove the latest definition | `undef x` |
| `fn` | Create typed function | `fn [[Integer] [Integer] [dup mul]]` |
| `var` | Scoped variable block | `5 var [[x] mul x x]` returns `25` |
| `args` | Current `fn` args list (inside body) | `args . 0` |
| `quote` | Prevent evaluation of next token | `quote [add 1 2]` |
| `referent` | What a quoted atom's name refers to | `def x 5  (quote x) referent` returns `5` |
| `word` | Splice marker: spreads a list's elements (any other value: itself) into the token stream | `[0 word [1,2,3] 4]` returns `[0 1 2 3 4]` |

> **Core bindings are frozen; core words are open.** `def`/`undef` may
> not rebind a built-in word to a *value*, nor touch the literals
> `true`/`false`/`none` — `def add 42`, `def true …`, `undef if` all
> raise `[aql/reserved_word]`. But `def <built-in> fn […]` **merges**
> the fn's signatures into the word in the current scope (fn body /
> module body / top level) — a *word extension*: new argument-type
> tuples append after the built-in's own (locked) signatures, so no
> previously-valid call changes its dispatch; a tuple exactly matching
> a locked signature raises `[aql/locked_signature]`; `undef <word>`
> pops the extension. The sealed words `def` / `make` / `word` cannot
> be extended at all. A module exports its merged word like any fn
> (`export "M" {add: add/r}`) and importing the module transplants the
> extension one level into the importer. A **module** may extend a core
> word only with at least one **user-minted** argument type per
> signature — a type the module creates with `refine` or `class`.
> Built-in types don't qualify, whether kernel (`Integer`, `Map`, …)
> or registered by `aql:` modules (`Date`, `Matrix`, …): a
> builtin-only tuple like `[Integer Map]` raises
> `[aql/extend_user_type]`, so `add 1 {}` can never start working
> because of an import (top-level programs are unrestricted). See
> `lang/spec/open-words.tsv` and `design/OPEN-WORDS.0.md`.
> Re-`def`ing your **own** words still shadows as before (`def x 1; def
> x 2` ⇒ `x` is `2`), and a built-in *type* name (`Integer`, …) was
> already unusable as a `def` target.

#### Splices and spread — `word`

`word v` wraps its (unevaluated) argument in a splice marker. When the
marker reaches the evaluation pointer it is replaced by its payload: a
plain list contributes its **top-level elements**, any other value
contributes itself. This is AQL's spread operator, and it works inline,
bound, and in argument positions:

```
[0 word [1,2,3] 4]            # returns [0 1 2 3 4]      — inline spread
def vs word [2,3]             # bind a spread
[1 vs 4]                      # returns [1 2 3 4]        — spread in a literal
add3 1 vs                     # ≡ add3 1 2 3             — spread into a call
add vs                        # ≡ add 2 3 — returns 5
```

A **data**-splice-bound word (the payload holds only values) is
equivalent to the written paren group wherever it stands as an
argument: `f w ≡ f (w)`. A multi-value spread fills several parameter
slots; an empty one (`def e word []`) contributes nothing.

Three deliberate exceptions:

* **Name-capture slots win.** `/q` slots take the word's *name*
  regardless of its binding — `quote vs` is the atom `vs`, and
  `inspect vs` inspects the definition.
* **Code splices are macros, not spreads.** A payload containing words
  (`def inc word [1 add]`) runs Forth-style against the **live** stack
  when it fires (`1 inc inc inc` returns `4`); it is *not* expanded in
  argument positions. Group it explicitly — `f (p)` — to pass its
  result as an argument.
* **Rebinding aliases.** `def y vs` copies the *binding* — the marker
  itself — so `y` is the same splice as `vs` (and spreads everywhere
  `vs` does). Write `def y (vs)` to force expansion at a `def`.

(For spreading *tokens into generated code* at expansion time, see
`splice` under **[Macros](#macros)**; for concatenating into a
FlexList in place, `append [3,4] fl` already spreads its list
argument's elements.)

#### `fn` shape

A `fn` body is a flat list of `[input-sig] [output-sig] [body]`
triples. Inputs may be plain types or `name:Type` pairs (the names
become local bindings during the body); the output-sig declares the
return type(s):

```
def inc fn [[n:Integer] [Integer] [add n 1]]
inc 5                         # returns 6

def avg fn [[a:Number b:Number] [Float] [(add a b) div 2.0]]
avg 3 4                       # returns 3.5
```

**Return types are checked.** When the body finishes, each declared
output type must accept the corresponding result value, by the same
rule the parameters use (see [fn type semantics](#fn-type-semantics)
below). A mismatch is an error, not a silent pass:

```
def bad fn [[] [Integer] ['hi']]
bad                           # returns [aql/type_error] return value 1: expected Integer got ProperString
```

Multiple triples declare overloads (the engine tries each in order);
multiple output types declare multiple return values.

**`/q` params capture names.** A param typed `Atom/q` takes the next
*bare word* as its atom name — the argument is presented as if quoted,
exactly like the built-in name-takers (`def`, `inspect`, `quote`) —
and the capture wins over any binding the word currently has:

```
def greet fn [[n:Atom/q] [String] [`hi ${n}`]]
greet world                   # returns 'hi world' — no quoting needed
def world 99
greet world                   # returns 'hi world' — capture trumps the binding
greet world/q                 # returns 'hi world' — an explicit atom works too
```

**Type-valued params take type literals.** A param typed `Type`
admits exactly what `is Type` admits — the one membership question,
asked identically at params, `is`, and returns:

```
def fresh fn [[t:Type coll:List] [Any] [make t coll]]
fresh FlexList [1,2]          # returns (flex [1 2]) — the literal drives construction
Map is Type                   # returns true
```

The `/t` suffix bounds the slot: `Map/t` is sugar for `Type<Map>`,
itself sugar for `(Type of [Map])` — the type of type literals
conforming to `Map`. Bounds compose with named types, including named
disjunctions:

```
def MapOrList (Map tor List)
def container fn [[t:MapOrList/t x:Any] [Any] [make t x]]
container Map {a:1}           # returns {a:1}
container List [1,2,3] size   # returns 3
List is MapOrList/t           # returns true
Integer is MapOrList/t        # returns false — and `container Integer …` is a signature error
```

A bare structural bound must be named first (`def MapOrList (Map tor
List)`, then `MapOrList/t`); inline disjunctions in ordinary value
slots (`x:(Integer tor String)`) constrain values as written.

#### Anonymous functions — `=>`

`sig => body` builds an anonymous `Function`: signature on the left,
a single body token on the right. The arrow **groups itself** — it
parses as `(sig afn body)`, binding tighter than the surrounding
call — so it works with or without explicit parens, including as a
`def` operand or a higher-order argument. It closes over enclosing
fn bindings lexically:

```
(x:Integer => [x mul 2]) 5                    # returns 10
filter p:Any => [p.value gt 3] [1 2 3 4 5]    # returns [4 5]
def double x:Integer => [x mul 2]
double 7                                      # returns 14

def make-adder x:Integer => y:Integer => [x add y]
def add5 (make-adder 5)
add5 3                                        # returns 8
```

The signature spellings:

* `x:Integer => …` — bare convenience form, **one** typed param.
* `[x:Integer y:Integer] => …` — the bracket form; required for
  multiple params, value patterns, optionals, and `|` barriers
  (everything `fn`'s input list accepts).
* `[] => …` — zero params.

The body must be **one** token — wrap multi-token bodies as `[…]` or
`(…)`, both captured as code and run per call (a bare-word body like
`=> x` fails; write `=> [x]`). Chained arrows curry right-
associatively, as in `make-adder` above: each inner lambda is
constructed at call time with the outer params bound and captured.
The arrow produces exactly one signature with return type `Any`; for
declared returns or multiple overloads use the full
`fn [[input] [output] [body] …]` form.

Parameter and return annotations may name **any** type — builtins,
and user-defined types introduced with `def NAME refine …`. A value
is accepted at a slot when it is a *member* of the declared type, and
the membership rule is **the same at parameters, returns, and the
`is` word** (one question: `v is T`). How membership is decided
depends on what kind of type `T` is:

**Builtins and structural types — nominal subtyping.** A value
matches when its own type is the declared type or a descendant.

```
def first fn [[xs:List] [Any] [xs get 0]]
first [10 20 30]              # returns 10
```

**Class / Record / Table types — nominal, by construction.** An
instance built with `make` carries the type's tag, so it satisfies
both parameter and return slots of that type (and of any supertype):

```
def Box (class {v:0})
def wrap fn [[n:Integer] [Box] [make Box {v:n}]]
typeof (wrap 5)               # returns Box
(wrap 5) get 'v'              # returns 5
```

**Bare refinement — a *newtype*.** `def Pos (refine Integer)` adds no
predicate; it is a distinct nominal type. A plain `Integer` is **not**
a `Pos` — you construct one explicitly with a typed `def`. The same
strict rule holds at parameters and returns:

```
def Pos (refine Integer)
42 is Pos                                          # returns false
def g fn [[n:Pos] [Integer] [n]]
42 g                                               # returns [aql/signature_error] no matching signature for g
def x:Pos 42   x g                                 # returns 42

def mk fn [[] [Pos] [7]]
mk                                                 # returns [aql/type_error] return value 1: expected Pos got Integer
def mk2 fn [[] [Pos] [def x:Pos 7 x]]
mk2                                                # returns 7
```

**Predicate refinement — a *subset type*.** `def Big (Integer gt 10)`
carves out a subset by a predicate. Any base-type value that
satisfies the predicate is a member — no explicit construction
needed — and the predicate is enforced at parameters **and** returns
alike:

```
def Big (Integer gt 10)
50 is Big                                          # returns true
5  is Big                                          # returns false
def g fn [[n:Big] [Integer] [n]]
50 g                                               # returns 50
5  g                                               # returns [aql/signature_error] no matching signature for g

def mk fn [[] [Big] [50]]
mk                                                 # returns 50
def mkbad fn [[] [Big] [5]]
mkbad                                              # returns [aql/type_error] return value 1: expected Big got Integer
```

The newtype-vs-subset distinction and its cross-language rationale are
explained in **[Explanation: Function signatures](EXPLANATION.md#function-signatures-and-refinement-types)**
and pinned in `design/REFINE-NEWTYPE-VS-SUBSET.10.md`.

#### Recursion and tail calls

Functions recurse freely — a body's reference to its own (or a
not-yet-defined) name resolves at call time, so the standard idioms
need no forward declarations:

```
def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]]
fact 10                                            # returns 3628800
```

**Tail-call elimination is guaranteed.** A call in **tail position**
— the last thing a body does, with nothing of the caller's frame
pending — replaces the caller's frame instead of stacking a new one.
Tail recursion is therefore a real iteration construct: constant
space at any depth, exactly like `for`. Write the accumulator idiom
without a depth budget in mind:

```
def sum fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [sum (n sub 1) (acc add n)]]]
sum 10000 0                                        # returns 50005000 — constant space at any depth
```

The guarantee covers a tail call from fn `f` to fn `g` — self-recursion
(`f` = `g`) and mutual recursion alike — when all of:

* **Nothing pends below the call.** A call whose result feeds a
  waiting word is not a tail call: in `n add (f (n sub 1))` the parked
  `add` still consumes the frame's result, so the frames nest (that
  shape is linear-space by nature).
* **`g` re-binds every name `f`'s frame holds.** Parameters and
  captures cover themselves in self-recursion, and in mutual recursion
  between fns with the same parameter names. A body-local `def` that
  survives to the call keeps the frame alive instead (an enclosing
  frame's bindings stay visible to callees until the frame exits —
  the locally-defined-recursive-fn idiom `def go fn […] go 3` depends
  on exactly that, so such frames must nest).
* **Returns conform.** `g`'s declared returns equal `f`'s, or refine
  them position-wise (trivially true for self-recursion) — or `f`
  declares none.
* **Neither fn is generic.**

A tail call outside these conditions nests as an ordinary call —
results are always identical either way; only the resource profile
differs.

**Runaway taxonomy.** The two non-terminating shapes fail on the
resource they actually consume: an infinite *tail* loop is pure CPU
and trips the step budget (`[aql/evaluation_limit]`); unbounded
*non-tail* recursion grows the evaluation tape and trips its ceiling
(`[aql/tape_exhausted]`). Non-tail recursion is linear time and
linear space in the depth — fine to four-to-five-digit depths under
the default tape ceiling, but prefer a tail accumulator (or `for`)
when the depth is unbounded.

Module functions participate fully: a call into a module crosses the
module boundary once, and recursion inside it is eliminated as above.

### Macros

A **macro** is `fn`'s expand-time sibling: a transformer the engine runs on
its operands **as unevaluated code**, whose returned token list is **spliced
into the call site** in place of the call. Macros add new syntax / control
forms in AQL itself, rather than in Go.

| Word | Description | Example |
|------|-------------|---------|
| `macro` | Create a macro from `[[params] [body]]` | `def unless (macro [[c body] [quote [if unquote c [] unquote body]]])` |
| `unquote` | In a template: insert an operand's **value/form** as one node | `unquote cond` |
| `splice` | In a template: insert a list operand's **elements**, flattened | `splice xs` |
| `gensym` | A fresh, never-colliding atom (a unique name) | `gensym` returns `tmp$g1` |
| `macroexpand` | Expand a macro call to its token list, without running it | `macroexpand (unless x [y])` |

#### Defining and using a macro

`macro [[params] [body]]` mirrors `fn`, with two differences baked in: every
parameter is captured **raw** (the operand arrives as code — a word, literal,
`(paren)`, `[list]`, or `{map}` — never evaluated), and the body runs at
**expansion time** to produce a **template** whose tokens replace the call.

```
def unless (macro [[cond body] [
  quote [ if unquote cond [] unquote body ]
]])

def x 5
unless (x gt 10) [99]                 # returns 99 — body runs: the condition is false
```

`unless (x gt 10) [99]` expands to `if (x gt 10) [] [99]`, which then runs
normally — the condition is evaluated in the *generated* code, exactly as
written, so the body runs only when `x gt 10` is false.

#### The template — `quote`, `unquote`, `splice`

The template is an ordinary `quote [ … ]` region (default-data, the opposite of
AQL's default-eval). Inside it:

* bare tokens are literal code of the expansion;
* **`unquote x`** inserts `x` as **one grouped node** — a bare parameter name
  inserts that operand's captured form; a `(paren)` evaluates and inserts the
  result;
* **`splice xs`** evaluates `xs` to a list and inserts its **elements**,
  flattened into the surrounding sequence.

```
def callit (macro [[f xs] [ quote [ unquote f splice xs ] ]])
def add3 fn [[a:Integer b:Integer c:Integer] [Integer] [a add b add c]]
callit add3 [1 2 3]                   # returns 6 — splice spreads [1 2 3] as 1 2 3
```

A map value that needs an escape must be parenthesised — `{k: (unquote v)}`,
not `{k: unquote v}` (the latter splits into two map entries).

#### Hygiene

Macros are **hygienic by default**: a literal `def <name>` binder in a template
is automatically renamed to a fresh `gensym`, so it can never capture a
same-named variable at the call site — no manual `gensym` needed.

```
def myor (macro [[a b] [ quote [ def tmp unquote a  if tmp [tmp] [unquote b] ] ]])
def tmp 42
myor false tmp                        # returns 42 — the template's `tmp` is renamed; user `tmp` is safe
```

To bind a name the caller *should* see (an intentional, non-hygienic binding),
take the name through `unquote` — `def unquote name …` — so it is user-origin
and left untouched:

```
def defconst (macro [[name val] [ quote [ def unquote name unquote val ] ]])
defconst answer 42  answer            # returns 42
```

`gensym` mints a unique atom directly for hand-written cases; each call is
distinct (`gensym eq gensym` returns `false`).

#### Inspecting and staging

`macroexpand (mac operand…)` returns the **fully expanded** token list as data
(recursively expanding any nested macro calls) without running it — the tool
for seeing what a macro produces. A runaway recursive macro is caught with a
clear error rather than looping.

```
def twice (macro [[e] [ quote [ unquote e add unquote e ] ]])
macroexpand (twice 5)                 # returns [5 word(add) 5]
```

(The result is a *token list*: `add` shows as `word(add)` because it is an
unevaluated word in the expansion, not a call yet.)

Macros are **define-before-use**: a macro must be defined before its call site
is reached (using one earlier raises `undefined_word`). A macro referenced
inside a `fn` body expands when the body *runs*, so it need only be defined
before the call.

> **Deferred.** A `` `[ … ] `` quasiquote sugar (Phase 3) and compiled-mode
> expansion / `eval-when` staging (Phase 5, awaiting the IR backend) are
> designed but not yet shipped. See `design/MACROS.8.md`.

### Control flow

| Word | Description | Example |
|------|-------------|---------|
| `if` | Conditional; else branch optional | `if (5 gt 3) ["y"] ["n"]` |
| `case` | Dispatch on a value: match/block pairs + optional default | `case 2 [1 "one" 2 "two" "many"]` returns `'two'` |
| `for` | Numeric loop (counter or range) | `for 5 [42]` |
| `do` | Evaluate list as program | `do [add 1 2]` returns `3` |
| `error` | Handle an error value (a non-Error result passes through) | `do [1 div 0] error [drop 42]` |
| `raise` | Raise an error (code, message, optional payload) | `raise bad_input "expected a list"` |
| `break` | Exit `for` loop early | `for 10 [break]` |
| `continue` | Skip to next iteration | `for 10 [continue]` |

For `if`, the canonical form is all-forward `if cond [then] [else]`
— this is the form where the argument-order rule places cond into
`args[0]`, then into `args[1]`, else into `args[2]` as the handler
expects. See
**[Tutorial §3](TUTORIAL.md#the-argument-order-rule)**.

`if` coerces its condition to a boolean (the same rule as `convert
boolean`). The values that count as **false** are: `false`, `0` (and
`0.0`), `none`, the empty list `[]`/empty map `{}`, the empty string
`""`, and — as a special case — the exact string `"false"`.
**Everything else is true**, including non-empty strings that look
falsy: `"FALSE"`, `"0"`, and `"no"` are all truthy (only lowercase
`"false"` is special). A condition that produces *no* value at all
(e.g. an empty block `[]` as the condition) is an error, not a false.

#### `for` forms

```
for N [body]              # body runs N times (no iteration index pushed)
for [a, b] [body]         # body runs b-a times
for [a, b, step] [body]   # body runs (b-a)/step times
```

`for` does not push the iteration index onto the body's stack. To
process a sequence with the index/element, use `iota N each [body]`
(each pushes the element before running the body):

```
iota 5 each [dup mul]     # returns [0 1 4 9 16]
```

### List and array words

These are built-in (no import needed): the constructors, basic slicing,
and `flatten`/`size`. The specialised array vocabulary lives in the
[`aql:array-util` module](#the-aqlarray-module) below.

| Word | Description | Example |
|------|-------------|---------|
| `iota` | Generate `[0..N-1]` | `iota 5` returns `[0,1,2,3,4]` |
| `range` | Generate an arithmetic sequence `[start..stop)` | `range 2 6` returns `[2,3,4,5]`; `range 0 10 3` returns `[0,3,6,9]` |
| `take` | First N elements | `[1,2,3,4] take 2` returns `[1,2]` |
| `shed` | Drop first N | `[1,2,3,4] shed 2` returns `[3,4]` |
| `reverse` | Reverse order | `[1,2,3] reverse` returns `[3,2,1]` |
| `flatten` | Remove one nesting level; `flatten N` removes N; `flatten -1` fully flattens | `[[1,2],[3]] flatten` returns `[1,2,3]`; `flatten -1 [1,[2,[3]]]` returns `[1,2,3]` |
import "aql:array-util" | `ArrayUtil.indices` | Index of each needle in the haystack (`-1` when absent). Forward form `indices <needles> <haystack>` — haystack last | `ArrayUtil.indices [20,99,10] [10,20,30]` returns `[1,-1,0]` |
| `size` | Element / key count of a collection — works on any value (see [Size](#size)) | `[1,2,3] size` returns `3` |

A deep flatten is `flatten -1` (the core `flatten` word with a negative
depth) — there is deliberately no `ArrayUtil.flatten` (see
[ADR-001](ADR.md#adr-001)). Substring search is the string-only
`StringUtil.indexof` (`aql:string-util`); the list-membership lookup is
the distinctly-named `ArrayUtil.indices` (`aql:array-util`) — one word per
job, rather than one overloaded name.

### The `aql:array-util` module

The specialised APL-style array vocabulary lives in a built-in module,
imported with `import "aql:array-util"` and reached via the `array.` prefix.
This keeps the global namespace lean (mirroring how `aql:math` gates
`sin`/`cos`/…). Per [ADR-001](ADR.md#adr-001) no name here shadows a core
word: deep flatten stays a core overload (`flatten -1`), and the
list-membership lookup is the distinctly-named `ArrayUtil.indices` rather
than a duplicate of the string word `indexof`. `transpose` has no core
counterpart and so appears here under its plain name.

```
import "aql:array-util"
iota 6 ArrayUtil.reshape [2,3]        # returns [[0 1 2] [3 4 5]]
```

| Word | Description | Example |
|------|-------------|---------|
| `ArrayUtil.shape` | Dimensions of a nested list | `ArrayUtil.shape [[1,2,3],[4,5,6]]` returns `[2,3]` |
| `ArrayUtil.rank` | Number of dimensions | `ArrayUtil.rank [[1,2],[3,4]]` returns `2` |
| `ArrayUtil.reshape` | Change dimensions | `iota 6 ArrayUtil.reshape [2,3]` |
| `ArrayUtil.transpose` | Transpose a rank-2 list | `ArrayUtil.transpose [[1,2],[3,4]]` |
| `ArrayUtil.where` | Indices of truthy elements | `ArrayUtil.where [true,false,true]` returns `[0,2]` |
| `ArrayUtil.grade` | Indices that would sort | `ArrayUtil.grade [3,1,2]` returns `[1,2,0]` |
| `ArrayUtil.at` | Select by index list | `[10,20,30] ArrayUtil.at [2,0]` returns `[30,10]` |
| `ArrayUtil.insert-at` | New list with an element inserted at an index (`insert-at idx elem list`; index `len` appends; out of range errors) | `ArrayUtil.insert-at 1 99 [1,2,3]` returns `[1,99,2,3]` |
| `ArrayUtil.remove-at` | New list with the element at an index removed (out of range errors) | `ArrayUtil.remove-at 1 [1,2,3]` returns `[1,3]` |
| `ArrayUtil.sortby` | Sort by parallel key list | `["b","a","c"] ArrayUtil.sortby [2,1,3]` |
| `ArrayUtil.replicate` | Repeat each element N times | `[1,2,3] ArrayUtil.replicate [2,1,3]` |
| `ArrayUtil.expand` | Expand by Boolean mask | `[1,2,3] ArrayUtil.expand [true,false,true]` |
| `ArrayUtil.compress` | Select elements where a mask is true | `ArrayUtil.compress [true,false,true] [10,20,30]` returns `[10,30]` |
| `ArrayUtil.eachrank` | Apply a body at a given cell rank (0 = scalars, 1 = innermost lists, …) | `ArrayUtil.eachrank 1 [each [add 10]] [[1,2],[3,4]]` returns `[[11,12],[13,14]]` |
| `ArrayUtil.foldaxis` | Reduce a rank-2 list along an axis (0 = columns, 1 = rows) | `ArrayUtil.foldaxis 0 [add] [[1,2],[3,4]]` returns `[4,6]` |
| `ArrayUtil.member` | Per-element membership test | `[1,2,3] ArrayUtil.member [2,3,4]` returns `[true,true,false]` |
| `ArrayUtil.unique` | Remove duplicates | `ArrayUtil.unique [1,2,2,3]` returns `[1,2,3]` |
| `ArrayUtil.indices` | Index of each needle in the haystack (`-1` when absent); haystack is the final argument | `ArrayUtil.indices [20,99,10] [10,20,30]` returns `[1,-1,0]` |
| `ArrayUtil.group` | Group values by parallel keys (or indices by value) | `ArrayUtil.group ["a","b","a"] [1,2,3]` |
| `ArrayUtil.window` | Sliding window of size N | `[1,2,3,4] ArrayUtil.window 2` |
| `ArrayUtil.pairs` | Adjacent pairs | `ArrayUtil.pairs [1,2,3]` returns `[[1,2],[2,3]]` |

### Higher-order array words

| Word | Description | Example |
|------|-------------|---------|
| `each` | Map a function | `[1,2,3] each [dup mul]` |
| `fold` | Reduce with accumulator | `fold [add] [1,2,3] 0` returns `6` |
| `scan` | Running (prefix) fold | `scan [add] [1,2,3]` returns `[1,3,6]` |
| `filter` | Keep elements where a predicate holds | `filter [2 gt] [1,2,3,4]` returns `[3,4]` |
| `outer` | Outer product | `outer [mul] [3,4] [1,2]` |
| `inner` | Inner product | `inner [add] [mul] [3,4] [1,2]` |

These higher-order words follow the **argument-order rule**
(see [Tutorial §3](TUTORIAL.md#the-argument-order-rule)). The
all-forward form shown above maps each list argument left-to-right
into the signature: `fold` takes `body data init`, `scan` takes
`body data`, `outer` takes `body listB listA`, `inner` takes
`combineBody productBody listB listA`.

> **`fold` body stack order.** The body runs with the **accumulator
> pushed first and the current element on top of it**, so a two-arg
> word in the body sees `(element, accumulator)` as `(top, deeper)`:
>
> ```
> 0 fold [add] [1 2 3]      # returns 6
> 0 fold [sub] [10]         # returns -10  — acc minus element (0 - 10)
> [] fold [push] [1 2 3]    # returns [1 2 3] — element pushed onto the acc list
> ```

> **`filter` takes three predicate forms.** A quotation `[body]` runs
> once per element with the element on the stack — exactly like
> `each`/`fold` — and keeps the elements whose result is Boolean
> `true` (a non-Boolean result is an **error**, not a silent drop). A
> receiverless Reach lens keeps elements whose field is true. A
> Function callback over a **list** receives a `{key value}` pair map
> (read the element via `.value`); over a **map** it receives a `KeyVal`
> (read the value via `.v`) and the result keeps the map shape:
>
> ```
> filter [2 gt] [1 2 3 4]                            # returns [3 4]
> filter [2 gt] {a:1 b:5 c:3}                        # returns {b:5 c:3} — maps filter by value
> filter ([p:Any] => [p.value gt 3]) [1 2 3 4 5]     # returns [4 5]      (list: {key value} pair)
> filter ([kv:KeyVal] => [kv.v gt 2]) {a:1 b:5 c:3}  # returns {b:5 c:3}  (map: KeyVal)
> ```
>
> The lens form reads a field: `filter $.active accounts` keeps the
> elements whose `.active` is `true`.

> **Map iteration.** `each`, `for-each`, `fold`, `scan`, and `filter`
> also take a map, iterating its entries in insertion order. The
> quotation form gets each entry's **value** (the key is preserved); a
> lambda gets a `KeyVal` `{k v i n}` — `k` key, `v` value, `i` 0-based
> index, `n` total — so it can use the key/index/total. `each`, `scan`,
> and `filter` keep the map shape, `fold` reduces to one value,
> `for-each` produces nothing. To leave the map and get a list, use
> `keys` / `vals`:
>
> ```
> {a:1 b:2 c:3} each [mul 10]                       # returns {a:10 b:20 c:30}
> {a:1 b:2} each ([kv:KeyVal] => [kv.v add kv.i])   # returns {a:1 b:3}
> fold [add] {a:1 b:2 c:3} 0                        # returns 6
> {a:1 b:2 c:3} scan [add]                          # returns {a:1 b:3 c:6}  (running fold)
> {a:1 b:2 c:3} keys                                # returns ['a' 'b' 'c']
> {a:1 b:2 c:3} vals                                # returns [1 2 3]
> ```

### Size

`size` reports the **natural size** of *any* value as an `Integer`.
Unlike the collection words above — which accept only a concrete
list — `size` has signature `[Any]` and is a **total** function:
every value has a size and `size` never errors. For a list it returns
the element count, so it is the canonical way to ask "how long is this
list?"; it also generalises to maps, strings, numbers, and user types.

```
[10,20,30] size          # returns 3
```

The size of a value is the size of the collection it stands for, by
type:

| Value | Size | Example |
|-------|------|---------|
| List | element count | `[10,20,30] size` returns `3` |
| Map | key count | `{a:1, b:2} size` returns `2` |
| String | length in bytes | `"hello" size` returns `5` |
| Atom | length of the name | `foo/q size` returns `3` |
| Integer / Float | floored magnitude | `42 size` returns `42`, `7.9 size` returns `7` |
| Boolean | `1` for `true`, `0` for `false` | `true size` returns `1` |
| Path | segment count | `(make Path "a/b/c") size` returns `3` |
| class instance / Store / Table | field / entry / row count | `(make Pt {x:1 y:2}) size` returns `2` |
| `None`, a Date, a bare scalar, or any non-concrete value (e.g. a bare type literal) | `0` (never errors) | `None size` returns `0`, `List size` returns `0` |

Dispatch is type-driven: each type contributes its own size rule via
the kernel's `Sizer` capability (`eng.SizeOf`), and a type with no
`Sizer` in its lattice sizes to `0`. There is no separate `length`
word — `size` subsumes it.

### Maps and access

| Word | Description | Example |
|------|-------------|---------|
| `get` / `.` | Lookup field/key, or index a list | `{x:1} . x` returns `1`; `[10,20,30] 0 get` returns `10` |
| `getr` / `!.` | Strict lookup (errors if missing) | `{x:1} !. y` returns `error` |
| `has` | Key/index presence as a Boolean — true when **bound**, even to `none`; total (never raises, `none` parent answers `false`) | `{a:None} has a` returns `true`; `{a:1} has b` returns `false`; `[10,20] has 1` returns `true` |
| `set` | Set a key — in place on Store, class instances, and FlexMap / FlexList (see [Flex nodes](#flex-nodes--flexmap-and-flexlist)); copy-returning on Map | `{a:1} set b 2` returns `{a:1 b:2}`; `set a/q 1 (flex {})` |
| `context` | Push the current context Store | `context` |

> **`set` has two contracts, decided by the receiver's mutability.**
> On the **mutable** containers — Store, class instances, flex nodes — both
> the forward form (`b set flag 1`) and the stack form
> (`b 1 'flag' set`) write into the receiver itself and produce **no
> value**; read the container back to observe the write (so
> `def r (b set k v)` binds nothing). On an **immutable Map**, `set`
> returns a **new map** with the key bound and leaves the receiver
> untouched — the same copy-returning contract as `push`, so calls
> chain:
>
> ```
> {a:1} set b 2                 # returns {a:1 b:2} — new map; the literal is unchanged
> def k 'dyn'
> {a:1} set (k) 2               # returns {a:1 dyn:2} — computed key, like get (k)
> {} set a 1 set b 2            # returns {a:1 b:2} — incremental build chains
> ```
>
> For deep-path updates on plain data, `StructUtil.setpath` remains
> the tool (`{a:{b:1}} StructUtil.setpath "a/b" 2`).

> **`get`/`.` return `none` for anything not found — silently.** A
> missing map key (`{a:1} . b` returns `None`), an out-of-range list index
> (`[10,20,30] 5 get` returns `None`), and a negative index
> (`[10,20,30] -1 get` returns `None` — there is **no** Python-style
> end-indexing) all yield
> `none` rather than an error. Use the strict `getr`/`!.` form when a
> missing key should fail loudly. Note also that `get` indexes **lists
> and maps only** — strings are not indexable (`"hello" 0 get` is a
> signature error), and string length is `StringUtil`-module
> territory, not a base word.

> **A number can't be a reach receiver.** `get`'s receiver is always a
> container (Map / List / Store / class instance / module), so a numeric literal
> before a `.` is a **syntax error**, not a `get` on a number. In
> particular `1.2.3` (a malformed numeric literal), `1 . 2`, and
> `5 . foo` all raise `[aql/syntax_error]: a number has no members`. A
> plain `Float` like `5.0` is unaffected.

> **A bare word key is a *literal* name — like JavaScript `.key`. Wrap a
> variable (or any expression) in parens to use its *value* as the key —
> like `[expr]`.** `()` is to AQL what `[]` is to JS member access:
>
> | JavaScript | AQL | meaning |
> |------------|-----|---------|
> | `xs.i`     | `xs dot i` or `xs.i` | literal key/index named `i` |
> | `xs[i]`    | `xs get i`           | computed — the **value** of `i` |
>
> ```
> def xs [10 20 30]
> def i 1
> xs dot i          # returns None — literal key "i", absent (like xs.i)
> xs get i          # returns 20   — i evaluates to 1 (like xs[i])
> xs get 1          # returns 20   — literal index
> ```
>
> `dot` (and its `.` sugar) QUOTES a bare word as a literal key, so a
> same-named variable is ignored and an *undefined* bare key still reads
> `None` rather than raising — exactly as `xs.i` reads a missing property as
> `undefined` in JS. `get` EVALUATES its key, so `get i` uses the value of
> `i`; a bare word `get` needs that word to be bound or it is an
> `undefined word`. Reach for `get` (or `dot (…)`-with-a-paren) whenever the
> key/index is computed.

**Dotted access binds tightly.** A `.`/`!.` chain groups to a single
`( … )` so it binds to its immediate receiver, not to a surrounding call:
`size m.x` means `size (m.x)`, and `a.b.c` is `( a dot b dot c )`. Two
consequences:

- **Access the result of a call** by parenthesising the call:
  `(make Point {x:1 y:2}) .x`, `(import "data.json") . name` — bare
  `make … {} .x` would feed `.x`'s result *into* `make`.
- **`/r` is a pure reference to a *function word*: it resolves the name to
  the bound function and *advances the pointer* — it never calls the
  function in place.** `/r` is legal **only** for function words; a name
  bound to a non-function value (a plain value, a type) raises
  `[aql/illegal_ref]`, because a bare value name already pushes its value
  — there is no call/value asymmetry for `/r` to break. The same rule
  applies to the `ref` word. The reference holds at any arity and in any
  position (top level, list element, paren, `do`-block, map value): `g/r`
  yields the function as data, `add/r 2 3` leaves `[Function, 2, 3]` on
  the stack (the args are *not* consumed), and `[zero/r]` is
  `[<function>]` even for a 0-arg `zero` (it is **not** fired). To
  **call** a referenced function, use the bare word (`add 2 3`), `apply`
  it (`2 3 (quote (f/r)) apply`), or access it as a member (below), where
  `get` brings the value live.

- **A function stored in a plain map** is callable via dot. Store it with
  `/r` — `{fn: myfn/r}` — which holds the function as data; then
  `m.fn arg` retrieves it (via `get`) and the arg calls it. Stored *bare*
  (`{fn: myfn}`) the map value is auto-evaluated: `myfn` is dispatched
  0-arg, which fails if it needs arguments — so a bare entry like
  `{fn: myfn}` is a **build error** (bare words never degrade to data;
  use `/r` for a callable data value, or `/q` for an atom). **Module
  functions are exported the same way** — `export "m" {fn: fn/r}` (a
  bare `{fn: fn}` export errors for the same reason). The distinction is
  whether the value is *brought live*: `/r` itself holds; member access
  (`get`) and bare words dispatch.

### Flex nodes — FlexMap and FlexList

`Map` and `List` are immutable: the mutation words return new copies and
`set` has no Map/List signatures at all. **FlexMap** (`Node/Map/FlexMap`)
and **FlexList** (`Node/List/FlexList`) are their *mutable* child types:
`set` and the list-mutation family change the node **in place**, while
everything else — `get`/`.`, `each`, `sort`, `is`, `typeof`, `deq`,
dispatch into `Map`/`List` signature slots — is inherited unchanged.

| Word | Description | Example |
|------|-------------|---------|
| `flex` | Deep mutable copy of any Node: Map→FlexMap, List→FlexList, nested Nodes converted recursively | `flex {a:1}`; `flex [1 2]` |
| `node` | The inverse: deep immutable conversion (identity when nothing inside is flex) | `node (flex {a:1})` returns `{a:1}` |
| `make FlexMap m` / `make FlexList l` | Same conversions via the universal constructor; `make Map v` / `make List v` are the inverse (and work symmetrically on plain literals) | `make FlexMap {a:1}`; `make Map (flex {a:1})` |
| `set key val flexmap` | In-place key set (atom or string key); returns the node | `set b/q 2 (flex {a:1})` returns `(flex {a:1 b:2})` |
| `set idx val flexlist` | In-place index set, `0..len-1` only — out-of-range errors (**sparse FlexLists are an error**; use `append` to grow) | `set 1 99 (flex [1 2 3])` |
| `append` | Grow a FlexList in place: a list argument concatenates its elements; anything else appends as one element; wrap to append a list *as* an element | `append 4 fl`; `append [3 4] fl`; `append [[3 4]] fl` |
| `push` / `pop` / `unshift` / `shift` | On a FlexList: mutate in place and return the node (`pop`/`shift` also return the removed element). On a plain List: unchanged copy semantics | `push 3 (flex [1 2])`; `pop (flex [1 2])` returns `(flex [1]) 2` |

Key semantics (full design note: `design/FLEX-NODES.10.md`):

- **Conversions deep-copy.** `flex m` never aliases `m`'s entries, so
  mutating the flex copy cannot leak into the immutable source; `node f`
  snapshots, so later mutation of `f` cannot reach the result.
- **Mutators return the node** — calls chain:
  `set c/q 3 (set b/q 2 (flex {a:1}))`.
- **Bindings share the node.** `def g f` aliases the same container;
  mutation through either is visible through both (this is reference
  semantics, like Store and class instances).
- **Writes adopt — trees stay entirely one column.** A plain map/list
  stored into a flex container (`set`, `push`, `unshift`, `append`) is
  deep-flexed on the way in, so a flex tree is mutable at every depth
  (`set a {b:1} f` then `set b 9 f.a` sticks — it was silently lost
  when the inner stayed immutable). A flex value stored into a flex
  container stays a **shared handle** (what you pass explicitly is
  shared). Symmetrically, a flex value stored into a plain `Map`/`List`
  is snapshot to its immutable shape, so an immutable container can
  never change underneath through a live handle.
- **Equality ignores flexness**: `(flex {a:1}) deq {a:1}` is `true` and
  `cmp` orders flex/plain pairs by content. `eq` remains container
  identity (two separate `flex [1]` nodes are not `eq`).
- **`is` is nominal-subtype**: `(flex {}) is Map` and `… is FlexMap` are
  both true; `{} is FlexMap` is false.
- **Non-mutating words return plain nodes** (by design): `sort`, `each`,
  `filter`, `unify` on a flex input produce ordinary `List`/`Map` results.
- Canon renders flex nodes round-trippable: `(flex {a:1})`.

`FlexList` is a *Node* — it participates in everything List does
(each/sort/format/unify/`deq`, every `List` signature slot), grows and
shrinks in place, compares by content, and graduates to an immutable
List via `node`. It is the mutable list: build structural data
incrementally and flow it through the list words, then freeze with
`node`. See `design/FLEX-NODES.10.md`.

**Record and Table.** Both are Ideal type *descriptors*; their `make`
instances are plain immutable Map / List-of-Map shapes. Flex nodes give
the sanctioned mutable escape: `flex (make R {…})` is a mutable copy
(unvalidated while flex — mutation is unchecked), and `make R (node f)`
converts back through validation.

### Type words

| Word | Description | Example |
|------|-------------|---------|
| `typeof` | Type of a value (single Parent hop) | `typeof 42` returns `Integer` |
| `pathof` | Ancestry path (root first, leaf last) | `pathof Integer` |
| `is` | Type-compatibility test | `42 is Number` returns `true` |
| `convert` | Parse/serialise a scalar to a type | `convert Integer "42"` returns `42` |
| `base` | Zero / base value for a type | `base Integer` returns `0` |
| `refine` | Build a refinement of a base type | `class {count:0}` |
| `make` | Construct typed value or instance | `make Point [1 2]` |
| `gen` | Declare type parameters for the next constructor | `def Box gen [T] class {value:T}` |
| `of` | Instantiate a generic schema | `Box of [Integer]` |
| `extends` | Bound a parameter inside a `gen` entry | `gen [(T extends Number)]` |
| `default` | Default a parameter inside a `gen` entry | `gen [T (E default Error)]` |

> **`convert` parses text; it does not re-bucket numbers.** It turns a
> `String` into a number (`convert Integer "42"` returns `42`) or a number
> into text, but it will **not** move a value between the `Integer`
> and `Float` nodes: `3.9 convert Integer` and even `3.0 convert
> Integer` both error. To go from `Float` to `Integer`, use a
> rounding word (`MathUtil.floor`, `MathUtil.round`, `MathUtil.trunc`
> from `aql:math-util`). Note `make` is more permissive than
> `convert`: a `:Number` record field accepts a numeric **string** and
> coerces it (`make Point ["1" "2"]` returns `{x:1 y:2}`).

Named types are introduced by pairing `def` with a `refine`
expression: `def Point refine Record [x:Number y:Number]`,
`def Counter class {count: 0}`, `def Inventory refine Table
Row`. See **[HOWTO: Define a record/table/object type](HOWTO.md#define-a-record-type)**.

### Inspection

| Word | Description |
|------|-------------|
| `inspect` | Structured view of a value, word, or type |
| `canon` | Render a value as canonical AQL source (a String) |
| `trace` | Evaluate a list with step-by-step tracing |

**`canon` is round-trippable for data.** The result is the same
canonical rendering the executable spec compares against, and for data
values — scalars, atoms (`foo/q`), `none`, type literals, paths, and
plain or flex Node trees — evaluating it reproduces an equivalent
value. Flexness is marked at every level (`canon (flex {a:[1]})`
returns `'(flex {a:(flex [1])})'`), strings pick a quoting that
re-parses exactly (`canon "it's"` returns `"it's"` double-quoted, and
content with both quote kinds or backslashes is escaped), and a plain
list renders bare. Identity-bearing values — `Store`, class
instances, functions, timers — still render, but rebuilding them from
the source produces a *fresh* container: data round-trips, identity
does not.

### I/O

| Word | Description | Example |
|------|-------------|---------|
| `print` | Print with newline | `print "hello"` |
| `printstr` | Print without newline | `printstr "hi"` |
| `read` | Read a file or stdin | `read "data.json"` |
| `write` | Write a file or std stream | `write "out.txt" "hi"` |
| `stdin`, `stdout`, `stderr` | Pseudo-paths | `read stdin` |

`read` and `write` accept an Options map with `fmt: 'json | 'csv |
'tsv | 'jsonic | 'text` to force a format; otherwise the extension
decides.

### Networking — `fetch`

| Word | Description |
|------|-------------|
| `fetch` | HTTP request, returns a `Response` |

```
fetch "https://api.example.com/v1"
fetch "https://api.example.com/v1" {
  method:  'post,
  headers: {Authorization: "Bearer x"},
  body:    {x: 1}
}
```

`Response` is an Ideal with `status`, `headers`, `body` fields.
Requires the `fetch` capability.

### SQLite

| Word | Description |
|------|-------------|
| `sqlite-open` | Open or create a database |
| `sqlite-close` | Close a database |
| `sqlite-exec` | Execute statement(s) (no rows expected) |
| `sqlite-query` | Run a query, return rows as a list of maps |
| `sqlite-tx` | Run a list inside a transaction |

```
def db sqlite-open "data.db"
db sqlite-exec "CREATE TABLE t (id INTEGER, n TEXT)"
db sqlite-query "SELECT * FROM t"
```

Requires the `sqlite` capability.

### Modules

| Word | Description | Example |
|------|-------------|---------|
| `module` | Define a module inline | `module [def x 1]` |
| `import` | Import a module by name or file | `import "lib.aql"` |

`import` binds each `export "Name" {…}` to a **`ModuleExport`** instance.
A `ModuleExport` is *transparent* — `MathUtil.sqrt 16.0` still calls the
exported function — and carries two synthetic names: `Name.$name` (the
export name) and `Name.$module`, the **`Module`** descriptor it belongs
to. A `Module` (`Ideal/Module`) has fields `id`, `kind`
(`native`/`file`/`inline`), `file`, `folder`, and `exports`:

<!-- aql-test: skip -->
```
import aql:math
typeof Math                   # returns ModuleExport
MathUtil.$name                    # returns 'Math'
MathUtil.$module.id               # returns 'aql:math'
MathUtil.$module.kind             # returns 'native'
MathUtil.$module.exports          # returns ['Math']
```

<!-- aql-test: skip -->
```
import utils [def f [dup add]]
utils.f 3                     # returns 6

import aql:time-util

import [helper as h] "lib/utils.aql"
```

### Concurrency

| Word | Signature | Description |
|------|-----------|-------------|
| `now` | `[] → [Instant]` | Current UTC instant |
| `sleep` | `[Integer] → []` | Pause for N milliseconds |
| `timeout` | `[Integer, List] → [Timeout]` | Schedule one-shot callback |
| `interval` | `[Integer, List] → [Interval]` | Schedule repeating callback |
| `cancel` | `[Timeout|Interval] → []` | Cancel a timer |
| `await` | `[Options?, List] → [List|Any]` | Run blocks in parallel |

Await modes (passed as `{mode: 'atom}` in the Options map):

| Mode | Behaviour | JS equivalent |
|------|-----------|---------------|
| `'all` (default) | All must succeed; first error fails | `Promise.all` |
| `'full` | All complete, results carry `{status,value}` | `Promise.allSettled` |
| `'first` | First to complete wins | `Promise.race` |
| `'any` | First non-error wins | `Promise.any` |

### Unification

| Word | Description |
|------|-------------|
| `unify` | Unify two values; returns result and Boolean |

```
1 unify Number                       # returns 1 true
1 unify "x"                          # returns '~unify-fail' false
refine Record [x:Number] unify {x:1}  # returns '~unify-fail' false — records ≠ maps
```

### Help

| Word | Description |
|------|-------------|
| `help` | Print a language overview and how to use `describe` |
| `describe` | Document a word: signatures, examples, and notes (e.g. `describe add`) |

At the command line, `aql help` documents the CLI and its subcommands,
while `aql describe [word\|module]` documents the language. In the REPL,
`/help` prints the overview and `/describe <word>` looks one up.


## Built-in modules

Built-in modules ship with the binary but are not auto-loaded —
`import aql:xxx` to enable. Each binds one capital-initial namespace
(`import "aql:math-util"` → `MathUtil.sqrt`). The `-util` suffix marks
a utility library of pure helper functions; capability / framework
modules keep plain names.

| Module | Namespace | What's inside |
|--------|-----------|---------------|
| `aql:math-util` | `MathUtil` | Extended numerics: trig, statistics, special functions, IEEE-754 classifiers. |
| `aql:array-util` | `ArrayUtil` | Specialised APL-style array vocabulary (see above). |
| `aql:string-util` | `StringUtil` | String words — `upper`, `split`, `indexof` (haystack-last), `replace`, … all subject-last. |
| `aql:struct-util` | `StructUtil` | voxgig-struct data words — `clone`, `getpath`, `setpath`, `merge`, `walk`, `transform`, `jsonify`, … |
| `aql:bin-util` | `BinUtil` | Bitwise ops plus `popcount`, `clz`, `ctz`, `bitlen`, `fnv32`/`fnv64`, and `base64`/`hex` `-encode`/`-decode`. |
| `aql:logic-util` | `LogicUtil` | Derived boolean connectives — `nand`, `nor`, `xnor`, `iff`, `implies`. |
| `aql:type-util` | `TypeUtil` | Type utilities — `tpartial`, … |
| `aql:time-util` | `TimeUtil` | `now`, `parse`, `format`, `add`, `diff`, `date`, `datetime`, `instant`, `timeofday`, `duration`, `timezone`, timers. |
| `aql:matrix-util` | `MatrixUtil` | Tensor / Matrix / Vector types and linear algebra. |
| `aql:io` | `IO` | File and stream I/O — `read`, `write`, `stdin`, `stdout`, `trace` (only `print` stays in core). |
| `aql:net` | `Net` | HTTP / API words — `fetch`, `prepare`, `direct`. |
| `aql:test` | `Test`, `Assert` | Unit tests, declarative specs, property-based testing. |
| `aql:rand` | `Rand` | Seeded random generators (drives `Test.check-prop`). |
| `aql:query` | `Query` | SQL-flavoured query pipeline. |
| `aql:report` | `Report` | Tabular result reporting. |
| `aql:vm` | `Vm` | Run AQL source in-memory — `run`, `run-with`, `run-sandbox`, `run-compute`. |

> **`StructUtil.merge` is a deep, index-wise merge** — lists merge
> element-by-element (`{kids:[99]} {kids:[10,20]} StructUtil.merge`
> returns `{kids:[99,20]}`), so using it as a one-field update can fuse
> sibling structures. For a single-field edit, use the copy-returning
> `StructUtil.setpath`: `{a:1,b:2} StructUtil.setpath "b" 3` returns
> `{a:1, b:3}` (deep paths work too: `setpath "a/b/c" v`).


## Error codes

Errors are values of type `Ideal/Error` with a `code` atom and a
`message` string. Raise your own with `raise`:

```
raise "boom"                          # code user_error
raise bad_input "expected a list"     # a bare word names the code
raise {code: bad_input/q, message: "expected a list", got: 42}
```

The map form's extra keys ride along on the Error value: a handler
reads `e.code`, `e.message`, and any payload keys (`e.got`), and
`convert Map e` projects them all. Common codes:

| Code | Meaning |
|------|---------|
| `undefined_word` | A bare name was used outside a quoted slot. |
| `user_error` | Default code for `raise "message"`. |
| `def_error` | `def`'s value expression produced no value to bind. |
| `no_value_error` | A parenthesised argument expression produced no value for a call. |
| `uncalled_function` | A call matched no signature and its function value was never consumed. |
| `reserved_word` | `def`/`undef` targeted a built-in word's value binding, a sealed word (`def`/`make`/`word`), or the literal `true`/`false`/`none`. |
| `locked_signature` | A word extension's signature tuple exactly matches a locked (built-in) signature — locked signatures can never be replaced. |
| `extend_conflict` | Two different modules transplanted the same signature tuple onto one word at import. |
| `extend_user_type` | A module extended a core word with a builtin-only argument tuple — at least one user-minted type (`refine` / `class`) is required per signature; kernel and `aql:`-module types don't qualify. |
| `constraint_violation` | A generic type argument does not satisfy its `extends` bound. |
| `arity_mismatch` | `of` received the wrong number of type arguments (defaults fill only the tail). |
| `unbound_param` | A generic parameter could not be inferred and has no default. |
| `gen_without_constructor` | A `gen [...]` was never consumed by a type constructor. |
| `incomparable` | `cmp`/`lt`/`lte`/`gt`/`gte` got cross-family operands — use `tcmp`. |
| `type_mismatch` | A value didn't match an expected signature slot. |
| `arity_mismatch` | Wrong number of arguments. |
| `div_zero` | Division by zero. |
| `out_of_range` | Index or numeric value outside the legal range. |
| `unify_fail` | Two values cannot unify. |
| `not_found` | Strict lookup (`!.`, `getr`) found no key. |
| `io_error` | File I/O failed. |
| `cap_denied` | Operation needed a capability that wasn't enabled. |
| `cancelled` | Operation cancelled (timer, await branch). |

Use `do [...] error [...]` to catch them — a successful body skips
the handler and its value passes through. The error branch is
**stack-neutral**: it leaves exactly the handler's result, like the
success path. Inside the handler the caught Error is on the stack —
bind it with `var [[e] …]`, read fields with `get`, or ignore it (an
unconsumed error is dropped, not leaked beneath the result); an empty
handler `error []` passes the Error through as the result. Dispatch
on the code with `case`:

```
def attempt fn [[x:Integer] [Any] [
  do [risky x] error [
    dot code
    case [
      bad_input/q  "rejected: bad input"
      too_big/q    "rejected: too large"
      "unexpected failure"
    ]
  ]
]]
```

A match may also be a predicate over the whole Error (`[dot code eq
bad_input/q] [dot message]`), with the matched block reading the
error's fields — the value is on the stack for both, exactly like
the `error` handler itself.


## Capabilities

A capability is a runtime feature flag that gates side-effecting
words. The defaults match the CLI; embeddings (Wasm, library) may
disable any of them.

| Capability | Words it gates |
|------------|----------------|
| `fileio` | `read`, `write` |
| `fetch` | `fetch` |
| `sqlite` | `sqlite-*` |
| `timers` | `timeout`, `interval`, `sleep` |
| `subprocess` | (reserved) |
| `vault` | secret lookup via vault |

Words attempting to use a disabled capability raise an
`Error{code:'cap_denied}`.


## CLI reference

This section is a compact index of the `aql` binary. It is generated
from the same source as the live help — `aql help` lists the
subcommands and `aql help <subcommand>` summarises one — and the full
operational documentation, with every flag and worked examples, is
[CLI.md](CLI.md). For the *language* (words, categories, modules) use
`aql describe` instead; see [AGENTS.md](AGENTS.md) for the
`help` (tool) vs `describe` (language) split.

### Subcommands

One-shot commands (run and exit):

| Subcommand | Purpose | Key flags |
|------------|---------|-----------|
| `run` / `aql [script]` | Execute a script, `-e` expression, or (no args) the REPL | `-e`, `-check`, `-compile`, `-options`, `--perms`, `--allow`/`--deny` |
| `do <words…>` | Evaluate the arguments as one AQL expression and print the result | `--perms`, `--allow`/`--deny`, `--compile` |
| `check [script]` | Static type-check; print diagnostics | `--json`, `--soft`, `--emit`, `-e` |
| `fmt [file…]` | Format `.aql` files in place (whole tree if no args) | — |
| `describe [name]` | Document a word, category, or module (the *language*) | — |
| `help [subcommand]` | CLI usage, or one subcommand's summary | — |
| `prep [dir]` | Parse `aql.jsonic` → `.aql/aql.json` | — |
| `pack [dir]` | Build a publishable module zip (runs `prep` first) | — |
| `clean [dir]` | Delete `.aql/*` except dotfiles | — |
| `install <name>-x.y.z` | Download and install a module from a registry | `-r <url>` |
| `register` | Create an account on a registry | `-r <url>` |
| `login` | Log in to a registry; store the token | `-r <url>`, `--vault`, `--vault-alias` |
| `publish [dir]` | Pack and upload the current module | `-r <url>`, `--vault`, `--vault-alias` |
| `vault <mode>` | Manage the local key vault (see below) | `--folder`, `--suffix`, `-i` |
| `policy <op>` | Inspect permission profiles: `list`, `show`, `validate`, `test`, `explain` | — |
| `ctl <op> [svc]` | Control a running `aql serve`: `status`, `info`, `pause`, `resume`, `stop` | `--api`, `--token` |

Long-running services (stay up; composable under `serve`):

| Service | Purpose | Key flags |
|---------|---------|-----------|
| `repl` | Interactive read-eval-print loop (also bare `aql`) | `-r` |
| `registry` | Serve modules + auth endpoints over HTTP | `-r <folder>`, `-p <port>` |
| `lsp` | Language Server over stdio (default) or TCP | `-p <port>`, `-host` |
| `exec` | HTTP code-execution endpoint (`POST /v1/exec`) | `-bind`, `-p`, `-r`, `--perms` |
| `serve <svc> [+ <svc>…]` | Run one or more services in one process | `-c/--config <file>` |
| `tui` | Terminal UI driven by a running supervisor's `api` | `--api`, `--token` |

### Vault modes

`aql vault [--folder=PATH] [--suffix=NAME] <mode> [args…]`, or
`aql vault -i` for the interactive TUI. Passphrases are prompted
hidden; set `AQL_VAULT_PASSPHRASE` only for non-interactive use.
Backends: `auto` (default), `keychain`, `secret-service`, `wincred`,
`file`, `1password`.

| Mode | Purpose | Notable flags |
|------|---------|---------------|
| `init` | Initialise the vault; choose a backend | `--backend`, `--force` |
| `status` | Print backend, secret count, lock state, generation | — |
| `add [ns:]<alias>` | Store a secret | `--from-stdin`/`--from-env`/`--from-clipboard`, `--provider`, `--namespace`, `--expiry`, `-y` |
| `get <alias>` | Retrieve a secret (redacted unless `--reveal`) | `--reveal` |
| `list` | List aliases and metadata (no values) | `--namespace` |
| `expiry` | List/set/clear expiry reminders | `--namespace`, `--within` |
| `rm <alias>` | Remove a secret | `-y` |
| `mv <from> <to>` | Rename a key, move namespaces, or rename a namespace (`ns:`); capabilities re-bind to the new name by default | `--revoke-caps`, `--dry-run` |
| `rotate <alias>` | Replace a secret's value | `--from-stdin`/…, `--revoke-caps`, `--expiry`, `-y` |
| `verify` | Reconcile store and keyring (alias `fsck`) | `--prune` |
| `import <file>` | Import from a `.env` file or an encrypted bundle | — |
| `export` | Export a portable, passphrase-encrypted bundle | `--out`, `--namespace` |
| `grant <alias>` | Mint a scoped capability token (shown once) | `--agent`, `--hosts`, `--methods`, `--ttl`, `--max-calls`, `--max-cost-cents`, `--require-approval` |
| `revoke <token-id>` | Revoke a capability token | — |
| `lock` / `unlock` | Block / re-enable `get`/`grant` | — |
| `config` | View or set config (e.g. `namespace.default`, `journal.keep`) | `--set k=v`, `--unset k` |
| `password <verb>` | Scoped passwords (keyslots): `add`/`assign`/`set`/`rm`/`list` | `--scope`, `--namespaces`, `--rekey`, `-y` |
| `policy` | Declaratively `apply`/`show` aliases + capabilities | — |
| `proxy` | Local credential broker (HTTP) | `--listen`, `--allow-public` |
| `mcp` | Stdio MCP server exposing aliases as tools | `--agent` |
| `exec … -- <cmd>` | Run a command with secrets injected as env vars | `--for=RECIPE[=alias]` (repeatable), `--registry`, `--dry-run`, `--upper`, `--prefix`, `--clear-env` |
| `providers` | List built-in provider presets | — |
| `folder` | List known vaults; `folder add <dir>` registers one | — |
| `scan [paths…]` | Scan file contents for secret-like strings | `--home`, `--match-vault`, `--quiet`, `--max-bytes` |
| `audit` | Show the structured audit log | `--tail`, `--json` |
| `history` | Show the content-revision history | `--limit` |
| `restore <gen>` | Restore vault metadata to a past generation (admin) | `--list`, `--dry-run`, `-y` |

### Permission flags

Shared by every subcommand that runs user code (`run`, `do`, `exec`,
and the REPL). A *policy* is a set of allow/deny rules over
`scope.op` pairs; profiles are named bundles. See
[CLI.md → Permissions](CLI.md#permissions).

| Flag | Meaning |
|------|---------|
| `--perms <p>` | Policy: a profile name, a file path, or inline jsonic (auto-detected) |
| `--perms-file <path>` | Policy file path (explicit) |
| `--perms-inline <jsonic>` | Inline policy (explicit; `@-` = stdin, `@path` = file) |
| `--allow <scope.op>` | Add an allow rule (repeatable) |
| `--deny <scope.op>` | Add a deny rule (repeatable) |
| `--allow-global <cap>` | Raise a global hard cap (repeatable) |
| `--deny-global <cap>` | Lower a global hard cap (repeatable) |
| `--policy-dry-run` | Observe-only: log what the policy would do, allow every call |

Environment fallbacks: `AQL_POLICY`, `AQL_POLICY_FILE`. Bytecode
compilation: `-compile` / `AQL_COMPILE` enable it, `AQL_NO_COMPILE`
disables.

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Error (parse, type-check failure, runtime error, I/O, usage) |
| `2` | `vault scan` only: findings were reported |

A child process run under `vault exec` propagates its own exit code.
