# `aql:unicode` — Go `unicode`

> **Status: design proposal, not implemented.** A curated, hand-written
> native module wrapping Go's `unicode` (rune-level classification and
> case). Read [README.10.md](README.10.md) first for the shared
> conventions this note assumes.

## 1. Package & status

Go [`unicode`](https://pkg.go.dev/unicode) provides per-**rune**
predicates (`IsLetter`, `IsDigit`, …) and per-rune case mapping
(`ToUpper`, `ToLower`, `ToTitle`). This note specifies an idiomatic AQL
surface that operates on a single-character String (one rune). Nothing
is implemented yet.

## 2. Why curated

Go's `unicode` functions take a `rune` (an `int32`). AQL has no rune
type, and the raw `go:` bridge would force callers to pass an Integer
code point. The curated surface takes a **single-character String**
(the natural AQL spelling of a character), reads its first rune, and
applies the predicate / case map — so `"A" Unicode.is-upper` reads
exactly as intended. Rune-level classification has no equivalent in
`string-util` (which is whole-string oriented), so this is a genuinely
new, non-overlapping surface.

## 3. Import & namespace

```
import "aql:unicode"        # binds the Unicode namespace
```

`unicode` does not clash with a builtin type or existing module
namespace, so no `-util` suffix (naming rule in `lang/go/CLAUDE.md`
"Package layout"). Words are dot-accessed: `Unicode.is-letter`,
`Unicode.to-upper`, etc.

## 4. API

All words operate on a single-character String — the **first rune** of
the argument. Signatures are top-first, sig order; inner native sigs use
`BarrierPos: -1`. Single-arg words dispatch in both forward and swap
forms (the example column uses the args-before swap form).

| Go symbol | aql word | signature (top-first) | one-line doc | aql-ish refinement |
|---|---|---|---|---|
| `IsLetter(r) bool` | `is-letter` | `[String] -> Boolean` | True if the rune is a letter (Unicode L). | rune → first rune of a 1-char String. |
| `IsDigit(r) bool` | `is-digit` | `[String] -> Boolean` | True if the rune is a decimal digit (0–9). | rune → first rune. |
| `IsNumber(r) bool` | `is-number` | `[String] -> Boolean` | True if the rune is a number (Unicode N, incl. ⅗, Ⅻ). | rune → first rune; broader than `is-digit`. |
| `IsSpace(r) bool` | `is-space` | `[String] -> Boolean` | True if the rune is whitespace. | rune → first rune. |
| `IsUpper(r) bool` | `is-upper` | `[String] -> Boolean` | True if the rune is an uppercase letter. | rune → first rune. |
| `IsLower(r) bool` | `is-lower` | `[String] -> Boolean` | True if the rune is a lowercase letter. | rune → first rune. |
| `IsPunct(r) bool` | `is-punct` | `[String] -> Boolean` | True if the rune is punctuation (Unicode P). | rune → first rune. |
| `IsControl(r) bool` | `is-control` | `[String] -> Boolean` | True if the rune is a control character. | rune → first rune. |
| `IsTitle(r) bool` | `is-title` | `[String] -> Boolean` | True if the rune is a title-case letter. | rune → first rune. |
| `IsGraphic(r) bool` | `is-graphic` | `[String] -> Boolean` | True if the rune is a graphic (printable incl. spaces in L/M/N/P/S/Zs). | rune → first rune. |
| `IsPrint(r) bool` | `is-print` | `[String] -> Boolean` | True if the rune is printable (graphic, or ASCII space). | rune → first rune. |
| `ToUpper(r) rune` | `to-upper` | `[String] -> String` | Map the rune to upper case. | rune→rune becomes 1-char String→1-char String. |
| `ToLower(r) rune` | `to-lower` | `[String] -> String` | Map the rune to lower case. | rune→rune → String→String. |
| `ToTitle(r) rune` | `to-title` | `[String] -> String` | Map the rune to title case. | rune→rune → String→String. |

Note: `IsTitle` is included for completeness alongside `to-title`; if
the roster prefers the strict minimum it can be dropped without
affecting any other word.

## 5. Types

Scalars only — String in, Boolean or String out. No opaque handle type,
no `RegisterExternalBuiltin` / FixedID. The boundary is a String→rune
read in the handler (`[]rune(s)[0]`), not a `FromNative` slice copy.

## 6. Errors

Every word reads the **first rune** of the String argument, so an empty
String has no rune to classify:

| code | raised when |
|---|---|
| `expected-single-char` | the String argument is empty (no first rune). |

Design choices pinned by this note:
- Empty String → `expected-single-char` error (no rune).
- A multi-rune String is **not** an error — only the first rune is used
  (e.g. `"abc" Unicode.is-letter` classifies `'a'`). This keeps the
  words total over non-empty strings; an alternative strict mode
  (error on length > 1) is an open question (§10).

Guard with `AsConcreteString`, then check `len([]rune(s)) > 0` before
indexing; never panic (`eng/go/CLAUDE.md` "Panic Prevention").

## 7. Policy / capabilities

None — pure classification, runs under any policy.

## 8. Overlap (IMPORTANT)

`string-util` (`aql:string-util`, `lang/go/modules/docs_string.go`)
already owns **string-level** Unicode work:

- `StringUtil.normalize` — Unicode **NFC normalisation** + whitespace
  tidy, over a whole string.
- `StringUtil.changecase` — recase a whole string
  (lower/upper/camel/snake/…); `StringUtil.upper` / `StringUtil.lower`
  likewise operate on the whole string.

`aql:unicode` is **per-rune classification and case** — a different,
non-overlapping granularity. `Unicode.to-upper` upper-cases a *single
character*; `StringUtil.upper` upper-cases a *whole string*.
`Unicode.is-letter` answers "is this character a letter?"; nothing in
`string-util` answers that. No existing word is moved or changed; this
is purely additive.

## 9. Examples (args-before form)

```
import "aql:unicode"

"A" Unicode.is-upper                    # true
"a" Unicode.is-letter                   # true
"5" Unicode.is-digit                    # true
"Ⅻ" Unicode.is-number                   # true   (Roman numeral, a Number but not a Digit)
" " Unicode.is-space                    # true
"a" Unicode.to-upper                    # "A"
"A" Unicode.to-lower                    # "a"
"" Unicode.is-letter                    # ERROR:expected-single-char
```

## 10. Open questions / out of scope

- **Multi-rune strictness** — current proposal uses the first rune and
  ignores the rest. Should a length > 1 String error instead (strict
  single-char contract)? First-rune is more forgiving; strict is more
  honest. Left open.
- **`Is(rangeTab, r)` / `In(r, ranges...)` / script & category tables**
  (`unicode.Latin`, `unicode.Han`, …) — table-driven membership tests
  are out of scope for the first cut; revisit if script detection is
  requested.
- **`SimpleFold`** — case-insensitive fold iteration is niche; deferred.
- **Code-point access** — a `rune → Integer code point` word (and its
  inverse) is not proposed here; if needed it pairs more naturally with
  a future `aql:strconv`/string-index surface.

## 11. Implementation sketch

Wiring checklist — no Go code. Reference: `lang/go/modules/math.go`
(`BuildMathModule`, the pure-module canonical builder).

- `lang/go/modules/unicode.go` — `BuildUnicodeModule(parent
  *native.Registry) (native.ModuleDesc, error)`: isolated
  `native.DefaultRegistry()` sub-registry, register a `UnicodeNatives
  []native.NativeFunc` slice (inner sigs `BarrierPos: -1`), wrap each as
  an `FnDef` export into an `*OrderedMap`, return `ModuleDesc{ID:
  parent.Modules.NextID(), Exports: {"Unicode": …}}`. The predicate
  words can be generated from a `(name, func(rune) bool)` table and the
  case words from a `(name, func(rune) rune)` table, mirroring
  `math.go`'s `classifierNative` / table-driven loops.
- Register `"unicode": BuildUnicodeModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_unicode.go` — `registerDocs("aql:unicode", …)`
  one line per export (`TestModuleExportDocs` enforces it).
- `lang/spec/module-unicode.tsv` — rows leading with `import
  "aql:unicode"`; each positive row paired with an `ERROR:<substring>`
  negative sibling (notably the empty-String `expected-single-char`
  case).
- No FixedID entry, no policy wiring.
