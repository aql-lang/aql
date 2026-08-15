# CANON-ROUNDTRIP — canon always round-trips

> **Status:** Living reference. The reasoning, the measurements and the
> rejected alternatives behind **ADR-015** (recorded 2026-08-15 on
> maintainer instruction). The ADR is the rule; this is why it is that
> rule, what it currently costs, and what has to land before the gate
> can go green.

## 1. The contract

> `canon v` renders a value as boru source text which, re-parsed,
> yields a value `deq` to `v`.

Two things are deliberately strong, and both were chosen explicitly
over weaker readings that were on the table.

**It is a VALUE round-trip, not a textual fixpoint.** The weaker rule —
`canon(parse(canon(v)))` being byte-identical to `canon(v)` — is
attractive because it is checkable today and asks nothing about
equality. It was rejected as the *contract* because it can be satisfied
by a renderer that is consistently wrong: two distinct values that both
render as `word(foo)` reach a stable fixpoint while the distinction
they encode is gone. The fixpoint remains useful as a *diagnostic* (a
violation of it is always a violation of the contract), but the rule is
about preserving the value.

**There are no exempt kinds.** No closed escape list for handles,
functions, or host payloads — the option was offered and declined. The
argument for the strong form: canon is the serialisation boundary, so a
value that cannot be written is a value that cannot be moved between
processes, stored, or compared across the two ports. Carving out the
awkward kinds would put exactly the interesting values outside the
guarantee.

## 2. What this costs, measured

The contract is **not satisfied today**, and the gap is not a rounding
error. Measured 2026-08-15 against a build of this tree:

```
canon 1              -> 1                      ok
canon 'a'            -> 'a'                    ok
canon [1 2]          -> [1 2]                  ok
canon {a:1}          -> {a:1}                  ok
canon Integer        -> Integer                ok
canon foo/q          -> foo/q                  ok

def f fn [[x:Any] [Any] [x]]
canon (f/r)          -> fn f[[x:Any][Any][word(x)]]
                          ^^^^ debug spelling, and keyed on the
                               BINDING NAME rather than the function

canon (context)      -> Store(&{Ideal/Store map[__sys:Store(&{…
                          0xc0000163c0 …})] …})
                          ^^^^ a Go struct dump, including POINTER
                               ADDRESSES — non-deterministic between
                               runs, so it does not even round-trip to
                               itself
```

So the data half of the language already honours the rule; the failures
cluster in exactly the kinds §1 refused to exempt.

**NUR059** is the already-recorded instance of the first class (`foo/r`
→ `word(foo)` losing the modifier, `[:Box<Integer>]` →
`[:sugar(angle …)]`, the paren group). Its 2026-08-15 verdict — source-form
renderers on both engines — is the first tranche of ADR-015 work, and
this ADR is what makes it a rule rather than a preference.

## 3. The prerequisite: NUR031

A `deq` round-trip is unsatisfiable for a value that is not `deq` to
**itself**, and that is today's state for functions and host
`ExtensionPayload` values (NUR031). `f/r deq f/r` is false, so
`parse(canon(f/r)) deq f/r` cannot be true however good the renderer
is.

NUR031's 2026-08-14 verdict already directs the fix — route Ideal
`eq`/`deq` through the type's Behavior, and give functions a canon
**independent of the binding name**. That second half is literally this
ADR's requirement arriving from the other direction: a canon keyed on
the name a function happened to be reached through cannot round-trip,
because re-parsing it under a different binding yields a different
function.

So the sequencing is fixed, and the ADR states it: **NUR031 lands
before the gate can be green for the fn/host kinds.** Store already
satisfies the equality half (`context deq (context)` → true, from
NUR031's resolved handle tranche); what it lacks is a renderer.

## 4. Enforcement

A **property gate over the spec corpus, in both ports**. For every
value the corpus produces: render it, re-parse it, and require the
result `deq` the original. Chosen over pinning known kinds with rows —
rows are what the tree already had, and they are why NUR059's cases got
in: an unpinned kind regresses silently, because nothing asks the
general question.

Two properties of the gate that matter more than its existence:

- **It must run in Go and TS.** A one-port fix then fails the parity
  ledger loudly, which is the same forcing function `parser-parity`
  already applies — the ports cannot drift on a rule this central.
- **It cannot go green on day one**, per §2. It therefore lands with an
  explicit ledger of the kinds that still fail, each row naming the
  record that closes it (NUR059 for the sugar/modifier/paren kinds,
  NUR031 for fn and host payloads, a new record for Store's renderer if
  none covers it). The ledger only shrinks. This is the frontier-ledger
  pattern the compiler gates already use, and it is what keeps "the
  gate is red" from meaning "the gate is ignored".

## 5. What is NOT decided here

- **The concrete surface syntax** for the unspellable kinds. Requiring
  that a Store or a function round-trip does not by itself say what
  their source form looks like — a constructor call, a handle literal,
  something else. That design is the work NUR031's canon half and the
  Store renderer will have to settle, and it may well surface a
  language question (what does re-parsing a live handle even mean for a
  socket?) worth its own record.
- **Whether every value should exist.** The strong contract puts
  pressure the other way too: a value that genuinely cannot be given a
  meaningful source form is an argument that it should not be a
  first-class value. That is a bigger conversation than this ADR, and
  it is deliberately left open rather than pre-empted.

## Evidence

- `core/go/canon.go` — `canonString`'s doc comment already calls the
  round-trip "the canon contract"; this ADR promotes that from a local
  remark to a language rule.
- The §2 session, run against a build of this tree, 2026-08-15.
- `NUR.md` §NUR059 (the recorded instance, verdict: fix, both engines)
  and §NUR031 (the equality + name-independent-canon prerequisite).
