# Roc Adoption Plan — sequencing the findings into landable work

## Scope

`design/roc-in-boru-report.0.md` produced fourteen recommendations
(A1–A14), two registered non-uniformities (NUR079, NUR080) and a list of
declines. This note turns that into an ordered programme: what lands
first, what gates what, and what "done" means for each item.

It is a **plan**, not a decision record. Nothing here amends an ADR, and
each item still needs its own design note where the report's sketch is
not already sufficient. Items are named by their report IDs so the two
documents stay cross-referenceable.

The report's own closing warning is the sequencing principle: *the
failure mode is spending the novelty budget on ecosystem machinery while
the four-line fixes to `check` and `describe` stay unmade.* So the order
below is **repairs → cheap-and-certain → gates → visible answers →
supply chain → performance**, not the order of intellectual interest.


## 1. Sequencing principles

1. **A defect outranks an adoption.** NUR079 and NUR080 are live
   divergences the report surfaced; they are Phase 0 regardless of how
   cheap the adoptions are.
2. **Cheap and certain before grand and speculative.** Nine of the
   fourteen items are a flag, a document, or a call site. They land
   first because they are independently verifiable and cannot be
   invalidated by later design decisions.
3. **Never ship a lever without its dial.** `--pedantic` (A3) without a
   discoverable code catalogue (A9) gives users a gate they cannot aim.
   Pairs like this ship together or in immediate succession.
4. **Do not build a gate on an unstable surface.** A7's doc↔registry
   gates include tagging the manual's code fences; that step must wait
   for A12, or `make fmt-docs` corrupts `REFERENCE.md` the first time it
   runs.
5. **Advisory before gating.** Every new analysis facet (A8, A13) enters
   at info severity behind a flag. An over-approximating effect set that
   *gates* would break boru's open-by-default polarity — see the
   report's pitfall table, and note `unconditional_raise` is already
   firing that way today.


## 2. Dependency graph

```
NUR080 ─────────────────────────► (independent)

NUR079 ──► A1 Phase 1 ──► A1 Phase 2 ──► A8 step 2 (check --perms shadow eval)
                              │
                              └────────► A11-policy (playground policy)

A3 ──┬── A9/A16 (code catalogue + boru explain)
     └── (unlocks every advisory boru has or will build)

A12 (settle fmt, close NUR046) ──► A7 second half (```boru fence tagging)
A7 first half (word/scope/code tables) ── independent

A8 step 1 (describe Effects:/Raises:) ──► A8 step 2 (propagated raise sets)
A13 (declared types in describe) ── independent
A6 (check diagnostic parity) ── independent, but A6(a) gates the LSP range fix

A10 (front-end amortization) ──► needs the AOT-COMPILE §3.2 codec first
A5 (artifact digest) ── independent; CONTENT-ADDRESSING.0 §4.1 lists no blockers
```

Only three real chains exist. Everything else is parallelisable, which
is why the phase boundaries below are about *risk and review load*, not
technical necessity.


## 3. Phase 0 — the two defects

Both carry reproductions in the report §7 and NUR records; neither is
optional, and NUR079 is the only live compromise in the whole study.

### P0.1 — NUR079: policy must reach inside module bodies

Two halves, the second load-bearing:

- **(a) Wire the flags that already exist.** Register
  `cmd/go/internal/permsflags` on `check`, `describe` and `repl`, and
  pass a resolved policy in `cmd/go/internal/lsp/diagnostics.go` instead
  of an empty `lang.Options{}`. Honour `BORU_POLICY`, which
  `REFERENCE.md` documents and `check` ignores. `run` already does all
  of this — `permsflags.Register(fs, &pf)` — so this is reuse.
- **(b) Carry the policy into the module sub-registry.**
  `runModuleBodyCover` inherits output, the effect ledger, observe
  hooks, runtime stamping, `HostFileOps`, `CapMemFileOps`, host formats,
  `ParseFunc` and `BaseDir` — and no policy — while every gate treats a
  nil `HostPolicy` as allow. Attenuate, never widen; `Vm.run-sandbox` is
  the precedent.
- **(c) An egress floor during check.** Compose over whatever the user
  asked for so an analysis pass can never reach the network whatever the
  profile says, with a distinct diagnostic code when a module fails to
  load because of the floor. Add `boru check --no-exec-imports`.

**Done when:** a profile denies an in-body gated call; `boru check
--perms <profile>` is honoured; and the `read-only` reproduction in
report §7.1 exits non-zero with zero requests.

### P0.2 — NUR080: mint a fresh value for typed defs over literals

The static/concrete arm of `def name:T <literal>` must mint rather than
reparent the shared const-pool entry.

**Done when:** both source orderings agree across `--no-compile` and the
default, **and** `lang/spec/*.tsv` carries rows for the class so
`make verify-bytecode` can see it. The missing spec row is why the
differential corpus was blind; the fix is not complete without it.


## 4. Phase 1 — cheap and certain

Each is independently shippable in a single small PR.

| Item | Change | Done when |
|---|---|---|
| **A3** | `boru check --pedantic` — promote warnings/infos to a non-zero exit *inside* the existing `!soft` guard, at both the JSON and text sites | `check --pedantic` on a warning exits non-zero; `--soft --pedantic` still exits 0; documented in CLI.md and REFERENCE.md |
| **A4** | `describe` examples engine-verified: record the engine's error code instead of swallowing it, close the two verifier exclusions | no shipped example renders `;# ...`; until then AGENTS.md's no-drift claim is scoped to what is gated |
| **A14** | REPL piped-stream contract: results→stdout, diagnostics→stderr, no banner in non-tty mode | a piped `boru repl` writes only evaluation results to stdout |
| **A11** | Outward layer: repo description and topics, the playground URL in README, status above the fold, `examples/` promoted from `design/examples/apps/`, a CONTRIBUTING that names the real gates (`cover-gate` currently appears only in CLAUDE.md) | a contributor following README's pre-commit block passes CI |

**A3 lands first** and is the subject of the first implementation
commit alongside this plan. It is the smallest change with the widest
reach: boru currently ships a `warning` severity nothing can act on, so
every advisory in the tree and every advisory these phases add inherits
that ceiling.


## 5. Phase 2 — the honesty gates

The report's most-corroborated cluster (A7 was reached independently by
five of eight axes). These are gates that make a whole class of drift
impossible rather than fixing instances of it.

- **A12 — settle `fmt`.** Decide and record per sugar what the formatter
  normalises to; close NUR046 so pass 1 is a fixed point; then add
  `--check` and `--stdin`, keeping zero *style* options. A single-pass
  `--check` is unsound while 19 files need two passes, which is why this
  precedes A7's second half.
- **A7 — doc↔registry gates.** Every backticked word in a REFERENCE.md
  table resolves in the live registry; every scope name is in
  `policy.KnownScopes`; every diagnostic code appears in a table and
  vice versa. Then fix what it finds — the `fileio` vs `fileops` case
  answers ALLOW with exit 0 today, which is the compounding failure the
  gate exists to stop.
- **A9/A16 — `boru explain <code>` and a generated code table.** Ships
  with or immediately after A3, per principle 3.

## 6. Phase 3 — publish the answer at build time

This is the report's thesis made concrete, and the first phase where the
work is real engineering rather than plumbing.

- **A6 — `check` says what `run` already says.** Populate `Src` at the
  diagnostic construction sites so caret width stops being `len(Word)`;
  add `Spans` mirroring `BoruError`; fix the `drypass` reachability
  predicate. The renderer, the data-only builders, the LSP forwarding
  and the JSON schema all already exist — only the checker fails to fill
  them in.
- **A8 step 1 — `Effects:` and `Raises:` in `describe`**, seeded
  mechanically from the `policy.Check` gate sites and the
  `r.BoruError(code, message, word)` mint sites, gated by a drift test.
  Advisory only.
- **A13 — show declared types back.** A user fn's declared return types
  are currently dropped by `describe`; `boru describe <Type>` answers
  "no description available". Both are cheap and both are the
  transferable half of Roc's day-to-day payoff.
- **A8 step 2 — propagated raise sets** behind `check --raises`, only if
  step 1 earns it. Closed per-call-site sets, never open rows (report
  §5).

## 7. Phase 4 — supply chain

**A5 — the digest is the identity.** `boru pack` prints `sha256:` of the
archive; `boru install` hashes the body before extraction and refuses on
mismatch, with `--frozen` for CI. The digest must **not** come from the
registry — a digest served by the host that serves the bytes
authenticates nothing. Unconditionally: hash the registry's tokens and
add expiry. Conditionally on a product decision: name ownership, worth
building only if boru intends to operate a public registry.

Package-composition defects (install not running `prep`, `-r` flag
parsing, `build` not bundling installed deps) ride along here; they are
reproduced and none is a design question.

## 8. Phase 5 — performance

**A10 — amortize the front end.** Implement the AOT-COMPILE §3.2
pointer→symbolic codec, extend `buildrt.Config` with an optional
`Program`, make `ResolveCompileMode` cost-aware, and add a
`bench/scripts/` matrix that reports all four {check × compile} cells
rather than derived subtractions. Largest item in the queue and the one
most likely to be resequenced by measurement; it is last because
nothing else depends on it.

## 9. Explicitly not in this plan

The report's §5 declines are decisions, not backlog: the `!`/`$`/`?`
sigils, a call-site fallibility marker, open tag unions, Perceus/FBIP
in-place mutation, platform-chosen allocators, `x.to_str()` static
dispatch, implicit structural conformance, inline `expect` with
build-mode erasure, a version solver, non-blocking compilation as the
*default*, a third-party native platform tier, and the community/funding
apparatus. Each is argued there; none should reappear as a task without
re-opening that argument.

## 10. Status

| Item | Phase | Status |
|---|---|---|
| NUR079 | 0 | Pending — recorded, not started |
| NUR080 | 0 | Pending — recorded, not started |
| A3 | 1 | **Landed** with this plan |
| A4, A11, A14 | 1 | Not started |
| A7, A9/A16, A12 | 2 | Not started |
| A6, A8, A13 | 3 | Not started |
| A5 | 4 | Not started |
| A10 | 5 | Not started |

Keep this table current as items land; it is the one part of this note
that is meant to change.
