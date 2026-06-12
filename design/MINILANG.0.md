# AQL Mini-Languages — the `mini` macro and the MiniLang module

**Status:** design (rev 2, 2026-06-12) — supersedes rev 1's `xy/`
lexer-literal design (condensed in Appendix A; its kind catalogue is
retained in §5). Core mechanics of rev 2 were validated against the
live engine with a pure-AQL prototype — every claim marked ✓ ran; the
empirical findings are in §10.

Embedded domain notations — regular expressions, path queries,
transliteration, "natural" infix maths — delivered through one word,
one module, and one standard signature, with **no parser or lexer
changes**.

```
"aql:minilang" import end

"AbcD" mini re '[a-z]+'                  # → 'bc'
mini math 'x^2 + 3*y' {x:10, y:2}        # → 106
"2025-03-23" mini re-sub `(\d+)-(\d+)-(\d+)` {repl:'$3.$2.$1'}
                                          # → '23.03.2025'  (backtick src:
                                          #    backslash-safe — see F3)
```

---

## 1. Design in one paragraph

`mini` is a **macro** with surface `mini <kind> <src> <opts?>`. The
kind is a bare word captured as an atom; it names the expansion
target: the call rewrites to the **standard minilang call**

```
mini <kind> <src> <opts>   ⇒   MiniLang.lang_<kind> <src> <opts> end
```

(token chain `MiniLang get lang_<kind> <src> <opts> end`), where
`MiniLang.lang_<kind>` is a word with the **standard minilang
signature** (§3) exported by the `aql:minilang` module. A minilang can
take further inputs from the stack and leave any number of results;
it raises errors through the normal `raise`/`Ideal/Error` machinery.
New minilangs are registered from native Go (§6) or from pure AQL
(§7). Because expansion produces an ordinary typed word call, the
static checker sees through `mini` per-kind once expansion is visible
to check mode (§9).

---

## 2. Surface

```
mini <kind> <src>
mini <kind> <src> <opts>
```

| Operand | Capture | Rules |
|---------|---------|-------|
| `kind` | bare word → atom (raw, never evaluated) | **must be literal** — it selects the expansion target at expansion time |
| `src` | one raw form, re-emitted into the expansion | a String literal is canonical. Backtick strings without `${}` are accepted as literals (backslash-safe — see F3). A non-literal form (word, paren group) is allowed and evaluates at the call site, **except** for kinds that declare a compile hook (§11), which require a literal |
| `opts` | one raw Map form, re-emitted | optional; `mini` normalizes a missing opts to `{}`. Values are **not** evaluated at capture — they evaluate in the generated code, at the call site, exactly once ✓ |

`mini` itself is a **core word** (macro category, beside `macro` /
`quote` / `word`): it is language infrastructure and carries no kinds
of its own. All kinds — including the standard library of them — live
in modules and must be registered before first use (define-before-use,
same staging rule as macros).

---

## 3. The standard minilang signature

Every registered minilang `<name>` is exported as the word
`MiniLang.lang_<name>` whose signatures all share the **standard
prefix**:

```
MiniLang.lang_<name> :  [ src:String  opts:Map  …inputs ]  [ …outputs ]
```

- `sig[0]` **src** — the minilang source text.
- `sig[1]` **opts** — named parameters; `{}` when the caller gave none.
- `sig[2..]` **inputs** — kind-declared, zero or more. At a `mini`
  call site these are always filled **from the stack**, because the
  expansion's trailing `end` stops forward collection (§4). ✓
- **outputs** — kind-declared, any number. ✓ (a kind leaving two
  values was exercised in the prototype)

Examples of the two common shapes:

```
MiniLang.lang_re   : [src:String opts:Map subject:String]  [Map]      # filter — input from stack
MiniLang.lang_math : [src:String opts:Map]                 [Number]   # generator — inputs via opts
```

Because the expansion is the standard call, **`mini` is pure sugar**:
the desugared form is always writable by hand and means the same
thing ✓ —

```
"AbcD" mini re '[a-z]+'        ≡        "AbcD" MiniLang.lang_re '[a-z]+' {} end
```

### Why the `lang_` prefix

The kind namespace is **partitioned** inside the one export map:
`mini <kind>` resolves `lang_<kind>` and nothing else. This is safer
than bare kind names and allows **out-of-band exports** — words in
the `MiniLang` namespace that are part of the module's API but are
not minilangs and can never be invoked via `mini`:

```
MiniLang.register   # §7 — register a kind from AQL
MiniLang.kinds      # list registered kind atoms (discovery)
MiniLang.lang_re    # a kind — reachable as `mini re …`
```

`mini register …` would look up `lang_register` and fail loudly at
expansion time; conversely a future helper export can never shadow or
be shadowed by a kind. Registration validates the unprefixed name
(lowercase atom, no `lang_` prefix supplied by the caller).

---

## 4. `mini` is a macro — expansion semantics

Expansion uses the landed macro machinery (MACROS.8.md /
MACROS-PHASE1.10.md) and adds no new mechanism:

1. **Raw-capture** `kind` (word, kept un-evaluated), `src` (one form),
   `opts` (raw map; absent → `{}`).
2. **Resolve the target name**: `lang_` + kind atom. If the binding
   does not exist at expansion time, raise
   `[aql/mini_unknown_lang]` with a hint
   (`import "aql:minilang"` / `MiniLang.register`) at the **call
   site** span.
3. **Emit** the token list
   `MiniLang get lang_<kind>  <src-form>  <opts-form>  end`
   and splice it via the existing `__SP` path.
4. **Cache** the expansion under the standard macro key
   (word + operand canon) — identical mini calls expand once. ✓

### The trailing `end` (mandatory, inserted by `mini` itself)

A generated word with unsaturated forward-eligible positions would
otherwise **steal the tokens that follow the mini call** in source.
Demonstrated live (F1): without the `end`,

```
"cAaB" mini re 'Aa' "ZZ"      # match silently ran against "ZZ"; "cAaB" stranded
```

With the `end`, the forward phase stops at the boundary, the
remaining standard-sig positions fill **from the stack** (the §3
inputs contract), and `"ZZ"` passes through untouched ✓. The same
guard makes chained minis bind each subject correctly ✓:

```
"first" mini re 'rs' "second" mini re 'eco'      # two results, no cross-theft ✓
```

This is the engine's own documented idiom — the runaway-collection
error hint already prescribes *"group the call with parens — (… ) —
or end it with `end` or `;`"* — applied mechanically at the one site
where the user cannot apply it by hand (the generated code is
invisible). It is inserted by the `mini` expander, **not** by kind
authors, so it cannot be forgotten. `end` is an ordinary tape value
(`Word/__ED`), so it survives templates, the expansion cache, paren
groups, fn bodies, and `each` bodies ✓ (all exercised).

Consequences, all verified ✓:

- values still cross the boundary via the stack (`(… mini re 'x')
  .ok`, results consumed by later statements);
- a mini call composes inside paren groups, fn bodies, and
  higher-order bodies;
- an un-parenthesized *mixed* enclosing form (`foo a mini re 'x' b`)
  is cut at the `end` — `b` starts a new statement. That shape is
  already `mixed_form_call`-advisory territory; the cut is the
  predictable behaviour, and parens remain the composition tool.

Note for kind authors: kinds must not rely on barrier (`|`)
signatures for theft protection — the auto-`end` owns that. Barriers
remain a per-word design choice for genuinely pipeline-style words,
orthogonal to `mini`.

---

## 5. The `aql:minilang` module

A plain (non-`-util`) module — it is a DSL/framework module per the
naming rule — exporting the `MiniLang` namespace. The standard kinds
are rev 1's catalogue, carried over with the two-letter prefixes
becoming ordinary kind atoms (no lexer involvement):

| Kind atom | Minilang | Standard call shape (after `src opts`) |
|-----------|----------|----------------------------------------|
| `re` | regexp match | `subject:String` → `[Map]` match structure (`{ok ms fst lst n}`) |
| `re-sub` | regexp substitute | `subject:String` → `[String]`; `opts.repl` replacement, `$1…` backrefs, `opts.g` global |
| `re-test` | regexp test | `subject:String` → `[Boolean]` |
| `re-all` | regexp find-all | `subject:String` → `[List]` |
| `tr` | transliterate (Perl `tr///`) | `subject:String` → `[String]`; `opts` flags `d`/`s`/`c` |
| `jp` | JsonPath | `doc:Node` → `[Any]` |
| `jq` | jq filter | `doc:Node` → `[Any]` |
| `xp` | XPath | `doc:Any` → `[Any]` |
| `cs` | CSS selector | `doc:Any` → `[Any]` (pairs with XML.0.md) |
| `gl` | glob | `path:String` → `[Boolean]` |
| `sh` | POSIX shell pattern | `path:String` → `[Boolean]` |
| `fm` | format template | `args:Any` → `[String]` |
| `ur` | URL pattern | `url:String` → `[Any]` |
| `dt` | date/time format | `text:String` → `[Any]` |
| `math` | natural infix maths | *(generator — no stack input)* → `[Number]`; variables from `opts` |

(Names are atoms, so longer readable names are free where the
two-letter forms are cryptic — `re-sub` rather than rev 1's `rs/`.
Flags move out of slash-delimited positional syntax into `opts`:
`mini re-sub 'a+' {repl:'x', g:true}` replaces `rs/a+/x/g`. This
retires rev 1's per-prefix content-splitting and escaping rules
entirely.)

The `math` kind is the named-params showcase: the handler parses an
infix expression and evaluates it against variables supplied in
`opts` —

```
mini math 'x^2 + 3*y' {x:10, y:2}          # → 106 ✓ (prototype, fixed parse)
mini math 'x^2 + 3*y' {x:(2 add 8), y:w}   # opts values are call-site
                                            # expressions/vars — evaluated
                                            # in the generated code ✓
```

Free variables in `src` that are missing from `opts` (and, advisably,
unused `opts` keys) are loud errors. With a compile hook (§11) the
parse happens once, at expansion time.

---

## 6. Registering a minilang — native Go

Registration follows the existing native-module sub-registry pattern
(`BuildRandModule` is the model): an inner native per kind, plus an
FnDef wrapper exported under `lang_<name>`. The module centralizes
this in one helper so a kind is a single declaration:

```go
// lang/go/modules/minilang.go

// MiniLangDef declares one minilang kind.
type MiniLangDef struct {
    Name    string              // kind atom, unprefixed: "re", "math"
    Summary string              // one-line help text
    Inputs  []native.FnParam    // stack inputs AFTER the standard [src opts] prefix
    Returns []*native.Type
    Handler native.Handler      // args[0]=src, args[1]=opts, args[2..]=Inputs
}

// AddMiniLang registers def into the module's sub-registry and
// exports the wrapper as lang_<name>. Mirrors wrapRandFnDef:
// trivial-delegation body, BarrierPos -1 (CRITICAL for the swap
// form — see lang/go/CLAUDE.md "Module FnDef Wrappers").
func AddMiniLang(exports *native.OrderedMap, subReg *native.Registry, def MiniLangDef) error {
    if err := validateKindName(def.Name); err != nil { // lowercase atom, no lang_ prefix
        return err
    }
    inner := "minilang-" + def.Name
    params := append([]native.FnParam{
        {Name: "src", Type: native.TString},
        {Name: "opts", Type: native.TMap},
    }, def.Inputs...)
    subReg.RegisterNativeFunc(native.NativeFunc{
        Name: inner,
        Signatures: []native.NativeSig{{
            Args:       fnParamTypes(params),
            Returns:    def.Returns,
            BarrierPos: -1,
            Handler:    def.Handler,
        }},
    })
    exports.Set("lang_"+def.Name, wrapMiniFnDef(inner, params, def.Returns, subReg))
    return nil
}

func BuildMiniLangModule(parent *native.Registry) (native.ModuleDesc, error) {
    subReg, err := native.DefaultRegistry()
    if err != nil {
        return native.ModuleDesc{}, err
    }
    exports := native.NewOrderedMap()

    // ---- kind: re — regexp match ------------------------------------
    if err := AddMiniLang(exports, subReg, MiniLangDef{
        Name:    "re",
        Summary: "regular-expression match: subject from the stack, match map out",
        Inputs:  []native.FnParam{{Name: "subject", Type: native.TString}},
        Returns: []*native.Type{native.TMap},
        Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
            src, err := args[0].AsConcreteString()
            if err != nil {
                return nil, err
            }
            subject, err := args[2].AsConcreteString()
            if err != nil {
                return nil, err
            }
            pat, err := compiledPattern(src) // per-src memo: compile once, reuse (F-perf)
            if err != nil {
                return nil, r.AqlErrorHint("mini_parse_error",
                    fmt.Sprintf("re: %v", err), "lang_re",
                    "fix the pattern; remember '\\d' needs '\\\\d' in quoted strings (or use a backtick string)")
            }
            return []native.Value{matchResultMap(pat, subject, args[1])}, nil
        },
    }); err != nil {
        return native.ModuleDesc{}, err
    }

    // ... lang_re-sub, lang_re-test, lang_tr, lang_math, ... same shape ...

    // ---- out-of-band exports (NOT mini-callable: no lang_ prefix) ----
    exports.Set("register", wrapMiniFnDef("minilang-register", registerParams, nil, subReg)) // §7
    exports.Set("kinds", wrapMiniFnDef("minilang-kinds", nil,
        []*native.Type{native.TList}, subReg))

    return native.ModuleDesc{
        ID:      parent.Modules.NextID(),
        Exports: map[string]*native.OrderedMap{"MiniLang": exports},
    }, nil
}
```

plus the one-line table entry in `lang/go/modules/modules.go`:
`"minilang": BuildMiniLangModule`. `wrapMiniFnDef` is the standard
trivial-delegation wrapper (`Body: [Word(inner)]`, `Registry: subReg`,
`BarrierPos: -1`) — identical in shape to `wrapRandFnDef`
(`rand.go:181`).

Error contract: kind handlers return errors via
`r.AqlError`/`AqlErrorHint` with stable codes (`mini_parse_error`,
`mini_eval_error`, …) carrying `{kind, src, offset}` detail where
applicable, so `do … error […]` handlers can dispatch on `.code` and
the report points at the calling site (ERRORS.8.md §7 quality bar).

---

## 7. Registering a minilang — pure AQL

The out-of-band export `MiniLang.register` installs an AQL function
as a kind. The function must carry the standard signature prefix —
validated loudly at registration:

```
"aql:minilang" import end
"aql:string-util" import end

# a counting minilang: src is the substring sought, subject from the stack
MiniLang.register count (fn [
  [src:String opts:Map subject:String] [Integer]
  [ (StringUtil.match src subject {scope:all/q}).n ]
]) end

"banana" mini count 'a'          # → 3 ✓ (verified via the §10 prototype)
"banana" mini count 'z'          # → 0 ✓

# a generator minilang: polynomial over named params (no stack input)
MiniLang.register poly (fn [
  [src:String opts:Map] [Integer]
  [ ((opts.x pow 2) add (3 mul opts.y)) ]
]) end

mini poly 'x^2 + 3*y' {x:10, y:2}        # → 106 ✓ (prototype-proven shape)
```

`MiniLang.register` signature and contract:

```
MiniLang.register : [name:Atom/q  f:Function]  []
```

- `name` — unprefixed kind atom (`/q`: a bare word works). Rejected:
  capitalised names, names already carrying `lang_`, collisions with
  a registered kind (loud `mini_kind_exists`; re-registration of the
  *same* name is an explicit `undef`-then-register, never silent
  shadowing).
- `f` — every signature must start `[String, Map, …]`; otherwise
  `[aql/mini_bad_signature]` with the expected prefix in the hint.
- Effect: installs the function under `lang_<name>` in the `MiniLang`
  export map **wrapped as a dispatching FnDef** — see F4: a raw
  Function value placed in an export map does **not** forward-collect
  args through dot access today; only wrapper-backed exports do.
  `register` is a native word precisely so it can build that wrapper
  (the same one §6's `AddMiniLang` builds). This is also why
  registration is a word and not "set a key in a map".

Scoping follows module-binding scope: kinds registered in a module
body are visible wherever that module's import is; `undef`-style
teardown on scope exit matches existing binding rules. Two modules
registering the same kind name collide loudly at import time.

---

## 8. Errors

| Stage | Error | Notes |
|-------|-------|-------|
| expansion | `mini_unknown_lang` | kind not registered; hint suggests the import / `MiniLang.kinds`; span at the call site |
| expansion | `macro_error` (arity) | missing `src`; standard macro operand error ✓ |
| registration | `mini_bad_signature`, `mini_kind_exists`, `mini_bad_name` | §7 contract |
| runtime | `mini_parse_error` | malformed `src` for the kind's grammar, with `{kind, src, offset}` payload |
| runtime | kind-specific raises | ordinary `raise`/`Ideal/Error` values; `.code`-dispatchable; catchable with `do [...] error [...]` ✓ |

Everything is loud; there is no generic-fallback path (contrast rev
1's "unrecognized prefixes produce a generic MiniLang value", which
turned typos into values — F-rev1).

---

## 9. Static checking and discovery

Post-expansion, a mini call **is** an ordinary wrapper-word call, so
`aql check` validates per call site against the kind's standard
signature — `12345 mini re 'a+'` is a static signature error on
`lang_re`'s `subject:String`, and outputs type the surrounding
program per kind.

Two facts gate / support this (verified):

- check mode does **not** yet expand user-level `macro` definitions
  (F5) — `mini` as a *native* macro must take the native splice path,
  which the checker already follows: a `word`-spliced body containing
  a wrapper call **and the trailing `end`** checks clean with types
  flowing through ✓. So implementing `mini` natively (it must be
  native anyway, for the optional-opts arity and call-site error
  spans) delivers static checking without waiting on check-mode macro
  expansion; landing the latter then also covers AQL-prototyped
  macros generally.
- discovery: `describe mini` lists the registered kind atoms with
  each kind's effective signature (the standard call including its
  stack inputs and outputs) — `mini`'s own signature deliberately
  says nothing about stack effect, so the per-kind listing is the
  documentation. `aql describe aql:minilang` lists the exports;
  `aql describe aql:minilang:lang_re` documents one kind;
  `MiniLang.kinds` answers from code.

---

## 10. Validated by prototype (2026-06-12)

A pure-AQL prototype of the full rev 2 pipeline ran against the live
engine (built from this branch): a `mini` macro that raw-captures
`kind`/`src`/`opts`, builds the `lang_<kind>` word from the kind atom
at expansion time, and splices `<word> <src> <opts> end`; with
standard-signature kinds `lang_re` (filter — subject from stack,
backed by `StringUtil.match` literal matching) and `lang_poly`
(generator — inputs from `opts`).

Proven ✓: kind→word dispatch by name atom; subject-from-stack through
the standard signature (swap form); opts values as literals,
call-site expressions, and variables (deferred evaluation, evaluated
once in the generated code); multi-value kinds; `macroexpand`
introspection of mini calls; expansion caching; loud unknown-kind and
arity errors; auto-`end` in paren groups / fn bodies / `each` bodies /
chained mini calls; equivalence of the sugared and desugared forms;
check-mode inference through an `end`-carrying native splice.

Empirical findings the design must respect:

- **F1 — token theft.** Without the trailing `end`, the generated
  word forward-collects the *next source token* as its subject
  (silent wrong result). The auto-`end` closes it; spec rows must pin
  both directions (`"x" mini re 'p' "y"` leaves `"y"`; chained minis
  bind their own subjects).
- **F2 — argument order in generated code.** Binary non-commutative
  handlers compute `args[1] op args[0]`; a kind compiler emitting AQL
  (the `math` kind) must emit swap-form / fully-parenthesized code —
  `pow 10 2` is 2¹⁰, `10 pow 2` is 10². Every code-generating kind's
  battery needs a non-commutative case.
- **F3 — string escaping.** Both `'…'` and `"…"` process backslashes
  (`'\d+'` → `d+`): regex sources need `'\\d+'` or an
  interpolation-free backtick string (backslash-raw, verified). The
  `re` family's docs and `mini_parse_error` hint must say so.
- **F4 — raw `fn` values don't forward-dispatch as values (`afn`
  lambdas do).** With `def m {f: (fn [[a:Integer][Integer][a add 1]])}`,
  the call `m.f 5` leaves `fn (Integer)` and `5` as silent residue,
  while the same shape with `([a:Integer] => [a add 1])` dispatches
  to `6` — including through a real `module [export …]` import.
  Mechanism: `m.f 5` is the token chain `m get f 5`; `get` returns
  the raw value, which dispatches via `execFnDefLiteral`'s own-sig
  path (`engine.go:3011`). `compileFnDef` (`engine.go:3605`)
  barrier-resolves the authored sigs **but attaches a runnable
  Handler only when `fnDef.Anonymous`** (`engine.go:3619`), so a
  non-anonymous match falls through to the legacy pure-stack FnSig
  path (`engine.go:~3110`), which never reads forward tokens — the
  value stays as data and the operand pushes on top of it. The
  residue is *unnamed*, so even the end-of-run `uncalled_function`
  net (which flags named values, ERRORS.8 §5) stays silent. Named
  defs avoid the path entirely (`stepWord` → the InstallFnDef
  handler, barrier-resolved at registration), and native-module
  words avoid it too (a foreign sub-registry FnDef is not a stable
  own-sig handle — `engine.go:3040` — so the inner native is looked
  up and its registration-resolved sigs dispatch; the
  `BarrierPos: -1` wrapper rule). Both registration paths therefore
  install wrappers; `MiniLang.register` is native for this reason.
- **F5 — check-mode macro gap.** `aql check` does not expand
  user-level macros (a `def m (macro …)` use reports
  `undefined_word`). Native splice integrates today (see §9); native
  `mini` is the route to static checking now, check-mode macro
  expansion is the follow-on that generalizes it.
- **F6 — Word values have no data form; canon scope.** `canon`'s
  round-trip guarantee is scoped to **data** values (canon.tsv §1–4:
  scalars, atoms, paths, none, type literals, Node trees; identity
  values render but rebuild fresh). A raw-captured **Word** value is
  not source-expressible at all — bare `re` re-parses as an
  invocation and `re/q` as an Atom — so canon renders it as the
  deliberately non-syntax marker `word(re)` (the same rendering the
  macro.tsv goldens pin). Consequence: pure-AQL name-constructing
  macros currently have no clean route from a captured Word to its
  name (the prototype stripped the `word(…)` wrapper — a stopgap;
  `quote`/`inspect`/`convert` don't reach it). The native `mini`
  reads `WordInfo.Name` directly and never touches this.
  **Remedy (recommended, not a dependency):** extend `convert`
  (`lang/go/native/native_type.go:254`) with two Word-source sigs —

  ```go
  // Word → Atom / String: the data forms of a code identifier.
  // Completes the existing round trip: an Atom spliced via unquote
  // already becomes a Word (the gensym mechanism); this is the
  // inverse, and the Atom form canon-renders round-trippably (re/q).
  {Args: []*Type{TAtom, TWord},   TypeArgs: map[int]bool{0: true},
   Handler: convertWordHandler, Returns: []*Type{TAtom},   BarrierPos: -1},
  {Args: []*Type{TString, TWord}, TypeArgs: map[int]bool{0: true},
   Handler: convertWordHandler, Returns: []*Type{TString}, BarrierPos: -1},
  ```

  after which a pure-AQL `mini` reads cleanly
  (`def wn (convert Atom ("lang_" add (convert String kind)))`).
  `convert` is the right home (the established cross-type gateway
  with TypeArgs target dispatch); overloading `quote` would conflate
  token capture with binding reads, and `inspect` is diagnostics.
  Pair the negative: `convert Integer <word>` stays a signature
  error.

---

## 11. Phasing

| Phase | Deliverable |
|-------|-------------|
| 1 | native `mini` (two sigs: `[Atom/q String]`, `[Atom/q String Map]`; raw capture; `lang_` resolution; auto-`end`; expansion cache; call-site error spans) + `aql:minilang` module with `re` / `re-sub` / `re-test` / `re-all` (Go `regexp` backend, per-src compile memo) + `MiniLang.register` / `MiniLang.kinds` + spec battery `lang/spec/minilang.tsv` (positive/negative pairs per F1–F4) |
| 2 | `math` kind (Pratt parser; F2-aware emission), `tr`, `jp`, `fm`, `gl` |
| 3 | **compile hooks**: a kind may register an expansion-time compiler `(src, opts-form) → token list` that `mini` splices *instead of* the standard call — staged compilation of the DSL (parse once ever, splice precompiled carrier values, surface `src` syntax errors at expansion time with call-site spans; requires literal `src`). The standard call remains the semantic reference and the dynamic-src fallback |
| 4 | remaining catalogue kinds (`jq`, `xp`, `cs`, `ur`, `dt`, `sh`); optionally revisit rev 1's `xy/` literal as *pure lexer sugar desugaring to `mini`* if terseness demand materializes |

---

## Appendix A — rev 1 (superseded): `xy/` lexer literals

Rev 1 proposed two-letter whitespace-terminated lexer literals
(`rm/a+/`, `rs/foo/bar/g`, `jp/$.store.book`, …): a custom jsonic
LexMatcher, a `Scalar/MiniLang` value hierarchy carrying
prefix/content, per-prefix content parsers (slash-splitting with
`\/` escapes), and a Go-side prefix registry, with dispatch through
suffix-position signature matching.

Dropped because the `mini` macro keeps the registry idea while
deleting the delivery costs:

- **whitespace termination** forbade spaces in patterns; `src` is now
  an ordinary string literal;
- **per-prefix splitting/escaping rules** (`rs/p/r/flags`, `\/`) are
  replaced by one string plus an `opts` map;
- **two-letter prefix squat** and collision rules with two-letter
  words disappear — kinds are ordinary atoms;
- **silent generic fallback** ("unrecognized prefixes produce a
  generic MiniLang value") turned typos into values; unknown kinds
  now error at expansion time;
- **Go-only extensibility** (lexer matcher + Go registry) becomes
  registration from AQL itself (§7);
- **a new lexer matcher and value hierarchy** versus zero new
  syntax: rev 2 rides the landed macro machinery.

What rev 1 had that rev 2 gives up: terseness (`rm/a+/` vs
`mini re 'a+'`) and first-class pattern literals (`set pat rm/\d+/`).
Both are recoverable later — the literal syntax can return as pure
sugar that desugars to `mini` (Phase 4), and first-class compiled
values arrive with compile hooks (Phase 3) or per-domain modules
(e.g. a future full `aql:regexp` with a `compile → instance` API in
the `Rand.with-seed` style). Rev 1's kind catalogue is retained as
§5's standard library.
