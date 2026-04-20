# Boolean Operations in AQL

## Scope

This report surveys how AQL implements its boolean operators
(`and`, `or`, `not`, `xor`, `nand`, `implies`), where each operator is
overloaded, how argument collection interacts with concatenative
evaluation, and what short-circuiting behaviour (if any) the language
provides. Recommendations follow.

Sources reviewed:

- `aql/internal/engine/native_boolean_and.go`
- `aql/internal/engine/native_boolean_or.go`
- `aql/internal/engine/native_boolean_not.go`
- `aql/internal/engine/native_boolean_xor.go`
- `aql/internal/engine/native_boolean_nand.go`
- `aql/internal/engine/native_boolean_implies.go`
- `aql/internal/engine/native_helpers.go` (`registerBinaryBoolWord`)
- `aql/internal/engine/conditional.go` (`isTruthy`, `if`)
- `aql/internal/engine/signature.go` (`BarrierPos`, scoring)
- `aql/internal/engine/match.go` (barrier/forward-collection logic)
- `aql/doc/LANGREF.md` §"Boolean Words" and §"if"
- `aql/test/basic.tsv`, `aql/test/sigmatch.tsv`, `aql/test/curry_test.go`

---

## 1. The Boolean Word Set

| Word      | Arity | Signature(s)                                       | Forward? | BarrierPos |
|-----------|-------|----------------------------------------------------|----------|------------|
| `not`     | 1     | `[boolean] -> [boolean]`                           | yes      | 0 (none)   |
| `and`     | 2     | `[boolean, boolean] -> [boolean]`                  | yes      | 0          |
| `xor`     | 2     | `[boolean, boolean] -> [boolean]`                  | yes      | 0          |
| `nand`    | 2     | `[boolean, boolean] -> [boolean]`                  | yes      | 0          |
| `implies` | 2     | `[boolean, boolean] -> [boolean]`                  | yes      | 0          |
| `or`      | 2     | `[boolean, boolean] -> [boolean]` **and** `[any, any] -> [disjunct]` | yes | 1 on both  |

All six are registered with `ForwardPrecedence: true`, so the standard
concatenative mirror applies — they can appear in any of the prefix /
mixed / forward positions described in `aql/CLAUDE.md` §"Argument
Ordering". For example, `and` accepts:

```
true and false       # forward: sig[0]=true, sig[1]=false
true false and       # all-prefix: sig[0]=true, sig[1]=false
```

`and`, `xor` and `nand` share a single helper,
`registerBinaryBoolWord` (`native_helpers.go:29`), which bakes in the
`[TBoolean, TBoolean]` signature and wraps a pure Go lambda
(`a && b`, `a != b`, `!(a && b)`). `implies` is written inline but
structurally identical, returning `!left || right`. `not` is the sole
unary case (`[TBoolean] -> [TBoolean]`).

---

## 2. Overloading

### 2.1 `or` — logical OR **and** type disjunction

`or` is the only overloaded boolean operator. It carries two
signatures (`native_boolean_or.go:34-59`):

1. `[TBoolean, TBoolean] -> [TBoolean]` — ordinary logical OR.
2. `[TAny, TAny] -> [disjunct]` — builds a `DisjunctInfo`
   (union type), flattening nested disjuncts so that
   `A or B or C` collapses to a single three-alternative union
   rather than a nested tree. Widening (`CarrierDisjunctCap`) and
   subtype subsumption are applied via `JoinCarriers`.

Dispatch uses signature specificity. Because `TBoolean` is more
specific than `TAny` in the Scalar hierarchy, two concrete booleans
select the boolean handler; anything else (type literals, numbers,
records…) falls through to the disjunct handler. The test file
`sigmatch.tsv:147-153` documents this behaviour explicitly:

```
true or false          => true              # boolean signature wins
String or None         => Scalar/String|None # disjunct signature
```

Both signatures are tagged `BarrierPos: 1`. A comment at
`native_boolean_or.go:5` notes that matching `BarrierPos` on the
boolean signature is required so the scoring bonus
(`signature.go:338-341`, `500_000 + (MaxArgs-BarrierPos)*10_000`) does
not accidentally prefer the less-specific disjunct rule. The barrier
also prevents `a or b or c` from being collected as a single
three-arg greedy grab by the outer `or`.

### 2.2 All other boolean words — boolean only

`and`, `not`, `xor`, `nand`, `implies` are **not overloaded**. They
declare only a `TBoolean` signature, so passing any other type
produces a signature-match failure rather than, say, a truthy-coerce.
`1 and true` is an error, whereas `if 1 …` succeeds (see §4 below).

This is asymmetric: `or` is the one operator willing to work on
non-booleans, and what it does there (build a type union) has no
runtime analogue on `and` (which would ideally build an intersection).

---

## 3. Short-Circuiting

### 3.1 There is none for `and`/`or`/`implies`

AQL's boolean operators **never short-circuit**. The concatenative
dispatch pipeline collects both forward and stack arguments
(`match.go`, `engine.go` execution loop) before invoking a handler.
By the time `and`, `or`, `nand`, `xor`, or `implies` receive their
`args` slice, both operands have already been fully evaluated.

Consequences:

- `false and (1 div 0)` evaluates `1 div 0` first and errors out.
- `true or expensiveComputation` still runs `expensiveComputation`.
- There is no difference in cost between `false and cheap` and
  `false and expensive`.

The AQL-CODE-REVIEW-REPORT and LANGREF make no mention of
short-circuit semantics; a repository-wide search for `short-circ` turns
up only unrelated engine-internal optimisations.

### 3.2 Where lazy evaluation *does* exist: `if`

`if` (`conditional.go`) is the only truly lazy control word. It
declares `NoEvalArgs: {0: true, 1: true, 2: true}`, so list-form
branches are not auto-evaluated before dispatch; the handler then
returns only the matching branch for the engine to splice in. The
LANGREF calls this out explicitly (`LANGREF.md:2318-2323`):

```
if true 1 [10 div 0]    => 1    # no division error
if false [10 div 0] 2   => 2    # no division error
```

So in today's AQL, if you want short-circuiting you write
`if cond [then] [else]`, not `cond and then` or `cond or else`.

### 3.3 Apparent precedence is really argument collection

`LANGREF.md:1075` claims `true or false and false => true` because
"`and` binds first". There is no real operator-precedence table; the
effect comes from concatenative dispatch order combined with
`BarrierPos=1` on `or`. `and` (barrier 0) greedily consumes the two
booleans immediately to its right, producing `false`, which the outer
`or` then combines. The "precedence" note is descriptive, not a
language feature the parser knows about.

---

## 4. Truthiness vs. Strict Booleans

`isTruthy` (`conditional.go:10-41`) defines a richer notion of
truth used by `if`:

- `true` / `false` — direct
- integer: non-zero
- `None` / empty list / empty map / `""` — falsy
- non-empty list / map / string — truthy
- `"true"` / `"false"` strings — mapped
- anything else — truthy if `valToString` is non-empty

**Crucially**, the boolean operators *do not* consult `isTruthy`.
They require `TBoolean`. So the two notions of "truth" diverge:

```
if 1 "yes" "no"    => 'yes'      # isTruthy(1) is true
1 and true         => ERROR      # signature mismatch
```

Users must explicitly `convert x boolean` before feeding non-boolean
data to `and`/`or`/etc. There is no implicit coercion, nor a value-
returning `or` (Python-style `x or default`).

---

## 5. Interaction With Other Features

- **Currying works**. `def and_true [and true] end false and_true`
  → `false` (`basic.tsv:557-558`, `curry_test.go:183-195`). The
  bracketed body defers execution, and the resulting one-arg word
  threads the curried `true` into the second slot via the mirror rule.
- **Static type-check mode** dispatches the same signatures over
  carrier values (`engine.go:261`), so the disjunct branch of `or`
  produces disjunct carriers at check-time, enabling union-type
  narrowing in `if` branches. `and`/`xor`/`nand`/`implies` produce a
  `TBoolean` carrier.
- **Record/table values** never match the boolean signature of `or`,
  so they always produce a disjunct. This is deliberate but means
  there is no way to write a single word that means "logical OR of
  two booleans *or* union of two types" with consistent error
  behaviour — a user mistake (e.g. forgetting to `convert`) silently
  produces a type value instead of a boolean.
- **Disjunct flattening order** (`native_boolean_or.go:18-30`)
  preserves source order: left alternatives first, then right. That
  matters for `unify`, which tries alternatives in order.

---

## 6. Gaps and Surprises

1. **No short-circuit**. Most notable gap. Every other mainstream
   language (including Scheme, Forth with `IF`, Factor, Joy) offers
   short-circuit AND/OR.
2. **Strictly typed operands**. `and`/`or`/etc. do not accept truthy
   non-booleans, even though `if` does. This is inconsistent and
   surprises users porting from dynamic languages.
3. **`or` does two unrelated things**. Its type-disjunct meaning
   relies on the dispatcher picking the right signature. When the
   user *intended* booleans but passed (say) a map and a `None`,
   they silently get a disjunct instead of a clear error.
4. **Missing symmetric operators**. `nand` exists, but `nor` and
   `xnor` / `iff` do not — despite `nand` and `nor` being equally
   natural. There is also no intersection counterpart to the
   disjunct form of `or` (no `and`-as-type-intersection).
5. **No value-returning `or`**. `x or default` returning the first
   truthy value is a widely used idiom and is absent.
6. **No Boolean short-circuit even under static check**. Because the
   handler sees both args, the check-mode narrowing from `and` does
   not get the positive/negative flow-typing that `if` provides —
   missing an opportunity for tighter types in patterns like
   `x isType Integer and x gt 0`.
7. **Documentation ambiguity**. `LANGREF.md:1075` says "`and` binds
   first"; newcomers may read this as Pratt-style precedence. It is
   actually an artefact of `BarrierPos` on `or`.

---

## 7. Recommendations

### 7.1 Introduce short-circuit forms

Two viable shapes, not mutually exclusive:

- **Lazy list operand**: overload `and`/`or` with signatures
  `[TBoolean, TList] -> [TBoolean]` and mark the second arg with
  `NoEvalArgs`. When the LHS suffices to decide the result, return
  it; otherwise `do` the list body. Mirrors `if` and is idiomatic
  for a concatenative language. Examples:

  ```
  false and [expensiveCheck]   => false         # body not run
  true  or  [loadFallback]     => true          # body not run
  ```

- **New words**: `andalso`, `orelse` (Erlang style) or `&&`, `||`.
  Implement them as forward-precedence words with two signatures,
  one of which wraps the RHS in a sub-engine `do` and runs it only
  when needed. This keeps the strict `and`/`or` intact for users
  who want eager evaluation (useful for effectful code).

In either case, extend `implies` the same way — `a implies b`
should not evaluate `b` if `a` is false.

### 7.2 Offer a truthy-coercing family

Add `andt` / `ort` / `nott` (or reuse `&&`/`||` from 7.1) that run
`isTruthy` on their arguments and return booleans. Alternatively,
add implicit coercion only for lists and maps where there is little
ambiguity. The divergence between `if`'s truthiness and boolean
operators' strictness is a real wart.

### 7.3 Value-returning forms

Port the Python/Lua idiom:

- `coalesce a b` — returns `a` if `isTruthy(a)`, else `b`.
- `either a b` — returns first non-`None` of `a`, `b`.

These round out the "pick a default" patterns without overloading
`or` any further.

### 7.4 Fill in the symmetric operators

Add `nor` and `xnor` (aka `iff`) alongside `nand`, each three-line
wrappers around `registerBinaryBoolWord`. The cost is trivial and
the symmetry is worth it for logic-heavy code
(decision tables, type predicates).

### 7.5 Separate `or` from disjunction

The dual meaning of `or` is elegant in print but muddies dispatch
and error messages. Options, ordered from least to most invasive:

1. Keep `or`, but add a dedicated `union` word for type disjunction
   and reserve `or` for booleans once code has migrated.
2. Emit a check-mode warning when `or` dispatches to the disjunct
   handler on values that were inferred to be `TBoolean` at some
   earlier point — catches "I forgot `convert boolean`" bugs.
3. Leave `or` as-is but return a richer error from the boolean
   handler when a signature mismatch is fed to disjunct — something
   like `or: expected [boolean, boolean] or two type values, got …`.

### 7.6 Flow typing through `and`

Today `if (x isType Integer) [x add 1]` narrows `x` in the
then-branch. `x isType Integer and x gt 0` does not, because
`and`'s handler is opaque to the flow-typing pass. A short-circuit
`and` (7.1) could feed the narrowing restore into the RHS body,
mirroring the existing `applyGuardNarrowing` machinery in
`conditional.go`.

### 7.7 Documentation

- Replace "`and` binds first" with a concrete argument-collection
  explanation plus a note that there is **no** operator precedence.
- Explicitly document that boolean operators are strict (eager)
  and require `boolean` operands, and point users to `if` for
  lazy evaluation.
- Document `or`'s dual dispatch with a worked example of when each
  signature wins.

---

## 8. Summary

AQL's boolean layer is small, regular, and conceptually clean:
forward-precedence words with strict `[boolean, boolean]`
signatures, plus `not`. The one overload is `or`, which doubles as a
type-union constructor via a `[any, any]` signature guarded by
`BarrierPos`. The notable omission is short-circuit evaluation:
`and`, `or`, and `implies` all eagerly evaluate both operands
because the concatenative dispatcher resolves arguments before
invoking handlers. `if` fills this gap via `NoEvalArgs`, so the
machinery already exists — extending it to `and`/`or` is the single
most impactful improvement. Secondary improvements round out the
operator set (`nor`, `xnor`, `coalesce`), reconcile the strict /
truthy divergence with `if`, and make `or`'s dual role less
ambiguous.
