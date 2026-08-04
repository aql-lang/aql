# Lang/Eng Content Audit — and the Types-Module Proposal

> **Status:** Working design note. Records the 2026-08-03 full audit of
> `lang/go` content that is structurally load-bearing for the `eng/go`
> kernel, the maintainer's boundary principle, and the proposal that
> answers it. This document was the source material for **ADR-012**
> ("The kernel is mechanism"), recorded 2026-08-03 on maintainer
> approval — it discharges ADR candidate 2 of
> `design/NUR-RESOLUTION-PLAN.0.md`.
>
> Surfaced NURs: [NUR057](../NUR.md#nur057), [NUR058](../NUR.md#nur058);
> NUR059 and NUR060 were surfaced by §6 and RESOLVED by the stage-6
> implementation (records deleted per the register's resolved-record
> rule; the numbers are retired).

## 0. The boundary principle (maintainer direction, 2026-08-03)

> **eng is intended for language engine mechanics, not language
> specifics like actual types or words. This means eng can be used to
> write other languages, and it should contain all language
> mechanisms.**

This is the sharpened form of a principle the repo already half-holds:

- **No words live in eng.** Already true and enforced by construction:
  `eng/go` registers zero words, and `calc/go` (which links eng alone)
  is the executable proof (`calc/doc/explanation.md` §"The eng/lang
  split"; `lang/go/native/register.go:1-16`).
- **The kernel is purely algorithmic.** The precedent commit `4ccaee1`
  moved `ResolveColor` *out* of eng on exactly this ground.
- **"The kernel no longer mentions any user-facing domain type
  identity."** — `design/TYPE-DECOUPLING.10.md`, which moved the Time,
  Matrix, Fetch, and Timer families out of eng in 2026.

The principle **retargets NUR009's verdict direction**. NUR009
(2026-07-31) proposed "all globally visible descendants of `Node` or
`Scalar` belong in eng", with `Bytes`/`Time`/`Date`/`DateTime`/
`Instant` as likely migrations. The *ownership problem* it identified
is real — a global scalar the kernel cannot see is denied capabilities
the kernel grants by enumeration (`canonicalBaseType`,
`eng/go/depscalar.go:157-175`). But under the mechanics principle the
fix is not to move domain types into the kernel; it is to remove the
kernel's **enumerations** so that any registered type, wherever it
lives, can opt in through a mechanism (§5, stage 4). NUR009 stays
Pending; its remediation is re-routed through this document.

### 0.1 The mechanism test

A piece of content belongs in eng **iff the engine machinery itself
constructs it or branches on it** — independent of which language is
running on the engine:

1. Values the parser bridge or step loop create/branch on structurally
   (the existing rules 1-2 of eng/go/CLAUDE.md §"Where a Type Lives").
2. Meta- and structural types of the dispatch/construction machinery
   (rules 3-4: `Type`, `Function`, `Record`, `Class`, …).
3. Analysis, compile, and dispatch *algorithms* — including ones
   currently authored inside lang word handlers (§2).

Everything a *language* decides — surface word names, domain types,
their formats/validators/codecs — is content and lives above eng.
Two corollaries:

- **Word names never appear in eng.** Today the kernel hard-codes ~24
  lang-registered names (§3). The mechanism replacement is a *role*
  table (§5, stage 5): lang binds names to kernel roles at
  registration; eng branches on roles.
- **The test cuts both ways.** eng currently holds content-flavored
  material that fails it: `eng/go/micron.go` (Emailon/Urlon/Semveron/
  E164 validation vocabulary), `eng/go/iso4217.go` (a currency table),
  `eng/go/core_xml.go`. These are candidates for the *reverse* move
  (eng → types module) and are recorded in §7 as a follow-up audit,
  out of scope here.

## 1. Audit method

Three sweeps over the non-test tree, 2026-08-03:

1. every eng site that special-cases a name/type registered in lang;
2. every lang site implementing kernel-level machinery;
3. every documented statement about the boundary (design/, NUR.md,
   ADR.md, kg).

Sweep 3 confirmed **no prior audit of this question exists** — the
nearest artifacts are `design/TYPE-DECOUPLING-INVENTORY.10.md` (the
mirror direction) and NUR009's deferred "review individually".

## 2. Findings A — kernel machinery implemented inside lang

These violate the stated rule of eng/go/CLAUDE.md ("a language-wide
concern belongs here, not duplicated in lang") regardless of where any
type lives. Under the mechanism test they move to eng **as
algorithms**; the lang word registrations that invoke them stay put.

| # | What | Where (lang) | Kernel counterpart / defect |
|---|------|--------------|------------------------------|
| A1 | Typed-def semantics exist twice | `native/native_definition.go:908-1204` (`defTypedHandler`: predicate / refine-bare / DepScalar / FnUndef branches) | `eng/go/typed_bind.go::RunTypedBind` re-implements the same branches for the VM; its doc comment declares itself a mirror — "the SAME helpers … the SAME error strings, in the SAME order", down to bug-compatibility with lang's bare `fmt.Errorf`. Should be ONE eng entry point called by both engines. |
| A2 | Control-flow check/compile analysis | `native/native_control.go:578-915` (`if`, ~280 lines), `:1071-1229` (`for`, incl. a private residual cap), `:249-368`/`:1287-1351` (`do`/`error`), `conditional.go:210-400` (`case` desugars to a nested-if chain for the compiler), `case_exhaustive.go` (940-line lattice-coverage pass) | eng owns the recorder/lowerer these feed (`emit.go`, `lower.go`, `OpForSetup`/`OpFlowBreak`/…). The lowering *policy* is kernel mechanism. `design/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md:557-561` already instructs moving the `native_control.go:324-398` fallibility scan into eng. |
| A3 | A second, wrong signature matcher | `native/native_definition_fn.go:18-56` (`MatchFnSig`) | Matches by `args[j].Parent.ConformsTo(p.Type)` — the exact "boundary that asks a different question" eng/go/CLAUDE.md §"Refine matching" forbids (rule: symmetric `v.Is(t)`); ignores ascription, gradual carriers, the not-disjoint rule. It is the dispatch path for every host→boru callback (net_socket, parse, model, tui, codec, walk_core). eng should export the real matcher. |
| A4 | Cross-registry lattice adoption | `native/native_module_module.go:699-726` (`adoptEscapedTypes`) | Type-registration policy across registries — squarely the "language-wide concern" clause. |
| A5 | Duplicated/worked-around lattice walks | `modules/type.go:625-646` (LCA re-walk by ID — comment records the eng pointer-identity defect it works around), `:604-605`; `native/native_type_gen.go:132-134,150-152` (verbatim copy of `eng.PushGenBindings`) | Fix `eng.CommonAncestorType` (canonicalize), delete the copies. |
| A6 | Interpreter⇄VM semantic mirrors | `native/forloop.go:83-125` (`parseRange`) vs `eng/go/vm.go:2035-2048` (`opForSetup`); the `error`-handler input-strip probe (`native_control.go:1301,1384`); Mark/Move continuation construction for `if`/`for` | Each pair is a correctness contract synced by comment. Hoist the shared decoders into eng. |
| A7 | Helper-API breaches (see also NUR058) | ~19 raw `.Data != nil` probes (`conditional.go:7,28` — `isCodeBody` omits the carrier test on the `if`/`case` hot path; `native_control.go:498,509,533`; `native_type.go` ×6; …); direct `CheckState` field writes (`CaughtBodyDepth`, `CondBodyDepth`, `SuppressedRuntimeError`, `DefsUsed`); `behave` installing a Behavior without `CanonicalType` (`native_behave.go:176-202` — the precise failure eng/go/CLAUDE.md §"Canonical `*Type` Pointers" warns about, at the word it names); bare `r.Defs.Push`/`Pop` call frames in `native_behave.go` (no args-stack/baseline entries) | Extend the `data_nil_gate` to lang and to the `!= nil` form; add scoped-depth helpers (`EnterCaughtBody() func()`); route Behavior-body calls through the frame machinery. |

Clean under the same lens (verified, for calibration): `native/
native_compare.go` and `native/native_make.go` are model citizens —
pure sig tables over eng handlers; `aliases.go` is a logic-free
re-export shim; truthiness, equality, canon have no lang duplicates.

## 3. Findings B — word names the kernel hard-codes

eng registers no words, yet it *knows the names* of ~24 lang
registrations, and in several cases emits them as tokens. Ranked by
load:

1. **`__pa`** — the kernel EMITS the token into every fn-frame tail
   (`eng/go/fn_frame.go:193`) and three kernel scanners re-parse it;
   `engine.go:6190` even uses `Lookup("__pa") == nil` as the
   foreign-registry probe. Registered at
   `native/native_definition.go:243`. Pure kernel machinery.
2. **`dot`/`dotr`/`get`/`getr`** — `lowerReach` emits `dot`/`dotr` for
   all `.`/`!.` syntax (`engine.go:4773-4779`); `get_words.go` exists
   solely to classify the family, with a duplicate copy at
   `emit.go:5678`; five compile folds consume it.
   (`design/REACH.10.md:40` already declined moving the *handlers*;
   the emitted names and duplicate classifier remain kernel debts.)
3. **Parser-emitted names** — `usurp`/`stack-args`/`forward-args`/
   `force-arity` (the `/u /s /f /N` desugarings,
   `parser/parse.go:568-574`), `afn` (`=>`), `gen`/`of` (generics
   sugar), `mini`. The grammar is meaningless without these lang
   registrations.
4. **`def`/`make`/`word`** — already admitted kernel identities via
   `sealedWords` (`word_extend.go:38-42`); `def` additionally drives
   `bindsReferent`, macro hygiene (`macro_expand.go:218`), frame-state
   analysis, and parser sugar.
5. **`args`** — the kernel SYNTHESIZES `( args N dot )` token runs for
   defaulted-parameter overloads (`core_helpers.go:2549-2556`).
6. **`break`/`continue`/`for`/`apply`/`if`** — dedicated VM opcodes
   (`OpFlowBreak`/`OpFlowContinue`/`OpForSetup`/`OpCallDynApplyTop`)
   and kernel continuation types (`IfCont`, `ForCont`) host the
   semantics; lang registers the names.
7. **Analysis tables keyed on lang inventories** — `frameStateWords`
   (`fn_capture.go:18-26`), the `each`/`fold`/`scan`/`filter` callback
   ABI (`callable_words.go:648-676`, incl. a by-string lookup of the
   lang-registered `Node/Map/KeyVal`), the eleven Forth shuffle words
   (`emit.go:4624-4630`), narrowing keyed on `is`/`or`/`nor`/`xor`/
   `xnor`/`not` (`carrier.go:3900-3917` — `nor`/`xnor` live only in
   `boru:logic-util`), `import`/`module`/`unpack` literal windows
   (`carrier.go:546-601`), `flex` (`callable_words.go:799-808`),
   `print` as the sole hardcoded effect predicate (`engine.go:5157`).

None of these words can *move* (no-words-in-eng); the debt is the
kernel's name-knowledge, addressed by the role table in §5 stage 5.

## 4. Findings C — global types registered in lang

Types registered in lang with `eng.OwnerKernel` — the code already
declares them kernel-owned while they reside in lang:

| Type (FixedID) | Registered at | Kernel coupling today | Disposition under §0.1 |
|---|---|---|---|
| `Ideal/Module` (5000) | `native/native_module_types.go:47-49` | eng/go/CLAUDE.md rule 2 *already lists* `Module` as interpreter-structural, but no `TModule` exists; `carrier.go:1163-1170` identifies it by string path to fold module constants; `ModuleDesc` (its payload) is an eng payload already | **→ eng.** Modules/namespaces are engine mechanics; the kernel owns the gate, export growth, and descriptor payload. Declaring `TModule` (FixedID preserved) deletes the string-path probe and closes the documented rule-vs-reality gap. |
| `Node/Map/KeyVal` (5002) | `native/native_keyval.go:32` | `callable_words.go:702` looks it up by string to type compiled `each`/`filter` callbacks | **→ eng.** The compiled-callback pair shape is dispatch mechanics. |
| `Scalar/Bytes` (1009) | `native/native_bytes.go:23` | `gobridge.go:92-103` (`RegisterBytesBridge` exists only because "the Bytes type is owned by the lang layer"); `value.go:959` (`BinaryLayout`); the NUR009 `canonicalBaseType` omission | **→ types module** (§5). The bridge is already the correct mechanism shape and stays; the refinement-base omission is fixed by stage 4's capability, not by residence. |
| `Scalar/Time` family (1000-1999) | `native/native_temporal.go` (global root; children module-owned under `boru:time-util`) | family-Comparer doctrine in `compare.go:227-253` names them | **→ types module.** NUR009 listed them for eng; TYPE-DECOUPLING deliberately moved them out of eng. The principle settles the conflict: they are content, not mechanism. |
| `Ideal/Patrun` (5004), `Ideal/Pid` (5007), `Ideal/Service` (5008) | `native_patrun.go:41`, `native_process.go:41`, `native_service.go:47` | none structural | **→ types module** (weak-coupling tier; "review individually" per NUR009 — none shows kernel branching). |
| Fetch family (3000-3999), Timeout/Interval (4000-4999), Matrix/Tensor (2000-2999) | `native/fetch.go`, `native_misc.go`, `modules/matrix.go` | none structural | **→ types module** for Fetch/Timers; Matrix is module-delivered — decide with §7 open question 2. |

## 5. The types-module proposal (the question, answered)

**Question:** given that eng is for mechanics only, should all type
definitions and special logic live in a types module?

**Answer: yes for language-content types and their type-local logic;
no for mechanism types and for analysis machinery.** Three-way rule:

1. **eng** keeps (and, per §4, gains `Module` and `KeyVal`) only
   *mechanism* types — those the engine constructs or branches on.
2. **A new `types/go/` component** (Go module
   `github.com/boru-lang/boru/types/go`; imports eng; imported by
   lang) owns every other **global** type: registration (`RegisterType`
   with today's FixedIDs — paths and IDs unchanged, so the wire format
   and `fixedid_stability_test.go` are unaffected), Behaviors,
   Comparers, capability implementations, codecs and bridges (the
   Bytes codec, temporal parsing/formatting). This ends the current
   scatter across `native_bytes.go` / `native_temporal.go` /
   `native_keyval.go` / `fetch.go` / `native_misc.go`.
3. **`boru:*` modules** keep *module-scoped* types (the NUR009
   carve-out, e.g. `boru:io`'s `StreamKind`), unchanged.

What "special logic" means, split three ways — this is the crux:

- **Type-local logic** (Behavior: match/format/equal; Comparer; Sizer;
  converters; codecs) travels WITH the type into `types/go`.
- **Analysis/compile/dispatch logic** (§2: typed-def binding,
  control-flow lowering, sig matching, lattice adoption) is mechanism
  and moves to **eng**, whatever types it touches.
- **Word handlers** (the registrations and their glue) stay in
  **lang** (or their `boru:*` module), calling eng mechanisms and
  `types/go` values.

Resulting layering (calc unchanged; a future language imports eng and
optionally types):

```
eng/go   ← types/go  ← lang/go  ← cmd/go
(mechanics)  (boru's       (boru's      (CLI)
              global        words,
              types)        modules)
```

Options considered and rejected:

- **A `lang/go/types` package** — cheaper (no new go.mod), but any
  other language wanting boru's scalars would drag lang's whole module
  (sqlite, voxgig-struct, …). The component pattern
  (`<component>/go/`) is the repo's established shape.
- **Pushing global types into `boru:*` modules** — module-owned types
  are load-scoped; making `Bytes` availability depend on an import is
  a language-visible semantic change, and refinement bases /
  unqualified sig usage need global registration.

**Moving definitions is necessary but not sufficient.** Each kernel
coupling in §3/§4 must become a mechanism or the problem just changes
address. Staged:

- **Stage 0 — ungated defect fixes** (safe under every version of the
  boundary): NUR057 (seal `set`/`del` or key the exemptions on binding
  identity, the `flex` pointer-identity precedent), NUR058 (stamp the
  lang mirror diagnostics), delete the stale `"eval"`
  (`fn_capture.go:21`) and `"return"` (`carrier.go:1930,2055`;
  `callable_words.go:856`) entries, `behave` canonicalization, extend
  the data-nil gate to lang and the `!= nil` form, fix `isCodeBody`'s
  missing carrier test, decide `null`'s absence from
  `reservedLiterals`.
- **Stage 1 — mechanism consolidation into eng** (§2): one typed-def
  entry point; export the real matcher and delete `MatchFnSig`; hoist
  `parseRange`/strip-probe; dedupe `PushGenBindings`; move
  `adoptEscapedTypes`; fix `CommonAncestorType`.
- **Stage 2 — mechanism types into eng**: `TModule` (5000), `TKeyVal`
  (5002); delete the string-path probes.
- **Stage 3 — create `types/go`** and move the §4 content types.
  FixedIDs/paths unchanged; lang re-exports via `aliases.go` so
  downstream code is untouched.
- **Stage 4 — capability over enumeration**: refinement-base
  participation becomes an opt-in (a Behavior capability or a
  registration flag) consulted by `canonicalBaseType`; Bytes opts in.
  **This closes NUR009** without putting Bytes in the kernel — and
  composes with NUR056's construction-capability direction.
- **Stage 5 — the word-role table** (largest, last): a
  `WellKnownWords` role map on the Registry (binder, unbinder,
  frame-tail-pop, accessor×4, splice, apply, break/continue, …) bound
  by lang at registration; eng branches on roles; the parser takes its
  emitted-name table at construction. `__pa` stops being a word
  entirely and becomes a structural marker (the `DefCleanup`
  precedent). The §2-A2 analysis moves keyed on roles, which also
  fixes the `nor`/`xnor` module-load assumption and the `print`
  effect predicate (route through the effects machinery instead of a
  name).
- **Stage 6 — parser type-name opacity** (§6): the parser stops
  resolving capitalised names; one canonical engine resolver takes
  over everywhere. Independent of stages 2-5 and can run earlier;
  listed last only because it needs its own spec re-pinning round.

Stages 0-1 need no ADR. Stages 2-6 change residence/ownership or
pinned language semantics and should land under the ADR this document
feeds (candidate 2), on maintainer instruction.

## 6. Parser type-name opacity (maintainer direction, 2026-08-03)

> **The parser must have no dependency on eng's type inventory:
> capitalised type names are returned by the parser as opaque tokens,
> exactly as word names are, and the engine resolves them late.**

Today the parser resolves capitalised names in ONE production site —
`parseWord` (`eng/go/parser/parse.go:1444-1601`), reached from both
word context and data context — via a package-level alias of the
builtin name table (`parse.go:17`) plus `eng.ResolveTypePath` for
slash paths (`parse.go:1555-1560`). (A second site,
`resolveTextValue` `parse.go:1249`, is production-dead — test-only
callers.)

### 6.1 The directive's model already half-operates — by accident

`eng.refreshTypeNames` (`eng/go/types.go:128-130`, called from
`RegisterType`) REBINDS eng's map var; the parser's `var typeNames =
eng.TypeNameTable()` captured the map at parser package init and goes
stale. So every type registered after parser init — `Bytes`, the Time
family, `Timeout`/`Interval`, Matrix, plugins — already parses as a
plain Word and resolves late in the engine. Verified live:

```
boru do 'quote [Date Integer Bytes]'   →  [word(Date) Integer word(Bytes)]
boru do 'typeof Date'                  →  Time
```

Three resolution regimes coexist: init-frozen kernel builtins
(parse-time literal), late-registered globals (Word, engine-late),
user types (Word, engine-late). Parse output depends on package-init
order — and the `refreshTypeNames` doc comment ("so freshly-installed
types are immediately resolvable … in the parser") is false in
production. Recorded as **NUR059**. The directive ELIMINATES the
split rather than introducing late resolution.

### 6.2 The engine already owns the late cascade

`stepWord` resolves a capitalised Word by priority: registry type
binding (`TopTypeBody`, `engine.go:2771-2791` — how user `def Foo`
types work), then live builtin table + type path
(`engine.go:2927-2934`). The plan-time forward scan mirrors it
(`engine.go:8361-8419`), as do the compile emitter
(`emit.go:4856-4880`, "mirror stepWord's type-name cascade exactly"),
guards (`guard_predicate.go:109-131`), and fn-sig parsing
(`fn_params.go` is fully name-based: `ResolveSigType`,
`lookupTypeNameInRegistry`). Auto-evaluated lists and map values run
sub-engines through `stepWord`, so evaluated data positions resolve
too. Consequently these contexts need NO new logic: top-level words,
evaluated list elements, map values, fn-sig params and returns, case
arms, `is`/`typeof` operands.

### 6.3 The genuine gaps (what stage 6 must build)

The sites with no late pass, or with divergent partial cascades —
each is a per-site re-implementation of the same lookup, which is
itself the non-uniformity:

| Gap | Today | Fix |
|---|---|---|
| `def x:Integer 5` typed-name annotation | the `{x:T}` map is `NoEvalMapArgs`-raw and `ResolveTypedNameValue` (`registry.go:2379-2389`) is registry-ONLY → `def_error` for builtins-as-Words (user types already work) | canonical resolver (below) |
| Typed-container children `[:T]` / `{:T}` | `ChildTypeInfo.Child` has NO general resolver — and this is ALREADY BROKEN for user types: `[x] is [:Foo]` → silent false, `def xs:[:Foo]` errors; only parser-eager builtins, gen placeholders, and predicate types work (**NUR060**, unpinned by any spec row) | resolve `Child` at consumption via the canonical resolver — fixes user types too |
| `{a?:T}` optional fields | parser builds the `(T tor None tor Absent)` disjunct and runs `SimplifyDisjunctAlts` AT PARSE TIME (`parse.go:909-918`) — parse-time lattice reasoning; a Word alt never resolves (`ResolveWordsDeep` has no Disjunct descent) | move construction + simplification engine-side |
| `quote Integer` / Atom-`/q` capture | the forward scan claims a Word for a `/q` slot BEFORE the type fallback (`engine.go:8306-8319` vs `:8402`), so `quote Integer` → Atom and `inspect Integer` degrades (`inspectAtomHandler` is registry-only) | `/q` consumers of type names gain the builtin arm (the `native_process.go:491-508` template) |
| `ResolveWordValue` (unify prepass, `core_helpers.go:2294-2311`) | builtin-table ONLY — no registry arm, no path arm; becomes the busiest resolver post-change | route through the canonical resolver |

**The canonical resolver** is the stage's actual deliverable: ONE eng
function — Defs/`TopTypeBody` (user & shadowing) → live builtin table
→ type path — that `stepWord`, the plan scan, `ResolveWordValue`,
`ResolveTypedNameValue`, `ResolveFieldType`, child-type resolution,
and every `/q` consumer route through. This is the type-name analog
of stage 5's word-role table: the engine resolves; the parser is
name-blind. `InstallType`'s existing conflicts-with-existing-name
guard keeps builtins non-shadowable (and `def Integer 5` starts
failing with the PROPER error instead of a `signature_error`).

### 6.4 The semantic decision to make explicitly

Quoted containers are inert data — never auto-evaluated — so after
the change `quote [String Float]` holds `[word(String) word(Float)]`,
exactly as `quote [Foo Date]` already does today. The
"retain their meaning inside quotations" guarantee
(`parse.go:1553-1556`) becomes consumption-time rather than
parse-time: the meaning survives wherever the quotation is executed
or passed to a name-aware consumer (verified: `do (quote [Foo])`,
sig parsing, case, unify), but render/`eq`/ordering of quotations-as-
data change. This is the uniform semantics (builtins behave like
every other type name) and is taken as intended by the directive; it
must be stated in REFERENCE.md and re-pinned.

Token choice: the opaque token is a plain **Word in both contexts** —
NOT an Atom. `ParseFnParams` reserves the atom-in-type-slot space for
`/q` keyword params (`fn_params.go:101-110,182-200`); Atom emission
would collide with it.

### 6.5 Blast radius (measured)

~3,414 of 9,671 spec inputs mention a builtin type name, but most
recover through the existing engine passes. The hard core: 3 rows pin
`quote <TypeName>` (`eng/spec/types.tsv:116-118`), ~60-70 rows change
behaviour (quoted containers, typed-container children, `{a?:T}`),
and ~245 expected-column strings pin container renders that flip
`Integer` → `word(Integer)` wherever resolution lands after
rendering. Also in scope: the check-accuracy ratchet
(`pinnedFalsePositives=0` over all 7,264 lang/spec rows — every
data-context name the resolver misses is a new false positive), ~20
parser test functions pinning `NewTypeLiteral` output, `genhelp`
generated artifacts, and — critically — the **TypeScript engine**:
`eng/ts/src/spec-fixture.ts:1853-1859` eagerly resolves type names
"mirroring the parser's TypeNameTable lookup" and the cross-engine
differential gate (`test/go/engspec/crossdiff_test.go`) hard-fails
value divergence, so the TS side must change in lockstep.

### 6.6 What this buys toward "parser has no dep on eng"

The parser references 57 distinct eng symbols (~265 uses). Stage 6
removes the type-CONTENT class: `TypeNameTable`, `ResolveTypePath`,
the `NewTypeLiteral` resolution sites, `SimplifyDisjunctAlts` +
`TNone`/`TAbsent`/`NewDisjunct` (with the `{a?:T}` move), and the
semantic half of the `ChildEntry` constructors. What remains is
mechanism vocabulary: ~20 neutral value constructors (`Value`,
`NewWord`, `NewString`, `BoruError`, …) and ~25 engine-marker
constructors (`NewEnd`, `NewParenExpr`, `NewReach`, `NewSplice`,
`NewInterpString`, the XML family) — the parser's OUTPUT TYPE is
`[]eng.Value`, so it speaks the engine's token vocabulary by
construction. That is consistent with §0.1 (the vocabulary is
mechanics, not content). Full extraction — a neutral AST plus an
eng-side conversion layer — would sever even that, but it must be
designed against the Single-Pass Parsing rule (eng/go/CLAUDE.md): the
conversion layer must BE the single walk, not a second pass. Deferred
as open question 4.

## 7. Constraints on every stage

- **FixedIDs are wire-stable** — moves preserve path + ID; the gate is
  `lang/go/test/fixedid_stability_test.go` (its own home moves with
  stage 3).
- **No words in eng** — stage 5 moves logic and name-ownership, never
  registrations; `calc/go` must keep compiling against eng alone.
- **ADR gating** — NUR009's remediation requires the ownership ADR
  first; ADR entries only on explicit maintainer instruction.
- **Coverage** — ADR-008's 100% gate applies to `types/go` from its
  first commit; the Makefile fan-out and kg
  (`kg/project/boru-project.jsonic`) gain the new component when it is
  created.

## 8. Open questions

1. Component name: `types/go` vs `scalar/go` vs folding into a future
   stdlib component. (`types` clashes with nothing today; `-util`
   naming is a module convention, not a component one.)
2. Matrix/Tensor residence: global FixedIDs (2000-2999) but delivered
   by `BuildMatrixModule`. Module-owned with a global root (the Time
   pattern), or `types/go`?
3. The reverse audit (eng → types): `micron.go`'s validator
   vocabulary, `iso4217.go`, `core_xml.go`, and the `-on` naming rule
   fail the mechanism test but are deeply wired into `make`/refine;
   scope and sequence separately.
4. Whether `eng/go/parser` is itself content: partially answered by
   §6 — after stage 6 the parser carries zero type-inventory
   knowledge and only mechanism vocabulary; its emitted WORD names
   route through the stage-5 table. The remaining question is full
   extraction (neutral AST + eng-side converter, §6.6), deferred: it
   must be designed against the Single-Pass Parsing rule, and the XML
   literal matcher builds finished eng Values inside the lexer
   (`xml_literal.go:16-19`), so extraction re-plumbs that pipeline
   too.
