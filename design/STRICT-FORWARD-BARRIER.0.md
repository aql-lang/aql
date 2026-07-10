# Strict Forward Barrier (prototype)

Status: **experiment** — prototype gated behind `AQL_STRICT_BARRIER=1`,
default off (zero behaviour change when unset). This note records the
motivation, the measured fallout, and the open design questions. It is
NOT shipped semantics.

## The question

Today a function word beginning its own dispatch is a forward-collection
barrier *only when honouring the barrier is possible*: `commitBarrierForward`
(engine.go) commits the nearest parked forward iff a real overload
consumes exactly the args it already holds. When the parked word CANNOT
fire — `print` holding 0 of 1, a 2-arg-only fn holding 1 — it keeps
waiting, and the boundary word's **result** arrives into the open slot.
That wait-through is what makes these work without parens:

```aql
print add 1 2        ;# add's result feeds print
size iota 3          ;# iota's result feeds size
not not 5            ;# unary chaining
typeof gensym        ;# a nullary fn as an argument
```

The wait-through exists only for slots an unknown token cannot be pruned
against — in practice `Any` slots and untyped user-fn params. Typed
native slots (`get`, `add`) already fail loudly at plan time with the
"group the call in parens" hint. So the same surface shape is legal or
illegal depending on the *waiting* word's overload set, and adding an
overload to a word can silently re-bind existing call sites downstream
(`print add 1 2` works precisely because print has no 0-arg overload —
give it one and the line changes meaning).

The strict rule under test: **a function word never feeds forward
collection.** When a fn word begins dispatch and the pending forward
cannot commit, raise the same signature error the typed plan-time path
gives, instead of waiting.

## The prototype

`AQL_STRICT_BARRIER=1` (engine.go: `strictForwardBarrier`,
`strandedForwardError`, called from the two `commitBarrierForward` call
sites — `stepWord` and `stepWordUsurp`). One exemption: engine-internal
boundary words (`__`-prefixed frame-tail words) are not source-level
statement boundaries. There is deliberately NO def exemption — see the
ruling below.

## The ruling: `def foo add 1 2` is an error; keyword slots carry the idiom

An earlier revision exempted `def` — first by name, then via a
`Signature.RestArgs` "statement-rest slot" attribute — so that ANY
function word could keep feeding def's body slot. Both were rejected:
`def foo add 1 2` silently binding `3` is exactly the wait-through
class the strict rule exists to kill, and a Go-only signature
attribute violates the language's expressibility principle — AQL has
macros, so whatever def can declare, an AQL-authored binder must be
able to declare too.

What survives instead is the **KEYWORD slot** — the language-native
way to say "this argument position matches the literal word `fn`":

> A signature position with `/q` (QuoteArgs) AND a concrete **Atom
> pattern** admits exactly one literal word, matched by raw token
> NAME, binding-agnostically ("capture trumps binding" — the same
> rule every /q capture follows). Any other token makes the
> signature non-viable at that position.

def declares the fn-definition FORM as an ordinary overload
(native_definition.go):

```
[ name:Atom/q  fn:Atom/q=fn  sigs:List ]     ;# def name fn [in out body …]
```

All three operands resolve at plan time — nothing parks, no
wait-through — so `def f fn [...]` is pure structural dispatch and
works identically under the strict rule, while `def x add 1 2`,
`def s size [1 2 3]`, and `def x:T add 1 2` are stranded (write
`def x (add 1 2)`). This is Scheme's syntax-rules literals arriving
in AQL signatures: the same mechanism is what user macros/binders
need for DSL keywords (`for x in xs […]`).

Three kernel seams make keyword slots sound (all in this change):

1. **`patternsOk` quote-slot fix** (eng/go/match.go): a /q position's
   Atom pattern matches the raw token name (Word or Atom). It must
   NOT resolve the binding first — `fn` is Defs-bound to an
   FnDefInfo, which would never unify with `Atom("fn")`; the
   pre-existing resolve-then-unify order made keyword patterns
   dead-on-arrival.
2. **Token-aware capture** (`capturesForwardToken`, engine.go): the
   plan scan's function-word barrier consults the TOKEN — a keyword
   slot captures only its own literal; every other word stays a
   barrier. Without this, one keyword sig would disable the barrier
   at its position for every def statement (the cross-barrier
   pre-evaluation bug class).
3. **Keyword viability pruning** (`resolveForwardArgs`): the keyword
   overload's larger arity must not widen the pre-evaluation scan —
   `def g (fn […]) (g 3)` must not evaluate `(g 3)` before the 2-arg
   def binds g. A signature whose keyword slot cannot match the token
   at its position is pruned before any group evaluation.

### Known strict-mode fallout of the ruling (accepted, migration due)

Bare-word def bodies OTHER than `fn` now strand under the strict rule:
`refine` 485, `class` 463, `word` 311, `gen` 261 (including the
`def Name<T>` parser sugar, which desugars to `def Name gen [params]`
— eng/go/parser/parse.go:448), `Module.word` 82, `enum` 42, `make`
40, `surface` 41, `fnsig` 28, `quote` 24, plus open-ended shapes
(`def x add 1 2`). Each closed-set constructor can get its own
keyword overload (`gen` needs the 5-slot chain
`[name/q gen/q List fn/q List]`); open-ended shapes parenthesize.
Default (gate off) behaviour is unchanged for all of these.

Known gap: dot-access dispatch (`Rand.int …`) routes around `stepWord`,
so module-export words are not strict-checked by the prototype.

## Measured fallout (lang/spec, TestSpecProd)

Baseline: green. Strict: **18 rows across 11 files** — the entire
dependent surface of the wait-through in the language's own spec corpus:

| Idiom | Rows |
|---|---|
| Unary `Any` collector fed by a following call (`size iota 3`, `typeof iota 3`, `size range 2 5`, `not iota 3`, `not not 5`, `size/typeof/not def x 5 x`) | 8 |
| Nullary fn word as an argument (`context eq context`, `context deq context`, `gensym eq gensym`, `typeof gensym`, `typeof tw2`) | 5 |
| `quote` feeding a waiting slot (`foo/q eq quote foo`, `None dot quote x` ×2) | 3 |
| def's bound value feeding a waiting bigger-arity fn (`g 1 def x 5 x` — forward-barrier.tsv §6, edge-forward-2) | 2 |

Every failure is the intended kind (stranded-forward signature error
with a parens hint); no silent meaning changes were observed.

## Assessment

For the strict rule:

- Uniformity: typed and `Any` slots behave identically; one teachable
  rule ("a function word never feeds forward collection; group with
  parens") and the existing good error message.
- Robustness: whether `f X …` nests no longer depends on the *waiting*
  word's overload set, so growing an overload can't silently re-bind
  distant call sites.
- The speculative-commit machinery's hazard class (guard pre-emption,
  phantom else — FORWARD-COLLECTION-PHASES.10.md) loses its remaining
  "keeps waiting" arm.

Against:

- It outlaws unary prefix chaining (`not not 5`, `size iota 3`) and
  nullary-fn arguments (`typeof gensym`) — the places where the
  wait-through is genuinely pleasant. Note the tension: the same
  mechanism that makes `print add 1 2` fragile is the one that gives
  AQL its partial Polish-notation feel on `Any` slots. A stricter
  language is a more parenthesised language.
- The checker must mirror the rule for words whose binding kind is
  unknown at check time (forward references) — stays gradual there.

## If adopted

1. ~~def story~~ Done: the keyword-slot `[name/q fn/q sigs]` overload
   (see the ruling above). Open follow-ups: keyword overloads for the
   other closed-set constructors (`gen` — 5-slot chain — `refine`,
   `class`, `word`, `enum`, `make`, `surface`, `fnsig`, `quote`);
   surface keyword slots in `aql describe def` (help.SigInfo carries
   no per-slot markers today); the AQL-authored surface — ParseFnParams
   already carries value patterns and `FnParam.Quote`, so `fn/q` as a
   bare patterned param in a sig literal is the natural spelling.
2. Extend the check to the dot-access dispatch path.
3. Update the 18 spec rows (parenthesise) and pin the new errors as
   negative rows; update forward-barrier.tsv §6's "keeps waiting" rows.
4. Checker mirror + compile-pipeline parity (CompileCheck).
5. Migration: the error message already teaches the fix; a `fmt`
   auto-fix (insert parens) is mechanical.
