# Paren Representation — Flat Markers vs. Nested Sub-lists

**Status:** design / analysis note
**Scope:** evaluates representing word-context parenthesis groups as
**nested sub-list values** (like the existing data-context `ParenExpr`)
instead of inline `OpenParen … CloseParen` markers on the tape. Records the
empirical semantics that constrain the change, a preservation checklist,
and the residual **dotted-access** problem.
**Related:** `MACROS.0.md` (the marker-tape note this refines),
`LISP-ANALYSIS.0.md` §2 + §8 #5 (uniform code-as-data).

> The motivating context is the macro system: a quoted program is only a
> clean tree if grouping is structural. Today it isn't — paren grouping is
> a pair of inline markers, and dotted access desugars *into* those
> markers. This note asks what nesting parens would buy and cost, grounded
> in tested behavior rather than assumption.

---

## 1. The two representations AQL already has

The parser emits **different things** for parens in the two contexts
(`eng/go/CLAUDE.md` calls this divergence a complexity source):

- **Word context** (top level, lists): inline markers
  `OpenParen … CloseParen` on the single `e.stack` tape. Evaluated in place
  by `preEvalParens` (forward collapse) and `stepCloseParen` (span splice).
- **Data context** (inside maps): a single `ParenExpr` **value** carrying a
  nested token slice, run in a sub-engine by `autoEvalMap`.

The proposal: make word-context parens also a nested `ParenExpr`-style
value, unifying the two.

---

## 2. Empirical semantics of a word-context paren (tested)

Probed with `aql -e` on this branch. These are the contracts any new
representation must reproduce.

| # | Probe | Result | Contract |
|---|---|---|---|
| 1 | `10 (add 5)` | **error** (add can't see `10`) | inner expr does **not** consume the enclosing stack |
| 2 | `5 (dup)` | **error** | same — `OpenParen` is a **stack barrier** |
| 3 | `(1 2 3) 99` | `1 2 3 99` | results **flow out** onto the shared stack |
| 4 | `add 10 (add 1 2)` | `13` | a paren result is consumable by an outer forward word |
| 5 | `(def x 1) x` | `1` | defs **leak** to the enclosing scope |
| 6 | `(1 div 0) 99` | **halts** | errors **propagate** (unlike `do`, which catches) |

**The key correction to the MACROS.0.md note:** a word-context paren is
*already* a semantically **isolated sub-expression** for consumption — its
contents cannot reach back and pop values pushed before the `(`. The
`OpenParen` acts as a stack barrier; only *results* splice back out. So the
consumption-isolation a nested value would impose is **not a behavior
change** — it is the current behavior, merely made explicit in the
representation. This removes the main objection to nesting.

---

## 3. What nesting would change

Mostly *deletion* plus a representation swap, because §2 shows the
isolation contract already matches a nested value:

1. **`preEvalParens`'s forward scan disappears.** A nested group is a
   *single* forward token; the matcher evaluates it **on demand** when it
   needs the value/type — the "preEvalParens on demand" idea, now natural.
   No scanning for the matching `)`.
2. **`stepCloseParen` + splice machinery disappears** (~150 lines): no
   `CloseParen` case, no `stackRemove` span collapse, no
   `findCloseParenAfter`, no orphaned-forward checks. "Collapse the paren"
   becomes "evaluate this sub-list and splice its result(s)" — the
   `do`/`autoEvalMap` pattern.
3. **The two paren contexts unify** to one representation and one eval
   path, removing the parser's word-vs-data divergence.
4. **Quoted code becomes a real tree.** `quote ( a ( b c ) d )` would be
   `[a [b c] d]`. The macro walker (`unquote`/`splice` resolution) becomes
   plain tree recursion instead of a marker-tape span walk; `WalkBodyWords`
   simplifies. This is the LISP-ANALYSIS §8 #5 dividend (code-as-data B→A).

---

## 4. Preservation checklist (the four contracts)

A nested-eval implementation must reproduce each §2 contract; each is now a
*deliberate choice* rather than a free consequence of the flat tape:

| Contract | Evidence | How to preserve |
|---|---|---|
| Inner can't consume outer stack | `5 (dup)` errors | **Free** — fresh sub-eval starts with an empty operand view |
| Results flow out, splice onto shared stack | `(1 2 3) 99 → 1 2 3 99` | return N values and splice them (the `do`/`autoEvalMap` return-list pattern) |
| Defs leak to enclosing scope | `(def x 1) x → 1` | evaluate on the **same registry** (`New(r)` shares `r.Defs`) — not an isolated env |
| Errors **propagate**, not caught | `(1 div 0) 99` halts | use a **propagating** eval, NOT `do`'s catch-and-reify `doEvalList` |

The last row is the sharp one: implementing paren-eval as literally `do`
would silently flip parens from error-transparent to error-catching.

---

## 5. Costs

1. **Per-paren allocation / eval frame.** Inline markers are nearly free; a
   nested value is a heap token-slice and naive sub-engine spawning adds
   per-paren setup. Mitigation: a lightweight recursive eval frame on the
   same registry (no full sub-engine), offset by deleting `stepCloseParen`
   + the `preEvalParens` scan. Net perf is plausibly neutral — **measure,
   don't assume**.
2. **Execution-model shift: flat-tape walk → tree walk.** The single flat
   stream is the Forth/Joy/Factor essence; structural nesting nudges toward
   an applicative shape. But §2 shows parens are *already* isolated
   eval-units, so the representation is merely catching up to the
   semantics; the identity drift is smaller than it appears.
3. **Dotted access stays flat** (see §6) — so code-as-data is improved but
   not fully uniform by this change alone.

---

## 6. The dotted-access problem

Even with parens nested, **dotted access remains a code-as-data hazard**,
and it is worth understanding precisely because it *compounds* the paren
issue rather than being independent of it.

### 6.1 What the parser actually does

`convertTopLevelItems` (parse.go:204–294) desugars a path **eagerly, at
parse time**, into a `get`/`getr` chain **wrapped in a paren group**:

```
m.a.b        →   ( m get a get b )
m!.a         →   ( m getr a )
(expr).k     →   ( (expr) get k )
m.(expr)     →   ( m get (expr) )      # computed key
```

The paren wrapper exists to **isolate the path from forward collection**,
so `size m.a` means `size (m.a)`, not `(size m).a` (verified: `size m.a`
on `{a:[1 2 3]}` → `3`). The chain is **flat and left-composing**: each
`get` leaves the receiver for the next, so one group covers any depth.

### 6.2 Why this is a problem for code-as-data

1. **It does not round-trip as itself.** `.` is gone after parsing; the
   surface path is unrecoverable as a node. Tested:
   `def m {a:{b:2}}  quote m.a.b` yields **`2`**, not a quoted path — the
   group simply evaluates. You cannot easily get the *path* as data; the
   printer would have to *reverse-engineer* the `( recv get k … )` idiom to
   re-emit `.a.b`. Round-trip fidelity hinges on the printer reconstructing
   sugar from the desugared form — fragile, and the family of the retired
   `typeof (module …)` empty-render bug.

2. **It is a flat concatenative chain, not an access tree.** A Lisp-style
   walker wants `(get (get m a) b)` — explicit nesting that says *b accesses
   the result of a accessing m*. AQL produces `m get a get b`, where that
   nesting is **implicit in left-to-right stack composition**, not in
   structure. So even after parens become sub-lists, a path lowers to a
   *flat* `[m get a get b]`, and "this is a path" is still an idiom to be
   pattern-matched, not a node to be read.

3. **It compounds the paren-marker non-uniformity.** Because the path
   desugars *into* a paren group, it inherits every marker-tape problem of
   parens. Nesting parens helps (the wrapper becomes a sub-list) but does
   **not** turn the get-chain into a path node — the two issues stack.

4. **The `/`-modifier interaction forces non-local parser logic.** A
   modifier on a path applies to the **whole group**: `a.b/m` parses as
   `(a get b)/m`. `convertTopLevelItems` must peel the modifier off the
   *final key* (`groupModifier`), then place **word** modifiers
   (`usurp`/`stack-args`/`forward-args`) *before* the group (so they
   forward-collect the result before it auto-dispatches) and the
   `/r`/`/q` `__DM` marker *after* (for `execFnDefLiteral` to peek). This
   `pfx`/`sfx` repositioning is exactly the kind of look-ahead rewrite the
   single-pass model otherwise avoids — dotted-access-with-modifiers is one
   of the few genuine complexity hotspots in the linear converter.

5. **The post-dot key is keyword-like, a special case.** In word context a
   bare word is callable, but the key after `.` must be a self-quoting
   atom, not a dispatched word (the `m.trace` shadowing issue, LISP-ANALYSIS
   App. A #4). `get` captures the key without dispatching it — correct, but
   a positional special case rather than a uniform rule.

### 6.3 Implication

To make code genuinely uniform (the macro/optimizer dividend), dotted
access would need to lower to something **structural and quotable** — e.g.
a first-class `Path`/access node `(path m a b)` that the printer round-trips
and a walker reads directly — rather than an eagerly-desugared, paren-
wrapped, flat `get`-chain. That is a larger change than nesting parens and
is logically *downstream* of it: nest parens first (structural grouping),
then lift dotted access from "sugar that desugars into grouping" to "a node
within the tree."

---

## 7. Verdict

The marker-tape note in MACROS.0.md undersold the paren change: because the
consumption-isolation it would seem to break is **already the live
semantics** (§2), nesting parens is a *simplifying* engine change (delete
two big mechanisms, unify the two contexts) whose payoff is the homoiconic
tree the macro system wants. The blocker is not correctness but (a) the
per-paren eval cost and (b) committing to a tree-walk execution shape in a
concatenative engine.

Dotted access is the **residual**: even after nesting, a path is a flat,
non-quotable `get`-chain that drags non-local modifier logic into the
parser. Full code-as-data uniformity therefore wants **two** steps —
nest parens (this note) and then make dotted access a structural node —
with the second downstream of the first.

**Recommended order:** prototype nested parens behind the §4 four-contract
checklist; measure per-paren cost; then revisit dotted access as a
structural `Path` node once grouping is a real tree.
