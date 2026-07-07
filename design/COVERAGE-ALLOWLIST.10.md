# Coverage allowlist — provably-unreachable defensive guards

`make cover-gate` (ADR-008) enforces **100% unit-test coverage of every
*reachable* Go statement**. A small, reviewed allowlist —
`test/go/covergate/allowlist.tsv` — names the blocks that are excluded
from that floor because they are **provably unreachable through any test**
while remaining **valuable defensive code**. This note explains what may go
on the list, why, and the discipline that keeps it honest.

## Why an allowlist rather than "literally 100%"

Two roads lead to a green 100% gate for a block that no input can execute:

1. **Delete the guard.** Correct *only* when the code has no defensive
   value — a tautological comparison, a shadowed branch, a `return` after
   a loop that always returns. Those are removed at source under ADR-005
   ("no deliberate dead code"), not allowlisted.
2. **Keep the guard, exclude it.** Correct when the block is an
   **error-propagation or safety arm** whose call cannot fail *today* but
   defends against a future change, an external library, or data
   corruption. Deleting it to win a coverage number would silently drop a
   real error if that call ever started failing — a net regression. These
   are allowlisted.

The whole 2026-07 quality program pushed coverage from 78.5% to 100% of
reachable statements precisely by making the hard edges reachable with
**mocking seams** (design/TEST-SEAMS.10.md). The allowlist is the residue:
the arms where a seam would either sit on a hot dispatch path (measurable
allocation regression — see the reverted `screenResults` method-value
seam) or cannot force the failure at all because the guarded call is
total for every constructible input.

## What is on the list (categories)

Each entry is a cover-profile block key (`<file>:<startLine>.<col>,<endLine>.<col>`)
and a one-line reason. The categories:

- **§engine** — interpreter step/dispatch defensive index and error arms
  in `eng/go/engine.go`. Tautological index comparisons (a backward scan
  makes `fwdIdx < endIdx` a theorem), `stepEnd` error arms (`stepEnd`
  only ever returns nil), and `TopTypeBody`/`Lookup` arms shadowed by a
  preceding `Defs.Top`. Reachable only through the lang layer or a
  pathological program, which the cross-package census already runs.
- **§compiler** — bytecode compiler / VM defensive arms
  (`callable_words`, `emit`, `vm`, `lower`, `method_shape`, `user_poly`):
  the belt-and-braces result screens and match-drift refusals that a
  compiled-reachable handler cannot trigger (the emitter refuses the
  words that could produce a tape-coupled token; a real island re-steps
  one rather than returning it).
- **§kernel** — shared-assertion and gate-guaranteed kernel guards: an
  `As<T>` error arm sitting under an `Is<T>` guard that keys on the
  *identical* type assertion, or a read that its enclosing `containsFlex`
  / `containsSharedMutable` gate already proved will succeed.
- **§native** — native word-handler defensive error-propagation and
  same-assertion guards (the `anyToValue` convert-back arms that
  `valueToAny`'s JSON-shaped output never trips, `Is*`/`As*` pairs).
- **§modules** — module provably-invariant and grammar-defensive guards
  (a beam-trim whose queue is invariant at length 1, an emit-kind lookup
  whose input is always registered, re-validation shadowed upstream).
- **§formatter** — the pretty-printer's inline/tokenizer guards (a
  word-scan that always advances, single-line returns unreachable once
  the multi-line branch was taken).
- **§crypto** — vault crypto **defense-in-depth**: hex/base64 decodes of
  values the vault itself just minted with the inverse encoder, and a
  reserved-key check shadowed by an earlier alias validation. Kept
  deliberately — these defend stored-secret integrity against a future
  format change, and are the category we are *least* willing to delete.
- **§misc** — the lone `exec.NewServer` error arm (that constructor has a
  single `return s, nil`).

The per-block proofs live in the wave-6…9 coverage-agent reports; the
category reason on each line is the durable summary.

## Discipline — the list cannot rot

`covergate` treats the allowlist as **live**, not fire-and-forget:

- If an allowlisted block is **actually covered** by some suite, the gate
  **fails** and names it: the guard became reachable, so cover it and
  delete the exclusion ("graduate" it).
- If an allowlisted key matches **no** profiled block (a rename or a
  deletion left it dangling), the gate **fails** as stale.

So the list can only shrink or move deliberately. Adding an entry is a
reviewed act: it needs a proof that no test can reach the block and an
argument that the guard earns its keep as defence. When in doubt, prefer
a seam (make it reachable) or deletion (if it is truly dead) over an
allowlist entry.
