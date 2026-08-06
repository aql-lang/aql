# basic/go — Base Language Layer CLAUDE.md

The `basic` module (`github.com/boru-lang/boru/basic/go`, package
`basic`) is the boru **base language layer**: the fundamental words and
the predefined global content types, registered against the eng
kernel. It sits between `eng` and `lang`:

```
eng/go  ←  basic/go  ←  lang/go  ←  cmd/go
```

**The dependency rule is hard (ADR-013): basic depends on eng, and on
eng only.** No other boru sibling, no host-capability dependency
(sqlite, file ops, formats — those are lang's). The gate is
`depsgate_test.go`; if a change here seems to need more, the design is
wrong, not the go.mod.

## What lives here

- **Fundamental words** (each file exposes registration slices that
  lang's `Register` applies at its historical points, and
  `register.go` here composes for eng-only embedders):
  - `native_stack.go` — the forth-style stack vocabulary (`dup`,
    `swap`, `drop`, `over`, `rot`, `nip`, `tuck`, `dup2`, `swap2`,
    `drop2`, `over2`, `depth`, `pick`, `roll`).
  - `native_definition*.go` — `def`, `undef`, `var`, `fn`, `afn`,
    `fnsig`, `args`, `__pa`, and the synthesized def keyword forms.
  - `native_control.go` + `conditional.go` + `case_exhaustive.go` +
    `forloop.go` — `do`, `if`, `case`, `for`, `break`, `continue`,
    `error`, and the case-exhaustiveness checker.
  - `native_type_gen.go` — the type-generics words (`gen`,
    `extends`, `default`, `of`), coupled to the def keyword forms.
  - `native_const.go` — `const`.
- **Predefined content types** with their type-local logic (the
  ADR-012 rule 1 middle home):
  - `native_temporal.go` — the `Scalar/Time` family (FixedIDs
    1000-1003): registrations, Behaviors/Comparers, constructors,
    and the `boru:time-util` module mints.
  - `micron.go` + `micron_grammar.go` + `iso4217.go` — the
    `Scalar/Micron` structured-scalar family's CONTENT: the twelve
    leaf validators/constructors, the merged literal grammar
    (`MicronFromString`), the family Behavior/Comparer, the
    property tables, the currency table, and the `-on` naming
    rule. The identities stay kernel-declared (eng builtinDecls,
    the Resource/Entity precedent); the content plugs in through
    eng's capabilities — `InstallMicronIdeals` per registry (called
    by `Register` here, lang's `Register`, module sub-registries,
    and the engspec fixture set), the `SubtypeNamer` /
    `MicronSubtypeMinter` Behavior capabilities, and the render
    bridge.
  - `types_bytes.go` — `Scalar/Bytes` (1009): registration,
    Behavior (render/equality/ordering/size/const-bake), the Go
    bridge wiring, and the constructor/unwrapper pair. The bytes
    WORDS and the binary-frame machinery stay in lang.
  - `types_handles.go` — `Ideal/Patrun` (5004), `Ideal/Pid`
    (5007), `Ideal/Service` (5008): identity + Behavior shells.
    The patrun matcher and service state stay in lang with their
    words and implement the `PatrunFormatter`/`ServiceFormatter`
    delegation interfaces — never an upward import.
  - `types_timer.go` — the Timeout/Interval handle constructors and
    Behaviors (module-minted types, former FixedIDs retired).
  - `types_resource.go` — the `Resource`/`Entity` definitions.
  - `typeinit.go` — the init-time registration-error accumulator
    (ADR-005: record, never panic; surfaced at registry
    construction).

## Conventions

- `aliases.go` re-exports eng exactly like `lang/go/native/aliases.go`
  does — word files here stay eng-agnostic. Everything in
  `lang/go/CLAUDE.md` (argument ordering, registry bindings, helper
  API discipline, panic prevention, negative-test pairing) applies to
  this package unchanged; read that guide before changing word
  handlers here.
- Identifiers that lang's word files or test suites still reference
  are exported and re-imported through `lang/go/native/basic_shim.go`.
  When you export a new one, add the shim alias in the same change.
- Word registration order matters for overload appends. lang applies
  this module's slices at its historical points in
  `lang/go/native/register.go`; keep `Register` here consistent with
  that relative order.
- FixedIDs are wire-stable — never renumber
  (`lang/go/test/fixedid_stability_test.go` is the gate).
- The coverage gate (ADR-008) measures this module through the merged
  cross-suite profile: lang's suites legitimately cover statements
  here, exactly as they cover eng's.
