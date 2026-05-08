# Conversion notes: Go → TypeScript port of aqleng

This document is the audit trail from porting the Go engine to TypeScript
under `aqleng/ts/`, written while the port was in progress. The TS port
covers the subset the existing TSV specs exercise (Value/Type lattice,
Registry+capability, native dispatch, signature matching, error paths)
and passes the same `aqleng/test/spec/*.tsv` rows.

It is split into three layers:

1. **Concerns uncovered in the Go codebase** — assumptions or design
   choices that became visible only when re-stating them in another
   language, and that may or may not be intentional.
2. **Parity gaps in the TS port** — what is *not* yet ported, with
   notes on what porting them would require.
3. **Cross-cutting learnings** — points that should improve both
   implementations.

---

## 1. Concerns uncovered in the Go codebase

### 1.1 Value-tagged subtype lattice

`NewInteger(42)` builds a `Value` with `VType = Scalar/Number/Integer/42`,
not `Scalar/Number/Integer`. The literal value is encoded as a
*type-path leaf*. The same is true for empty vs non-empty strings
(`Scalar/String/Empty` vs `Scalar/String/Proper`).

This is a deliberate dispatch trick — pattern matching on numeric
values comes for free — but it has consequences:

- **`Type.Equal` is the wrong default test.** A test like
  `v.VType.Equal(TInteger)` returns false for a concrete integer.
  Internal callers all use `Matches`; an external user reading the
  type field will be surprised.
- **Type identity is unbounded.** Every distinct integer creates a
  distinct `Type`. The TS port mirrors this (`Number/Integer/${n}`),
  but for very large programs this might be measurable. Worth
  benchmarking before assuming.
- **Type-tag round-trips lose precision in general-purpose
  serializers.** Any code that round-trips Values through JSON has to
  reconstruct the type from the payload, not from the path string.

### 1.2 `Data interface{}` as a 14-way enum

The Go `Value.Data` field is `any`, and the engine type-switches it on
14+ concrete payload types: `int64`, `float64`, `string`, `bool`,
`[]Value`, `*OrderedMap`, `WordInfo`, `ForwardInfo`, `MarkInfo`,
`MoveInfo`, `FnDefInfo`, `*ObjectTypeInfo`, `RecordTypeInfo`, `TableData`,
`ChildTypeInfo`, `OptionsTypeInfo`, `DepScalarInfo`, etc. Each
accessor (`AsString`, `AsList`, `AsMap`, …) does its own cast.

This is hard to track in either language:

- New payload types are easy to forget when adding handler logic.
- The "concrete vs type-literal vs carrier" trichotomy lives entirely
  in the `Data == nil` / `Carrier == true` flags — there's no static
  guarantee that handlers handle all three cases.
- The CLAUDE.md "Panic Prevention" section has an explicit rule
  ("Always nil-check before dereferencing") because the type system
  cannot enforce this.

A discriminated-union style (one tag field, one payload field of the
matching shape) would be a real improvement in both languages. The TS
port would benefit immediately from `type Value = IntegerValue |
StringValue | …`.

### 1.3 Signature ordering by ad-hoc score

`signatureScore` sums `Specificity()` (path length) across arg types.
That ranks `[Integer, Integer]` (3+3=6) above `[Any, Integer]` (1+3=4)
above `[Any, Any]` (1+1=2). It works for the spec subset but:

- **Ties are resolved by stable sort + insertion order.** Two sigs
  with the same arg shapes will dispatch in registration order. If a
  module registers a sig later, it will lose ties — but registering
  *the same shape twice* is itself probably a bug the engine should
  reject.
- **Sum-of-specificity is not lattice-correct.** `[Integer]` and
  `[String]` both have score 3, but a value of type `Integer` matches
  the first and not the second. Specificity scoring only ranks; the
  actual match is still type-checked. This is fine, but it makes the
  ordering algorithm load-bearing for *which* sig fires first when
  multiple match — and the load isn't documented.

### 1.4 The mirror argument-ordering rule is verifiable, but rarely

`CLAUDE.md` describes the mirror rule with great care, but the
*implementation* of it is split across `rearrangeForForward` and
`matchSignature`'s nearestFirst flag. While porting I had to chase
the algorithm through three files to convince myself the TS version
matched. The spec already has rows that prove `sub 10 3`, `3 sub 10`,
`3 10 sub` all give 7 and `10 sub 3` gives -7 — but the *swap form*
`a f b` gets one inline test in CLAUDE.md and zero in the test
fixtures.

Adding a single TSV line for each non-equivalent permutation of every
arity 2–5 word would lock the rule in for both engines. (The spec
runner under TS hit this immediately on `"a" "b" concat` vs
`"hello" concat " world"`.)

### 1.5 Stack-only vs forward-precedence have different ordering rules

For ForwardPrecedence words, all-prefix matches use **nearest-first**
(top of stack → sig[0]). For stack-only words, all-prefix uses
**deepest-first** (bottom → sig[0]). This is documented in CLAUDE.md
once but easy to miss; both implementations have to special-case it.

This is intentional — a stack-only word like a binary operator
written postfix-style (`1 2 op`) wants `op(1, 2)` not `op(2, 1)`. But
it means the same physical stack arrangement dispatches differently
based on a flag set at registration. A reader of `1 2 add` cannot
tell what sig[0] will be without looking up `add`'s registration.

Worth documenting in a single highlighted block, ideally with a TSV
row showing both for the same physical stack.

### 1.6 `Registry.SetCapability(name, nil)` deletes — Go vs TS

The Go `SetCapability` deletes the entry when value is `nil`. The TS
port matched this for `null` and `undefined`, which is right by
intent — but the "set to falsy zero" case is now ambiguous in TS:
`SetCapability("flag", false)` *keeps* the capability (correct), but
`SetCapability("flag", 0)` also keeps it (correct), and the only
sentinel for delete is `null/undefined`.

Recommendation: rename to `setCapability(name, value): void` and
`deleteCapability(name): void`. The current shape is clever but the
delete-on-nil idiom is a footgun.

### 1.7 Method receivers and Value mutation

Go's `Value` is a struct (value type). Handlers receive copies; you
cannot mutate the engine's stack by mutating an arg. In TS, `Value`
is a class and `args[i]` is a reference. A misbehaved handler can in
principle mutate the data of a value the engine still has on the
stack.

The TS port mitigates by making `Value` constructor fields readonly,
but the `data` field is still typed `unknown` and a cast can reach
mutable backings (e.g. arrays for lists). The Go engine has no such
exposure because the field is `interface{}` and handlers get their
own copies of the wrapping struct.

This is a real divergence that would need either deep-cloning on
arg pass or `Object.freeze()` on construction.

### 1.8 Initialization-order coupling on well-known types

`mustType` runs at package init in Go and panics on bad inputs.
`var TInteger = mustType("Number/Integer")` works because Go promises
ordered init. The TS equivalent is `export const TInteger =
newType('Number/Integer')` — also fine, but `newType` is now exposed
to user code that might call it before module load completes (in
top-level circular imports). The export shape is identical to the
Go shape but the TS variant is one circular import away from a TDZ
crash.

---

## 2. Parity gaps in the TS port

The TS engine is the slice the current spec set reaches. The full
Go engine includes the following that are NOT yet ported:

| Area | Status in TS | Notes |
|---|---|---|
| Value payload types beyond int/dec/string/bool/atom/word/typeLiteral | absent | No List, Map, OrderedMap, ChildTypeInfo, RecordTypeInfo, OptionsTypeInfo, TableTypeInfo, TableData, FnDefInfo, ObjectTypeInfo, DepScalarInfo, DisjunctInfo, ErrorInfo, etc. |
| Carriers / static type-check mode | absent | `IsCheckMode`, `Carrier` flag, `StripToCarriers`, `Check.*` state. |
| Unify | absent | Whole `unify.go` — type unification, dependent-leaf checks, structural map/list unify. |
| Dependent scalars | absent | `DepInteger`, `DepBound`, `DepKind`, etc. |
| Module subsystem | absent | Module/import/export, `RunModuleBody`, `loadFileModule`, `installExports`. |
| FnDef / CallAQL | absent | User-defined functions with typed signatures; `def`/`fn`/`undef`/`var`/`call`/`args` words. |
| Control flow | absent | `if`/`for`/`do`/`error`/break/continue, `IfCont`/`ForCont`. |
| Trace / step budget | absent | `Trace`, `traceWrap`, step-budget enforcement. |
| Mark/Move primitives | absent | The `__MK`/`__MV` continuation tokens used by `for` and conditionals. |
| Forward token / paren pre-evaluation | absent | `NewForward`, `NewOpenParen`, `NewParenExpr`, `preEvalParens`. |
| Interpolated strings | absent | `NewInterpString`, `evalInterpString`. |
| Method modifiers (`/q`, `/N`) | absent | Quote-arg capture, arg-count modifiers on words. |
| Type registry (separate from def stack) | absent | `r.Types`, `PushType`/`PopType`, `ResolveTypedName`. |
| Object instances | absent | `ObjectInstanceInfo`, `NewObjectInstance`. |
| ID generation (`GenerateID`) | absent | TS Values have no ID field. |
| ReadList / ReadMap views | absent | TS uses arrays directly. |
| Async handlers | rejected at dispatch | The Handler signature returns `Value[] \| Promise<Value[]>` but the engine throws if a Promise comes back. The Go engine has no async handlers. |

The current TS port is roughly **600 lines** vs the Go engine's
**~10,000**. The 60× compression is mostly missing features, not
denser code — so a full port should expect to land in the 5–8k LOC
range (TS tends to be slightly more terse than Go for this style of
code).

---

## 3. Cross-cutting learnings

### 3.1 Both implementations would benefit from a discriminated `Value`

A single `data: unknown` field forces every accessor to type-assert.
In Go, this leaks via `interface{}` and the panic-prevention rules.
In TS, it leaks via `unknown` and a cast-heavy `asInteger()` /
`asString()` set. Refactoring `Value` to a tagged union (Go: a sealed
interface; TS: a discriminated union) would let both compilers verify
exhaustive handling.

### 3.2 The mirror rule needs more spec rows

Adding rows like `"a" "b" concat → "ba"` (already present) and
`"hello" concat " world" → " worldhello"` (also present, but rare)
to every multi-arg word's spec would catch any future regression in
either engine. Right now the mirror rule is "load-bearing CLAUDE.md
prose" — turning it into TSV rows turns it into a regression test.

### 3.3 Capability key constants are an obvious convention to formalise

Both ports converged on string keys (`"engine.fileops"`, etc.) defined
in the host package. Recommending and documenting a keyspace
convention (`<host-package>.<service>`) would make it easier for
multiple plugin sources to coexist without colliding.

### 3.4 TSV specs are language-agnostic regression tests

The single biggest win from this exercise: the Go and TS engines now
share a regression suite (`aqleng/test/spec/*.tsv`). When a future
behaviour is intentionally changed, the spec file changes once and
both engines must re-pass. When a future behaviour is *unintentionally*
changed in one language, the diff between the two languages' test
output catches it.

A small CI job that runs `go test ./aqleng/go/... -run TestSpec` and
`vitest run --dir aqleng/ts/` and checks both passed would seal this
in.

### 3.5 The spec format itself revealed two implementation bugs (in me)

While writing the TS port:

- I mis-stated the mirror form for concat in the strings spec (had
  `"hello" concat " world" → "hello world"`; the actual semantics
  per the rule give `" worldhello"`). The TS engine was correct; my
  Go-side test data was wrong. Both engines now agree on the same
  row.
- I expected `add nope 3` to surface an `undefined_word` error
  because `nope` is undefined. Both engines actually surface a
  `signature_error` because forward collection rejects `nope` (a
  word, not Integer) before it can dispatch. The TSV row was updated
  to assert what the engines actually do.

Two facts the spec now nails down that prose alone hadn't.
