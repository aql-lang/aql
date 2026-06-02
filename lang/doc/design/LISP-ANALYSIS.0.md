# AQL through a LISP/Scheme Lens — Analysis & Improvement Proposals

**Status:** analysis / design note
**Scope:** evaluates AQL's metaprogramming and functional-programming
surface against the classical LISP/Scheme tradition — *code as data*,
hygienic macros, combinators, higher-order functions, and the broader
"lisp-ish mentality" — and proposes concrete, prioritized improvements.

> AQL is **not** a LISP. It is a *concatenative* language (Forth/Joy/Factor
> lineage) with a **data syntax borrowed from jsonic/JSON** and a
> value-is-type lattice. The interesting question is not "is AQL a LISP?"
> but "which of LISP's powers does AQL's design already grant, which does
> it forgo, and which could it gain cheaply without betraying its
> concatenative, data-first identity?" That framing drives every
> recommendation below.

---

## 0. TL;DR scorecard

| LISP capability | AQL today | Grade | One-line verdict |
|---|---|---|---|
| Code as data (homoiconicity) | lists evaluate by default; `quote`, `word` splice, `do`/`call`, fn bodies are lists | **B** | Real and idiomatic, but "code" is a *list of tokens*, not a uniform tree; no reader/printer round-trip for all forms |
| Higher-order functions | `fn`/`afn` lambdas, lexical closures, `ref`/`usurp`/`apply`, `each`/`fold`/`scan`/`outer`/`inner` | **A−** | Strong; first-class functions with capture; only naming/coverage gaps |
| Combinators | implicit concatenative composition; `usurp`/`stack-args`/`forward-args`/`force-arity` (composable) | **B+** | Excellent point-free substrate; missing a few named staples (`compose`, `dip`, `keep`) |
| Macros | `word` (Forth-style splice), `NoEvalArgs` quotation | **C** | Unhygienic, name-capturing, no expansion model; powerful but a footgun |
| Hygiene / `gensym` | none | **D** | No fresh-symbol generation, no capture avoidance |
| `eval` / metacircularity | `do`/`call`/`module` run lists as sub-programs | **B−** | Eval-by-another-name exists; no first-class `eval`/`read`, no environment reification |
| REPL / dynamic mentality | REPL, dynamic defs, minimal core + words, type-as-value | **A−** | Very lisp-ish in spirit |

Net: **AQL already lives in the homoiconic, higher-order, REPL-driven
world LISP pioneered** — its data syntax doubles as code, lambdas are
first-class with closures, and the concatenative core is a combinator
playground. The two weakest dimensions are **macro hygiene** (effectively
absent) and the **uniformity / reflective completeness of "code as
data."** Those are where the leverage is.

---

## 1. Background: AQL's evaluation model in one screen

A few facts the rest of this note leans on (all from `lang/go/CLAUDE.md`
and `eng/go/CLAUDE.md`):

- **One data syntax, two contexts.** jsonic parses the source; the same
  literal is *word context* (top level / lists: unquoted text → callable
  Words) or *data context* (inside maps: unquoted text → atoms/values).
- **Lists evaluate by default.** `[1 add 2] → [3]`. A list is a *program
  fragment* that auto-evaluates when consumed as an arg or left on the
  stack. `quote` / the `/q` suffix / `NoEvalArgs` sig positions suppress
  this.
- **Words are the unit of behaviour.** Everything is a registered word
  dispatched by a single signature-matching rule (forward + stack
  collection at a `BarrierPos`).
- **Functions are values.** `fn`/`afn` (`=>`) build `Function` values with
  **implicit lexical capture** (closures). `ref` (`/r`) yields a function
  as inert data; `usurp` (`/u`) and the new `stack-args`/`forward-args`/
  `force-arity` words wrap a function to change its dispatch shape, and
  **compose**.
- **`word` is a splice marker** (`__SP`): `def doub word [dup add]` expands
  its body Forth-style at the call site — AQL's nearest thing to a macro.
- **Types are values** on a lattice; `def Foo …` binds a type, `make`
  instantiates, `refine` subtypes. Recent work added `Ideal/Module` +
  `Ideal/ModuleExport` and an `IdealConverter` capability.

So AQL's "S-expression" is the **evaluated list** `[...]`, and its
"special form" toolkit is **quotation** (`quote`, `/q`, `NoEvalArgs`) plus
**splice** (`word`).

---

## 2. Code as data (homoiconicity)

### What AQL has

LISP's defining trick — programs are the language's own primary data
structure — has a genuine AQL analogue:

- A bracketed **list is literally a program fragment**. `[dup add]` is
  both "a list of three tokens" and "the body that doubles its input."
- **`quote` / `/q`** turn the dial from *evaluate* to *inspect*:
  `quote [1 add 2] → [Integer(1), Word(add), Integer(2)]`; `foo/q → Atom(foo)`.
- **`do` / `call`** are *eval*: they run a list as a sub-program against
  the live stack (`do [body]`). `module [ … ]` runs a list as a module
  body.
- **fn/afn bodies are lists**, captured raw via `NoEvalArgs` and later
  executed — i.e. code stored as data and run on demand.
- **`word`** splices a quoted list's tokens into the instruction stream —
  compile-time-ish code assembly.
- The **parser is reusable at runtime** (`r.ParseFunc` / `lang.Parse`), so
  `read` (string → tokens) is available to the host and to module loading.

This is a real "code = data" story: you can build a list, inspect it,
quote it, splice it, and run it.

### Where it falls short of LISP

1. **"Code" is a flat token list, not a uniform tree.** In LISP every form
   is a cons cell; `car`/`cdr`/`cons` traverse *all* code uniformly.
   AQL's quoted body is a `[]Value` of heterogeneous tokens (Words, atoms,
   `OpenParen`/`CloseParen` markers, `Forward`/`End` markers, `ParenExpr`,
   `InterpString`). Paren grouping and dotted access are **markers in the
   stream**, not nested sub-lists, so a program that wants to *walk and
   rewrite code* must understand the marker tape, not just recurse on
   lists. (`WalkBodyWords` in `fn_capture.go` exists precisely because the
   stream isn't a clean tree.)
2. **No general `read`/`eval`/`print` triad at the language level.** `do`
   evaluates a list, but there is no `eval` that takes an arbitrary quoted
   value (atom, word, nested structure) and runs it with an explicit
   environment; no `read` word (string→code) surfaced to AQL; the
   *printer* (`canon`) round-trips most but not all forms (the earlier
   `typeof (module …)` empty-render bug, since fixed, is the family of
   problem).
3. **Quotation is positional, not structural.** Whether a list is code or
   data depends on *context* (`Eval` flag, `NoEvalArgs`, `Quoted`) decided
   at parse/dispatch time — not on an explicit `quote`/`quasiquote`
   wrapper you can see in the value. This makes "is this value code?" a
   non-local question.

**Verdict: B.** The homoiconic *spirit* is present and used daily
(quotation + splice + `do`). What's missing is **uniform structural
access to code** and a **reflective `read`/`eval`/`print`** surface.

---

## 3. Higher-order functions

### What AQL has — and it's strong

- **First-class functions**: `fn [[params][returns][body]]` and the `=>`
  lambda (`afn`). They are ordinary `Function` values.
- **Lexical closures with implicit capture** (`FnDefInfo.Captured`): inner
  fns snapshot enclosing-fn locals — the canonical `make-adder` factory
  works (`def add5 (make-adder 5); add5 3 → 8`).
- **Functions as data**: `ref`/`/r` produce a non-invoking function value;
  `apply` / `usurp` consume one; `Math.sqrt` (a module export) is a
  retrievable, passable function value.
- **The map/fold family**: `each`, `fold`, `scan`, `outer`, `inner`
  (`map`≈`each`, `reduce`≈`fold`). `filter` takes a `{key,value}`-pair
  predicate.
- **Dispatch-shaping combinators** (recent work): `usurp` (reverse args),
  `stack-args` / `forward-args` (force dispatch direction), `force-arity`
  (fix arity) — each returns a *new function* and they **compose**.

### Gaps

1. **Inconsistent / sparse naming vs the LISP/FP canon.** There is no
   `map`, `reduce`, `filter`-on-element (the pair-callback surprised even
   this analysis), `zip`, `flat-map`, `take`/`drop`-while as first-class
   higher-order words with predictable names. `each` vs `map`, `fold` vs
   `reduce` is a learnability tax.
2. **No point-free *value* combinators by name.** `compose`, `pipe`/`|>`,
   `curry`, `partial`, `flip` (≈`usurp` for 2-arg), `const`, `identity`.
   Concatenative juxtaposition *is* composition, but a named `compose`
   that returns a function value (so it can be stored, passed, and
   `convert`ed) is missing — and would slot naturally next to the new
   `usurp`/`stack-args` family.
3. **Predicate ergonomics.** `filter`'s `{key,value}` pair callback and
   the `[p:Any]` typed-param requirement are correct but unintuitive; a
   thin `where ([x] => …)` element-predicate wrapper would pay for itself.

**Verdict: A−.** The substrate (first-class fns + closures + the wrapper
combinators) is genuinely excellent. The gaps are *vocabulary*, not
*power*.

---

## 4. Combinators

AQL is, by construction, a **combinator language**: juxtaposition is
composition, and the stack is the implicit plumbing. This is the Joy/
Factor inheritance and it's a real strength.

- **Point-free is the default**, not a discipline you opt into.
- The **`/`-modifier + companion-word system** (`usurp`/`stack-args`/
  `forward-args`/`force-arity`) is a small, composable algebra for
  *adapting a function's calling convention* — closer to Factor's
  `dip`/`keep`/`bi`/`tri` shuffle combinators than to anything in
  Scheme, and it now composes cleanly (see `path-modifier.tsv §8`).

What's missing relative to Factor (the concatenative gold standard for
combinators):

- **Dataflow combinators**: `dip` (run under the top), `keep` (run but
  retain input), `bi`/`tri`/`cleave` (apply N quotations to one value),
  `bi*`/`spread` (apply N quotations to N values). These are the
  concatenative equivalent of `let`-binding and tuple plumbing; their
  absence pushes users toward `def`-ing temporaries.
- **A named `compose`** that yields a `Function` value (distinct from
  inline juxtaposition, which can't be stored).

**Verdict: B+.** Best-in-class substrate; missing the standard shuffle/
dataflow combinator vocabulary that makes deep point-free code readable.

---

## 5. Macros and hygiene

This is the weakest dimension — and the most LISP-defining one.

### What exists

- **`word` (splice / `__SP`)** is an unhygienic, Forth-style macro: it
  pastes a quoted body's tokens into the live stream, re-stepped against
  the stack. `def doub word [dup add]; 5 doub → 10`.
- **`NoEvalArgs`** lets a word receive its list arguments unevaluated —
  the mechanism behind `if`/`for`/`fn`/`do` — which is exactly LISP's
  *special form* / fexpr capability at the *native* (Go) level.

### What's missing

1. **No user-level macro definition.** You cannot write, in AQL, "a word
   that receives its arguments unevaluated, transforms them as data, and
   returns code to run." `NoEvalArgs` is a Go-side `Signature` field, not
   something `def`/`fn` can request. So *all* metaprogramming that needs
   unevaluated operands must be written in Go.
2. **No hygiene, no `gensym`.** `word` splices names verbatim; a macro body
   that introduces a temporary (`def tmp …`) will capture / be captured by
   the call site. There is no fresh-symbol generator, no syntactic
   renaming, no `syntax-rules`-style pattern macro.
3. **No quasiquote/unquote for code.** `InterpString` (backtick `${…}`) is
   *string* interpolation; there is no `` `(a ${x} c) `` that builds a
   *code list* with holes — the single most-used macro-construction tool
   in the Scheme world.
4. **No expansion phase / macro-expand introspection.** Splicing happens
   at the pointer during execution; there's no separate, inspectable
   expansion step (so no `macroexpand`, no compile-time error surface).

**Verdict: macros C, hygiene D.** AQL has the *primitive* (`word` splice +
`NoEvalArgs`) but not the *system*. For a language that already treats
lists as code, this is the biggest unrealized LISP dividend.

---

## 6. `eval`, environments, metacircularity

- **`do` / `call` ≈ `eval`** of a code list against the current stack;
  `module [ … ]` evaluates in a fresh sub-registry (a reified, isolated
  *environment* — closer to a first-class environment than most languages
  expose).
- **The registry is the environment**, and `import` now binds a
  *reflective* `ModuleExport`/`Module` pair (you can ask a module for its
  `id`, `kind`, `exports`) — a genuinely lisp-ish "the environment is
  data" move.
- **Missing:** a first-class `eval value` for arbitrary quoted values with
  an explicit environment argument; `read string → code`; the ability to
  capture / pass / extend an environment as a value (à la Scheme's
  first-class environments or Kernel's `$vau`).

**Verdict: B−.** Eval exists under other names; environment reification is
partial (modules) and would be powerful if generalized.

---

## 7. General "lisp-ish mentality"

Strongly present:

- **Interactive REPL**, dynamic (re)definition, "grow the language by
  adding words" — the Forth/LISP extensibility ethos.
- **Minimal core, everything-is-a-word** — like LISP's small special-form
  core plus a library.
- **Types are values** on a lattice (`type ≡ value`), and *behaviours* are
  attached to types (the `Comparer`/`IdealConverter` capabilities) — a CLOS/
  generic-function flavour: dispatch and conversion live on the type, found
  by an LCA walk up the lattice, with a base fallback. That is a more
  "lisp-ish" (open, extensible, late-bound) object story than nominal OO.

Tension with LISP:

- **Concatenative ≠ applicative.** No `(f x y)` nesting; arguments flow via
  the stack and `BarrierPos`. This is *more* uniform in some ways but means
  LISP idioms (deeply nested expressions, `let*` scoping) translate to
  paren-grouping + `def` temporaries, which can read less cleanly than
  S-expressions for nested data transforms.

---

## 8. Suggested improvements

Ordered by **leverage ÷ cost**. Each is framed to *fit AQL's
concatenative, data-first identity* — not to bolt a parenthesised LISP on
top.

### Tier 1 — high leverage, moderate cost

**1. A hygienic-ish macro facility (`macro` + `gensym` + quasiquote).**
This is the headline gap. Concretely:

- `gensym` word → a fresh, never-colliding `Atom`/Word name. Cheap
  (a counter on the registry); unlocks capture-free temporaries today,
  even for hand-written `word` macros.
- **Quasiquote for code**: a `` `[ … ${expr} … ] `` form (or a
  `quasi`/`unquote` word pair) that builds a **code list** with evaluated
  holes — the dual of the existing string interpolation. Reuse the
  `InterpString` machinery, but emit a `[]Value` (tokens) instead of a
  String.
- `macro name [params] [body]` (or `def name (macro …)`): a `def`-level
  way to mark a word as receiving operands **unevaluated** (surface the
  existing `NoEvalArgs` to user code) and to return code to splice. Even
  *non-hygienic* user macros + `gensym` would move metaprogramming out of
  Go and into AQL — the single biggest "become more of a LISP" step.

*Why first:* AQL already lists-are-code and has `word`/`NoEvalArgs`; this
is assembling existing parts into a usable system, and it is the dimension
where AQL is furthest from its own potential.

**2. Combinator vocabulary: `compose`, `dip`, `keep`, `bi`, `flip`,
`identity`, `const`.** Implement as wrapper words returning `Function`
values, exactly like the new `usurp`/`stack-args` family (and make them
compose with it). `flip` is `usurp` restricted to 2 args; `compose f g`
returns `(g then f)` as a value. This closes the Factor-combinator gap and
makes deep point-free code readable without `def` temporaries.

**3. Canonical higher-order names: `map`, `reduce`, `filter` (element
form), `zip`, `flat-map`.** Add as aliases / thin wrappers over
`each`/`fold` and an element-predicate `filter`. Pure learnability win;
keep the existing pair-callback `filter` available as `filter-pairs`.

### Tier 2 — high leverage, higher cost (design required)

**4. First-class `eval` and `read`.** `eval (quote …)` / `eval <list>`
against the current (or a passed) environment; `read "src" → code`
(surface `lang.Parse`). Pairs naturally with #1 (macros need to *return*
code that something *evals*).

**5. Uniform code as data — a structural view.** Give quoted code a
**list-shaped** API: make paren groups and dotted access recursively
*walkable as nested lists* (or provide `code->list` / `list->code` that
normalize the marker tape to/from a real tree). This is what lets users
write code-walking macros and optimizers in AQL itself, the way LISP walks
cons cells. It is the deepest change (touches the parser's marker model)
but it is what would move "code as data" from **B** to **A**.

**6. First-class environments.** Generalize the module sub-registry into a
reifiable `Environment` value: capture the current bindings, extend, eval
in. AQL is unusually close (modules already do this internally); exposing
it would give Scheme-grade reflective power and a clean substrate for
sandboxed `eval`.

### Tier 3 — nice to have / longer horizon

7. **`syntax-rules`-style pattern macros** layered on #1+#5 (real
   hygiene via automatic renaming, not just `gensym`).
8. **Tail-call guarantee** for the recursion idiom (the docs lean on
   forward-ref recursion; confirm/guarantee it doesn't grow the Go stack).
9. **Continuations** (`call/cc` or delimited `shift`/`reset`) — powerful
   but a large undertaking and arguably against the grain of the
   stack-machine model; list as a research item, not a near-term goal.
10. **`printer` completeness audit** — guarantee `read ∘ print` round-trips
    every value form (the retired `Word/__MD` empty-`typeof` was an
    instance of this class).

---

## 9. Recommended roadmap

| # | Item | Tier | Effort | Moves which grade |
|---|---|---|---|---|
| 1 | `gensym` | 1 | S | hygiene D→C |
| 2 | quasiquote-for-code | 1 | M | code-as-data, macros |
| 3 | user `macro` (surface `NoEvalArgs`) | 1 | M | macros C→B |
| 4 | `compose`/`dip`/`keep`/`bi`/`flip`/`const`/`identity` | 1 | S–M | combinators B+→A |
| 5 | `map`/`reduce`/element-`filter`/`zip` names | 1 | S | HOF A−→A |
| 6 | first-class `eval`/`read` | 2 | M | eval B−→A− |
| 7 | structural code↔list view | 2 | L | code-as-data B→A |
| 8 | first-class environments | 2 | L | eval/metacircularity |
| 9 | `syntax-rules` hygiene | 3 | L | hygiene C→A |
| 10 | tail-call guarantee; printer round-trip audit | 3 | S–M | robustness |

**Smallest high-value slice:** ship **#1 `gensym` + #4 combinators +
#5 names** first (all small, all pure additions, no engine surgery), then
tackle **#2 quasiquote + #3 user macros** as the flagship
"AQL-gets-real-metaprogramming" milestone.

---

## 10. Closing assessment

AQL is **not** a LISP, and shouldn't try to be one — its concatenative
core and jsonic data syntax are a coherent, distinctive identity. But it
sits much closer to the LISP tradition than its surface suggests: **lists
are code, functions are first-class values with closures, the environment
(registry/modules) is partly reflective, and types carry open,
late-bound behaviours.** Two LISP superpowers remain largely unrealized —
**a real (hygienic) macro system** and **uniform, walkable code-as-data** —
and both are *assembling existing primitives* (`word`, `NoEvalArgs`,
`quote`, the parser, the marker tape) into a coherent surface rather than
inventing new machinery. Those are the investments that would let AQL
honestly claim LISP's most enduring promise: **a language you extend in
itself.**
