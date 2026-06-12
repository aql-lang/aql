# TCO, staged — a safety-first implementation path

**Status:** Discovery note / proposal. Companion to `TCO.10.md` (the
deferred frame-replacement design): this note re-reviews that design
against the traced code, corrects three points in it, and breaks the
work into independently shippable stages, each gated so that a slip in
the dispatch core is caught by machinery rather than by luck. Nothing
here changes behaviour until Stage 3, and every behavioural stage lands
behind a kill switch with a dual-mode differential gate.

Code referenced below: `eng/go/core_helpers.go` (`buildFnBodyHandler`,
`InstallFnDef`), `eng/go/engine.go` (`execMatch`, `spliceMatchResults`,
`execFnDefSig`, `stepDefCleanup`, the paren-collapse return-check
sweep), `lang/go/native/native_definition.go` (`popArgsHandler`).

## 1. Review of TCO.10.md — what holds, what needs amending

The traced "implementation reality" in TCO.10.md is accurate: a `def f
fn […]` call dispatches through `stepWord → execMatch →
buildFnBodyHandler closure → spliceMatchResults`; the handler pushes
all three per-call stacks (FnBaseline, Args, param/capture defs)
*before* `execMatch` splices the returned `( body… __DC __pa undef-tail
[__RC] )` tokens; no frame-extent tracking exists; `execMatch` is the
hottest path in the kernel. The two safety properties (arguments are
already values; captures are snapshots) also hold as stated.

Four amendments, found by re-tracing:

**(a) The hook point can sit in `execMatch`, before the handler runs —
`buildFnBodyHandler` does not need restructuring.** TCO.10.md's
consequence 1 ("state-push ordering is backwards for replacement")
assumed the teardown/install interleaving had to happen inside the
handler. It doesn't: if the caller frame's teardown runs *before*
`match.Sig.Handler` is invoked, the handler then pushes the new frame's
state onto already-popped stacks — replacement falls out of ordering,
with the handler byte-for-byte unchanged. Teardown-before-handler is
also the only sound order: both teardown flavours (undef-by-name and
snapshot truncation) would pop the *callee's* freshly installed `p` if
they ran after the handler installed it. What this needs is for
`execMatch` to recognise fn-frame dispatch before calling the handler —
a marker on the registered signature (set in `InstallFnDef`), one nil
check on the non-fn fast path.

One failure-ordering note: after eager teardown, the handler's only
error return is `Args.Push` — and since teardown just popped one Args
entry, the push cannot newly exceed a depth limit. So "teardown
succeeded, handler failed, tape half-edited" is structurally
unreachable on this path. Re-verify if the handler ever grows another
error return.

**(b) "Keep the caller's `__RC`" contradicts O(1) as written.** The
caller's ReturnCheck lives inside the caller's paren pair; keeping it
means keeping a `( __RC )` shell per tail call — 3 tokens and one paren
depth level per iteration, i.e. O(d), just with a smaller constant.
There are two coherent variants, and the plan below uses both, in
order:

| | shell variant | full replacement |
|---|---|---|
| splice | contiguous, over the call region only (as today) + delete the marker run | whole frame region `(ₒ … )ₒ` replaced by callee's `( … )` |
| ReturnCheck | caller's `__RC` stays in place, callee's arrives as normal | callee's kept, caller's discarded (gate: caller has none, or callee ⊑ caller) |
| leftover values low in the frame | unchanged semantics — they stay, the kept `__RC`/collapse handles them exactly as today | must cause rejection (they'd be dropped) — correct anyway, since a frame that accretes values per call is not constant-space-able |
| paren-balance risk | none (no paren is ever spliced without its partner) | the splice spans paren pairs; extent must be exact |
| result | per-call stacks O(1); tape residue 3 tokens/call (~5–10× depth headroom) | true O(1) — tail recursion ≡ iteration |

The shell variant is observationally identical to today's semantics by
construction (the RC and the leftover-value rule are literally the same
tokens executing at the same collapse), which makes it the right first
behavioural stage; full replacement is the destination. Keeping the
*callee's* RC in full replacement is also the more internally
consistent choice: the `UnnamedCount` slack the RC allows then matches
the frame contents it actually checks, and a return-type error names
the fn that produced the value.

**(c) The forward scan alone is insufficient — a frame-interior check
below the call is load-bearing, not optional.** TCO.10.md's
`isTailPosition` sketch scans forward from the call accepting
close-parens and tail markers. That has false positives. `n add (f (n
sub 1))` parks an `add` Forward *below* the group; the forward scan
from `f` sees `) __DC __pa …` — a perfect tail pattern — yet the
frame's result feeds `add` before the RC, so it is not a tail call.
Benign for `add` (it consumes resolved values), fatal in general: `def
x (f …)` parks a def-installing Forward, and eager `__DC` before it
resolves would let `x` leak past the frame (the exact leak `__DC` was
added to fix). The probe therefore has two halves:

- **forward** from the dispatch site: accept only `)`* then the frame
  tail (`__DC`? `__pa` (`undef` name)* `__RC`?) then the frame's `)`;
- **backward** from the call region's lowest index to the frame's open
  paren: accept only concrete values (shell variant) or nothing but
  structural open-parens (full variant); reject any Forward, Mark,
  Move, or pending word.

The backward half needs a frame boundary to stop at, which brings in:

**(d) Prefer a self-describing frame-open token over a parallel
frame-position stack.** TCO.10.md sketches a per-frame side stack of
open-paren indices "kept in lockstep with the tape's `__pa`/`)`
tokens" — a new cross-structure invariant in the hottest path, exactly
the per-call-stack misalignment class the kernel guide warns about. The
alternative: have the two frame synthesizers open the frame with a
*marked* OpenParen (same type, so every `IsOpenParen` site behaves
identically; payload carries fn identity for the self-recursion gate).
Then both probe halves are self-anchoring on the tape — the tape stays
the single source of truth, and there is no side structure to drift.
Without the mark, the backward scan cannot tell the frame's `(` from an
`if`-branch group's `(` and would have to over-scan to tape start,
rejecting valid tail calls under any pending outer forward (`add (g
5)` where `g` tail-calls — common and must work).

## 2. The reframing that makes the change reviewable

Eager teardown is not new semantics. The markers being executed early
(`__DC` truncation, `__pa` pop, the undef tail) are *already on the
tape, already scheduled*; TCO runs them now instead of after the callee
returns, then reuses their tape slots. The transformation is sound
exactly when the callee provably cannot observe the difference, which
is the conjunction of four conditions — two from TCO.10.md, two new:

1. arguments are already concrete values at dispatch (post
   auto-eval) — teardown cannot affect them;
2. captures are construction-time snapshots — they never read the
   caller's defs at call time;
3. **nothing pending sits below the call inside the frame** (no
   Forward/Mark/Move — §1c) — otherwise caller code still runs after
   the callee and may observe missing defs;
4. **arg auto-evaluation installed no defs** — `f {x:(def y 3 …)}`
   evaluates its map values in `execMatch` before dispatch; today such
   a `y` is (dynamically) visible to the callee until the caller's
   `__DC` runs, so tearing down first would change visibility. Gate
   cheaply: a monotone install counter on the registry, compared
   across the auto-eval block.

Conditions 3 and 4 are checks, not hopes — when they fail, the call
nests exactly as today. Correctness never depends on TCO firing
(TCO.10.md's own framing, preserved).

## 3. The stages

Each stage is its own PR, lands green on the full suite, and is
useful even if the next stage never happens.

### Stage 0 — pin current behaviour (no code change)

- Add the recursion spec-row corpus *first*, asserting today's
  behaviour at safe depths: self-recursion, mutual recursion, non-tail
  recursion, `args.N` visibility, capture shadowing across calls,
  return-type mismatch, `break`/`continue` from inside fn bodies,
  multi-value-accreting bodies (`[n mul 2 f (n sub 1)]`), void
  callees, the `n add (f …)` and `def x (f …)` forward shapes. Every
  later stage diffs against these pinned rows. (No deep-recursion rows
  exist today — `s2 1_000_000` in TCO.10.md is aspirational.)
- A Go test pinning tokens-parked-per-frame and the depth at which the
  default tape ceiling trips, so Stages 3/4 assert their resource
  claims numerically rather than anecdotally.
- Decide the kill-switch surface now (registry option / env var), so
  every behavioural stage is born switchable.

### Stage 1 — one frame-tail synthesizer, one teardown helper (refactor)

Today the tail is synthesized in two places with *different shapes*:
`buildFnBodyHandler` emits `__DC __pa undef… [__RC]`, while
`execFnDefSig`'s no-captured-registry branch emits `__pa undef…
[__RC]` — **no `__DC`**, so body-local defs in a Function-value
dispatch appear to leak past the frame (the leak `__DC` was added to
fix for named fns, DX-REPORT Issue 2). Whether bug or latent accident,
TCO cannot sit on top of two divergent tail shapes. This stage:

- extracts a single tail synthesizer used by both sites, and a single
  Go-side `teardownFrame` helper expressing what the markers do
  (`stepDefCleanup` truncation, `popArgsHandler`'s Args+baseline pop,
  `UninstallDef` per name) so the markers and, later, the TCO path
  execute the *same code*;
- adds the marked frame-open paren (§1d) — semantically inert, every
  paren predicate unchanged;
- optionally consolidates the marker run into one `__ft` token
  carrying {snapshot, names}. This makes the probe and the eager
  teardown trivial and is exercised by *every* fn call from day one —
  but it changes the token stream, so the sweep sites that today
  handle the markers piecemeal (the paren-collapse `__DC` sweep, the
  flow-control skip list, the drain filters) must each be audited.
  Decide A (consolidate) vs B (keep shapes, share helpers only) on
  that audit; A front-loads mechanical, suite-gated risk, B leaves a
  multi-token scanner to maintain forever. Either way the undef-tail
  semantics audit happens here: what `undef <name>` does for every
  legal param/capture name shape (notably capitalised captured type
  defs, which Retire minted types — the gate in Stage 3 simply
  refuses frames whose teardown names are capitalised until rows
  prove that path).

Risk: moderate, mechanical, fully suite-gated; precedent is the tape
swap ("mechanical but large; own PR; gate on full suite").

### Stage 2 — the probe, detection-only (dry run)

Implement the two-halved probe (§1c) and wire it to *nothing but
telemetry*: a counter/trace note (and recorder hook) behind a debug
flag. Zero behaviour change — the probe is read-only. This stage owns
the probe's correctness budget:

- unit tests on synthetic tapes for every accept/reject token class;
- the Stage-0 fixture programs asserted to fire / not fire exactly
  where expected;
- a property test fuzzing tape shapes around the pattern (the
  `TestTapeDifferential` precedent);
- the whole spec suite run with detection on, fire statistics
  eyeballed for surprises.

This buys what the `recur` stepping-stone was supposed to buy —
confidence in tail detection — without shipping a permanent word
(§5). Gate ordering keeps the hot path flat: non-fn dispatch pays one
nil check; fn dispatch in non-tail position pays a few token reads.
Benchmark with the existing dispatch/recursion benchmarks; budget the
regression (<1% on non-fn dispatch) before Stage 3.

### Stage 3 — shell-variant TCO for direct self-recursion

First behavioural change, deliberately the narrowest useful slice:

- **Gate:** probe passes (both halves; backward half accepts leftover
  concrete values — shell semantics keep them correct) AND callee is
  the same registered fn signature as the enclosing frame (identity
  from the marked frame-open) AND `Gen == nil` AND no capitalised
  teardown names AND no defs installed during arg auto-eval (§2.4)
  AND not check mode. Everything else nests as today.
- **Mechanism:** eagerly execute the scanned tail markers via the
  Stage-1 helper, delete the marker run, splice the callee tokens over
  the call region exactly as `spliceMatchResults` does today. The
  caller's `( __RC )` shell stays; paren balance is untouchable by
  construction.
- **Effect:** all three per-call stacks O(1); tape residue 3
  tokens/call. Not yet constant space — a real ~5–10× depth headroom
  improvement and full validation of probe + eager teardown in
  production, with semantics provably unchanged.
- **Gates:** kill switch (default on, instantly revertible); the
  dual-mode differential — the entire spec suite executed with TCO on
  and off must produce identical results, which turns every existing
  row into a TCO test for free; step accounting unchanged so an
  infinite tail loop still trips `evaluation_limit`.

### Stage 4 — full frame replacement, then general tail calls

Two sub-steps, same apparatus:

1. **Self-recursion, full replacement.** Splice
   `[frame-open .. frame-close]` with the callee's `( … )`; keep the
   callee's `__RC` (identical to the caller's for self-recursion);
   backward half of the probe tightens to reject leftover values.
   Constant space lands here: the `s2 1_000_000` row, plus rows
   asserting the runaway taxonomy (tail runaway → `evaluation_limit`,
   non-tail → `tape_exhausted`).
2. **Any fn→fn tail call** (mutual recursion) through the
   `buildFnBodyHandler` path, with the return-conformance gate:
   caller has no declared returns, or callee's returns are declared
   and conform (callee ⊑ caller, via the existing `v.Is`/ConformsTo
   machinery). Captures allowed (rows for shadowing across a TCO'd
   call); generics admitted only with rows proving the
   bind/teardown/Retire interaction.

The novel invariant to audit before this stage merges: TCO edits the
tape *ahead of the pointer*, which ordinary dispatch never does. Every
site that holds a tape index across a `stepWord` (the in-paren
re-evaluation loop's `openIdx`, `stepMoveCont`'s mark/move indices,
macro expansion, recorder skip windows) must be checked against
ahead-edits; the in-paren loop's recalc-after-every-step discipline
appears to cover it (the spliced region is strictly inside the frame,
which is strictly inside any enclosing held region), but that is an
audit conclusion to write down, not assume. If any site is doubtful,
gate Stage 4 to top-level dispatch first and lift after the audit.

### Stage 5 — CallAQL trampoline (module fns)

Separate Go-recursive path (`Registry.CallAQL`, captured
sub-registry); its TCO analogue is a standard trampoline — detect the
tail self-call and loop within one Run invocation. Independent code
path, independently testable, needs its own dispatch trace first (the
lesson of TCO.10.md's first draft: trace before sketching). Ordering
is free — it can land any time after Stage 1.

### Stage 6 — make it a guarantee

Only after Stage 4 has soaked: document tail-call elimination in
REFERENCE/EXPLANATION as a language property, revisit the default tape
ceiling (less headroom needed for legitimate recursion), and demote
the kill switch to a diagnostic. This is the point of no return —
once documented, programs will *require* TCO, and turning it off
becomes a semantics break. Everything before this stage is
reversible; that is most of why the stages are ordered this way.

## 4. Cross-cutting safety apparatus

- **Kill switch** from Stage 0; every behavioural stage ships behind
  it, default-on, bisectable in the field.
- **Dual-mode differential suite** as a CI job from Stage 3: full spec
  suite, TCO on vs off, byte-identical outputs. Strongest single tool
  here; cost is one extra suite run.
- **Probe property tests** from Stage 2 (fuzz tape shapes; assert
  fire ⇒ dual-mode equivalence on generated programs once Stage 3
  lands — the engine itself is the reference model, mirroring
  `TestTapeDifferential`).
- **Hot-path budget**: benchstat on dispatch microbenchmarks and the
  RECURSION-PERFORMANCE acceptance numbers at every stage; the
  non-fn-dispatch cost must stay at one nil check.
- **Negative rows alongside every positive** (repo rule): for each
  shape that must fire, a sibling that must *not* (the forward-below
  shapes, value-accreting bodies, RC mismatch, generics until
  admitted, capitalised teardown names).

## 5. Alternatives reviewed

- **Explicit `recur` word** (TCO.10.md's stepping stone): *not*
  "trivially correct" as described. Non-tail `recur` (`n add (recur
  (n sub 1))`) hits exactly the §1c hazards, so a safe `recur` needs
  the same probe (or a checker pass that AQL doesn't have) — at which
  point it is Stage 3 with extra permanent syntax users must learn
  and the natural spelling still unoptimised. The detection dry-run
  (Stage 2) delivers the de-risking `recur` promised, without the API
  commitment. Recommend dropping it.
- **Library-level relief valve**: a native `iterate`-style word (apply
  a Function value to a seed until a sentinel, looping in Go around
  `CallAQL`) gives users constant-space functional iteration *today*
  with zero dispatch-core risk. Orthogonal to all stages; worth
  considering if expressiveness pressure arrives before Stage 4. `for`
  already covers the imperative shape.
- **Lazy marker squashing**: rejected in TCO.10.md; concur — it never
  fires on the runaway case and bounds nothing but markers.
- **Separate call stack** (RECURSION-PERFORMANCE remediation 1):
  strictly larger blast radius than frame replacement for the same
  tail-recursion payoff; the gap-buffer tape already removed the
  quadratic cost for non-tail shapes. Out of scope.
