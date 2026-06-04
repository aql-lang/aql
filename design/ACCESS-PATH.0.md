# AccessPath — a first-class node for dot-access sugar

**Status:** design / implementation plan
**Scope:** lift dotted access (`m.a.b`, `a."x".c`, `a.(expr).b`, `a!.x`) from
its current flat `get`-chain (a `ParenExpr` after the paren-nesting work) to a
**first-class, quotable, round-trippable `AccessPath` value** with per-segment
operator and literal-or-computed keys.
**Predecessor:** `PAREN-REPRESENTATION.0.md` §6/§9 (the "dotted access is the
residual" finding) and `LISP-ANALYSIS.0.md` §2/§8 #5 (uniform code-as-data).
**Decisions (this plan):** first-class runtime value · motivation = code-as-data
**and** programmatic paths · computed keys supported in the node.

> The dot-chain is the last non-uniform corner of "code as data": today
> `m.a.b` is an *idiom* (`( m get a get b )`), not a *node* — you can't quote
> it as a path, the printer can't round-trip it as `m.a.b`, and a code-walker
> must pattern-match the get-chain. This plan makes a path a real value:
> inspectable, constructible, applicable, and rendered back as `m.a.b`. It
> also removes the Step-7 `dotchain` +17% (no more `ParenExpr`→marker
> round-trip on access).

---

## 1. Current grammar (confirmed)

`convertTopLevelItems` (parse.go) accepts: a **receiver** (`isChainReceiver` —
any value/word/string/number/paren group) followed by one or more **segments**,
each `.key` → `get` or `!.key` → `getr`, where the **key** is a literal
(word/string/number) *or* a computed `(expr)`. The whole chain lowers to a
single `ParenExpr([recv get/getr k …])`. A `/`-modifier applies to the **whole
path result** (`/u`/`/s`/`/f`/`/N` → prefix words; `/r`/`/q` → suffix `__DM`).

Verified forms: `a.x.c`, `a."x".c`, `a.(k).c`, `m.(key)`, `a.0`, `a!.x`,
`(m.a).b`. `get` is lenient (missing → `None`); `getr` is strict (missing →
error). **`Scalar/Path` is unrelated** — it is the *filesystem* path type
(`PathInfo{Parts []string, Abs bool}`); this node must be **new** and
differently named.

---

## 2. The type and payload

A new kernel-declared type (the parser emits it, so eng must know it at parse
time — like `Word/__PE` for `ParenExpr`), placed in the **Ideal** branch for
first-class value semantics:

```
Ideal/AccessPath        // typeof → AccessPath; inspectable, quotable
```

Payload (eng `payload.go`, sealed marker):

```go
type AccessPathInfo struct {
    Receiver []Value     // tokens of the receiver expression (e.g. [Word("m")],
                         //   or a ParenExpr for (expr).k); evaluated to the base
    Segments []PathSeg
    Eval     bool        // evaluate-by-default (like list Eval); quote/codequote suppress
}

type PathSeg struct {
    Getr     bool        // false = get (lenient) · true = getr (strict)
    Computed bool        // true → Key is a sub-expression to evaluate at access
    KeyLit   Value       // literal key (Atom / String / Integer) when !Computed
    KeyExpr  []Value     // computed-key tokens when Computed (e.g. (expr))
}
```

`FixedID`: pick from the documented `5000–9999` kernel/language range (Module
is 5000, ModuleExport 5001 — use the next free slot, e.g. **5002**); add to the
`fixedid_stability_test.go` snapshot. Behavior: a custom `TypeBehavior` whose
`Format` renders the canonical dotted surface (§5) and an `IdealConverter`
projecting to an inspectable map (§4).

---

## 3. Parser change

In `convertTopLevelItems`, the dot-chain branch builds an `AccessPath` value
instead of a `ParenExpr` get-chain:

- **Receiver** — `emitPrimary` the first item into `Receiver` tokens (a word,
  scalar, or a nested `ParenExpr` for `(expr).k`).
- **Each segment** — record `Getr` (`.` vs `!.`), and the key: a literal
  (`KeyLit` — the word becomes an Atom, string/number as-is) or, for a paren
  key, `Computed=true` with `KeyExpr` = the paren's tokens.
- **Modifiers** — unchanged: the `pfx`/`sfx` tokens still wrap the resulting
  `AccessPath` value exactly as they wrap the group today (`/u`→`usurp (path)`,
  `/r`→`(path) __DM`). The node is a single value, so the existing wrap logic
  applies verbatim.

This stays within the single-pass converter — no new rewrite pass.

---

## 4. Evaluation

An `AccessPath` with `Eval=true`, when **stepped** at the pointer / **collected
unquoted** as a forward arg / **left on the stack** at end of Run,
**auto-evaluates** to the accessed value — mirroring list `Eval` semantics, so
`m.a.b` stays eager exactly as today. `quote`/`codequote` suppress it (§6).

Two implementation stages:

1. **Correctness-first (lower to get-chain).** Evaluate by materialising the
   `recv get/getr k …` token span (computed keys splice their `KeyExpr`) and
   running it in place — the same proven mechanism `ParenExpr` uses
   (`expandParenExpr`/in-place collapse). Guarantees identical get/getr
   semantics, recorder transparency, and the four paren contracts.
2. **Perf (direct walk).** Replace the lowering with a direct evaluator: eval
   `Receiver` → base, then for each segment call the `get`/`getr` handler
   directly with the literal/evaluated key. This removes the Step-7 `dotchain`
   +17% (no marker round-trip) and is the perf dividend. Gate behind the same
   spec suite; ship only if benchmarks confirm the win.

Semantics are **unchanged** — the node lowers to / calls the same `get`/`getr`.

---

## 5. Canon round-trip (the code-as-data win)

`canon`/`Format` renders an `AccessPath` back to its surface:

- `m` `.a` `.b` → `m.a.b`; getr segment → `!.`; string key → `."x"`; computed
  key → `.(expr)` (canon of the `KeyExpr`); receiver that is a `ParenExpr` →
  `(expr).k`.
- Guarantee `read ∘ print` round-trips: `codequote (m.a.b)` then `canon` →
  `m.a.b`. Add to the printer round-trip audit (LISP-ANALYSIS §8 #10).

This is what makes a quoted path *walkable* (one node with `Receiver` +
`Segments`) instead of a flat `get`-chain idiom.

---

## 6. First-class surface (motivation = "both")

Because the answer was a **first-class value** + **programmatic paths**:

- **Quoting** — `codequote m.a.b` captures the `AccessPath` raw (the
  `RawParens`/Quoted mechanism generalises: a non-evaluated `AccessPath` is
  data). `quote`/`codequote` set `Eval=false`/`Quoted`. So a macro can capture
  and inspect a path.
- **Constructor** — a `path` word for programmatic construction:
  `path m [a b]` → an `AccessPath` over receiver `m`, literal segments `a`,`b`
  (all `get`). A list-of-segments form covers getr/computed via a small segment
  encoding (e.g. atoms = get, `!name` = getr, `(expr)` = computed). Mirrors how
  `fn`/`module` build their structured values.
- **Apply / access** — evaluating the path *is* applying it; additionally allow
  `apply` on an `AccessPath` value, and let it interoperate with the
  `aql:struct-util` `getpath`/`setpath`/`inject` family (those take a path as a
  list of keys today). An `AccessPath` should be accepted wherever a path-list
  is — `setpath path val m`, `getpath path m` — unifying the two notions of
  "path".
- **Inspection** — `inspect`/`convert Map` → `{receiver, segments:[{op,key}…]}`;
  `typeof m.a.b`-as-value → `AccessPath`. So tooling and macros can read it.

---

## 7. Touchpoints

- `eng/go/types.go` + `typetable.go` — declare `Ideal/AccessPath`, FixedID.
- `eng/go/payload.go` — `AccessPathInfo` + `PathSeg` markers.
- `eng/go/value.go` — `NewAccessPath`, `AsAccessPath`, `IsAccessPath`; behavior.
- `eng/go/parser/parse.go` — `convertTopLevelItems` dot-chain → `AccessPath`.
- `eng/go/engine.go` — main-loop / forward / `stepLiteral` evaluate an
  `Eval` `AccessPath` (Stage 1 lowering; Stage 2 direct walk); honor
  `Quoted`/`Eval` for quotability.
- `eng/go/canon.go` (or wherever rendering lives) — round-trip rendering.
- `lang/go/native/` — `path` constructor word; `apply`-on-path; teach
  `getpath`/`setpath`/`inject` to accept an `AccessPath`.
- `lang/go/native/help/` + `describe` — document `path` and the node.
- snapshots: `fixedid_stability_test.go`, fnmodel golden (new `path` word).

---

## 8. Test plan

- **Parse**: every §1 form produces an `AccessPath` with the right
  receiver/segments (op + literal/computed); modifiers wrap it.
- **Eval parity**: `a.x.c`, `a."x".c`, `a.(k).c`, `a.0`, `a!.x`, `(m.a).b`
  return the same results as today (Stage-1 and Stage-2 evaluators both).
- **getr strictness** preserved (missing key errors; `get` → `None`).
- **Round-trip**: `canon (codequote m.a.b)` → `m.a.b`; string/computed/getr
  segments render correctly; `read ∘ print` idempotent.
- **First-class**: `path m [a b]` builds an equivalent node; `getpath`/`setpath`
  accept an `AccessPath`; `inspect` projects the structure.
- **Quotability**: `codequote m.a.b` is a non-evaluated `AccessPath`;
  `m.a.b` (bare) still evaluates eagerly.
- **Negative**: malformed `path` args; computed key that errors propagates;
  `AccessPath` type-literal no-panic (`TestTypeLiteralNoPanic`).
- **Perf**: re-run the Step-7 `dotchain` benchmark — Stage 2 should erase the
  +17%.

---

## 9. Phasing (each gated green)

| Phase | Deliverable |
|---|---|
| A | Type + payload + `NewAccessPath`/accessors + behavior stub (no parser change yet; construct in tests). Lattice/FixedID snapshot. |
| B | Parser emits `AccessPath` for dot-chains; **Stage-1 lowering** evaluator. Full suite green at semantic parity (this is the activating flip, like paren Step 1). |
| C | Canon round-trip rendering + quotability (`Eval`/`Quoted`); spec rows. |
| D | First-class surface: `path` constructor, `apply`, `getpath`/`setpath` accept `AccessPath`, `inspect`/`convert`. |
| E | **Stage-2 direct-walk** evaluator; re-benchmark (erase the +17%). |

Phases A–C deliver the code-as-data dividend (quotable, round-trippable path);
D delivers the programmatic-path capability; E is the perf optimisation.

---

## 10. Risks & open questions

1. **Type placement** — the parser (eng) emits `AccessPath`, so the type must be
   eng-declared even though it is "Ideal/first-class." Confirm eng-declared
   Ideal is acceptable (precedent: `Word/__PE` ParenExpr is eng-declared;
   `Ideal/Module` is lang-declared but the parser does *not* emit it — here the
   parser does, so eng it must be).
2. **Two notions of "path"** — `Scalar/Path` (filesystem) vs `Ideal/AccessPath`
   (data access). Keep them distinct; do **not** overload. The `getpath`/
   `setpath` words currently take a **list** of keys — accepting an
   `AccessPath` there is additive (a third accepted shape), not a replacement.
3. **Eager-by-default vs value** — `m.a.b` must stay eager (every existing
   program relies on it). The `Eval` flag + quote-suppression is the same
   contract lists use; confirm no path is left unevaluated on the stack
   unintentionally (end-of-Run `autoEvalStack` must evaluate an `Eval`,
   non-`Quoted` `AccessPath`).
4. **Computed-key side effects / ordering** — a computed key `(expr)` evaluates
   at access time, left-to-right; preserve current ordering (Stage-1 lowering
   gets this for free; Stage-2 must replicate).
5. **`path` segment encoding** — the list form needs a compact way to express
   getr and computed segments (`!name` for getr? `(expr)` for computed?).
   Decide the surface before Phase D.
6. **Scope of `getpath`/`setpath` unification** — how far to merge the
   list-path and `AccessPath` notions (Phase D) is itself a sub-decision; the
   minimal version is "accept an `AccessPath` by lowering it to a key list."

**Recommended first slice:** Phases **A + B** (type + parser flip + Stage-1
lowering) — that lands the structural node at exact semantic parity, gated like
the paren flip; then C (round-trip/quotability) is the code-as-data payoff and
the rest is additive.
