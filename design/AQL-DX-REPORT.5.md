# AQL Developer Experience Report: Implementing `aql:decision`

## Context

This report documents the experience of implementing a decision modeling
library (`aql:decision`) as a native AQL module. The goal was a pure-AQL
implementation covering decision tables, decision trees, condition
evaluation, and hit-policy resolution. The module was informed by a
comprehensive TypeScript API specification covering builders, evaluators,
predicates, and tracing.

The implementation ultimately became a hybrid: AQL builders + Go
evaluators. This report explains why, documents the issues encountered,
and suggests language improvements.

---

## What worked well

### 1. Map literals as data structures

Building condition/rule/table/tree models as nested maps was natural
and readable:

```aql
{when:{field:age,op:"gte",value:18}, then:{category:"adult"}}
```

The inline map syntax is expressive and composes well. Nested maps work
seamlessly. This made the data-modeling side of the decision library
straightforward — conditions, rules, tables, and tree nodes are all
just maps.

### 2. Simple `fn` definitions for builders

Pure data-construction functions like `cond`, `make-rule`, `make-table`
worked perfectly:

```aql
def cond fn [[field:Atom op:String value:Any] [Map] [
  do {field: field, op: op, value: value}
]]
```

These are concise and clear. The typed parameter syntax `field:Atom`
reads well. The `do` word for map construction with evaluated values
is the right primitive.

### 3. Dot-notation for module access

`decision.cond`, `decision.eval-table` etc. gives clean namespacing.
The FnDef auto-invocation mechanism works reliably for the first level
of call. Module words feel like a natural extension of the language.

### 4. Comparison operators

The full set of comparison words (`eq`, `neq`, `lt`, `gt`, `lte`, `gte`)
and boolean logic (`and`, `or`, `not`) provided everything needed for
condition evaluation. The `if` word with its three-arg form
`if cond [then] [else]` is clear.

---

## Issues found

### Issue 1: List auto-evaluation strips def references (critical)

**Severity:** Critical — blocks compositional programming patterns.

The most impactful problem. When a list literal like `[c1 c2]` is
consumed as an fn argument, the `Eval` flag is stripped and the
elements are NOT resolved from DefStacks. The list contains atoms
`c1`, `c2` instead of the map values those names reference.

```aql
def c1 (age "gte" 18 decision.cond)
def c2 (score "gt" 50 decision.cond)
[c1 c2] decision.all-of    # BROKEN: children = [atom(c1), atom(c2)]
```

The children list receives unresolved atoms instead of the condition
maps that `c1` and `c2` point to.

**Workaround:** Inline everything — no def'd names inside list literals
passed as fn arguments. This forces verbose, non-compositional code:

```aql
[{field:age,op:"gte",value:18} {field:score,op:"gt",value:50}] decision.all-of
```

This is the single biggest barrier to writing real AQL libraries. It
means you cannot name intermediate values and then collect them into
a list for further processing.

**Suggested fix:** When a list is consumed as a fn argument, resolve
any word elements that have DefStack entries before passing to the
handler. Alternatively, provide an explicit `eval` word that forces
list evaluation: `eval [c1 c2]` → `[map1, map2]`.


### Issue 2: Def leakage from `fn` bodies via CallAQL (critical)

**Severity:** Critical — causes silent, hard-to-diagnose failures.

Local `def` bindings inside an fn body persist in the registry's
DefStacks after the fn returns via CallAQL. This causes silent name
collisions in subsequent calls.

```aql
def eval-cond fn [[c:Map input:Map] [Boolean] [
  def op (c.op)        # 'op' persists in DefStacks after return
  apply-op op lhs rhs
]]
```

After calling `eval-cond`, a stale `op` def exists in the shared
registry. Any later code that uses `.op` in dot notation (which the
parser expands to `get op`) finds the leaked def value instead of
treating `op` as an atom for map key lookup.

This produced failures that were extremely hard to diagnose. The
symptom was "no matching signature for apply-op" in a completely
different function, because a dot-accessor like `c0.op` resolved
`op` from a leaked def rather than as a key name.

**Workaround:** Use ugly prefixed names (`__ec-op`, `__ao-lhs`) for
all local defs in fn bodies, or avoid local defs entirely.

**Suggested fix:** CallAQL should snapshot DefStacks entries before
running the body and restore them after, or track which defs were
created during body execution and clean them up (analogous to how
it already cleans up named parameters via `uninstallDef`).


### Issue 3: Arg ordering is confusing across contexts (moderate)

**Severity:** Moderate — causes development friction and bugs.

In prefix matching, `args[0]` is the top of stack (nearest to the
word). In FnDef `execFnDefLiteral`, `candidate[0]` is the deepest
value (first pushed). When a registered word runs inside a FnDef body,
the args may reverse again depending on matching mode.

```go
// For: c input eval-cond
// In the registered handler: args[0]=input (top), args[1]=c (deeper)
// Developer must manually swap from "natural" reading order
```

For same-type signatures like `[Map, Map]`, there is no way for the
engine to auto-correct the ordering based on types, so the developer
must manually reason about prefix vs forward vs FnDef layering at each
call site.

During the decision module implementation, getting arg ordering right
for the Go evaluators required repeated trial-and-error. The atan2
pattern (swapping args in the handler) is a known workaround but is
not documented as a general pattern.

**Suggested fix:** Document the arg ordering rules explicitly for
module/FnDef authors, with examples covering each context (direct
prefix, direct forward, FnDef auto-invoke, CallAQL nested). Consider
adding a convention comment pattern. Long-term, consider a mechanism
where named fn params always bind in declaration order regardless of
stack position.


### Issue 4: Registered words shadow map keys in dot notation (moderate)

**Severity:** Moderate — forces awkward naming choices.

Any registered word name becomes unusable as a module export key
because the dot-to-get conversion in
`eng/go/parser/parse.go::convertTopLevelItems` emits the key as a
bare Word token following the synthesized `get`:

```aql
matrix.trace    # executes 'trace' (debug word) instead of map lookup
matrix.make     # executes 'make' (type constructor) instead of map lookup
matrix.at       # executes 'at' (array gather) instead of map lookup
```

This forced renaming several matrix module exports:
- `trace` → `tr`
- `make` → `create`
- `at` → `elem`

The problem grows as more built-in words are added — each new word
potentially shadows module export names.

**Suggested fix:** at the conversion site in `convertTopLevelItems`,
emit the post-dot key as a `String` literal rather than passing the
next token through as a Word: `matrix.trace` → token sequence
`matrix get "trace"` instead of `matrix get trace`. This makes dot
access always work as a key lookup regardless of the word namespace.


### Issue 5: No way to build a list of evaluated values (moderate)

**Severity:** Moderate — blocks common data construction patterns.

There is no ergonomic way to create a list containing the evaluated
results of def'd names:

- `[c1 c2]` → unevaluated atoms when consumed as fn arg (Issue 1)
- `do [c1 c2]` → evaluates the list body, pushes results, but does
  not produce a list value
- `push` has a specific signature that doesn't compose easily inline
  (`push element list` where list must already be on the stack)
- `quote []` creates an empty list but there's no clean way to
  append evaluated values to it

**Suggested fix:** Add a `collect` or `list` word that collects N
values from the stack into a list:

```aql
c1 c2 2 collect    # → [map1, map2]
```

Or extend `stack-collect` to return a proper list value.


### Issue 6: FnDef values don't support forward argument collection (design limitation)

**Severity:** Design limitation — two calling conventions in one language.

Module words accessed via dot notation are FnDef values, not registered
words. They can only consume args already on the stack — no forward
collection:

```aql
math.sin 0.5        # BROKEN: 0.5 comes after, not on the stack
0.5 math.sin         # WORKS: arg before function

3 7 math.min         # WORKS: both args before function
3 math.min 7         # BROKEN: forward collection not supported
```

This forces a different calling convention from built-in words
(`add 1 2` works with forward, but `math.min 3 7` does not). Users
must learn two conventions depending on whether a word is built-in
or from a module.

The impact extends to composition in list bodies:

```aql
for 5 [i math.negate]     # must put arg before function
for 5 [math.negate i]     # BROKEN: negate can't forward-collect i
```

**Suggested fix:** Consider making `execFnDefLiteral` attempt forward
collection when prefix matching fails, analogous to how registered
words with forward arg collection work. This would unify the calling
convention.

---

## Impact on the decision module

These issues forced the decision module from a pure-AQL implementation
to a hybrid architecture:

- **AQL builders** (cond, make-rule, make-table, make-tree, etc.) —
  these are pure AQL. They work because they are simple data-in,
  data-out functions with no cross-fn calls, no iteration, and no
  recursion.

- **Go evaluators** (eval-cond, eval-pred, eval-table, eval-tree,
  decide) — these had to be implemented in Go because:
  - Recursive predicate evaluation (all/any/not groups) needs
    reliable cross-function calls (blocked by Issue 2)
  - Table rule iteration needs reliable list building with
    evaluated values (blocked by Issues 1 and 5)
  - Tree traversal needs reliable state management across fn calls
    (blocked by Issue 2)

A pure-AQL decision engine would have been approximately 80 lines of
AQL code. The hybrid is approximately 350 lines of Go code plus 30
lines of AQL. The Go code is more verbose but works reliably.

---

## Summary of suggested improvements

| Priority | Issue | Fix |
|----------|-------|-----|
| **P0** | Def leakage from fn bodies | CallAQL should clean up body-local defs on return |
| **P0** | List auto-eval strips def refs | Resolve word elements when list consumed as fn arg |
| **P1** | Dot notation shadowed by registered words | Emit string keys at the dot-to-get conversion site: `x.y` → `x get "y"` |
| **P1** | No list-building word | Add `N collect` to gather stack values into a list |
| **P2** | FnDef no forward collection | Extend `execFnDefLiteral` with forward attempt |
| **P2** | Arg ordering confusing | Document prefix/forward/FnDef arg ordering for module authors |

The P0 issues (def leakage and list evaluation) are the primary
blockers for writing non-trivial AQL libraries. Fixing them would
make the decision module — and similar data-processing libraries —
implementable entirely in AQL.
