# DIAGNOSTIC-VALUES.0 — show the values

Status: LANDED for the return-count diagnostic; the sweep across the
remaining value-subject diagnostics is open work (§5).
Owner surfaces: core/go (the shared builders + `diagValueList`),
eng/go (the VM's RET check), check/go (the static mirror).

Companion to [DIAGNOSTICS.0.md](DIAGNOSTICS.0.md), which built the
*structure* of a report — spans, notes, suggestions. This note is about
its *content*: what the words actually say.

## 1. The report that started it

```
>> def si1 fn [s:String i:Integer [convert Integer s]]
>> si1 '1'
  error: [boru/type_error]: si1: expected 1 return value(s), got 2
  --> 1:33
  1 | si1 '1'
                                      ^^^ si1: expected 1 return value(s), got 2
```

Everything structural is right. There is a code, a position, a caret, a
primary span. And the reader still cannot act on it, because the message
describes a disagreement about a *count* and never says what the values
were.

The second value here was `{i:Integer}` — the map a `name:Type` pair
lowers to, spliced onto the body by the return-by-value sugar (deleted;
see [FN-OUTPUT-SIG.0.md](FN-OUTPUT-SIG.0.md)). A reader who could SEE
`{i:Integer}` sitting in the returns would have recognised their own
output signature in the value and found the bug immediately. Instead the
message sent them looking at `convert`, which was innocent.

That is the general shape of the failure: the diagnostic knew the
answer and printed the arithmetic instead.

## 2. The rule

**A diagnostic names the values it is about.**

Not the count of them, not their types alone — the values, rendered.
Types are already shown where they are the subject (`expected String,
got Integer`); this rule is about the case where the message's subject
IS a value, or a run of them, and the message settles for describing it.

Corollaries:

- **The count and the values agree.** Where a message reports both, the
  rendered list has exactly the reported number of entries. Callers pass
  the same slice the count was taken over — never a wider one that
  happens to be in scope. Where the unnamed-arg allowance has been spent
  off the bottom of the frame, the reported values are the ones above it
  (§4).
- **Every engine says the same thing.** The text is built once, in
  `core/go/return_check_msg.go`, and the interpreter, the compiled VM
  and the static checker all call it — the byte-identity contract that
  file already carried, extended to cover the values.
- **Abbreviate, never truncate.** A bounded head with an explicit
  elision (`… (N more)`) is legible; a line cut off at a character
  budget is not, and a message that omits the values to stay short has
  failed at its job rather than succeeded at brevity.

## 3. Abbreviation

`diagValueList` (core/go/boru_error.go) renders a run of values and
abbreviates twice over, because a diagnostic can be swamped from two
directions independently:

- at most `diagMaxListHead` (8) values, tail elided as `… (N more)`;
- each value through the existing `diagValue`, which applies the same
  head-and-elide to a long *list* value.

So neither a hundred small returns nor one enormous one can push the
useful part off the line. The two limits share `diagMaxListHead`
deliberately: one number to tune, and the nesting reads consistently.

Empty is a real answer, not a missing one. A body that returned nothing
has no values to show, so `ReturnCountErrorText` emits the bare count —
`got 0` is already the whole story, and `— []` would be noise.

## 4. Applied

`f: expected 1 return value(s), got 2 — [1 {i:Integer}]`

Sites, all through the shared builder:

| Engine | Site | Values reported |
|---|---|---|
| Interpreter | `core/go/engine.go` (`returnCountError`) | `results` when the body under-returned; `results[UnnamedCount:]` when it over-returned |
| Compiled VM | `eng/go/vm.go` (`vmReturnCountErr`) | `stack[stackBase:]` / `stack[stackBase+NUnnamed:]` — the same two slices |
| Checker | `check/go/check_fnbody.go` | `stk[unnamedCount:]`, the analysed residual |

The two cases differ because the unnamed-arg allowance is only spent when
there is a surplus to spend it on. Under-returning reports everything the
body left; over-returning reports the values above the allowance, which
is exactly the count the message quotes.

The checker's values are carriers, not runtime values, so its text can
differ from the runtime's on a gradual body — it renders what it knows,
which is the point. Where the analysis is exact (the concrete-argument
call that gates the mirror in the first place) the two agree.

## 5. Open work

The rule is general; this change applied it to the diagnostic that
motivated it. Candidates for the sweep, roughly in order of how often
they are read:

- `no_signature` — reports the argument TYPES (`got (Integer)`), not the
  arguments. The values are in hand at the dispatch failure.
- The arity errors (`still waiting for N argument(s)`) — same.
- `describeStackTypes` (core/go/boru_error.go) is named for what it
  does: it renders the types around the pointer. It is the natural place
  to add values, and it feeds several messages at once.

Each is a separate change with its own spec rows, and each risks
widening a message that some test pins verbatim; doing them one at a
time is deliberate.
