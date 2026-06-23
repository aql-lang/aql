// The compiled-program data model for the AQL bytecode VM, ported from
// eng/go/bytecode.go and shaped for V8 JIT optimisation.
//
// V8 NOTES: the instruction stream is a pair of PARALLEL TYPED ARRAYS —
// `ops` (one dense byte per instruction) and `args` (one int32 per
// instruction) — NOT an array of {op,arg} objects. `ops[pc]` is then a
// single byte load returning a SMI, with no per-instruction heap object,
// no GC walk, and no megamorphic property access in the dispatch loop.
// Opcodes are a `const enum` with CONTIGUOUS values from 1 so the VM's
// `switch (op)` lowers to a jump table; never leave a hole.
import type { Signature } from './signature.ts'
import type { Value } from './value.ts'

// VM opcodes. Module-level `const` numbers (not a `const enum`, which
// Node's type-stripping rejects): V8 const-folds these into the dispatch
// `switch`, so cases stay integer literals and the switch lowers to a
// jump table. Keep values CONTIGUOUS from 1 (no holes) so that holds.
// Grows as later stages land (jumps, loops, user fns, …).
//
// Stage 1 (straight-line native calls) uses a locals-threaded layout:
// each call's result is stored to a frame local and re-pushed where a
// later call or the residual consumes it — simpler and equally
// V8-friendly versus the swap-scheduling eng/go/lower.go performs.
export const OpPushConst = 1 // push Consts[arg]
export const OpCallNative = 2 // pop sig.args.length (top = sig[0]), call handler, push results
export const OpStoreLocal = 3 // pop top into locals[arg]
export const OpPushLocal = 4 // push locals[arg]
export const OpJmpIfFalse = 5 // pop top; if coerceBoolean(v) is false, jump to abs pc `arg`
export const OpJmp = 6 // unconditional jump to abs pc `arg` (forward, or the loop back-edge)
export const OpForSetup = 7 // pop start,end,step (start on top); open a loop with iterator slot `arg`
export const OpForNext = 8 // if loop done, pop it and jump to abs pc `arg`; else bind locals[iterSlot]=cur, cur+=step
export const OpMakeList = 9 // pop `arg` values; push a list (deepest = element 0)
export const OpMakeMap = 10 // pop makeMaps[arg].length values; push a map keyed by makeMaps[arg] (value for keys[0] deepest)
export const OpTrap = 11 // raise the AqlError described by traps[arg] and abort (terminal)

/** An opcode is one of the Op* constants above. */
export type Op = number

/** One interned signature: the word plus the exact Signature selected. */
export interface SigRef {
  word: string
  sig: Signature
}

/**
 * One trap: the AQL error an OpTrap raises — the taxonomy code, the detail
 * message, and the word it is attributed to, taken verbatim from the
 * matching runtime error so the compiled stream errors byte-identically to
 * the interpreter. Mirrors eng/go bytecode.go::TrapSpec (Go's `hint` is
 * dropped — the TS AqlError carries only code/detail/word).
 */
export interface TrapSpec {
  code: string
  detail: string
  word: string
}

/** A compiled program: parallel-array code + the operand pools. */
export interface Program {
  /** Dense opcodes, one byte each; parallel to `args`. */
  readonly ops: Uint8Array
  /** One int32 per instruction: a pool index or (later) a jump target. */
  readonly args: Int32Array
  /** Constant pool, indexed by a PushConst arg. */
  readonly consts: readonly Value[]
  /** Signature pool, indexed by a CallNative arg. */
  readonly sigs: readonly SigRef[]
  /** Key lists for OpMakeMap, indexed by a MakeMap arg. */
  readonly makeMaps: readonly (readonly string[])[]
  /** Trap specs for OpTrap, indexed by a Trap arg. */
  readonly traps: readonly TrapSpec[]
  /** Operand-stack high-water mark — the preallocated stack capacity. */
  readonly maxStack: number
  /** Frame-local slot count — the preallocated locals capacity. */
  readonly numLocals: number
}

/**
 * A growable code buffer that freezes to the parallel typed arrays. Built
 * with plain number[] (cheap append) then compacted once — the typed
 * arrays are what the hot VM loop reads.
 */
export class CodeBuilder {
  private readonly ops: number[] = []
  private readonly args: number[] = []

  /** Append an instruction; returns its index (for jump backpatching). */
  emit(op: Op, arg: number): number {
    this.ops.push(op)
    this.args.push(arg)
    return this.ops.length - 1
  }

  /** Overwrite the arg of a previously-emitted instruction (jump target). */
  patch(index: number, arg: number): void {
    this.args[index] = arg
  }

  get length(): number {
    return this.ops.length
  }

  freeze(): { ops: Uint8Array; args: Int32Array } {
    return { ops: Uint8Array.from(this.ops), args: Int32Array.from(this.args) }
  }
}

/** Render a Program as human-readable assembly (for golden tests). */
export function disassemble(p: Program): string {
  const lines: string[] = []
  for (let pc = 0; pc < p.ops.length; pc++) {
    const op = p.ops[pc]!
    const arg = p.args[pc]!
    let text: string
    switch (op) {
      case OpPushConst:
        text = `PUSH_CONST ${arg}`
        break
      case OpCallNative:
        text = `CALL_NATIVE ${arg} (${p.sigs[arg]?.word ?? '?'})`
        break
      case OpStoreLocal:
        text = `STORE_LOCAL ${arg}`
        break
      case OpPushLocal:
        text = `PUSH_LOCAL ${arg}`
        break
      case OpJmpIfFalse:
        text = `JMP_IF_FALSE ${arg}`
        break
      case OpJmp:
        text = `JMP ${arg}`
        break
      case OpForSetup:
        text = `FOR_SETUP ${arg}`
        break
      case OpForNext:
        text = `FOR_NEXT ${arg}`
        break
      case OpMakeList:
        text = `MAKE_LIST ${arg}`
        break
      case OpMakeMap:
        text = `MAKE_MAP ${arg} {${(p.makeMaps[arg] ?? []).join(' ')}}`
        break
      case OpTrap:
        text = `TRAP ${arg} (${p.traps[arg]?.code ?? '?'})`
        break
      default:
        text = `OP_${op} ${arg}`
    }
    lines.push(`${pc.toString().padStart(3, '0')}  ${text}`)
  }
  return lines.join('\n')
}
