# Conversion notes: Go → TypeScript port of aqleng

This document is the audit trail from porting the Go engine to TypeScript
under `aqleng/ts/`. The TS port covers the subset the existing TSV
specs exercise (Value/Type lattice, Registry+capability, native
dispatch, signature matching, error paths) and runs the SAME
`aqleng/test/spec/*.tsv` files via Node 24's built-in `node:test`
runner with `--experimental-strip-types`.

It is split into three layers:

1. **Concerns uncovered in the Go codebase** — assumptions or design
   choices that became visible only when re-stating them in another
   language.
2. **Parity gaps in the TS port** — what is *not* yet ported.
3. **Cross-cutting learnings** — points that should improve both
   implementations.

---

## 1. Concerns uncovered in the Go codebase

### 1.1 Value-tagged subtype lattice — recommended fix

`NewInteger(42)` builds a Value with `VType = Scalar/Number/Integer/42`.
The literal value is encoded as a *type-path leaf*. Same for empty vs
non-empty strings (`Scalar/String/Empty` vs `Scalar/String/Proper`).

Why this is surprising:

- `v.VType.Equal(TInteger)` returns **false** for a concrete integer.
  Every internal caller knows to use `Matches`, but an external user
  reading `v.VType` will reach for `Equal` first.
- Two integers with different values have *different* `Type` instances,
  even though semantically they're both `Integer`. This wastes a bit of
  memory and makes any code that builds a `Set<Type>` or
  `Map<Type, X>` keyed on identity quietly wrong.
- The lattice is unbounded — every distinct integer expands the set
  of live types. The IDs you see in dumps are stable but the type
  parts list is not.

**Recommended replacement.** Decouple "what type is this?" from
"what specific literal is this?". Type identity stays at the *kind*
level (`Scalar/Number/Integer`); literal-pattern dispatch moves to a
first-class `Pattern` field on the signature. Concretely:

```
v.VType            // = Scalar/Number/Integer (always, for any integer)
v.Data             // = the actual int64
v.LiteralPattern   // optional: a Value the matcher compares structurally
                   //   when present, used by `def double[2] (x => 4)` style sigs
```

The Go engine already has `Signature.Patterns map[int]Value` for
record/list patterns; integer-literal dispatch can ride on the same
mechanism instead of being smuggled through type paths. The benefit:

- `Equal(TInteger)` works as users expect.
- Cardinality of `Type` is bounded by the lattice (~80 known kinds),
  not by the program's literal count.
- Pattern dispatch is in one place (`Patterns`) instead of two
  (`Patterns` + value-tagged sub-paths).

The cost is a single match.go change to consult `Patterns` for scalar
literals, and a one-time migration of any external code that today
inspects `Number/Integer/<n>` paths. That's a net simplification.

### 1.2 `Data interface{}` as an unchecked 14-way enum — more detail

The Go `Value.Data` is `interface{}`. Across the engine it is type-
asserted into 14+ concrete payload types: `int64`, `float64`, `string`,
`bool`, `[]Value`, `*OrderedMap`, `WordInfo`, `ForwardInfo`,
`MarkInfo`, `MoveInfo`, `FnDefInfo`, `*ObjectTypeInfo`, `RecordTypeInfo`,
`OptionsTypeInfo`, `TableData`, `ChildTypeInfo`, `DepScalarInfo`,
`DisjunctInfo`, `ErrorInfo`, etc. No single switch statement sees all
of them; the casts are scattered:

- `value.go` accessors: `AsString`, `AsInteger`, `AsList`, `AsMap`,
  `AsAtom`, `AsWord`, `AsForward`, `AsTableType`, `AsChildType`,
  `AsRecordType`, `AsOptionsType`, `AsObjectType`, `AsObjectInstance`,
  `AsDepScalar`, `AsDisjunct`, `AsError`, `AsModule`, `AsPath`,
  `AsCalDuration`, `AsClkDuration`, `AsDate`, etc.
- `value.go::String()`: a long type-switch covering most payload types.
- Native `native_*.go` handlers: each does its own
  `v.Data.(SomeType)` to access fields the type accessor doesn't expose.
- `unify.go`, `compare.go`: more type switches over the same set.

Why this is a real problem:

- **Adding a payload type silently breaks anywhere the switch isn't
  updated.** The compiler does NOT tell you. CLAUDE.md's "Panic
  Prevention" section exists exactly because the type system can't
  enforce exhaustive handling — *the rule is enforced by code review
  and tests, not the language*.
- **The "concrete vs type-literal vs carrier" trichotomy lives in
  flags** (`Data == nil`, `Carrier == true`). There's no static
  guarantee any handler treats all three. The CLAUDE.md guards are
  hand-written.
- **Cross-cutting refactors are dangerous.** Renaming a payload field
  doesn't error out call sites that go through `interface{}`; runtime
  panics surface them.

**Recommended replacement.** Make `Value` a sealed interface (Go) /
discriminated union (TS) so the compiler can prove every handler
covers every variant.

Go sketch:

```go
type Value interface { isValue(); VType() Type; Carrier() bool }

type IntegerValue   struct { N int64 }
type StringValue    struct { S string }
type ListValue      struct { Elems []Value }
type WordValue      struct { Info WordInfo }
type RecordTypeValue struct { Info RecordTypeInfo }
// …

func (IntegerValue) isValue() {}
func (IntegerValue) VType() Type { return TInteger }
// …
```

A handler then accepts a typed parameter or type-switches with a
default that the compiler refuses to drop:

```go
switch v := v.(type) {
case IntegerValue: …
case StringValue:  …
default:           // compiler-warning if a new variant exists
}
```

TS gets it for free with discriminated unions:

```ts
type Value =
  | { kind: 'integer'; data: bigint }
  | { kind: 'string'; data: string }
  | { kind: 'list';    data: readonly Value[] }
  | { kind: 'word';    info: WordInfo }
  | { kind: 'record'; info: RecordTypeInfo }
  // …
```

Switch statements with `kind` are exhaustively-checkable via the
`never` type. Adding a new variant produces compile errors at every
site that needs updating — exactly the property CLAUDE.md tries to
enforce by prose.

The cost is high: rewriting every accessor and every native handler
that reaches into Data. Done incrementally (one variant at a time,
keeping `Data` as a fallback), it can ship over multiple PRs.

### 1.3 Mirror argument-ordering rule — test rows + consolidation

Two recommendations, taken together:

**(a) More TSV rows.** The mirror rule (`f a b` ≡ `b f a` ≡ `b a f`,
but `a f b` is the swap form) is documented prose in CLAUDE.md and
tested by *some* rows in `arithmetic.tsv`/`strings.tsv`. Adding a
canonical row for every (arity, position-of-`f`) pair across each
non-commutative test word would lock the rule into the regression
suite. The TS port hit a real authoring bug
(`"hello" concat " world"` was assumed to be a mirror form; it's
actually the swap form) that more rows would have caught immediately.

**(b) Consolidate the implementation into one function.** Today the
mirror rule is split:

- `engine.go::rearrangeForForward` reorders collected values
  (forward args first, stack args reversed) and writes them back into
  the live stack.
- `match.go::matchSignature` reads them with a `nearestFirst` flag
  that depends on `stackOnly`, `ForceStack`, etc.
- The interaction between the two is implicit; a reader has to chase
  three files (engine.go, match.go, CLAUDE.md) to convince themselves
  the algorithm is right.

A **single pure function** would be easier to reason about and easier
to unit-test directly:

```go
// applyMirror returns the args in signature order given:
//   - forwardArgs: tokens collected from after the word, source order
//   - prefixArgs:  tokens consumed from before the word, ascending stack index
//   - stackOnly:   true → all-prefix uses deepest-first; false → nearest-first
func applyMirror(forwardArgs, prefixArgs []Value, stackOnly bool) []Value
```

A test file `mirror_test.go` (and `mirror.test.ts`) with one assertion
per row of CLAUDE.md's mirror table would lock the rule down once,
without engine state, without paren handling, without sig matching —
just the ordering arithmetic. The current code mixes that arithmetic
with stack-rewriting and sig type-checking; pulling it out makes both
loops simpler.

The TS port already has this shape: `tryForwardSplit` and
`tryStackOnly` produce the args array in sig order in *one place*. The
Go engine should follow.

### 1.4 Stack-only vs forward-precedence: language choice, with implementation echoes

The asker correctly noted: is the difference in the *implementation
functions*, or in the *AQL language itself*? **Both — but the language
choice is the cause and the implementation is the effect.**

The language semantics:

- For a **forward-precedence** word `f` declared with sig `[A, B]`:
  - `f x y`     → handler sees `(A=x, B=y)`
  - `y f x`     → handler sees `(A=x, B=y)`  (mirror)
  - `y x f`     → handler sees `(A=x, B=y)`  (mirror, all-prefix nearest-first)
  - `x f y`     → handler sees `(A=y, B=x)`  (swap form)
- For a **stack-only** word `g` declared with sig `[A, B]`:
  - `x y g`     → handler sees `(A=x, B=y)`  (deepest-first)

Same physical stack `1 2 op` produces opposite arg bindings depending
on how `op` was registered. Authors of `op` know; readers of the call
site cannot tell without looking up the registration.

The implementation echoes this:

- `match.go::matchSignature` carries a `nearestFirst` boolean derived
  from `!stackOnly && !ForceStack` and uses it to choose stack-arg
  walk direction.
- `rearrangeForForward` only fires for forward-precedence words; for
  stack-only words the stack values are read in source order.

**Why it exists.** Forward-precedence words read like infix /
function-call notation across all four equivalent forms — `add 2 3`,
`2 add 3`, `2 3 add` all behave the same. Stack-only words read as
postfix consumers — `1 2 sub` should be "subtract 2 from 1"
(stack-bottom is the minuend). Trying to apply the mirror rule to
stack-only words would either make `1 2 sub = -1` (counter-intuitive)
or force every stack-only word author to push the args in reverse.

**Why it hurts.** A reader looking at `1 2 op` cannot determine the
binding without consulting the word's registration flag. The same
visual call site means two different things depending on a property
of the callee.

**Recommendations:**

- Document this in a single highlighted block in CLAUDE.md (the
  "Argument Ordering" section currently mentions the mirror but
  doesn't contrast against stack-only).
- Add TSV rows demonstrating the contrast: a hypothetical 2-arg
  stack-only `divmod` and a 2-arg forward-precedence `divmod` with
  the same physical stack and different observed bindings.
- Consider a *visible marker* at the call site for stack-only words —
  e.g., suffix `:s` to disambiguate. Probably unnecessary in practice,
  but worth weighing.

### 1.5 `SetCapability(name, nil)` overloaded as delete — fixed

Accepted recommendation; the API is now split:

- `SetCapability(name, value)` — INSTALL or REPLACE; storing nil/null
  is a real operation (capability is present, value is nil).
- `DeleteCapability(name) bool` — REMOVE; returns true if a
  capability was present.

Implemented in both Go (`aqleng/go/capability.go`) and TS
(`aqleng/ts/src/registry.ts`). The TS variant additionally exposes
`hasCapability(name)` and changes `capability(name)` to return a
`(value, ok)` tuple so a stored null is distinguishable from "not
present" — Go always had this through its `(any, bool)` return.

The previous behaviour was a real footgun: a typed-nil interface
argument silently became a delete instead of an "install nil" call.
Splitting removes the ambiguity.

### 1.6 Value mutation semantics — partly inherent to TS

Yes, partly. JavaScript / TypeScript has no value types, so any
`Value` is reference-shared. The Go engine gets immutability for free
because handlers receive `Value` *by copy*; mutating the copy doesn't
touch the engine's stack. The TS engine cannot offer the same
guarantee at the type-system level.

That said, "you can't avoid it in TS" is not the whole answer. There
are mitigations of varying strength:

| Approach | Strength | Cost |
|---|---|---|
| `readonly` modifiers on fields | compile-time only | none, bypassed by `as any` |
| `Object.freeze(this)` in Value constructor | runtime, throws in strict mode | small per-construct cost |
| Deep-freeze on `data` field | runtime | larger; precludes lazy initialisation |
| Wrap in immer / Immutable.js | runtime + ergonomic | full library dependency |
| Convention only | none | none |

The pragmatic recommendation for aqleng-ts: **`Object.freeze(this)`
in the Value constructor** plus the existing `readonly` modifiers.
That gives `'use strict'` callers a runtime error if they mutate a
shared Value; the perf cost is on the order of a single boolean flip
per construct, negligible against the bigint allocation a `newInteger`
already does. Deep-freezing `data` is generally too costly because
many payloads (`OrderedMap`, `[]Value`) mutate during construction
— freeze the wrapper, trust the payload to be opaque.

The Go vs TS gap remains real, but the absolute risk in the spec
subset is low (no list/map mutation paths). Worth a follow-up that
adds `Object.freeze(this)` and a single test that asserts mutation
throws.

---

## 2. Parity gaps in the TS port

The TS engine is the slice the current spec set reaches. Not yet
ported (each is its own follow-up):

| Area | Notes |
|---|---|
| Value payload types beyond int/dec/string/bool/atom/word/typeLiteral | No List, Map, OrderedMap, ChildTypeInfo, RecordTypeInfo, OptionsTypeInfo, TableTypeInfo, TableData, FnDefInfo, ObjectTypeInfo, DepScalarInfo, DisjunctInfo, ErrorInfo, etc. |
| Carriers / static type-check mode | `IsCheckMode`, `Carrier` flag, `StripToCarriers`, `Check.*` state. |
| Unify | Whole `unify.go` — type unification, dependent-leaf checks, structural map/list unify. |
| Dependent scalars | `DepInteger`, `DepBound`, `DepKind`, etc. |
| Module subsystem | Module/import/export, `RunModuleBody`, `loadFileModule`, `installExports`. |
| FnDef / CallAQL | User-defined functions; `def`/`fn`/`undef`/`var`/`call`/`args` words. |
| Control flow | `if`/`for`/`do`/`error`, break/continue, `IfCont`/`ForCont`. |
| Trace / step budget | `Trace`, `traceWrap`, step-budget enforcement. |
| Mark/Move primitives | `__MK`/`__MV` continuation tokens. |
| Forward token / paren pre-evaluation | `NewForward`, `NewOpenParen`, `NewParenExpr`, `preEvalParens`. |
| Interpolated strings | `NewInterpString`, `evalInterpString`. |
| Method modifiers (`/q`, `/N`) | Quote-arg capture, arg-count modifiers on words. |
| Type registry (separate from def stack) | `r.Types`, `PushType`/`PopType`, `ResolveTypedName`. |
| Object instances | `ObjectInstanceInfo`, `NewObjectInstance`. |
| ID generation (`GenerateID`) | TS Values have no ID field. |
| Async handlers | Rejected at dispatch. The Go engine has no async handlers either. |

Current TS port: ~600 LOC. Go engine: ~10,000. A full port should
land 5–8k.

---

## 3. Cross-cutting learnings

### 3.1 Discriminated `Value` in both languages

See §1.2. Single biggest engineering improvement for both
implementations.

### 3.2 The mirror rule wants its own test file

See §1.3. A dedicated `mirror_test.go` / `mirror.test.ts` that calls
`applyMirror(forwardArgs, prefixArgs, stackOnly)` on every
permutation of every arity 2–5 word would lock the algorithm down
in one place, decoupled from sig matching and stack rewriting.

### 3.3 Capability key constants are an obvious convention to formalise

Both ports converged on string keys (`"engine.fileops"`, etc.) defined
in the host package. Recommending a keyspace convention
`<host-package>.<service>` would make multi-plugin coexistence
predictable.

### 3.4 TSV specs are a language-agnostic regression contract

The Go and TS engines now share a regression suite
(`aqleng/test/spec/*.tsv`). A small CI step that runs both — `go test
./aqleng/go/... -run TestSpec` and `node --test --experimental-strip-types
aqleng/ts/src/spec.test.ts` — and fails the build if either does
locks behavioural equivalence in.

### 3.5 The spec format itself revealed two implementation bugs (in me)

While writing the TS port:

- I mis-stated the mirror form for `concat` in the strings spec
  (had `"hello" concat " world" → "hello world"`; the actual
  semantics give `" worldhello"`). The TS engine was correct; my
  Go-side test data was wrong. Both engines now agree on the row.
- I expected `add nope 3` to surface an `undefined_word` error
  because `nope` is undefined. Both engines actually surface a
  `signature_error` because forward collection rejects `nope` (a
  word, not Integer) before it can dispatch. The TSV row was updated
  to assert what the engines actually do.

Two facts the spec now nails down that prose alone hadn't.

### 3.6 Type-parameter erasure in TS makes `Cap[T]` weaker

Go's `Cap[T]` performs `v.(T)` which validates at runtime and
returns `(zero, false)` on mismatch. TS type parameters are erased,
so `cap<T>(r, name)` can only do an unchecked cast — the host is
trusted to install the right concrete shape under the agreed name.
A test in `aqleng/ts/src/capability.test.ts` pins this so future
drift is visible. If runtime validation matters, the TS host needs
to provide its own `instanceof`-style predicate at the call site.
