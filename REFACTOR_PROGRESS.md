# PROGRESS — Function-model consolidation refactor

Branch: `claude/bloom-filter-dx-issues-0Gu1V`
Plan (APPROVED): `/root/.claude/plans/for-if-remove-the-moonlit-gadget.md`

## STAGE 0 — equivalence harness — COMPLETE (committed + pushed)

All gates green: eng/go and lang/go both `go vet ./...`=0 and
`go test ./...`=0.

Two golden harnesses (regenerate with the noted flags ONLY after an
intentional change; otherwise they must stay byte-identical):
- `lang/go/native/fnmodel_equivalence_test.go` (+ testdata golden).
  TestFnModelEquivalence. Flag `-update`. Dumps (1) every word's sorted
  compiled `Signatures` (arity/type-paths/barrier/noeval/quote/typearg/
  fullstack/fallback/runcheck/returns/+returnsfn) via `r.Defs.Names()` +
  `r.Lookup(name).Signatures`, and (2) a behavior corpus (native fwd/
  stack/swap, `if` code-bodies, named fns, recursion, multi-overload,
  afn, closures, each/fold, stack words). 0 errors in corpus.
- `lang/go/modules/fnmodel_wrapper_equivalence_test.go` (+ testdata
  golden). TestFnModelWrapperEquivalence. Flag `-update-wrapper`. Pins
  the MODULE-WRAPPER dispatch path (execFnDefLiteral captured-sub-
  registry / CallAQL branch) via mathRegistry + `arg ( math get NAME )`.
  abs=5, sign=-1, negate=-4, ceil=3, floor=2.

## KEY ARCHITECTURAL FINDING (sharpens the plan)
The named AQL fn path ALREADY compiles to a Go handler: `InstallFnDef`
(core_helpers.go:228-458) lowers `def f fn […]` into `RegisterNativeFunc`
with a body-splicing handler closure + a check-mode `returnsFn`. So named
fns already dispatch via `execMatch`. The REMAINING duality is concentrated
in **`execFnDefLiteral`** (engine.go:1800-1989) — Function-VALUE-on-stack
dispatch (afn/`=>`, captured FnDefs, module wrappers) — which builds
handler-less `Signatures` via `fnSigsToSignatures` (engine.go:2204) and
falls back to `execFnDefSigStackMatch` (2054) / `execFnDefSig` (2243) /
`CallAQL` (registry.go:715). THIS is the real target of the Stage-1 spike.

## NEXT: STAGE 1 — compileFnDef + spike
Add `compileFnDef`: attach a Handler to EVERY sig (native passthrough;
AQL closure over the body-runner), resolve BarrierPos, sort. Route
InstallFnDef / upsertFnDef / execFnDefLiteral's afn-lazy path
(fnSigsToSignatures at engine.go:1827-1835) through it. SPIKE: prove
CallAQL-style execution is complete for the execFnDefLiteral cases
(forward collection, captures, returns, def-cleanup, recursion). Fallback:
Handler-sentinel that splices inline — one localized branch in execMatch.
Convert trivial-delegation (engine.go:1958-1976) to a compile-time handler.
After Stage 1, every Signature.Handler is non-nil.

## REMAINING STAGES (from approved plan)
- Stage 2: delete Handler==nil fallbacks + module-closure split
  (engine.go:1882-1883,1923-1924,1947-1977) → dispatch = match → execMatch.
- Stage 3: merge structs. matchSignature/CompareSignatures/sigSlotValue read
  `FnSig.Params[i].Type`; fold Signature-only fields onto FnSig; retire
  `Signature`; delete `FnDefInfo.Signatures` + `fnSigsToSignatures`. Repoint
  readers: canon.go:140, fnsig.go:129-150, registry.go:942-998,
  core_helpers.go:41/232/472-479/840, engine.go:1827/2057, unify.go:120-123,
  modules/type.go:244-248, native_inspect.go:91/206, native_behave.go:132-135,
  native_definition.go:516-518, native_definition_fn.go:23-24.
- Stage 4: NativeFunc/NativeSig become thin shim lowering into FnSig via
  compileFnDef. 348 NativeSig literals UNCHANGED (out of scope).
  RegisterNativeFunc (registry.go:676) already does field-by-field
  NativeSig→Signature copy.
- Stage 5: remove dead bridges (execFnDefSig, execFnDefSigStackMatch,
  fnDefSigByParamCount, lazy compile). Update eng/go/CLAUDE.md (Signature
  Ordering, trivial-delegation, "two representations"). Add
  lang/doc/design/FUNCTION-MODEL.0.md. Fix USURP.0.md FnDefInfo note.

## INVARIANTS (do not regress)
- Signature ordering: top-first sig order; matchSignature sole source of truth.
- One BarrierPos sentinel: FnSig uses -1 (unset→resolve once at install);
  Signature uses post-resolved (0=no barrier). Pick ONE, resolve at the
  single install boundary AFTER handler-attach (barrier tiebreak in
  CompareSignatures reads resolved BarrierPos).
- Sealed payload: FnDefInfo keeps marker; Handler/Body inside FnSig (never a
  Value) need no marker.
- Check-mode return inference is the one irreducible asymmetry: analyze AQL
  Body (AnalyseFnBody) vs native Returns/ReturnsFn — mode-local read.
- Gate EACH stage: eng/go + lang/go `go vet ./...` + `go test ./...`, plus
  both equivalence harnesses, plus fixedid_stability/data_nil_gate/value_mode.

## HARNESS/REGISTRY FACTS (confirmed)
- Multi-module repo, NO go.work. `cd <mod> && go test ./...`. Modules:
  eng/go, lang/go, cmd/go, calc/go, wpg, test/go, test/solardemo.
  Pre-commit from root: `make fmt && make vet && make lint && make test`.
- Run a program in a test: `r,_ := native.DefaultRegistry(); toks,_ :=
  parser.Parse(src); out,_ := native.NewTop(r).Run(toks)` (native pkg)
  or `native.New(r).Run(toks)` (modules pkg — eng/go/engine is NOT a
  dependency there). Parser import: `github.com/aql-lang/aql/eng/go/parser`.
- Enumerate words: `r.Defs.Names()`. Lookup: `r.Lookup(name) *FnDefInfo`.
- Module wrappers need `modules.InstallResolver(r)` OR build via
  BuildMathModule(r) + push exports as defs (see modules mathRegistry).
- `Signature.TotalArgs()` = len(Args). `Type.Path()` = stable type path.

## CRITICAL ENV GOTCHA
The tool RESULT/display channel intermittently corrupts file-content reads
and stdout (padding with repeated garbage, truncating multi-line output,
even garbling a Write confirmation). The ON-DISK files are correct —
proven by go compiler/vet, go test exit codes, grep -q, git status.
RULES:
1. NEVER trust displayed file contents/stdout for correctness.
2. Verify via EXIT CODES: `cmd >/dev/null 2>&1; echo "X=$?"`, `grep -q`.
3. ONE tool call per message — parallel batches cascade-cancel on the
   first error and waste the whole batch (this bit me repeatedly).
4. If a Write/Edit looks corrupted on readback, rewrite via Bash heredoc
   (`cat > file <<'EOF'`); confirm with `gofmt -l` (empty=ok) + `go vet`.
5. NEVER `git add -A` right after a cancelled batch — a scratch probe
   (zzprobe_test.go) got committed that way and broke the build. Check
   `git status --porcelain` first; remove zz* probes before adding.
6. cwd persists across Bash calls; `cd /home/user/aql && …` for root ops.
