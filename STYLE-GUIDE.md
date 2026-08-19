# boru Style Guide

House style for boru source. Where a rule is enforced by `boru fmt`, this
guide says so; where it is not (yet), it says that too, and the guide is
the authority until the formatter catches up.

Style is not taste here. boru's surface deliberately admits several
spellings of the same thing — that is what makes forward arguments and
the signature sugar possible — so a house style is what keeps a codebase
from carrying five spellings of one idea.

Related: [AGENTS.md](AGENTS.md) for the doc map and the tooling,
[REFERENCE.md](REFERENCE.md) for what each form *means*,
[NUR.md](NUR.md) for the recorded non-uniformities.


## S1 — Function signatures: use the fewest square brackets

**Rule.** Write a signature in the spelling with the fewest square
brackets that expresses it. `boru fmt` normalises toward this form.

For the common case — **one parameter, one return** — that is the
three-argument form with a bare input and a bare output:

```boru
def double fn x:Integer Integer [mul 2 x]        ;# YES — 1 bracket pair
```

Not any of these, which are the same function written longer:

```boru
def double fn x:Integer [Integer] [mul 2 x]      ;# no — 2
def double fn [x:Integer Integer [mul 2 x]]      ;# no — 2
def double fn [[x:Integer] Integer [mul 2 x]]    ;# no — 3
def double fn [x:Integer [Integer] [mul 2 x]]    ;# no — 3
def double fn [[x:Integer] [Integer] [mul 2 x]]  ;# no — 4
```

All six are the *same value*, not merely the same answer — `canon` of
each is `deq` to `canon` of any other. The body's brackets are not
optional: the body is always a list.

### The one spelling that is not valid

A **bracketed input** is not a longer way of writing a bare one. It
selects the other form entirely:

```boru
def double fn [x:Integer] [Integer] [mul 2 x]
;# error: [boru/fn_error]: fn: list length must be a non-zero multiple of 3
```

`fn` has two signatures — `fn <input> <output> <body>` (three arguments)
and `fn [ …triples… ]` (one list). A **list** in the input position
always selects the second, so `[x:Integer]` is read as a spec list of
length 1, and `[Integer]` and `[mul 2 x]` are left stranded as separate
arguments. `boru describe fn` states the rule: *"The 3-arg form requires
a NON-LIST input … a list input always selects the spec-list form."*

So the input is bare or the whole signature is a list. Never one bracket
pair around the input alone.

### When you cannot reduce it

The three-argument form takes exactly one input item, so these keep the
spec-list form — that is correct style, not a violation:

```boru
def add2 fn [[a:Integer b:Integer] [Integer] [add a b]]   ;# 2+ parameters
def zero fn [[] [Integer] [42]]                            ;# 0 parameters — [] is a list
def m fn [[a:Integer] [Integer] [add 1 a]
          [s:String]  [String]  [s]]                       ;# overloads
```

For overloads, the flat triple spelling is available and shorter when
every input is a single parameter:

```boru
def m fn [a:Integer Integer [add 1 a] s:String String [s]]  ;# same function
```

Prefer whichever of those two reads better for the arity you have: flat
for a few one-parameter overloads, bracketed when the inputs are wide
enough that the triple boundaries stop being obvious.

### Why fewest brackets

Three reasons, in order of weight:

1. **One spelling per idea.** Six ways to write one signature is six
   things a reader has to recognise as the same thing.
2. **The brackets carry no information here.** `[x:Integer]` and
   `x:Integer` in the input slot denote the same single parameter; the
   extra pair is noise that looks like structure.
3. **It matches the formatter.** `boru fmt` already rewrites the fully
   canonical form to the short one, so the short form is the fixed point
   the tool is heading toward.

### Formatter status (as of 2026-08-19)

`boru fmt` implements this rule **only from the fully canonical
spelling**:

| input | `boru fmt` output |
|---|---|
| `fn [[x:Integer] [Integer] [mul 2 x]]` | `fn x:Integer Integer [mul 2 x]` ✅ |
| `fn [[Integer] [Integer] [add 1]]` | `fn Integer Integer [add 1]` ✅ |
| `fn x:Integer [Integer] [mul 2 x]` | unchanged ❌ |
| `fn [x:Integer Integer [mul 2 x]]` | unchanged ❌ |
| `fn [[x:Integer] Integer [mul 2 x]]` | unchanged ❌ |
| `fn [x:Integer [Integer] [mul 2 x]]` | unchanged ❌ |

It correctly leaves the irreducible forms alone (2+ parameters, zero
parameters, multi-overload spec lists).

So `boru fmt` does not yet make this rule self-enforcing: four of the six
spellings survive it untouched, and a file can pass `fmt` while carrying
several spellings of one signature. Recorded as **NUR088**. Until that
lands, apply S1 by hand — running `fmt` is necessary but not sufficient.


## S2 — Forward call form is canonical

Write `f a b c`, not the mirror-equivalent stack forms, in new code and
examples (`AGENTS.md`).

**Mind the operand order when you convert.** Forward and swap are *not*
the same expression for a non-commutative word:

```
$ boru do 'sub 10 3'    →   -7        $ boru do '10 sub 3'   →    7
$ boru do 'lte 5 1'     →   true      $ boru do '5 lte 1'    →   false
```

`lte x y` means `y <= x`; `sub a b` means `b - a`. The all-stack form
`10 3 sub` is the one that agrees with `10 sub 3` (README.md). So the
forward spelling of "n ≤ 1" is `lte 1 n`, and of "n − 1" is `sub 1 n`.

Transcribing a guard from infix habit inverts it silently: a factorial
written `if (lte n 1) [1] [...]` returns `1` for every input, exit 0, no
diagnostic. **Re-run examples after converting them** — this is not a
formatting change, it is an edit to the expression.


## S3 — Passing a function: use `/r`

A bare name bound to a function **calls** it (ADR-011). To pass the
function itself, write `/r`:

```boru
each [f] xs        ;# a quotation naming f
hof f/r xs         ;# f as an argument
```

A bare `f` before a `Function`-typed slot happens to resolve as a
reference today, but that exception is struck from ADR-011 and is
scheduled for removal (**NUR078**). Write `/r` now.


## S4 — Reading a parameter whose kind you do not know

`[p]` calls a function-bound parameter; `p/r` refuses a non-function one.
Neither is total. When a parameter may hold either, read it positionally:

```boru
def ident fn t:Any Any [(args).0]
```

For an *enclosing* function's parameter, box it first — `args` is the
innermost function's own argument list:

```boru
def ctrue fn t:Any Function [ def bx [(args).0] ( fn f:Any Any [ bx.0 ] ) ]
```

Recorded as **NUR085**; see
[`design/HIGHER-ORDER-FUNCTIONS.0.md`](design/HIGHER-ORDER-FUNCTIONS.0.md)
§4 for the full idiom sheet.


## S5 — Computed keys: parenthesise for the quoting accessors

`get` evaluates a bare bound key; `has` and `set` quote it. So:

```boru
m get k          ;# fine — get evaluates k
m has (k)        ;# needed — a bare k looks up the literal "k"
m set (k) v      ;# needed
```

A bare key to `has`/`set` is a silent wrong answer, not an error
(NUR040's class).
