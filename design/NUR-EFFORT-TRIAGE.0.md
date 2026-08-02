# NUR Effort Triage — the low/medium-effort resolutions

> **Status:** point-in-time triage, 2026-08-02. Every open record in
> [NUR.md](../NUR.md) — the 12 Pending entries plus the six Allowed
> entries that carry a standing fix proposal — was assessed for the
> effort of its *recorded or most plausible resolution path*, grounded
> in code inspection and live reproduction on a freshly built binary
> (every divergence assessed below was re-confirmed to reproduce,
> except where noted). This document **ranks; it does not decide** —
> verdicts remain the maintainer's, per NUR.md's own rules, and
> nothing here changes a record's status.

**Effort scale.** *low* = half a day or less (doc/config/one-liner
plus pins). *low-medium* = about one day (localized code change, a
few files). *medium* = 2–4 days (multi-file or cross-layer, moderate
risk). *high* = a week or more, architectural, or blocked on an open
design decision / new ADR. Every estimate includes the repo's fixed
cost floor: ADR-008's 100% cover-gate on new Go statements, `lang/spec`
TSV pinning, and REFERENCE/guide updates.

---

## The answer: resolvable at low–medium effort

Nine items. Four are pure verdict-plus-documentation work; five are
small, well-scoped code fixes whose direction the maintainer has
already recorded.

| # | Item | Effort | The work | What gates it |
|---|------|--------|----------|---------------|
| 1 | **NUR014** — cross-leaf numeric equality is leaf-pair-dependent | low | Zero Go delta. Every pin the argued Allowed form needs already exists (REFERENCE.md:195–200, `bignum.tsv:47–63`, `edge-scalars-1.tsv:24–25`); the work is rewriting the compact record into the full argued form and removing the pending row. | Maintainer ALLOW verdict (the record carries only a *proposed* allow). |
| 2 | **NUR018** — Store and Error excluded from `make` | low | Doc+spec only on the allow path: argued record, one REFERENCE sentence, an eng/go/CLAUDE.md rule-4 clarification, two negative rows in `eng/spec/make.tsv`. The erroring arm in `core_make.go::isTypeLike` is already covered. | The "one-line verdict" the record itself asks for. |
| 3 | **NUR019** — `slice` is the String family's core straggler | low | Document the real rationale — `slice` is a core *sequence* word polymorphic over String/List/Bytes (9 unqualified signatures), kin of `size`/`take`/`reverse` — plus two filing fixes (REFERENCE.md:1148 parenthetical, `help_categories.go` string-category Desc). The MOVE alternative is medium and semantically worse (it would split a polymorphic word). | Maintainer ALLOW on the sequence-word rationale. |
| 4 | **NUR041** — the `read-only` profile denies file reads | low | Pure config+pins: an allow block for `fileops` read-ops in `read-only.jsonic` (merge semantics verified safe), the sandbox.jsonic comment correction, additive policy-test pins, e2e comment updates. Fix semantics live-verified via the equivalent `-allow fileops.read`. | Verdict is recorded (fix listed first). Two small choices: `read` alone vs the coherent `read`+`stat`+`list` set, and whether the **same latent gap found in the `client` profile** (see findings below) rides along. |
| 5 | **NUR013** — NaN total-order slot vs IEEE relationals | low-medium | The maintainer-directed totalOrder comparison is done in substance: boru already conforms for its single observable quiet NaN; the one fixable gap is **signed zeros** (`-0.0 tcmp 0.0` → 0; totalOrder wants −0 first). Fix = a Signbit tiebreak in `numberCompareBehavior.Compare` (float + big-rat paths), a relational carve-out so `-0.0 lt 0.0` stays false, flipped/new spec rows, and the writeup across IEEE-754-COMPLIANCE / TYPE-ORDERING / REFERENCE. NaN sign/payload ordering is unobservable in boru → argued acceptance. | Final Allowed verdict over the residual divergences once the comparison is recorded. If the maintainer accepts the zeros tie instead, the whole item collapses to *low* (writeup + record). |
| 6 | **NUR031 (narrow half)** — Module descriptor reflexive equality | low-medium | `M.$module eq M.$module → false` today. Exact in-repo precedent (Timeout/Interval opaque handles): two arms in `opaqueIdealExactEqual`/`DeepEqual`, a `handleKind` case in `compare_deqkey.go`, one payload tweak in `NewModuleInstance` (box a `*ModuleDesc` — `ModuleDesc` itself is not Go-comparable), tests + ~8 spec rows + REFERENCE.md:1219 amendment. Satisfies the standing "at minimum reflexive" requirement for modules. | One maintainer choice: identity token (boxed pointer = per-import identity, the safe mirror; vs `ModuleDesc.ID` = per-load identity, cross-import `eq`, needs an ID audit). **Rewrites the record, does not close it** — Function/Word identity and Behavior routing stay design-gated. |
| 7 | **NUR037** — fn-local fn undefined in compiled mode only | low-medium | The per-unit refuse-and-fall-back mechanism already exists (`MarkUncompilable` → interpreter re-run, the NUR051 precedent), and the scope test needed already exists as the ComputeCaptures rule. Fix = one predicate + one guard site in `recordDispatchOutcome`, refusing units whose code body names a fn-local fn — default run, `-no-compile`, and `check` then agree ("slow, not wrong" restored). Day is spent on coverage tests, differential-gate spec rows, and retiring the house-rule docs. | None — the recorded verdict explicitly sanctions refusal ("a refusal is merely slow"). The preferred closure-capture fix remains available later as a *medium* widening and does not gate deleting the record. |
| 8 | **NUR044** — `boru build` skips `run`'s preflight | low-medium | The shared preflight exists (`check.PreflightColor`); wiring it into build with `-no-check`/`BORU_NO_CHECK` is ~15 lines. The real content is the discovered **import-anchoring trap**: check resolves relative imports against the cwd, build against the entry dir (verified live) — the preflight needs a baseDir-aware variant anchored to `cfg.EntryDir` or the fix breaks the existing multi-file e2e. All inside cmd/go; no spec impact. | None — the recorded verdict directs exactly this. |
| 9 | **NUR047** — regex match offsets are bytes in a rune-indexed language | low-medium | One construction site (`reMatchResult`, shared by `lang_re` and `run-re`): a single-pass byte→rune conversion covers everything; check-mode shape is unit-agnostic. Every existing offset pin is ASCII, so no expectation churn. Remove grep.boru's Bytes workaround; its three multi-byte pinning tests keep their expected strings verbatim and simply invert their meaning (they now guard the fix). New non-ASCII spec rows + doc-string updates. | Effectively none — the recorded verdict states the preference ("fix by returning rune offsets"); a scheduling nod flips the record from Allowed to fixed. |

### Cheap *halves* of bigger items (real options, flagged separately)

These don't close their records on their own terms without a
maintainer pick, but each is a genuinely small unit of honest work:

- **NUR023(a) — the mechanical half (low):** flipping the ~13 zero-arg
  `BarrierPos: 0` registrations to `-1` is *provably inert*
  (`registry.go:1585` normalizes `-1` to 0 for 0-arg sigs — stored
  signatures are byte-identical), and pinning `apply`'s `[Function]`
  case into REFERENCE's closed list is a doc edit. An executable
  uniformity test (no stack-only sig outside the closed list) would
  keep it true. The record itself only closes with the
  maintainer-instructed ADR-004 refinement — the (b) half — which is
  what makes the full item medium.
- **NUR042 remove-the-flag path (low):** deleting `-policy-dry-run`
  (field, registration, one read, one test, two doc rows) is half a
  day. The record's *preferred* path — the observe-only decorator —
  is medium (see below). The verdict is an explicit disjunction, so
  this is a legitimate resolution, not a dodge.
- **NUR045 delete-the-schema path (low-medium):** removing the dead
  per-export `words` schema, stripping `sandbox.jsonic`'s inert
  `deny: ["sleep"]`, making profile validation *reject* per-module
  `words` blocks so the dead-schema class cannot recur, and
  documenting import-granularity-only gating is about a day —
  contained in `lang/go/policy` + profiles + docs. The enforce path
  is medium (below). Either way the false guarantee dies; the record
  reserves the choice to the maintainer's hot-path judgement.

---

## Just above the line: medium (2–4 days each)

- **NUR022 — `del`/`set` symmetry.** The FlexXml/Class/Micron arms are
  pattern-following, but the Store arm forces new kernel machinery: a
  `CowDel` with a tombstone that the prototype-chain `Get`,
  `storeEntryMap`, and check-mode context typing must all honor. The
  verdict's first investigation step is discharged: absent-vs-none
  **is** real and observable (`has` distinguishes; `del` of a
  none-valued key removes the entry). Probing also exposed a
  pre-existing Store enumeration/lookup asymmetry (findings below)
  that the spec rows will trip over.
- **NUR023 — full record.** The ADR-004 refinement (BarrierPos
  semantics, argument-handling categories, the closed list including
  `apply`, chaining rationale) plus category-explaining diagnostics;
  the diagnostics ripple into pinned ERROR rows and the
  compiled-vs-interpreted Detail-equality gate. ADR half is
  maintainer-instruction-gated.
- **NUR026 — unified string lexer.** Vendoring ~278 lines of escape
  machinery from tabnas/parser into a boru-owned matcher, rerouting
  *every* quoted string in the language plus the template scanner
  through it. Well-scoped and verdict-directed, but corpus-wide blast
  radius and a heavy cover-gate bill on the error branches. One
  unstated sub-choice worth a one-line confirmation: templates'
  unknown-escape handling flips from keep-backslash to drop-backslash.
- **NUR042 — dry-run decorator (preferred path).** ~100 LOC decorator
  modeled on the composed-policy precedent, but the flag is registered
  on **six** subcommands (not two), the build path must bake a config
  bit, and there's a wrap-ordering hazard (flattening the wrapper
  would silently bake an allow-all profile).
- **NUR045 — per-export gate, enforce path.** Small per site but must
  land on both engines under the parity discipline; the VM's
  `callPolyIn` needs the module ID threaded onto `PolyRef`. Two
  findings materially help the maintainer's cost call: the
  per-(module,export) decision is *fully static* (an import-time
  pre-evaluation makes per-call cost zero), and `boru policy explain`
  *already answers DENY* for the dead rule — the inspection surface
  promises the denial the runtime doesn't perform, which weighs
  toward enforcing.
- **NUR046 — formatter idempotence.** The suspected mechanism in the
  record is not quite right: the true cause is re-parse
  statement-segmentation drift (root-level newlines emitted
  mid-statement), already noted in `format_cov_test.go`'s own
  comments. Fix is localized to `format.go` (two layout strategies),
  but the canonical form changes for **19** checked-in `.boru` files
  (not six), and the internal run-until-fixed-point shortcut is *not*
  acceptable — the converged pass-2 layout is strictly worse, which
  the record's own reasoning rejects. Needs the double-format
  regression guard landing with it.
- **NUR049 — symmetric paren barrier.** The engine needs almost no
  sealing — every probe shows dispatch already stops at the open
  paren, so backward reach fails deterministically *today* (the
  record's "reaches backward" claim did not reproduce in any context;
  see findings). The real defect is that the failure isn't *static*:
  error-handler bodies are wholly unchecked (`error [zzz-undefined]`
  checks clean). The fix is un-gating the already-written seeded
  handler-body check run in `errorReturnsFn` (diagnostics only, not
  the compile-gated narrowing), triaging what it newly surfaces
  (~61 corpus handlers, ~75 spec rows), fixing the misleading
  `strandedForwardError` help text that *recommends* the broken form,
  repairing two shipped examples, and an e2e that forces the error
  arms. The verdict's required compatibility check is effectively
  done and clean: the corpus sweep found no sanctioned point-free
  pattern relying on backward reach.

---

## Design-gated: high (not schedulable as ordinary fixes)

- **NUR009 — Bytes and the DepScalar bases.** The verdict reframes it
  as kernel type-hierarchy *ownership*: a new ADR (candidate 2) plus
  likely migrations of Bytes/Time/Date/DateTime/Instant into eng.
- **NUR030 — `group`'s render-key fold.** Open design decision among
  status quo / String-only keys / grouped pairs, sitting on the
  language-wide Map-keys-as-rendered-strings question.
- **NUR031 — full closure.** Function/Word identity (stable canon
  independent of binding name) and routing Ideal equality through the
  type's Behavior — the ADR the record defers to, tracked with the
  NUR050 lineage. The narrow module-descriptor half above is the part
  that doesn't need to wait.

---

## Incidental findings surfaced by the triage

Recorded here so they aren't lost; none is acted on by this document.

1. **`client` profile has NUR041's exact gap** — `boru policy explain
   client fileops.read path=x` → DENY (blame `fileops.words
   default=deny`) while HOWTO promises "Read disk". Fix alongside
   NUR041 or record separately.
2. **Store enumeration/lookup asymmetry** (found probing NUR022):
   after two `context set`s, `size (context)` → 1 and `convert Map`
   shows only the newest COW layer's key while `get`/`has` see both
   through the prototype walk. Store-del spec rows will collide with
   this; it may deserve its own NUR.
3. **NUR049's premise is partially stale:** the one-directional
   backward *reach* does not reproduce anywhere — paren groups
   already fail deterministically when self-insufficient. What
   remains is the static-checking gap (unchecked error-handler
   bodies) and help text actively recommending the broken idiom
   (`forwardParensSuggestion`/`strandedForwardError`), plus a second
   latent instance of the bug class in
   `design/examples/todo/audit.boru:29` (`err: (dot code)`).
4. **NUR046's blast radius is 19 files, not six** — including three
   shipped `lang/go/modules/*.boru` — and a second non-idempotent
   shape (bracket-free over-width root statements losing hanging
   indent) exists in the same class.
5. **`describe apply` misleads:** it reports word-level "Precedence:
   forward" although its `[Function]` signature is stack-only — the
   label needs to be per-signature (part of NUR023's diagnostics
   work).

## Suggested sequencing, if the low–medium list is worked as a batch

Verdict-only first (NUR014, NUR018, NUR019 — one maintainer session
covers all three), then the recorded-verdict code fixes in isolation
from each other: NUR041 (+client-profile decision), NUR044, NUR047,
NUR037, NUR013, NUR031-narrow (after the identity-token choice). Each
is independently landable; none stacks on another, so they can merge
in any order without cross-record conflicts.
