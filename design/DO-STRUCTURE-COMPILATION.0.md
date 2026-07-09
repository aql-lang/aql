# Compilation of `do [ … ]` structures

Investigation of how the AQL bytecode compiler lowers the `do` word — the
error-trapping "evaluate this body as code" construct. Covers the two `do`
signatures (`do [List]` and `do {Map}`), every distinct compile strategy the
recorder picks, why each is chosen, and a differential-parity check of
compiled vs. interpreted execution.

All disassembly below is reproducible with:

```bash
cd cmd/go && go build -o bin/aql ./aql
./bin/aql check --emit -e 'do [1 add 2]'      # show the bytecode + site report
./bin/aql run  -force-compile -e 'do [1 add 2]'  # require the VM path (no fallback)
```

## 1. What `do` is

`do` has two signatures (`lang/go/native/native_control.go`):

| Sig | Shape | Semantics |
|-----|-------|-----------|
| `[List]` | `do [body]` | Runs the body's tokens with **no per-call inputs**, **traps any error** and surfaces it as an `Error` **value**, and returns the body's **entire residual stack**. |
| `[Map]` | `do {m}` | Recursively evaluates the map's values as code, returns the rebuilt map. |

The `[List]` sig is the interesting one for compilation. It declares:

- `NoEvalArgs: {0: true}` — the body list is **not** auto-evaluated as an
  argument; it is code held as data.
- `Callable: &CallableSpec{BodyPos: 0, BodyOut: 1, Inputs: () => []}` — it is a
  code-body higher-order word whose body takes 0 inputs and nets 1 result. This
  is what makes the body **closure-compilable**.
- `CompileEffect: CompileFallbackBody` — if the closure can't be built, the
  construct may still compile as a Stage-5 interpreter island rather than
  forcing a whole-program interpreter fallback.

The runtime handler (`doListHandler`) is a single seam:

```go
result, err := InvokeBody(r, args[0], nil)   // eng/go/invoke.go
if err != nil { return []Value{NewError(err)}, nil }  // trap → Error value
return result, nil
```

`InvokeBody` is the interpreter/VM bridge: when `r.Invoker` is set (the VM is
running) the body runs re-entrantly on the VM; when it is nil a pooled
sub-engine runs the reconstructed token stream. Same handler, same result
either way — this is why `do` has one code path across both engines.

## 2. The five compile strategies (observed)

The recorder picks a lowering per call site. `check --emit` reveals which:

### (a) Single-value literal body → **closure** (`PUSH_CLOSURE` + `CALL_NATIVE`)

```
do [1 add 2]
0000 PUSH_CLOSURE f0   ; closure do$body/0
0001 CALL_NATIVE  s0   ; do (List)
fn f0 do$body/0 (locals=0):
0000 PUSH_CONST k1 ; 1
0001 PUSH_CONST k0 ; 2
0002 CALL_NATIVE s1 ; add (Number, Number)
0003 RET
```

The body compiles to its own fn unit `do$body/0`. The dispatch pushes it as a
closure value; the `do (List)` handler invokes it through `InvokeBody → r.Invoker
→ vmContext.invokeClosureOn`, which sees a `ClosurePayload` and runs the unit
directly on the VM — no interpreter sub-engine. Body ends with `RET`
(`callable_words.go::compileClosureBody`).

### (b) Diverging body → **closure with NO `RET`** (structured exception handling)

```
do [raise oops "boom"]        do [1 div 0]
0000 PUSH_CLOSURE f0          0000 PUSH_CLOSURE f0
0001 CALL_NATIVE  s0 ; do     0001 CALL_NATIVE  s0 ; do
fn f0 do$body/0:              fn f0 do$body/0:
0000 PUSH_CONST 'boom'        0000 PUSH_CONST 1
0001 PUSH_CONST oops/q        0001 PUSH_CONST 0
0002 CALL_NATIVE raise        0002 CALL_NATIVE div (Number, Number)
   (no RET — diverges)           (no RET — diverges)
```

`raise` carries `CompileDiverges` (always raises); `div`/`mod`-by-static-zero
carries `CompileValueDiverges` (raises for that operand shape). The recorder
treats a body ending in a divergent terminal as producing no value, so the
closure compiles with **no `RET`** — the raised error propagates out of the VM
and the enclosing `do` handler catches it via `InvokeBody`'s `err` return,
turning it into an `Error` value exactly as the interpreter does. This is the
bytecode form of structured exception handling: a trapping body is *catchable*
rather than uncompilable. (`eng/go/value.go` `CompileDiverges` /
`CompileValueDiverges` docs.)

### (c) Multi-value / empty inert body → **baked const list** (`PUSH_CONST` + `CALL_NATIVE`)

```
do [10 20 30]                 do []
0000 PUSH_CONST k0 ; [10 20 30]  0000 PUSH_CONST k0 ; []
0001 CALL_NATIVE s0 ; do      0001 CALL_NATIVE s0 ; do
```

The closure path requires a **single** output (`BodyOut: 1`); a 3-value body
nets 3 and declines it. But because `[10 20 30]` (and `[]`) is **fully inert
const data** (no words to dispatch), the body bakes as a plain const list and
the `do (List)` handler runs it. At VM time `invokeClosureOn` sees the operand is
a `ListPayload`, **not** a `ClosurePayload`, and falls to
`RunResolved(reg, nil, bodyTokens(body))` — the same pooled-sub-engine run the
no-Invoker interpreter branch takes. Byte-identical either way. (This is the
`noEvalBodiesInert` bake in `carrier.go`.)

### (d) Map body → **poly** (`CALL_NATIVE_POLY`)

```
do {a:(1 add 2)}
0000 PUSH_CONST      k0 ; {a:3}
0001 CALL_NATIVE_POLY p0 ; do/1 (poly)
```

The `do {Map}` sig auto-evaluates its map values *before* the handler runs, so
the operand arrives concrete and the static result is `dynamic(Any)`. It lowers
to `CALL_NATIVE_POLY`, which re-runs the kernel's own `MatchSignature` at run
time (`vmContext.callPolyIn`) — the same first-match the interpreter takes.

### (e) Genuinely uncompilable body → **whole-program interpreter fallback**

If the body neither compiles to a closure nor bakes as an inert const nor
islands (e.g. it splices tape-coupled tokens, or a computed body carrier the
checker can't materialise), the whole program falls back to the interpreter
silently under `--compile`, or aborts with the refusal reason under
`--force-compile` (`RunCompiledStrict`, `lang/go/aql.go`). None of the common
`do` forms hit this — the closure and inert-const paths cover them.

## 3. Static typing of the body (`doListReturnsFn`)

The check-mode return function models the residual so downstream consumers type
correctly:

- Non-empty body that runs to an **empty** residual (`do [raise …]`,
  `do [1 div 0]`) → typed as `Error` carrier, so `do [raise …] dot code` type-checks.
- **Empty** body (`do []`) → stays empty (it did *not* raise); inventing an
  `Error` there would wrongly admit `do [] convert Map`.
- Multi-value literal body → returns the **full** residual stack (arity matches
  the runtime), which also declines the single-output closure/island paths.
- Computed (carrier) body → bounded `dynamic(Any)` escape hatch, and during a
  compile pass keeps refusing to lower (checker precision must not imply compile
  coverage — pinned by `lang/go/code_effect_test.go`).

Because `do` traps errors, `doListReturnsFn` raises `CheckState.CaughtBodyDepth`
around the body analysis so guaranteed-runtime-error *mirrors* inside the body
are downgraded to info rather than reported as program errors (see
`eng/go/CLAUDE.md` "Check-mode guaranteed-error mirrors").

## 4. Differential parity check

`aql run -no-compile` vs. `aql run -force-compile` agree on every form tried
(20+ cases), including nested `do`, `do` over higher-order bodies, `do` bodies
that reference module-level defs, `do` inside a fn body capturing an enclosing
param, and every divergence/trap case:

| Program | Result (both engines) | Strategy |
|---|---|---|
| `do [1 add 2]` | `3` | closure |
| `do [10 20 30]` | `10 20 30` | baked const |
| `do []` | *(empty)* | baked const |
| `do [raise oops "boom"]` | `error(boom)` | closure, no RET |
| `do [1 div 0]` | `error(division by zero)` | closure, no RET |
| `do {a:(1 add 2) b:5}` | `{a:3 b:5}` | poly |
| `do [do [1 add 2] add 10]` | `13` | nested closures |
| `do [if (1 gt 0) [42] [0]]` | `42` | closure |
| `def x 5  do [x add 1]` | `6` | closure (module-scope read) |
| `do [each [mul 2] [1 2 3]]` | `[2 4 6]` | closure over a nested closure |
| `def bump fn [[x:Integer] [Integer] [do [x add 100]]]  bump 5` | `105` | closure **capturing** enclosing param `x` |
| `do [do [raise x "e"]]` | `error(e)` | inner traps → Error, outer returns it |

The capture case disassembles to a `do$body/1` unit with `[x]` local that the
enclosing `bump/1` frame supplies via `PUSH_LOCAL` before `PUSH_CLOSURE` —
lexical capture flows through the closure boundary correctly.

## 5. Summary

`do [ … ]` compilation is **sound and complete over its common forms**, with the
recorder choosing among four native strategies (single-value closure, diverging
closure with no RET, baked inert-const list, and Map poly) and falling back to
the interpreter only for genuinely irreducible bodies. The single
`InvokeBody`/`doListHandler` seam is what keeps the trap-and-return semantics
byte-identical across the interpreter and the VM. No defects were found during
this investigation.
