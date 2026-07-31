# boru — Lean formalization + differential harness (prototype)

A machine-checked prototype of boru's core semantics in Lean 4, plus a
"tracer-bullet" harness that differential-tests the model against the
real `boru` engine. This is the first concrete step of the milestone-6
plan in
[`design/FORMAL-VERIFICATION.0.md`](../../design/FORMAL-VERIFICATION.0.md):
a *deep embedding* of a tractable fragment of the abstract machine
specified in [`FORMAL-SPEC.md`](../../FORMAL-SPEC.md) §4 and §6.

## Layout

| File | What it is |
|------|-----------|
| [`BoruCore.lean`](BoruCore.lean) | the model (library): syntax, values, words, forward collection, evaluation, the theorems |
| [`Demo.lean`](Demo.lean) | a runnable wrapper (`lean --run Demo.lean`) printing the executable sanity checks |
| [`harness/tracer.py`](harness/tracer.py) | the differential harness: runs each program through **both** the model and `boru do`, then compares |
| [`lean-toolchain`](lean-toolchain) | pins `leanprover/lean4:v4.15.0` |

## What the model covers

`BoruCore.lean` models two value types (`Int`, `Bool`), variable-arity
words (unary `not`; binary `add`/`sub`/`mul` and comparisons
`gt`/`lt`/`eq`), the data stack, the `end` barrier, and — the point of
the exercise — **forward collection** (`FORMAL-SPEC` §6.4): a word
gathering up to `arity` leading literals to its right, filling any
remaining slots from the stack top-first, and leaving leftover literals
on the stack (the engine's `add 1 2 7 => 3 7`).

### Theorems (machine-checked; `lean BoruCore.lean` exits 0, no `sorry`)

| Theorem | Statement |
|---|---|
| `spelling_equiv` | for **any** binary word and operands, the spellings `y x op`, `y op x`, `op x y` agree |
| `sub_spellings` | the concrete `sub` case, `= some [Int 7]` |
| `unary_spellings` | a unary word collects its arg forward *or* from the stack |
| `binary_caps_and_leaves_leftover` | arity-2 collection caps and pushes the leftover (`add 1 2 7`) |
| `barrier_blocks_forward` | an `end` under-supplies the word (a *negative* result) |
| `run_deterministic` | evaluation is deterministic |
| `sub_trans` (+ lattice examples) | the §5.1 type lattice `⊑` is reflexive and transitive |

## What the model deliberately does NOT cover

The engine is *coercive* where this model is *strict*: `boru do 'add true
2'` is `"2true"` and `'not 5'` is `false`, whereas the model treats a
type-mismatched argument vector as a stuck signature error. Those
coercion rules — and user functions, modules, effects, concurrency,
floats/overflow, and refinement predicates — are later milestones. The
harness therefore covers only the homogeneous-type fragment where model
and engine agree.

## The harness — what it does and does not prove

`harness/tracer.py` drives one thin slice through the whole pipeline:

```
boru source ──tokenize──▶ Lean Tape ──lean──▶ model output
     │                                            │
     └─────────── boru do ─────▶ engine output ───┴──▶ compare
```

It compiles `BoruCore.lean` (re-checking the proofs), generates a Lean
module that evaluates every program, runs the reference `boru` engine on
the same sources, normalizes both outputs, and asserts agreement.

This is **differential testing, not proof**: it can *refute* "the model
matches the engine" but never prove it. Its value is turning the
model↔engine correspondence from a handful of hand examples into a
re-runnable check whose failures point at exactly where model and
implementation diverge. The proofs live on the model side; the harness
is the (cheapest, §4.3) bridge to the implementation.

Current run: **22/22 programs agree** (spelling equivalence, arithmetic,
comparisons, the unary word forward and from the stack, arity/leftover,
the `end` barrier, and underflow).

## Build / run

The toolchain is pinned in [`lean-toolchain`](lean-toolchain). With a
standard Lean install ([`elan`](https://github.com/leanprover/elan)):

```bash
cd formal/lean
lean BoruCore.lean                 # type-check the proofs (exit 0, no `sorry`)
python3 harness/tracer.py         # differential-test against the engine
```

The harness builds the `boru` engine itself (`go build` in `cmd/go`).
Override discovery with `LEAN=/path/to/lean` and/or `BORU=/path/to/boru`.

### Offline / restricted-network install

If `elan`'s default download host is blocked but GitHub is reachable:

1. fetch the `elan` binary from `github.com/leanprover/elan/releases`
   and run `./elan-init -y --default-toolchain none`;
2. download `lean-4.15.0-linux.tar.zst` from
   `github.com/leanprover/lean4/releases`, decompress (`zstd`, or
   `python3 -c 'import zstandard,sys; ...'`) and untar;
3. `elan toolchain link boruproto <extracted-dir>`.

Because the pinned `lean-toolchain` names a channel `elan` would try to
fetch from the blocked host, invoke the linked binary directly in such
environments, e.g. `LEAN=/root/lean/v4.15.0/bin/lean python3
harness/tracer.py`.
