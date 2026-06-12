# Tail-Call Optimisation for the AQL Tape Machine

**Status:** Discovery note — design only, nothing implemented. Follows
`RECURSION-PERFORMANCE.10.md` (which made recursion linear-time via the
gap-buffer tape) and `TAPE-DATA-STRUCTURE.10.md`. TCO is the remaining
step that would make tail recursion **constant-space**, equivalent to
iteration.

## Why tail recursion still grows the tape

A call splices the fn body inline into the tape, bracketed by the
per-call machinery: captures/params installed as defs, then the body
tokens, then the synthesized cleanup tail — the `DefCleanup` marker
(`__DC`, undoes body-local defs and params), the `__pa` token (pops the
per-call Args list and FnBaseline), and a `ReturnCheck` (`__RC`) when a
return type is declared. Three registry stacks (Args, def-snapshot,
FnBaselines) are pushed per call and popped by that tail
(eng/go/CLAUDE.md "Per-Call Stacks").

When the *last* action of a body is another call, the inner body is
spliced **inside** the outer frame, in front of the outer frame's
cleanup tail. Nothing of the outer frame is ever needed again — but its
`__DC __pa __RC` markers stay parked on the tape until the innermost
call returns and the whole chain unwinds. At depth *d* that is O(*d*)
parked marker groups and O(*d*) entries on each per-call stack. For a
*non-terminating* tail loop the tape grows forever (now caught by the
tape-exhaustion guard, but as a failure, not as the loop the programmer
wrote).

## The key insight: tail position is visible ON the tape

In a compiler, TCO needs static analysis of "tail position". In this
tape machine it needs none: **at dispatch time, a call is a tail call
iff everything between it and the end of the enclosing frame is exactly
the frame's own cleanup tail** (`__DC`, `__pa`, optionally `__RC`, and
the frame's closing paren). That is a cheap structural probe of a few
tape entries at a known offset — the tape *is* the continuation, so
"nothing left to do afterwards" is literally readable.

Two properties of the engine make the transformation safe at that
moment:

1. **Arguments are already values.** Dispatch happens after forward
   collection / stack matching, so the callee's args are fully evaluated
   concrete Values before we decide anything. Tearing down the caller's
   bindings cannot affect them.
2. **Captures are snapshots.** A closure's `FnDefInfo.Captured` values
   were copied at construction (shallow Value copies); they do not read
   the caller's def stack at call time. Undefining the caller's locals
   before running the callee is therefore invisible to the callee.

## The mechanism (frame replacement)

In `execFnDefSig` / `execFnDefLiteral`, after the sig is matched and
args are in hand, add one probe:

```
if isTailPosition(valIdx):           # only cleanup markers remain ahead
    runFrameCleanupNow()             # __DC semantics: undef params/locals;
                                     # __pa semantics: pop Args + FnBaseline
    splice the callee body OVER the remaining frame region
    push the callee's Args/snapshot/baseline (replacing, not nesting)
else:
    nest as today
```

The cleanup logic already exists (`stepDefCleanup`, the `__pa` handler);
TCO calls it eagerly instead of leaving the markers parked. Tape length
and all three per-call stacks then stay **O(1)** across any chain of
tail calls — self-recursion or mutual recursion alike, since the probe
never asks who the callee is.

### Return-type checking

Each frame's `__RC` enforces its declared return. Replacing frames
collapses the chain of pending checks, so the simple sound rule is:

> TCO fires only when the callee's declared return type conforms to the
> caller's (callee ⊑ caller). The caller's `__RC` is kept; the callee's
> would be redundant.

Self-recursion — the dominant case — satisfies this trivially
(identical types). Calls that fail the rule just nest as today;
correctness never depends on TCO firing.

### What is NOT affected

- `args` stays correct: the per-call Args entry is replaced, so `args.N`
  sees the callee's own args, as ever.
- Quoted/NoEval bodies: body tokens are inert data until spliced.
- `CallAQL` (module fns running in a captured sub-registry) is a
  separate, Go-recursive path; its TCO analogue is the standard
  trampoline — detect the tail self-call and loop within the same Run
  invocation instead of recursing. Worth doing second; the splice path
  covers ordinary `def f fn […]` recursion.
- Check mode: the analyser already converges recursion via memoisation;
  TCO is runtime-only (and could be disabled under check for diagnostic
  fidelity).

## What it buys

- `s2 1_000_000` runs in O(1) tape space at loop speed — tail recursion
  becomes a real iteration construct, not a bounded-depth convenience.
- The runaway taxonomy becomes clean: a **non-tail** runaway grows the
  tape and trips `tape_exhausted`; a **tail** runaway is pure CPU and
  trips `evaluation_limit`. Today an infinite tail loop is misclassified
  as a memory problem.
- The default tape ceiling could be tightened (less headroom needed for
  legitimate deep recursion), making the memory guard sharper.

## Alternatives considered

- **Explicit `recur` word** (Clojure-style): `recur a b` rebinds the
  current frame's params and jumps to the body start. No detection
  logic, trivially correct, a contained implementation — but it only
  covers self-recursion, adds a word users must know to use, and leaves
  the natural spelling (`f` calling itself in tail position) growing the
  tape. Reasonable as a stepping stone; not a substitute.
- **Lazy marker squashing**: when the pointer reaches a run of adjacent
  `__DC __pa` groups, execute and remove them all in one pass. This
  bounds *marker* accumulation but not the per-call stacks, and the
  squash only happens on the unwind — an infinite tail loop never
  unwinds, so it does not fix the runaway case. Rejected.

## Sketch of the work

1. `isTailPosition(valIdx)` — structural probe in engine.go: scan from
   the dispatch site to the frame end accepting only `__DC` / `__pa` /
   `__RC` / closing-paren tokens.
2. Factor the eager teardown out of `stepDefCleanup` + the `__pa`
   handler so dispatch can invoke it directly.
3. The frame-replacement splice in `execFnDefSig` (and the trivial-
   delegation path in `execFnDefLiteral` if it can carry AQL bodies).
4. Return-conformance gate (callee ⊑ caller) using the existing
   `v.Is`/ConformsTo machinery.
5. Spec rows: deep tail recursion at depths far past the tape ceiling
   (e.g. `s2 1_000_000`), mutual tail recursion, NON-tail recursion
   unchanged (`n add (s …)` still nests), `args` visibility, capture
   shadowing across a TCO'd call, and a return-type-mismatch case that
   must nest rather than mis-fire.
6. The `CallAQL` trampoline as a follow-up.
