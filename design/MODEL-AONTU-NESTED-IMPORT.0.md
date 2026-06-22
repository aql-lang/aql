# aql:model vs. voxgig/struct test model — upstream aontu nested-import defect

Status: **Validation finding, June 2026.** Documents an upstream
`github.com/rjrodger/aontu` (Go) defect that blocks `aql:model` from
reproducing the canonical `voxgig/struct` test model. No AQL-side fix is
possible; the defect is in the vendored aontu engine and is a parity gap
with the canonical TypeScript implementation. Tracked here (not as an ADR)
per the design-note convention; file upstream at
https://github.com/rjrodger/aontu when in scope.

## What was validated

`voxgig/struct` ships a model at `build/test/test.jsonic` that builds the
struct test specs by `@`-importing nine sibling `.jsonic` files under a
`struct:` tree, plus an inline `primary:` tree. The committed reference
output is `build/test/test.json` (~390 KB).

The goal was to confirm `aql:model` (which wraps the Go
`github.com/voxgig/model` → Go aontu) can build that model and match the
reference exactly.

## Result

`aql:model` **runs the build successfully** (`Model.run` → `ok:true`),
exercising the full pipeline — file-path source through the `aql:io`
FileOps capability, aontu resolution with `@"file"` imports / `&`
defaults / `key()`, and the JSON write — but the output is **705 bytes,
not ~390 KB**: the entire `@`-imported `struct` tree is missing; only the
inline `primary:` tree survives. So the build does not match the
reference.

## Root cause (upstream Go aontu v0.1.4)

A **colon-chain (nested-path) key whose value is a bare `@"file"` import**
silently resolves to `{}` instead of loading the file. With `minor.jsonic`
present in the base dir:

```
aontu.NewWithBase(dir).Generate(`x: @"minor.jsonic"`)              -> loads (24,968 B)   OK
aontu.NewWithBase(dir).Generate(`struct: { minor: @"minor.jsonic" }`) -> loads (24,983 B)   OK
aontu.NewWithBase(dir).Generate(`struct: minor: @"minor.jsonic"`)     -> {}                 BUG
aontu.NewWithBase(dir).Generate(`s: m: @"minor.jsonic"`)              -> {}                 BUG
```

`struct: minor: 1` and `struct: minor: {a:1}` (nested-path with non-import
values) resolve fine, and `key()` works on its own — so the trigger is
specifically **nested-path key + bare `@import` value**. The struct test
model uses exactly that shape (`struct: minor: @"minor.jsonic"`, …) for
every spec import, so the Go engine drops the whole `struct` branch.

The defect reproduces identically at three layers, so it is **not** in the
`aql:model` wrapper:

| Layer | `struct` resolved? |
| --- | --- |
| `aql:model` `Model.run` | no (705 B) |
| voxgig `model.New(...).Run()` | no (223 B model) |
| `aontu.NewWithBase(...).Generate(...)` | no (223 B) |

The canonical TypeScript aontu (which generated the reference) resolves
the nested-path import form correctly.

## Secondary mismatch (key ordering)

Independent of the import bug: the reference `test.json` is in **insertion
order** (`id, doc, in, out`, i.e. TypeScript-generated), whereas
voxgig/model's Go writer marshals `map[string]any` with **alphabetically
sorted** keys (`encoding/json`). voxgig/model's own README documents this
as a known, accepted cross-language difference. So even with the import
bug fixed, a byte-exact match against a TS-produced reference would not
hold — a semantic (order-independent) comparison is the meaningful check.

## Consequences for aql:model

- The `aql:model` module itself is sound: it drove a real multi-file model
  build end-to-end on disk and on an in-memory FS (see
  `lang/go/test/model_test.go`).
- It cannot reproduce the `voxgig/struct` test model until the upstream
  aontu nested-import defect is fixed. When validating model output
  against a TS-built reference, compare **semantically** rather than
  byte-for-byte.
