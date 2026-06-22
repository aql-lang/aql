# EMIT — value→string emission (the inverse of `parse`)

This note records the design of the `emit` word and the `aql:emitlang` module.
It is a design record, not a normative spec; the executable spec is
`lang/spec/module-emitlang.tsv` and the canonical word/module docs come from
`aql describe emit` / `aql describe aql:emitlang`.

## Why

AQL could turn strings into data (`parse <kind> <src>`, the `aql:parselang`
module + the tabnas decoder family) but had no symmetric word for the inverse —
turning data structures into strings. `write` encodes to a *file* via
`Format.Encode`, and a few ad-hoc encoders existed (`valueToJsonic`,
`encodeDelimited`), but the tabnas family was decode-only and there was no
value→string word, no rendering control (compact vs. pretty), and no
natural-format default per structure.

`emit` is `parse`'s mirror image:

```
parse : string → value        emit : value → string
```

## Shape (mirrors `aql:parselang`)

- Core macro word **`emit`** (`lang/go/native/native_macro.go`): expands
  `emit <kind> <opts?> <data>` → `EmitLang get emit_<kind> <data> <opts> end`,
  spliced at the call site (same `__SP` mechanism as `parse`/`mini`). `data` is
  the required LAST surface arg, `opts` the optional middle one (arity
  disambiguates). The kind is OPTIONAL: a bare `emit <data>` routes to
  `emit_auto`, which picks the value's **natural format**.
- Module **`aql:emitlang`** (`lang/go/modules/emitlang.go`), `EmitLang`
  namespace: per-kind `emit_<name>` exports (sig `[value:Any opts:Map] →
  String`), `emit_auto`, and out-of-band `register` / `kinds`. Host API
  `RegisterHostEmitter` + `RegisterFormatEmitter`.

## One canonical walk-based emitter — no exceptions

Every format is a thin profile over a single traversal: `classifyNode`
(`lang/go/native/emit.go`) enumerates a value's ordered children through the
**walk engine** (`childrenOf` / `walkEntry` in `walk_core.go`) — the same
machinery the `walk` word uses. json, jsonic, yaml, csv, tsv, toml, ini and xml
all flow through it; there is no bespoke parallel traversal. The older
single-purpose emitters (`valueToJsonic`, `encodeDelimited`, the `Format.Encode`
methods) are routed through this one core, so `write`, `emit`, and any future
encoder share exactly one code path.

`classifyNode` decides container-ness by TYPE, so an empty container still emits
as `[]`/`{}` (the walk engine reports an empty container as a non-container).
Every concrete access is guarded — type literals and non-concrete carriers
classify as scalar leaves and never panic.

## Report table — structure × format

`✓✓` = natural target · `✓` = optional / sensible · `—` = not supported

| Structure | json | jsonic | yaml | csv/tsv | xml | toml | ini |
|-----------|------|--------|------|---------|-----|------|-----|
| Map / Record / Options / Object / Store | ✓✓ | ✓ | ✓ | — | — | ✓¹ | ✓¹ |
| List / Array | ✓✓ | ✓ | ✓ | ✓² | — | — | — |
| Table | ✓ | ✓ | ✓ | ✓✓ | — | — | — |
| Xml | ✓³ | — | — | — | ✓✓ | — | — |
| Scalar (String/Number/Boolean/Atom/None) | ✓✓ | ✓ | ✓ | — | — | — | — |
| Error | ✓✓ | ✓ | ✓ | — | — | — | — |

¹ toml/ini only for `{k:scalar}` / `{section:{k:v}}` shapes; deeper nesting
raises `emit_unsupported`. ² csv only for list-of-records / list-of-lists.
³ Xml→json renders the element as its source string (optional).

**Natural target** (what a bare `emit <data>` picks, via `NaturalEmitKind`):
Map / Record / Object / Store / Error / Scalars → `json`; List / Array → `json`;
Table → `csv`; Xml → `xml`. A value with no natural target (e.g. a Function)
raises `emit_no_natural`.

## Options

Emission is controlled by the optional middle map argument:

- **json / jsonic** — `{pretty:true}` or `{indent:N}` → newline + N-space
  indentation (default 2); otherwise compact. jsonic additionally renders
  identifier map keys unquoted.
- **yaml** — `{indent:N}` (default 2), block style.
- **csv** — `{separation:sep}` overrides the field separator. (tsv is tab-fixed.)
- **xml** — `{pretty:true}` indents element children.

Map keys follow AQL's own map semantics (sorted), so emitted output matches the
language's canonical rendering and round-trips through `parse`.

## Errors

- `emit_unknown_lang` — an explicit kind that is not registered (expansion-time,
  at the call site; mirrors `parse`'s `parse_unknown_lang`).
- `emit_unsupported` — a shape a format cannot represent (e.g. `emit toml [1 2]`,
  `emit xml {a:1}`, `emit ini {a:{b:{c:1}}}`).
- `emit_no_natural` — `emit <data>` where the value has no natural target.
- `emit_bad_name` / `emit_bad_signature` / `emit_kind_exists` — `EmitLang.register`
  validation, inverting `parse`'s register checks.
