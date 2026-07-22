# AOT-COMPILE

Design for **ahead-of-time (AOT) bytecode compilation baked into the binary**:
at Go build time, compile the AQL-implemented core modules (`aql:sift`,
`aql:repl`, `aql:vault-tui`, and future AQL modules) to bytecode, store the
compiled form inside the `aql` binary, and load it instead of parse+interpret
at startup — so those modules run on the VM, not the tree-walking interpreter.
The **same mechanism generalizes to user AQL code built into a binary** via
`aql build`.

This is a **design RFC** — no implementation here. It builds on the landed
bytecode compiler and the runtime-stamping machinery
(`design/aql-bytecode-*.md`, `design/RUNTIME-STAMPING.0.md`,
`design/NET-COMPILE-FRONTIER.0.md`). The claims below were adversarially
verified against the code; the sharp edges that survived are called out inline.

> **Decisions proposed at design time** (the forks this RFC closes; a reviewer
> ratifies or reopens them):
> (1) the baked artifact is an **optional, self-invalidating cache**, never a
> correctness dependency — the `.aql` source stays embedded and any
> version-mismatch or unresolvable reference falls back to today's
> parse+interpret (§2);
> (2) the artifact is **binary-baked and version-tagged** — the compiler and
> the bytecode it produced normally ship in the same binary, driving the
> version-mismatch fallback to ~never; the tag is what *guarantees* safety, so
> this is a difference of *degree* from a persisted `.aqlc`, not of kind (§1);
> (3) the enabling change is **teaching fn-dispatch to prefer a compiled unit**
> — today *nothing* dispatches an AQL-bodied fn to its compiled ref under an
> interpreted top-level, not even callbacks; stamping and AOT merely *feed*
> that dispatch (§4, the load-bearing section);
> (4) the compiled form is the **existing detached stamper's output**, moved to
> build time and serialized — a per-root *multi-unit* `*Program` (root fn plus
> its co-compiled callee subtree), attached fail-safe (§3);
> (5) delivery is a **`go generate` tool emitting a committed, version-gated
> artifact** (the `genhelp` convention), embedded in `lang/go` so one bake
> serves both the native binary and the wasm build (§5);
> (6) **ship compiled units only inside binaries, never in source packs** — a
> registry `.aqlc` consumed by a different `aql` version reintroduces the
> staleness class and is deferred (§7).

---

## 1. The standing decision this revisits, and the honest version of the argument

The codebase deliberately decided **not** to persist compiled bytecode:
"bytecode is an execution mode, not a build artifact… eager compile-at-load,
**no build step, no persisted `.aqlc`**" (`design/aql-bytecode-plan.0.md:71-72`).
The bytecode report evaluated an `aql compile → .aqlc` step explicitly and chose
eager-at-load, warning: "`.aqlc` files baked across versions risk mismatches.
Either version-tag the file and recompile on mismatch, **or never persist**"
(`aql-bytecode-report.0.md:1338-1556`).

The hazard was a compiled artifact that **outlives or crosses the compiler
version that produced it**, silently miscompiling against a changed
opcode/`Program` schema. This RFC does **not** claim to make that class
impossible — it claims to make its mitigation *cheap and its trigger rare*:

- **Version-tag + fallback (the actual guarantee).** A format-version tag (a
  hash of the opcode + `Program` schema + compiler version) is checked at load;
  on mismatch — or on any per-unit reference that fails to re-resolve — the
  loader **ignores the artifact and falls back to parse+interpret**. The `.aql`
  source stays embedded. This is precisely the "version-tag and recompile on
  mismatch" mitigation the report named — the *same mechanism class* as a
  version-tagged `.aqlc`, not a categorically different one.
- **Binary-baking makes the trigger rare (the difference of degree).** Because
  the bytecode ships in the same binary as the compiler that produced it, the
  common case is always consistent and the fallback effectively never fires.
  For a user `aql build --native`, the guarantee holds only *because* the
  version gate catches a binary rebuilt against a different `lang/go` and
  silently drops the perf benefit — it is enforced by the gate, not by
  construction.

So the honest framing is: **"warm the JIT's per-fn compile cache at build time,
version-tag it, and bake it into the binary as a self-invalidating
accelerator."** The staleness *class* is the same one the report analyzed; what
changes is that the mitigation is free (no user build step) and the trigger
probability is ~zero. A reviewer ratifying this RFC is agreeing that
*version-tagged + binary-pinned + always-has-a-source-fallback* is a worthwhile
trade the earlier "never persist" stance declined mainly to avoid a build-step
DX cost that a `go generate` bake (§5) does not impose on users.

---

## 2. The performance model — and the reason nothing is fast today

**What runs today.** The AQL-implemented modules parse their embedded `.aql`
source once per process (`sync.Once` around `ParseFunc`, `sift.go:48`) and
install their `def … fn` bindings by interpreting the body
(`native.New(modReg).Run(tokens)`, `sift.go:95-99`). They then run on the
**tree-walking interpreter forever** — and, crucially, *there is no dispatch
path that would use a compiled unit even if one existed*:

- The interpreter's ordinary fn-dispatch (`execFnDefLiteral` `engine.go:4927+`,
  the sub-registry module-fn path `engine.go:5128-5227`, `CallAQL`
  `registry.go:1880`) contains **no** `CompiledRef`/`.Compiled` read
  (grep-confirmed). A normally-dispatched module word (`Sift.parse …`) runs its
  token body regardless of any attached unit.
- Even the callback seam does **not** help under an interpreted top-level.
  `InvokeCallback` runs the VM only when `invokeCompiledUnit` can host it —
  `r.canHostVM()` (true only when no interpreter run is in flight:
  `vmRunning==0 && interpRunDepth==0`, `registry.go:505-517`) **or** a non-nil
  `r.nestedRunner` (set only inside a VM run, `vm.go:341`). `RunInterp`
  increments `interpRunDepth` for the whole activation (`registry.go:440-450`),
  and the `aql:tui` driver fires `update`/`view` via `InvokeCallback` *mid-run*
  on that same registry (`tui_run.go:375,418`). So `canHostVM()==false`,
  `nestedRunner==nil` → `invokeCompiledUnit` returns `ran=false` → `CallAQL`,
  the interpreter (`invoke.go:88,102-117`).

**The consequence sets up the whole design:** `aql vault -i --aql` runs via
`RunInterp` (it never compiles), so even a stamped or baked `update`/`view` unit
would be ignored — the callback fires inside the interpreter activation. **The
foundational change is therefore not "produce compiled units" — it is "make
dispatch use them"** (§4). Stamping (Tier 1) and AOT-baking (Tier 2) only differ
in *how the unit is produced*; both are inert without the dispatch change *or*
without running the whole program under `RunCompiled` (which sets
`nestedRunner`, making callbacks host the VM).

**The tiers**, then, all presuppose the §4 dispatch change (or a VM top-level):

| Tier | Unit production | Startup compile | Startup parse | New machinery beyond §4 dispatch |
|---|---|---|---|---|
| **0 (today)** | none | — | paid | — (all interpreted) |
| **1 JIT-stamp** | compile per fn at load (`StampFnValue`) | paid per process | paid | arm stamping; stamp both exports and `modReg.Defs` bindings |
| **2 AOT-bake** *(the ask)* | serialize at build, attach at load | **amortized** | paid | the codec (§3.2) + `go generate` bake (§5) |
| **3 AOT + skip-parse** | as Tier 2 | amortized | **skipped** | + a per-fn binding manifest |

**Honest guidance.** The steady-state VM win (the fold, the parser, the hot
loops on the VM) comes from the **§4 dispatch change plus *any* unit source**;
Tier 1 gets there with zero serialization. AOT's *specific* added value over
Tier 1 is **amortizing compile cost across process starts** — for
cold-start-heavy workloads (a CLI invoked in a loop; the wasm playground paying
compile at page load), not a long-running server. Recommended path: **land the
§4 dispatch change + Tier 1 first** (small, and the foundation Tier 2 attaches
to), then **Tier 2** once cold-start compile cost is *measured* to justify the
codec.

---

## 3. What the compiled form is, and how it attaches

### 3.1 The unit: a co-compiled multi-unit `*Program`, attached fail-safe

`StampDetachedSig` compiles one fn body and attaches the result as a
`CompiledFnRef` on the signature's `AQLImpl.Compiled` (`stamp_runtime.go:35-171`,
`sigimpl.go:51-62`); `StampFnValue` loops every signature independently, and a
body that refuses **silently stays interpreted** — per-signature, fail-safe
(`stamp_runtime.go:231-266`).

**But a non-leaf fn does not compile to a "standalone one-unit" program.** The
VM's user-call opcodes dispatch to `&p.Fns[Arg]` — an index into *the same
Program's* `Fns` pool (`OpCallUser`/`OpTailCallUser` `vm.go:1727`, `OpCallUserPoly`
`vm.go:1710`) — and **never read a callee's `CompiledRef`**. Cross-fn compiled
execution is therefore **intra-`Program`**: when a stamped root fn calls another
module fn, that callee's unit is compiled *into the root's program* so
`RecordUserCall` can reference it (`engine.go:5179-5182`, via
`compileStoredFnUnit`/`compileClosureBody` `emit.go:1976-1991`). A root fn thus
bakes to a **multi-unit program = the root plus its whole compilable callee
subtree**. Two consequences the design must own:

- **Duplication.** A shared helper (`sift`'s normalize step, `vt-clamp`, …) is
  inlined into *every* root program that transitively calls it. The artifact is
  *not* "one unit per fn"; its size scales with the call graph's transitive
  closure, not its node count.
- **Entry points only.** A separately-baked `view` unit is reached on the VM
  only when `view` is entered **directly** (a callback, or §4-taught dispatch) —
  never *from* another baked unit (that path uses the caller's inlined copy). So
  the set of fns worth baking as roots is the **dispatch entry points** (exported
  words + callback targets like `update`/`view`), not every internal helper.

The `CompiledRef` consumers today are `InvokeCallback` (`invoke.go:66`), `spawn`
(which reads the ref and calls `RunUnit` directly, `native_process.go:218`),
`await` (`native_temporal_await.go:47`), and the stampers; `RunUnit`
(`vm.go:244`) is the *entry* that starts a run from `ref.Prog`, not a per-call
opcode. AOT reuses this attach point: set `AQLImpl.Compiled = CompiledFnRef{Prog,
Unit, Captures}` on each baked root sig.

**Freshness — the one place AOT is strictly weaker than JIT stamping.** A
`CompiledFnRef` guards against a dependency rebind via `depSnap`/`depNames` and
recompiles via `restamp` (`bytecode.go:687,744-747`, `stamp_runtime.go:190-194`).
A baked unit has no live compile context, so:
- `depSnap == nil` would run on the VM but *never* check dep freshness — unsound
  if a module dep is later rebound (the poisoning that protects it never ran).
- Reconstructing `depSnap` requires the serialized **`depNames`** (the dep-name
  set from `storedHandlerDeps`, `emit.go:2202-2219`) — so `depNames` must be in
  the artifact (§3.2). Even then, `restamp` is unavailable, so on the first dep
  rebind `depsFresh` fails → `jitRestamp` returns nil → permanent fall back to
  the interpreter for that ref (fail-safe, not unsafe).

For the three core modules this never triggers — they do not rebind their
module-level dependencies after load — so baked units use the reconstructed-
`depSnap` path and stay on the VM. The design must serialize `depNames` and
accept "permanent interpreter degrade on any post-load dep rebind" as the
documented limit; it must **not** ship `depSnap == nil`.

### 3.2 Serialization: pointer → symbolic-reference rewriting (the net-new work)

**No bytecode serialization exists today** (the only serializer, `aql build`'s
`buildrt.EncodePayload`, bakes *source text*, `cmd/go/internal/buildrt/buildrt.go:201-289`).
A `*Program` (`bytecode.go:975-1120`) is a flat, pooled structure — `Code []Instr`
of `{Op, Arg int32}` where every operand indexes a typed side-pool — but the
pools hold **live Go pointers**. The codec rewrites each to a stable symbolic
identity a fresh process re-resolves. The table below is the *whitelist*: a unit
is baked only if every field it uses is on it; **anything unrecognized → refuse
the unit** (it stays interpreted).

| Pool / field | Today | Baked as | Re-resolved at load by |
|---|---|---|---|
| `Instr`, indices, `MakeMaps`, `Interps`, `GlobalBinds`, `Debug` | plain data | verbatim | — |
| `Types []TypeRef{Name,ID}` | string ID | ID string | `Types.LookupByID` (`vm.go:1522`) — **FixedID only**; a minted (`dynamicIDBase`) ID → **refuse** |
| `Sigs []SigRef{Word,Sig*,Guard}` | `*Signature` ptr | `Word`+`Guard` | `Lookup(word)` against the **owning module's** registry (§3.3) |
| `PolyRef{Word,Arity,Reg}` | word + reg ptr | `Word`+`Arity` | re-resolve `Reg` to the owning module; re-match live (`bytecode.go:508-533`) |
| `UserPolyRef{Word,Arity,Reg,Impls,Sigs}` | word + reg + **live `Impl` identities** + stored-mode `Sigs` | — | **refuse** any unit with a user-poly call: `Impls` are pointer identities the VM verifies against the *live* table (always fail cross-process → permanent defer, `bytecode.go:566-593`), and stored-mode `Sigs` (a body-local overload table "a live name Lookup could never resolve") cannot be re-derived |
| `TypedBinds []TypedBindSpec{Def*, Cons*}` | `*Type` + `*Value` | type ID + serialized value pattern | `LookupByID`; **refuse** if `Cons` is a predicate-fn body or non-serializable value |
| `CompiledFn.Reg *Registry` | ptr | — | rebind to the fn's **owning** module registry (§3.3) |
| `CompiledFn.Returns/Params []*Type` | ptrs | type IDs | `LookupByID` |
| `CompiledFn.ParamPatterns []*Value` | value ptrs | serialized value patterns, else **refuse** | — |
| `Consts []Value` | values | scalar/list/map by whitelist, **recursively** rewriting each element's `Parent *Type`→ID and dropping process-minted `ID`s; nested-fn→unit ref; **refuse** on any unrecognized `Value.Data` payload kind (`DepScalarInfo`, `*StoreInstanceInfo`, `*TimeoutInfo`, `TimePayload`, `ExtensionPayload`, …) | — |
| `Fallbacks []FallbackSpan{Tokens}` | un-lowered interpreter-island tokens (arbitrary Values) | — | **refuse** any unit with a non-empty `FallbackSpan` (islands are, by definition, the shapes the compiler could not lower; round-tripping arbitrary tokens is out of scope) |
| `depNames` | dep-name set | verbatim | rebuild `depSnap` at load (§3.1) |
| `Debug []SrcPos` | source rows | **preserve verbatim** (coverage — §6) | — |
| `storedFnRefs`, `restamp`, mutexes | runtime-only | dropped | reconstructed fresh (`restamp` stays nil — §3.1) |

**Format:** a purpose-built **versioned binary codec**, not `gob` (gob ties the
wire format to Go layout, cannot encode the unexported `storedFnRefs`, and is
fragile against the `Program` struct's active churn). The codec is the single
authority for the wire schema; a hash of that schema is the format-version tag
(§1).

**Scope rule that keeps this tractable:** bake only units whose every reference
is whitelist-clean — FixedID types + word names, no user-poly calls, no
interpreter islands, no exotic const payloads. The three target modules mint
**zero** user types (verified: no capitalized `def`/`refine`/`behave` in
`sift.aql`/`vault_tui.aql`/`repl` preamble; params are only
`Integer/String/Map/List/Any/Boolean`, all FixedID-stable,
`fixedid_stability_test.go:32-151`). Units that hit any refuse-rule fall back to
the interpreter — the mechanism degrades per-root, never miscompiles.

### 3.3 Re-resolution spans the whole import closure

References do **not** all rebind to a single `modReg`. `sift.aql` imports
`aql:string-util`, `aql:array-util`, `aql:minilang` and calls into all three
(`sift.aql:20-22,76-79`); `vault_tui.aql` imports `aql:tui`/`aql:vault`/
`aql:math-util`/`aql:string-util` (`vault_tui.aql:25-28`). A co-compiled unit's
`SigRef`/`PolyRef.Reg`/`CompiledFn.Reg` therefore point at **several distinct
foreign sub-registries** (`bytecode.go:519-524,1052-1062`). The loader must
re-resolve **each reference against its owning module's live sub-registry across
the entry's full import closure**, which imposes a **load-ordering constraint**:
a dependent module's baked references can only be re-resolved after its imports
are themselves loaded (and, if an import is AQL-implemented like `aql:minilang`,
itself loaded — baked or interpreted). The loader walks imports first, exactly
as `import` resolution does today.

---

## 4. The load-bearing change: teach dispatch to prefer compiled units

Per §2, *no* dispatch path uses a fn's compiled ref under an interpreted
top-level — not ordinary dispatch, and not callbacks fired mid-`RunInterp`. So
the single change that unlocks every tier is: **make the fn-dispatch path prefer
`sig.CompiledRef()` when it can host the VM**, routing through
`invokeCompiledUnit`/`RunUnit` with the same `internal_error → CallAQL`
fail-safe and effect fence `InvokeCallback` already uses (`invoke.go:88-117`).
Two sub-parts:

- **Primary dispatch (`execFnDefLiteral`/`execFnDefSig`).** Consult the compiled
  ref so a normally-dispatched module word (`Sift.parse …`) runs its baked unit.
  This is the hottest path in the engine; it must carry the fail-safe and be
  proven byte-identical by the differential gate (§6) before it is default-on.
- **Callback hosting under an interpreted top-level.** Because `canHostVM()` is
  false during a `RunInterp` activation, a mid-run callback cannot host a VM
  today. Options: (a) allow a *re-entrant* VM host from within an interpreter
  run (lift the `interpRunActive` bar for a compiled-unit call, guarded by the
  same soundness fence) — the general fix; or (b) run TUI/long-lived apps under
  `RunCompiled` at the top level so `nestedRunner` is set and callbacks host the
  VM natively. (b) is a smaller change but only helps whole-program-compilable
  entry points; (a) is the general enabler and is the recommended target.

This section is the crux: **without it, both stamping and AOT produce units that
are never executed.** It is also independently the Tier-1 enabler. Its
interaction with the effect fence and the differential gate is the primary
review risk of the whole RFC.

---

## 5. Delivery: one codec, two carriers

**Core modules → `go generate` + committed artifact (the `genhelp` pattern).** A
standalone tool under `cmd/go/` (mirroring `cmd/go/genhelp`, which runs the
engine at generate time and writes a committed `gofmt`ed `*_gen.go` with a `DO
NOT EDIT` header) compiles each embedded core module's **entry-point fns** as
co-compiled roots (§3.1), serializes them, and writes a committed generated Go
file under `lang/go/modules/` embedding the artifact. Living in `lang/go`, **one
bake is linked by both the native `cmd/go` binary and the `wpg/wasm` build**. A
**staleness test** (the `make status`/`make spec-gen` discipline) re-bakes and
diffs against the committed artifact, failing CI on drift. The generator runs
host-side (not under the `js/wasm` tag), as `genhelp` does.

**User code → `aql build`.** `aql build` already bakes
`buildrt.Config{Source, Files, Compile-mode}` and compiles at runtime inside the
built binary (`buildrt.go:139-289`). Generalize by compiling the entry + import
closure's entry-point fns at build time and storing the serialized units in
`Config`; `buildrt.Main` attaches them via the same loader. Same codec, a
`[]byte` carrier instead of generated Go, so it works for both self-embed and
`--native`.

---

## 6. Correctness & verification

The one forbidden outcome is a baked unit that **miscompiles** — diverges from
the interpreter (`NET-COMPILE-FRONTIER.0.md:312-313`). Every existing guard
applies, plus new gates:

1. **Differential parity (reuse).** Extend `TestSpecCompiledDifferential`
   (byte-identical VM-vs-interpreter, floor `minCompiledRows=2265`,
   `compiled_differential_test.go:41`): for each baked root, assert
   `VM(baked) == interpreter(source)` in value, error, and output.
2. **Staleness gate (new).** Re-bake the core modules, diff against the
   committed artifact, fail on drift.
3. **Format-version gate (new).** Load-time schema-hash check; mismatch → ignore
   + fall back; tested with a deliberately-mismatched header.
4. **§4 dispatch gate.** The dispatch change (the highest-risk piece) is proven
   byte-identical by (1) before default-on, with the soundness-bailout fallback
   exercised.
5. **`make verify-bytecode`** (differential + whole-corpus + property fuzz +
   `-race` + `-tags aqldebug`) and **`make cover-gate`** (ADR-008 100% on all new
   Go — codec, loader, generator, dispatch change) must pass.

The fail-safe design means a codec bug or an unresolved reference degrades to
"unit refused, fn interpreted," never a wrong answer — the interpreter is the
oracle.

### Coverage-gate preservation

Coverage for `aql:sift`/`aql:vault-tui` (ADR-008) is **source-row based**
(`coverage.go:44-155`). The denominator is the parsed **registered source text**
(`RegisterCoverSource`, `sift.go:69`); the VM numerator is `noteVMCoverage(debug,
pc) → noteCoverage(debug[pc])` over `Program.Debug` `SrcPos` (`coverage.go:126-135`).
An AOT load is coverage-safe **iff** it (a) still embeds and registers the `.aql`
source (the denominator — even though it is no longer parsed for execution), (b)
preserves original `SrcPos.Row`s on every baked unit (§3.2), **and** (c) runs the
unit on a registry whose `coverID` is the module's — `noteVMCoverage` reads
`r.coverID` on the *running* registry, so a baked `CompiledFn.Reg` must be the
cover-tagged `modReg` for attribution to fire. Drop any of the three and the
tagged modules can never reach 100%. `aql:repl` sets no cover ID and is
unaffected.

---

## 7. Scope, phasing, and deferrals

**Phase 1 — the §4 dispatch change + Tier 1 JIT-stamp.** Land the dispatch
change (the enabler); arm stamping and `StampFnValue` the module fns in
`BuildXxxModule`, stamping **both** the export values and the `modReg.Defs`
bindings (internal bare-name calls dispatch through the latter;
`EnableRuntimeStamping` is per-registry and `StampFnValue` returns a clone,
`registry.go:527-534`, `stamp_runtime.go:222-267`). Immediate steady-state VM
win, no serialization. Gated by the differential harness.

**Phase 2 — Tier 2 AOT-bake for core modules.** The versioned codec (§3.2), the
`go generate` bake + committed artifact + staleness gate (§5), and the loader
that attaches baked roots in place of the JIT compile — keeping parse+install as
today (the safe MVP; it changes zero binding semantics, only overlays refs).
Gated by differential + staleness + format-version + coverage gates.

**Phase 3 — user `aql build` integration** (§5).

**Phase 4 (deferred).**
- **Tier 3 skip-parse:** bake a per-fn binding manifest (name, signatures with
  type IDs, captures — nil for these modules) so the loader reconstructs
  `FnDefInfo`s without parsing the body, skipping the startup parse. Requires
  faithfully reproducing `InstallFnDef` without the body run; lands after Tier 2.
- **User-poly / island / minted-type units:** the refuse-rules in §3.2 exclude
  these from baking. Widening the bakeable subset (a portable user-poly
  encoding; interpreter-island token round-tripping; minted-type re-mint/`Adopt`,
  `typetable.go:312-334`) is future work; none is needed for the core modules.
- **Wasm execution flip:** the playground runs `RunInterp` (`wpg/wasm/main.go:46`);
  consuming baked units there is a separate change this bake enables.

**Explicitly declined: compiled units in source packs / the registry.** Shipping
bytecode in an `aql pack` zip consumed by a *different* `aql` version reintroduces
the cross-version staleness class (§1). `pack` stays source-only; compiled units
live only inside version-pinned binaries. A registry that ever ships bytecode must
carry the report's full version-header + recompile-on-mismatch machinery
(`aql-bytecode-report.0.md:1542-1556`) — out of scope.

---

## 8. Risks

- **The §4 dispatch change touches the hottest path.** It must carry the
  `InvokeCallback` fail-safe + effect fence and be proven byte-identical before
  default-on. This is the single highest-risk element (§4, §6 gate 4).
- **Co-compilation duplication.** Baking roots inlines shared callees into each
  root; artifact size scales with the transitive call closure. Mitigation: bake
  only dispatch entry points as roots; measure artifact size before committing to
  the set.
- **`Program` struct churn vs. codec maintenance.** The representation is under
  active development. Mitigation: the artifact is a self-invalidating,
  version-tagged cache (§1) + the staleness gate — churn costs regeneration,
  never correctness.
- **Miscompile (the forbidden outcome).** Mitigation: the differential gate + a
  whitelist codec that refuses on any unrecognized field/payload + fail-safe
  fallback at every unresolved reference and VM soundness bailout.
- **Freshness degrade.** A baked unit permanently interprets after any post-load
  dep rebind (§3.1); documented limit, benign for the core modules, must be
  tested (a rebind forces the fallback, still byte-identical).
- **Coverage regression.** Mitigation: keep source registration, preserve
  `SrcPos`, run units on the cover-tagged registry (§6); pinned by cover-gate.
- **Benefit smaller than the complexity.** Tier 1 (+ the §4 change) delivers the
  steady-state win cheaply; Tier 2 is gated on a *measured* cold-start compile
  cost. If compile-at-load proves negligible for these modules, stop at Tier 1.
