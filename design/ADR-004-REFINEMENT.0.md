# ADR-004 refinement — argument-handling categories

**Status: DRAFT / discovery.** This note is the material for a refined
ADR-004, written under NUR023's 2026-08-14 verdict. It is **not** an ADR
entry and does not become one implicitly: promotion into `ADR.md` is a
separate, explicit maintainer decision (`ADR.md`'s own header rule, and
`lang/go/CLAUDE.md` §"ADRs — only on explicit instruction"). Until then
this is discovery, however settled it reads.

**Why it exists.** ADR-004 as accepted (2026-06-09) is six sentences:
every word ships forward-collecting; the sole exception is the Forth
stack vocabulary. NUR023 found two argument-taking words outside that
"sole exception" (`apply`'s `[Function]` overload, `__casematch`) and a
0-arg guidance split between two documents. Both were closable as
documentation debt and have been closed — the two words are pinned in
REFERENCE.md's closed list with their rationales, and the 0-arg passages
are reconciled. What could not be closed that way is the underlying
gap: **ADR-004 describes a default but not a system.** It names one
category ("forward") and one exception ("the stack vocabulary"), while
the implementation has four categories, a sentinel, a per-call-site
override pair, and an orthogonal quoting axis. A rule that cannot
express `apply` is a rule that will keep generating NUR records.

The canonical, code-adjacent statements of the mechanism remain
`eng/go/CLAUDE.md` §"Signature Ordering" and `lang/go/CLAUDE.md`
§"Argument Ordering"; this note does not replace them. It supplies what
an ADR needs and they do not: the **categories**, the **rationale**, and
the **rule for choosing**.

---

## 1. Barrier positions

Every `Signature` carries one integer, `BarrierPos`, which is the
position of the `|` marker in the signature. It defines a single split:

> Positions `[0 .. BarrierPos-1]` are **forward-eligible** — filled from
> forward tokens in source order, falling back to the stack. Positions
> `[BarrierPos .. N-1]` are **stack-only**. Stack consumption is always
> top-down: `sig[i]` reads the top, `sig[i+1]` the next-deeper.

| Value | Meaning |
|---|---|
| `BarrierAllForward` (`-1`) | **Unset sentinel.** "No `|` was specified." Resolved once at registration. |
| `0` | All positions stack-only. `[\| a b c]`. |
| `N` (= `TotalArgs()`) | All positions forward-eligible. `[a b c \|]`. |
| `0 < B < N` | Mixed: forward fills the leading `B`, the stack fills the rest. |

**The sentinel resolves exactly once, at a single boundary.** In
`Registry.upsertFnDef` (`core/go/registry.go`), `-1` becomes
`TotalArgs()`. Every read of `BarrierPos` downstream sees an explicit
value, which is what keeps the "no zero-value overload" rule
(`eng/go/CLAUDE.md`) intact: `0` unambiguously means stack-only, because
"unspecified" was spelled `-1` and is already gone.

There is **no per-word "stack default" mode.** A stack-only signature
must set `BarrierPos: 0` explicitly at its registration site. The
`ForwardArgs` flag and the `RegisterStackOnly` method that once provided
one were retired in the BarrierPos cleanup. (NUR023's own text describes
the sentinel as resolving "to `len(Args)` or `0` based on the
`forwardArgs` flag" — that is **stale**; the flag no longer exists and
`-1` always resolves to `TotalArgs()`. Corrected here.)

### The 0-arg case

At zero arguments `TotalArgs()` is `0`, so `-1` and `0` normalize to the
**byte-identical** stored signature. Both spellings are therefore
correct and neither is a default the other violates. `design/go-modules/
README.10.md` and `RUNTIME.10.md` now say so; a refined ADR should say
it once rather than leave two documents to be compared. The choice is
idiom: `0` reads as "there is nothing to collect", `-1` as "this word is
ordinary". Prefer `0` for constants and clock words, matching the
existing majority.

---

## 2. The categories

A word occupies exactly one of four categories, and the category is a
property of each **signature**, not of the word — which is why a word
can be mixed overall.

### 2.1 Forward-eligible (the default)

`BarrierPos == TotalArgs()`. The canonical call form is
`word arg1 arg2 …`, and written order matches declared order. New words
ship here. This is ADR-004's cultural default and it is unchanged by
this refinement.

### 2.2 Mixed-barrier

`0 < BarrierPos < TotalArgs()`. Forward fills the leading positions and
the stack supplies the rest. This category is not an exception to the
default — it is the default *parameterised*, and it is common: 20 of the
249 describable core words have signatures that disagree about sourcing
(`or`, `otherwise`, `get`, `getr`, `dot`, `dotr`, `has`, `apply`,
`guard`, `error`, `exposes`, `of`, `extends`, `default`, `tor`, `tand`,
`teq`, `is`, `as`, `tis`).

That count is the strongest argument that ADR-004 is incomplete rather
than merely under-exemplified: a rule with one exception does not
describe a system where 8% of the core vocabulary is neither purely
forward nor purely stack.

### 2.3 Stack-only

`BarrierPos == 0`. Every argument comes from the stack. **This is a
closed list**, pinned in REFERENCE.md §"Stack manipulation", and adding
to it "needs the same justification weight as a new init-time panic"
(ADR-004, as amplified by NUR023). The list has two tiers:

- **The Forth vocabulary** — `dup`, `drop`, `swap`, `over`, `rot`,
  `nip`, `tuck`, `dup2`, `drop2`, `swap2`, `over2`, `pick`, `roll`,
  `depth`, `stack`. Their entire meaning *is* the stack; there is no
  argument to name because the operand is the shape of the stack itself.
- **Semantic necessity** — two non-shuffle words, admitted under
  ADR-004's own exception clause ("whose entire meaning *is* the
  stack"), each with a stated reason:
  - `apply` (the `[Function]` overload): `args… fn apply` means "take
    the function off the stack and apply it to the preceding values."
    Forward collection would force callers to write the function's
    arguments *after* the function, fighting the left-to-right flow the
    word exists to serve. Consequence, documented: `apply f/r 5` raises;
    the spelling is `5 f/r apply`.
  - `__casematch`: the `case` desugar's internal match probe (each
    clause lowers to `if (v match __casematch) …`), always fed by the
    synthesized chain's stack discipline. The `__` prefix marks it
    internal by convention — **not** by enforcement, which is why it is
    user-reachable and therefore had to be pinned rather than waved off.

**The admission test a refined ADR should state:** a stack-only
registration is justified only when the word's *meaning* is the stack
arrangement — not when stack form merely reads better, and not when it
is simply how the word is currently called. "It is only called
internally" is explicitly not sufficient; `__casematch` is describable
and reachable, and convention is not enforcement.

### 2.4 Quoting slots (the orthogonal axis)

`QuoteArgs` and `NoEvalArgs` are **not** categories of argument
sourcing; they cross-cut all three above and are frequently confused
with them, which is reason enough for the ADR to name the distinction.

- **`QuoteArgs[i]`** — position `i` captures an upcoming Word as an
  `Atom` during forward collection, without evaluating it (`def name …`,
  `get key map`, `set key val store`). It affects collection.
  boru-defined fns declare the same capability as `name:Atom/q`.
- **`NoEvalArgs[i]`** — position `i` suppresses list auto-evaluation in
  `execMatch` (`fn` bodies, `if`/`for` branches, `do`/`call` bodies). It
  does **not** affect collection or Word→Atom conversion.

A refined ADR should state the invariant: quoting decides *what a
collected token becomes*; `BarrierPos` decides *where the argument comes
from*. They compose freely.

---

## 3. The chaining rationale

ADR-004 asserts the forward default as a cultural preference. The
deeper reason — the one that makes it a *design* decision rather than a
style choice — is composition, and it is worth stating because it also
explains why the per-call-site levers are the right escape hatch and
per-word flips are not.

**The mirror equivalence.** Under the single split rule,
`f a b ≡ b f a ≡ b a f`. All three are the same call: collection moves
forward until the barrier, then backward from the stack prefix. This is
what lets a value flow into a word from either direction without the
word knowing which happened — the property every pipeline relies on.

**The swap form is the lone non-equivalence.** `a f b` is `k=1`: `b`
fills `sig[0]`, `a` fills `sig[1]`. It is a *different split of the same
operands*, not a different rule, which is why `10 sub 3 = 7` while
`sub 10 3 = -7`. Non-commutative operators read naturally in swap form,
and the handler convention (`args[1] OP args[0]`) exists to make that
reading the natural one.

**Why the levers are per-call-site.** `/s` forces stack (limit 0) and
`/f` forces forward (limit N) at a single call, and grouping `( … )`
contains a call. Because the levers are per-site, a word's declared
category stays a stable fact a reader can rely on. A per-word flip would
make the same spelling mean different things in different programs,
which is precisely the non-uniformity this register exists to prevent —
so "per-word flips are rejected" is not conservatism, it is the same
argument that motivates the closed list.

---

## 4. Diagnostics should explain the category

NUR023's verdict asks that diagnostics *explain why a word occupies its
category* rather than merely reporting a failed dispatch. Half of this
has landed and is worth recording as the precedent:

`boru describe` previously branched on a single binary `ForwardArgs`
flag, so **every mixed-barrier word printed the full forward
equivalence chain — including a spelling it refuses.** `apply` advertised
`apply x y <=> y apply x <=> y x apply` while `apply f/r 5` raised;
`or` advertised a chain while `or false true` raised `insufficient_args`.
The fix (`precedenceShape` / `writePrecedenceMixed` in
`lang/go/native/help/help.go`) classifies a word by whether its
signatures *agree* about sourcing, and renders the disagreeing case as a
per-group split plus the one spelling that satisfies every row — full
stack form, which always dispatches because a forward-eligible position
accepts a stack value too.

What remains, and what a refined ADR would license: a **dispatch-failure
diagnostic that names the category**. Today `apply f/r 5` raises a bare
`signature_error`; it could say that `apply`'s `[Function]` overload is
stack-only and suggest `5 f/r apply`. The information exists in the
signature — the diagnostic simply does not consult it. This is the same
shape as NUR066's proposed `dot` diagnostic, and the two should probably
share a mechanism.

---

## 5. What promotion would change in ADR-004

If this material is promoted, the ADR's substance changes in three
ways and its default does not change at all:

1. **"The sole exception is the Forth stack vocabulary" becomes false
   and is replaced** by the four categories, with stack-only defined as
   a closed list of two tiers plus the admission test in §2.3.
2. **Mixed-barrier is named as a first-class category**, not an
   unmentioned middle ground — with the 20-word measurement as the
   evidence that it is ordinary rather than exceptional.
3. **The rationale is stated** (§3), so the forward default reads as a
   composition argument rather than a preference, and the rejection of
   per-word flips follows from it instead of standing alone.

Unchanged: new words ship forward-eligible; the levers are the
call-site modifiers and grouping; a stack-only registration carries
init-time-panic justification weight.

**Open for the maintainer**, and deliberately not decided here:

- Whether `apply`'s `[Function]` overload should be *kept* (pinned, as
  now) or *removed* in favour of requiring `/s` at the call site.
  NUR023's original verdict listed both. Keeping it is the status quo
  and is now documented; removing it is a breaking change to a
  documented spelling.
- Whether the 0-arg idiom should be *stated as a preference* (`0` for
  constants) or left explicitly free. This note recommends the former
  only weakly — the byte-identical normalization means nothing is at
  stake beyond readability.
- Whether the category-naming diagnostic (§4) belongs in the ADR at all,
  or is an implementation follow-up the ADR merely permits.

---

## Evidence

- `core/go/registry.go` — `upsertFnDef`, the single sentinel-resolution
  boundary; the doc comment recording that `ForwardArgs` /
  `RegisterStackOnly` were retired.
- `core/go/match.go` / `eng/go/CLAUDE.md` §"Signature Ordering" — the
  single split rule and the top-first convention.
- `lang/go/CLAUDE.md` §"Argument Ordering" — the call-form worked
  examples and the `/s` `/f` levers.
- `REFERENCE.md` §"Stack manipulation" — the pinned closed list, both
  tiers, with `apply` and `__casematch`'s rationales.
- `lang/go/native/help/help.go` — `precedenceShape`,
  `writePrecedenceMixed`; `lang/go/native/help/precedence_test.go` pins
  all three shapes in both directions.
- `design/FORWARD-COLLECTION-PHASES.10.md` — the two-phase collection
  model (plan-time token walk, run-time arrival loop) that a refined ADR
  should point at rather than restate.
- `NUR.md` §NUR023 — the record this note discharges the drafting half
  of.
