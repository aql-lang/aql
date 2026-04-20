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
