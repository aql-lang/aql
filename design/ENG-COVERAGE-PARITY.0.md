# ENG-COVERAGE-PARITY.0 — the standalone 100%/100% program for the kernel twins

**Status:** In progress · **Started:** 2026-08-04 (maintainer
instruction: "Both the go and ts coverage of eng needs to be 100% and
it needs to be standalone without relying on tests of other modules.
Go and ts need total parity.")

## The contract

1. **Standalone.** eng proves itself with its OWN suite, on both
   implementations. The repo-wide ADR-008 gate keeps measuring the
   merged cross-suite profile (lang's suites legitimately cover eng
   statements there); the standalone gates measure eng/go by eng/go's
   tests alone and eng/ts by its own `node --test` suite.
2. **Parity of metric.** Go's gate unit is the covered STATEMENT
   (covergate over a `-coverpkg`-instrumented profile, pragma
   allowlist for provably-unreachable guards); TS's gate unit is the
   covered LINE (node:test's built-in coverage, `node:coverage`
   ignore comments reserved for the same provably-unreachable class).
   Go statements ≡ TS lines is the parity equivalence.
3. **Target 100% on both; floors are ratchets.** Until the target is
   reached, each gate enforces a floor that ONLY RISES:
   - Go: `make cover-gate-eng`, floor `ENG_GATE_FLOOR` (root
     Makefile). Current: **89** (measured 89.7%,
     22,252/24,796 statements).
   - TS: `make test-ts`, floor `TS_GATE_LINES` (root Makefile).
     Current: **93** (measured 93.09% lines; branches 87.2%,
     functions 90.9% recorded but not yet gated).
   Raise the floor in the same PR that raises coverage; lowering
   either floor is a build break by intent.
4. **Parity of corpus.** The shared `eng/spec` corpus remains the
   behavioral contract both engines replay row-for-row (the
   cross-engine differential). Corpus growth serves both engines'
   coverage at once and is the preferred instrument wherever a gap is
   expressible as rows; engine-local tests carry the rest.

## What already stands (2026-08-04)

- **The standalone corpus lanes** (`eng/go/corpus_standalone_test.go`):
  interpret, check, and compile-or-fallback over the full `eng/spec`
  corpus on a bare `eng.NewRegistry` + the `specfix` fixture words —
  with a mechanism-only stand-in for the content layer's Micron Ideal
  (Pathon rows construct through the kernel's own `MakePathon`; the
  six Emailon/Urlon validator rows skip via `specfix.ErrSkipRow` and
  stay covered by the full harness in `test/go/engspec`).
- **`eng/go/specfix`** — the TSV runner (formerly
  `test/go/specrunner`) and the fixture-word registrations (formerly
  private to `test/go/engspec`) as one eng-owned package, shared by
  the standalone lanes, the engspec harness, and the cross-engine
  differential. Fixture declarations were upgraded to carry the REAL
  declared models where the compiled lane needs them (the stack words'
  `ReturnsIdentity` wiring — an empty `Returns` compiled splice bodies
  to programs whose operands vanished).
- **A kernel soundness fix the lanes exposed**: the interpolation bake
  probe (`evalInterpParts`) tested only the hole's own Carrier flag, so
  a hole evaluating to a CONTAINER holding a carrier (`${[1 addq 2]}`
  with a non-folding word) baked the check-time type-tag render as a
  constant string. The probe is now recursive
  (`valueCarriesCarrier`); with lang's folding words the old bake was
  coincidentally sound, which is why it survived until a
  non-folding fixture ran under the compiled lane.
- **The compiled lane honours the VM's soundness-bailout contract**:
  an `internal_error` from `RunProgram` falls back to a fresh
  interpreter run, mirroring lang's compiled-by-default entry.
- **specfix meets ADR-008 as production code** (2026-08-05): the
  fixture words' guard arms the corpus never reaches are pinned by
  `eng/go/specfix_probe_test.go` (class types, dep scalars, table
  types, and a signature-less fn constructed through the kernel's
  exported API and bound via the def table), the runner's rendered-skip
  arm and the map walk's error arms by specfix's own unit tests, and
  the provably-unreachable remainder (raw NoEval list slots are always
  concrete; sole-caller pre-checks; runtime-only handlers never see
  non-concrete containers) carries proof-carrying `//covergate:allow`
  pragmas. The probes also flushed out and removed a broken typed-nil
  guard in the fixture map walk. Merged repo gate back at 100%.

## The remaining gap — Go (3,061 statements, per-file)

Concentrated in the compile pipeline and the paths only rich programs
drive: `emit.go` (~557), `carrier.go` (~355), `engine.go` (~300),
`lower.go` (~284), `core_helpers.go` (~190), `callable_words.go`
(~154), `vm.go` (~114), then a long tail (`fn_params`, `registry`,
`method_shape`, `unify_*`, `weak_flex`, `trace`, `engine_pool`,
`module_gate`, `word_extend`, `generics_*`, …).

Staged plan:

1. **Control-flow fixtures.** `specfix` has no `if` / `case` / `for`:
   the branch/loop recording machinery (`RecordBranch`, the
   loop-carried analysis, the join machinery) is unreachable
   standalone. Add minimal fixture control words that orchestrate the
   SAME eng mechanisms basic's handlers do (the mechanism calls are
   eng-exported; the fixtures are registrations, exactly the
   engspec pattern).
2. **Compiled program batteries.** Eng-local differential batteries
   (crafted fn/branch/loop/closure programs over the fixture
   vocabulary, compiled vs interpreted) targeting `emit`/`lower`/`vm`/
   `carrier` — the standalone twin of lang's bytecode clusters.
3. **Corpus growth (parity instrument).** Gaps expressible as rows go
   into `eng/spec` so BOTH engines earn them; every added row runs
   through the cross-engine differential by construction.
4. **Seam tails.** The remaining arms currently reached only by lang's
   seam suites get eng-local seam tests (same design/TEST-SEAMS.10.md
   conventions), or a pragma where the guard is provably unreachable.

## The remaining gap — TS (~14% of lines, per-file)

The engine core is strong (compile/error/signature/resolve/lower/
capability at 100%; match 99%, emit 98%, vm 95%, engine 94.5%). The
gap concentrates in `parser/convert.ts` (72.7%), `parser/grammar.ts`
(77.9%), `parser/errors.ts` (48.0%), `spec-fixture.ts` (72.9%),
`canon.ts` (71.8%), `value.ts` (89.5%), `parser/xml.ts` (83.5%).

Staged plan: a parser-focused unit campaign first (convert/grammar/
errors are the divergence-prone surface the cross-engine differential
can only catch when a row happens to trip it), then canon/value, then
the fixture harness; corpus growth from the Go side lifts here too.
Gate the branches/functions metrics once lines reach 100.

### Fixture-parity backlog (2026-08-05)

The TS fixture guard probes (`src/fixture-probe.test.ts` — the twin of
`eng/go/specfix_probe_test.go`) surfaced fixture behaviors that
diverged from the Go reference on paths no corpus row reaches.
ALIGNED (2026-08-05, probe rows moved into the shared table):

- `get`: the atom-key sigs now quote their key; every miss returns the
  None TYPE literal (was the `none` value on list misses and a
  dispatch failure on map keys).
- `do`: a raising list body surfaces as an Error VALUE carrying the
  bare detail — TS gained the error-value kind (`ErrorInfo` /
  `newErrorValue` in value.ts, the `error(<message>)` canon arm) the
  hatch needs.
- `refine Record`: the at-least-one-field and per-element pair guards
  now match the Go messages.

- the 1-arg bare `refine`: the base type passes through unchanged (the
  paired `def` mints the subtype); a non-type argument raises the Go
  message.

The probe-surfaced backlog is CLOSED (the object-with-parent
constructor path remains a fixture gap on both engines' corpora, not
a divergence). Where expressible, corpus rows should follow so the
differential guards the aligned behaviors permanently.

## Ratchet log

| Date | Go floor | TS floor | Note |
|---|---|---|---|
| 2026-08-04 | 86 | 85 | Gates introduced; standalone lanes + specfix landed. |
| 2026-08-05 | 87 | 85 | specfix guard probes + pragmas (fixtures at 100%); merged repo gate restored to 100%. |
| 2026-08-05 | 88 | 85 | Stage-4 wave 1: unit probes for the pure-function tail (SizeOf, table/JSON render, Disassemble, the unify families, fn-spec params/returns, RunPredicate, CallBoru, registry arms) — 316 statements. |
| 2026-08-05 | 89 | 85 | Stage-4 waves 2–4: fixture if/for (count + range) + doq closures (specfix/control.go), the eng-local battery (value, compiled, and check lanes; 78 rows, most VM-executed) — 542 statements total closed. |
| 2026-08-05 | 89 | 88 | TS wave 1: the parse-error translation campaign (errors.ts 48→100), the canon tails (71.8→98.5), and the parser breadth battery (convert.ts 72.7→82.2) — 85.68→88.53 lines. |
| 2026-08-05 | 89 | 90 | TS wave 2: the legacy hand-rolled tokenizer (dead since the jsonic parser became the single path; its value contract predates ADR-012 opacity) removed after an oracle experiment proved it diverges by design; fixture guard probes landed and the fixture-parity backlog recorded — 88.53→90.90 lines. |
| 2026-08-05 | 89 | 91 | TS wave 3: three fixture-parity alignments (get quoting + None-literal misses, do's error-value hatch with the new ErrorInfo value kind, refine Record guards) and 20 parse-battery rows (generics angle sugar, underscore/0d numerics, escape tails) — 90.90→91.38 lines. |
| 2026-08-05 | 89 | 92 | TS wave 4: the 1-arg bare refine port closes the fixture-parity backlog; 19 battery rows for map-shorthand modifier keys and the XML family (entities, error taxonomy, interpolation holes, comments) — 91.38→92.37 lines. |
| 2026-08-05 | 89 | 93 | TS waves 5–6: value.ts predicate-table probes; battery rows for arrow folds, angle composition, interpolation segments, unterminated containers, and the map-value dot-chain folds — 92.37→93.09 lines. |
