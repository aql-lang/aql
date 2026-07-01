# Capture-threaded interpreter islands (Stage 5 follow-on)

Status: **design note / implementation plan — no code yet.** Grounded in the
live tree at the merge of PR #210 (`c61f86e`). This is the concrete next
increment for Stage 5 (span-level `OpFallback` islands). See
`aql-bytecode-plan.0.md` (Stage 5) and `aql-bytecode-runtime-independence.0.md`
(the P7 refusals→0 / islands→0 finish line).

## 1. Where we are

The island machinery is **fully built but dormant**: `OpFallback`
(`bytecode.go`), `FallbackSpan` (`bytecode.go:387`), the VM's `runFallback`
(`vm.go:554`), `lowerFallback` (`lower.go:1409`), and the record-side
`tryRecordFallback` → `RecordFallback` (`carrier.go:1112`, `emit.go:1955`) all
exist. Yet **0 rows island** (`COMPILED_STATUS.md`: islanded = 0). Reason: the
easy higher-order words already compile natively through the closure path
(`tryRecordClosure`), so the island path never fires; and every *remaining*
refusal is past a capability the island path does not yet have.

The 31 value-row whole-program refusals (excluding the ~80 `unmatched dispatch
recovered` ERROR rows, which are correctly dispositioned as error-parity, not
debt) fall into families that each need a distinct new capability:

| family | rows (examples) | missing capability |
| --- | --- | --- |
| code-body word w/ outer-local body | `case.tsv:50` (`do`), `corpus-core.tsv:50` (`walk`), `module-test.tsv:48` (`test-check-prop`) | **capture-threaded islands** (this note) |
| dynamic method / fn-value dispatch | `module-log.tsv:53–80` (`l.info`, `c.add`, `s.finish`, …) | dynamic method dispatch on a dynamic receiver (`OpCallDynamicMixed`-class) |
| function-valued operand | `recursion.tsv:90–92` (`apply`), `module-log.tsv:62,83` (`log-register`) | Stage-3 fn-as-argument lowering |
| residual / operand provenance | `path-modifier.tsv` (`m.a/u …`), `module-parse.tsv:14–18` | provenance recovery — the operand has no compiled home; **not islandable** (nothing to thread) |
| if / fn body provenance | `recursion.tsv:71,72` | branch/closure residual provenance |

Only the first family is addressable by an island: the others are either
dynamic-fn machinery or genuinely unmaterialisable operands.

## 2. The gap

`tryRecordFallback` already islands a code-body higher-order word when its body
is *self-contained* — `bodyFreeForFallback` (`carrier.go:1269`) requires every
body word to be a registered native / fn-def (`r.Lookup`) or a known literal.
It **rejects a value-`def`-bound name** on purpose: at VM run time that binding
is not a live registry def (top-level value-defs compile to frame locals /
consts, not a re-run of `def`), so the island's shared-registry sub-engine
(`vc.island() = New(vc.r)`, `vm.go:140`) cannot resolve it — it would diverge.

The three code-body rows all reference exactly such outer locals in their
bodies:

- `case.tsv:50`: `def e (do [raise bad_input "nope"]) do [e dot code case […]]`
  — body references `e` (a runtime error value).
- `corpus-core.tsv:50`: `def acc (flex []) walk … (m:Any => [acc (m.path) append]) acc`
  — lambda captures `acc` (a flex). (`flex` also needs VM reference-cell
  support — see `reducibleWords`; this row is doubly blocked.)
- `module-test.tsv:48`: the Test harness — additionally needs cross-registry
  module-preamble closures.

So the clean, general first target is **`do` / `walk`-shaped code bodies that
capture a runtime-computed outer local whose value has a compiled home**
(a frame local or a prior event result). `case.tsv:50` (single capture `e`,
no flex) is the minimal proof-of-concept.

## 3. The increment

Thread the body's free outer-local references into the island as **named
bindings**: preload each captured value and install it as a `def` in the
sub-engine before running the span.

Four coordinated changes:

1. **Record side (`carrier.go`, `tryRecordFallback` / `bodyFreeForFallback`).**
   When a body word is neither a `Lookup` hit nor a literal, check whether it
   resolves to a value with **compiled provenance** (a frame local or a prior
   compiled event result — the same `resolveOperand` test the data-thread path
   uses). If every otherwise-free name does, collect them as *captures* instead
   of refusing. A name with no compiled home still refuses (whole-program
   fallback stands — soundness preserved).

2. **`FallbackSpan` (`bytecode.go:387`).** Add `Captures []FallbackCapture`
   where `FallbackCapture{Name string}`; the captured values ride the operand
   stack like threaded inputs (deepest-first, in a fixed order parallel to
   `Captures`). Bump `NIn` semantics to `len(Captures) + dataThread`.

3. **Lowering (`lower.go:1409`, `lowerFallback`).** Lift the current ≤1-input
   limit for the capture case: push the captured operands in `Captures` order
   (deepest first), then the ≤1 data-thread input on top, then `OpFallback`.
   The ordering assertion in `runFallback` (`vm.go:565`, currently
   `NIn > 1 → error`) becomes an ordering *contract* the lowerer must honour.

4. **VM (`vm.go:554`, `runFallback`).** Pop `len(Captures)` values, install
   each as a `def` binding (`r.Defs.Push(name, v)`) in the island sub-engine,
   run the span, then pop the bindings (`r.Defs.Pop`) — a scoped install/teardown
   around `vc.island().Run(island)`, mirroring a fn-call frame. The data-thread
   input (if any) still preloads onto the operand stack as today.

## 4. Soundness

Per the design philosophy (`COMPILABLE-SUBSET.md` §8), **soundness rides on the
differential gate**: an island's result must equal the interpreter's. The
argument here:

- A captured value is the program's **real runtime value** (from its compiled
  home), so binding it as `def name = v` in the sub-engine reproduces exactly
  the scope the interpreter's body sees.
- The island's residual is treated as **dynamic** (`anyDynamicCarrier` poisons
  any downstream typed dispatch), so no downstream compiled site can make an
  unsound assumption about the islanded result.
- `break`/`continue`/`return` in the body still refuse (`bodyFreeForFallback`
  already bails on sentinels) — no cross-boundary flow-control hazard.
- `flex` captures stay refused until the VM value model grows reference cells
  (separate tier-2 item) — so `corpus-core.tsv:50` is NOT unblocked by this
  increment alone; `case.tsv:50` (POC) and any non-flex capture is.

The gate suite is the proof (all must stay green, `-count=1`):
`TestSpecCompiledDifferential`, the compiled property gate, the full-corpus
gate, `verify-bytecode`, and the cross-engine crossdiff. `make status`
regenerates `COMPILED_STATUS.md`; the census ceilings
(`compiled_metafallback_test.go`: `reducibleCeiling`, `computeRefusalCeiling`,
and the island count) ratchet **down** as rows move refused → islanded.

## 5. Scope / risk

This is a multi-file change in the most miscompile-sensitive part of the tree
(cf. `MISCOMPILE-HUNT-FINDINGS.0.md`). The N-input ordering (step 3) is the
delicate part — the current code bounds threading to 1 precisely because
multi-input preload order is the inverse of the interpreter's top-down
collection (`vm.go:565`). Recommended sequencing:

1. **POC** — single capture, `case.tsv:50` only (threading 1→2 with a strict
   deepest-first contract + an assertion). Prove islands 0→1 with the full gate
   suite green, then `make status`.
2. **Generalise** — N captures, ordered; unblock the remaining non-flex
   code-body rows.
3. **Follow-ons** (separate notes): `flex` reference cells; dynamic method
   dispatch for the `module-log` family; fn-as-argument lowering.

No ADR — this is discovery per the repo's ADR rule.
