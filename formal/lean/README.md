# AQL — Lean formalization (prototype)

A mechanized, machine-checked prototype of AQL's core ideas, in Lean 4.
This is the first concrete step of the milestone-6 plan in
[`design/FORMAL-VERIFICATION.0.md`](../../design/FORMAL-VERIFICATION.0.md):
a *deep embedding* of a tractable fragment of the abstract machine
specified in [`FORMAL-SPEC.md`](../../FORMAL-SPEC.md) §4 and §6.

## What it covers

[`AqlCore.lean`](AqlCore.lean) (single file, no Mathlib) models integer
literals, binary words (`add`/`sub`/`mul`), the data stack, the `end`
barrier, and — the point of the exercise — **forward collection**: a
word drawing its arguments from both the tokens to its right and the
values already on the stack (`FORMAL-SPEC` §6.4).

It then proves:

| Theorem | What it states |
|---|---|
| `spelling_equiv` | the three spellings `y x op`, `y op x`, `op x y` all compute `applyOp op x y` — for **all** ops and operands (not enumerated examples) |
| `spelling_agree` | …and therefore agree with each other |
| `barrier_blocks_forward` | an `end` between a word and a forward operand under-supplies the word (a *negative* result — what must be rejected) |
| `run_deterministic` | evaluation is deterministic |
| `sub_trans` (+ `Integer ⊑ Any`) | the `FORMAL-SPEC` §5.1 type lattice: `⊑` is reflexive and transitive |

The file is also executable: `#eval`s and a `main` reproduce
`aql do '...'` so the same artifact proves *and* computes.

## Scope (honest limits)

Covered: the binary-word fragment + forward collection + the `end`
barrier + a slice of the type lattice. **Not** covered (later
milestones): user functions, modules, effects/capabilities, concurrency,
floats/overflow, and refinement-predicate discharge.

## Cross-validation against the real engine

The model's normal forms were checked against the reference
implementation; they agree:

```
aql do 'sub 3 10'      => 7      aql do '10 sub 3'  => 7
aql do '10 3 sub'      => 7      aql do 'mul 4 5'   => 20
aql do '10 sub end 3'  => error: [aql/signature_error] no matching signature for sub
```

This is the cheapest bridge of `FORMAL-VERIFICATION.0.md` §4.3: Lean as
the spec, validated against the engine via examples (and, in the
durable version, the `*.tsv` conformance corpus).

## Build / run

The repo pins the toolchain in [`lean-toolchain`](lean-toolchain)
(`leanprover/lean4:v4.15.0`). With a standard Lean install
([`elan`](https://github.com/leanprover/elan)):

```bash
cd formal/lean
lean AqlCore.lean        # type-check the proofs (exit 0, no `sorry`)
lean --run AqlCore.lean  # run the executable cross-checks
```

### Offline / restricted-network install

If `elan`'s default download host is blocked but GitHub is reachable,
install the toolchain straight from GitHub release assets:

1. fetch the `elan` binary from
   `github.com/leanprover/elan/releases` and run `./elan-init -y
   --default-toolchain none`;
2. download `lean-4.15.0-linux.tar.zst` from
   `github.com/leanprover/lean4/releases`, decompress (`zstd` /
   `python3 -m … zstandard`) and untar;
3. `elan toolchain link aqlproto <extracted-dir>` and `elan default
   aqlproto`.

(`elan toolchain install` itself pings `release.lean-lang.org`, so the
link-a-local-toolchain route above is the one that works behind a
restrictive policy.)
