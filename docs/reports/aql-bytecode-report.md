# AQL Bytecode Compilation — Full Report

This is the expanded version of `aql-bytecode-outline.md`, written
one step at a time. This first instalment covers:

1. **Using a carrier-style checker to extract a bytecode mapping**
2. **Fixed-arity function calls as the compilation target**

Subsequent instalments will cover the instruction set, branches and
loops, user-defined functions, escape hatches, performance analysis,
and prior art.

---

## 1. Carrier-driven compilation

### 1.1 What the carrier checker already gives us

The static type-check mode described in
`aql/doc/CARRIER-STATIC-TYPECHECK-REPORT.md` replaces concrete values
on the stack with *carrier* values (`Value{Carrier:true, Data:nil}`)
and runs the same engine. Because the machinery is shared — same
`matchSignature`, same `SortSignatures`, same forward collection,
same mark/move — at every point where the runtime engine would have
called a handler, the checker instead knows:

- the **word name** at the pointer,
- the **exact signature** selected (`*Signature` from the sorted list),
- the **arg positions** on the stack, in signature order
  (`MatchResult.Positions`),
- the **declared return types** (`Signature.Returns`) or a
  `ReturnsFn`-computed carrier set,
- any **disjunct** in the input that forced a widened return.

The checker runs `execMatch` in `engine.go:807` with `CheckMode=true`
(`engine.go:865`), calls `carrierResults` instead of `match.Sig.Handler`,
and then reuses `spliceMatchResults` (`engine.go:946`) to replace the
word and its args with carriers. The stack splicing, pointer moves,
and forward resolution are **identical** to runtime.

That identity is the key. Every dispatch decision the runtime would
make has already been made, deterministically, at check time. A
compilation pass can record those decisions as it goes and emit a
bytecode stream that hard-codes each one.

### 1.2 Carrier → bytecode as a recording pass

The cleanest way to describe the compilation pass is:

> A **third mode** of the engine, `CompileMode`, runs the same Run()
> loop as the checker. At every point where the checker would resolve
> a carrier dispatch, the compiler instead appends instructions to a
> growing `[]Instr` and also pushes carriers onto its stack (to keep
> driving the checker forward).

In other words: the compiler is the checker with a recording side
effect. The carriers it pushes let subsequent dispatches resolve
normally; the instructions it emits are what will actually execute
at runtime.

Concretely, for each checker event:

| Checker event                         | Emitted bytecode                    |
|---------------------------------------|-------------------------------------|
| Push concrete literal (in check mode, stripped to carrier) | `PUSH_CONST k` — the original literal interned in a constant table |
| `matchSignature` → `Signature` with fixed `Args` and `Returns` | `CALL_NATIVE sig_id` (fixed arity known from `len(sig.Args)`) |
| `rearrangeForForward` reorders stack values | A small sequence of `SWAP`/`ROLL`/`PICK` instructions, or — better — choose operand positions at emit time so no reorder is needed |
| `RunInCheckMode` word (`def`, `fn`, `type`, `import`, `module`) | Execute during compile, update the compile-time registry, emit nothing runtime |
| `if` mark/move | `JMP_IF_FALSE else; ...; JMP end; ...` with labels resolved at emit time |
| `for` mark/move | `FOR_SETUP; ...; FOR_NEXT body_start` pattern |
| Auto-eval list literal consumed as a code body | Compile the list's token stream inline as a sub-program referenced by handle |
| `CheckFullStackFn` signature | Either compile-time expand (if the full-stack shape is decided at compile time) or emit a specialised opcode |

Because the recording is driven by the checker's actual dispatch,
forward collection, paren evaluation, and all the other
text-stream peculiarities are *already resolved* by the time
`execMatch` fires. The compiler never has to reason about source
ordering — it reasons about dispatched calls in the order they
actually execute.

### 1.3 What needs to be captured per call site

For each `execMatch` the compiler needs to know where each arg
**came from** so it can emit the right operand-sourcing instructions.
Three cases:

1. **Stack arg** — the value was already on the stack when the word
   was reached. In the bytecode this maps to a stack value already
   left there by previous instructions. No explicit operand sourcing
   is required.
2. **Forward arg** — the value was a token after the word. The
   checker already executed those tokens (their literals pushed
   carriers, their words dispatched) before `execMatch` fired, so
   by the time the call happens they are **already on the carrier
   stack in signature order** (via `rearrangeForForward`). In
   bytecode, we simply emit the forward operands' instructions
   before the `CALL_NATIVE`, and the reorder step becomes either
   a no-op or a deterministic `SWAP`/`ROLL` sequence.
3. **Literal bodies** (lists, maps, quoted atoms) — these are
   passed without evaluation. The compiler records the body as a
   nested bytecode program (or an interned value) and emits
   `PUSH_CONST body_id`.

The upshot: after the pass, the program is a **flat linear sequence
of fixed-arity operations** with no token-stream left.

### 1.4 Handling `def` and shadowing

`def` binds a name to a value (or a list body) in a per-name stack
(`DefStacks`). In the runtime engine, looking up `foo` walks its
DefStack; in the compiler, the same walk happens at compile time
because `def` is `RunInCheckMode`. So when the compiler reaches a
`Word("foo")` token and the checker resolves it to "the list body
defined N steps back", the compiler can:

- **Inline** the body's bytecode at that call site (simple and fast,
  but grows the program linearly with call sites), or
- **Emit** a `CALL_USER fn_id` to a single compiled copy, paired
  with a compile-time symbol table mapping name → fn_id.

The first option is appropriate for tiny bodies (`dup add`, etc.);
the second handles recursion and larger bodies. The carrier
checker's per-signature fixed-point analysis (Phase 3 of the
checker plan) already computes exactly what's needed to decide: if
a user function has a single concrete input carrier signature and
a single return signature, emit `CALL_USER`; if multiple
signatures survive, emit a dispatch table (see §2.4 below).

### 1.5 What the compiler cannot resolve

Two classes of site force the compiler to either widen to `Any` or
fall back to an interpreted region:

1. **Carrier disjuncts at the dispatch point.** If the checker
   arrives at `add` with inputs `Carrier<Integer|Decimal>`,
   signature matching still succeeds (the `[TNumber,TNumber]` sig
   covers both), but the concrete return depends on inputs. Two
   options: (a) emit a specialised dispatch opcode that inspects
   the top-of-stack tag at runtime and jumps to one of two
   implementations, (b) emit a single generic call and let the
   handler branch internally as it already does.
2. **Fundamentally dynamic sites.** `do` on a computed list,
   `context get x` with an unknown key, `def` rebinding inside a
   conditional branch — the checker already widens these to
   `Carrier<Any>` and flags them; the compiler emits a
   `FALLBACK_INTERP span_id` opcode that hands the relevant
   token subsequence back to the text-stream engine. This is the
   same boundary the checker already defines.

Both cases are *local*: typed regions around them still compile.
The bytecode VM and the interpreter share the `[]Value` stack
representation, so the handoff is free.

### 1.6 Summary of §1

The carrier checker is essentially a dispatch-recording machine.
Turning it into a compiler requires one new side effect per
`execMatch` — appending instructions — and one new pass at the end
— resolving labels and interning constants. Every runtime
dispatch decision becomes a compile-time decision; every
`matchSignature` call becomes a single `CALL_NATIVE sig_id`
in the bytecode.

---

## 2. Fixed-arity function calls

### 2.1 The dispatch cost today

At runtime, every word token triggers:

1. `resolveWordRef` / DefStack lookup — one map probe.
2. `matchSignature` — an outer loop over sorted candidates, an inner
   loop over arg positions, with `sigTypeMatches`, pattern unify
   calls, forward-collection state checks, barrier checks, and
   specificity scoring short-circuits (`match.go`).
3. Forward collection state updates if the word waits on future
   tokens (insert `Forward` marker; advance pointer through
   subsequent literals that feed it).
4. `rearrangeForForward` — reorder the resolved values into sig
   order.
5. `execMatch` — splice the word and its args out of the stack,
   potentially allocate a new slice, invoke the handler, splice
   results back in.

In straight-line numerical code, steps 2, 3, 4, 5 dominate. Each
one allocates small slices or maps (`skipSet := make(map[int]bool,
n+1)` at `engine.go:952`, for example). Go's escape analysis
catches some, but the overhead per token is far larger than the
actual work for a word like `add [Int,Int] → Int`, which is a
single i64 addition.

### 2.2 The fixed-arity invariant

After compilation, every `CALL_NATIVE sig_id` has:

- **Fixed N** = `len(sig.Args)`, baked into the opcode's operand.
- **Known direction of arg consumption**: popped from the stack in
  signature order. (Forward vs. stack sourcing is resolved at
  compile time; at runtime they are already on the stack.)
- **Known return shape**: `len(sig.Returns)` values pushed, typed
  per the signature.
- **No type check** on the arguments: the checker has already
  proven they match. The handler receives exactly the types it
  declared.

The runtime dispatch loop becomes:

```go
for pc < len(code) {
    op := code[pc]
    switch op.Code {
    case OpCallNative:
        sig := sigs[op.Arg]                 // O(1) index
        n   := sig.Arity                     // baked in
        args := stack[len(stack)-n:]         // no slice copy needed
        stack = stack[:len(stack)-n]
        results, err := sig.Fn(args)         // direct call
        if err != nil { return err }
        stack = append(stack, results...)
        pc++
    // ... other opcodes
    }
}
```

Five cheap operations per call vs. dozens (plus allocations) in the
interpreter. The `sig.Fn` itself does the actual work — `int64 +
int64`, `strings.ToUpper`, etc. — and for monomorphic numeric
sites the entire loop body compiles to a handful of machine
instructions.

### 2.3 Handler calling convention

Every compiled native handler takes a fixed-size slice. Because the
checker has proven the inputs, handlers can be **specialised**
per `sig_id` rather than per `NativeFunc.Name`. The current
`Handler` type is:

```go
type Handler func(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error)
```

For the compiled VM, three simplifications apply:

1. **`ctx` and `stack` become optional.** Only `FullStack`
   signatures need `stack`; the compiler emits a different opcode
   (`OpCallFullStack`) for them, so the common case drops the
   parameter.
2. **`r *Registry` stays** but is the VM's runtime registry (which
   still holds DefStacks, context, type defs).
3. **Return is `(Value, error)` or a small fixed-size array** for
   single-value returns. The overwhelmingly common case —
   arithmetic, comparison, string ops — returns exactly one
   `Value`. A specialised opcode `OpCallNative1` that returns a
   single `Value` avoids one `append` and one slice header.

A further optimisation: for words whose arity is 1 or 2 and whose
return is a single `Value`, emit `OpCallNative1_1` and
`OpCallNative2_1` opcodes. The dispatch table has three hot
shapes instead of N, and Go's `switch` can branch-predict cleanly.

### 2.4 Monomorphic vs. polymorphic sites

After the checker runs, call sites fall into three classes:

**Monomorphic.** The checker resolved a single `Signature`.
`CALL_NATIVE sig_id` — one opcode, direct handler. This is the
target case; it's where all the speed-up comes from. Expected
fraction on typed code: 80–95%.

**Polymorphic with small disjunct.** The checker couldn't rule out
2–3 signatures — for example `add` when its input is
`Carrier<Integer|Decimal>`. Two compilation strategies:

- *Split*: split the disjunct at a *previous* point (e.g. duplicate
  the branch that produced it) so each copy reaches a monomorphic
  `add`. Works well for small disjuncts produced by `if`.
- *Dispatch opcode*: `CALL_NATIVE_POLY disp_id`. `disp_id` indexes
  a small table of `(input_tag → sig_id)`. The runtime peeks the
  top-of-stack `VType`, indexes the table, calls. Still one map-free
  branch plus one indirect call.

The second strategy dominates because splitting interacts badly
with loop bodies (doubles their code size per join).

**Polymorphic with value-dependent return.** The hardest case. A
single signature matches the inputs but the return type depends on
the concrete values — `add [Number,Number]` returns `Integer` if
both inputs are integers, else `Decimal`
(`native_helpers.go:50-79`). The VM must either:

- **Re-split the signature at compile time.** Produce two sig_ids,
  one for `[Integer,Integer]→Integer` and one for `[Number,Number]→
  Decimal`, and use the dispatch opcode.
- **Leave the runtime branch inside the handler.** The handler's
  internal tag check is fast (one interface comparison) and
  preserves the existing implementation. Downstream consumers see
  `Carrier<Integer|Decimal>`, which falls into the polymorphic
  case above.

Splitting is preferable because it cascades: a monomorphic producer
of `Integer` reaches a monomorphic `add[Integer,Integer]→Integer`
consumer, and the whole chain stays monomorphic. The Phase 0 work
on `Returns` annotations should be extended to generate these
split sig_ids automatically from signatures that declare a
`ReturnsFn`.

### 2.5 Arity larger than 0/1/2

A handful of AQL words take 3+ args (list/map builders, `fn`
definitions, temporal words). These use the generic
`OpCallNative` opcode with an arity operand. They are rare enough
on hot paths (arithmetic, comparison, list folds) that a single
generic opcode suffices; no specialisation needed.

### 2.6 Stack discipline

Because arity is fixed and return count is fixed per `sig_id`, the
compiler can track the **compile-time stack depth** precisely and
allocate the runtime `[]Value` stack once at the program's maximum
observed depth, with no reallocations. This is the pattern used by
Lua 5 and Python bytecode: compute `max_stack` during emission and
pre-size the stack.

This also enables **stack-slot assignment** for `def`-bound locals:
rather than a DefStack map lookup per reference, the compiler
assigns each local a fixed slot in a local frame, and `PUSH_LOCAL
slot` is an array index. DefStack shadowing is handled by
scope-tracked slot reuse (push on entry, pop on exit).

### 2.7 What fixed-arity does NOT eliminate

Three costs remain even with fully compiled fixed-arity calls:

1. **Handler work itself.** `int64 + int64` is still executed;
   `strings.ToUpper` still scans the string. Fixed-arity removes
   dispatch overhead, not computation.
2. **Allocation inside handlers.** List and map operations still
   allocate. The compiler can help at specific sites (e.g.
   `MAKE_LIST n` with a pre-sized slice, `LIST_APPEND` for
   in-place builders) but most allocations are inherent to the
   semantics.
3. **Boxing.** AQL `Value` is a tagged union (`VType` + `Data
   interface{}`). Every int still pays the `interface{}` boxing
   cost inside `Data`. True un-boxing requires typed opcodes
   (`IADD`, `FADD`, `SADD`) with a parallel un-boxed value
   representation — a bigger change, discussed in a later
   instalment.

### 2.8 Summary of §2

Fixed-arity calls turn every monomorphic site into a one-cycle
dispatch plus a direct handler call. Forward collection and
signature matching — the two largest sources of interpreter
overhead — disappear entirely for the typed portion of the
program. Polymorphic sites survive with a small table-driven
dispatch. Value-dependent returns cascade cleanly if the Phase 0
annotations split them into monomorphic variants.

The next instalment will cover the instruction set sketch, the
encoding of branches and loops (replacing mark/move), and how
user-defined functions get compiled with their own frames.

---

## 3. Instruction set

This section sketches the instruction set. It is a proposal, not
a spec — the exact opcodes will shake out in implementation — but
it is enumerated here so the remaining sections can refer to
specific ops.

### 3.1 Design principles

- **Fixed-width, single-word instructions.** Each `Instr` is a
  small struct `{Code uint8, Arg int32}` so the program is a
  cache-friendly `[]Instr`. Complex operands (constant table
  indices, sig_ids, label offsets) fit in the 32-bit `Arg`.
- **No type tags on opcodes for monomorphic sites.** `CALL_NATIVE`
  covers every statically-resolved call; the sig_id table holds
  the arity and handler. This keeps the opcode alphabet small.
- **Specialised ops only where the shape is ubiquitous.** `PICK`,
  `ROLL`, `DUP`, `SWAP`, `DROP` already dominate Forth-style
  execution and deserve native opcodes. `OpCallNative1_1` and
  `OpCallNative2_1` (arity 1 and 2, one result) cover >90% of
  arithmetic and comparison sites.
- **Labels resolved at emit time.** Branches carry absolute PCs;
  no runtime label lookup.
- **Constant pool is flat.** A `[]Value` constants table indexed by
  `PUSH_CONST`. Interning makes literal `1`, `true`, `""` share
  slots across the program.

### 3.2 Opcodes

**Stack manipulation** (direct Forth heritage):

- `PUSH_CONST k` — push constants[k].
- `DUP`, `DROP`, `SWAP`, `OVER`, `NIP`, `TUCK`, `ROT`
- `PICK n`, `ROLL n` — with `n` from the arg field.
- `2DUP`, `2DROP`, `2SWAP`, `2OVER`

**Locals** (for compiled `def` bindings):

- `PUSH_LOCAL slot` — push frame[slot].
- `STORE_LOCAL slot` — pop and store into frame[slot].
- `ENTER_SCOPE n`, `EXIT_SCOPE n` — reserve / release n slots.

**Calls**:

- `CALL_NATIVE sig_id` — arity in sig table.
- `CALL_NATIVE1_1 sig_id` — 1-in, 1-out fast path.
- `CALL_NATIVE2_1 sig_id` — 2-in, 1-out fast path.
- `CALL_NATIVE_FULL sig_id` — for `FullStack` signatures (`depth`,
  `pick`, `roll`, `stack`). Handler receives the full stack view.
- `CALL_NATIVE_POLY disp_id` — small table-driven dispatch for
  residual disjuncts (§2.4).
- `CALL_USER fn_id` — user-defined fn; arity in fn table.
- `RET` — return from user fn.

**Control flow**:

- `JMP pc`
- `JMP_IF_FALSE pc` — pop top, jump if falsy.
- `JMP_IF_TRUE pc`

**Loop support** (replacing mark/move for `for`, `each`, `fold`,
`scan`):

- `FOR_SETUP` — pop range spec, push iterator state record.
- `FOR_NEXT body_pc end_pc` — if iterator exhausted, jump to
  end_pc; else bind iterator value (into a known local slot) and
  jump to body_pc.
- `EACH_SETUP` / `EACH_NEXT` — iterate a list.
- `FOLD_SETUP` / `FOLD_NEXT` — iterate a list with an accumulator
  slot.

**Structure builders**:

- `MAKE_LIST n` — pop n values, push a list.
- `MAKE_MAP n` — pop 2n values (alternating keys/values), push
  a map.
- `MAKE_TYPED_LIST t_id n` — same, but with element type fixed.
- `LIST_APPEND` — in-place append for fold-style builders.

**Type ops**:

- `TYPE_TAG` — push the VType of top-of-stack as a type literal.
- `TYPE_CHECK t_id` — assert top-of-stack matches type; emit only
  at disjunct narrowing points, raise a typed error on mismatch.
- `TYPE_COERCE t_id` — narrow at a checked boundary (e.g. after
  a guard `if [x is Integer]`).

**Def / registry plumbing** (rare at runtime, used for runtime
`def` rebinding):

- `REG_DEF_PUSH name_id, sig_id` — push a compiled fn onto a
  DefStack at runtime.
- `REG_DEF_POP name_id` — remove the top binding.

**Fallback**:

- `FALLBACK_INTERP span_id` — resume interpretation over a
  recorded token span (§1.5). The VM hands the current stack to
  an engine instance, lets it run, and resumes with the resulting
  stack.

**Halt / errors**:

- `RETURN_TOP` — program's top-level result.
- `HALT`
- `RAISE err_id` — raise a structured error with interned
  message.

That's ~35 opcodes. A 6-bit opcode field with a 26-bit operand
would leave plenty of room; a byte-wide opcode with a 32-bit
operand fits Go's `struct{Code uint8; Arg int32}` naturally.

### 3.3 Encoding and layout

The program is a value `Program` with:

```go
type Program struct {
    Code      []Instr            // flat instruction stream
    Constants []Value            // interned literals
    Sigs      []*Signature       // sig_id → signature
    PolyDisp  []PolyDispTable    // disp_id → dispatch table
    UserFns   []CompiledFn       // fn_id → compiled fn
    DebugInfo []SrcSpan          // pc → source span (optional)
    MaxStack  int                // pre-size the runtime stack
    MaxLocals int                // pre-size the locals frame
}
```

`CompiledFn` has its own `Code`, `MaxStack`, `MaxLocals`, and
param/return arity. User-fn call semantics are a classic frame
push: push return PC, push locals frame pointer, set `pc = fn.Code`
start. `RET` unwinds.

### 3.4 Example lowering

Source:

```
def square fn [[Integer] [Integer] [dup mul]]
5 square
```

After compile:

```
; User fn registration runs at compile time (RunInCheckMode).
; The body is compiled to CompiledFn{Code: [DUP, CALL_NATIVE2_1 mul_i_i, RET], ...}
; At runtime, only the body below executes.

PUSH_CONST   0          ; constants[0] = Integer(5)
CALL_USER    square_fn
RETURN_TOP
```

The `square` call site is a single `CALL_USER`. `dup mul` inside
the body is two instructions. Comparable interpreter execution is
roughly: parse `5` → push; resolve `square` → DefStack lookup →
splice body tokens → step through `dup` → matchSignature → splice
→ step through `mul` → matchSignature → splice. The bytecode
path is 3 ops total.

---

## 4. Replacing mark/move with static branches

Mark/move is the runtime mechanism AQL uses for `if`, `for`, and
similar words that splice new tokens onto the stack for later
evaluation. `if`'s 3-arg handler (`conditional.go:61-85`) returns
a token sequence `[Mark, condTokens..., MoveIf{Then, Else}]`;
the engine loop processes the mark, runs the condition tokens,
hits the move, evaluates the result, and splices one of the
branches in place. This is elegant for an interpreter — the main
loop stays oblivious to control flow — but it relies on mutating
the token stream, which a bytecode VM cannot do cheaply.

Every mark/move site can be statically lowered because the
compile-time checker already knows which branches exist, what
their bodies are, and when they rejoin.

### 4.1 `if` → conditional branch

Source:

```
if [x lt 10] [x 2 mul] [x 3 mul]
```

Compile:

```
PUSH_LOCAL   x_slot
PUSH_CONST   10                ; 10
CALL_NATIVE2_1 lt_i_i          ; x < 10
JMP_IF_FALSE else_label

; then branch
PUSH_LOCAL   x_slot
PUSH_CONST   2
CALL_NATIVE2_1 mul_i_i
JMP          end_label

else_label:
PUSH_LOCAL   x_slot
PUSH_CONST   3
CALL_NATIVE2_1 mul_i_i

end_label:
```

Key points:

- **The condition is just code.** Any list body used as an `if`
  condition becomes a sequence of instructions that leaves one
  value on the stack. The condition list's `NoEvalArgs` flag at
  the interpreter becomes implicit in the bytecode lowering —
  the body's tokens are simply emitted here.
- **The result type is a disjunct** of the two branches' top-of-
  stack carrier types, unified by the checker. If the checker
  widens to `Any`, the compiler may emit a `TYPE_TAG` at the end
  for downstream dispatchers, but monomorphic joins need no
  extra op.
- **2-arg `if` with no else** emits `JMP_IF_FALSE end_label`
  directly with no `JMP` between branches. If the checker
  detects that the then-branch produces a value the enclosing
  context consumes, it must emit a `PUSH_CONST none` on the false
  path to preserve stack arity.

### 4.2 Condition body with side effects

The condition list may itself contain calls (`if [x is Integer
and [y gt 0]] ...`). These compile straight: the condition's
instruction sequence runs, leaves a boolean on top, `JMP_IF_FALSE`
consumes it. No mark/move bookkeeping is needed because the
condition's own bytecode already leaves the stack in the expected
shape — the compiler verifies this against the carrier stack at
emission time.

### 4.3 `for` → counted loop

Source:

```
for 10 [i print]
```

Compile:

```
PUSH_CONST   10                 ; n
FOR_SETUP                       ; pops n, pushes iterator state; reserves i_slot
body_label:
FOR_NEXT     end_label          ; if exhausted → end_label; else store i into i_slot
PUSH_LOCAL   i_slot
CALL_NATIVE1_0 print_any        ; side-effect print; 0 returns
JMP          body_label
end_label:
; iterator state popped by FOR_NEXT when it exits; for-result list
; is synthesised by FOR_SETUP/FOR_NEXT (they maintain a hidden accum).
```

A few notes:

- **Iterator state is a hidden slot.** `FOR_SETUP` allocates an
  iteration record in the locals frame. `FOR_NEXT` increments
  and checks. The iterator variable `i` is a named local in the
  scope — the compiler knows its slot from the `for` signature's
  convention (`ReturnsFn` already binds the iterator name `"i"`
  during the carrier pass, `forloop.go:60`).
- **`break` and `continue`** become `JMP end_label` and
  `JMP body_label` respectively. Their current sentinel-error
  mechanism in the runtime engine is unnecessary in the bytecode
  VM.
- **Range variants** (`for [1,10]`, `for [0,10,2]`) compile to
  the same opcodes; only the initial values on the stack differ.
  `FOR_SETUP` pops whatever the range descriptor is and
  normalises to a `{start, end, step}` record.
- **The per-iteration accumulator list.** `for` produces a list of
  per-iteration top-of-stack values. The compiler emits a
  hidden `LIST_APPEND` into an accumulator slot at each
  iteration end. `FOR_NEXT`'s exit path leaves the accumulator
  on the runtime stack. The checker's carrier already types this
  as a typed-list whose element type is the body's top-of-stack
  — that type becomes the `MAKE_TYPED_LIST`'s element parameter.

### 4.4 `each` / `fold` / `scan`

These higher-order words take a list and a code body. Because
the body is a literal list at most call sites (the checker
already flags non-literal bodies as hard-to-type), the compiler
can inline the body and emit a small loop:

```
each [l] [body]   →
  PUSH_CONST   l_id            ; the list
  EACH_SETUP                   ; pops list, sets up iter + accum
body_label:
  EACH_NEXT    end_label       ; store element into elem_slot; exit if done
  ; inline body: pushes 0+ results; the last N values (from sig) are accum
  ...body bytecode...
  LIST_APPEND                  ; accum.append(top)
  JMP          body_label
end_label:
```

`fold` differs only in that its accumulator is user-visible and
initialised from an earlier stack value:

```
fold [[l] [init] [body]]   →
  ...init bytecode → leaves init on stack...
  STORE_LOCAL accum_slot
  PUSH_CONST  l_id
  FOLD_SETUP
body_label:
  FOLD_NEXT   end_label        ; exits when list exhausted
  PUSH_LOCAL  accum_slot
  PUSH_LOCAL  elem_slot
  ...body bytecode...          ; pops two, pushes one new accum
  STORE_LOCAL accum_slot
  JMP         body_label
end_label:
  PUSH_LOCAL  accum_slot
```

The checker already computes the body's effect as a function
from `(accum_type, elem_type) → accum_type'`, iterating to a
fixed point (`CARRIER-STATIC-TYPECHECK-REPORT.md` §"Loop and
recursion termination"). At compile time the same fixed-point
iteration tells us what `accum_slot`'s type is, which in turn
drives any type tagging the VM needs.

### 4.5 Mark/move → no runtime artefact

Under bytecode compilation the following mechanisms **disappear
entirely** from runtime:

- `NewMark`, `NewMoveIf`, `NewMoveFor`, `stepMark`, `stepMove`
- `ForCont`, `IfCont` continuation records
- The `NextMarkID` counter
- `handleLoopBreak` / `handleLoopContinue` (replaced by static
  `JMP` targets)
- Body token splicing via `spliceArg` (the body is already
  bytecode)

The interpreter retains all of these for the interpreted mode,
but the compiled code never touches them. This removes a
significant slice of per-step engine overhead on branch- or
loop-heavy code.

### 4.6 What survives: error handling and errors with source

Bytecode branches change what a runtime error looks like.
Currently, errors pinpoint a token by its `Pos` field. In the
VM, errors pinpoint a `pc`. The `DebugInfo []SrcSpan` table
maps pc → source span for exactly this reason. When
`CALL_NATIVE` returns an error, the VM wraps it with
`DebugInfo[pc]`, giving error messages identical in precision
to the interpreter's.

### 4.7 Summary of §§3–4

The instruction set is small (~35 opcodes) and dominated by
stack manipulation, fixed-arity calls, and conditional branches.
Mark/move — AQL's most interpreter-flavoured mechanism — lowers
cleanly to static branches and loops because the carrier
checker has already resolved every branch and every iteration
shape. `break`/`continue` become `JMP`. Higher-order words with
literal bodies become inline loops with an accumulator slot.

The next instalment will cover user-defined functions (frame
discipline, multiple-signature dispatch, recursion), the
register / constants layout, and how the compiler interacts
with the `RunInCheckMode` words that already run during the
carrier pass.

---

## 5. User-defined functions

User-defined functions in AQL come from two constructs: `def` —
installing a literal-substitution body or a fn value under a name
— and `fn` — building a multi-signature function value. Both are
`RunInCheckMode` (see `native_definition_def.go:64,72` and
`native_definition_fn.go:59`), which means the carrier checker
actually executes them at check time, mutating the registry's
`DefStacks`. This is exactly the behaviour we want for
compilation: the compiler shares the same registry and so
already knows every binding by the time it reaches a call site.

### 5.1 What `def` produces

`def name body` has two shapes that matter for compilation:

1. **Literal substitution.** `def pi 3.14159` or `def doubled [dup
   add]`. The body is a value (scalar or list). At every call
   site of `pi` the interpreter splices the body's tokens onto
   the stack. There is no separate arity or signature — the body
   is just code.
2. **Named `fn` value.** `def square fn [[Integer] [Integer] [dup
   mul]]`. The body is a `FnDefInfo` with a list of signature
   triples `[input, output, body]`. Each triple has its own
   input types, output types, and body tokens.

Both shapes get lowered differently by the compiler.

### 5.2 Compiling a literal-substitution `def`

For `def pi 3.14159`:

- The compiler's registry pass (running under `RunInCheckMode`)
  installs the binding.
- The compiler records `name → Value` in its *inlining table*.
- At each `Word("pi")` reference, the compiler emits
  `PUSH_CONST const_pi`. No call, no dispatch.

For `def doubled [dup add]`:

- Same registry install.
- The inlining table records `name → []Instr` by compiling the
  body once as a standalone instruction sequence.
- At each `Word("doubled")` reference, the compiler chooses
  between **inlining** (paste the instructions directly, cheap
  if the body is small) and **emitting a CALL_USER** to a single
  compiled copy (better for large bodies or recursive defs).

A reasonable heuristic: inline if the body is ≤8 instructions
and non-recursive; otherwise promote to a `CompiledFn`. This
mirrors how modern JITs decide inlining, but the threshold is
small because AQL's literal defs are usually terse.

### 5.3 Compiling a `fn` value

`fn [[I] [I] [dup mul]]` is a multi-signature function. For
each triple, the checker already runs a carrier analysis per
signature. For the compiler this means:

- Each triple produces its own `CompiledFn` — same body bytecode
  across signatures if they share one body, specialised if the
  body references input-typed locals that would differ.
- The overall fn value becomes a `CompiledFnSet` — a small
  dispatch table keyed by input type.

The compiled shape of `def square fn [[I] [I] [dup mul]]`:

```
CompiledFnSet "square" {
  sigs: [
    { Args: [Integer], Returns: [Integer], Code: [DUP, CALL_NATIVE2_1 mul_i_i, RET] },
  ],
  argCount: 1,
}
```

At a call site, `CALL_USER square_id` resolves to this set. If
the set has exactly one signature — the common case — the call
is direct: push frame, jump to the body. If multiple signatures
exist, the VM needs a `CALL_USER_POLY` variant that inspects the
argument's VType and indexes into the set.

### 5.4 Multi-signature dispatch at compile time

The carrier checker resolves multi-signature `fn` values
deterministically: it orders signatures by specificity
(`SortSignatures`), then applies the same first-match rule as
runtime. So at compile time, every `CALL_USER` site has a
definite target signature — *unless* the caller's carrier
includes a disjunct that straddles two signatures.

Example where the disjunct stays:

```
def f fn [
  [Integer] [Integer] [dup mul]       ; sig 0
  [Decimal] [Decimal] [dup mul]       ; sig 1
]
if [cond] [1 f] [1.0 f]
```

Each branch of the `if` is monomorphic (`f[Integer]` vs.
`f[Decimal]`), so if the checker propagates the disjunct to the
join, the downstream site sees `Integer|Decimal`. But *within
each branch*, the compiler emits a direct `CALL_USER f_sig0` or
`CALL_USER f_sig1`. No runtime dispatch at the call point.

Where the disjunct *is* the input — e.g. `f x` where `x :
Integer|Decimal` flowing from a prior `get` — the compiler emits
`CALL_USER_POLY f_set_id`, and the VM switches on the top-of-
stack type tag. Still O(1); still no signature scan.

### 5.5 Frames, locals, and named parameters

When a `fn` signature declares named parameters
(`fn [[x:Integer] [Integer] [x mul x]]`), the parser installs
each name as a `def` inside the body's scope. The carrier
checker already models this: it pushes a `DefCleanup` marker on
entry and pops bindings on exit.

The bytecode VM handles this with a classic **call frame**:

```
Frame {
    returnPC  int
    localsPtr int        // index into a flat locals stack
    prevFrame *Frame
}
```

Each `CompiledFn` declares its `MaxLocals`. `CALL_USER`:

1. Pops N args off the operand stack.
2. Pushes a new frame; reserves `MaxLocals` slots.
3. Stores args into the first N local slots (in signature
   order).
4. Sets `pc = fn.Code` start.

`RET` unwinds: pops the frame, frees locals, pushes the function's
declared results onto the caller's operand stack from the
callee's top-of-stack, and restores `pc`.

Named parameters (`x`) become `PUSH_LOCAL x_slot` references.
This is one array index vs. a DefStack map probe per reference —
a significant win for parameter-heavy bodies.

### 5.6 Recursion

Recursion is trivial in this model because user fns are
addressable by `fn_id`. A recursive `factorial`:

```
def factorial fn [
  [Integer] [Integer]
  [dup 1 lte [drop 1] [dup 1 sub factorial mul] if]
]
```

Compiles to a `CompiledFn` whose body contains `CALL_USER
factorial_id` — a direct reference to itself. The checker's
fixed-point analysis has already computed the return type;
registration happens once, before the body is lowered, so the
self-reference resolves.

What the bytecode VM does *not* need to do for recursion:

- No tail-call optimisation in v1 — user calls push a frame; very
  deep recursion hits a stack limit. (Forth and Lua take the
  same approach; TCO can be added later by detecting a `RET` that
  immediately follows a `CALL_USER` and emitting `TAIL_CALL_USER`
  instead.)
- No trampolining — the operand stack and frame stack are
  separate; frames are small (a return PC, a locals pointer, and
  a parent pointer).

### 5.7 Polymorphic recursion

`factorial` above is monomorphic (Integer → Integer). A
polymorphic fn, e.g. a generic `length` over lists of varying
element types, needs either:

- **Per-instantiation specialisation.** Each observed input type
  produces its own `CompiledFn`. Works well because typed lists
  only have a handful of concrete element types per program.
- **A single generic implementation.** Carry element type as a
  runtime operand and dispatch inside. Loses some of the
  fixed-arity benefit.

The first strategy is cleaner and aligns with how the carrier
checker already operates (it analyses each call-site input
separately). The number of specialisations is bounded by the
checker's disjunct widening, so code blow-up is controlled.

### 5.8 Summary of §5

User fns compile cleanly because `def`/`fn` are already
`RunInCheckMode`: the compiler's checker pass registers them in
the shared registry, so the compile-side CALL emitter sees every
binding. Literal defs inline or promote to `CompiledFn`. Multi-
signature fns become `CompiledFnSet` dispatch tables, with the
vast majority of call sites resolving to a single signature at
compile time. Named parameters become slot indices. Recursion is
a plain `CALL_USER` to the same `fn_id`.

---

## 6. Interaction with `RunInCheckMode` words

`RunInCheckMode` is the escape valve that lets certain words
execute their side effects during the carrier pass so subsequent
analysis has complete information. The list today:

- `def` — installs a DefStack binding.
- `undef` — removes one.
- `fn` — builds a function value (and installs it when paired
  with `def`).
- `type` / `typedef` — adds a type to the registry.
- `import` / `export` — cross-module linkage.
- `module` — creates a sub-engine with an isolated registry.
- `var` — installs a context variable.
- Possibly `record`, `table`, `object`, `resource` — schema
  installers.

Every one of these is a **compile-time-only** operation from the
bytecode VM's perspective. They manipulate the registry that
drives dispatch; by the time the bytecode is running, the
registry is frozen (or at most changed by plain runtime-visible
`def`/`undef` pairs — see §6.4).

### 6.1 Compile-time execution

During the compile pass (which is the checker pass with a
recording side effect), every `RunInCheckMode` signature runs
normally. That is:

- `def foo 1` installs `foo → Value(1)` in the registry.
- `def doubled [dup add]` installs the body.
- `fn [[I] [I] [dup mul]]` builds and returns an `FnDefInfo`.
- `type Point record [x:Number, y:Number]` adds a record type.
- `import utils` loads and analyses the module.

The compiler sees the result on the carrier stack (where
applicable) and records whatever it needs into its own tables —
inlining tables, user-fn tables, type tables, import graphs.
**No runtime bytecode is emitted for these operations** in the
monomorphic common case.

### 6.2 Module imports

`import utils` compiles the imported module once. The compiler
maintains an import cache (already present for the checker) and
merges the module's exported fn_ids, constants, and type ids
into the current program's tables with a namespace prefix.

Cross-module `CALL_USER utils.helper_id` is just another entry
in the `UserFns` table; dispatch cost is identical to in-module
calls. Modules with cyclic imports need the same fixed-point
treatment as recursive fns — register stubs for declarations
first, then compile bodies.

### 6.3 `type`, `record`, `table`

Type installers produce tables entries at compile time. Runtime
type checks (`TYPE_CHECK t_id`) reference these tables. The
bytecode program carries its own `Types []Type` table so
deployed bytecode is self-contained — you don't need the
original source to know what `Point` is.

### 6.4 Runtime `def`

AQL allows a `def` inside a loop body or conditional branch:

```
for 10 [i dup def current_i ...]
```

This is a runtime mutation of the DefStack. Under compilation
there are two sensible strategies:

1. **Promote to a local.** If the checker can prove the binding
   is used only within the enclosing scope, compile `def x body`
   to `STORE_LOCAL x_slot`. This is the overwhelmingly common
   case and should be the default.
2. **Emit registry ops.** For `def` that leaks to enclosing
   scopes (possible via `each`/`fold` sub-engines sharing the
   registry), emit `REG_DEF_PUSH name_id` / `REG_DEF_POP
   name_id`. The VM mutates the runtime registry on the fly.
   Slower, but semantically identical to the interpreter.

The compiler picks strategy 1 by default; the carrier checker's
def-binding state tracks which names are scope-local. Only
`def`s that the checker had to treat as producing disjuncts
across scopes fall back to strategy 2.

### 6.5 `do` and the dynamic escape hatch

`do body` evaluates `body` as code. If `body` is a literal list,
the compiler lowers it inline (it is just another code fragment).
If `body` is computed — `ops at 0 do` — the compiler cannot
know what code it names. The checker already widens these to
`Carrier<Any>` and logs a diagnostic; the compiler emits
`FALLBACK_INTERP span_id` and a recorded token span, and the
runtime hands off to an engine instance. The engine runs in
pure interpreter mode over the computed list, then returns
control. Expensive, but only at the genuinely dynamic sites.

A useful property: because both modes share the same `[]Value`
stack representation, the handoff is just passing the current
stack slice. No marshalling.

### 6.6 Registry freezing at runtime

Once compilation is complete, the runtime registry is frozen
for everything the compiled program will resolve statically:
fn_ids are fixed, sig_ids are fixed, constant pool is fixed.
The only runtime-mutable part is DefStacks (for runtime
`def`/`undef`), context variables, and module load state.

This enables a further optimisation: the VM can memoise parts
of dispatch (e.g., type tag → sig_id in a `CALL_NATIVE_POLY`
table) because the mapping is known to be stable for the run.

### 6.7 Summary of §6

`RunInCheckMode` words are compile-time primitives. The
compiler executes them during the carrier pass, captures their
effects in compile-time tables, and emits nothing (or almost
nothing) in the runtime bytecode. Runtime `def`/`undef`
promote to locals where possible, falling back to registry
mutation ops otherwise. `do` on computed code is the only
construct that forces a true interpreter fallback — the same
boundary the carrier checker already draws.

The next instalment will cover performance analysis — expected
speed-ups per construct, benchmarking methodology, memory
behaviour, and the specific costs that fixed-arity cannot
eliminate (allocation, boxing, I/O).
