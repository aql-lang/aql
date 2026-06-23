// The bytecode VM: executes a compiled Program, ported from the
// eng/go/vm.go run loop. Shaped for V8 JIT optimisation.
//
// V8 NOTES:
//   - The operand stack is a PREALLOCATED Value[] (capacity maxStack)
//     driven by an integer stack pointer `sp` — `stack[sp++] = v` /
//     `stack[--sp]` — never push/pop/splice, so the backing store stays
//     a packed-elements array and there is no length churn.
//   - Dispatch is a single `for` over the parallel typed-array code with
//     a `switch (op)` over a dense contiguous opcode range (jump table).
//   - CALL_NATIVE reuses a per-arity scratch array for the handler's
//     args[] (no per-dispatch allocation); safe because no
//     compiled-reachable native retains its args slice (same invariant
//     vm.go relies on).
//   - Operands are the engine's own `Value` objects (a single hidden
//     class), so the stack is a monomorphic packed array.
import { OpCallNative, OpPushConst, OpPushLocal, OpStoreLocal, type Program } from './bytecode.ts'
import { AqlError } from './error.ts'
import type { Registry } from './registry.ts'
import { Value } from './value.ts'

/**
 * Execute a compiled Program against the registry and return the residual
 * value stack (bottom → top), matching what the interpreter's run()
 * returns for the same source.
 */
export function runProgram(p: Program, registry: Registry): Value[] {
  const ops = p.ops
  const args = p.args
  const consts = p.consts
  const sigs = p.sigs
  const n = ops.length

  const stack: Value[] = new Array<Value>(Math.max(p.maxStack, 1))
  let sp = 0
  const locals: Value[] = new Array<Value>(Math.max(p.numLocals, 0))
  // Per-arity scratch buffers for handler args (reused across calls).
  const scratch: Value[][] = []

  for (let pc = 0; pc < n; pc++) {
    const op = ops[pc]!
    const arg = args[pc]!
    switch (op) {
      case OpPushConst:
        stack[sp++] = consts[arg]!
        break
      case OpPushLocal:
        stack[sp++] = locals[arg]!
        break
      case OpStoreLocal:
        locals[arg] = stack[--sp]!
        break
      case OpCallNative: {
        const sr = sigs[arg]!
        const a = sr.sig.args.length
        let as = scratch[a]
        if (as === undefined) {
          as = new Array<Value>(a)
          scratch[a] = as
        }
        // sig position 0 is the top of stack: args[i] = stack[sp-1-i].
        for (let i = 0; i < a; i++) as[i] = stack[sp - 1 - i]!
        sp -= a
        const out = sr.sig.handler(as, null, [], registry)
        if (out instanceof Promise) {
          throw new AqlError('unsupported', `async handlers are not supported in the VM`, sr.word)
        }
        const results = out as Value[]
        for (let i = 0; i < results.length; i++) {
          const v = results[i]!
          // Handlers that emit tape-coupled tokens (words/marks/moves)
          // can't be faithfully run off-tape — refuse to the caller.
          if (v.isWord() || v.isForward() || v.isMark() || v.isMove()) {
            throw new AqlError('internal_error', `${sr.word}: tape-coupled handler result`, sr.word)
          }
          stack[sp++] = v
        }
        break
      }
      default:
        throw new AqlError('internal_error', `bad opcode ${op} at pc ${pc}`)
    }
  }
  return stack.slice(0, sp)
}
