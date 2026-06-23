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
import {
  OpCallNative,
  OpFallback,
  OpForNext,
  OpForSetup,
  OpJmp,
  OpJmpIfFalse,
  OpMakeList,
  OpMakeMap,
  OpPushConst,
  OpPushLocal,
  OpStoreLocal,
  OpTrap,
  type Program,
} from './bytecode.ts'
import { coerceBoolean } from './coretype.ts'
import { Engine } from './engine.ts'
import { AqlError } from './error.ts'
import type { Registry } from './registry.ts'
import { newInteger, newList, newMap, OrderedMap, Value } from './value.ts'

/** Active counted-loop state in the VM. */
interface VmLoop {
  cur: bigint
  end: bigint
  step: bigint
  slot: number
  exitPC: number
}

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
  const traps = p.traps
  const fallbacks = p.fallbacks
  const n = ops.length

  const stack: Value[] = new Array<Value>(Math.max(p.maxStack, 1))
  let sp = 0
  const locals: Value[] = new Array<Value>(Math.max(p.numLocals, 0))
  // Per-arity scratch buffers for handler args (reused across calls).
  const scratch: Value[][] = []
  const loops: VmLoop[] = []

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
      case OpJmpIfFalse:
        if (!coerceBoolean(stack[--sp]!)) pc = arg - 1 // loop does pc++
        break
      case OpJmp:
        pc = arg - 1 // loop does pc++ (forward, or the loop back-edge)
        break
      case OpForSetup: {
        // Stack (top-down): start, end, step.
        const start = stack[--sp]!.asInteger()
        const end = stack[--sp]!.asInteger()
        const step = stack[--sp]!.asInteger()
        if (step === 0n) throw new AqlError('for_error', 'for: step cannot be zero')
        // The matching FOR_NEXT is the next instruction; its arg is the
        // (backpatched) loop-exit pc.
        loops.push({ cur: start, end, step, slot: arg, exitPC: args[pc + 1]! })
        break
      }
      case OpForNext: {
        const lp = loops[loops.length - 1]!
        const done = lp.step >= 0n ? lp.cur >= lp.end : lp.cur <= lp.end
        if (done) {
          loops.pop()
          pc = arg - 1 // jump to loop exit
        } else {
          locals[lp.slot] = newInteger(lp.cur)
          lp.cur += lp.step
        }
        break
      }
      case OpMakeList: {
        const elems = stack.slice(sp - arg, sp) // deepest = element 0
        sp -= arg
        stack[sp++] = newList(elems, { eval: false })
        break
      }
      case OpMakeMap: {
        const keys = p.makeMaps[arg]!
        const om = new OrderedMap()
        const base = sp - keys.length
        for (let i = 0; i < keys.length; i++) om.set(keys[i]!, stack[base + i]!) // keys[0] deepest
        sp = base
        stack[sp++] = newMap(om)
        break
      }
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
      case OpTrap: {
        // A check-mode-suppressed runtime error compiled in place: raise the
        // byte-identical AqlError (the interpreter errors at this same point).
        const t = traps[arg]!
        throw new AqlError(t.code, t.detail, t.word)
      }
      case OpFallback: {
        // Interpreter island: pop nIn threaded values (top of stack = the
        // island's first input), prepend them to the recorded tokens, and
        // re-run the span through a sub-engine; push its results back. The
        // compiled code on either side keeps running.
        const fb = fallbacks[arg]!
        const nIn = fb.nIn
        const island: Value[] = []
        for (let i = sp - nIn; i < sp; i++) island.push(stack[i]!)
        sp -= nIn
        for (let i = 0; i < fb.tokens.length; i++) island.push(fb.tokens[i]!)
        const results = new Engine(registry).run(island)
        for (let i = 0; i < results.length; i++) {
          const v = results[i]!
          if (v.isWord() || v.isForward() || v.isMark() || v.isMove()) {
            throw new AqlError('internal_error', `${fb.desc}: tape-coupled island result`, fb.desc)
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
