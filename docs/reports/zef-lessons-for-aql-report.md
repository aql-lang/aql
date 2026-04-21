# Lessons from Zef for the AQL Implementation

This report distills the optimization journey described in Filip
Pizlo's *How To Make a Fast Dynamic Language Interpreter*
(https://zef-lang.dev/implementation) and maps each technique onto
the current AQL codebase in `aql/internal/engine/`. Zef's journey
took an AST-walking interpreter from 80× slower than Lua to within
roughly the cost of its own memory-safe runtime — a 16.6× speed-up
in Fil-C++ (67× with the Yolo-C++ port). The changes were almost
entirely structural: value representation, inline caching, object
model, and a handful of common-sense specializations. No JIT, no
bytecode, no SSA.

AQL today is shaped like the *starting* point of that journey:
strings everywhere, hashtables everywhere, an `interface{}`-boxed
`Value`, a generic signature matcher that runs on every call. That
makes almost every lesson in the article directly applicable.

## Contents

1. Zef's starting point and the twenty-one changes
2. Where AQL sits today
3. Top-tier wins
4. Medium-tier wins
5. Minor and build-level wins
6. Techniques that do not transfer
7. Suggested sequencing
8. Appendix: per-change mapping table

---

## 1. Zef's starting point and the twenty-one changes

Zef began as an AST-walking interpreter with two deliberate
performance choices (a 64-bit tagged value with NaN-boxing of
doubles, and C++ as the implementation language) and a long list
of expedient-but-slow defaults: string-keyed hashtables for every
scope, `std::string` everywhere for names, recursive C++ calls
to walk the scope chain, a separate `IntObject` heap type for
large integers.

The twenty-one changes, with their individual speed-ups relative
to Zef's own baseline, are:

| # | Change | Δ vs. previous |
|---|---|---|
| 1 | Direct operators (`Binary<>`/`Unary<>` AST nodes) | +17.5% |
| 2 | Direct RMWs (`+=` etc.) | +3.7% |
| 3 | Avoid `IntObject` dispatch on fast paths | +1% |
| 4 | Symbols (hash-consed name pointers) | +18% |
| 5 | Value inline header | +2.8% |
| 6 | Object model + inline caches + watchpoints | **4.55×** |
| 7 | `Arguments` struct (no `optional<vector>`) | +33% |
| 8 | Getter specialization | +5.6% |
| 9 | Setter specialization | +3.4% |
| 10 | Inline `callMethod` | +3.2% |
| 11 | Global `(class, symbol)` method-lookup hashtable | +15% |
| 12 | Avoid `std::optional` on hot path | +1.7% |
| 13 | Specialized zero/one/two-argument types | +3.8% |
| 14 | Pass `Value` by value on slow paths | +10% |
| 15 | De-duplicate `DotSetRMW::evaluate` | 0% |
| 16 | Specialize `sqrt` | +1.6% |
| 17 | Specialize `toString` | +2.7% |
| 18 | Constant-array-literal specialization | +8.1% |
| 19 | `Value::callOperator` by value | +6.5% |
| 20 | Disable RTTI / libc++ hardening | +1.8% |
| 21 | No asserts in release | ~0% |

Cumulative: 16.6× in Fil-C++; 67× in Yolo-C++.

The article's single most important observation, repeated
throughout, is that change #6 only works because its three
sub-changes land together. A new object model is useless without
inline caches to exploit it, and inline caches are unsafe without
watchpoints to invalidate them when scopes or classes mutate.

## 2. Where AQL sits today

Relevant architectural facts, gathered from `aql/CLAUDE.md` and
`aql/internal/engine/`:

- **Value representation.** `Value{VType Type, Data interface{}}`
  in `engine/value.go:478`. Every integer, float, string, list, and
  map is allocated through the interface boxing path. Type literals
  have `Data == nil`; several subtypes (`RecordTypeInfo`,
  `OptionsTypeInfo`, `ChildTypeInfo`) share `VType == TMap`, which
  forces defensive `nil`-checks throughout.
- **Word dispatch.** `execMatch` walks `DefStacks`, calls
  `matchSignature` (`engine/match.go`), optionally runs
  `rearrangeForForward` (`engine/engine.go`) to implement the
  concatenative mirror rule, then invokes a registered handler from
  `registry.go`. String comparison happens at every level.
- **Scope chain.** `DefStacks` is a stack of maps keyed by word
  name strings. No pre-resolution: every reference re-walks and
  re-hashes.
- **Dotted access.** The `.` and `!` `.` token pairs are mapped to
  the `get` and `getr` words during `convertTopLevelItems`
  (`parser/parse.go:166`), so `foo.a.b` reaches the engine as
  `foo get a get b`. Each `get` is still a generic `OrderedMap`
  string-keyed lookup at runtime.
- **Auto-evaluation.** Lists marked `Eval=true` run `autoEvalList`
  every time they are consumed as an argument, re-walking the
  element list and resolving `Word` atoms through `DefStacks`.
- **Arguments.** Forward and stack arguments are collected into
  `[]Value` slices and reordered by `rearrangeForForward` per call.
- **Parse pipeline.** `internal/parser/parse.go` on top of
  jsonic v0.1.6; produces engine `Value`s and a few distinguished
  wrappers (`ParenExpr`, `InterpString`, `numberVal`).

Mapped onto Zef's axes: AQL has the "strings everywhere",
"hashtables everywhere", and "recursive scope walk" problems that
Zef's changes #4, #6, and #11 targeted. It has the boxed-number
problem that Zef's tagged-value representation sidesteps. It does
**not** have the RMW, getter/setter, or C++-union pathologies that
several of Zef's smaller changes addressed.

## 3. Top-tier wins

### 3.1 Symbols (Zef #4: +18%)

Intern every name once into a hash-consed `*Atom`. Replace
`string`-keyed maps with `*Atom`-keyed maps so equality and
hashing become pointer operations. Sites to convert:

- Word-name matching inside `execMatch`.
- `DefStacks` lookup keys.
- `OrderedMap` keys in `engine/value.go` (the hot ones are the
  per-record field maps, not user-data maps).
- Type path strings used by type comparisons.

CLAUDE.md already uses "atom" informally (`quote` produces
`Atom(a)`); formalising that into an interned symbol with a pointer
identity is the natural next step.

**Cost.** Large but shallow — most edits are signature changes
from `string` to `*Atom`. Pairs well with a `Symbol()` constructor
that does the interning lookup at parse time.

### 3.2 Object model + inline caches + watchpoints (Zef #6: 4.55×)

The single biggest win in the article, and the one most dependent
on sequencing. For AQL:

1. **Resolve pass.** During parse (or as a post-parse pass over
   jsonic output), assign each `Word` node a resolved binding when
   the binding is statically visible — typically a `DefStacks`
   offset, a native-handler pointer, or a field offset within a
   known record shape. Dotted access (`get`) on a known
   `RecordTypeInfo`/`ObjectTypeInfo` resolves to a field offset.
2. **Inline cache slots.** Give each call-site AST node a
   three-word IC slot: `(shape_or_scope_generation, offset_or_handler,
   watchpoint_id)`. On the fast path, compare the generation to the
   cached one; if it matches, jump straight to the cached offset or
   handler.
3. **Watchpoints / generation counters.** Give every `DefStacks`
   frame and every shape object a generation counter. `def`,
   `undef`, and `var`-mutation bump it. Cache hits gate on the
   counter; misses fall back to the existing slow path and refill
   the slot.

The article is emphatic that these three must co-develop. A
resolve pass without IC slots does not help (the savings show up
only at call time). IC slots without watchpoints are unsound
(AQL's `def` permits mid-program rebinding). Watchpoints without a
stable shape model invalidate everything on every `def`, defeating
the cache.

**Cost.** Large, touches parser + engine + `DefStacks` + registry.
Structurally invasive but each sub-piece is self-contained.

### 3.3 Global method-lookup hashtable (Zef #11: +15%)

A simpler, near-orthogonal precursor to the per-site ICs: one
process-wide (or engine-wide) cache keyed by
`(defstack_generation, *Atom word_name)` → `{handler, Signature}`.

- Check before the full `DefStacks` walk + `matchSignature`.
- Bump the generation on any mutation; the whole cache invalidates
  implicitly (entries with stale generations are simply replaced on
  next miss).
- For monomorphic call sites (the majority in most programs) this
  collapses word dispatch to a single map lookup.

This can ship before the full IC machinery and captures most of
the call-dispatch cost.

### 3.4 Direct operators (Zef #1: +17.5%)

AQL today routes every call — including `add`, `sub`, `mul`,
`dup`, `swap`, `drop` — through `execMatch` →
`rearrangeForForward` → `matchSignature` → registered handler.
For a small, closed set of hot builtins, the parser can emit a
specialized call node whose handler is already bound and whose
argument order is known at parse time, skipping the generic
machinery.

The registered-handler list in `engine/registry.go` is the
authoritative source for which names are safe to specialize; a
small table inside the parser (or a parse-time hook) avoids
hardcoding names in two places.

Natural candidates: the `native_math_*.go` set, the `native_stack_*.go`
set, `get`/`set`, `dup`, `drop`, `swap`.

### 3.5 Specialized arguments (Zef #13: +3.8%; Zef #7: 1.33×)

Most AQL natives take zero, one, two, or three arguments. Today
each call allocates a `[]Value` slice and `rearrangeForForward`
may reorder it. Specialize:

- `execMatch0(handler)` — zero-arg.
- `execMatch1(handler, a)` — one-arg.
- `execMatch2(handler, a, b)` — two-arg.
- `execMatch3(handler, a, b, c)` — three-arg.

In Go, this is also the escape-analysis equivalent of Zef #7 —
removing the `[]Value` allocation keeps arguments in registers /
on the stack for hot builtins.

**Cost.** Moderate. Registry can record the handler's expected
arity once; dispatch picks the matching specialization at call
time.

## 4. Medium-tier wins

### 4.1 Value representation

`Value{VType Type, Data interface{}}` boxes every scalar. Every
`int64`, `float64`, and user string passes through Go's interface
header, often promoting to the heap. Go's GC barriers and the
pointer-map metadata make the JavaScriptCore NaN-tagging trick
impractical, but a flat struct achieves the same category of win:

```go
type Value struct {
    Kind Kind        // 1 byte enum
    _    [7]byte     // padding
    I    int64       // integer / bool / small enums
    F    float64     // float
    S    string      // string header (reuses heap data)
    P    unsafe.Pointer // map / list / user type
    // VType becomes derivable from Kind + P's runtime type
}
```

Eliminates the interface box on the hot path. `AsMap()` / `AsList()`
become a `Kind` switch rather than a type assertion.

This change is invasive and should land **before** the resolve
pass (§3.2) so that offsets and IC slots are sized for the final
layout, not the interface-boxed layout.

**Cost.** Very large. Touches every file in `engine/`, every
native handler, and the parser's converter functions. But it is
the Go-idiomatic equivalent of the Zef NaN-tagged representation,
and the article's 4× Fil-C→Yolo gap is almost entirely this
category.

### 4.2 Array-literal specialization (Zef #18: +8.1%)

`autoEvalList` re-walks every `Eval=true` list each time it is
consumed. Many of those lists are literal — `[1 2 3]`, `["a" "b"]` —
and contain no `Word`, `ParenExpr`, or `InterpString`. Detect this
during `convertWordList`/`convertDataList` and mark the list
`Const=true`. `execMatch` can skip `autoEvalList` entirely for
constant lists.

This is an easy flag addition and complements the existing
`Eval`/`Quoted` flags on the list struct.

### 4.3 Pass `Value` by value (Zef #14: +10%; Zef #19: +6.5%)

Audit `engine/` for `*Value` parameters where `Value` (a plain
struct) would suffice. Each `*Value` in a hot path is an
escape-analysis hint; passing by value tends to keep the value in
a register and prevents the struct from being heap-allocated.

This interacts well with the two-value type-assertion rule from
CLAUDE.md's panic-prevention section — both forms work identically
when `Value` is passed by value.

## 5. Minor and build-level wins

- **Avoid interface{} assertions on hot paths (Zef #12 analog).**
  After §4.1, `AsMap()`/`AsList()` collapse to a `Kind` switch and
  the defensive `nil`-check discipline applies only to cold paths.
- **Build flags (Zef #20).** Use `-gcflags=all=-B` to disable
  bounds checks in release builds; ensure `-race` is off for
  benchmarking. Mild but free.
- **Inline dispatch (Zef #5, #10).** Go's inliner is automatic
  but has budget limits. Once `execMatch` is split into
  specialized zero/one/two-arg variants, confirm each specialization
  fits the inliner budget; restructure if not.

## 6. Techniques that do not transfer

- **Zef #2 (RMW `+=`).** Concatenative AQL has no RMW syntax.
- **Zef #3 (`IntObject` dispatch).** AQL already has a single
  integer representation.
- **Zef #8/#9 (getter/setter pattern inference).** AQL has no
  class-member getter/setter idiom. The closest equivalent —
  `def name [get field]` — is already cheap after §3.3.
- **Zef #12 (`std::optional` unions).** Go has no `std::optional`;
  the underlying lesson — watch for allocator-forced heap
  escapes — is captured by §4.3.
- **Zef #16/#17 (`sqrt`, `toString` specialization).**
  Method-call-on-primitive is not AQL's dispatch shape; the word
  `sqrt` is already dispatched uniformly and will benefit from
  §3.3 and §3.4.
- **Zef #21 (no asserts).** Go's testing model is independent
  of release builds.
- **NaN-tagging with explicit bit offsets.** Go's GC barriers
  and pointer-map metadata preclude the
  `value + 0x1000000000000` trick; §4.1 is the Go-idiomatic
  substitute.
- **Yolo-C++ port (Zef's extra 4×).** AQL has no analogous mode;
  most of that gap in Go terms is already covered by §4.1.

## 7. Suggested sequencing

The article's central warning — "some things only work if they
land together" — is the main constraint. Proposed order:

1. **§3.1 Symbols.** Pure groundwork. Does not require any other
   change and pays for itself immediately in `execMatch` and
   `OrderedMap`.
2. **§4.1 Value representation.** Must precede §3.2 so offsets
   and IC slots are sized against the final layout. Biggest
   single-PR disruption in the plan.
3. **§3.3 Global method-lookup hashtable.** Cheap, captures most
   of the call-dispatch cost, and validates the generation-counter
   machinery that §3.2 will depend on.
4. **§3.2 Resolve pass + per-site ICs + watchpoints.** The
   centerpiece. Lands as a coordinated set of changes across
   parser, engine, and registry.
5. **§3.4 Direct operators.** Purely additive on top of §3.2.
6. **§3.5 Specialized arguments.** Orthogonal; can slot in
   anywhere after §4.1.
7. **§4.2 Constant-list specialization.** Small, independent.
8. **§4.3 Pass-by-value audit.** Sweep once the hot paths have
   settled.
9. **§5 Build flags.** Last; only meaningful once the structural
   work is done.

## 8. Appendix: per-change mapping

| Zef # | Zef Δ | AQL applicability | AQL counterpart |
|---|---|---|---|
| 1 | +17.5% | Direct | §3.4 Direct operators |
| 2 | +3.7% | None | Concatenative — no RMW |
| 3 | +1% | None | Already single-int rep |
| 4 | +18% | Direct | §3.1 Symbols |
| 5 | +2.8% | Partial | §5 inline audit |
| 6 | 4.55× | Direct | §3.2 Resolve + IC + WP |
| 7 | 1.33× | Direct | §3.5 Specialized args |
| 8 | +5.6% | None | No getter idiom |
| 9 | +3.4% | None | No setter idiom |
| 10 | +3.2% | Partial | §5 inline audit |
| 11 | +15% | Direct | §3.3 Global lookup table |
| 12 | +1.7% | Analog | §4.1 unboxed Value |
| 13 | +3.8% | Direct | §3.5 Specialized args |
| 14 | +10% | Direct | §4.3 Pass by value |
| 15 | 0% | N/A | No equivalent duplication |
| 16 | +1.6% | None | Dispatch already uniform |
| 17 | +2.7% | None | Dispatch already uniform |
| 18 | +8.1% | Direct | §4.2 Const list fast path |
| 19 | +6.5% | Direct | §4.3 Pass by value |
| 20 | +1.8% | Direct | §5 Build flags |
| 21 | ~0% | None | Different testing model |

---

**Source.** Filip Pizlo, *How To Make a Fast Dynamic Language
Interpreter*, https://zef-lang.dev/implementation.
