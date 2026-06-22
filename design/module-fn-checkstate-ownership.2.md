# §6 follow-on — compiling the test framework: the run-spec blocker, root-caused

Status: investigation, on top of the landed §5 refactor
(`module-fn-checkstate-ownership.1.md` §5a/§5b/§5c, commits `865a84d`,
`2816e8f`). Records what §5 unblocked and the PRECISE root cause that still
blocks `module-test.tsv:38` (`run-spec`) — found by hands-on tracing, and it is
NOT what the first-pass guess (element typing) assumed.

## What §5 already unblocked (measured)

`test-describe`'s closure path (`tryRecordClosure` -> the `CallableSpec`
`BodyOut:0` body unit) now compiles a broad range of bodies that previously
refused — for-loop tails in a `BodyOut:0` closure, value-defs of computed
values, computed-count `for (n) [...]`, AND the dynamic subject dispatch
(`Test.invoke`). So the closure infra, value-def-locals, computed loops and
dynamic dispatch are NOT blockers. `run-spec` falls back faithfully; its refusal
reason moved from the §5b leak artifact to the genuine "code-body word
test-describe".

## The root cause: `quote` of a computed operand matches the WRONG sig

`run-spec`'s body does `def cases quote (s get "cases")` (and `def subs quote
(...)`, `def in quote (...)`). `quote` has two signatures:

- sig 1 `[TAtom]` with `QuoteArgs[0]` — the `quote foo` word->Atom capture;
- sig 2 `[TAny]` with `NoEvalArgs[0]`, `RunInCheckMode`, `ReturnsFn:
  ReturnsIdentity(0)` — `quote (value)`, which PRESERVES the operand's type.

`(s get "cases")` is a `get` on a generic `Map` param, so it returns an **`Any`
carrier**. An `Any` carrier optimistically conforms to `TAtom` (the
dynamic-modality match-everything rule), so **sig 1 is tried first and matches**
— its `QuoteArgs` operand is not an inert const (it's computed), so
`hasUncoveredQuoteArg && !quoteInertOK` fires:
`MarkUncompilable("quoted-operand word quote")`. Sig 2 — the correct
value-identity sig that would have compiled and kept the type — is never reached.

Verified (Q_DEBUG at the refusal): `QuoteArgs=map[0:true]` sig matched an
`Any[conc=false]` operand. And the discriminating control: WITHOUT the `quote`,
the identical Any-carrier `get` chain (`def cs (s get "cases") def c (cs 0 get)
c get "k"`) COMPILES — the `get`s match `List`/`Map` optimistically. So:

> The lever is `quote`'s sig-matching on an `Any` carrier — NOT element typing.
> The `.1`/earlier `.2` "needs typed list elements" guess is wrong: element
> field-access already compiles; `quote` is the only word whose first (QuoteArgs)
> sig optimistically claims an Any operand and then refuses it.

## Why the naive recovery fix regresses (verified, reverted)

Extending the non-disjunct dispatch recovery to `tryRecordPoly` for non-concrete
args (so a `get`-on-Any records `OpCallNativePoly`) pushes `refusalCeiling` 10 ->
23 (0 divergences): the poly result is a dynamic `Any` carrier that POISONS
downstream typed consumers corpus-wide. Do not broaden the recovery; the fix
belongs at `quote`.

## The fix (focused) + remaining cascade

1. **`quote`'s word-capture sig must not optimistically match a non-concrete
   `Any`/dynamic carrier** — only a genuine `Atom`/word. Then a computed-value
   operand falls to sig 2 (`ReturnsIdentity`, `RunInCheckMode`), which preserves
   the type and compiles. This is a `matchSignature`/`QuoteArgs` precision change:
   a `QuoteArgs` position is a LITERAL-word capture, so it should require the
   operand to actually be a word/Atom, not ride the Any-conforms-to-everything
   rule. Guard it tightly — it must not break `quote foo`, the `/q` keys of
   `get`/`set`/`def`, or other `QuoteArgs` words; pair positive (`quote (expr)`
   compiles, type preserved) with negative (`quote foo` still captures the Atom).
2. (then) `test-record`'s 7-arg call with `[]` + `(c get "name")` operands, and
   the recursive `run-spec` over `subs` (`for (subs size) [..run-spec..]`) —
   re-attempt the reverted `evCallUser` value-def-local promotion (`.0` §7) here.
3. Land on its own branch; gate: `module-test.tsv:38` ->
   `{total:2 passed:2 failed:0}` compiled == interpreter, 0 divergences, and
   `TestModuleFnCheckPathGate` staying green. Watch `refusalCeiling` /
   any-frontier for unintended shifts (the §1 quote-match change is corpus-wide).
