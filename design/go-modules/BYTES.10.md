# `aql:bytes` — Go `bytes` — **NICHE / optional, recommend deferring**

> **Status: design proposal, not implemented — and flagged NICHE.** This
> note documents the surface for completeness but recommends **not**
> building the module yet. Read [README.10.md](README.10.md) first.

## 1. Package & status

Go [`bytes`](https://pkg.go.dev/bytes) is the `[]byte` analogue of
`strings`: search, compare, slice, trim, and build byte slices, plus a
`bytes.Buffer`. Nothing is implemented yet, and (see §10) the
recommendation is to keep it that way for now.

## 2. Why curated — and why this one is weak

**Key design tension, up front:** AQL has **no Bytes type** in the
lattice. Every `bytes` function is defined over `[]byte`, which has no
direct AQL counterpart. So a curated module must first *invent* a
representation, and then most of what it offers simply duplicates
`String` / `string-util` words that already exist. The value over the
raw `go:` bridge is therefore low, and the cost (a fabricated byte model
that the rest of the language doesn't understand) is real. This note is
candid: the right call is to **defer**.

## 3. Import & namespace

```
import "aql:bytes"          # would bind the Bytes namespace
```

`bytes` does not clash with a builtin type or existing module namespace
(there is no `Bytes` builtin — that is precisely the problem), so no
`-util` suffix. Words would dot-access as `Bytes.contains`, etc.

## 4. API (proposed representation: List of Integers 0–255)

Proposed model: a byte sequence is a **`List` of Integers**, each in
0–255. The boundary converts `List[Integer] ↔ []byte` (range-checking
each element). Signatures top-first, sig order; inner sigs `BarrierPos:
-1`.

| Go symbol | aql word | signature (top-first) | one-line doc | aql-ish refinement |
|---|---|---|---|---|
| `Contains(b,sub) bool` | `contains` | `[List sub, List b] -> Boolean` | True if b contains sub. | `[]byte` → `List[Integer 0–255]`; subject last. |
| `HasPrefix(b,p) bool` | `has-prefix` | `[List p, List b] -> Boolean` | True if b starts with p. | List model. |
| `HasSuffix(b,s) bool` | `has-suffix` | `[List s, List b] -> Boolean` | True if b ends with s. | List model. |
| `Index(b,sub) int` | `index` | `[List sub, List b] -> Integer` | First index of sub, -1 if absent. | byte index (not rune); List model. |
| `Equal(a,b) bool` | `equal` | `[List b, List a] -> Boolean` | True if the two byte lists are identical. | duplicates list `eq`. |
| `Repeat(b,n) []byte` | `repeat` | `[Integer n, List b] -> List` | Concatenate n copies of b. | List model; n top arg. |
| `Join(s,sep) []byte` | `join` | `[List sep, List-of-Lists s] -> List` | Join byte lists with a separator. | List model. |
| `Split(b,sep) [][]byte` | `split` | `[List sep, List b] -> List` | Split b on sep into a list of byte lists. | List model. |
| `TrimSpace(b) []byte` | `trim-space` | `[List] -> List` | Strip leading/trailing ASCII whitespace bytes. | List model. |
| `ToUpper(b) []byte` | `to-upper` | `[List] -> List` | ASCII-upper each byte. | duplicates `StringUtil.upper`. |
| `ToLower(b) []byte` | `to-lower` | `[List] -> List` | ASCII-lower each byte. | duplicates `StringUtil.lower`. |
| `bytes.Buffer` | (deferred) | — | — | A growable buffer would need an opaque external handle (`RegisterExternalBuiltin`, FixedID 10000+). Not worth it given the niche status — out of scope. |

Every row above has a near-identical `String` / `StringUtil` sibling
(`StringUtil.contains`, `StringUtil.split`, `StringUtil.upper`, …)
operating on the type AQL users actually have. The byte-list versions
add value only for true binary data, which is exactly the case the
lattice can't represent today.

## 5. Types

Proposed: `List` of Integers (0–255) for byte sequences — **no new
type**. `bytes.Buffer` would require an opaque external handle
(`RegisterExternalBuiltin` with a `FixedID` in the 10000+ range, plus a
`lang/go/test/fixedid_stability_test.go` entry); deferred. The lack of a
real Bytes leaf is the central open question (§10).

## 6. Errors

| code | raised when |
|---|---|
| `expected-byte-list` | an argument is not a concrete List, or an element is not an Integer in 0–255. |

Guard with `RequireConcreteList` and range-check each element before
converting to `[]byte`; never panic (`eng/go/CLAUDE.md` "Panic
Prevention").

## 7. Policy / capabilities

None — pure, in-memory. (No file/network I/O; `bytes.Buffer` as an
`io.Writer` is out of scope, so no `aql:io` capability seam is needed.)

## 8. Overlap

**Heavy overlap with `String` / `aql:string-util`** — this is the core
reason to defer. `contains`, `index`, `has-prefix`/`has-suffix`,
`split`, `join`, `repeat`, `trim-space`, `to-upper`, `to-lower` all have
String-typed equivalents (`lang/go/modules/docs_string.go`:
`StringUtil.contains`, `StringUtil.indexof`, `StringUtil.split`,
`StringUtil.repeat`, `StringUtil.trim`, `StringUtil.upper`,
`StringUtil.lower`). On the List-of-Integers model, several also overlap
core list words (`eq`, list slicing). The byte module would mostly
re-implement existing surfaces against a fabricated representation.

## 9. Examples (args-before form — illustrative only)

```
import "aql:bytes"

[111 108] [104 101 108 108 111] Bytes.contains   # true  ("ol" in "hello" as bytes)
[104 105] Bytes.to-upper                          # [72 73]
[104 105] 3 Bytes.repeat                          # [104 105 104 105 104 105]
[256] Bytes.to-upper                              # ERROR:expected-byte-list  (out of 0–255)
```

## 10. Open questions / out of scope — **recommend deferring the whole module**

- **No Bytes type in the lattice** — the blocking issue. The
  List-of-Integers model is workable but verbose, type-unsafe (any List
  of small ints looks like bytes), and disjoint from String. **Open
  question / prerequisite:** add a real `Bytes` (binary) leaf to the
  type lattice. Only then does a `bytes` module earn its keep — encode
  String↔Bytes, hashing inputs, base64/hex sources, binary file reads
  (`aql:io`). Until then, **defer this module**.
- **`bytes.Buffer`** — out of scope (opaque handle not justified at
  niche priority).
- **`bytes.Reader` / `io.Reader` adapters** — out of scope; that is
  `aql:io` territory if a Bytes type ever lands.

**Recommendation:** do not implement `aql:bytes`. Revisit only after a
Bytes/binary type is added to the lattice; track that as the gating open
question above.

## 11. Implementation sketch (if revived)

Should a Bytes type be added and the module revived, the wiring mirrors
the pure-module pattern in `lang/go/modules/math.go`
(`BuildMathModule`):

- `lang/go/modules/bytes.go` — `BuildBytesModule(parent
  *native.Registry)` building an isolated sub-registry of
  `BytesNatives` (inner sigs `BarrierPos: -1`), exported under
  `{"Bytes": …}`; register `"bytes": BuildBytesModule` in the `modules`
  map (`lang/go/modules/modules.go`).
- `lang/go/modules/docs_bytes.go` — `registerDocs("aql:bytes", …)`.
- `lang/spec/module-bytes.tsv` — rows leading with `import "aql:bytes"`,
  each positive paired with an `ERROR:<substring>` sibling
  (`expected-byte-list`).
- A `bytes.Buffer` handle, if ever wanted, needs a `RegisterExternalBuiltin`
  FixedID in the 10000+ range plus a `fixedid_stability_test.go` entry.
- No policy wiring.
