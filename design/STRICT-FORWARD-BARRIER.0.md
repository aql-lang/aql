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
sites — `stepWord` and `stepWordUsurp`). Two exemptions:

- **A waiting slot declared `Signature.RestArgs`** — statement-rest
  collection. The slot's value may legitimately be produced by the
  following function word(s); today `def`'s body slot (position 1 of
  all three def signatures, `native_definition.go`) declares it, so
  `def f fn [...]`, `def x add 1 2`, and `def x:T add 1 2` stay legal.
  This began as a hardcoded `FuncName == "def"` exemption and was
  promoted to the signature attribute after the fn/q investigation
  (below): the policy is now visible per-signature, per-position, and
  any future binder opts in by declaring it rather than by engine
  special-case. Without SOME def story, `def f fn [...]` — every fn
  definition in the language — is an error: `def` parks waiting for
  its value slot and `fn` is a function word.
- **Engine-internal boundary words** (`__`-prefixed frame-tail words):
  they are not source-level statement boundaries.

## Why not a literal-token overload (`def [Name/q fn/q SigList]`)?

The tempting alternative — give def a 3-arg overload whose middle slot
matches the literal word `fn`, so the fn-definition idiom is ordinary
structural dispatch — was investigated and rejected on three findings:

1. **The corpus is bigger than `fn`.** Bare-word def bodies ≈ 4,480
   occurrences repo-wide: fn 2,296, refine 485, class 463, `word`
   311, gen 261, Module.word 82, enum 42, make 40, surface 41, fnsig
   28, quote 24 … plus open-ended shapes (`def x add 1 2`,
   `def s range 2 5`, `def my-mod module […]`). Worse, the `def
   Name<T>` generics sugar desugars in the PARSER to `def Name gen
   [params]` (eng/go/parser/parse.go:448) — a chain of TWO dispatches
   (`gen`, then `fn` via the pending-gen protocol) that no 3-slot
   signature can match. Enumerating constructor overloads turns an
   open set into a closed one and still misses these.
2. **No slot kind can match a literal token today.** The natural
   composition — `QuoteArgs` + `Patterns{1: Atom("fn")}` — provably
   never matches: `patternsOk` (eng/go/match.go:39) resolves a
   def-bound forward word through `r.Defs.Top` BEFORE unifying, and
   `fn` IS Defs-bound (natives live in the DefTable), so the
   FnDefInfo value never unifies with `Atom("fn")`. An unpatterned
   `/q` slot at position 1 instead FALSE-matches every
   `def NAME <word> <list>` row (`def doub word [dup addq]`,
   `def Color enum [red green blue]`, …) — a silent-meaning-change
   class. A literal-word slot would be a NEW kernel slot kind
   threaded through ~10 matcher/planner/emit seams.
3. **`QuoteArgs[1]` leaks plan-time.** `capturesForward`
   (engine.go:1364) ORs capture flags across ALL of a word's sigs —
   per-word, not per-matched-sig — so one new capturing sig at
   position 1 would disable the function-word barrier at position 1
   for EVERY def statement, re-opening the documented cross-barrier
   pre-evaluation bug class.

`RestArgs` avoids all three: it changes nothing at plan time (it is
not a capture flag), it keeps the whole existing corpus legal (the
body slot stays `Any` — any constructor, any chain, any arity), and
it is consulted at exactly one point (the strict-rule stranding check
at the runtime boundary).

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

1. ~~Promote the `def` exemption to a declared signature attribute.~~
   Done: `Signature.RestArgs` (see above). Open follow-ups: surface
   the attribute in `aql describe def` (help.SigInfo carries no
   per-slot markers today), decide whether `var` declares it too, and
   pick a signature-literal notation for AQL-authored binders.
2. Extend the check to the dot-access dispatch path.
3. Update the 18 spec rows (parenthesise) and pin the new errors as
   negative rows; update forward-barrier.tsv §6's "keeps waiting" rows.
4. Checker mirror + compile-pipeline parity (CompileCheck).
5. Migration: the error message already teaches the fix; a `fmt`
   auto-fix (insert parens) is mechanical.
