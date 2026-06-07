# Silent forward-collection traps (DX investigations)

**Status:** investigations only — no code change. Two silent-failure traps
surfaced by the voxgig trie/bloom DX reports (`design/VOXGIG-AQL-REPORTS.0.md`,
bloom #3 and trie #6), both reproduced on the current build. Each is a case
where forward collection does something locally reasonable but globally
surprising, and fails **quietly** rather than loudly — the dominant cost the
DX reports call out.

---

## Trap 1 — a zero-value expression in an argument slot is silently skipped

### Symptom (bloom #3)

Binding the result of a void-returning call derails the *next* word:

```aql
def Bits (refine Object {})
def mark fn [ [i:Integer b:Bits] [] [ b 1 (convert String i) set ] ]
def b (make Bits {})
def _ (mark 5 b)
"ok" print            # error: no matching signature for print
```

### Scope (verified)

Broader than mutators or the `_` name — it is **any** zero-value expression in
**any** forward word's argument slot:

| Input | Result | |
|---|---|---|
| `def x (noop)` then `"ok" print` | `no matching signature for print` | any void fn |
| `def x (noop) 42` then `x` | `x = 42` | **silently binds the next token** |
| `def x () 42` then `x` | `x = 42` | empty paren `()` behaves identically |
| `def x noop 42` then `x` | `x = 42` | bare void word (no paren) too |
| `add 1 (noop) 5` | `6` | not `def`-specific |
| `add 1 (5 drop) 2` | `3` | any zero-value expression |
| `add 1 (if false [9] []) 2` | `3` | an empty `if` branch yields **zero** values |

where `def noop fn [[] [] []]`.

### Root cause

Forward collection counts **values arriving at the pointer**, not tokens. In
`stepLiteral`'s collection block (`eng/go/engine.go:2006-2049`) each value that
reaches the pointer while a `Forward` marker is pending is collected and
`CollectedArgs++` (`:2049`); the word fires when `CollectedArgs >=
ExpectedArgs`.

A zero-value expression produces **no arrival** — the paren simply collapses to
nothing and the pointer advances to the next real token, which is then
collected into that slot. Paren evaluation (`stepCloseParen` /
`evalParenGroupAt`) is architecturally decoupled from arg-slot accounting, so
the collecting word cannot tell *"my argument vanished"* from *"that was a
side-effect statement and this next token is my argument."* Hence the
mislocated failure: with a following token, the word binds the wrong one and a
*later* word starves; with none (`… end`), the word itself starves
(`no matching signature for def`).

### Why a blanket runtime error is wrong

"Zero values in an argument slot → error" is too aggressive: `(if c [v] [])`
with the empty branch taken legitimately yields zero values, so
`add 1 (if c [t] []) 2` (conditionally contribute a term) is reasonable AQL and
would start erroring.

### Recommendation (deferred — investigation only)

The two cases separate cleanly:

- **Binding words (`def`/`var`):** binding a name to a zero-value expression is
  never useful and today fails confusingly (wrong token bound, or an error two
  statements later). A clear, located error here is unambiguously better and
  safe — e.g. `def: (mark 5 b) produced no value to bind to "_" — give the call
  a return value, or call it as a bare statement`.
- **General words:** leave runtime behaviour (the conditional-term pattern is
  valid); add a **check-mode advisory** when a *void-declared* fn's result is
  consumed as a forward argument (return-type info is available statically).
  Non-gating, consistent with `forward_strands_operand`.
- **Docs:** note the trap + workarounds.

Implementation catch: the zero-value paren never reaches the collector, so a
binding-error must be raised at paren-collapse / forward-completion time by
consulting the pending `Forward`. Contained, but on the hot path; and "which
words are binding" is a language-design call with test blast radius (e.g. the
`addq 1 ( )` spec row, which currently expects a generic signature error).

---

## Trap 2 — a bare variable as a `get` key/index is captured as a literal atom

### Symptom (trie #6)

A bound variable used as an index/key is not resolved — it is taken as the
literal key-atom, yielding a silent `None`:

```aql
def xs [10 20 30]
def i 1
xs get i        # => None      (NOT 20)
xs get (i)      # => 20        (parenthesised: i evaluates to 1)
xs get 1        # => 20        (literal index)
```

It bit the trie author twice and passes tests vacuously — `None` propagates
quietly into later code.

### Scope & root cause (verified)

`get` is overloaded with an **`[Atom …]` key family** alongside the
`[Integer …]` / `[String …]` families (`describe get`):

```
[ [Integer Array]   Any ]   [ [String Node]  Any ]
[ [Integer Node]    Any ]   [ [Atom   Node]  Any ]   ← captures a bare word
[ [Atom    Object]  Any ]   [ [Atom   Store] Any ]   …
```

The `[Atom …]` signatures exist on purpose: they make `m get key` (and the
desugaring of `m.key`) work, where `key` is meant as a **literal name**, not a
variable. So during forward collection a following **bare word** is captured as
an `Atom` (the Word→Atom path in the collector, `eng/go/engine.go:2010-2017`)
and matched against `[Atom Node]` — *before* the def stack is ever consulted.
Consequences, all confirmed:

- `xs get i` → the atom key `"i"`, absent from the list → `None`.
- `xs get i` with `i` **undefined** → still `None` (not `undefined word`!) — the
  variable is never looked up, so the usual strict-undefined rule never fires.
- `m get a` where `a` is **both** a defined value (`0`) and a map key (`a:99`) →
  `99` — the literal-key interpretation wins; the binding is shadowed.

So this is the dot-access ergonomic (`m.key`) colliding with "variable as a
computed key/index." The only disambiguator today is parens: `xs get (i)`.

### Why it's nasty

The same property that makes `m.key` convenient — capture the bare word as a
name — silently defeats `get <var>`, and it specifically **suppresses** the
strict-undefined-word error that would otherwise catch the typo. `None` is a
valid value, so nothing downstream complains.

### Options (for discussion)

1. **Prefer a bound value over an atom key.** If the bare word resolves to a
   binding, use its value; fall back to the literal atom only for unbound
   names. Most intuitive, but a behaviour change to `m get key` dispatch and to
   dot-access desugaring, and ambiguous when a name is *both* a binding and a
   real key (the `a`/`99` case above) — picks the variable, which may surprise
   the dot-access reader.
2. **Check-mode advisory.** Flag `get`/`set` whose key is a bare word that is
   *also* a live binding (the likely "I meant the variable" case) and suggest
   `(name)`. Zero runtime change; consistent with the other advisories.
3. **Docs + a structural note.** Document that `get`/`set` keys are literal
   names; use `(expr)` for a computed key/index. Cheapest; leaves the silent
   `None` in place.

Recommendation: **(2) + (3)** — keep the dot-access ergonomic intact, surface
the likely mistake via `aql check`, and document the `(expr)` rule. (1) is the
"correct-feeling" fix but is the riskiest and changes a core, widely-relied-on
dispatch.

---

## Common thread

Both traps are forward collection making a locally-sensible choice that is
globally surprising **and silent**: Trap 1 reaches past a vanished argument;
Trap 2 captures a name where a value was meant. Neither is a crash or a clear
error — they produce wrong values or mislocated failures, which is exactly the
class the DX reports flag as the costliest. The consistent remedy direction —
loud where it is safe (binding words), advisory where runtime behaviour must
stay (general dispatch, dot-access) — mirrors the `forward_strands_operand`
advisory already shipped (`design/FORWARD-STRAND-ADVISORY.0.md`).
